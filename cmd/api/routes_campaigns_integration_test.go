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

func seedPaymentCampaign(t *testing.T, db *sqlx.DB, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := db.Get(&id, `INSERT INTO campaigns
		(admin_id, name, description, participation, amount_type, starts_at, ends_at)
		VALUES ($1, 'Event fund', 'Cooperative event', 'VOLUNTARY', 'FLEXIBLE', NOW() - INTERVAL '1 day',
		NOW() + INTERVAL '30 days') RETURNING id`, userID); err != nil {
		t.Fatal(err)
	}
	return id
}

func campaignResult(t *testing.T, w *httptest.ResponseRecorder, status int) dto.Campaign {
	t.Helper()
	var response struct{ Data dto.Campaign }
	if w.Code != status || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Data.ID == uuid.Nil {
		t.Fatalf("campaign request: want %d, got %d %s", status, w.Code, w.Body.String())
	}
	return response.Data
}

func campaignInput() dto.CampaignInput {
	return dto.CampaignInput{
		Name: "  End-of-year event  ", Description: "  Support the cooperative event  ",
		Participation: "REQUIRED", AmountType: "FIXED", Amount: 100_000,
		StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(24 * time.Hour),
	}
}

func TestCampaignFixedAndFlexibleContributions(t *testing.T) {
	for _, amountType := range []string{"FIXED", "FLEXIBLE"} {
		t.Run(amountType, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router, fake := paymentsTestRouter(t, db, testOwnerAccount)
			seedSessionUser(t, db, []string{"admin", "member"}, true, true)
			access := invitationAdminToken(t, router)
			input := campaignInput()
			input.AmountType = amountType
			if amountType == "FLEXIBLE" {
				input.Amount, input.Participation = 0, "VOLUNTARY"
			}
			campaign := campaignResult(t, sessionRequest(router, http.MethodPost, "/api/v1/campaigns",
				invitationBody(t, input), access, nil), http.StatusCreated)
			if campaign.Name != strings.TrimSpace(input.Name) || campaign.Description != strings.TrimSpace(input.Description) ||
				campaign.Status != "OPEN" || campaign.Participation != input.Participation || campaign.TotalContributed != 0 {
				t.Fatalf("unexpected campaign: %+v", campaign)
			}
			if (campaign.Amount != nil) != (amountType == "FIXED") {
				t.Fatal("campaign amount does not match its mode")
			}
			paymentInput := dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &campaign.ID}
			quotePath := "/api/v1/payments/quote?purpose=SPECIAL_DEPOSIT&campaign_id=" + campaign.ID.String()
			if amountType == "FLEXIBLE" {
				paymentInput.Amount = 75_000
				quotePath += "&amount=75000"
			}
			var quote struct{ Data dto.PaymentQuote }
			w := sessionRequest(router, http.MethodGet, quotePath, "", access, nil)
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &quote) != nil ||
				quote.Data.CampaignID == nil || *quote.Data.CampaignID != campaign.ID {
				t.Fatalf("campaign quote: %d %s", w.Code, w.Body.String())
			}
			payment := startPayment(t, router, access, paymentInput)
			if payment.CampaignID == nil || *payment.CampaignID != campaign.ID || payment.Amount != quote.Data.Amount ||
				payment.Description != campaign.Name || fake.lastCheckout(t).Metadata.CampaignID != campaign.ID.String() {
				t.Fatal("checkout lost its campaign association or amount")
			}
			path := "/api/v1/campaigns/" + campaign.ID.String()
			if pending := campaignResult(t, sessionRequest(router, http.MethodGet, path, "", access, nil), http.StatusOK); pending.TotalContributed != 0 || pending.ContributionCount != 0 {
				t.Fatal("pending checkout counted as a contribution")
			}
			fake.pay(payment.Reference, payment.Total)
			var wg sync.WaitGroup
			codes := make([]int, 4)
			for i := range codes {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					codes[i] = signedWebhook(router, paystack.EventChargeSuccess, payment.Reference).Code
				}(i)
			}
			wg.Wait()
			for _, code := range codes {
				if code != http.StatusOK {
					t.Fatalf("concurrent webhook: %d", code)
				}
			}
			// Both fixed and flexible campaigns allow repeat contributions.
			second := startPayment(t, router, access, paymentInput)
			if second.Reference == payment.Reference {
				t.Fatal("repeat contribution reused a paid checkout")
			}
			fake.pay(second.Reference, second.Total)
			if w := signedWebhook(router, paystack.EventChargeSuccess, second.Reference); w.Code != http.StatusOK {
				t.Fatalf("repeat contribution webhook: %d", w.Code)
			}
			paid := campaignResult(t, sessionRequest(router, http.MethodGet, path, "", access, nil), http.StatusOK)
			if paid.TotalContributed != payment.Amount+second.Amount || paid.ContributionCount != 2 || paid.ContributingMembers != 1 {
				t.Fatalf("campaign totals include fees or duplicate webhook credits: %+v", paid)
			}
			var contributions struct{ Data dto.ListResponse[dto.Payment] }
			w = sessionRequest(router, http.MethodGet, path+"/contributions?limit=1", "", access, nil)
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &contributions) != nil ||
				len(contributions.Data.Items) != 1 || contributions.Data.NextCursor == nil || contributions.Data.Items[0].Member == nil {
				t.Fatalf("admin contribution history: %d %s", w.Code, w.Body.String())
			}
			firstReference := contributions.Data.Items[0].Reference
			w = sessionRequest(router, http.MethodGet, path+"/contributions?limit=1&cursor="+
				url.QueryEscape(*contributions.Data.NextCursor), "", access, nil)
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &contributions) != nil ||
				len(contributions.Data.Items) != 1 || contributions.Data.Items[0].Reference == firstReference ||
				contributions.Data.NextCursor != nil {
				t.Fatalf("contribution history pagination: %d %s", w.Code, w.Body.String())
			}
			if countRows(t, db, "SELECT COUNT(*) FROM transactions WHERE ledger = 'SAVINGS'") != 0 ||
				countRows(t, db, "SELECT COUNT(*) FROM transactions WHERE ledger = 'SPECIAL_DEPOSIT'") != 2 {
				t.Fatal("campaign contributions entered personal savings or were duplicated")
			}
		})
	}
}

func TestCampaignAccess(t *testing.T) {
	tests := []struct {
		name           string
		roles          []string
		member, active bool
		read, create   int
	}{
		{name: "owner", roles: []string{"owner"}, read: 200, create: 201},
		{name: "admin", roles: []string{"admin"}, read: 200, create: 201},
		{name: "active member", roles: []string{"member"}, member: true, active: true, read: 200, create: 403},
		{name: "pending member", roles: []string{"member"}, member: true, read: 403, create: 403},
		{name: "no role", roles: []string{}, read: 403, create: 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router, _ := paymentsTestRouter(t, db, testOwnerAccount)
			userID := seedSessionUser(t, db, tt.roles, tt.member, tt.active)
			access := invitationAdminToken(t, router)
			id := seedPaymentCampaign(t, db, userID)
			for _, path := range []string{"/api/v1/campaigns", "/api/v1/campaigns/" + id.String(),
				"/api/v1/campaigns/" + id.String() + "/contributions"} {
				if w := sessionRequest(router, http.MethodGet, path, "", access, nil); w.Code != tt.read {
					t.Errorf("%s: want %d, got %d %s", path, tt.read, w.Code, w.Body.String())
				}
				if w := sessionRequest(router, http.MethodGet, path, "", "", nil); w.Code != http.StatusUnauthorized {
					t.Errorf("anonymous %s: got %d", path, w.Code)
				}
			}
			body := invitationBody(t, campaignInput())
			if w := sessionRequest(router, http.MethodPost, "/api/v1/campaigns", body, access, nil); w.Code != tt.create {
				t.Errorf("create: want %d, got %d %s", tt.create, w.Code, w.Body.String())
			}
			if w := sessionRequest(router, http.MethodPost, "/api/v1/campaigns", body, "", nil); w.Code != http.StatusUnauthorized {
				t.Errorf("anonymous create: got %d", w.Code)
			}
			writeStatus := http.StatusForbidden
			if tt.create == http.StatusCreated {
				writeStatus = http.StatusOK
			}
			for _, req := range []struct{ method, suffix, body string }{
				{method: http.MethodPut, body: body},
				{method: http.MethodPatch, suffix: "/status", body: `{"status":"CANCELLED","reason":"Admin cancelled the event"}`},
				{method: http.MethodPut, suffix: "/resolution", body: `{"resolution":"Admin will contact contributors"}`},
				{method: http.MethodPatch, suffix: "/refund-status", body: `{"is_refunded":true}`},
			} {
				path := "/api/v1/campaigns/" + id.String() + req.suffix
				if w := sessionRequest(router, req.method, path, req.body, access, nil); w.Code != writeStatus {
					t.Errorf("campaign management %s: want %d, got %d %s", req.suffix, writeStatus, w.Code, w.Body.String())
				}
				if w := sessionRequest(router, req.method, path, req.body, "", nil); w.Code != http.StatusUnauthorized {
					t.Errorf("anonymous campaign management %s: %d", req.suffix, w.Code)
				}
			}
			if tt.member && !tt.active {
				w := sessionRequest(router, http.MethodPost, "/api/v1/payments",
					invitationBody(t, dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", Amount: 100_000, CampaignID: &id}), access, nil)
				if w.Code != http.StatusForbidden {
					t.Fatalf("pending member campaign checkout: %d", w.Code)
				}
			}
		})
	}
}

func TestCampaignValidationAndList(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, _ := paymentsTestRouter(t, db, testOwnerAccount)
	seedSessionUser(t, db, []string{"admin"}, false, false)
	access := invitationAdminToken(t, router)
	tests := []struct {
		name   string
		change func(*dto.CampaignInput)
	}{
		{name: "blank name", change: func(i *dto.CampaignInput) { i.Name = "  " }},
		{name: "blank purpose", change: func(i *dto.CampaignInput) { i.Description = " \t " }},
		{name: "name too long", change: func(i *dto.CampaignInput) { i.Name = strings.Repeat("a", 256) }},
		{name: "fixed amount too small", change: func(i *dto.CampaignInput) { i.Amount = 49_999 }},
		{name: "fixed amount missing", change: func(i *dto.CampaignInput) { i.Amount = 0 }},
		{name: "flexible amount supplied", change: func(i *dto.CampaignInput) { i.AmountType = "FLEXIBLE" }},
		{name: "invalid mode", change: func(i *dto.CampaignInput) { i.AmountType = "MONTHLY" }},
		{name: "invalid participation", change: func(i *dto.CampaignInput) { i.Participation = "SOME" }},
		{name: "missing start", change: func(i *dto.CampaignInput) { i.StartsAt = time.Time{} }},
		{name: "end before start", change: func(i *dto.CampaignInput) { i.EndsAt = i.StartsAt.Add(-time.Hour) }},
		{name: "already ended", change: func(i *dto.CampaignInput) { i.EndsAt = time.Now().Add(-time.Minute) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := campaignInput()
			tt.change(&input)
			w := sessionRequest(router, http.MethodPost, "/api/v1/campaigns", invitationBody(t, input), access, nil)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("invalid campaign: %d %s", w.Code, w.Body.String())
			}
		})
	}
	if countRows(t, db, "SELECT COUNT(*) FROM campaigns") != 0 {
		t.Fatal("invalid campaign saved")
	}
	input := campaignInput()
	open := campaignResult(t, sessionRequest(router, http.MethodPost, "/api/v1/campaigns",
		invitationBody(t, input), access, nil), http.StatusCreated)
	input.Name, input.Participation = "Next event", "VOLUNTARY"
	input.StartsAt = time.Now().Add(time.Hour)
	upcoming := campaignResult(t, sessionRequest(router, http.MethodPost, "/api/v1/campaigns",
		invitationBody(t, input), access, nil), http.StatusCreated)
	if upcoming.Status != "UPCOMING" {
		t.Fatalf("upcoming campaign status: %+v", upcoming)
	}
	var list struct {
		Data dto.ListResponse[dto.Campaign]
	}
	w := sessionRequest(router, http.MethodGet, "/api/v1/campaigns?limit=1", "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil ||
		len(list.Data.Items) != 1 || list.Data.Items[0].ID != upcoming.ID || list.Data.NextCursor == nil {
		t.Fatalf("newest-first campaign list: %d %s", w.Code, w.Body.String())
	}
	w = sessionRequest(router, http.MethodGet, "/api/v1/campaigns?limit=1&cursor="+
		url.QueryEscape(*list.Data.NextCursor), "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil ||
		len(list.Data.Items) != 1 || list.Data.Items[0].ID != open.ID || list.Data.NextCursor != nil {
		t.Fatalf("campaign next page: %d %s", w.Code, w.Body.String())
	}
	w = sessionRequest(router, http.MethodGet, "/api/v1/campaigns?status=open&participation=required", "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil ||
		len(list.Data.Items) != 1 || list.Data.Items[0].ID != open.ID {
		t.Fatalf("campaign filters: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"?status=BAD", "?participation=SOME", "?cursor=bad", "?sort=name:asc"} {
		if w := sessionRequest(router, http.MethodGet, "/api/v1/campaigns"+path, "", access, nil); w.Code != http.StatusBadRequest {
			t.Errorf("invalid campaign filter %s: %d", path, w.Code)
		}
	}
	for _, suffix := range []string{"", "/contributions"} {
		for _, target := range []struct {
			id     string
			status int
		}{
			{id: "bad", status: http.StatusBadRequest}, {id: uuid.NewString(), status: http.StatusNotFound},
		} {
			w := sessionRequest(router, http.MethodGet, "/api/v1/campaigns/"+target.id+suffix, "", access, nil)
			if w.Code != target.status {
				t.Errorf("unknown campaign %s: want %d, got %d", suffix, target.status, w.Code)
			}
		}
	}
}

func TestCampaignCheckoutValidationAndClosing(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, fake := paymentsTestRouter(t, db, testOwnerAccount)
	userID := seedSessionUser(t, db, []string{"admin", "member"}, true, true)
	access := invitationAdminToken(t, router)
	flexID := seedPaymentCampaign(t, db, userID)
	input := campaignInput()
	fixed := campaignResult(t, sessionRequest(router, http.MethodPost, "/api/v1/campaigns",
		invitationBody(t, input), access, nil), http.StatusCreated)
	input.StartsAt = time.Now().Add(time.Hour)
	upcoming := campaignResult(t, sessionRequest(router, http.MethodPost, "/api/v1/campaigns",
		invitationBody(t, input), access, nil), http.StatusCreated)
	missing, zero := uuid.New(), uuid.Nil
	tests := []struct {
		name   string
		input  dto.CreatePaymentInput
		status int
	}{
		{name: "missing campaign", input: dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", Amount: 100_000}, status: 400},
		{name: "zero campaign", input: dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &zero}, status: 400},
		{name: "unknown campaign", input: dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &missing}, status: 404},
		{name: "another purpose", input: dto.CreatePaymentInput{Purpose: "SAVINGS", Amount: 100_000, CampaignID: &flexID}, status: 400},
		{name: "wrong fixed amount", input: dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", Amount: 50_000, CampaignID: &fixed.ID}, status: 400},
		{name: "missing flexible amount", input: dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &flexID}, status: 400},
		{name: "below minimum", input: dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", Amount: 49_999, CampaignID: &flexID}, status: 400},
		{name: "amount overflow", input: dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", Amount: 9_223_372_036_854_775_807, CampaignID: &flexID}, status: 400},
		{name: "not started", input: dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &upcoming.ID}, status: 409},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := sessionRequest(router, http.MethodPost, "/api/v1/payments", invitationBody(t, tt.input), access, nil)
			if w.Code != tt.status {
				t.Fatalf("campaign checkout: want %d, got %d %s", tt.status, w.Code, w.Body.String())
			}
		})
	}
	if countRows(t, db, "SELECT COUNT(*) FROM payments") != 0 {
		t.Fatal("invalid campaign checkout created a payment")
	}
	for _, path := range []string{
		"/api/v1/payments/quote?purpose=SPECIAL_DEPOSIT&campaign_id=bad",
		"/api/v1/payments?campaign_id=bad",
	} {
		if w := sessionRequest(router, http.MethodGet, path, "", access, nil); w.Code != http.StatusBadRequest {
			t.Fatalf("malformed campaign id: %d", w.Code)
		}
	}
	payment := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &fixed.ID})
	if _, err := db.Exec("UPDATE campaigns SET ends_at = NOW() - INTERVAL '1 minute' WHERE id = $1", fixed.ID); err != nil {
		t.Fatal(err)
	}
	w := sessionRequest(router, http.MethodPost, "/api/v1/payments",
		invitationBody(t, dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &fixed.ID}), access, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("closed campaign checkout: %d %s", w.Code, w.Body.String())
	}
	// A checkout started while open retains its agreed amount and can settle later.
	fake.pay(payment.Reference, payment.Total)
	if w := signedWebhook(router, paystack.EventChargeSuccess, payment.Reference); w.Code != http.StatusOK {
		t.Fatalf("late campaign webhook: %d", w.Code)
	}
	closed := campaignResult(t, sessionRequest(router, http.MethodGet, "/api/v1/campaigns/"+fixed.ID.String(),
		"", access, nil), http.StatusOK)
	if closed.Status != "CLOSED" || closed.TotalContributed != payment.Amount {
		t.Fatalf("closed campaign lost verified funds: %+v", closed)
	}
	var list struct{ Data dto.ListResponse[dto.Payment] }
	w = sessionRequest(router, http.MethodGet, "/api/v1/payments?campaign_id="+fixed.ID.String(), "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Data.Items) != 1 ||
		list.Data.Items[0].Reference != payment.Reference {
		t.Fatalf("admin campaign checkout filter: %d %s", w.Code, w.Body.String())
	}
}

func TestCampaignMemberContributionPrivacy(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, fake := paymentsTestRouter(t, db, testOwnerAccount)
	userID := seedSessionUser(t, db, []string{"admin", "member"}, true, true)
	access := invitationAdminToken(t, router)
	campaignID := seedPaymentCampaign(t, db, userID)
	seedListedMembers(t, db, []listedMember{
		{slug: "ara000002", first: "Other", last: "Member", email: "other@example.com", confirmed: true, active: true},
	})
	if _, err := db.Exec(`UPDATE users SET password_hash = (SELECT password_hash FROM users WHERE id = $1)
		WHERE email = 'other@example.com'`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_roles (user_id, role_id)
		SELECT u.id, r.id FROM users u CROSS JOIN roles r WHERE u.email = 'other@example.com' AND r.name = 'member'`); err != nil {
		t.Fatal(err)
	}
	other := readAuthResponse(t, sessionRequest(router, http.MethodPost, "/api/v1/login",
		`{"email":"other@example.com","password":"test-password"}`, "", nil)).AccessToken
	first := startPayment(t, router, access, dto.CreatePaymentInput{
		Purpose: "SPECIAL_DEPOSIT", Amount: 100_000, CampaignID: &campaignID,
	})
	second := startPayment(t, router, other, dto.CreatePaymentInput{
		Purpose: "SPECIAL_DEPOSIT", Amount: 75_000, CampaignID: &campaignID,
	})
	for _, payment := range []dto.Payment{first, second} {
		fake.pay(payment.Reference, payment.Total)
		if w := signedWebhook(router, paystack.EventChargeSuccess, payment.Reference); w.Code != http.StatusOK {
			t.Fatalf("contribution webhook: %d", w.Code)
		}
	}
	path := "/api/v1/campaigns/" + campaignID.String()
	member := campaignResult(t, sessionRequest(router, http.MethodGet, path, "", other, nil), http.StatusOK)
	if member.MyContribution == nil || *member.MyContribution != second.Amount ||
		member.TotalContributed != first.Amount+second.Amount || member.ContributingMembers != 2 {
		t.Fatalf("personal contribution not separated from campaign funds: %+v", member)
	}
	var list struct{ Data dto.ListResponse[dto.Payment] }
	w := sessionRequest(router, http.MethodGet, path+"/contributions?member_id="+userID.String(), "", other, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Data.Items) != 1 ||
		list.Data.Items[0].Reference != second.Reference || list.Data.Items[0].Member != nil {
		t.Fatalf("member history exposed another member: %d %s", w.Code, w.Body.String())
	}
	w = sessionRequest(router, http.MethodGet, path+"/contributions", "", access, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Data.Items) != 2 {
		t.Fatalf("admin history lost member contributions: %d %s", w.Code, w.Body.String())
	}
	if w := sessionRequest(router, http.MethodGet, "/api/v1/payments/"+first.Reference, "", other, nil); w.Code != http.StatusNotFound {
		t.Fatalf("another member's campaign checkout exposed: %d", w.Code)
	}
	campaignResult(t, sessionRequest(router, http.MethodPatch, path+"/status",
		`{"status":"CANCELLED","reason":"Event will not hold"}`, access, nil), http.StatusOK)
	campaignResult(t, sessionRequest(router, http.MethodPatch, path+"/refund-status",
		`{"is_refunded":true}`, access, nil), http.StatusOK)
	visible := campaignResult(t, sessionRequest(router, http.MethodGet, path, "", other, nil), http.StatusOK)
	if visible.Status != "CANCELLED" || !visible.IsRefunded || visible.MyContribution == nil ||
		*visible.MyContribution != second.Amount || visible.TotalContributed != first.Amount+second.Amount {
		t.Fatal("member cannot see campaign status/refund flag or the flag changed contribution amounts")
	}
}
