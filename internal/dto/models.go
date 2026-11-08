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
	Email     string `json:"email" validate:"required,email,max=255"`
	FirstName string `json:"first_name" validate:"required,max=255"`
	LastName  string `json:"last_name" validate:"max=255"`
}

type UpdateMemberProfileInput struct {
	Phone          string `json:"phone" validate:"required,min=7,max=16"`
	Address        string `json:"address" validate:"required,max=2000"`
	NextOfKinName  string `json:"next_of_kin_name" validate:"required,max=255"`
	NextOfKinPhone string `json:"next_of_kin_phone" validate:"required,min=7,max=16"`
}

type MemberInvitation struct {
	Email                 string     `json:"email"`
	Member                *Member    `json:"member"`
	RequiresPasswordSetup bool       `json:"requires_password_setup"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
	Delivery              string     `json:"delivery"`
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

type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "PENDING"
	PaymentStatusSuccess PaymentStatus = "SUCCESS"
	PaymentStatusFailed  PaymentStatus = "FAILED"
)

// CreatePaymentInput starts a Paystack checkout. Amount is required for
// SAVINGS, flexible SPECIAL_DEPOSIT and SHARES; the server sets it for
// REGISTRATION_FEE, fixed campaigns and FINES. CampaignID is required for
// SPECIAL_DEPOSIT; FineID is required for FINES.
type CreatePaymentInput struct {
	Purpose     LedgerType `json:"purpose" validate:"required,oneof=REGISTRATION_FEE SAVINGS SPECIAL_DEPOSIT SHARES FINES"`
	Amount      int64      `json:"amount" validate:"omitempty,gt=0"`
	FineID      *uuid.UUID `json:"fine_id,omitempty"`
	CampaignID  *uuid.UUID `json:"campaign_id,omitempty"`
	Description string     `json:"description" validate:"max=255"`
}

// PaymentQuote is the breakdown a member pays, in kobo.
type PaymentQuote struct {
	Purpose               LedgerType `json:"purpose"`
	CampaignID            *uuid.UUID `json:"campaign_id,omitempty"`
	Currency              string     `json:"currency"`
	Amount                int64      `json:"amount"`
	MaintenanceFee        int64      `json:"maintenance_fee"`
	MaintenanceFeePercent int32      `json:"maintenance_fee_percent"`
	ProcessingCharge      int64      `json:"processing_charge"`
	Total                 int64      `json:"total"`
}

type Payment struct {
	ID                    uuid.UUID     `json:"id"`
	Reference             string        `json:"reference"`
	Purpose               LedgerType    `json:"purpose"`
	Status                PaymentStatus `json:"status"`
	Currency              string        `json:"currency"`
	Amount                int64         `json:"amount"`
	MaintenanceFee        int64         `json:"maintenance_fee"`
	MaintenanceFeePercent int32         `json:"maintenance_fee_percent"`
	ProcessingCharge      int64         `json:"processing_charge"`
	Total                 int64         `json:"total"`
	Description           string        `json:"description"`
	FineID                *uuid.UUID    `json:"fine_id,omitempty"`
	CampaignID            *uuid.UUID    `json:"campaign_id,omitempty"`
	AuthorizationURL      *string       `json:"authorization_url,omitempty"`
	FailureReason         *string       `json:"failure_reason,omitempty"`
	PaidAt                *time.Time    `json:"paid_at,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
	// Member is set on admin views only.
	Member *Member `json:"member,omitempty"`
}

// PaymentFilters narrows the admin payment list.
type PaymentFilters struct {
	MemberID   *uuid.UUID
	CampaignID *uuid.UUID
	Status     *PaymentStatus
	Purpose    *LedgerType
}

// PaymentVerification is an admin re-check: the stored payment after
// verifying it, and the status Paystack reported (success, abandoned,
// failed, reversed, ...).
type PaymentVerification struct {
	Payment        *Payment `json:"payment"`
	PaystackStatus string   `json:"paystack_status"`
}

// MemberSummary is the admin view of a member, including the account email.
// EmailConfirmed is false until the invited person sets their password.
type MemberSummary struct {
	Member
	Email          string    `json:"email"`
	EmailConfirmed bool      `json:"email_confirmed"`
	CreatedAt      time.Time `json:"created_at"`
}

type MemberFilter struct {
	IsActive *bool   `json:"is_active,omitempty"`
	Search   *string `json:"search,omitempty"`
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
	RefreshToken string    `json:"refresh_token,omitempty"`
}

type AuthUser struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	Roles       []string   `json:"roles"`
	Member      *Member    `json:"member,omitempty"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

type GrantAdminInput struct {
	Email string `json:"email" validate:"required,email"`
}

type AdminAccount struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Roles []string  `json:"roles"`
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
	CreatedAt   time.Time         `json:"created_at"`
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
	// Member is the fined member.
	Member    *Member   `json:"member,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type TransactionFilters struct {
	MemberSlug *string `json:"member_slug,omitempty"`
	LedgerType *string `json:"ledger_type,omitempty"`
	Type       *string `json:"type,omitempty"`
	// Admin transaction list only.
	MemberID *uuid.UUID             `json:"member_id,omitempty"`
	Status   *TransactionStatusType `json:"status,omitempty"`
	Search   *string                `json:"search,omitempty"`
}

// LedgerTotals are confirmed deposits across all members, in kobo.
type LedgerTotals struct {
	MembershipFees        int64 `json:"membership_fees"`
	Savings               int64 `json:"savings"`
	CampaignContributions int64 `json:"campaign_contributions"`
	Shares                int64 `json:"shares"`
	Fines                 int64 `json:"fines"`
}

type FineFilter struct {
	MemberID *uuid.UUID `json:"member_id,omitempty"`
	Paid     *bool      `json:"paid,omitempty"`
}
