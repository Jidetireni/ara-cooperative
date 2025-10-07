package transactions

import (
	"context"
	"database/sql"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
)

func (t *Transaction) ChargeRegistrationFee(ctx context.Context, input *dto.TransactionsInput) (*dto.Transactions, error) {
	transaction, err := t.CreateTransaction(ctx, TransactionParams{
		Input:      *input,
		Type:       repository.TransactionTypeDEPOSIT,
		LedgerType: repository.LedgerTypeREGISTRATIONFEE,
	})
	if err != nil {
		return nil, err
	}

	user := users.FromContext(ctx)
	member, err := t.getMemberByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	member.ActivatedAt = sql.NullTime{Time: transaction.CreatedAt, Valid: true}
	_, err = t.MemberRepo.Update(ctx, member, nil)
	if err != nil {
		return nil, err
	}

	return transaction, nil
}
