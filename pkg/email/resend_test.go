package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jidetireni/ara-cooperative/internal/config"
)

func TestResendSend(t *testing.T) {
	type observedRequest struct {
		auth string
		body resendRequest
	}
	observed := make(chan observedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/emails" {
			http.Error(w, "unexpected route", http.StatusNotFound)
			return
		}
		var body resendRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		observed <- observedRequest{auth: r.Header.Get("Authorization"), body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_1"}`))
	}))
	t.Cleanup(server.Close)

	sender, err := New(&config.Config{Email: config.EmailConfig{
		ResendAPIKey: "re_test", ResendBaseURL: server.URL + "/",
		FromAddress: "no-reply@mail.aracoop.xyz", FromName: "Ara Cooperative",
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(context.Background(), &SendEmailInput{
		To: "member@example.com", Subject: "Invitation", Body: "<p>Hi</p>", Text: "Hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := <-observed
	if got.auth != "Bearer re_test" {
		t.Fatal("missing bearer API key")
	}
	b := got.body
	if b.From != `"Ara Cooperative" <no-reply@mail.aracoop.xyz>` || len(b.To) != 1 || b.To[0] != "member@example.com" ||
		b.Subject != "Invitation" || b.HTML != "<p>Hi</p>" || b.Text != "Hi" {
		t.Fatalf("unexpected payload: %+v", b)
	}
}

func TestResendSendError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403,"name":"validation_error","message":"The domain is not verified."}`))
	}))
	t.Cleanup(server.Close)

	sender, err := New(&config.Config{Email: config.EmailConfig{
		ResendAPIKey: "re_test", ResendBaseURL: server.URL, FromAddress: "a@example.com",
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(context.Background(), &SendEmailInput{To: "m@example.com", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "domain is not verified") || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want Resend error message, got %v", err)
	}
	if strings.Contains(err.Error(), "re_test") {
		t.Fatal("error leaked the API key")
	}
}

func TestDevelopmentSendsThroughResendWhenConfigured(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"id":"email_1"}`))
	}))
	t.Cleanup(server.Close)
	input := &SendEmailInput{To: "member@example.com", Subject: "Invitation", Text: "Hi"}

	withResend, err := New(&config.Config{IsDev: true, Email: config.EmailConfig{
		ResendAPIKey: "re_test", ResendBaseURL: server.URL + "/", FromAddress: "no-reply@mail.aracoop.xyz",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := withResend.Send(context.Background(), input); err != nil || calls != 1 {
		t.Fatalf("development with Resend did not send: calls=%d err=%v", calls, err)
	}

	printOnly, err := New(&config.Config{IsDev: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := printOnly.Send(context.Background(), input); err != nil || calls != 1 {
		t.Fatalf("development without Resend should only print: calls=%d err=%v", calls, err)
	}
}
