//go:build integration

package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
)

func TestExistingMemberInvitationAndActivation(t *testing.T) {
	for _, role := range []string{"owner", "admin"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			sender := &invitationEmailSender{}
			router, _ := paymentsTestRouterWithEmail(t, db, testOwnerAccount, sender)
			seedSessionUser(t, db, []string{role}, false, false)
			adminAccess := invitationAdminToken(t, router)
			body := `{"email":"existing@example.com","first_name":"Existing","is_existing_member":true}`
			invite := invitationResult(t, sessionRequest(router, http.MethodPost, "/api/v1/members/", body, adminAccess, nil))
			if !invite.Member.IsExistingMember {
				t.Fatal("invitation lost existing-member classification")
			}
			// Delivery retries preserve the original classification even if the toggle is omitted.
			retry := invitationResult(t, sessionRequest(router, http.MethodPost, "/api/v1/members/",
				`{"email":"existing@example.com","first_name":"Existing"}`, adminAccess, nil))
			if retry.Member.ID != invite.Member.ID || !retry.Member.IsExistingMember {
				t.Fatal("invitation retry changed the member classification")
			}
			listed := memberListResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/", "", adminAccess, nil))
			if len(listed.Items) != 1 || !listed.Items[0].IsExistingMember {
				t.Fatal("admin member list lost existing-member classification")
			}
			email, secret := sender.setup(t)
			setup := sessionRequest(router, http.MethodPost, "/api/v1/set-password",
				invitationBody(t, dto.SetPasswordInput{Email: email, Password: "member-password", Token: secret}), "", nil)
			var session struct{ Data dto.AuthResponse }
			if setup.Code != http.StatusCreated || json.Unmarshal(setup.Body.Bytes(), &session) != nil ||
				session.Data.User == nil || session.Data.User.Member == nil {
				t.Fatalf("password setup failed: %d %s", setup.Code, setup.Body.String())
			}
			if !session.Data.User.Member.IsExistingMember || session.Data.User.Member.IsActive {
				t.Fatal("password setup must preserve classification without activating membership")
			}
			access := session.Data.AccessToken
			if w := sessionRequest(router, http.MethodGet, "/api/v1/savings/me", "", access, nil); w.Code != http.StatusForbidden {
				t.Fatal("existing member accessed financial features before completing application")
			}
			for _, request := range []struct{ method, path, body string }{
				{http.MethodGet, "/api/v1/payments/quote?purpose=REGISTRATION_FEE", ""},
				{http.MethodPost, "/api/v1/payments", `{"purpose":"REGISTRATION_FEE"}`},
			} {
				if w := sessionRequest(router, request.method, request.path, request.body, access, nil); w.Code != http.StatusConflict {
					t.Fatalf("existing-member fee request: want 409, got %d %s", w.Code, w.Body.String())
				}
			}
			input := validProfileInput()
			input.DeclarationAccepted = false
			if w := sessionRequest(router, http.MethodPut, "/api/v1/members/me", invitationBody(t, input), access, nil); w.Code != http.StatusBadRequest {
				t.Fatal("existing member saved application without declaration")
			}
			if member := profileResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/me", "", access, nil)); member.IsActive {
				t.Fatal("invalid application activated membership")
			}
			input.DeclarationAccepted = true
			for range 2 {
				saved := profileResult(t, sessionRequest(router, http.MethodPut, "/api/v1/members/me", invitationBody(t, input), access, nil))
				if !saved.IsExistingMember || !saved.IsActive || saved.DeclarationAcceptedAt == nil || saved.PassportPhoto != nil {
					t.Fatalf("existing member did not activate without photo or payment: %+v", saved)
				}
			}
			if w := sessionRequest(router, http.MethodGet, "/api/v1/savings/me", "", access, nil); w.Code != http.StatusOK {
				t.Fatalf("activated member cannot access savings: %d %s", w.Code, w.Body.String())
			}
			var me struct{ Data dto.AuthUser }
			w := sessionRequest(router, http.MethodGet, "/api/v1/me", "", access, nil)
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &me) != nil || me.Data.Member == nil ||
				!me.Data.Member.IsExistingMember || !me.Data.Member.IsActive {
				t.Fatal("account context lost existing-member activation")
			}
			login := sessionRequest(router, http.MethodPost, "/api/v1/login",
				`{"email":"existing@example.com","password":"member-password"}`, "", nil)
			if member := readAuthResponse(t, login).User.Member; member == nil || !member.IsExistingMember || !member.IsActive {
				t.Fatal("login lost existing-member activation")
			}
			refreshed := readAuthResponse(t, sessionRequest(router, http.MethodPost, "/api/v1/refresh", "", "", refreshCookie(t, login)))
			if member := refreshed.User.Member; member == nil || !member.IsExistingMember || !member.IsActive {
				t.Fatal("refresh lost existing-member activation")
			}
			if countRows(t, db, "SELECT COUNT(*) FROM payments") != 0 || countRows(t, db, "SELECT COUNT(*) FROM transactions") != 0 {
				t.Fatal("existing-member activation created a payment or ledger entry")
			}
		})
	}
}

func TestMembersCannotSetExistingMemberFlag(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	seedSessionUser(t, db, []string{"member"}, true, false)
	access := invitationAdminToken(t, router)
	if w := sessionRequest(router, http.MethodPost, "/api/v1/members/",
		`{"email":"other@example.com","first_name":"Other","is_existing_member":true}`, access, nil); w.Code != http.StatusForbidden {
		t.Fatal("ordinary member could invite an existing member")
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(invitationBody(t, validProfileInput())), &input); err != nil {
		t.Fatal(err)
	}
	input["is_existing_member"] = true
	if w := sessionRequest(router, http.MethodPut, "/api/v1/members/me", invitationBody(t, input), access, nil); w.Code != http.StatusBadRequest {
		t.Fatal("profile accepted an admin-only existing-member flag")
	}
	member := profileResult(t, sessionRequest(router, http.MethodPut, "/api/v1/members/me",
		invitationBody(t, validProfileInput()), access, nil))
	if member.IsExistingMember || member.IsActive {
		t.Fatal("new member bypassed registration payment")
	}
}
