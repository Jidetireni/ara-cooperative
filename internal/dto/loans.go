package dto

import (
	"time"

	"github.com/google/uuid"
)

type LoanInput struct {
	Amount         int64  `json:"amount" validate:"required,gt=0"`
	Purpose        string `json:"purpose" validate:"required,max=2000"`
	DurationMonths int32  `json:"duration_months" validate:"required,gt=0"`
}

type LoanApprovalInput struct {
	Amount              int64  `json:"amount" validate:"required,gt=0"`
	DurationMonths      int32  `json:"duration_months" validate:"required,gt=0"`
	InterestMethod      string `json:"interest_method" validate:"required,oneof=FLAT REDUCING_BALANCE"`
	InterestRatePercent string `json:"interest_rate_percent" validate:"required,max=20"`
	InterestPeriod      string `json:"interest_period" validate:"required,oneof=MONTHLY ANNUAL"`
	Reason              string `json:"reason,omitempty" validate:"max=2000"`
	OverrideReason      string `json:"override_reason,omitempty" validate:"max=2000"`
}

type LoanRejectionInput struct {
	Reason string `json:"reason" validate:"required,max=2000"`
}

type LoanDisbursementInput struct {
	Amount    int64     `json:"amount" validate:"required,gt=0"`
	PaidAt    time.Time `json:"paid_at" validate:"required"`
	Reference string    `json:"reference" validate:"required,max=100"`
}

type LoanSettingsInput struct {
	MinimumSavings    int64  `json:"minimum_savings" validate:"gte=0"`
	MinimumShareUnits string `json:"minimum_share_units" validate:"required,max=20"`
}

type LoanSettings struct {
	MinimumSavings    int64      `json:"minimum_savings"`
	MinimumShareUnits string     `json:"minimum_share_units"`
	UpdatedBy         *uuid.UUID `json:"updated_by,omitempty"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
}

type LoanAdvisoryFlag struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type LoanEligibility struct {
	MemberID                 uuid.UUID          `json:"member_id"`
	Savings                  int64              `json:"savings"`
	ShareUnits               string             `json:"share_units"`
	MinimumSavings           int64              `json:"minimum_savings"`
	MinimumShareUnits        string             `json:"minimum_share_units"`
	PastApplications         int64              `json:"past_applications"`
	OtherPendingApplications int64              `json:"other_pending_applications"`
	ApprovedUndisbursedLoans int64              `json:"approved_undisbursed_loans"`
	ActiveLoans              int64              `json:"active_loans"`
	Flags                    []LoanAdvisoryFlag `json:"flags"`
	CheckedAt                time.Time          `json:"checked_at"`
}

type Loan struct {
	ID                      uuid.UUID        `json:"id"`
	MemberID                uuid.UUID        `json:"member_id"`
	Member                  *Member          `json:"member,omitempty"`
	RequestedAmount         int64            `json:"requested_amount"`
	Purpose                 string           `json:"purpose"`
	RequestedDurationMonths int32            `json:"requested_duration_months"`
	Status                  string           `json:"status"`
	ApprovedAmount          *int64           `json:"approved_amount,omitempty"`
	ApprovedDurationMonths  *int32           `json:"approved_duration_months,omitempty"`
	InterestMethod          *string          `json:"interest_method,omitempty"`
	InterestRatePercent     *string          `json:"interest_rate_percent,omitempty"`
	InterestPeriod          *string          `json:"interest_period,omitempty"`
	ReviewReason            *string          `json:"review_reason,omitempty"`
	OverrideReason          *string          `json:"override_reason,omitempty"`
	EligibilitySnapshot     *LoanEligibility `json:"eligibility_snapshot,omitempty"`
	ReviewedBy              *uuid.UUID       `json:"reviewed_by,omitempty"`
	ReviewedAt              *time.Time       `json:"reviewed_at,omitempty"`
	DisbursedAmount         *int64           `json:"disbursed_amount,omitempty"`
	DisbursedAt             *time.Time       `json:"disbursed_at,omitempty"`
	DisbursementReference   *string          `json:"disbursement_reference,omitempty"`
	DisbursedBy             *uuid.UUID       `json:"disbursed_by,omitempty"`
	TransactionID           *uuid.UUID       `json:"transaction_id,omitempty"`
	CreatedAt               time.Time        `json:"created_at"`
	UpdatedAt               *time.Time       `json:"updated_at,omitempty"`
}

type LoanFilters struct {
	MemberID *uuid.UUID
	Status   *string
	Search   *string
}
