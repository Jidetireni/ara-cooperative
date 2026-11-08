//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/helpers"
	"github.com/Jidetireni/ara-cooperative/pkg/email"
	"github.com/google/uuid"
)

type invitationEmailSender struct {
	mu       sync.Mutex
	messages []email.SendEmailInput
	fail     bool
}

func (s *invitationEmailSender) Send(_ context.Context, input *email.SendEmailInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("test delivery unavailable")
	}
	s.messages = append(s.messages, *input)
	return nil
}

func (s *invitationEmailSender) setup(t *testing.T) (string, string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) == 0 {
		t.Fatal("invitation email missing")
	}
	body := s.messages[len(s.messages)-1].Body
	match := regexp.MustCompile(`href="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatal("invitation link missing")
	}
	link, err := url.Parse(html.UnescapeString(match[1]))
	if err != nil || link.Path != "/set-password" || link.RawQuery != "" {
		t.Fatal("setup link must carry its credentials in the fragment")
	}
	// Browser location.hash is still escaped; URLSearchParams decodes it once.
	values, err := url.ParseQuery(link.EscapedFragment())
	if err != nil || len(values.Get("token")) != 64 {
		t.Fatal("strong setup token missing")
	}
	return values.Get("email"), values.Get("token")
}

func invitationBody(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func invitationResult(t *testing.T, w *httptest.ResponseRecorder) dto.MemberInvitation {
	t.Helper()
	var response struct{ Data dto.MemberInvitation }
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("invite failed: %d %s", w.Code, w.Body.String())
	}
	if response.Data.Member == nil || response.Data.Member.IsActive {
		t.Fatal("invitation must return a pending member")
	}
	if strings.Contains(w.Body.String(), "token") || strings.Contains(w.Body.String(), "password_hash") {
		t.Fatal("invitation response exposes setup credentials")
	}
	return response.Data
}

func invitationAdminToken(t *testing.T, router http.Handler) string {
	t.Helper()
	return readAuthResponse(t, sessionRequest(router, http.MethodPost, "/api/v1/login",
		`{"email":"session@example.com","password":"test-password"}`, "", nil)).AccessToken
}

func TestMemberInvitationFlow(t *testing.T) {
	for _, role := range []string{"owner", "admin"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			sender := &invitationEmailSender{}
			router := invitationTestRouter(t, db, sender)
			seedSessionUser(t, db, []string{role}, false, false)
			access := invitationAdminToken(t, router)
			body := `{"email":"Ada@Example.com","first_name":"Ada","last_name":"Member"}`
			invite := invitationResult(t, sessionRequest(router, http.MethodPost, "/api/v1/members/", body, access, nil))
			if invite.Email != "ada@example.com" || !invite.RequiresPasswordSetup || invite.Delivery != "sent" {
				t.Fatalf("incorrect invitation result: %+v", invite)
			}
			if invite.ExpiresAt == nil || time.Until(*invite.ExpiresAt) < 29*time.Minute {
				t.Fatal("invitation expiry does not match its email")
			}
			address, secret := sender.setup(t)
			var storedHash string
			if err := db.Get(&storedHash, `SELECT token FROM tokens WHERE user_id =
				(SELECT user_id FROM members WHERE id = $1) AND token_type = 'set_password_token'`, invite.Member.ID); err != nil {
				t.Fatal(err)
			}
			if storedHash != helpers.HashToken(secret) {
				t.Fatal("setup secret was not hashed in storage")
			}
			// A second incomplete profile must not collide on phone or member number.
			second := invitationResult(t, sessionRequest(router, http.MethodPost, "/api/v1/members/",
				`{"email":"second@example.com","first_name":"Second"}`, access, nil))
			if second.Member.Slug == invite.Member.Slug || invite.Member.Phone != "" {
				t.Fatal("pending profiles did not receive independent member identities")
			}
			setupBody := invitationBody(t, dto.SetPasswordInput{Email: strings.ToUpper(address), Password: "member-password", Token: secret})
			setup := sessionRequest(router, http.MethodPost, "/api/v1/set-password", setupBody, "", nil)
			var response struct{ Data dto.AuthResponse }
			if setup.Code != http.StatusCreated || json.Unmarshal(setup.Body.Bytes(), &response) != nil {
				t.Fatalf("invitation acceptance failed: %d %s", setup.Code, setup.Body.String())
			}
			member := response.Data.User
			if member == nil || member.Member == nil || member.Member.IsActive || member.ConfirmedAt == nil {
				t.Fatal("password setup must confirm the account without activating membership")
			}
			if len(member.Roles) != 1 || member.Roles[0] != "member" {
				t.Fatal("invitation granted unexpected roles")
			}
			if me := sessionRequest(router, http.MethodGet, "/api/v1/me", "", response.Data.AccessToken, nil); me.Code != http.StatusOK {
				t.Fatal("pending member cannot read their account")
			}
			for _, path := range []string{"savings/me", "special-deposit/me", "shares/me/total", "fines/me"} {
				if denied := sessionRequest(router, http.MethodGet, "/api/v1/"+path, "", response.Data.AccessToken, nil); denied.Code != http.StatusForbidden {
					t.Fatalf("pending member accessed %s: %d", path, denied.Code)
				}
			}
			if replay := sessionRequest(router, http.MethodPost, "/api/v1/set-password", setupBody, "", nil); replay.Code != http.StatusBadRequest {
				t.Fatal("setup link was accepted twice")
			}
			login := sessionRequest(router, http.MethodPost, "/api/v1/login",
				`{"email":"ADA@example.com","password":"member-password"}`, "", nil)
			if readAuthResponse(t, login).User.Member.IsActive {
				t.Fatal("ordinary login activated membership")
			}
		})
	}
}

func TestMemberInvitationRetryAndDelivery(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	sender := &invitationEmailSender{fail: true}
	router := invitationTestRouter(t, db, sender)
	seedSessionUser(t, db, []string{"admin"}, false, false)
	access := invitationAdminToken(t, router)
	body := `{"email":"invite@example.com","first_name":"Invited"}`
	first := invitationResult(t, sessionRequest(router, http.MethodPost, "/api/v1/members/", body, access, nil))
	if first.Delivery != "failed" {
		t.Fatal("failed delivery was reported as sent")
	}
	sender.fail = false
	second := invitationResult(t, sessionRequest(router, http.MethodPost, "/api/v1/members/", body, access, nil))
	address, oldSecret := sender.setup(t)
	third := invitationResult(t, sessionRequest(router, http.MethodPost, "/api/v1/members/", body, access, nil))
	_, newSecret := sender.setup(t)
	if first.Member.ID != second.Member.ID || second.Member.ID != third.Member.ID || oldSecret == newSecret {
		t.Fatal("retry duplicated membership or reused the old link")
	}
	for name, value := range map[string]string{"old": oldSecret, "wrong": strings.Repeat("0", 64)} {
		setup := invitationBody(t, dto.SetPasswordInput{Email: address, Password: "member-password", Token: value})
		if got := sessionRequest(router, http.MethodPost, "/api/v1/set-password", setup, "", nil); got.Code != http.StatusBadRequest {
			t.Fatalf("%s token accepted", name)
		}
	}
	if _, err := db.Exec(`UPDATE tokens SET expires_at = NOW() - INTERVAL '1 minute'
		WHERE token = $1`, helpers.HashToken(newSecret)); err != nil {
		t.Fatal(err)
	}
	setup := invitationBody(t, dto.SetPasswordInput{Email: address, Password: "member-password", Token: newSecret})
	if got := sessionRequest(router, http.MethodPost, "/api/v1/set-password", setup, "", nil); got.Code != http.StatusBadRequest {
		t.Fatal("expired token accepted")
	}
}

func TestMemberInvitationPreservesExistingAdmin(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	sender := &invitationEmailSender{}
	router := invitationTestRouter(t, db, sender)
	id := seedSessionUser(t, db, []string{"admin"}, false, false)
	access := invitationAdminToken(t, router)
	result := invitationResult(t, sessionRequest(router, http.MethodPost, "/api/v1/members/",
		`{"email":"session@example.com","first_name":"Staff"}`, access, nil))
	if result.RequiresPasswordSetup || result.ExpiresAt != nil || len(sender.messages) != 1 || strings.Contains(sender.messages[0].Body, "/set-password") {
		t.Fatal("existing account was asked to change its password")
	}
	var userID uuid.UUID
	if err := db.Get(&userID, "SELECT user_id FROM members WHERE id = $1", result.Member.ID); err != nil || userID != id {
		t.Fatal("existing account was not reused")
	}
	login := sessionRequest(router, http.MethodPost, "/api/v1/login",
		`{"email":"session@example.com","password":"test-password"}`, "", nil)
	if got := readAuthResponse(t, login); len(got.User.Roles) != 2 || got.User.Member == nil {
		t.Fatal("existing admin access or personal membership was lost")
	}
}

func TestMemberInvitationAccessAndValidation(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	id := seedSessionUser(t, db, []string{"member"}, true, false)
	access := invitationAdminToken(t, router)
	body := `{"email":"invite@example.com","first_name":"Invited"}`
	for name, auth := range map[string]string{"anonymous": "", "member": access} {
		want := http.StatusForbidden
		if name == "anonymous" {
			want = http.StatusUnauthorized
		}
		if got := sessionRequest(router, http.MethodPost, "/api/v1/members/", body, auth, nil); got.Code != want {
			t.Fatalf("%s can invite: %d", name, got.Code)
		}
	}
	if _, err := db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE name = 'admin'`, id); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		`{"email":"bad","first_name":"Ada"}`,
		`{"email":"invite@example.com","first_name":"   "}`,
		`{"email":"invite@example.com","first_name":"Ada","phone":"08000000000"}`,
	} {
		if got := sessionRequest(router, http.MethodPost, "/api/v1/members/", invalid, access, nil); got.Code != http.StatusBadRequest {
			t.Fatalf("invalid invitation accepted: %d", got.Code)
		}
	}
	if _, err := db.Exec("UPDATE members SET activated_at = NOW() WHERE user_id = $1", id); err != nil {
		t.Fatal(err)
	}
	if got := sessionRequest(router, http.MethodPost, "/api/v1/members/",
		`{"email":"session@example.com","first_name":"Existing"}`, access, nil); got.Code != http.StatusConflict {
		t.Fatal("active member was invited again")
	}
}

func TestMemberSetupConcurrentAcceptance(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	sender := &invitationEmailSender{}
	router := invitationTestRouter(t, db, sender)
	seedSessionUser(t, db, []string{"admin"}, false, false)
	access := invitationAdminToken(t, router)
	invitationResult(t, sessionRequest(router, http.MethodPost, "/api/v1/members/",
		`{"email":"invite@example.com","first_name":"Invited"}`, access, nil))
	address, secret := sender.setup(t)
	body := invitationBody(t, dto.SetPasswordInput{Email: address, Password: "member-password", Token: secret})
	start := make(chan struct{})
	results := make(chan int, 2)
	for range 2 {
		go func() {
			<-start
			results <- sessionRequest(router, http.MethodPost, "/api/v1/set-password", body, "", nil).Code
		}()
	}
	close(start)
	counts := map[int]int{}
	for range 2 {
		counts[<-results]++
	}
	if counts[http.StatusCreated] != 1 || counts[http.StatusBadRequest] != 1 {
		t.Fatalf("setup link did not have exactly one successful consumer: %v", counts)
	}
}
