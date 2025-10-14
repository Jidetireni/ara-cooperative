package transactions

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
)

func (t *Transaction) SetSharesUnitPrice(ctx context.Context, input dto.SetShareUnitPriceInput) (string, error) {
	if input.UnitPrice <= 0 {
		return "", &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "unit price must be a positive integer",
		}
	}

	t.mu.Lock()
	t.unitPrice = input.UnitPrice
	t.mu.Unlock()
	return "Share unit price updated successfully", nil
}

func (t *Transaction) GetSharesUnitPrice(ctx context.Context) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	unitPrice := t.unitPrice
	if unitPrice == 0 {
		unitPrice = DefaultSharesUnitPrice
	}
	return unitPrice
}

func (t *Transaction) BuyShares(ctx context.Context, input dto.BuySharesInput) (*dto.Shares, error) {
	user := users.FromContext(ctx)
	if user.ID == uuid.Nil {
		return nil, &svc.ApiError{
			Status:  http.StatusUnauthorized,
			Message: "unauthenticated",
		}
	}

	unitPrice := t.GetSharesUnitPrice(ctx)
	if unitPrice <= 0 {
		return nil, &svc.ApiError{
			Status:  http.StatusServiceUnavailable,
			Message: "unit price is not set",
		}
	}

	// Compute fractional units with fixed precision
	const precision = 4
	scaled := math.Floor(float64(input.Amount) * math.Pow10(precision) / float64(unitPrice))
	computedUnits := scaled / math.Pow10(precision)

	if input.Units > 0 {
		const eps = 1e-4
		if math.Abs(computedUnits-input.Units) > eps {
			return nil, &svc.ApiError{
				Status:  http.StatusBadRequest,
				Message: fmt.Sprintf("amount %d does not correspond to units %.4f at unit price %d", input.Amount, input.Units, unitPrice),
			}
		}
	} else {
		// Populate units server-side if client didn't send it
		input.Units = computedUnits
	}

	member, err := t.getMemberByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	tx, err := t.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Create transaction
	transaction, status, err := t.createTransactionWithStatus(ctx, member.ID, TransactionParams{
		Input: dto.TransactionsInput{
			Amount:      input.Amount,
			Description: fmt.Sprintf("Purchase of %.4f shares", input.Units),
		},
		Type:       repository.TransactionTypeDEPOSIT,
		LedgerType: repository.LedgerTypeSHARES,
	}, tx)
	if err != nil {
		return nil, err
	}

	// Create shares record
	shares, err := t.ShareRepo.Create(ctx, repository.Share{
		TransactionID: transaction.ID,
		Units:         fmt.Sprintf("%.4f", input.Units),
		UnitPrice:     unitPrice,
	}, tx)

	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	share, err := t.MapPopShareToDTO(&repository.PopShare{
		ID:            shares.ID,
		TransactionID: shares.TransactionID,
		MemberID:      transaction.MemberID,
		Description:   transaction.Description,
		Reference:     transaction.Reference,
		Amount:        transaction.Amount,
		Type:          repository.TransactionTypeDEPOSIT,
		Units:         shares.Units,
		UnitPrice:     shares.UnitPrice,
		CreatedAt:     shares.CreatedAt,
		ConfirmedAt:   status.ConfirmedAt,
		RejectedAt:    status.RejectedAt,
	}, transaction, status)
	if err != nil {
		return nil, err
	}

	return share, nil
}

func (t *Transaction) MapPopShareToDTO(share *repository.PopShare, txn *repository.Transaction, status *repository.TransactionStatus) (*dto.Shares, error) {
	units, err := strconv.ParseFloat(share.Units, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid share units: %w", err)
	}

	return &dto.Shares{
		ID:          share.ID,
		Transaction: *t.MapRepositoryToDTO(txn, status),
		Units:       units,
		UnitPrice:   share.UnitPrice,
		CreatedAt:   share.CreatedAt,
	}, nil
}
