//go:build integration

package main

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/services/transactions"
	"github.com/Jidetireni/ara-cooperative/pkg/paystack"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const (
	testPaystackKey    = "sk_test_integration"
	testOwnerAccount   = "ACCT_platform_owner"
	testCallbackURL    = "http://localhost:3000/payments/callback"
	testMaintenanceFee = 5
)

// fakePaystack records checkouts and answers verification with whatever the
// test marked as paid.
type fakePaystack struct {
	mu          sync.Mutex
	initialized []paystack.InitializeInput
	charges     map[string]paystack.Transaction
	verified    []string
}

func newFakePaystack(t *testing.T) (*fakePaystack, *httptest.Server) {
	t.Helper()
	fake := &fakePaystack{charges: map[string]paystack.Transaction{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /transaction/initialize", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testPaystackKey {
			http.Error(w, `{"status":false,"message":"bad key"}`, http.StatusUnauthorized)
			return
		}
		var input paystack.InitializeInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, `{"status":false,"message":"bad body"}`, http.StatusBadRequest)
			return
		}
		fake.mu.Lock()
		fake.initialized = append(fake.initialized, input)
		fake.mu.Unlock()
		fmt.Fprintf(w, `{"status":true,"message":"ok","data":{"authorization_url":"https://checkout.paystack.test/%s","access_code":"ac_%s","reference":%q}}`,
			input.Reference, input.Reference, input.Reference)
	})
	mux.HandleFunc("GET /transaction/verify/{reference}", func(w http.ResponseWriter, r *http.Request) {
		reference := r.PathValue("reference")
		fake.mu.Lock()
		fake.verified = append(fake.verified, reference)
		charge, ok := fake.charges[reference]
		fake.mu.Unlock()
		if !ok {
			charge = paystack.Transaction{Status: paystack.StatusAbandoned, Reference: reference, Currency: "NGN"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "message": "ok", "data": charge})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return fake, server
}

// pay marks a reference as successfully charged for amount kobo.
func (f *fakePaystack) pay(reference string, amount int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	paidAt := time.Now()
	f.charges[reference] = paystack.Transaction{
		ID: int64(len(f.charges) + 1), Status: paystack.StatusSuccess, Reference: reference,
		Amount: amount, Currency: "NGN", Fees: 1_500, Channel: "card", PaidAt: &paidAt,
	}
}

func (f *fakePaystack) lastCheckout(t *testing.T) paystack.InitializeInput {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.initialized) == 0 {
		t.Fatal("no checkout was initialized")
	}
	return f.initialized[len(f.initialized)-1]
}

func paymentsTestRouter(t *testing.T, db *sqlx.DB, subaccount string) (http.Handler, *fakePaystack) {
	t.Helper()
	fake, server := newFakePaystack(t)
	cfg := &config.Config{
		Server: config.ServerConfig{FEURL: "http://localhost:3000"},
		Paystack: config.PaystackConfig{
			SecretKey: testPaystackKey, BaseURL: server.URL,
			CallbackURL: testCallbackURL, OwnerSubaccount: subaccount,
		},
		Payments: config.PaymentsConfig{MaintenanceFeePercent: testMaintenanceFee},
	}
	return buildTestRouter(t, db, cfg, nil, paystack.New(testPaystackKey, server.URL)), fake
}

func paymentResult(t *testing.T, w *httptest.ResponseRecorder, status int) dto.Payment {
	t.Helper()
	var response struct{ Data dto.Payment }
	if w.Code != status || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("payment request: want %d, got %d %s", status, w.Code, w.Body.String())
	}
	return response.Data
}

func startPayment(t *testing.T, router http.Handler, access string, input dto.CreatePaymentInput) dto.Payment {
	t.Helper()
	payment := paymentResult(t, sessionRequest(router, http.MethodPost, "/api/v1/payments",
		invitationBody(t, input), access, nil), http.StatusCreated)
	if payment.Status != dto.PaymentStatusPending || payment.AuthorizationURL == nil || payment.Reference == "" {
		t.Fatalf("checkout not started: %+v", payment)
	}
	return payment
}

func completePaymentProfile(t *testing.T, router http.Handler, access string) dto.Member {
	t.Helper()
	return profileResult(t, sessionRequest(router, http.MethodPut, "/api/v1/members/me",
		`{"phone":"08012345678","address":"12 Example Road, Lagos","next_of_kin_name":"Tola","next_of_kin_phone":"08098765432"}`,
		access, nil))
}

func webhookRequest(router http.Handler, body, signature string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/paystack", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if signature != "" {
		r.Header.Set("X-Paystack-Signature", signature)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func signedWebhook(router http.Handler, event, reference string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"event":%q,"data":{"reference":%q,"status":"success"}}`, event, reference)
	mac := hmac.New(sha512.New, []byte(testPaystackKey))
	mac.Write([]byte(body))
	return webhookRequest(router, body, hex.EncodeToString(mac.Sum(nil)))
}

func countRows(t *testing.T, db *sqlx.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.Get(&count, query, args...); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestPaystackRegistrationFeeActivatesMember(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, fake := paymentsTestRouter(t, db, testOwnerAccount)
	userID := seedSessionUser(t, db, []string{"member"}, true, false)
	access := invitationAdminToken(t, router)

	amount := transactions.DefaultRegistrationFee
	fee := amount * testMaintenanceFee / 100
	var quote struct{ Data dto.PaymentQuote }
	w := sessionRequest(router, http.MethodGet, "/api/v1/payments/quote?purpose=REGISTRATION_FEE", "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &quote) != nil {
		t.Fatalf("quote: %d %s", w.Code, w.Body.String())
	}
	q := quote.Data
	if q.Amount != amount || q.MaintenanceFee != fee || q.MaintenanceFeePercent != testMaintenanceFee ||
		q.ProcessingCharge <= 0 || q.Total != amount+fee+q.ProcessingCharge {
		t.Fatalf("unexpected quote: %+v", q)
	}

	if member := completePaymentProfile(t, router, access); member.IsActive {
		t.Fatal("profile completion activated membership before payment")
	}
	payment := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: dto.LedgerType("REGISTRATION_FEE")})
	if payment.Total != q.Total || payment.Amount != amount || payment.MaintenanceFee != fee {
		t.Fatalf("checkout does not match quote: %+v", payment)
	}
	checkout := fake.lastCheckout(t)
	if checkout.Amount != q.Total || checkout.Reference != payment.Reference || checkout.Email != "session@example.com" ||
		checkout.CallbackURL != testCallbackURL || checkout.Currency != "NGN" {
		t.Fatalf("unexpected checkout request: %+v", checkout)
	}
	if checkout.Metadata.PaymentID != payment.ID.String() || checkout.Metadata.Purpose != "REGISTRATION_FEE" ||
		checkout.Metadata.Amount != amount || checkout.Metadata.MaintenanceFee != fee || checkout.Metadata.MemberNumber != "ara000001" {
		t.Fatalf("metadata missing payment details: %+v", checkout.Metadata)
	}
	split := checkout.Split
	if split == nil || split.Type != paystack.SplitTypeFlat || split.BearerType != paystack.BearerTypeSubaccount ||
		split.BearerSubaccount != testOwnerAccount || len(split.Subaccounts) != 1 {
		t.Fatalf("owner split must name the owner subaccount as fee bearer: %+v", split)
	}
	ownerShare := split.Subaccounts[0]
	if ownerShare.Subaccount != testOwnerAccount || ownerShare.Share != q.MaintenanceFee+q.ProcessingCharge {
		t.Fatalf("owner split must receive maintenance plus processing and bear Paystack fees: %+v", split)
	}
	if checkout.Amount-ownerShare.Share != amount {
		t.Fatal("cooperative main account does not retain the full principal")
	}
	var snapshot struct {
		Account string `json:"settlement_subaccount"`
		Share   int64  `json:"settlement_share"`
	}
	if err := db.Get(&snapshot, "SELECT settlement_subaccount, settlement_share FROM payments WHERE reference = $1",
		payment.Reference); err != nil {
		t.Fatal(err)
	}
	if snapshot.Account != testOwnerAccount || snapshot.Share != ownerShare.Share {
		t.Fatal("saved split does not match the owner allocation sent to Paystack")
	}

	// Not yet paid: status check leaves it pending.
	pending := paymentResult(t, sessionRequest(router, http.MethodGet, "/api/v1/payments/"+payment.Reference, "", access, nil), http.StatusOK)
	if pending.Status != dto.PaymentStatusPending {
		t.Fatalf("unpaid checkout changed state: %+v", pending)
	}

	fake.pay(payment.Reference, q.Total)
	if w := webhookRequest(router, `{"event":"charge.success","data":{"reference":"`+payment.Reference+`"}}`, "bad"); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: want 401, got %d", w.Code)
	}
	if countRows(t, db, "SELECT COUNT(*) FROM members WHERE user_id = $1 AND activated_at IS NOT NULL", userID) != 0 {
		t.Fatal("unsigned webhook activated the member")
	}

	for range 2 { // Paystack retries; the second delivery must change nothing.
		if w := signedWebhook(router, paystack.EventChargeSuccess, payment.Reference); w.Code != http.StatusOK {
			t.Fatalf("webhook: %d %s", w.Code, w.Body.String())
		}
	}
	if countRows(t, db, "SELECT COUNT(*) FROM members WHERE user_id = $1 AND activated_at IS NOT NULL", userID) != 1 {
		t.Fatal("membership fee did not activate the member")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM transactions t JOIN transaction_status s ON s.transaction_id = t.id
		WHERE t.reference = $1 AND t.amount = $2 AND t.ledger = 'REGISTRATION_FEE' AND s.confirmed_at IS NOT NULL`,
		payment.Reference, amount) != 1 {
		t.Fatal("expected exactly one confirmed ledger entry for the underlying amount")
	}
	paid := paymentResult(t, sessionRequest(router, http.MethodGet, "/api/v1/payments/"+payment.Reference, "", access, nil), http.StatusOK)
	if paid.Status != dto.PaymentStatusSuccess || paid.PaidAt == nil || paid.AuthorizationURL != nil {
		t.Fatalf("payment not marked successful: %+v", paid)
	}

	again := sessionRequest(router, http.MethodPost, "/api/v1/payments", `{"purpose":"REGISTRATION_FEE"}`, access, nil)
	if again.Code != http.StatusConflict {
		t.Fatalf("second membership fee: want 409, got %d", again.Code)
	}
}

func TestPaystackMembershipCheckoutRequiresCompleteProfile(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, fake := paymentsTestRouter(t, db, testOwnerAccount)
	userID := seedSessionUser(t, db, []string{"member"}, true, false)
	access := invitationAdminToken(t, router)

	for index, field := range []string{"phone", "address", "next_of_kin_name", "next_of_kin_phone"} {
		for _, value := range []struct {
			name  string
			value any
		}{
			{name: "missing", value: nil},
			{name: "blank", value: " \t "},
		} {
			t.Run(field+"/"+value.name, func(t *testing.T) {
				fields := []any{"08012345678", "12 Example Road, Lagos", "Tola", "08098765432", userID}
				fields[index] = value.value
				if _, err := db.Exec(`UPDATE members SET phone = $1, address = $2,
					next_of_kin_name = $3, next_of_kin_phone = $4 WHERE user_id = $5`, fields...); err != nil {
					t.Fatal(err)
				}
				quote := sessionRequest(router, http.MethodGet,
					"/api/v1/payments/quote?purpose=REGISTRATION_FEE", "", access, nil)
				if quote.Code != http.StatusOK {
					t.Fatalf("incomplete profile cannot view fee quote: %d %s", quote.Code, quote.Body.String())
				}
				checkout := sessionRequest(router, http.MethodPost, "/api/v1/payments",
					`{"purpose":"REGISTRATION_FEE"}`, access, nil)
				if checkout.Code != http.StatusConflict || !strings.Contains(checkout.Body.String(), "next-of-kin") {
					t.Fatalf("incomplete profile checkout: %d %s", checkout.Code, checkout.Body.String())
				}
			})
		}
	}
	if countRows(t, db, "SELECT COUNT(*) FROM payments") != 0 {
		t.Fatal("incomplete profile created a payment record")
	}
	fake.mu.Lock()
	initialized := len(fake.initialized)
	fake.mu.Unlock()
	if initialized != 0 {
		t.Fatal("incomplete profile started a Paystack checkout")
	}
	if countRows(t, db, "SELECT COUNT(*) FROM members WHERE user_id = $1 AND activated_at IS NOT NULL", userID) != 0 {
		t.Fatal("rejected checkout activated membership")
	}
}

func TestPaystackPurposesApplyOnce(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, fake := paymentsTestRouter(t, db, testOwnerAccount)
	userID := seedSessionUser(t, db, []string{"member"}, true, true)
	access := invitationAdminToken(t, router)
	var memberID uuid.UUID
	if err := db.Get(&memberID, "SELECT id FROM members WHERE user_id = $1", userID); err != nil {
		t.Fatal(err)
	}

	// Status reads stay pending even after Paystack is paid, until the webhook arrives.
	savings := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "SAVINGS", Amount: 1_000_000})
	fake.pay(savings.Reference, savings.Total)
	if got := paymentResult(t, sessionRequest(router, http.MethodGet,
		"/api/v1/payments/"+savings.Reference, "", access, nil), http.StatusOK); got.Status != dto.PaymentStatusPending {
		t.Fatal("status read confirmed a payment before its webhook")
	}
	fake.mu.Lock()
	verificationCount := len(fake.verified)
	fake.mu.Unlock()
	if verificationCount != 0 {
		t.Fatal("status read called Paystack verification")
	}
	if countRows(t, db, "SELECT COUNT(*) FROM transactions WHERE reference = $1", savings.Reference) != 0 {
		t.Fatal("status read credited savings before its webhook")
	}
	if w := signedWebhook(router, paystack.EventChargeSuccess, savings.Reference); w.Code != http.StatusOK {
		t.Fatalf("savings webhook: %d %s", w.Code, w.Body.String())
	}
	if got := paymentResult(t, sessionRequest(router, http.MethodGet, "/api/v1/payments/"+savings.Reference, "", access, nil), http.StatusOK); got.Status != dto.PaymentStatusSuccess {
		t.Fatalf("savings not applied: %+v", got)
	}
	if countRows(t, db, "SELECT COUNT(*) FROM transactions WHERE member_id = $1 AND ledger = 'SAVINGS' AND amount = 1000000", memberID) != 1 {
		t.Fatal("savings ledger entry missing")
	}

	// Shares: concurrent webhooks create one share row from the checkout quote.
	shares := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "SHARES", Amount: 10 * transactions.DefaultSharesUnitPrice})
	fake.pay(shares.Reference, shares.Total)
	var wg sync.WaitGroup
	codes := make([]int, 5)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = signedWebhook(router, paystack.EventChargeSuccess, shares.Reference).Code
		}()
	}
	wg.Wait()
	for _, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("concurrent webhook answered %d", code)
		}
	}
	if countRows(t, db, `SELECT COUNT(*) FROM shares s JOIN transactions t ON t.id = s.transaction_id
		WHERE t.reference = $1 AND s.units = 10 AND s.unit_price = $2`, shares.Reference, transactions.DefaultSharesUnitPrice) != 1 {
		t.Fatal("expected one share purchase of 10 units")
	}

	// Fines: the fine is marked paid and linked to the ledger entry.
	var fineID uuid.UUID
	if err := db.Get(&fineID, `INSERT INTO fines (admin_id, member_id, amount, reason, deadline)
		VALUES ($1, $2, 250000, 'Late meeting', NOW() + INTERVAL '7 days') RETURNING id`, userID, memberID); err != nil {
		t.Fatal(err)
	}
	if w := sessionRequest(router, http.MethodPost, "/api/v1/payments",
		`{"purpose":"FINES","fine_id":"`+fineID.String()+`","amount":1}`, access, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("fine amount mismatch: want 400, got %d", w.Code)
	}
	fine := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "FINES", FineID: &fineID})
	if fine.Amount != 250_000 {
		t.Fatalf("fine payment amount = %d", fine.Amount)
	}
	fake.pay(fine.Reference, fine.Total)
	if w := signedWebhook(router, paystack.EventChargeSuccess, fine.Reference); w.Code != http.StatusOK {
		t.Fatalf("fine webhook: %d", w.Code)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM fines f JOIN transactions t ON t.id = f.transaction_id
		WHERE f.id = $1 AND f.paid_at IS NOT NULL AND t.reference = $2`, fineID, fine.Reference) != 1 {
		t.Fatal("fine not marked paid")
	}
	if w := sessionRequest(router, http.MethodPost, "/api/v1/payments",
		`{"purpose":"FINES","fine_id":"`+fineID.String()+`"}`, access, nil); w.Code != http.StatusConflict {
		t.Fatalf("paid fine: want 409, got %d", w.Code)
	}
}

func TestPaystackMismatchAndIgnoredEvents(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, fake := paymentsTestRouter(t, db, testOwnerAccount)
	userID := seedSessionUser(t, db, []string{"member"}, true, true)
	access := invitationAdminToken(t, router)

	campaignID := seedPaymentCampaign(t, db, userID)
	payment := startPayment(t, router, access, dto.CreatePaymentInput{
		Purpose: "SPECIAL_DEPOSIT", Amount: 100_000, CampaignID: &campaignID,
	})
	fake.pay(payment.Reference, payment.Total-1)
	if w := signedWebhook(router, paystack.EventChargeSuccess, payment.Reference); w.Code != http.StatusOK {
		t.Fatalf("webhook: %d", w.Code)
	}
	failed := paymentResult(t, sessionRequest(router, http.MethodGet, "/api/v1/payments/"+payment.Reference, "", access, nil), http.StatusOK)
	if failed.Status != dto.PaymentStatusFailed || failed.FailureReason == nil {
		t.Fatalf("underpayment must fail: %+v", failed)
	}
	if countRows(t, db, "SELECT COUNT(*) FROM transactions WHERE reference = $1", payment.Reference) != 0 {
		t.Fatal("underpayment was credited")
	}

	for _, w := range []*httptest.ResponseRecorder{
		signedWebhook(router, paystack.EventChargeSuccess, "not_ours_123"),
		signedWebhook(router, "transfer.success", payment.Reference),
	} {
		if w.Code != http.StatusOK {
			t.Fatalf("ignored event: want 200, got %d %s", w.Code, w.Body.String())
		}
	}
}

func TestPaystackCheckoutValidation(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, _ := paymentsTestRouter(t, db, testOwnerAccount)
	userID := seedSessionUser(t, db, []string{"member"}, true, false)
	access := invitationAdminToken(t, router)

	tests := []struct {
		name string
		body string
		want int
	}{
		{"savings before activation", `{"purpose":"SAVINGS","amount":100000}`, http.StatusForbidden},
		{"loan is not payable online", `{"purpose":"LOAN","amount":100000}`, http.StatusBadRequest},
		{"fine id on another purpose", `{"purpose":"REGISTRATION_FEE","fine_id":"` + uuid.NewString() + `"}`, http.StatusBadRequest},
		{"wrong membership fee", `{"purpose":"REGISTRATION_FEE","amount":5}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		if w := sessionRequest(router, http.MethodPost, "/api/v1/payments", tt.body, access, nil); w.Code != tt.want {
			t.Errorf("%s: want %d, got %d %s", tt.name, tt.want, w.Code, w.Body.String())
		}
	}

	completePaymentProfile(t, router, access)
	startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "REGISTRATION_FEE"})
	if _, err := db.Exec("UPDATE members SET activated_at = NOW() WHERE user_id = $1", userID); err != nil {
		t.Fatal(err)
	}
	if w := sessionRequest(router, http.MethodPost, "/api/v1/payments", `{"purpose":"SAVINGS","amount":1}`, access, nil); w.Code != http.StatusBadRequest {
		t.Errorf("savings below minimum: want 400, got %d", w.Code)
	}

	// Another member's payment is not visible.
	var otherReference string
	if err := db.Get(&otherReference, `WITH u AS (
			INSERT INTO users (email, password_hash) VALUES ('other@example.com', 'x') RETURNING id
		), m AS (
			INSERT INTO members (user_id, slug, first_name, last_name, phone)
			SELECT id, 'ara000002', 'Other', 'Member', '08000000001' FROM u RETURNING id
		)
		INSERT INTO payments (member_id, purpose, reference, amount, maintenance_fee, maintenance_fee_percent,
			processing_charge, total, description)
		SELECT id, 'SAVINGS', 'ara_sav_other', 1000, 0, 0, 0, 1000, 'Savings deposit' FROM m RETURNING reference`); err != nil {
		t.Fatal(err)
	}
	if w := sessionRequest(router, http.MethodGet, "/api/v1/payments/"+otherReference, "", access, nil); w.Code != http.StatusNotFound {
		t.Errorf("other member's payment: want 404, got %d", w.Code)
	}
	if w := sessionRequest(router, http.MethodGet, "/api/v1/payments/quote?purpose=SAVINGS", "", "", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous quote: want 401, got %d", w.Code)
	}
}

func TestPaystackCheckoutRequiresOwnerSubaccount(t *testing.T) {
	tests := []struct {
		name        string
		owner       string
		maintenance int32
		purpose     dto.LedgerType
	}{
		{name: "missing owner for membership", maintenance: 5, purpose: "REGISTRATION_FEE"},
		{name: "blank owner for savings", owner: " \t ", maintenance: 5, purpose: "SAVINGS"},
		{name: "missing owner with no maintenance", purpose: "SAVINGS"},
		{name: "blank owner with no maintenance", owner: " \t ", purpose: "REGISTRATION_FEE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			fake, server := newFakePaystack(t)
			cfg := &config.Config{
				Paystack: config.PaystackConfig{
					SecretKey: testPaystackKey, BaseURL: server.URL, OwnerSubaccount: tt.owner,
				},
				Payments: config.PaymentsConfig{MaintenanceFeePercent: tt.maintenance},
			}
			router := buildTestRouter(t, db, cfg, nil, paystack.New(testPaystackKey, server.URL))
			seedSessionUser(t, db, []string{"member"}, true, tt.purpose != "REGISTRATION_FEE")
			access := invitationAdminToken(t, router)
			completePaymentProfile(t, router, access)
			input := dto.CreatePaymentInput{Purpose: tt.purpose}
			if tt.purpose == "SAVINGS" {
				input.Amount = 100_000
			}
			quotePath := "/api/v1/payments/quote?purpose=" + string(input.Purpose)
			if input.Amount > 0 {
				quotePath += fmt.Sprintf("&amount=%d", input.Amount)
			}
			quote := sessionRequest(router, http.MethodGet, quotePath, "", access, nil)
			if quote.Code != http.StatusOK {
				t.Fatalf("quote without owner subaccount: %d %s", quote.Code, quote.Body.String())
			}
			checkout := sessionRequest(router, http.MethodPost, "/api/v1/payments",
				invitationBody(t, input), access, nil)
			if checkout.Code != http.StatusServiceUnavailable ||
				!strings.Contains(checkout.Body.String(), "owner settlement account is not configured") {
				t.Fatalf("checkout without owner subaccount: %d %s", checkout.Code, checkout.Body.String())
			}
			if countRows(t, db, "SELECT COUNT(*) FROM payments") != 0 {
				t.Fatal("missing owner subaccount created a payment record")
			}
			fake.mu.Lock()
			initialized := len(fake.initialized)
			fake.mu.Unlock()
			if initialized != 0 {
				t.Fatal("missing owner subaccount started a Paystack checkout")
			}
		})
	}
}

func TestPaystackExistingCheckoutWebhookWithoutOwnerSubaccount(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, _ := paymentsTestRouter(t, db, testOwnerAccount)
	seedSessionUser(t, db, []string{"member"}, true, true)
	access := invitationAdminToken(t, router)
	payment := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "SAVINGS", Amount: 100_000})

	// Removing the setting must not prevent an existing checkout from being credited.
	router, fake := paymentsTestRouter(t, db, "")
	fake.pay(payment.Reference, payment.Total)
	for range 2 {
		if w := signedWebhook(router, paystack.EventChargeSuccess, payment.Reference); w.Code != http.StatusOK {
			t.Fatalf("existing checkout webhook: %d %s", w.Code, w.Body.String())
		}
	}
	paid := paymentResult(t, sessionRequest(router, http.MethodGet,
		"/api/v1/payments/"+payment.Reference, "", access, nil), http.StatusOK)
	if paid.Status != dto.PaymentStatusSuccess {
		t.Fatalf("existing checkout not credited: %+v", paid)
	}
	if countRows(t, db, "SELECT COUNT(*) FROM transactions WHERE reference = $1 AND amount = 100000",
		payment.Reference) != 1 {
		t.Fatal("existing checkout must be credited once")
	}
	if countRows(t, db, "SELECT COUNT(*) FROM payments WHERE reference = $1 AND settlement_subaccount = $2",
		payment.Reference, testOwnerAccount) != 1 {
		t.Fatal("existing checkout lost its original owner allocation")
	}
}

func TestPaystackNotConfigured(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := invitationTestRouter(t, db, nil)
	seedSessionUser(t, db, []string{"member"}, true, false)
	access := invitationAdminToken(t, router)

	if w := sessionRequest(router, http.MethodPost, "/api/v1/payments", `{"purpose":"REGISTRATION_FEE"}`, access, nil); w.Code != http.StatusServiceUnavailable {
		t.Errorf("checkout without key: want 503, got %d", w.Code)
	}
	if w := webhookRequest(router, `{"event":"charge.success"}`, "sig"); w.Code != http.StatusServiceUnavailable {
		t.Errorf("webhook without key: want 503, got %d", w.Code)
	}
}

func TestAdminListsAndVerifiesPayments(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, fake := paymentsTestRouter(t, db, testOwnerAccount)
	userID := seedSessionUser(t, db, []string{"member", "admin"}, true, true)
	access := invitationAdminToken(t, router)

	savings := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "SAVINGS", Amount: 500_000})
	campaignID := seedPaymentCampaign(t, db, userID)
	campaign := startPayment(t, router, access, dto.CreatePaymentInput{
		Purpose: "SPECIAL_DEPOSIT", Amount: 100_000, CampaignID: &campaignID,
	})

	var list struct{ Data dto.ListResponse[dto.Payment] }
	w := sessionRequest(router, http.MethodGet, "/api/v1/payments?status=pending&limit=10", "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Data.Items) != 2 {
		t.Fatalf("pending list: %d %s", w.Code, w.Body.String())
	}
	if first := list.Data.Items[0]; first.Reference != campaign.Reference || first.Member == nil || first.Member.Slug != "ara000001" {
		t.Fatalf("list is not newest first with member: %+v", first)
	}

	verify := func(reference string, status int) dto.PaymentVerification {
		t.Helper()
		var response struct{ Data dto.PaymentVerification }
		w := sessionRequest(router, http.MethodPost, "/api/v1/payments/"+reference+"/verify", "", access, nil)
		if w.Code != status || (status == http.StatusOK && json.Unmarshal(w.Body.Bytes(), &response) != nil) {
			t.Fatalf("verify %s: want %d, got %d %s", reference, status, w.Code, w.Body.String())
		}
		return response.Data
	}

	// Not paid on Paystack: stays pending and says why.
	abandoned := verify(campaign.Reference, http.StatusOK)
	if abandoned.PaystackStatus != paystack.StatusAbandoned || abandoned.Payment.Status != dto.PaymentStatusPending {
		t.Fatalf("abandoned verify: %+v", abandoned)
	}

	// Paid but the webhook never came: verify applies it once.
	fake.pay(savings.Reference, savings.Total)
	applied := verify(savings.Reference, http.StatusOK)
	if applied.PaystackStatus != paystack.StatusSuccess || applied.Payment.Status != dto.PaymentStatusSuccess ||
		applied.Payment.Member == nil || applied.Payment.PaidAt == nil {
		t.Fatalf("paid verify: %+v", applied)
	}
	verify(savings.Reference, http.StatusConflict)
	if w := signedWebhook(router, paystack.EventChargeSuccess, savings.Reference); w.Code != http.StatusOK {
		t.Fatalf("late webhook: %d", w.Code)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM transactions t JOIN transaction_status s ON s.transaction_id = t.id
		WHERE t.reference = $1 AND t.ledger = 'SAVINGS' AND t.amount = 500000 AND s.confirmed_at IS NOT NULL`, savings.Reference); n != 1 {
		t.Fatalf("savings credited %d times", n)
	}

	verify("ara_sav_unknown", http.StatusNotFound)
	for _, query := range []string{"?status=DONE", "?purpose=LOAN", "?member_id=nope", "?cursor=bad"} {
		if w := sessionRequest(router, http.MethodGet, "/api/v1/payments"+query, "", access, nil); w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d", query, w.Code)
		}
	}
	w = sessionRequest(router, http.MethodGet, "/api/v1/payments?status=SUCCESS&purpose=SAVINGS", "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Data.Items) != 1 {
		t.Fatalf("success filter: %d %s", w.Code, w.Body.String())
	}
}

func TestPaymentAdminRoutesNeedAdmin(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, _ := paymentsTestRouter(t, db, testOwnerAccount)
	seedSessionUser(t, db, []string{"member"}, true, true)
	access := invitationAdminToken(t, router)
	payment := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "SAVINGS", Amount: 500_000})

	for _, req := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/payments"},
		{http.MethodPost, "/api/v1/payments/" + payment.Reference + "/verify"},
	} {
		if w := sessionRequest(router, req.method, req.path, "", access, nil); w.Code != http.StatusForbidden {
			t.Errorf("member %s %s: want 403, got %d", req.method, req.path, w.Code)
		}
		if w := sessionRequest(router, req.method, req.path, "", "", nil); w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s: want 401, got %d", req.method, req.path, w.Code)
		}
	}
	// The member's own status read stays read-only.
	if got := paymentResult(t, sessionRequest(router, http.MethodGet, "/api/v1/payments/"+payment.Reference, "", access, nil), http.StatusOK); got.Status != dto.PaymentStatusPending {
		t.Fatalf("status read changed the payment: %+v", got)
	}
}

func TestPaystackOwnerSplitWithZeroMaintenance(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	fake, server := newFakePaystack(t)
	cfg := &config.Config{
		Server: config.ServerConfig{FEURL: "http://localhost:3000"},
		Paystack: config.PaystackConfig{
			SecretKey: testPaystackKey, BaseURL: server.URL,
			CallbackURL: testCallbackURL, OwnerSubaccount: testOwnerAccount,
		},
		Payments: config.PaymentsConfig{MaintenanceFeePercent: 0},
	}
	router := buildTestRouter(t, db, cfg, nil, paystack.New(testPaystackKey, server.URL))
	seedSessionUser(t, db, []string{"member"}, true, false)
	access := invitationAdminToken(t, router)
	completePaymentProfile(t, router, access)
	payment := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "REGISTRATION_FEE"})
	checkout := fake.lastCheckout(t)
	if payment.MaintenanceFee != 0 || payment.ProcessingCharge <= 0 || checkout.Split == nil {
		t.Fatal("zero-maintenance payment must still cover Paystack processing")
	}
	split := checkout.Split
	if split.BearerType != paystack.BearerTypeSubaccount || split.BearerSubaccount != testOwnerAccount ||
		len(split.Subaccounts) != 1 || split.Subaccounts[0].Share != payment.ProcessingCharge {
		t.Fatalf("owner allocation must cover processing only: %+v", split)
	}
	if checkout.Amount-split.Subaccounts[0].Share != payment.Amount {
		t.Fatal("processing fee reduced the cooperative's principal")
	}
}
