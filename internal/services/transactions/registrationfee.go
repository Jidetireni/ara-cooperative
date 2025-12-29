package transactions

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
)

func (t *Transaction) ChargeRegistrationFee(ctx context.Context, input *dto.TransactionsInput) (*dto.Transactions, error) {
	actor, ok := users.FromContext(ctx)
	if !ok {
		return nil, svc.UnauthenticatedError()
	}

	if input.Amount != DefaultRegistrationFee {
		return nil, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("registration fee must be %d", DefaultRegistrationFee),
		}
	}

	member, err := t.getMemberByUserID(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	if member.ActivatedAt.Valid {
		return nil, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "member already activated",
		}
	}

	tx, err := t.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

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

	return t.TransactionRepo.MapRepositoryToDTO(transaction, status), nil
}
