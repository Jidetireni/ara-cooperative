//go:build integration

package main

import (
	"net/http"
	"testing"
)

func TestAdminUpdatesExistingMember(t *testing.T) {
	for _, role := range []string{"admin", "owner"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router, _ := paymentsTestRouter(t, db, testOwnerAccount)
			seedSessionUser(t, db, []string{role, "member"}, true, false)
			access := invitationAdminToken(t, router)
			path := "/api/v1/members/ara000001/existing-member"
			setExisting := func(enabled bool) {
				t.Helper()
				body := `{"is_existing_member":false}`
				if enabled {
					body = `{"is_existing_member":true}`
				}
				member := profileResult(t, sessionRequest(router, http.MethodPatch, path, body, access, nil))
				if member.IsExistingMember != enabled {
					t.Fatalf("classification not saved: %+v", member)
				}
			}
			setExisting(true)
			member := profileResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/me", "", access, nil))
			if member.IsActive {
				t.Fatal("incomplete application activated on toggle")
			}
			if w := sessionRequest(router, http.MethodGet, "/api/v1/payments/quote?purpose=REGISTRATION_FEE", "", access, nil); w.Code != http.StatusConflict {
				t.Fatalf("fee exemption not applied: %d %s", w.Code, w.Body.String())
			}
			setExisting(false)
			if w := sessionRequest(router, http.MethodGet, "/api/v1/payments/quote?purpose=REGISTRATION_FEE", "", access, nil); w.Code != http.StatusOK {
				t.Fatalf("pending member fee not restored: %d %s", w.Code, w.Body.String())
			}
			member = profileResult(t, sessionRequest(router, http.MethodPut, "/api/v1/members/me", invitationBody(t, validProfileInput()), access, nil))
			if member.IsActive || member.IsExistingMember {
				t.Fatal("disabled exemption still activated membership")
			}
			setExisting(true)
			member = profileResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/me", "", access, nil))
			if !member.IsActive || member.PassportPhoto != nil {
				t.Fatal("saved application did not activate without a photo")
			}
			setExisting(true)
			setExisting(false)
			member = profileResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/me", "", access, nil))
			if !member.IsActive {
				t.Fatal("disabling exemption revoked active membership")
			}
			if countRows(t, db, "SELECT COUNT(*) FROM payments") != 0 || countRows(t, db, "SELECT COUNT(*) FROM transactions") != 0 {
				t.Fatal("classification changes created financial entries")
			}
			for _, body := range []string{`{}`, `{"is_existing_member":null}`, `{"is_existing_member":"true"}`} {
				if w := sessionRequest(router, http.MethodPatch, path, body, access, nil); w.Code != http.StatusBadRequest {
					t.Fatalf("invalid boolean accepted: %d %s", w.Code, w.Body.String())
				}
			}
			if w := sessionRequest(router, http.MethodPatch, "/api/v1/members/unknown/existing-member", `{"is_existing_member":true}`, access, nil); w.Code != http.StatusNotFound {
				t.Fatalf("unknown member: got %d", w.Code)
			}
		})
	}
}

func TestExistingMemberUpdateRequiresAdmin(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	seedSessionUser(t, db, []string{"member"}, true, false)
	access := invitationAdminToken(t, router)
	path := "/api/v1/members/ara000001/existing-member"
	body := `{"is_existing_member":true}`
	if w := sessionRequest(router, http.MethodPatch, path, body, "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned request: got %d", w.Code)
	}
	if w := sessionRequest(router, http.MethodPatch, path, body, access, nil); w.Code != http.StatusForbidden {
		t.Fatalf("ordinary member changed exemption: got %d", w.Code)
	}
}
