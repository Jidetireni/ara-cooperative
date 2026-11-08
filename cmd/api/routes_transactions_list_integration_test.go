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

type ledgerEntry struct {
	member, reference, ledger, description string
	amount                                 int64
	status                                 string // PENDING, CONFIRMED or REJECTED
}

// seedLedger inserts transactions oldest first with a status row each, plus a
// share row for SHARES entries. It returns member IDs by slug.
func seedLedger(t *testing.T, db *sqlx.DB, entries []ledgerEntry) map[string]uuid.UUID {
	t.Helper()
	seedListedMembers(t, db, []listedMember{
		{slug: "ara000101", first: "Ada", last: "Okafor", email: "ada@example.com", confirmed: true, active: true},
		{slug: "ara000102", first: "Bola", last: "Ade", email: "bola@example.com", confirmed: true, active: true},
	})
	members := map[string]uuid.UUID{}
	rows, err := db.Query("SELECT slug, id FROM members WHERE slug LIKE 'ara0001%'")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var slug string
		var id uuid.UUID
		if err := rows.Scan(&slug, &id); err != nil {
			t.Fatal(err)
		}
		members[slug] = id
	}
	rows.Close()

	base := time.Now().Add(-time.Hour)
	for i, e := range entries {
		created := base.Add(time.Duration(i) * time.Minute)
		var id uuid.UUID
		if err := db.Get(&id, `INSERT INTO transactions (member_id, description, reference, amount, type, ledger, created_at)
			VALUES ($1, $2, $3, $4, 'DEPOSIT', $5, $6) RETURNING id`,
			members[e.member], e.description, e.reference, e.amount, e.ledger, created); err != nil {
			t.Fatal(err)
		}
		var confirmed, rejected *time.Time
		switch e.status {
		case "CONFIRMED":
			confirmed = &created
		case "REJECTED":
			rejected = &created
		}
		if _, err := db.Exec(`INSERT INTO transaction_status (transaction_id, confirmed_at, rejected_at) VALUES ($1, $2, $3)`,
			id, confirmed, rejected); err != nil {
			t.Fatal(err)
		}
		if e.ledger == "SHARES" {
			if _, err := db.Exec(`INSERT INTO shares (transaction_id, units, unit_price, created_at) VALUES ($1, $2, 50000, $3)`,
				id, float64(e.amount)/50000, created); err != nil {
				t.Fatal(err)
			}
		}
	}
	return members
}

func transactionList(t *testing.T, w *httptest.ResponseRecorder) dto.ListResponse[dto.Transactions] {
	t.Helper()
	var response struct {
		Data dto.ListResponse[dto.Transactions]
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("transaction list failed: %d %s", w.Code, w.Body.String())
	}
	return response.Data
}

func references(items []dto.Transactions) string {
	refs := make([]string, 0, len(items))
	for _, item := range items {
		refs = append(refs, item.Reference)
	}
	return strings.Join(refs, ",")
}

var testLedger = []ledgerEntry{
	{"ara000101", "ref-fee-ada", "REGISTRATION_FEE", "Membership fee", 100000, "CONFIRMED"},
	{"ara000101", "ref-sav-ada", "SAVINGS", "Savings deposit", 500000, "CONFIRMED"},
	{"ara000102", "ref-sav-bola", "SAVINGS", "Savings deposit", 200000, "PENDING"},
	{"ara000102", "ref-camp-bola", "SPECIAL_DEPOSIT", "Borehole project", 300000, "CONFIRMED"},
	{"ara000101", "ref-shr-ada", "SHARES", "Purchase of 3 shares", 150000, "CONFIRMED"},
	{"ara000102", "ref-sav-rejected", "SAVINGS", "Savings deposit", 900000, "REJECTED"},
}

func TestAdminListsTransactions(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	seedSessionUser(t, db, []string{"admin"}, false, false)
	access := invitationAdminToken(t, router)
	members := seedLedger(t, db, testLedger)
	get := func(query string) dto.ListResponse[dto.Transactions] {
		return transactionList(t, sessionRequest(router, http.MethodGet, "/api/v1/transactions"+query, "", access, nil))
	}

	all := get("")
	if got := references(all.Items); got != "ref-sav-rejected,ref-shr-ada,ref-camp-bola,ref-sav-bola,ref-sav-ada,ref-fee-ada" {
		t.Fatalf("newest-first order = %s", got)
	}
	first := all.Items[0]
	if first.Member.Slug != "ara000102" || first.Member.FirstName != "Bola" || first.Status.Status != dto.TransactionStatusTypeRejected ||
		first.CreatedAt.IsZero() || first.Amount != 900000 {
		t.Fatalf("transaction details missing: %+v", first)
	}

	cases := map[string]string{
		"?ledger_type=SAVINGS":                                 "ref-sav-rejected,ref-sav-bola,ref-sav-ada",
		"?status=PENDING":                                      "ref-sav-bola",
		"?status=CONFIRMED":                                    "ref-shr-ada,ref-camp-bola,ref-sav-ada,ref-fee-ada",
		"?status=REJECTED":                                     "ref-sav-rejected",
		"?member_id=" + members["ara000101"].String():          "ref-shr-ada,ref-sav-ada,ref-fee-ada",
		"?search=bola":                                         "ref-sav-rejected,ref-camp-bola,ref-sav-bola",
		"?search=" + url.QueryEscape("ada okafor"):             "ref-shr-ada,ref-sav-ada,ref-fee-ada",
		"?search=borehole":                                     "ref-camp-bola",
		"?search=ref-fee":                                      "ref-fee-ada",
		"?search=ara000102&ledger_type=SAVINGS&status=PENDING": "ref-sav-bola",
		"?search=" + url.QueryEscape("%"):                      "",
		"?sort=created_at:asc":                                 "ref-sav-rejected,ref-shr-ada,ref-camp-bola,ref-sav-bola,ref-sav-ada,ref-fee-ada",
	}
	for query, want := range cases {
		if got := references(get(query).Items); got != want {
			t.Errorf("%s = %q, want %q", query, got, want)
		}
	}

	page := get("?limit=4")
	if len(page.Items) != 4 || page.NextCursor == nil {
		t.Fatalf("first page: %d items, cursor %v", len(page.Items), page.NextCursor)
	}
	rest := get("?limit=4&cursor=" + url.QueryEscape(*page.NextCursor))
	if got := references(rest.Items); got != "ref-sav-ada,ref-fee-ada" || rest.NextCursor != nil {
		t.Fatalf("second page = %s", got)
	}

	var totals struct{ Data dto.LedgerTotals }
	w := sessionRequest(router, http.MethodGet, "/api/v1/transactions/totals", "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &totals) != nil {
		t.Fatalf("totals: %d %s", w.Code, w.Body.String())
	}
	want := dto.LedgerTotals{MembershipFees: 100000, Savings: 500000, CampaignContributions: 300000, Shares: 150000}
	if totals.Data != want {
		t.Fatalf("totals = %+v, want confirmed only %+v", totals.Data, want)
	}

	var shares struct{ Data dto.ListResponse[dto.Shares] }
	w = sessionRequest(router, http.MethodGet, "/api/v1/shares?member_id="+members["ara000101"].String(), "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &shares) != nil {
		t.Fatalf("share purchases: %d %s", w.Code, w.Body.String())
	}
	if len(shares.Data.Items) != 1 || shares.Data.Items[0].Units != 3 || shares.Data.Items[0].Transaction.Member.Slug != "ara000101" {
		t.Fatalf("share purchases = %+v", shares.Data.Items)
	}
	w = sessionRequest(router, http.MethodGet, "/api/v1/shares?member_id="+members["ara000102"].String(), "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &shares) != nil || len(shares.Data.Items) != 0 {
		t.Fatalf("member filter on share purchases: %d %s", w.Code, w.Body.String())
	}
}

func TestTransactionListAccessAndValidation(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	seedSessionUser(t, db, []string{"member"}, true, true)
	member := invitationAdminToken(t, router)
	for _, path := range []string{"/api/v1/transactions", "/api/v1/transactions/totals", "/api/v1/shares"} {
		if w := sessionRequest(router, http.MethodGet, path, "", member, nil); w.Code != http.StatusForbidden {
			t.Errorf("member %s: want 403, got %d", path, w.Code)
		}
		if w := sessionRequest(router, http.MethodGet, path, "", "", nil); w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s: want 401, got %d", path, w.Code)
		}
	}

	if _, err := db.Exec(`INSERT INTO roles (name) VALUES ('admin') ON CONFLICT (name) DO NOTHING;
		INSERT INTO user_roles (user_id, role_id) SELECT u.id, r.id FROM users u, roles r WHERE r.name = 'admin'`); err != nil {
		t.Fatal(err)
	}
	admin := invitationAdminToken(t, router)
	for _, query := range []string{
		"?ledger_type=SAVING", "?type=GIFT", "?status=DONE", "?member_id=nope",
		"?search=" + strings.Repeat("a", 101), "?cursor=bad",
	} {
		if w := sessionRequest(router, http.MethodGet, "/api/v1/transactions"+query, "", admin, nil); w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d %s", query, w.Code, w.Body.String())
		}
	}
	if w := sessionRequest(router, http.MethodGet, "/api/v1/shares?member_id=nope", "", admin, nil); w.Code != http.StatusBadRequest {
		t.Errorf("shares bad member_id: want 400, got %d", w.Code)
	}
}

func TestAdminFineListShowsMember(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	adminID := seedSessionUser(t, db, []string{"admin"}, false, false)
	access := invitationAdminToken(t, router)
	members := seedLedger(t, db, nil)
	if _, err := db.Exec(`INSERT INTO fines (admin_id, member_id, amount, reason, deadline)
		VALUES ($1, $2, 250000, 'Late to meeting', NOW() + INTERVAL '3 days')`, adminID, members["ara000102"]); err != nil {
		t.Fatal(err)
	}
	var response struct{ Data dto.ListResponse[dto.Fine] }
	w := sessionRequest(router, http.MethodGet, "/api/v1/fines?paid=false", "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil || len(response.Data.Items) != 1 {
		t.Fatalf("fine list: %d %s", w.Code, w.Body.String())
	}
	fine := response.Data.Items[0]
	if fine.Member == nil || fine.Member.Slug != "ara000102" || fine.Member.FirstName != "Bola" || fine.CreatedAt.IsZero() {
		t.Fatalf("fine is missing its member: %+v", fine)
	}
}

func TestMemberListsOwnTransactions(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	userID := seedSessionUser(t, db, []string{"member"}, true, true)
	access := invitationAdminToken(t, router)
	seedLedger(t, db, []ledgerEntry{
		{member: "ara000101", description: "Savings deposit", reference: "other-1", amount: 500000, ledger: "SAVINGS", status: "CONFIRMED"},
	})
	for i, ledger := range []string{"SAVINGS", "SHARES"} {
		var id uuid.UUID
		if err := db.Get(&id, `INSERT INTO transactions (member_id, description, reference, amount, type, ledger)
			SELECT id, 'Mine', $2, 100000, 'DEPOSIT', $3 FROM members WHERE user_id = $1 RETURNING id`,
			userID, fmt.Sprintf("mine-%d", i), ledger); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO transaction_status (transaction_id, confirmed_at) VALUES ($1, NOW())`, id); err != nil {
			t.Fatal(err)
		}
	}

	list := func(query string) dto.ListResponse[dto.Transactions] {
		t.Helper()
		var response struct {
			Data dto.ListResponse[dto.Transactions]
		}
		w := sessionRequest(router, http.MethodGet, "/api/v1/transactions/me"+query, "", access, nil)
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil {
			t.Fatalf("%s: %d %s", query, w.Code, w.Body.String())
		}
		return response.Data
	}

	// Another member's ID and a search are ignored: members only ever see their own entries.
	all := list("?member_id=" + uuid.NewString() + "&search=other")
	if len(all.Items) != 2 {
		t.Fatalf("want own 2 transactions, got %d", len(all.Items))
	}
	for _, item := range all.Items {
		if item.Reference == "other-1" {
			t.Fatal("member saw another member's transaction")
		}
	}
	if savings := list("?ledger_type=SAVINGS"); len(savings.Items) != 1 || savings.Items[0].Reference != "mine-0" {
		t.Fatalf("ledger filter: %+v", savings.Items)
	}
	if w := sessionRequest(router, http.MethodGet, "/api/v1/transactions/me?status=DONE", "", access, nil); w.Code != http.StatusBadRequest {
		t.Errorf("bad status: want 400, got %d", w.Code)
	}
	if w := sessionRequest(router, http.MethodGet, "/api/v1/transactions/me", "", "", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: want 401, got %d", w.Code)
	}
}
