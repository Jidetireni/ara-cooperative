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
	return result, nil
}
