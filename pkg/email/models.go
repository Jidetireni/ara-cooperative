package email

type EmailTemplateType string

const (
	SMTPHost       = "smtp.gmail.com"
	SMTPPort       = 587
	ARAFromEmail   = "jtirenipraise@gmail.com"
	EmailDirectory = "./email/templates"

	EmailTemplateTypeWelcome       EmailTemplateType = "welcome"
	EmailTemplateTypePasswordReset EmailTemplateType = "password_reset"
)

type SendEmailInput struct {
	To      string
	Subject string
	Body    string
}
