package transactions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/samber/lo"
)

func (t *Transaction) SetSharesUnitPrice(ctx context.Context, input dto.SetShareUnitPriceInput) error {
	if input.UnitPrice <= 0 {
		return &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "unit price must be a positive integer",
		}
	}

	tx, err := t.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = t.ShareRepo.CreateUnitPrice(ctx, input.UnitPrice, tx)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	err = t.RedisPkg.SetPrimitive(ctx, SharesUnitPriceRedisKey, strconv.FormatInt(input.UnitPrice, 10), SharesUnitPriceCacheTTL)
	if err != nil {
		t.Logger.Error().Err(err).Msg("failed to update shares unit price cache")
		_ = t.RedisPkg.Delete(ctx, SharesUnitPriceRedisKey)
	}

	return nil
}

func (t *Transaction) GetSharesUnitPrice(ctx context.Context) (int64, error) {
	priceStr, err := t.RedisPkg.GetPrimitive(ctx, SharesUnitPriceRedisKey)
	if err == nil {
		price, err := strconv.ParseInt(priceStr, 10, 64)
		if err == nil {
			return price, nil
		}
	}

	price, err := t.ShareRepo.GetUnitPrice(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultSharesUnitPrice, nil
		}
		return 0, err
	}

	go func() {
		err = t.RedisPkg.SetPrimitive(context.Background(), SharesUnitPriceRedisKey, strconv.FormatInt(price, 10), SharesUnitPriceCacheTTL)
		if err != nil {
			t.Logger.Error().Err(err).Msg("failed to set shares unit price cache")
		}
	}()

	return price, nil
}

func (t *Transaction) calculateShareQuote(ctx context.Context, amount int64) (*calculateShareQuoteResult, error) {
	if amount <= 0 {
		return nil, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "amount must be positive",
		}
	}

	unitPrice, err := t.GetSharesUnitPrice(ctx)
	if err != nil {
		return nil, err
	}

	if unitPrice <= 0 {
		return nil, &svc.APIError{
			Status:  http.StatusServiceUnavailable,
			Message: "unit price is not currently set",
		}
	}

	scaledUnit := (amount * SharePrecisionScale) / unitPrice
	spent := (scaledUnit * unitPrice) / SharePrecisionScale
	remainder := amount - spent
	unitsFloat := float64(scaledUnit) / SharePrecisionScale

	return &calculateShareQuoteResult{
		unitsFloat: unitsFloat,
		remainder:  remainder,
		unitPrice:  unitPrice,
	}, nil
}

func (t *Transaction) GetShareQuote(ctx context.Context, amount int64) (*dto.GetUnitsQuote, error) {
	result, err := t.calculateShareQuote(ctx, amount)
	if err != nil {
		return nil, err
	}

	return &dto.GetUnitsQuote{
		Units:     result.unitsFloat,
		Remainder: result.remainder,
		UnitPrice: result.unitPrice,
	}, nil
}

func (t *Transaction) BuyShares(ctx context.Context, input dto.BuySharesInput) (*dto.Shares, error) {
	actor, ok := users.FromContext(ctx)
	if !ok {
		return nil, svc.UnauthenticatedError()
	}

	unitPrice, err := t.GetSharesUnitPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get shares unit price: %w", err)
	}
	if unitPrice <= 0 {
		return nil, &svc.APIError{
			Status:  http.StatusServiceUnavailable,
			Message: "shares unit price is not set",
		}
	}

	result, err := t.calculateShareQuote(ctx, input.Amount)
	if err != nil {
		return nil, err
	}
	if result.unitsFloat <= 0 {
		return nil, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "amount is too small to purchase any shares",
		}
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
		Input: dto.TransactionsInput{
			Amount:      input.Amount,
			Description: fmt.Sprintf("Purchase of %f shares", result.unitsFloat),
		},
		Type:       repository.TransactionTypeDEPOSIT,
		LedgerType: repository.LedgerTypeSHARES,
	}, tx)
	if err != nil {
		return nil, err
	}

	shares, err := t.ShareRepo.Create(ctx, repository.Share{
		TransactionID: transaction.ID,
		Units:         fmt.Sprintf("%.4f", result.unitsFloat),
		UnitPrice:     unitPrice,
	}, tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return t.MapPopShareToDTO(&repository.PopShare{
		ID:            shares.ID,
		TransactionID: shares.TransactionID,
		Units:         shares.Units,
		UnitPrice:     shares.UnitPrice,
	}, transaction, status)
}

func (t *Transaction) GetTotalShares(ctx context.Context) (*dto.SharesTotal, error) {
	filters := repository.ShareRepositoryFilter{
		Confirmed:  lo.ToPtr(true),
		Rejected:   lo.ToPtr(false),
		Type:       lo.ToPtr(repository.TransactionTypeDEPOSIT),
		LedgerType: repository.LedgerTypeSHARES,
	}

	return t.calculateShareTotals(ctx, filters)
}

func (t *Transaction) GetMemberTotalShares(ctx context.Context) (*dto.SharesTotal, error) {
	actor, ok := users.FromContext(ctx)
	if !ok {
		return nil, svc.UnauthenticatedError()
	}

	member, err := t.getMemberByUserID(ctx, actor.ID)
	if err != nil {
		return nil, err
	}

	filters := repository.ShareRepositoryFilter{
		Confirmed:  lo.ToPtr(true),
		Rejected:   lo.ToPtr(false),
		Type:       lo.ToPtr(repository.TransactionTypeDEPOSIT),
		MemberID:   &member.ID,
		LedgerType: repository.LedgerTypeSHARES,
	}

	return t.calculateShareTotals(ctx, filters)
}

func (t *Transaction) calculateShareTotals(ctx context.Context, filters repository.ShareRepositoryFilter) (*dto.SharesTotal, error) {
	total, err := t.ShareRepo.CountTotalSharesPurchased(ctx, filters)
	if err != nil {
		return nil, err
	}

	var units float64
	if total.Units != "" {
		units, err = strconv.ParseFloat(total.Units, 64)
		if err != nil {
			return nil, fmt.Errorf("data corruption: invalid unit count in db: %w", err)
		}
	}

	return &dto.SharesTotal{
		Units:  units,
		Amount: total.Amount,
	}, nil
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
