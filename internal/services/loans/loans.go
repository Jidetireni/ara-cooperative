package loans

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type LoanRepository interface {
	Get(ctx context.Context, filter repository.LoanRepositoryFilter, tx *sqlx.Tx) (*repository.Loan, error)
	GetPopulated(ctx context.Context, filter repository.LoanRepositoryFilter, tx *sqlx.Tx) (*repository.PopulatedLoan, error)
	ListPopulated(ctx context.Context, filter repository.LoanRepositoryFilter, opts repository.QueryOptions) (*repository.ListResult[repository.PopulatedLoan], error)
	Count(ctx context.Context, filter repository.LoanRepositoryFilter, tx *sqlx.Tx) (int64, error)
	Create(ctx context.Context, loan *repository.Loan, tx *sqlx.Tx) (*repository.Loan, error)
	Update(ctx context.Context, loan *repository.Loan, tx *sqlx.Tx) (*repository.Loan, error)
	GetSettings(ctx context.Context, tx *sqlx.Tx) (*repository.LoanSetting, error)
	UpdateSettings(ctx context.Context, settings *repository.LoanSetting, tx *sqlx.Tx) (*repository.LoanSetting, error)
	MapRepositoryToDTOModel(loan *repository.PopulatedLoan) *dto.Loan
	ListGuarantors(ctx context.Context, filter repository.LoanGuarantorRepositoryFilter, tx *sqlx.Tx) ([]*repository.PopulatedLoanGuarantor, error)
	ListGuarantorRequests(ctx context.Context, memberID uuid.UUID, opts repository.QueryOptions) (*repository.ListResult[repository.PopulatedLoanGuarantor], error)
	CreateGuarantors(ctx context.Context, loanID uuid.UUID, memberIDs []uuid.UUID, tx *sqlx.Tx) error
	UpdateGuarantorStatus(ctx context.Context, id uuid.UUID, status string, respondedAt time.Time, tx *sqlx.Tx) error
}

// BankResolver confirms payout accounts with the bank (Paystack).
type BankResolver interface {
	Resolve(ctx context.Context, bankCode, accountNumber string) (*dto.BankAccount, error)
}

type FineRepository interface {
	Count(ctx context.Context, filter repository.FineRepositoryFilter, tx *sqlx.Tx) (int64, error)
}

type MemberRepository interface {
	Get(ctx context.Context, filter repository.MemberRepositoryFilter, tx *sqlx.Tx) (*repository.Member, error)
}

type TransactionRepository interface {
	GetBalance(ctx context.Context, filter repository.TransactionRepositoryFilter) (int64, error)
	Create(ctx context.Context, transaction repository.Transaction, tx *sqlx.Tx) (*repository.Transaction, error)
	CreateStatus(ctx context.Context, status repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
}

type ShareRepository interface {
	CountTotalSharesPurchased(ctx context.Context, filter repository.ShareRepositoryFilter) (*repository.SharesTotalRows, error)
}

var (
	_ LoanRepository        = (*repository.LoanRepository)(nil)
	_ MemberRepository      = (*repository.MemberRepository)(nil)
	_ TransactionRepository = (*repository.TransactionRepository)(nil)
	_ ShareRepository       = (*repository.ShareRepository)(nil)
	_ FineRepository        = (*repository.FineRepository)(nil)
)

type Loan struct {
	DB              *sqlx.DB
	LoanRepo        LoanRepository
	MemberRepo      MemberRepository
	TransactionRepo TransactionRepository
	ShareRepo       ShareRepository
	FineRepo        FineRepository
	Banks           BankResolver
}

func New(
	db *sqlx.DB,
	loanRepo LoanRepository,
	memberRepo MemberRepository,
	transactionRepo TransactionRepository,
	shareRepo ShareRepository,
	fineRepo FineRepository,
	banks BankResolver,
) *Loan {
	return &Loan{
		DB: db, LoanRepo: loanRepo, MemberRepo: memberRepo, TransactionRepo: transactionRepo,
		ShareRepo: shareRepo, FineRepo: fineRepo, Banks: banks,
	}
}

func requireAdmin(ctx context.Context) (*users.UserContextValue, error) {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	if !actor.IsAuthenticatedAsAdmin {
		return nil, apiError(http.StatusForbidden, "admin access required")
	}
	return actor, nil
}

func (l *Loan) currentMember(ctx context.Context) (*repository.Member, error) {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	if !actor.IsAuthenticatedAsMember {
		return nil, apiError(http.StatusForbidden, "member access required")
	}
	member, err := l.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{UserID: &actor.ID}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, err
	}
	if !member.ActivatedAt.Valid {
		return nil, apiError(http.StatusForbidden, "active membership required")
	}
	return member, nil
}

func (l *Loan) Create(ctx context.Context, input dto.LoanInput) (result *dto.Loan, err error) {
	member, err := l.currentMember(ctx)
	if err != nil {
		return nil, err
	}
	application, guarantorNumbers, err := normalizeApplication(input)
	if err != nil {
		return nil, err
	}
	// Ask the bank who owns the payout account before opening the transaction;
	// the bank's name for the holder is what admins see at disbursement.
	account, err := l.Banks.Resolve(ctx, input.DisbursementBankCode, application.DisbursementAccountNumber.String)
	if err != nil {
		return nil, err
	}
	application.DisbursementBankCode = sql.NullString{String: account.BankCode, Valid: true}
	application.DisbursementBankName = sql.NullString{String: account.BankName, Valid: true}
	application.DisbursementAccountName = sql.NullString{String: account.AccountName, Valid: true}
	application.MemberID = member.ID
	tx, err := l.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	guarantorIDs := make([]uuid.UUID, 0, len(guarantorNumbers))
	for _, number := range guarantorNumbers {
		guarantor, err := l.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{Slug: lo.ToPtr(number)}, tx)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apiError(http.StatusBadRequest, "no member has membership number "+number)
		}
		if err != nil {
			return nil, err
		}
		if guarantor.ID == member.ID {
			return nil, apiError(http.StatusBadRequest, "you cannot be your own guarantor")
		}
		guarantorIDs = append(guarantorIDs, guarantor.ID)
	}
	loan, err := l.LoanRepo.Create(ctx, application, tx)
	if err != nil {
		return nil, err
	}
	if err := l.LoanRepo.CreateGuarantors(ctx, loan.ID, guarantorIDs, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return l.Get(ctx, loan.ID)
}

// normalizeApplication checks the loan form and returns the loan row plus the
// guarantors' membership numbers.
func normalizeApplication(input dto.LoanInput) (*repository.Loan, []string, error) {
	input.Purpose = strings.TrimSpace(input.Purpose)
	if input.Amount <= 0 || input.DurationMonths <= 0 || input.Purpose == "" {
		return nil, nil, apiError(http.StatusBadRequest, "amount, purpose and duration_months are required")
	}
	start, err := time.Parse(time.DateOnly, strings.TrimSpace(input.RepaymentStartDate))
	if err != nil {
		return nil, nil, apiError(http.StatusBadRequest, "repayment_start_date must be a date in YYYY-MM-DD format")
	}
	if start.Before(time.Now().Truncate(24 * time.Hour)) {
		return nil, nil, apiError(http.StatusBadRequest, "repayment_start_date cannot be in the past")
	}
	if input.RepaymentDurationMonths <= 0 || input.MonthlyRepaymentAmount <= 0 {
		return nil, nil, apiError(http.StatusBadRequest, "repayment_duration_months and monthly_repayment_amount are required")
	}
	bankCode := strings.TrimSpace(input.DisbursementBankCode)
	accountNumber := strings.TrimSpace(input.DisbursementAccountNumber)
	if bankCode == "" || !accountNumberPattern.MatchString(accountNumber) {
		return nil, nil, apiError(http.StatusBadRequest, "choose a bank and enter a 10-digit account number")
	}
	numbers := lo.Map(input.GuarantorMemberNumbers, func(n string, _ int) string { return strings.ToLower(strings.TrimSpace(n)) })
	if len(numbers) != 2 || lo.Contains(numbers, "") || numbers[0] == numbers[1] {
		return nil, nil, apiError(http.StatusBadRequest, "name two different guarantors by membership number")
	}
	if !input.DeclarationAccepted {
		return nil, nil, apiError(http.StatusBadRequest, "accept the authorization and understanding declaration to apply")
	}
	return &repository.Loan{
		RequestedAmount: input.Amount, Purpose: input.Purpose, RequestedDurationMonths: input.DurationMonths,
		RepaymentStartDate:        sql.NullTime{Time: start, Valid: true},
		RepaymentDurationMonths:   sql.NullInt32{Int32: input.RepaymentDurationMonths, Valid: true},
		MonthlyRepaymentAmount:    sql.NullInt64{Int64: input.MonthlyRepaymentAmount, Valid: true},
		DisbursementAccountNumber: sql.NullString{String: accountNumber, Valid: true},
		DeclarationAcceptedAt:     sql.NullTime{Time: time.Now(), Valid: true},
	}, numbers, nil
}

var accountNumberPattern = regexp.MustCompile(`^[0-9]{10}$`)

func (l *Loan) Get(ctx context.Context, id uuid.UUID) (*dto.Loan, error) {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	filter := repository.LoanRepositoryFilter{ID: &id}
	if !actor.IsAuthenticatedAsAdmin {
		member, err := l.currentMember(ctx)
		if err != nil {
			return nil, err
		}
		filter.MemberID = &member.ID
	}
	loan, err := l.LoanRepo.GetPopulated(ctx, filter, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, err
	}
	result := []*dto.Loan{l.LoanRepo.MapRepositoryToDTOModel(loan)}
	if err := l.attachGuarantors(ctx, result); err != nil {
		return nil, err
	}
	return result[0], nil
}

// attachGuarantors loads the guarantors for a page of loans in one query.
func (l *Loan) attachGuarantors(ctx context.Context, loans []*dto.Loan) error {
	if len(loans) == 0 {
		return nil
	}
	ids := lo.Map(loans, func(loan *dto.Loan, _ int) uuid.UUID { return loan.ID })
	guarantors, err := l.LoanRepo.ListGuarantors(ctx, repository.LoanGuarantorRepositoryFilter{LoanIDs: ids}, nil)
	if err != nil {
		return err
	}
	byLoan := lo.GroupBy(guarantors, func(g *repository.PopulatedLoanGuarantor) uuid.UUID { return g.LoanID })
	for _, loan := range loans {
		loan.Guarantors = lo.Map(byLoan[loan.ID], func(g *repository.PopulatedLoanGuarantor, _ int) dto.LoanGuarantor {
			return mapGuarantor(g)
		})
	}
	return nil
}

func memberCard(member repository.Member) dto.MemberCard {
	return dto.MemberCard{
		ID: member.ID, Slug: member.Slug, FirstName: member.FirstName, MiddleName: member.MiddleName.String,
		LastName: member.LastName, IsActive: member.ActivatedAt.Valid,
	}
}

func mapGuarantor(g *repository.PopulatedLoanGuarantor) dto.LoanGuarantor {
	result := dto.LoanGuarantor{
		Position: g.Position, Member: memberCard(g.Member), Address: g.Member.Address.String, Status: g.Status,
	}
	if g.RespondedAt.Valid {
		result.RespondedAt = &g.RespondedAt.Time
	}
	return result
}

func (l *Loan) List(
	ctx context.Context, filters dto.LoanFilters, opts *dto.QueryOptions,
) (*dto.ListResponse[dto.Loan], error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	return l.list(ctx, filters, opts)
}

func (l *Loan) ListMine(
	ctx context.Context, filters dto.LoanFilters, opts *dto.QueryOptions,
) (*dto.ListResponse[dto.Loan], error) {
	member, err := l.currentMember(ctx)
	if err != nil {
		return nil, err
	}
	filters.MemberID, filters.Search = &member.ID, nil
	return l.list(ctx, filters, opts)
}

func (l *Loan) list(
	ctx context.Context, filters dto.LoanFilters, opts *dto.QueryOptions,
) (*dto.ListResponse[dto.Loan], error) {
	if filters.Status != nil {
		switch *filters.Status {
		case "PENDING", "APPROVED", "REJECTED", "ACTIVE":
		default:
			return nil, apiError(http.StatusBadRequest, "status must be PENDING, APPROVED, REJECTED or ACTIVE")
		}
	}
	if filters.Search != nil && len(*filters.Search) > 100 {
		return nil, apiError(http.StatusBadRequest, "search must be at most 100 characters")
	}
	result, err := l.LoanRepo.ListPopulated(ctx, repository.LoanRepositoryFilter{
		MemberID: filters.MemberID, Status: filters.Status, Search: filters.Search,
	}, repository.QueryOptions{Limit: opts.Limit, Cursor: opts.Cursor, Sort: opts.Sort})
	if err != nil {
		return nil, err
	}
	items := lo.Map(result.Items, func(item *repository.PopulatedLoan, _ int) *dto.Loan { return l.LoanRepo.MapRepositoryToDTOModel(item) })
	if err := l.attachGuarantors(ctx, items); err != nil {
		return nil, err
	}
	return &dto.ListResponse[dto.Loan]{
		Items:      lo.Map(items, func(item *dto.Loan, _ int) dto.Loan { return *item }),
		NextCursor: result.NextCursor,
	}, nil
}

func apiError(status int, message string) error {
	return &svc.APIError{Status: status, Message: message}
}
