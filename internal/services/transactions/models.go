package transactions

import (
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/google/uuid"
)

const (
	DefaultSharesUnitPrice       = 50_000
	DefaultRegistrationFee int64 = 10_0000

	MinSavingsDepositAmount int64 = 10_000
	MinSpecialDepositAmount int64 = 50_000

	SharesUnitPriceRedisKey = "shares_unit_price"
	SharesUnitPriceCacheTTL = time.Hour * 24 * 7
	SharePrecisionScale     = 1e4
)

// TransactionParams contains parameters for creating transactions
type TransactionParams struct {
	Input      dto.TransactionsInput
	Type       repository.TransactionType
	LedgerType repository.LedgerType
}

// BalanceFilter contains parameters for balance calculations
type BalanceFilter struct {
	MemberID   uuid.UUID
	LedgerType repository.LedgerType
	Type       *repository.TransactionType
}

type calculateShareQuoteResult struct {
	scaledUnits int64
	unitsFloat  float64
	remainder   int64
	unitPrice   int64
}
