package loans

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

var decimalPattern = regexp.MustCompile(`^[0-9]{1,14}(\.[0-9]{1,4})?$`)

// decimal accepts exact, non-negative decimals with the database's precision.
func decimal(value string) (string, *big.Rat, error) {
	value = strings.TrimSpace(value)
	if !decimalPattern.MatchString(value) {
		return "", nil, apiError(http.StatusBadRequest, "use a non-negative decimal with at most four decimal places")
	}
	number, ok := new(big.Rat).SetString(value)
	if !ok {
		return "", nil, apiError(http.StatusBadRequest, "invalid decimal")
	}
	return number.FloatString(4), number, nil
}

func mapSettings(settings *repository.LoanSetting) *dto.LoanSettings {
	result := &dto.LoanSettings{MinimumSavings: settings.MinimumSavings, MinimumShareUnits: settings.MinimumShareUnits}
	if settings.UpdatedBy.Valid {
		result.UpdatedBy = &settings.UpdatedBy.UUID
	}
	if settings.UpdatedAt.Valid {
		result.UpdatedAt = &settings.UpdatedAt.Time
	}
	return result
}

func (l *Loan) Settings(ctx context.Context) (*dto.LoanSettings, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	settings, err := l.LoanRepo.GetSettings(ctx, nil)
	if err != nil {
		return nil, err
	}
	return mapSettings(settings), nil
}

func (l *Loan) UpdateSettings(ctx context.Context, input dto.LoanSettingsInput) (*dto.LoanSettings, error) {
	actor, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	units, _, err := decimal(input.MinimumShareUnits)
	if err != nil {
		return nil, err
	}
	if input.MinimumSavings < 0 {
		return nil, apiError(http.StatusBadRequest, "minimum_savings must be non-negative")
	}
	settings, err := l.LoanRepo.UpdateSettings(ctx, &repository.LoanSetting{
		ID: 1, MinimumSavings: input.MinimumSavings, MinimumShareUnits: units, UpdatedBy: uuid.NullUUID{UUID: actor.ID, Valid: true},
	}, nil)
	if err != nil {
		return nil, err
	}
	return mapSettings(settings), nil
}

func (l *Loan) Eligibility(ctx context.Context, id uuid.UUID) (*dto.LoanEligibility, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	loan, err := l.LoanRepo.Get(ctx, repository.LoanRepositoryFilter{ID: &id}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, err
	}
	return l.eligibility(ctx, loan, nil)
}

func (l *Loan) eligibility(ctx context.Context, loan *repository.Loan, tx *sqlx.Tx) (*dto.LoanEligibility, error) {
	settings, err := l.LoanRepo.GetSettings(ctx, tx)
	if err != nil {
		return nil, err
	}
	savings, err := l.TransactionRepo.GetBalance(ctx, repository.TransactionRepositoryFilter{
		MemberID: &loan.MemberID, LedgerType: lo.ToPtr(repository.LedgerTypeSAVINGS),
		Type: lo.ToPtr(repository.TransactionTypeDEPOSIT), Confirmed: lo.ToPtr(true), Rejected: lo.ToPtr(false),
	})
	if err != nil {
		return nil, err
	}
	shares, err := l.ShareRepo.CountTotalSharesPurchased(ctx, repository.ShareRepositoryFilter{
		MemberID: &loan.MemberID, LedgerType: lo.ToPtr(repository.LedgerTypeSHARES), Confirmed: lo.ToPtr(true),
	})
	if err != nil {
		return nil, err
	}
	result := &dto.LoanEligibility{
		MemberID: loan.MemberID, Savings: savings, ShareUnits: shares.Units,
		MinimumSavings: settings.MinimumSavings, MinimumShareUnits: settings.MinimumShareUnits,
		Flags: []dto.LoanAdvisoryFlag{}, CheckedAt: time.Now(),
	}
	for _, item := range []struct {
		status *string
		target *int64
	}{
		{target: &result.PastApplications}, {status: lo.ToPtr("PENDING"), target: &result.OtherPendingApplications},
		{status: lo.ToPtr("APPROVED"), target: &result.ApprovedUndisbursedLoans}, {status: lo.ToPtr("ACTIVE"), target: &result.ActiveLoans},
	} {
		*item.target, err = l.LoanRepo.Count(ctx, repository.LoanRepositoryFilter{
			MemberID: &loan.MemberID, ExcludeID: &loan.ID, Status: item.status,
		}, tx)
		if err != nil {
			return nil, err
		}
	}
	flag := func(code, message string) {
		result.Flags = append(result.Flags, dto.LoanAdvisoryFlag{Code: code, Message: message})
	}
	if savings < settings.MinimumSavings {
		flag("INSUFFICIENT_SAVINGS", "Savings are below the configured minimum")
	}
	_, actual, err := decimal(shares.Units)
	if err != nil {
		return nil, err
	}
	_, minimum, err := decimal(settings.MinimumShareUnits)
	if err != nil {
		return nil, err
	}
	if actual.Cmp(minimum) < 0 {
		flag("INSUFFICIENT_SHARES", "Shareholding is below the configured minimum")
	}
	if result.OtherPendingApplications > 0 {
		flag("OTHER_PENDING_APPLICATIONS", "Member has other pending applications")
	}
	if result.ApprovedUndisbursedLoans > 0 {
		flag("APPROVED_UNDISBURSED_LOANS", "Member has approved loans awaiting disbursement")
	}
	if result.ActiveLoans > 0 {
		flag("ACTIVE_LOANS", "Member has active loans; repayment status is not tracked in this version")
	}
	if err := l.byelawChecks(ctx, loan, result, actual, flag, tx); err != nil {
		return nil, err
	}
	return result, nil
}

// Bye-law limits. They raise advisory flags; an admin may still approve by
// recording an override reason. The interest cap is enforced on approval.
const (
	byelawMinimumShareUnits    = 50 // bye-law 46: every member holds at least 50 shares
	byelawMembershipMonths     = 6  // bye-law 65: no loan within 6 months of admission
	byelawMaxRepaymentMonths   = 12 // bye-law 71: repay within twelve months
	byelawMaxAnnualInterestPct = 15 // bye-law 67: at most 15 percent per annum
	byelawGuarantorsRequired   = 2  // bye-law 68: two sureties who are members
)

func (l *Loan) byelawChecks(
	ctx context.Context, loan *repository.Loan, result *dto.LoanEligibility, shareUnits *big.Rat,
	flag func(code, message string), tx *sqlx.Tx,
) error {
	member, err := l.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{ID: &loan.MemberID}, tx)
	if err != nil {
		return err
	}
	if member.ActivatedAt.Valid {
		result.MembershipActivatedAt = &member.ActivatedAt.Time
		if loan.CreatedAt.Before(member.ActivatedAt.Time.AddDate(0, byelawMembershipMonths, 0)) {
			flag("MEMBERSHIP_UNDER_SIX_MONTHS", "Applied within 6 months of membership activation (bye-law 65)")
		}
	}
	if shareUnits.Cmp(big.NewRat(byelawMinimumShareUnits, 1)) < 0 {
		flag("BELOW_BYELAW_SHARE_MINIMUM", "Holds fewer than the 50 shares every member must hold (bye-law 46)")
	}
	if loan.RequestedDurationMonths > byelawMaxRepaymentMonths ||
		(loan.RepaymentDurationMonths.Valid && loan.RepaymentDurationMonths.Int32 > byelawMaxRepaymentMonths) {
		flag("REPAYMENT_OVER_TWELVE_MONTHS", "Requested repayment runs past 12 months (bye-law 71)")
	}
	guarantors, err := l.LoanRepo.ListGuarantors(ctx, repository.LoanGuarantorRepositoryFilter{LoanID: &loan.ID}, tx)
	if err != nil {
		return err
	}
	result.Guarantors = []dto.LoanGuarantorCheck{}
	if len(guarantors) < byelawGuarantorsRequired {
		flag("GUARANTORS_MISSING", "Fewer than two guarantors are named (bye-law 68)")
	}
	var notAccepted, inactive, onLoan, indebted bool
	for _, g := range guarantors {
		check, err := l.guarantorCheck(ctx, g, tx)
		if err != nil {
			return err
		}
		result.Guarantors = append(result.Guarantors, *check)
		notAccepted = notAccepted || check.Status != "ACCEPTED"
		inactive = inactive || !check.IsActive
		onLoan = onLoan || check.OpenLoans > 0
		indebted = indebted || check.UnpaidFines > 0
	}
	if notAccepted {
		flag("GUARANTOR_NOT_ACCEPTED", "A guarantor has not accepted the guarantee")
	}
	if inactive {
		flag("GUARANTOR_NOT_ACTIVE", "A guarantor is not an active member (bye-law 68)")
	}
	if onLoan {
		flag("GUARANTOR_ON_LOAN", "A guarantor has an approved or active loan; guarantors must not be on loan")
	}
	if indebted {
		flag("GUARANTOR_INDEBTED", "A guarantor has unpaid fines; guarantors must not be indebted to the cooperative")
	}
	return nil
}

// guarantorCheck reports a guarantor's savings, shares and obligations, as the
// form's "Total Shares and Savings in ARA Cooperative" line.
func (l *Loan) guarantorCheck(ctx context.Context, g *repository.PopulatedLoanGuarantor, tx *sqlx.Tx) (*dto.LoanGuarantorCheck, error) {
	savings, err := l.TransactionRepo.GetBalance(ctx, repository.TransactionRepositoryFilter{
		MemberID: &g.MemberID, LedgerType: lo.ToPtr(repository.LedgerTypeSAVINGS),
		Type: lo.ToPtr(repository.TransactionTypeDEPOSIT), Confirmed: lo.ToPtr(true), Rejected: lo.ToPtr(false),
	})
	if err != nil {
		return nil, err
	}
	shares, err := l.ShareRepo.CountTotalSharesPurchased(ctx, repository.ShareRepositoryFilter{
		MemberID: &g.MemberID, LedgerType: lo.ToPtr(repository.LedgerTypeSHARES), Confirmed: lo.ToPtr(true),
	})
	if err != nil {
		return nil, err
	}
	var openLoans int64
	for _, status := range []string{"APPROVED", "ACTIVE"} {
		count, err := l.LoanRepo.Count(ctx, repository.LoanRepositoryFilter{MemberID: &g.MemberID, Status: lo.ToPtr(status)}, tx)
		if err != nil {
			return nil, err
		}
		openLoans += count
	}
	unpaidFines, err := l.FineRepo.Count(ctx, repository.FineRepositoryFilter{MemberID: &g.MemberID, Paid: lo.ToPtr(false)}, tx)
	if err != nil {
		return nil, err
	}
	name := strings.Join(lo.Compact([]string{g.Member.FirstName, g.Member.MiddleName.String, g.Member.LastName}), " ")
	return &dto.LoanGuarantorCheck{
		MemberID: g.MemberID, Slug: g.Member.Slug, Name: name, Status: g.Status, IsActive: g.Member.ActivatedAt.Valid,
		Savings: savings, ShareUnits: shares.Units, OpenLoans: openLoans, UnpaidFines: unpaidFines,
	}, nil
}
