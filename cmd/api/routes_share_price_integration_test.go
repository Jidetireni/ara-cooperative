//go:build integration

package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/pkg/paystack"
)

func TestSharePriceUsesConfigurationAndPreservesCheckout(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	fake, server := newFakePaystack(t)
	cfg := &config.Config{
		Shares:   config.SharesConfig{UnitPrice: 75_000},
		Paystack: config.PaystackConfig{SecretKey: testPaystackKey, BaseURL: server.URL, OwnerSubaccount: testOwnerAccount},
		Payments: config.PaymentsConfig{MaintenanceFeePercent: 5},
	}
	router := buildTestRouter(t, db, cfg, nil, paystack.New(testPaystackKey, server.URL))
	seedSessionUser(t, db, []string{"member", "admin"}, true, true)
	access := invitationAdminToken(t, router)
	if _, err := db.Exec("INSERT INTO share_unit_prices (price) VALUES (999999)"); err != nil {
		t.Fatal(err)
	}
	var quote struct{ Data dto.GetUnitsQuote }
	w := sessionRequest(router, http.MethodGet, "/api/v1/shares/quotes?amount=112500", "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &quote) != nil || quote.Data.UnitPrice != 75_000 || quote.Data.Units != 1.5 {
		t.Fatalf("share quote did not use configured price: %d %s", w.Code, w.Body.String())
	}
	if w := sessionRequest(router, http.MethodPatch, "/api/v1/shares/unit-price", `{"unit_price":100}`, access, nil); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("removed price-write endpoint: %d", w.Code)
	}
	payment := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "SHARES", Amount: 112_500})
	cfg.Shares.UnitPrice = 100_000
	router = buildTestRouter(t, db, cfg, nil, paystack.New(testPaystackKey, server.URL))
	fake.pay(payment.Reference, payment.Total)
	if w := signedWebhook(router, paystack.EventChargeSuccess, payment.Reference); w.Code != http.StatusOK {
		t.Fatal("share webhook failed after price change")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM shares s JOIN transactions t ON t.id = s.transaction_id
		WHERE t.reference = $1 AND s.unit_price = 75000 AND s.units = 1.5`, payment.Reference) != 1 {
		t.Fatal("configuration change altered previously quoted shares")
	}
	w = sessionRequest(router, http.MethodGet, "/api/v1/shares/quotes?amount=112500", "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &quote) != nil || quote.Data.UnitPrice != 100_000 || quote.Data.Units != 1.125 {
		t.Fatal("new quotes did not use the changed configuration")
	}
}
