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
	"github.com/google/uuid"
)

func (t *Transaction) ChargeFine(ctx context.Context, input *dto.FineInput) (*dto.Fine, error) {
	user := users.FromContext(ctx)
	if user.ID == uuid.Nil {
		return nil, &svc.ApiError{
			Status:  http.StatusUnauthorized,
			Message: "unauthenticated",
		}
	}

	// Ensure member exists (avoid FK violation)
	_, err := t.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{ID: &input.MemberID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &svc.ApiError{Status: http.StatusNotFound, Message: "member not found"}
		}
		return nil, err
	}

	// Parse deadline (RFC3339 string)
	deadline, err := time.Parse(time.RFC3339, input.Deadline)
	if err != nil {
		return nil, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "invalid deadline format (use RFC3339)",
		}
	}

	created, err := t.FineRepo.Create(ctx, &repository.Fine{
		AdminID:  user.ID,
		MemberID: input.MemberID,
		Amount:   input.Amount,
		Reason:   input.Reason,
		Deadline: deadline,
	}, nil)
	if err != nil {
		return nil, err
	}

	// Notify member via email (non-blocking)

	return &dto.Fine{
		ID:            created.ID,
		MemberID:      created.MemberID,
		Amount:        created.Amount,
		TransactionID: created.TransactionID.UUID,
		Reason:        created.Reason,
		Deadline:      created.Deadline,
		Paid:          created.PaidAt.Valid,
		CreatedAt:     created.CreatedAt,
	}, nil
}

func (t *Transaction) PayFine(ctx context.Context, fineID uuid.UUID, txInput *dto.TransactionsInput) (*dto.Fine, error) {
	user := users.FromContext(ctx)
	if user.ID == uuid.Nil {
		return nil, &svc.ApiError{
			Status:  http.StatusUnauthorized,
			Message: "unauthenticated",
		}
	}

	// Resolve member from user
	member, err := t.getMemberByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	// Fetch fine and validate ownership
	fine, err := t.FineRepo.Get(ctx, repository.FineRepositoryFilter{
		ID:       &fineID,
		MemberID: &member.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &svc.ApiError{
				Status:  http.StatusNotFound,
				Message: "fine not found",
			}
		}
		return nil, err
	}

	if fine.PaidAt.Valid {
		return nil, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "fine already paid",
		}
	}

	if fine.Amount != txInput.Amount {
		return nil, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "payment amount does not match fine amount",
		}
	}

	description := "Fine payment"
	if txInput.Description != "" {
		description = txInput.Description
	}

	tx, err := t.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = tx.Rollback()
	}()

	createdTx, _, err := t.createTransactionWithStatus(ctx, member.ID, TransactionParams{
		Input: dto.TransactionsInput{
			Amount:      fine.Amount,
			Description: description,
		},
		Type:       repository.TransactionTypeDEPOSIT,
		LedgerType: repository.LedgerTypeFINES,
	}, tx)
	if err != nil {
		return nil, err
	}

	_, err = t.TransactionRepo.UpdateStatus(ctx, repository.TransactionStatus{
		TransactionID: createdTx.ID,
		ConfirmedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}, tx)
	if err != nil {
		return nil, err
	}

	updatedFine, err := t.FineRepo.Update(ctx, &repository.Fine{
		ID:            fine.ID,
		MemberID:      fine.MemberID,
		AdminID:       fine.AdminID,
		Amount:        fine.Amount,
		TransactionID: repository.ToNullUUID(createdTx.ID),
		Reason:        fine.Reason,
		Deadline:      fine.Deadline,
		PaidAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}, tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &dto.Fine{
		ID:            fine.ID,
		MemberID:      fine.MemberID,
		Amount:        fine.Amount,
		TransactionID: createdTx.ID,
		Reason:        fine.Reason,
		Deadline:      fine.Deadline,
		Paid:          updatedFine.PaidAt.Valid,
		CreatedAt:     fine.CreatedAt,
	}, nil
}
