//go:build integration

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/google/uuid"
)

func profileResult(t *testing.T, w *httptest.ResponseRecorder) dto.Member {
	t.Helper()
	var response struct{ Data dto.Member }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("profile request failed: %d %s", w.Code, w.Body.String())
	}
	if response.Data.ID == uuid.Nil {
		t.Fatal("profile identity missing")
	}
	return response.Data
}

// validProfileInput fills every field of the paper membership form.
func validProfileInput() dto.UpdateMemberProfileInput {
	pledge := int64(500_000)
	return dto.UpdateMemberProfileInput{
		Title: "Mrs", MiddleName: "Adunni", Phone: "08012345678", AlternatePhone: "08011112222",
		Address: "  12 Example Road, Lagos  ", PlaceOfBirth: "Ado Ekiti", Nationality: "Nigerian",
		StateOfOrigin: "Ekiti", HomeTown: "Ara", LGA: "Ado",
		NextOfKinName: "  Tola Member  ", NextOfKinAddress: "3 Kin Close, Ibadan", NextOfKinPhone: "+2348098765432",
		Workplace: &dto.MemberWorkplace{Company: "Example Ltd", Address: "1 Office Way, Lagos", Since: "2020-01-15",
			Position: "Accountant", Responsibilities: "Payroll"},
		Beneficiaries: []dto.MemberBeneficiaryInput{
			{Name: "Kemi Member", Relationship: "Daughter", Address: "12 Example Road, Lagos", Phone: "08033334444"},
			{Name: "Femi Member", Relationship: "Son", Address: "12 Example Road, Lagos", Phone: "08055556666"},
		},
		MonthlyContributionPledge: &pledge,
		DeclarationAccepted:       true,
	}
}

func TestMemberProfileCompletion(t *testing.T) {
	tests := []struct {
		name   string
		roles  []string
		active bool
	}{
		{name: "pending member", roles: []string{"member"}},
		{name: "active member", roles: []string{"member"}, active: true},
		{name: "admin participating as member", roles: []string{"admin", "member"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router := sessionTestRouter(t, db)
			seedSessionUser(t, db, tt.roles, true, tt.active)
			access := invitationAdminToken(t, router)
			before := profileResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/me", "", access, nil))
			input := validProfileInput()
			saved := profileResult(t, sessionRequest(router, http.MethodPut, "/api/v1/members/me",
				invitationBody(t, input), access, nil))
			if saved.ID != before.ID || saved.Slug != before.Slug || saved.FirstName != before.FirstName || saved.LastName != before.LastName {
				t.Fatal("profile update changed member identity")
			}
			if saved.IsActive != tt.active || saved.Phone != input.Phone || saved.Address != strings.TrimSpace(input.Address) {
				t.Fatalf("profile update lost activation state or contact details: %+v", saved)
			}
			if saved.NextOfKinName != strings.TrimSpace(input.NextOfKinName) || saved.NextOfKinPhone != input.NextOfKinPhone ||
				saved.NextOfKinAddress != input.NextOfKinAddress {
				t.Fatal("next-of-kin details were not saved")
			}
			if saved.Title != "Mrs" || saved.MiddleName != "Adunni" || saved.LGA != "Ado" || saved.StateOfOrigin != "Ekiti" ||
				saved.Workplace == nil || *saved.Workplace != *input.Workplace || len(saved.Beneficiaries) != 2 ||
				saved.Beneficiaries[1].Name != "Femi Member" || saved.MonthlyContributionPledge == nil ||
				*saved.MonthlyContributionPledge != 500_000 || saved.IntroducedByNumber != "" ||
				saved.DeclarationAcceptedAt == nil {
				t.Fatalf("membership form details were not saved: %+v", saved)
			}
			if saved.PassportPhoto != nil || saved.ProfileComplete {
				t.Fatal("profile reported complete without a passport photo")
			}
			read := profileResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/me", "", access, nil))
			if !reflect.DeepEqual(read, saved) {
				t.Fatal("profile read disagrees with saved data")
			}
			var me struct{ Data dto.AuthUser }
			response := sessionRequest(router, http.MethodGet, "/api/v1/me", "", access, nil)
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &me) != nil || me.Data.Member == nil ||
				me.Data.Member.ID != saved.ID || me.Data.Member.Phone != saved.Phone || me.Data.Member.LGA != saved.LGA {
				t.Fatal("account context did not reload the updated profile")
			}
			login := sessionRequest(router, http.MethodPost, "/api/v1/login",
				`{"email":"session@example.com","password":"test-password"}`, "", nil)
			account := readAuthResponse(t, login)
			if account.User.Member == nil || account.User.Member.Phone != saved.Phone || !reflect.DeepEqual(account.User.Roles, tt.roles) {
				t.Fatal("profile update changed roles or login account context")
			}
			if !tt.active {
				financial := sessionRequest(router, http.MethodGet, "/api/v1/savings/me", "", access, nil)
				if financial.Code != http.StatusForbidden {
					t.Fatal("profile completion unlocked financial features")
				}
			}
			input.Address = "45 Updated Road, Lagos"
			updated := profileResult(t, sessionRequest(router, http.MethodPut, "/api/v1/members/me",
				invitationBody(t, input), access, nil))
			if updated.Address != input.Address || updated.ID != saved.ID || updated.IsActive != tt.active {
				t.Fatal("member could not edit their existing completed profile")
			}
		})
	}
}

func TestMemberProfileAccess(t *testing.T) {
	tests := []struct {
		name    string
		roles   []string
		member  bool
		deleted bool
		status  int
	}{
		{name: "admin without personal membership", roles: []string{"admin"}, status: http.StatusForbidden},
		{name: "owner without personal membership", roles: []string{"owner"}, status: http.StatusForbidden},
		{name: "member role without a profile", roles: []string{"member"}, status: http.StatusNotFound},
		{name: "deleted membership", roles: []string{"member"}, member: true, deleted: true, status: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router := sessionTestRouter(t, db)
			id := seedSessionUser(t, db, tt.roles, tt.member, false)
			access := invitationAdminToken(t, router)
			if tt.deleted {
				if _, err := db.Exec("UPDATE members SET deleted_at = NOW() WHERE user_id = $1", id); err != nil {
					t.Fatal(err)
				}
			}
			for _, method := range []string{http.MethodGet, http.MethodPut} {
				body := ""
				if method == http.MethodPut {
					body = invitationBody(t, validProfileInput())
				}
				if got := sessionRequest(router, method, "/api/v1/members/me", body, "", nil); got.Code != http.StatusUnauthorized {
					t.Fatalf("anonymous %s returned %d", method, got.Code)
				}
				if got := sessionRequest(router, method, "/api/v1/members/me", body, access, nil); got.Code != tt.status {
					t.Fatalf("%s returned %d, want %d", method, got.Code, tt.status)
				}
			}
		})
	}
}

func TestMemberProfileRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	seedSessionUser(t, db, []string{"member"}, true, false)
	access := invitationAdminToken(t, router)
	before := profileResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/me", "", access, nil))
	tests := []struct {
		name   string
		change func(map[string]any)
	}{
		{name: "missing phone", change: func(v map[string]any) { delete(v, "phone") }},
		{name: "blank address", change: func(v map[string]any) { v["address"] = "  " }},
		{name: "blank next of kin", change: func(v map[string]any) { v["next_of_kin_name"] = "\t" }},
		{name: "invalid phone", change: func(v map[string]any) { v["phone"] = "not-a-phone" }},
		{name: "invalid next of kin phone", change: func(v map[string]any) { v["next_of_kin_phone"] = "++2348098765432" }},
		{name: "short phone", change: func(v map[string]any) { v["phone"] = "123" }},
		{name: "long address", change: func(v map[string]any) { v["address"] = strings.Repeat("a", 2001) }},
		{name: "activation injection", change: func(v map[string]any) { v["is_active"] = true }},
		{name: "user identity injection", change: func(v map[string]any) { v["user_id"] = uuid.NewString() }},
		{name: "member identity injection", change: func(v map[string]any) { v["id"] = uuid.NewString() }},
		{name: "role injection", change: func(v map[string]any) { v["roles"] = []string{"owner"} }},
		{name: "missing title", change: func(v map[string]any) { delete(v, "title") }},
		{name: "blank lga", change: func(v map[string]any) { v["lga"] = " " }},
		{name: "missing next of kin address", change: func(v map[string]any) { delete(v, "next_of_kin_address") }},
		{name: "no beneficiaries", change: func(v map[string]any) { v["beneficiaries"] = []any{} }},
		{name: "five beneficiaries", change: func(v map[string]any) {
			b := v["beneficiaries"].([]any)[0]
			v["beneficiaries"] = []any{b, b, b, b, b}
		}},
		{name: "beneficiary without phone", change: func(v map[string]any) {
			v["beneficiaries"] = []any{map[string]any{"name": "A", "relationship": "Son", "address": "Lagos"}}
		}},
		{name: "declaration not accepted", change: func(v map[string]any) { v["declaration_accepted"] = false }},
		{name: "future workplace date", change: func(v map[string]any) {
			v["workplace"] = map[string]any{"company": "X", "address": "Y", "since": "2999-01-01"}
		}},
		{name: "negative pledge", change: func(v map[string]any) { v["monthly_contribution_pledge"] = -1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input map[string]any
			if err := json.Unmarshal([]byte(invitationBody(t, validProfileInput())), &input); err != nil {
				t.Fatal(err)
			}
			tt.change(input)
			response := sessionRequest(router, http.MethodPut, "/api/v1/members/me", invitationBody(t, input), access, nil)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid input returned %d: %s", response.Code, response.Body.String())
			}
			if after := profileResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/me", "", access, nil)); !reflect.DeepEqual(after, before) {
				t.Fatal("invalid input changed the stored profile")
			}
		})
	}
}

func TestMemberProfilePhoneConflictAndOwnership(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	seedSessionUser(t, db, []string{"member"}, true, false)
	access := invitationAdminToken(t, router)
	var otherID uuid.UUID
	if err := db.Get(&otherID, "INSERT INTO users (email) VALUES ('other@example.com') RETURNING id"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO members (user_id, slug, first_name, last_name, phone)
		VALUES ($1, 'ara000002', 'Other', 'Member', '08099999999')`, otherID); err != nil {
		t.Fatal(err)
	}
	input := validProfileInput()
	input.Phone = "08099999999"
	before := profileResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/me", "", access, nil))
	conflict := sessionRequest(router, http.MethodPut, "/api/v1/members/me", invitationBody(t, input), access, nil)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("phone conflict returned %d: %s", conflict.Code, conflict.Body.String())
	}
	if after := profileResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members/me", "", access, nil)); !reflect.DeepEqual(after, before) {
		t.Fatal("phone conflict partially changed the profile")
	}
	input.Phone = "08012345678"
	path := "/api/v1/members/me?user_id=" + otherID.String()
	saved := profileResult(t, sessionRequest(router, http.MethodPut, path, invitationBody(t, input), access, nil))
	if saved.ID != before.ID || saved.FirstName != before.FirstName {
		t.Fatal("query parameter redirected the update to another member")
	}
	var otherAddress *string
	if err := db.Get(&otherAddress, "SELECT address FROM members WHERE user_id = $1", otherID); err != nil || otherAddress != nil {
		t.Fatal("another member's profile was changed")
	}
}
