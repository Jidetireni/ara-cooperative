// Package paystack is a small client for the Paystack endpoints Ara uses:
// initialize and verify a transaction, and check webhook signatures.
package paystack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrNotConfigured is returned when no secret key is set.
var ErrNotConfigured = errors.New("paystack is not configured")

const (
	EventChargeSuccess = "charge.success"

	StatusSuccess   = "success"
	StatusFailed    = "failed"
	StatusAbandoned = "abandoned"
	StatusReversed  = "reversed"

	// BearerSubaccount makes the transaction's subaccount pay Paystack's fee
	// from its share.
	BearerSubaccount = "subaccount"

	requestTimeout = 15 * time.Second
	maxBodySize    = 1 << 20
)

type Client struct {
	secretKey  string
	baseURL    string
	httpClient *http.Client
}

func New(secretKey, baseURL string) *Client {
	return &Client{
		secretKey:  secretKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// Enabled reports whether a secret key is configured.
func (c *Client) Enabled() bool {
	return c != nil && c.secretKey != ""
}

// APIError is a non-success answer from Paystack.
type APIError struct {
	HTTPStatus int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("paystack: %s (HTTP %d)", e.Message, e.HTTPStatus)
}

// Metadata travels with the transaction and comes back in webhooks and
// verification. Ara trusts its own database, not these values.
type Metadata struct {
	PaymentID        string `json:"payment_id"`
	MemberID         string `json:"member_id"`
	MemberNumber     string `json:"member_number"`
	Purpose          string `json:"purpose"`
	Amount           int64  `json:"amount"`
	MaintenanceFee   int64  `json:"maintenance_fee"`
	ProcessingCharge int64  `json:"processing_charge"`
	FineID           string `json:"fine_id,omitempty"`
	CampaignID       string `json:"campaign_id,omitempty"`
	CancelAction     string `json:"cancel_action,omitempty"`
}

type InitializeInput struct {
	Email       string   `json:"email"`
	Amount      int64    `json:"amount"` // kobo
	Currency    string   `json:"currency"`
	Reference   string   `json:"reference"`
	CallbackURL string   `json:"callback_url,omitempty"`
	Metadata    Metadata `json:"metadata"`
	// Subaccount receives Amount minus TransactionCharge. TransactionCharge
	// (kobo) goes to the main account and overrides the subaccount's default
	// percentage. Unlike a dynamic flat split, which needs at least NGN 100
	// per subaccount, this accepts small subaccount shares.
	Subaccount        string `json:"subaccount,omitempty"`
	TransactionCharge int64  `json:"transaction_charge,omitempty"`
	Bearer            string `json:"bearer,omitempty"`
}

type InitializeResult struct {
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Reference        string `json:"reference"`
}

// Transaction is the verified state of a payment.
type Transaction struct {
	ID              int64           `json:"id"`
	Status          string          `json:"status"`
	Reference       string          `json:"reference"`
	Amount          int64           `json:"amount"`
	Currency        string          `json:"currency"`
	Fees            int64           `json:"fees"`
	Channel         string          `json:"channel"`
	GatewayResponse string          `json:"gateway_response"`
	PaidAt          *time.Time      `json:"paid_at"`
	Metadata        json.RawMessage `json:"metadata"`
}

// WebhookEvent is the outer webhook envelope; Data depends on Event.
type WebhookEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// ChargeData is the part of a charge event Ara needs to find its payment.
type ChargeData struct {
	Reference string `json:"reference"`
}

type envelope[T any] struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    T               `json:"data"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

func (c *Client) InitializeTransaction(ctx context.Context, input InitializeInput) (*InitializeResult, error) {
	var result InitializeResult
	if err := c.call(ctx, http.MethodPost, "/transaction/initialize", input, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) VerifyTransaction(ctx context.Context, reference string) (*Transaction, error) {
	var result Transaction
	if err := c.call(ctx, http.MethodGet, "/transaction/verify/"+url.PathEscape(reference), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ValidSignature checks X-Paystack-Signature: hex HMAC-SHA512 of the raw body
// keyed with the secret key.
func (c *Client) ValidSignature(body []byte, signature string) bool {
	if !c.Enabled() || signature == "" {
		return false
	}
	mac := hmac.New(sha512.New, []byte(c.secretKey))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (c *Client) call(ctx context.Context, method, path string, body any, out any) error {
	_, err := c.callWithMeta(ctx, method, path, body, out)
	return err
}

// callWithMeta also returns Paystack's meta object (pagination cursors).
func (c *Client) callWithMeta(ctx context.Context, method, path string, body any, out any) (json.RawMessage, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode paystack request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paystack request %s: %w", path, err)
	}
	defer resp.Body.Close()

	var result envelope[json.RawMessage]
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(&result); err != nil {
		return nil, &APIError{HTTPStatus: resp.StatusCode, Message: "unreadable response"}
	}
	if resp.StatusCode >= http.StatusBadRequest || !result.Status {
		message := result.Message
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, &APIError{HTTPStatus: resp.StatusCode, Message: message}
	}
	if err := json.Unmarshal(result.Data, out); err != nil {
		return nil, fmt.Errorf("decode paystack %s data: %w", path, err)
	}
	return result.Meta, nil
}

// Bank is a Nigerian bank Paystack can resolve accounts at.
type Bank struct {
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Code   string `json:"code"`
	Active bool   `json:"active"`
}

// maxBankPages stops a misbehaving cursor from looping forever.
const maxBankPages = 20

// ListBanks returns every Nigerian bank, following Paystack's cursor.
func (c *Client) ListBanks(ctx context.Context) ([]Bank, error) {
	var banks []Bank
	next := ""
	for page := 0; page < maxBankPages; page++ {
		query := url.Values{"country": {"nigeria"}, "currency": {"NGN"}, "perPage": {"100"}, "use_cursor": {"true"}}
		if next != "" {
			query.Set("next", next)
		}
		var batch []Bank
		raw, err := c.callWithMeta(ctx, http.MethodGet, "/bank?"+query.Encode(), nil, &batch)
		if err != nil {
			return nil, err
		}
		banks = append(banks, batch...)
		var meta struct {
			Next *string `json:"next"`
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &meta)
		}
		if meta.Next == nil || *meta.Next == "" || *meta.Next == next {
			return banks, nil
		}
		next = *meta.Next
	}
	return banks, nil
}

// ResolvedAccount is the account holder's name as the bank reports it.
type ResolvedAccount struct {
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
}

// ResolveAccount asks Paystack who owns accountNumber at bankCode. Paystack
// answers 422 when the account does not exist.
func (c *Client) ResolveAccount(ctx context.Context, accountNumber, bankCode string) (*ResolvedAccount, error) {
	query := url.Values{"account_number": {accountNumber}, "bank_code": {bankCode}}
	var out ResolvedAccount
	if err := c.call(ctx, http.MethodGet, "/bank/resolve?"+query.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
