package dto

import (
	"time"

	"github.com/google/uuid"
)

// LoanInput mirrors the paper loan application form. Personal and
// membership details come from the member's profile and records.
type LoanInput struct {
	Amount         int64  `json:"amount" validate:"required,gt=0"`
	Purpose        string `json:"purpose" validate:"required,max=2000"`
	DurationMonths int32  `json:"duration_months" validate:"required,gt=0"`

	// The applicant's proposed repayment plan. Information only: repayment
	// schedules and collection stay deferred.
	RepaymentStartDate      string `json:"repayment_start_date" validate:"required,datetime=2006-01-02"`
	RepaymentDurationMonths int32  `json:"repayment_duration_months" validate:"required,gt=0"`
	MonthlyRepaymentAmount  int64  `json:"monthly_repayment_amount" validate:"required,gt=0"`

	// DisbursementBankCode comes from GET /banks. The API resolves the account
	// with Paystack and records the bank's name for the account holder.
	DisbursementBankCode      string `json:"disbursement_bank_code" validate:"required,max=20"`
	DisbursementAccountNumber string `json:"disbursement_account_number" validate:"required,len=10,numeric"`

	// GuarantorMemberNumbers are the two guarantors' membership numbers (slugs).
	GuarantorMemberNumbers []string `json:"guarantor_member_numbers" validate:"required,len=2,dive,required,max=50"`

	// DeclarationAccepted confirms the form's authorization and understanding.
	DeclarationAccepted bool `json:"declaration_accepted"`
}

type LoanGuaranteeInput struct {
	Decision string `json:"decision" validate:"required,oneof=ACCEPT DECLINE"`
}

// LoanGuarantor is a guarantor named on a loan application.
type LoanGuarantor struct {
	Position    int16      `json:"position"`
	Member      MemberCard `json:"member"`
	Address     string     `json:"address,omitempty"`
	Status      string     `json:"status"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
}

// LoanGuarantorCheck is a guarantor's standing at review time.
type LoanGuarantorCheck struct {
	MemberID    uuid.UUID `json:"member_id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	IsActive    bool      `json:"is_active"`
	Savings     int64     `json:"savings"`
	ShareUnits  string    `json:"share_units"`
	OpenLoans   int64     `json:"open_loans"`
	UnpaidFines int64     `json:"unpaid_fines"`
}

// GuarantorRequest is a loan application on which the current member is named
// as a guarantor.
type GuarantorRequest struct {
	LoanID                  uuid.UUID  `json:"loan_id"`
	Applicant               MemberCard `json:"applicant"`
	RequestedAmount         int64      `json:"requested_amount"`
	Purpose                 string     `json:"purpose"`
	RequestedDurationMonths int32      `json:"requested_duration_months"`
	LoanStatus              string     `json:"loan_status"`
	Position                int16      `json:"position"`
	Status                  string     `json:"status"`
	RespondedAt             *time.Time `json:"responded_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
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
	MemberID                 uuid.UUID            `json:"member_id"`
	Savings                  int64                `json:"savings"`
	ShareUnits               string               `json:"share_units"`
	MinimumSavings           int64                `json:"minimum_savings"`
	MinimumShareUnits        string               `json:"minimum_share_units"`
	PastApplications         int64                `json:"past_applications"`
	OtherPendingApplications int64                `json:"other_pending_applications"`
	ApprovedUndisbursedLoans int64                `json:"approved_undisbursed_loans"`
	ActiveLoans              int64                `json:"active_loans"`
	MembershipActivatedAt    *time.Time           `json:"membership_activated_at,omitempty"`
	Guarantors               []LoanGuarantorCheck `json:"guarantors"`
	Flags                    []LoanAdvisoryFlag   `json:"flags"`
	CheckedAt                time.Time            `json:"checked_at"`
}

type Loan struct {
	ID                      uuid.UUID `json:"id"`
	MemberID                uuid.UUID `json:"member_id"`
	Member                  *Member   `json:"member,omitempty"`
	RequestedAmount         int64     `json:"requested_amount"`
	Purpose                 string    `json:"purpose"`
	RequestedDurationMonths int32     `json:"requested_duration_months"`

	RepaymentStartDate        *string         `json:"repayment_start_date,omitempty"`
	RepaymentDurationMonths   *int32          `json:"repayment_duration_months,omitempty"`
	MonthlyRepaymentAmount    *int64          `json:"monthly_repayment_amount,omitempty"`
	DisbursementBankCode      *string         `json:"disbursement_bank_code,omitempty"`
	DisbursementBankName      *string         `json:"disbursement_bank_name,omitempty"`
	DisbursementAccountName   *string         `json:"disbursement_account_name,omitempty"`
	DisbursementAccountNumber *string         `json:"disbursement_account_number,omitempty"`
	DeclarationAcceptedAt     *time.Time      `json:"declaration_accepted_at,omitempty"`
	Guarantors                []LoanGuarantor `json:"guarantors"`

	Status                 string           `json:"status"`
	ApprovedAmount         *int64           `json:"approved_amount,omitempty"`
	ApprovedDurationMonths *int32           `json:"approved_duration_months,omitempty"`
	InterestMethod         *string          `json:"interest_method,omitempty"`
	InterestRatePercent    *string          `json:"interest_rate_percent,omitempty"`
	InterestPeriod         *string          `json:"interest_period,omitempty"`
	ReviewReason           *string          `json:"review_reason,omitempty"`
	OverrideReason         *string          `json:"override_reason,omitempty"`
	EligibilitySnapshot    *LoanEligibility `json:"eligibility_snapshot,omitempty"`
	ReviewedBy             *uuid.UUID       `json:"reviewed_by,omitempty"`
	ReviewedAt             *time.Time       `json:"reviewed_at,omitempty"`
	DisbursedAmount        *int64           `json:"disbursed_amount,omitempty"`
	DisbursedAt            *time.Time       `json:"disbursed_at,omitempty"`
	DisbursementReference  *string          `json:"disbursement_reference,omitempty"`
	DisbursedBy            *uuid.UUID       `json:"disbursed_by,omitempty"`
	TransactionID          *uuid.UUID       `json:"transaction_id,omitempty"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              *time.Time       `json:"updated_at,omitempty"`
}

type LoanFilters struct {
	MemberID *uuid.UUID
	Status   *string
	Search   *string
}

// Bank is a Nigerian bank from Paystack's list.
type Bank struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

// BankAccount is an account Paystack resolved at a bank.
type BankAccount struct {
	BankCode      string `json:"bank_code"`
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
}
