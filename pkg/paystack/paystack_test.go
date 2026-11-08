package paystack

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestValidSignature(t *testing.T) {
	c := New("sk_test_secret", "")
	body := []byte(`{"event":"charge.success"}`)
	if !c.ValidSignature(body, sign("sk_test_secret", body)) {
		t.Fatal("valid signature rejected")
	}
	for name, sig := range map[string]string{
		"empty":      "",
		"wrong key":  sign("sk_test_other", body),
		"other body": sign("sk_test_secret", []byte(`{"event":"charge.failed"}`)),
		"garbage":    "abc",
	} {
		if c.ValidSignature(body, sig) {
			t.Fatalf("%s signature accepted", name)
		}
	}
	if New("", "").ValidSignature(body, sign("", body)) {
		t.Fatal("unconfigured client accepted a signature")
	}
}

func TestInitializeAndVerify(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk_test_secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":false,"message":"Invalid key"}`))
			return
		}
		switch r.URL.Path {
		case "/transaction/initialize":
			var input InitializeInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Amount != 105_000 || input.Metadata.Purpose != "SAVINGS" {
				t.Errorf("unexpected initialize body: %+v %v", input, err)
			}
			_, _ = w.Write([]byte(`{"status":true,"message":"ok","data":{"authorization_url":"https://checkout.paystack.com/abc","access_code":"abc","reference":"` + input.Reference + `"}}`))
		case "/transaction/verify/ara-ref%2F1", "/transaction/verify/ara-ref/1":
			// Large amounts must survive without float rounding.
			_, _ = w.Write([]byte(`{"status":true,"message":"ok","data":{"id":9007199254740993,"status":"success","reference":"ara-ref/1","amount":9007199254740993,"currency":"NGN","fees":150}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":false,"message":"Transaction reference not found"}`))
		}
	}))
	defer server.Close()

	c := New("sk_test_secret", server.URL)
	init, err := c.InitializeTransaction(context.Background(), InitializeInput{
		Email: "ada@example.com", Amount: 105_000, Currency: "NGN", Reference: "ara-ref",
		Metadata: Metadata{Purpose: "SAVINGS"},
	})
	if err != nil || init.AuthorizationURL != "https://checkout.paystack.com/abc" || init.Reference != "ara-ref" {
		t.Fatalf("initialize: %+v %v", init, err)
	}

	tx, err := c.VerifyTransaction(context.Background(), "ara-ref/1")
	if err != nil || tx.Amount != 9007199254740993 || tx.Status != StatusSuccess {
		t.Fatalf("verify: %+v %v", tx, err)
	}

	_, err = c.VerifyTransaction(context.Background(), "missing")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusNotFound {
		t.Fatalf("missing reference error: %v", err)
	}

	_, err = New("sk_test_wrong", server.URL).VerifyTransaction(context.Background(), "x")
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("bad key error: %v", err)
	}

	if _, err := New("", server.URL).VerifyTransaction(context.Background(), "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured client: %v", err)
	}
}

func TestInitializeOwnerSplit(t *testing.T) {
	observed := make(chan map[string]json.RawMessage, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid checkout", http.StatusBadRequest)
			return
		}
		observed <- body
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"status":true,"data":{"authorization_url":"https://checkout.paystack.test/owner","reference":"owner-split"}}`))
		if err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client := New("sk_test_split", server.URL)
	// A NGN 50 owner share: below the NGN 100 minimum of a dynamic flat split.
	_, err := client.InitializeTransaction(context.Background(), InitializeInput{
		Email: "member@example.com", Amount: 105_000, Currency: "NGN", Reference: "owner-split",
		Subaccount: "ACCT_owner", TransactionCharge: 100_000, Bearer: BearerSubaccount,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := <-observed
	var got struct {
		Subaccount        string `json:"subaccount"`
		TransactionCharge int64  `json:"transaction_charge"`
		Bearer            string `json:"bearer"`
	}
	raw, _ := json.Marshal(body)
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Subaccount != "ACCT_owner" || got.TransactionCharge != 100_000 || got.Bearer != "subaccount" {
		t.Fatalf("checkout did not route the owner share: %+v", got)
	}
	if _, ok := body["split"]; ok {
		t.Fatal("checkout must not send a dynamic split")
	}
}
