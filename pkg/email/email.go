package email

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"gopkg.in/gomail.v2"
)

type Email struct {
	config *config.Config
	// resend is set when RESEND_API_KEY is configured; SMTP is used otherwise.
	resend *resendSender
}

func New(cfg *config.Config) (*Email, error) {
	e := &Email{config: cfg}
	if key := cfg.Email.ResendAPIKey; key != "" {
		e.resend = newResendSender(key, cfg.Email.ResendBaseURL, cfg.Email.FromAddress, cfg.Email.FromName)
	}
	return e, nil
}

func (e *Email) Send(ctx context.Context, input *SendEmailInput) error {
	// In dev mode
	if e.config.IsDev {
		fmt.Printf("--- Email to be sent to %s ---\n", input.To)
		fmt.Printf("Subject: %s\n", input.Subject)
		fmt.Println("Body:")
		if input.Text != "" {
			fmt.Println(input.Text)
		} else {
			fmt.Println(input.Body)
		}
		fmt.Println("---------------------------------")
		return nil
	}

	if e.resend != nil {
		return e.resend.send(ctx, input)
	}

	smtp := e.config.Email
	m := gomail.NewMessage()
	m.SetAddressHeader("From", smtp.FromAddress, smtp.FromName)
	m.SetHeader("To", input.To)
	m.SetHeader("Subject", input.Subject)
	if input.Text != "" {
		m.SetBody("text/plain", input.Text)
		m.AddAlternative("text/html", input.Body)
	} else {
		m.SetBody("text/html", input.Body)
	}

	// Port 465 uses implicit TLS; other ports upgrade with STARTTLS.
	d := gomail.NewDialer(smtp.Host, smtp.Port, smtp.Username, smtp.Password)
	d.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: smtp.Host}

	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
