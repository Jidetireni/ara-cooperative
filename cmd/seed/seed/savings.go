package seed

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

func (s *Seed) createSavingsTransaction(ctx context.Context, seedTx SeedTransaction, memberID uuid.UUID, createdAt time.Time, confirmed bool) error {
	// Start database transaction
	dbTx, err := s.DB.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer dbTx.Rollback()

	// Create transaction
	reference := lo.RandomString(12, lo.AlphanumericCharset)
	transaction := repository.Transaction{
		MemberID:    memberID,
		Description: seedTx.Description,
		Amount:      seedTx.Amount,
		Reference:   reference,
		Type:        repository.TransactionTypeDEPOSIT,
		Ledger:      repository.LedgerTypeSAVINGS,
		CreatedAt: sql.NullTime{
			Time:  createdAt,
			Valid: true,
		},
	}

	createdTransaction, err := s.TransactionRepo.Create(ctx, transaction, dbTx)
	if err != nil {
		return fmt.Errorf("create transaction: %w", err)
	}

	// Create transaction status
	status := repository.TransactionStatus{
		TransactionID: createdTransaction.ID,
		CreatedAt: sql.NullTime{
			Time:  createdAt,
			Valid: true,
		},
	}

	// Set confirmed status if required
	if confirmed {
		status.ConfirmedAt = sql.NullTime{
			Time:  createdAt.Add(time.Minute * 30), // Confirmed 30 minutes later
			Valid: true,
		}
	}

	_, err = s.TransactionRepo.CreateStatus(ctx, status, dbTx)
	if err != nil {
		return fmt.Errorf("create transaction status: %w", err)
	}

	// Commit the transaction
	if err := dbTx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// Helper function to create multiple transactions for a member
func (s *Seed) createSavingsTransactionsForMember(ctx context.Context, memberID uuid.UUID, transactions []SeedTransaction) error {
	for i, transaction := range transactions {
		// Create transactions with different dates (spread over time)
		daysAgo := (len(transactions) - i) * 7 // Weekly intervals, newest first
		createdAt := time.Now().AddDate(0, 0, -daysAgo)
		confirmed := transaction.Confirmed

		err := s.createSavingsTransaction(ctx, transaction, memberID, createdAt, confirmed)
		if err != nil {
			return fmt.Errorf("create transaction %d for member %s: %w", i+1, memberID, err)
		}
	}
	return nil
}
