package email

type EmailTemplateType string

const (
	// EmailTemplateTypeMemberInvitation invites a member to set a password or sign in.
	EmailTemplateTypeMemberInvitation EmailTemplateType = "member_invitation"
	// EmailTemplateTypeLoanGuarantorRequest asks a member to accept or decline guaranteeing a loan.
	EmailTemplateTypeLoanGuarantorRequest EmailTemplateType = "loan_guarantor_request"
)

type SendEmailInput struct {
	To      string
	Subject string
	// Body is the HTML part.
	Body string
	// Text is the optional plain-text part, shown by clients that don't render HTML.
	Text string
}

// Rendered is one template in both formats.
type Rendered struct {
	HTML string
	Text string
}

// MemberInvitationData fills the member_invitation template.
type MemberInvitationData struct {
	FirstName string
	// ActionURL is the set-password link (with its one-time token) or the login page.
	ActionURL string
	// NeedsPasswordSetup selects the set-password wording over the sign-in wording.
	NeedsPasswordSetup bool
	// ExpiresInMinutes is shown for set-password links.
	ExpiresInMinutes int
}

// LoanGuarantorRequestData fills the loan_guarantor_request template.
type LoanGuarantorRequestData struct {
	FirstName       string
	ApplicantName   string
	ApplicantNumber string
	// Amount is the requested amount, formatted for reading (for example ₦400,000.00).
	Amount         string
	Purpose        string
	DurationMonths int32
	// ActionURL opens the page where the guarantor accepts or declines.
	ActionURL string
}
