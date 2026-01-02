package dto

import (
	"time"

	"github.com/google/uuid"
)

type TransactionStatusType string
type TransactionType string
type LedgerType string

const (
	TransactionStatusTypePending   TransactionStatusType = "PENDING"
	TransactionStatusTypeConfirmed TransactionStatusType = "CONFIRMED"
	TransactionStatusTypeRejected  TransactionStatusType = "REJECTED"

	TransactionTypeDeposit    TransactionType = "DEPOSIT"
	TransactionTypeWithdrawal TransactionType = "WITHDRAWAL"

	TransactionTypeLoanDisbursement TransactionType = "LOAN_DISBURSEMENT"
	TransactionTypeLoanRepayment    TransactionType = "LOAN_REPAYMENT"

	LedgerTypeSAVINGS         LedgerType = "SAVINGS"
	LedgerTypeSPECIALDEPOSIT  LedgerType = "SPECIAL_DEPOSIT"
	LedgerTypeLOAN            LedgerType = "LOAN"
	LedgerTypeSHARES          LedgerType = "SHARES"
	LedgerTypeFINES           LedgerType = "FINES"
	LedgerTypeREGISTRATIONFEE LedgerType = "REGISTRATION_FEE"
)

type CreateMemberInput struct {
	Email          string `json:"email" validate:"required,email"`
	FirstName      string `json:"first_name" validate:"required"`
	LastName       string `json:"last_name" validate:"required"`
	Phone          string `json:"phone" validate:"required"`
	Address        string `json:"address" validate:"required"`
	NextOfKinName  string `json:"next_of_kin_name" validate:"required"`
	NextOfKinPhone string `json:"next_of_kin_phone" validate:"required"`
}

type Member struct {
	ID             uuid.UUID `json:"id"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	Slug           string    `json:"slug"`
	Phone          string    `json:"phone"`
	Address        string    `json:"address"`
	NextOfKinName  string    `json:"next_of_kin_name"`
	NextOfKinPhone string    `json:"next_of_kin_phone"`
	IsActive       bool      `json:"is_active"`
}

type User struct {
	ID               uuid.UUID  `json:"id"`
	Email            string     `json:"email"`
	PasswordHash     string     `json:"password_hash,omitempty"`
	EmailConfirmedAt *time.Time `json:"email_confirmed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type SetPasswordInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Token    string `json:"token" validate:"required"`
}

type LoginInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type AuthResponse struct {
	User         *AuthUser `json:"user"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
}

type AuthUser struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	Roles       []string   `json:"roles"`
	Member      *Member    `json:"member,omitempty"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

type JWTClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Roles  []string  `json:"roles"`
	Type   string    `json:"type"` // "access" or "refresh"
}

type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword"`
}

type QueryOptions struct {
	Limit  uint32  `json:"limit"`
	Cursor *string `json:"cursor,omitempty"`
	Sort   *string `json:"sort,omitempty"`
}

type ListResponse[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type UpdateTransactionStatusInput struct {
	Confirmed  *bool   `json:"confirmed"`
	Reason     *string `json:"reason,omitempty"`
	LedgerType string  `json:"ledger_type"`
}

type TransactionStatusResult struct {
	Confirmed *bool  `json:"confirmed"`
	Message   string `json:"message,omitempty"`
}

type SetShareUnitPriceInput struct {
	UnitPrice int64 `json:"unit_price" validate:"required,gt=0"`
}

type GetUnitsQuote struct {
	Units     float64 `json:"units" validate:"required,gt=0"`
	Remainder int64   `json:"remainder"`
	UnitPrice int64   `json:"unit_price"`
}

type BuySharesInput struct {
	Amount int64   `json:"amount" validate:"required,gt=0"`
	Units  float64 `json:"units,omitempty" validate:"gte=0"`
}

type Shares struct {
	ID          uuid.UUID    `json:"id"`
	Transaction Transactions `json:"transaction"`
	Units       float64      `json:"units"`
	UnitPrice   int64        `json:"unit_price"`
	CreatedAt   time.Time    `json:"created_at"`
}

type SharesTotal struct {
	Units  float64 `json:"units"`
	Amount int64   `json:"amount"`
}

type TransactionsInput struct {
	Amount      int64  `json:"amount" validate:"required,gt=0"`
	Description string `json:"description" validate:"required"`
}

type Transactions struct {
	ID          uuid.UUID         `json:"id"`
	Amount      int64             `json:"amount"`
	Description string            `json:"description"`
	Type        TransactionType   `json:"type"`
	LedgerType  LedgerType        `json:"ledger_type"`
	Reference   string            `json:"reference"`
	Status      TransactionStatus `json:"status"`
	Member      Member            `json:"member"`
}
type TransactionStatus struct {
	ID          uuid.UUID             `json:"id"`
	Status      TransactionStatusType `json:"status"`
	ConfirmedAt *time.Time            `json:"confirmed_at,omitempty"`
	RejectedAt  *time.Time            `json:"rejected_at,omitempty"`
	Reason      *string               `json:"reason,omitempty"`
}

type FineInput struct {
	Amount   int64     `json:"amount" validate:"required,gt=0"`
	MemberID uuid.UUID `json:"member_id" validate:"required"`
	Reason   string    `json:"reason" validate:"required"`
	Deadline time.Time `json:"deadline" validate:"required"`
}

type Fine struct {
	ID          uuid.UUID     `json:"id"`
	Amount      int64         `json:"amount"`
	Transaction *Transactions `json:"transactions,omitempty"`
	Reason      string        `json:"reason"`
	Deadline    time.Time     `json:"deadline"`
	Paid        bool          `json:"paid"`
}

type TransactionFilters struct {
	MemberSlug *string `json:"member_slug,omitempty"`
	LedgerType *string `json:"ledger_type,omitempty"`
	Type       *string `json:"type,omitempty"`
}

type FineFilter struct {
	MemberID *uuid.UUID `json:"member_id,omitempty"`
	Paid     *bool      `json:"paid,omitempty"`
}
