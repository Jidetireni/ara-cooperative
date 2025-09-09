package savings

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/helpers"
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
)

type TransactionRepository interface {
	Create(ctx context.Context, transaction repository.Transaction, tx *sqlx.Tx) (*repository.PopTransaction, error)
	CreateStatus(ctx context.Context, transactionStatus repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
	GetStatus(ctx context.Context, filter repository.TransactionRepositoryFilter) (*repository.TransactionStatus, error)
	UpdateStatus(ctx context.Context, transactionStatus repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
	GetBalance(ctx context.Context, filter repository.TransactionRepositoryFilter) (int64, error)
}

type MemberRepository interface {
	Get(ctx context.Context, filter repository.MemberRepositoryFilter) (*repository.Member, error)
}

type Saving struct {
	DB              *sqlx.DB
	TransactionRepo TransactionRepository
	MemberRepo      MemberRepository
}

func New(db *sqlx.DB, transactionRepo TransactionRepository, memberRepo MemberRepository) *Saving {
	return &Saving{
		DB:              db,
		TransactionRepo: transactionRepo,
		MemberRepo:      memberRepo,
	}
}

func (s *Saving) Deposit(ctx context.Context, input dto.SavingsDepositInput) (*dto.Savings, error) {
	user := users.FromContext(ctx)
	member, err := s.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{
		UserID: &user.ID,
	})
	if err != nil {
		return &dto.Savings{}, err
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return &dto.Savings{}, err
	}
	defer tx.Rollback()

	reference := helpers.GenerateUniqueReference("savings_deposit")
	transaction, err := s.TransactionRepo.Create(ctx, repository.Transaction{
		MemberID:    member.ID,
		Description: input.Description,
		Amount:      input.Amount,
		Reference:   reference,
		Type:        repository.TransactionTypeDEPOSIT,
		Ledger:      repository.LedgerTypeSAVINGS,
	}, tx)
	if err != nil {
		return &dto.Savings{}, err
	}

	status, err := s.TransactionRepo.CreateStatus(ctx, repository.TransactionStatus{
		TransactionID: transaction.ID,
	}, tx)
	if err != nil {
		return &dto.Savings{}, err
	}

	//TODO: integrate a payment platform here but for now it would be manual
	if err := tx.Commit(); err != nil {
		return &dto.Savings{}, err
	}

	return s.MapRepositoryToDTO(&repository.PopTransaction{
		ID:          transaction.ID,
		Amount:      transaction.Amount,
		Description: transaction.Description,
		Type:        transaction.Type,
		Reference:   transaction.Reference,
		CreatedAt:   transaction.CreatedAt,
		ConfirmedAt: status.ConfirmedAt,
		RejectedAt:  status.RejectedAt,
	}), nil
}

func (s *Saving) Confirm(ctx context.Context, transactionID *uuid.UUID) (bool, error) {
	status, err := s.TransactionRepo.GetStatus(ctx, repository.TransactionRepositoryFilter{
		ID:         transactionID,
		LedgerType: repository.LedgerTypeSAVINGS,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return false, &svc.ApiError{
				Status:  404,
				Message: "savings not found",
			}
		}

		return false, err
	}

	if status.RejectedAt.Valid || status.ConfirmedAt.Valid {
		return false, &svc.ApiError{
			Status:  http.StatusConflict,
			Message: "savings has already been confirmed",
		}
	}

	updatedStatus, err := s.TransactionRepo.UpdateStatus(ctx, repository.TransactionStatus{
		TransactionID: *transactionID,
		ConfirmedAt:   sql.NullTime{Time: time.Now(), Valid: true},
	}, nil)
	if err != nil {
		return false, err
	}

	return updatedStatus.ConfirmedAt.Valid, nil
}

func (s *Saving) Reject(ctx context.Context, transactionID *uuid.UUID) (bool, error) {
	status, err := s.TransactionRepo.GetStatus(ctx, repository.TransactionRepositoryFilter{
		ID:         transactionID,
		LedgerType: repository.LedgerTypeSAVINGS,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return false, &svc.ApiError{
				Status:  404,
				Message: "savings not found",
			}
		}

		return false, err
	}

	if status.RejectedAt.Valid || status.ConfirmedAt.Valid {
		return false, &svc.ApiError{
			Status:  http.StatusConflict,
			Message: "savings has already been rejected",
		}
	}

	updatedStatus, err := s.TransactionRepo.UpdateStatus(ctx, repository.TransactionStatus{
		TransactionID: *transactionID,
		RejectedAt:    sql.NullTime{Time: time.Now(), Valid: true},
	}, nil)
	if err != nil {
		return false, err
	}

	return updatedStatus.RejectedAt.Valid, nil
}

func (s *Saving) GetBalance(ctx context.Context) (int64, error) {
	user := users.FromContext(ctx)
	member, err := s.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{
		UserID: &user.ID,
	})
	if err != nil {
		return 0, err
	}

	totalDeposits, err := s.TransactionRepo.GetBalance(ctx, repository.TransactionRepositoryFilter{
		MemberID:   &member.ID,
		Type:       lo.ToPtr(repository.TransactionTypeDEPOSIT),
		Confirmed:  lo.ToPtr(true),
		LedgerType: repository.LedgerTypeSAVINGS,
	})
	if err != nil {
		return 0, err
	}

	totalWithdrawals, err := s.TransactionRepo.GetBalance(ctx, repository.TransactionRepositoryFilter{
		MemberID:   &member.ID,
		Type:       lo.ToPtr(repository.TransactionTypeWITHDRAWAL),
		Confirmed:  lo.ToPtr(true),
		LedgerType: repository.LedgerTypeSAVINGS,
	})
	if err != nil {
		return 0, err
	}

	return totalDeposits - totalWithdrawals, nil
}

func (s *Saving) MapRepositoryToDTO(src *repository.PopTransaction) *dto.Savings {
	var txnType dto.TransactionType
	if src.Type == repository.TransactionTypeDEPOSIT {
		txnType = dto.TransactionTypeDeposit
	} else {
		txnType = dto.TransactionTypeWithdrawal
	}

	status := dto.SavingsStatusPending
	if src.ConfirmedAt.Valid {
		status = dto.SavingsStatusConfirmed
	} else if src.RejectedAt.Valid {
		status = dto.SavingsStatusRejected
	}

	createdAt := src.CreatedAt.Time

	return &dto.Savings{
		TransactionID:   src.ID,
		Amount:          src.Amount,
		Description:     src.Description,
		TransactionType: txnType,
		Reference:       src.Reference,
		Status:          status,
		CreatedAt:       createdAt,
	}
}
