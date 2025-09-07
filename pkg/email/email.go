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
	cache  *EmailTemplateCache
}

func New(cfg *config.Config) (*Email, error) {
	cache, err := NewEmailTemplateCache(EmailDirectory, 10)
	if err != nil {
		return nil, err
	}

	return &Email{
		config: cfg,
		cache:  cache,
	}, nil
}

func (e *Email) Send(ctx context.Context, input *SendEmailInput) error {
	// In dev mode
	if e.config.IsDev {
		fmt.Printf("--- Email to be sent to %s ---\n", input.To)
		fmt.Printf("Subject: %s\n", input.Subject)
		fmt.Println("Body:")
		fmt.Println(input.Body)
		fmt.Println("---------------------------------")
		return nil
	}

	m := gomail.NewMessage()
	m.SetHeader("From", ARAFromEmail)
	m.SetHeader("To", input.To)
	m.SetHeader("Subject", input.Subject)
	m.SetBody("text/html", input.Body)

	d := gomail.NewDialer(SMTPHost, SMTPPort, ARAFromEmail, e.config.Email.Password)
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
