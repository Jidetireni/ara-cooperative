//go:build integration

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/pkg/paystack"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func loanResult(t *testing.T, w *httptest.ResponseRecorder, status int) dto.Loan {
	t.Helper()
	var response struct{ Data dto.Loan }
	if w.Code != status || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Data.ID == uuid.Nil {
		t.Fatalf("loan request: want %d, got %d %s", status, w.Code, w.Body.String())
	}
	return response.Data
}

func requestLoan(t *testing.T, router http.Handler, access string) dto.Loan {
	t.Helper()
	return loanResult(t, sessionRequest(router, http.MethodPost, "/api/v1/loans",
		`{"amount":1000000,"purpose":"  School fees  ","duration_months":12}`, access, nil), http.StatusCreated)
}

func loanActor(t *testing.T, db *sqlx.DB, router http.Handler, roles []string, member bool) (uuid.UUID, string) {
	t.Helper()
	id := uuid.New()
	email := id.String() + "@example.com"
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, email_confirmed_at)
		SELECT $1, $2, password_hash, NOW() FROM users WHERE email = 'session@example.com'`, id, email); err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		var roleID uuid.UUID
		if err := db.Get(&roleID, `INSERT INTO roles (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id`, role); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", id, roleID); err != nil {
			t.Fatal(err)
		}
	}
	if member {
		if _, err := db.Exec(`INSERT INTO members (user_id, slug, first_name, last_name, activated_at)
			VALUES ($1, $2, 'Other', 'Member', NOW())`, id, "ara"+id.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	w := sessionRequest(router, http.MethodPost, "/api/v1/login",
		invitationBody(t, map[string]string{"email": email, "password": "test-password"}), "", nil)
	return id, readAuthResponse(t, w).AccessToken
}

func loanTerms() dto.LoanApprovalInput {
	return dto.LoanApprovalInput{Amount: 900_000, DurationMonths: 6, InterestMethod: "FLAT",
		InterestRatePercent: "5.25", InterestPeriod: "MONTHLY", Reason: "Approved final terms"}
}

func TestLoanApplicationApprovalAndDisbursement(t *testing.T) {
	for _, tt := range []struct{ method, period, role string }{
		{method: "FLAT", period: "MONTHLY", role: "admin"},
		{method: "REDUCING_BALANCE", period: "ANNUAL", role: "owner"},
	} {
		t.Run(tt.method, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router, fake := paymentsTestRouter(t, db, testOwnerAccount)
			seedSessionUser(t, db, []string{"member"}, true, true)
			member := invitationAdminToken(t, router)
			adminID, admin := loanActor(t, db, router, []string{tt.role}, false)
			loan := requestLoan(t, router, member)
			if loan.Status != "PENDING" || loan.Purpose != "School fees" || loan.ApprovedAmount != nil || loan.DisbursedAt != nil {
				t.Fatalf("unexpected application: %+v", loan)
			}
			path := "/api/v1/loans/" + loan.ID.String()
			terms := loanTerms()
			terms.InterestMethod, terms.InterestPeriod = tt.method, tt.period
			approved := loanResult(t, sessionRequest(router, http.MethodPost, path+"/approve", invitationBody(t, terms), admin, nil), http.StatusOK)
			if approved.Status != "APPROVED" || approved.ApprovedAmount == nil || *approved.ApprovedAmount != terms.Amount ||
				approved.RequestedAmount != loan.RequestedAmount || approved.ReviewedBy == nil || *approved.ReviewedBy != adminID ||
				approved.InterestRatePercent == nil || *approved.InterestRatePercent != "5.2500" ||
				approved.EligibilitySnapshot == nil || len(approved.EligibilitySnapshot.Flags) != 0 {
				t.Fatalf("approval lost terms or review snapshot: %+v", approved)
			}
			if countRows(t, db, "SELECT COUNT(*) FROM transactions WHERE ledger = 'LOAN'") != 0 {
				t.Fatal("approval recorded money before disbursement")
			}
			seen := loanResult(t, sessionRequest(router, http.MethodGet, path, "", member, nil), http.StatusOK)
			if seen.Status != "APPROVED" || seen.InterestMethod == nil || *seen.InterestMethod != tt.method {
				t.Fatal("member cannot see approved terms")
			}
			input := dto.LoanDisbursementInput{Amount: terms.Amount, PaidAt: time.Now(), Reference: "  TRANSFER-001  "}
			var wg sync.WaitGroup
			codes := make([]int, 4)
			body := invitationBody(t, input)
			for i := range codes {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					codes[i] = sessionRequest(router, http.MethodPost, path+"/disbursement", body, admin, nil).Code
				}(i)
			}
			wg.Wait()
			var successes int
			for _, code := range codes {
				if code == http.StatusOK {
					successes++
				} else if code != http.StatusConflict {
					t.Fatalf("disbursement returned %d", code)
				}
			}
			if successes != 1 {
				t.Fatalf("recorded disbursement %d times", successes)
			}
			active := loanResult(t, sessionRequest(router, http.MethodGet, path, "", member, nil), http.StatusOK)
			if active.Status != "ACTIVE" || active.TransactionID == nil || active.DisbursementReference == nil || *active.DisbursementReference != "TRANSFER-001" {
				t.Fatalf("missing external disbursement record: %+v", active)
			}
			if countRows(t, db, `SELECT COUNT(*) FROM transactions t JOIN transaction_status s ON s.transaction_id = t.id
				WHERE t.type = 'LOAN_DISBURSEMENT' AND t.ledger = 'LOAN' AND t.amount = $1 AND s.confirmed_at IS NOT NULL`, terms.Amount) != 1 {
				t.Fatal("disbursement ledger entry missing or duplicated")
			}
			fake.mu.Lock()
			requests := len(fake.initialized) + len(fake.verified)
			fake.mu.Unlock()
			if requests != 0 {
				t.Fatal("loan disbursement called Paystack")
			}
			for _, suffix := range []string{"approve", "reject"} {
				payload := invitationBody(t, terms)
				if suffix == "reject" {
					payload = `{"reason":"Too late"}`
				}
				if w := sessionRequest(router, http.MethodPost, path+"/"+suffix, payload, admin, nil); w.Code != http.StatusConflict {
					t.Fatalf("active loan reviewed again: %d %s", w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestLoanAdvisoryChecksAndOverride(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, fake := paymentsTestRouter(t, db, testOwnerAccount)
	seedSessionUser(t, db, []string{"member"}, true, true)
	member := invitationAdminToken(t, router)
	_, admin := loanActor(t, db, router, []string{"admin"}, false)
	w := sessionRequest(router, http.MethodPut, "/api/v1/loans/settings",
		`{"minimum_savings":200000,"minimum_share_units":"3.5"}`, admin, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", w.Code, w.Body.String())
	}
	loan := requestLoan(t, router, member)
	path := "/api/v1/loans/" + loan.ID.String()
	var checks struct{ Data dto.LoanEligibility }
	w = sessionRequest(router, http.MethodGet, path+"/eligibility", "", admin, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &checks) != nil || len(checks.Data.Flags) != 2 {
		t.Fatalf("missing threshold flags: %d %s", w.Code, w.Body.String())
	}
	terms := loanTerms()
	if w := sessionRequest(router, http.MethodPost, path+"/approve", invitationBody(t, terms), admin, nil); w.Code != http.StatusConflict {
		t.Fatalf("approval without override: %d", w.Code)
	}
	// Only confirmed deposits and share units count; a pending payment is excluded.
	for _, payment := range []dto.CreatePaymentInput{{Purpose: "SAVINGS", Amount: 100_000}, {Purpose: "SHARES", Amount: 75_000}} {
		paid := startPayment(t, router, member, payment)
		fake.pay(paid.Reference, paid.Total)
		if w := signedWebhook(router, paystack.EventChargeSuccess, paid.Reference); w.Code != http.StatusOK {
			t.Fatal("contribution webhook failed")
		}
	}
	startPayment(t, router, member, dto.CreatePaymentInput{Purpose: "SAVINGS", Amount: 900_000})
	other := requestLoan(t, router, member)
	terms.OverrideReason = "  Committee approved despite low savings and shares  "
	approved := loanResult(t, sessionRequest(router, http.MethodPost, path+"/approve", invitationBody(t, terms), admin, nil), http.StatusOK)
	snapshot := approved.EligibilitySnapshot
	if snapshot == nil || snapshot.Savings != 100_000 || snapshot.ShareUnits != "1.5000" || snapshot.OtherPendingApplications != 1 ||
		len(snapshot.Flags) != 3 || approved.OverrideReason == nil || *approved.OverrideReason != strings.TrimSpace(terms.OverrideReason) {
		t.Fatalf("incorrect advisory snapshot: %+v", approved)
	}
	if w := sessionRequest(router, http.MethodPut, "/api/v1/loans/settings", `{"minimum_savings":0,"minimum_share_units":"0"}`, admin, nil); w.Code != 200 {
		t.Fatal("settings reset failed")
	}
	saved := loanResult(t, sessionRequest(router, http.MethodGet, path, "", admin, nil), http.StatusOK)
	if saved.EligibilitySnapshot.MinimumSavings != 200_000 {
		t.Fatal("settings change altered a past approval snapshot")
	}
	w = sessionRequest(router, http.MethodGet, "/api/v1/loans/"+other.ID.String()+"/eligibility", "", admin, nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &checks) != nil || checks.Data.ApprovedUndisbursedLoans != 1 {
		t.Fatal("approved but undisbursed loan was not shown as an advisory")
	}
	loanResult(t, sessionRequest(router, http.MethodPost, path+"/disbursement",
		invitationBody(t, dto.LoanDisbursementInput{Amount: terms.Amount, PaidAt: time.Now(), Reference: "ADVISORY-TRANSFER"}), admin, nil), http.StatusOK)
	w = sessionRequest(router, http.MethodGet, "/api/v1/loans/"+other.ID.String()+"/eligibility", "", admin, nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &checks) != nil || checks.Data.ActiveLoans != 1 || checks.Data.ApprovedUndisbursedLoans != 0 {
		t.Fatal("active loan history was not reflected in advisory checks")
	}
}

func TestLoanSelfApprovalAndRejection(t *testing.T) {
	for _, role := range []string{"admin", "owner"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router, _ := paymentsTestRouter(t, db, testOwnerAccount)
			seedSessionUser(t, db, []string{role, "member"}, true, true)
			self := invitationAdminToken(t, router)
			_, reviewer := loanActor(t, db, router, []string{"admin"}, false)
			loan := requestLoan(t, router, self)
			path := "/api/v1/loans/" + loan.ID.String()
			if w := sessionRequest(router, http.MethodPost, path+"/approve", invitationBody(t, loanTerms()), self, nil); w.Code != http.StatusForbidden {
				t.Fatalf("self approval: %d %s", w.Code, w.Body.String())
			}
			loanResult(t, sessionRequest(router, http.MethodPost, path+"/approve", invitationBody(t, loanTerms()), reviewer, nil), http.StatusOK)
			other := requestLoan(t, router, self)
			otherPath := "/api/v1/loans/" + other.ID.String()
			rejected := loanResult(t, sessionRequest(router, http.MethodPost, otherPath+"/reject", `{"reason":"  Documents incomplete  "}`, reviewer, nil), http.StatusOK)
			if rejected.Status != "REJECTED" || rejected.ReviewReason == nil || *rejected.ReviewReason != "Documents incomplete" || rejected.ApprovedAmount != nil {
				t.Fatalf("rejection not recorded: %+v", rejected)
			}
			if w := sessionRequest(router, http.MethodPost, otherPath+"/approve", invitationBody(t, loanTerms()), reviewer, nil); w.Code != http.StatusConflict {
				t.Fatalf("rejected loan approved: %d", w.Code)
			}
			if countRows(t, db, "SELECT COUNT(*) FROM transactions WHERE ledger = 'LOAN'") != 0 {
				t.Fatal("review created financial entries")
			}
		})
	}
}

func TestLoanAccessPrivacyAndLists(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, _ := paymentsTestRouter(t, db, testOwnerAccount)
	seedSessionUser(t, db, []string{"member"}, true, true)
	member := invitationAdminToken(t, router)
	_, admin := loanActor(t, db, router, []string{"admin"}, false)
	_, other := loanActor(t, db, router, []string{"member"}, true)
	first := requestLoan(t, router, member)
	second := requestLoan(t, router, other)
	requestLoan(t, router, other)
	path := "/api/v1/loans/" + first.ID.String()
	if w := sessionRequest(router, http.MethodGet, path, "", other, nil); w.Code != http.StatusNotFound {
		t.Fatal("another member's loan was exposed")
	}
	var list struct{ Data dto.ListResponse[dto.Loan] }
	w := sessionRequest(router, http.MethodGet, "/api/v1/loans/me?member_id="+second.MemberID.String(), "", member, nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Data.Items) != 1 || list.Data.Items[0].ID != first.ID {
		t.Fatalf("member list identity can be overridden: %d %s", w.Code, w.Body.String())
	}
	w = sessionRequest(router, http.MethodGet, "/api/v1/loans?member_id="+first.MemberID.String()+"&search=School&status=pending", "", admin, nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Data.Items) != 1 || list.Data.Items[0].Member == nil {
		t.Fatal("admin filters/populated member missing")
	}
	w = sessionRequest(router, http.MethodGet, "/api/v1/loans?limit=1", "", admin, nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Data.Items) != 1 || list.Data.NextCursor == nil {
		t.Fatal("loan paging missing")
	}
	firstID := list.Data.Items[0].ID
	w = sessionRequest(router, http.MethodGet, "/api/v1/loans?limit=1&cursor="+url.QueryEscape(*list.Data.NextCursor), "", admin, nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Data.Items) != 1 || list.Data.Items[0].ID == firstID {
		t.Fatal("loan paging repeated an item")
	}
	for _, query := range []string{"?status=PAID", "?member_id=bad", "?cursor=bad", "?sort=purpose:asc"} {
		if w := sessionRequest(router, http.MethodGet, "/api/v1/loans"+query, "", admin, nil); w.Code != 400 {
			t.Errorf("invalid filter %s: %d", query, w.Code)
		}
	}
	for _, req := range []struct{ method, path, body string }{
		{method: http.MethodGet, path: "/api/v1/loans"},
		{method: http.MethodGet, path: "/api/v1/loans/settings"},
		{method: http.MethodPut, path: "/api/v1/loans/settings", body: `{"minimum_savings":0,"minimum_share_units":"0"}`},
		{method: http.MethodGet, path: path + "/eligibility"},
		{method: http.MethodPost, path: path + "/approve", body: invitationBody(t, loanTerms())},
		{method: http.MethodPost, path: path + "/reject", body: `{"reason":"No"}`},
		{method: http.MethodPost, path: path + "/disbursement", body: invitationBody(t, dto.LoanDisbursementInput{Amount: 100_000, PaidAt: time.Now(), Reference: "x"})},
	} {
		if w := sessionRequest(router, req.method, req.path, req.body, member, nil); w.Code != http.StatusForbidden {
			t.Errorf("member access %s: %d", req.path, w.Code)
		}
		if w := sessionRequest(router, req.method, req.path, req.body, "", nil); w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous access %s: %d", req.path, w.Code)
		}
	}
	if w := sessionRequest(router, http.MethodPost, "/api/v1/loans", `{"amount":1,"purpose":"x","duration_months":1}`, admin, nil); w.Code != http.StatusForbidden {
		t.Fatal("admin without membership applied for a personal loan")
	}
	if _, err := db.Exec("UPDATE members SET activated_at = NULL WHERE id = $1", first.MemberID); err != nil {
		t.Fatal(err)
	}
	for _, req := range []struct{ method, path, body string }{
		{method: http.MethodGet, path: path}, {method: http.MethodGet, path: "/api/v1/loans/me"},
		{method: http.MethodPost, path: "/api/v1/loans", body: `{"amount":1,"purpose":"x","duration_months":1}`},
	} {
		if w := sessionRequest(router, req.method, req.path, req.body, member, nil); w.Code != http.StatusForbidden {
			t.Errorf("inactive member loan access: %d", w.Code)
		}
	}
}

func TestLoanValidationAndDuplicateTransfer(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, _ := paymentsTestRouter(t, db, testOwnerAccount)
	seedSessionUser(t, db, []string{"member"}, true, true)
	member := invitationAdminToken(t, router)
	_, admin := loanActor(t, db, router, []string{"admin"}, false)
	for _, body := range []string{`{}`, `{"amount":0,"purpose":"x","duration_months":1}`, `{"amount":1,"purpose":"  ","duration_months":1}`, `{"amount":1,"purpose":"x","duration_months":0}`} {
		if w := sessionRequest(router, http.MethodPost, "/api/v1/loans", body, member, nil); w.Code != 400 {
			t.Fatalf("invalid application accepted: %d", w.Code)
		}
	}
	for _, body := range []string{`{"minimum_savings":-1,"minimum_share_units":"1"}`, `{"minimum_savings":0,"minimum_share_units":"-1"}`, `{"minimum_savings":0,"minimum_share_units":"1.12345"}`} {
		if w := sessionRequest(router, http.MethodPut, "/api/v1/loans/settings", body, admin, nil); w.Code != 400 {
			t.Fatalf("invalid settings accepted: %d", w.Code)
		}
	}
	loan := requestLoan(t, router, member)
	path := "/api/v1/loans/" + loan.ID.String()
	for _, rate := range []string{"-1", "100.0001", "NaN", "5.12345"} {
		terms := loanTerms()
		terms.InterestRatePercent = rate
		if w := sessionRequest(router, http.MethodPost, path+"/approve", invitationBody(t, terms), admin, nil); w.Code != 400 {
			t.Fatalf("invalid rate accepted: %d", w.Code)
		}
	}
	if w := sessionRequest(router, http.MethodPost, path+"/reject", `{"reason":"   "}`, admin, nil); w.Code != 400 {
		t.Fatal("blank rejection reason accepted")
	}
	if w := sessionRequest(router, http.MethodPost, path+"/disbursement", invitationBody(t, dto.LoanDisbursementInput{Amount: 900_000, PaidAt: time.Now(), Reference: "x"}), admin, nil); w.Code != 409 {
		t.Fatal("pending loan disbursed")
	}
	approved := loanResult(t, sessionRequest(router, http.MethodPost, path+"/approve", invitationBody(t, loanTerms()), admin, nil), http.StatusOK)
	for _, input := range []dto.LoanDisbursementInput{
		{Amount: 1, PaidAt: time.Now(), Reference: "wrong amount"},
		{Amount: 900_000, PaidAt: time.Now().Add(time.Hour), Reference: "future"},
		{Amount: 900_000, PaidAt: approved.ReviewedAt.Add(-time.Hour), Reference: "before approval"},
		{Amount: 900_000, PaidAt: time.Now(), Reference: "   "},
	} {
		if w := sessionRequest(router, http.MethodPost, path+"/disbursement", invitationBody(t, input), admin, nil); w.Code != 400 {
			t.Fatalf("invalid disbursement accepted: %d %s", w.Code, w.Body.String())
		}
	}
	loanResult(t, sessionRequest(router, http.MethodPost, path+"/disbursement", invitationBody(t, dto.LoanDisbursementInput{Amount: 900_000, PaidAt: time.Now(), Reference: "BANK-REF"}), admin, nil), http.StatusOK)
	second := requestLoan(t, router, member)
	terms := loanTerms()
	terms.OverrideReason = "Approved despite active loan history"
	secondPath := "/api/v1/loans/" + second.ID.String()
	loanResult(t, sessionRequest(router, http.MethodPost, secondPath+"/approve", invitationBody(t, terms), admin, nil), http.StatusOK)
	w := sessionRequest(router, http.MethodPost, secondPath+"/disbursement", invitationBody(t, dto.LoanDisbursementInput{Amount: 900_000, PaidAt: time.Now(), Reference: "BANK-REF"}), admin, nil)
	if w.Code != 409 {
		t.Fatalf("duplicate transfer reference: %d %s", w.Code, w.Body.String())
	}
	stillApproved := loanResult(t, sessionRequest(router, http.MethodGet, secondPath, "", admin, nil), http.StatusOK)
	if stillApproved.Status != "APPROVED" || stillApproved.TransactionID != nil || countRows(t, db, "SELECT COUNT(*) FROM transactions WHERE ledger = 'LOAN'") != 1 {
		t.Fatal("duplicate transfer left a ledger entry or activated the loan")
	}
	for _, id := range []struct {
		id     string
		status int
	}{{id: "bad", status: 400}, {id: uuid.NewString(), status: 404}} {
		if w := sessionRequest(router, http.MethodGet, "/api/v1/loans/"+id.id, "", admin, nil); w.Code != id.status {
			t.Errorf("unknown loan: %d", w.Code)
		}
	}
}
