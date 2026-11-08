package loans

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

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
)

type Loan struct {
	DB              *sqlx.DB
	LoanRepo        LoanRepository
	MemberRepo      MemberRepository
	TransactionRepo TransactionRepository
	ShareRepo       ShareRepository
}

func New(
	db *sqlx.DB,
	loanRepo LoanRepository,
	memberRepo MemberRepository,
	transactionRepo TransactionRepository,
	shareRepo ShareRepository,
) *Loan {
	return &Loan{DB: db, LoanRepo: loanRepo, MemberRepo: memberRepo, TransactionRepo: transactionRepo, ShareRepo: shareRepo}
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

func (l *Loan) Create(ctx context.Context, input dto.LoanInput) (*dto.Loan, error) {
	member, err := l.currentMember(ctx)
	if err != nil {
		return nil, err
	}
	input.Purpose = strings.TrimSpace(input.Purpose)
	if input.Amount <= 0 || input.DurationMonths <= 0 || input.Purpose == "" {
		return nil, apiError(http.StatusBadRequest, "amount, purpose and duration_months are required")
	}
	loan, err := l.LoanRepo.Create(ctx, &repository.Loan{
		MemberID: member.ID, RequestedAmount: input.Amount, Purpose: input.Purpose, RequestedDurationMonths: input.DurationMonths,
	}, nil)
	if err != nil {
		return nil, err
	}
	return l.Get(ctx, loan.ID)
}

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
	return l.LoanRepo.MapRepositoryToDTOModel(loan), nil
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
	return &dto.ListResponse[dto.Loan]{
		Items:      lo.Map(result.Items, func(item *repository.PopulatedLoan, _ int) dto.Loan { return *l.LoanRepo.MapRepositoryToDTOModel(item) }),
		NextCursor: result.NextCursor,
	}, nil
}

func apiError(status int, message string) error {
	return &svc.APIError{Status: status, Message: message}
}
