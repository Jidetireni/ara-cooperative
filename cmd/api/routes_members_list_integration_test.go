//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type listedMember struct {
	slug, first, last, email string
	confirmed, active        bool
	deleted                  bool
}

// seedListedMembers inserts accounts and memberships with increasing creation times,
// so the newest-first order is predictable.
func seedListedMembers(t *testing.T, db *sqlx.DB, members []listedMember) {
	t.Helper()
	base := time.Now().Add(-time.Hour)
	for i, m := range members {
		var confirmedAt, activatedAt, deletedAt *time.Time
		created := base.Add(time.Duration(i) * time.Minute)
		if m.confirmed {
			confirmedAt = &created
		}
		if m.active {
			activatedAt = &created
		}
		if m.deleted {
			deletedAt = &created
		}
		var userID uuid.UUID
		if err := db.Get(&userID, `INSERT INTO users (email, email_confirmed_at) VALUES ($1, $2) RETURNING id`,
			m.email, confirmedAt); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO members (user_id, slug, first_name, last_name, activated_at, created_at, deleted_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, userID, m.slug, m.first, m.last, activatedAt, created, deletedAt); err != nil {
			t.Fatal(err)
		}
	}
}

func memberListResult(t *testing.T, w *httptest.ResponseRecorder) dto.ListResponse[dto.MemberSummary] {
	t.Helper()
	var response struct {
		Data dto.ListResponse[dto.MemberSummary]
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("member list failed: %d %s", w.Code, w.Body.String())
	}
	return response.Data
}

func listedSlugs(items []dto.MemberSummary) string {
	slugs := make([]string, 0, len(items))
	for _, item := range items {
		slugs = append(slugs, item.Slug)
	}
	return strings.Join(slugs, ",")
}

func TestAdminListsMembers(t *testing.T) {
	for _, role := range []string{"owner", "admin"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router := sessionTestRouter(t, db)
			seedSessionUser(t, db, []string{role}, false, false)
			seedListedMembers(t, db, []listedMember{
				{slug: "ara000101", first: "Ada", last: "Okafor", email: "ada@example.com", confirmed: true, active: true},
				{slug: "ara000102", first: "Bola", last: "Ade", email: "bola@example.com", confirmed: true},
				{slug: "ara000103", first: "Chidi", last: "Eze", email: "chidi+1@example.com"},
				{slug: "ara000104", first: "Gone", last: "Member", email: "gone@example.com", confirmed: true, deleted: true},
				{slug: "ara000105", first: "Percent", last: "100%", email: "pct@example.com"},
			})
			access := invitationAdminToken(t, router)

			all := memberListResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members", "", access, nil))
			if got := listedSlugs(all.Items); got != "ara000105,ara000103,ara000102,ara000101" {
				t.Fatalf("list order or soft-delete filtering wrong: %s", got)
			}
			if all.NextCursor != nil {
				t.Fatal("single page returned a next cursor")
			}
			byslug := map[string]dto.MemberSummary{}
			for _, item := range all.Items {
				byslug[item.Slug] = item
			}
			if a := byslug["ara000101"]; a.Email != "ada@example.com" || !a.EmailConfirmed || !a.IsActive || a.CreatedAt.IsZero() {
				t.Fatalf("active member summary wrong: %+v", a)
			}
			if c := byslug["ara000103"]; c.Email != "chidi+1@example.com" || c.EmailConfirmed || c.IsActive {
				t.Fatalf("invited member summary wrong: %+v", c)
			}
			listBody := sessionRequest(router, http.MethodGet, "/api/v1/members", "", access, nil).Body.String()
			if strings.Contains(listBody, "password") {
				t.Fatal("member list exposed password data")
			}

			filters := []struct {
				query string
				want  string
			}{
				{"is_active=true", "ara000101"},
				{"is_active=false", "ara000105,ara000103,ara000102"},
				{"search=" + url.QueryEscape("CHIDI+1@"), "ara000103"},
				{"search=" + url.QueryEscape("bola ade"), "ara000102"},
				{"search=000101", "ara000101"},
				{"search=" + url.QueryEscape("100%"), "ara000105"},
				{"search=" + url.QueryEscape("%"), "ara000105"},
				{"search=" + url.QueryEscape("_"), ""},
				{"search=gone", ""},
				{"search=" + url.QueryEscape("   ") + "&is_active=false", "ara000105,ara000103,ara000102"},
				{"search=ade&is_active=false", "ara000102"},
			}
			for _, f := range filters {
				got := memberListResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members?"+f.query, "", access, nil))
				if listedSlugs(got.Items) != f.want {
					t.Fatalf("%s returned %q, want %q", f.query, listedSlugs(got.Items), f.want)
				}
			}

			first := memberListResult(t, sessionRequest(router, http.MethodGet, "/api/v1/members?limit=3", "", access, nil))
			if listedSlugs(first.Items) != "ara000105,ara000103,ara000102" || first.NextCursor == nil {
				t.Fatalf("first page wrong: %s", listedSlugs(first.Items))
			}
			second := memberListResult(t, sessionRequest(router, http.MethodGet,
				"/api/v1/members?limit=3&cursor="+url.QueryEscape(*first.NextCursor), "", access, nil))
			if listedSlugs(second.Items) != "ara000101" || second.NextCursor != nil {
				t.Fatalf("second page wrong: %s", listedSlugs(second.Items))
			}
		})
	}
}

func TestMemberListAccessAndValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		roles  []string
		query  string
		status int
	}{
		{name: "anonymous", status: http.StatusUnauthorized},
		{name: "member", roles: []string{"member"}, status: http.StatusForbidden},
		{name: "invalid active filter", roles: []string{"admin"}, query: "?is_active=maybe", status: http.StatusBadRequest},
		{name: "search too long", roles: []string{"admin"}, query: "?search=" + strings.Repeat("a", 101), status: http.StatusBadRequest},
		{name: "invalid cursor", roles: []string{"admin"}, query: "?cursor=not-a-cursor", status: http.StatusBadRequest},
		{name: "client sort ignored", roles: []string{"admin"}, query: "?sort=" + url.QueryEscape("id;drop table members:asc"), status: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router := sessionTestRouter(t, db)
			access := ""
			if tt.roles != nil {
				seedSessionUser(t, db, tt.roles, true, true)
				access = invitationAdminToken(t, router)
			}
			got := sessionRequest(router, http.MethodGet, "/api/v1/members"+tt.query, "", access, nil)
			if got.Code != tt.status {
				t.Fatal(fmt.Sprintf("%s returned %d, want %d: %s", tt.query, got.Code, tt.status, got.Body.String()))
			}
		})
	}
}
