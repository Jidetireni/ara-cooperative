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
	_, err := client.InitializeTransaction(context.Background(), InitializeInput{
		Email: "member@example.com", Amount: 106_599, Currency: "NGN", Reference: "owner-split",
		Split: &Split{
			Type: SplitTypeFlat, BearerType: BearerTypeSubaccount, BearerSubaccount: "ACCT_owner",
			Subaccounts: []SplitSubaccount{{Subaccount: "ACCT_owner", Share: 6_599}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := <-observed
	var split struct {
		Type             string `json:"type"`
		BearerType       string `json:"bearer_type"`
		BearerSubaccount string `json:"bearer_subaccount"`
		Subaccounts      []struct {
			Subaccount string `json:"subaccount"`
			Share      int64  `json:"share"`
		} `json:"subaccounts"`
	}
	if err := json.Unmarshal(body["split"], &split); err != nil {
		t.Fatal(err)
	}
	if split.Type != "flat" || split.BearerType != "subaccount" || split.BearerSubaccount != "ACCT_owner" {
		t.Fatalf("checkout did not identify the owner as fee bearer: %+v", split)
	}
	if len(split.Subaccounts) != 1 || split.Subaccounts[0].Subaccount != "ACCT_owner" || split.Subaccounts[0].Share != 6_599 {
		t.Fatalf("checkout did not allocate maintenance plus processing to owner: %+v", split)
	}
}
