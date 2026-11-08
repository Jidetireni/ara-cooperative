package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const (
	resendRequestTimeout = 15 * time.Second
	resendMaxErrorBody   = 4 << 10
)

// resendSender sends through Resend's HTTPS API (port 443). Hosts such as
// Railway's Free, Trial and Hobby plans block outbound SMTP, so this is the
// sender to use there. https://resend.com/docs/api-reference/emails/send-email
type resendSender struct {
	apiKey     string
	baseURL    string
	from       string
	httpClient *http.Client
}

func newResendSender(apiKey, baseURL, fromAddress, fromName string) *resendSender {
	from := (&mail.Address{Name: fromName, Address: fromAddress}).String()
	return &resendSender{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		from:       from,
		httpClient: &http.Client{Timeout: resendRequestTimeout},
	}
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	Text    string   `json:"text,omitempty"`
}

type resendErrorBody struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func (s *resendSender) send(ctx context.Context, input *SendEmailInput) error {
	payload, err := json.Marshal(resendRequest{
		From: s.from, To: []string{input.To}, Subject: input.Subject,
		HTML: input.Body, Text: input.Text,
	})
	if err != nil {
		return fmt.Errorf("encode resend email: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/emails", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ara-cooperative-api")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send resend email: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, resendMaxErrorBody))
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, resendMaxErrorBody))
	var apiErr resendErrorBody
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
		return fmt.Errorf("resend: %s: %s (HTTP %d)", apiErr.Name, apiErr.Message, resp.StatusCode)
	}
	return fmt.Errorf("resend: HTTP %d", resp.StatusCode)
}
