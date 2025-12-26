package transactions

import (
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/google/uuid"
)

const (
	DefaultSharesUnitPrice int64 = 50000
	DefaultRegistrationFee int64 = 100000

	MinSavingsDepositAmount int64 = 10000
	MinSpecialDepositAmount int64 = 50000

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
	unitsFloat float64
	remainder  int64
	unitPrice  int64
}
