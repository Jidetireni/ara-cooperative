//go:build integration

package main

import (
	"net/http"
	"net/url"
	"testing"
)

// List endpoints accept only created_at:asc|desc; anything else is a 400 that
// never reaches SQL, and the tables survive injection attempts.
func TestListSortAndCursorValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		roles []string
		path  string
	}{
		{name: "admin fines", roles: []string{"admin"}, path: "/api/v1/fines"},
		{name: "admin pending transactions", roles: []string{"admin"}, path: "/api/v1/transactions/pending"},
		{name: "member fines", roles: []string{"member"}, path: "/api/v1/fines/me"},
	}
	cases := []struct {
		query  string
		status int
	}{
		{"", http.StatusOK},
		{"?sort=created_at:asc", http.StatusOK},
		{"?sort=created_at:desc", http.StatusOK},
		{"?sort=" + url.QueryEscape("created_at;drop table fines--:asc"), http.StatusBadRequest},
		{"?sort=" + url.QueryEscape("(select 1):asc"), http.StatusBadRequest},
		{"?sort=amount:asc", http.StatusBadRequest},
		{"?sort=created_at:sideways", http.StatusBadRequest},
		{"?cursor=not-a-cursor", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router := sessionTestRouter(t, db)
			seedSessionUser(t, db, tt.roles, true, true)
			access := invitationAdminToken(t, router)
			for _, c := range cases {
				got := sessionRequest(router, http.MethodGet, tt.path+c.query, "", access, nil)
				if got.Code != c.status {
					t.Fatalf("%s%s returned %d, want %d: %s", tt.path, c.query, got.Code, c.status, got.Body.String())
				}
			}
			var tables int
			if err := db.Get(&tables, `SELECT COUNT(*) FROM information_schema.tables
				WHERE table_schema = current_schema() AND table_name IN ('fines', 'transactions', 'members')`); err != nil || tables != 3 {
				t.Fatalf("tables changed after sort attempts: %d %v", tables, err)
			}
		})
	}
}
