package transactions

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/Jidetireni/ara-cooperative/pkg/cache"
	"github.com/Jidetireni/ara-cooperative/pkg/logger"
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

var (
	_ RedisPkg = (*cache.Redis)(nil)
)

type TransactionRepository interface {
	Create(ctx context.Context, transaction repository.Transaction, tx *sqlx.Tx) (*repository.Transaction, error)
	CreateStatus(ctx context.Context, transactionStatus repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
	GetStatus(ctx context.Context, filter repository.TransactionRepositoryFilter) (*repository.TransactionStatus, error)
	UpdateStatus(ctx context.Context, transactionStatus repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
	GetBalance(ctx context.Context, filter repository.TransactionRepositoryFilter) (int64, error)
	List(ctx context.Context, filter repository.TransactionRepositoryFilter, opts repository.QueryOptions) (*repository.ListResult[repository.PopTransaction], error)
	Get(ctx context.Context, filter repository.TransactionRepositoryFilter) (*repository.PopTransaction, error)
	MapRepositoryToDTO(txn *repository.Transaction, status *repository.TransactionStatus) *dto.Transactions
}

type MemberRepository interface {
	Get(ctx context.Context, filter repository.MemberRepositoryFilter) (*repository.Member, error)
	Update(ctx context.Context, member *repository.Member, tx *sqlx.Tx) (*repository.Member, error)
}

type ShareRepository interface {
	Create(ctx context.Context, share repository.Share, tx *sqlx.Tx) (*repository.Share, error)
	CountTotalSharesPurchased(ctx context.Context, filter repository.ShareRepositoryFilter) (*repository.SharesTotalRows, error)
	CreateUnitPrice(ctx context.Context, price int64, tx *sqlx.Tx) error
	GetUnitPrice(ctx context.Context) (int64, error)
	MapRepositoryToDTO(share *repository.Share, txn *repository.Transaction, status *repository.TransactionStatus) *dto.Shares
}

type FineRepository interface {
	Create(ctx context.Context, fine *repository.Fine, tx *sqlx.Tx) (*repository.Fine, error)
	Get(ctx context.Context, filter repository.FineRepositoryFilter, tx *sqlx.Tx) (*repository.Fine, error)
	Update(ctx context.Context, fine *repository.Fine, tx *sqlx.Tx) (*repository.Fine, error)
	MapRepositoryToDTO(fine *repository.Fine, txn *repository.Transaction, status *repository.TransactionStatus) *dto.Fine
	List(ctx context.Context, filter repository.FineRepositoryFilter, opts repository.QueryOptions) (*repository.ListResult[repository.Fine], error)
}

type RedisPkg interface {
	SetPrimitive(ctx context.Context, key string, value string, expiration time.Duration) error
	GetPrimitive(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

type Transaction struct {
	DB              *sqlx.DB
	TransactionRepo TransactionRepository
	MemberRepo      MemberRepository
	ShareRepo       ShareRepository
	FineRepo        FineRepository
	RedisPkg        RedisPkg
	Logger          *logger.Logger
}

func New(db *sqlx.DB, transRepo TransactionRepository, memberRepo MemberRepository, shareRepo ShareRepository, fineRepo FineRepository, redisPkg RedisPkg, logger *logger.Logger) *Transaction {
	return &Transaction{
		DB:              db,
		TransactionRepo: transRepo,
		MemberRepo:      memberRepo,
		ShareRepo:       shareRepo,
		FineRepo:        fineRepo,
		RedisPkg:        redisPkg,
		Logger:          logger,
	}
}

func (t *Transaction) UpdateStatus(ctx context.Context, id *uuid.UUID, input *dto.UpdateTransactionStatusInput) (*dto.TransactionStatusResult, error) {
	ledger := repository.LedgerType(input.LedgerType)
	status, err := t.TransactionRepo.GetStatus(ctx, repository.TransactionRepositoryFilter{
		StatusID:   id,
		LedgerType: lo.ToPtr(ledger),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &svc.APIError{
				Status:  http.StatusNotFound,
				Message: "transaction status not found",
			}
		}
		return nil, err
	}

	// Determine desired action
	if input.Confirmed == nil {
		return nil, &svc.APIError{
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
		return nil, &svc.APIError{
			Status:  http.StatusConflict,
			Message: "cannot confirm a rejected transaction",
		}
	}

	if !wantConfirmed && status.ConfirmedAt.Valid {
		return nil, &svc.APIError{
			Status:  http.StatusConflict,
			Message: "cannot reject a confirmed transaction",
		}
	}

	var confirmedAt sql.NullTime
	var rejectedAt sql.NullTime
	if wantConfirmed {
		confirmedAt = sql.NullTime{Time: time.Now(), Valid: true}
	} else {
		rejectedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	tx, err := t.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	updatedStatus, err := t.TransactionRepo.UpdateStatus(ctx, repository.TransactionStatus{
		ID:          status.ID,
		ConfirmedAt: confirmedAt,
		RejectedAt:  rejectedAt,
	}, tx)
	if err != nil {
		return nil, err
	}

	result := &dto.TransactionStatusResult{
		Confirmed: lo.ToPtr(updatedStatus.ConfirmedAt.Valid),
	}
	if wantConfirmed {
		result.Message = "transaction confirmed successfully"
		switch ledger {
		case repository.LedgerTypeREGISTRATIONFEE:
			txn, err := t.TransactionRepo.Get(ctx, repository.TransactionRepositoryFilter{
				ID: lo.ToPtr(updatedStatus.TransactionID),
			})
			if err != nil {
				return nil, err
			}
			member, err := t.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{
				ID: lo.ToPtr(txn.MemberID),
			})
			if err != nil {
				return nil, err
			}
			member.ActivatedAt = sql.NullTime{Time: time.Now(), Valid: true}
			_, err = t.MemberRepo.Update(ctx, member, tx)
			if err != nil {
				return nil, err
			}

		case repository.LedgerTypeFINES:
			txn, err := t.TransactionRepo.Get(ctx, repository.TransactionRepositoryFilter{
				ID: lo.ToPtr(updatedStatus.TransactionID),
			})
			if err != nil {
				return nil, err
			}
			fine, err := t.FineRepo.Get(ctx, repository.FineRepositoryFilter{
				TransactionID: &txn.ID,
			}, tx)
			if err != nil {
				return nil, err
			}
			fine.PaidAt = sql.NullTime{Time: time.Now(), Valid: true}
			_, err = t.FineRepo.Update(ctx, fine, tx)
			if err != nil {
				return nil, err
			}
		}
	} else {
		result.Message = "transaction rejected successfully"
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return result, nil
}

// TODO: integrate a payment platform here but for now it would be manual
// CreateTransaction creates a generic transaction with status tracking
func (t *Transaction) DepositSavings(ctx context.Context, input dto.TransactionsInput) (*dto.Transactions, error) {
	if input.Amount < MinSavingsDepositAmount {
		return nil, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "minimum savings deposit amount is NGN 100",
		}
	}

	return t.processDeposit(ctx, input, repository.LedgerTypeSAVINGS)
}

func (t *Transaction) DepositSpecial(ctx context.Context, input dto.TransactionsInput) (*dto.Transactions, error) {
	if input.Amount < MinSpecialDepositAmount {
		return nil, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "minimum special deposit amount is NGN 500",
		}
	}

	return t.processDeposit(ctx, input, repository.LedgerTypeSPECIALDEPOSIT)
}

func (t *Transaction) processDeposit(ctx context.Context, input dto.TransactionsInput, ledger repository.LedgerType) (*dto.Transactions, error) {
	actor, ok := users.FromContext(ctx)
	if !ok {
		return nil, svc.UnauthenticatedError()
	}

	member, err := t.getMemberByUserID(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	tx, err := t.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()
	transaction, status, err := t.createTransactionWithStatus(ctx, member.ID, TransactionParams{
		Input:      input,
		Type:       repository.TransactionTypeDEPOSIT,
		LedgerType: ledger,
	}, tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return t.TransactionRepo.MapRepositoryToDTO(transaction, status), nil
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

func (t *Transaction) GetSavingsBalance(ctx context.Context) (int64, error) {
	return t.getBalance(ctx, repository.LedgerTypeSAVINGS)
}

func (t *Transaction) GetSpecialDepositBalance(ctx context.Context) (int64, error) {
	return t.getBalance(ctx, repository.LedgerTypeSPECIALDEPOSIT)
}

func (t *Transaction) getBalance(ctx context.Context, ledger repository.LedgerType) (int64, error) {
	actor, ok := users.FromContext(ctx)
	if !ok {
		return 0, svc.UnauthenticatedError()
	}

	member, err := t.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{
		UserID: &actor.ID,
	})
	if err != nil {
		return 0, err
	}

	totalDeposits, err := t.TransactionRepo.GetBalance(ctx, repository.TransactionRepositoryFilter{
		MemberID:   &member.ID,
		Type:       lo.ToPtr(repository.TransactionTypeDEPOSIT),
		Confirmed:  lo.ToPtr(true),
		LedgerType: lo.ToPtr(ledger),
	})
	if err != nil {
		return 0, err
	}

	totalWithdrawals, err := t.TransactionRepo.GetBalance(ctx, repository.TransactionRepositoryFilter{
		MemberID:   &member.ID,
		Type:       lo.ToPtr(repository.TransactionTypeWITHDRAWAL),
		Confirmed:  lo.ToPtr(true),
		LedgerType: lo.ToPtr(ledger),
	})
	if err != nil {
		return 0, err
	}

	return totalDeposits - totalWithdrawals, nil
}
