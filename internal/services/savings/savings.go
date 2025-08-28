package savings

import (
	"context"
	"time"

	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
	"github.com/Jidetireni/ara-cooperative.git/internal/helpers"
	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
	"github.com/jmoiron/sqlx"
)

var (
	_ SavingRepository      = (*repository.SavingRepository)(nil)
	_ TransactionRepository = (*repository.TransactionRepository)(nil)
)

type TransactionRepository interface {
	Create(ctx context.Context, transaction repository.Transaction, tx *sqlx.Tx) (*repository.Transaction, error)
}

type SavingRepository interface {
	CreateStatus(ctx context.Context, savingStatus repository.SavingsStatus, tx *sqlx.Tx) (*repository.SavingsStatus, error)
}

type Saving struct {
	DB              *sqlx.DB
	SavingRepo      SavingRepository
	TransactionRepo TransactionRepository
}

func New(db *sqlx.DB, savingRepo SavingRepository, transactionRepo TransactionRepository) *Saving {
	return &Saving{
		DB:              db,
		SavingRepo:      savingRepo,
		TransactionRepo: transactionRepo,
	}
}

func (s *Saving) Deposit(ctx context.Context, input dto.SavingsDepositInput) (dto.Savings, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return dto.Savings{}, err
	}
	defer tx.Rollback()

	reference := helpers.GenerateUniqueReference(input.MemberID, time.Now(), "savings_deposit")
	transaction, err := s.TransactionRepo.Create(ctx, repository.Transaction{
		MemberID:    input.MemberID,
		Description: input.Description,
		Amount:      input.Amount,
		Reference:   reference,
		Type:        repository.TransactionTypeDEPOSIT,
		Ledger:      repository.LedgerTypeSAVINGS,
	}, tx)
	if err != nil {
		return dto.Savings{}, err
	}

	savingsStatus, err := s.SavingRepo.CreateStatus(ctx, repository.SavingsStatus{
		TransactionID: transaction.ID,
	}, tx)
	if err != nil {
		return dto.Savings{}, err
	}

	//TODO: integrate a payment platform here but for now it would be manual
	if err := tx.Commit(); err != nil {
		return dto.Savings{}, err
	}

	return s.MapRepositoryToDTO(transaction, savingsStatus), nil
}

func (s *Saving) MapRepositoryToDTO(transaction *repository.Transaction, savingsStatus *repository.SavingsStatus) dto.Savings {

	status := dto.SavingsStatusPending
	if savingsStatus.ConfirmedAt.Valid {
		status = dto.SavingsStatusConfirmed
	} else if savingsStatus.RejectedAt.Valid {
		status = dto.SavingsStatusRejected
	} else {
		status = dto.SavingsStatusPending
	}

	return dto.Savings{
		TransactionID: transaction.ID,
		Amount:        transaction.Amount,
		Reference:     transaction.Reference,
		Status:        status,
		CreatedAt:     transaction.CreatedAt.Time,
	}
}
