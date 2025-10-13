package transactions

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
)

func (t *Transaction) ChargeRegistrationFee(ctx context.Context, input *dto.TransactionsInput) (*dto.Transactions, error) {
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

	if member.ActivatedAt.Valid {
		return nil, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "member already activated",
		}
	}

	tx, err := t.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	transaction, status, err := t.createTransactionWithStatus(
		ctx,
		member.ID,
		TransactionParams{
			Input:      *input,
			Type:       repository.TransactionTypeDEPOSIT,
			LedgerType: repository.LedgerTypeREGISTRATIONFEE,
		},
		tx,
	)
	if err != nil {
		return nil, err
	}

	if !transaction.CreatedAt.Valid {
		return nil, fmt.Errorf("transaction failed")
	}

	if err = tx.Commit(); err != nil {
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
