package transactions

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

var (
	_ TransactionRepository = (*repository.TransactionRepository)(nil)
	_ MemberRepository      = (*repository.MemberRepository)(nil)
	_ ShareRepository       = (*repository.ShareRepository)(nil)
	_ FineRepository        = (*repository.FineRepository)(nil)
)

type TransactionRepository interface {
	Create(ctx context.Context, transaction repository.Transaction, tx *sqlx.Tx) (*repository.Transaction, error)
	CreateStatus(ctx context.Context, transactionStatus repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
	GetStatus(ctx context.Context, filter repository.TransactionRepositoryFilter) (*repository.TransactionStatus, error)
	UpdateStatus(ctx context.Context, transactionStatus repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
	GetBalance(ctx context.Context, filter repository.TransactionRepositoryFilter) (int64, error)
	List(ctx context.Context, filter repository.TransactionRepositoryFilter, opts repository.QueryOptions) (*repository.ListResult[repository.PopTransaction], error)
}

type MemberRepository interface {
	Get(ctx context.Context, filter repository.MemberRepositoryFilter) (*repository.Member, error)
	Update(ctx context.Context, member *repository.Member, tx *sqlx.Tx) (*repository.Member, error)
}

type ShareRepository interface {
	Create(ctx context.Context, share repository.Share, tx *sqlx.Tx) (*repository.Share, error)
	CountTotalSharesPurchased(ctx context.Context, filter repository.ShareRepositoryFilter) (*repository.SharesTotalRows, error)
}

type FineRepository interface {
	Create(ctx context.Context, fine *repository.Fine, tx *sqlx.Tx) (*repository.Fine, error)
	Get(ctx context.Context, filter repository.FineRepositoryFilter) (*repository.Fine, error)
	Update(ctx context.Context, fine *repository.Fine, tx *sqlx.Tx) (*repository.Fine, error)
}

type Transaction struct {
	DB              *sqlx.DB
	TransactionRepo TransactionRepository
	MemberRepo      MemberRepository
	ShareRepo       ShareRepository
	FineRepo        FineRepository

	// Shares unit price management
	mu        sync.RWMutex
	unitPrice int64
}

func New(db *sqlx.DB, transRepo TransactionRepository, memberRepo MemberRepository, shareRepo ShareRepository, fineRepo FineRepository) *Transaction {
	return &Transaction{
		DB:              db,
		TransactionRepo: transRepo,
		MemberRepo:      memberRepo,
		ShareRepo:       shareRepo,
		FineRepo:        fineRepo,
	}
}

func (t *Transaction) UpdateStatus(ctx context.Context, id *uuid.UUID, input *dto.UpdateTransactionStatusInput, legder repository.LedgerType) (*dto.TransactionStatusResult, error) {
	status, err := t.TransactionRepo.GetStatus(ctx, repository.TransactionRepositoryFilter{
		ID:         id,
		LedgerType: lo.ToPtr(legder),
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &svc.ApiError{
				Status:  http.StatusNotFound,
				Message: "savings transaction not found",
			}
		}
		return nil, err
	}

	// Determine desired action
	if input.Confirmed == nil {
		return nil, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "confirmed field is required",
		}
	}

	wantConfirmed := *input.Confirmed
	if wantConfirmed && status.ConfirmedAt.Valid {
		return &dto.TransactionStatusResult{
			Confirmed: lo.ToPtr(true),
			Message:   "transaction already confirmed",
		}, nil
	}

	if !wantConfirmed && status.RejectedAt.Valid {
		return &dto.TransactionStatusResult{
			Confirmed: lo.ToPtr(false),
			Message:   "transaction already rejected",
		}, nil
	}

	// Conflict checks
	if wantConfirmed && status.RejectedAt.Valid {
		return nil, &svc.ApiError{
			Status:  http.StatusConflict,
			Message: "cannot confirm a rejected transaction",
		}
	}

	if !wantConfirmed && status.ConfirmedAt.Valid {
		return nil, &svc.ApiError{
			Status:  http.StatusConflict,
			Message: "cannot reject a confirmed transaction",
		}
	}

	var updateStatus repository.TransactionStatus
	updateStatus.TransactionID = *id
	if wantConfirmed {
		updateStatus.ConfirmedAt = sql.NullTime{Time: time.Now(), Valid: true}
	} else {
		updateStatus.RejectedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	updatedStatus, err := t.TransactionRepo.UpdateStatus(ctx, updateStatus, nil)
	if err != nil {
		return nil, err
	}

	result := &dto.TransactionStatusResult{
		Confirmed: lo.ToPtr(updatedStatus.ConfirmedAt.Valid),
	}
	if wantConfirmed {
		result.Message = "transaction confirmed successfully"
	} else {
		result.Message = "transaction rejected successfully"
	}

	return result, nil
}

// TODO: integrate a payment platform here but for now it would be manual
// CreateTransaction creates a generic transaction with status tracking
func (t *Transaction) CreateTransaction(ctx context.Context, params TransactionParams) (*dto.Transactions, error) {
	user := users.FromContext(ctx)
	if user.ID == uuid.Nil {
		return nil, &svc.ApiError{
			Status:  http.StatusUnauthorized,
			Message: "unauthenticated",
		}
	}

	member, err := t.getMemberByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	tx, err := t.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	transaction, status, err := t.createTransactionWithStatus(ctx, member.ID, params, tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return t.MapPopTransactionToDTO(&repository.PopTransaction{
		ID:          transaction.ID,
		MemberID:    transaction.MemberID,
		Description: transaction.Description,
		Reference:   transaction.Reference,
		Amount:      transaction.Amount,
		Type:        transaction.Type,
		CreatedAt:   transaction.CreatedAt,
		ConfirmedAt: status.ConfirmedAt,
		RejectedAt:  status.RejectedAt,
	}), nil
}

// Helper method to get member by user ID
func (t *Transaction) getMemberByUserID(ctx context.Context, userID uuid.UUID) (*repository.Member, error) {
	return t.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{
		UserID: &userID,
	})
}

// Helper method to create transaction with status
func (t *Transaction) createTransactionWithStatus(ctx context.Context, memberID uuid.UUID, params TransactionParams, tx *sqlx.Tx) (*repository.Transaction, *repository.TransactionStatus, error) {
	reference := lo.RandomString(12, lo.AlphanumericCharset)
	transaction, err := t.TransactionRepo.Create(ctx, repository.Transaction{
		MemberID:    memberID,
		Description: params.Input.Description,
		Amount:      params.Input.Amount,
		Reference:   reference,
		Type:        params.Type,
		Ledger:      params.LedgerType,
	}, tx)
	if err != nil {
		return nil, nil, err
	}

	status, err := t.TransactionRepo.CreateStatus(ctx, repository.TransactionStatus{
		TransactionID: transaction.ID,
	}, tx)
	if err != nil {
		return nil, nil, err
	}

	// Notify admins of new transaction (non-blocking)

	return transaction, status, nil
}

func (t *Transaction) GetBalance(ctx context.Context, legder repository.LedgerType) (int64, error) {
	user := users.FromContext(ctx)
	if user.ID == uuid.Nil {
		return 0, &svc.ApiError{
			Status:  http.StatusUnauthorized,
			Message: "unauthenticated",
		}
	}

	member, err := t.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{
		UserID: &user.ID,
	})
	if err != nil {
		return 0, err
	}

	totalDeposits, err := t.TransactionRepo.GetBalance(ctx, repository.TransactionRepositoryFilter{
		MemberID:   &member.ID,
		Type:       lo.ToPtr(repository.TransactionTypeDEPOSIT),
		Confirmed:  lo.ToPtr(true),
		LedgerType: lo.ToPtr(legder),
	})
	if err != nil {
		return 0, err
	}

	totalWithdrawals, err := t.TransactionRepo.GetBalance(ctx, repository.TransactionRepositoryFilter{
		MemberID:   &member.ID,
		Type:       lo.ToPtr(repository.TransactionTypeWITHDRAWAL),
		Confirmed:  lo.ToPtr(true),
		LedgerType: lo.ToPtr(legder),
	})
	if err != nil {
		return 0, err
	}

	return totalDeposits - totalWithdrawals, nil
}

func (t *Transaction) MapPopTransactionToDTO(pop *repository.PopTransaction) *dto.Transactions {
	var txnType dto.TransactionType
	switch pop.Type {
	case repository.TransactionTypeDEPOSIT:
		txnType = dto.TransactionTypeDeposit
	case repository.TransactionTypeWITHDRAWAL:
		txnType = dto.TransactionTypeWithdrawal
	default:
		// Fallback to deposit if unknown type
		txnType = dto.TransactionTypeDeposit
	}

	status := dto.SavingsStatusPending
	if pop.ConfirmedAt.Valid {
		status = dto.SavingsStatusConfirmed
	} else if pop.RejectedAt.Valid {
		status = dto.SavingsStatusRejected
	}

	createdAt := pop.CreatedAt.Time
	return &dto.Transactions{
		ID:          pop.ID,
		MemberID:    pop.MemberID,
		Description: pop.Description,
		Reference:   pop.Reference,
		Amount:      pop.Amount,
		Type:        txnType,
		Status:      status,
		CreatedAt:   createdAt,
	}
}
