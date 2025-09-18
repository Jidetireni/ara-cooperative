package transactions

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

var _ TransactionRepository = (*repository.TransactionRepository)(nil)

type TransactionRepository interface {
	CreateStatus(ctx context.Context, transactionStatus repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
	GetStatus(ctx context.Context, filter repository.TransactionRepositoryFilter) (*repository.TransactionStatus, error)
	UpdateStatus(ctx context.Context, transactionStatus repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
}

type Transaction struct {
	DB              *sqlx.DB
	TransactionRepo TransactionRepository
}

func New(db *sqlx.DB, transRepo TransactionRepository) *Transaction {
	return &Transaction{
		DB:              db,
		TransactionRepo: transRepo,
	}
}

func (t *Transaction) UpdateStatus(ctx context.Context, id *uuid.UUID, input *dto.UpdateTransactionStatusInput) (*dto.TransactionStatusResult, error) {
	status, err := t.TransactionRepo.GetStatus(ctx, repository.TransactionRepositoryFilter{
		ID:         id,
		LedgerType: repository.LedgerTypeSAVINGS,
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
