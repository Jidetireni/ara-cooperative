//go:build integration

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/pkg/paystack"
	"github.com/google/uuid"
)

func TestCampaignEditingPreservesCheckouts(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, fake := paymentsTestRouter(t, db, testOwnerAccount)
	userID := seedSessionUser(t, db, []string{"admin", "member"}, true, true)
	access := invitationAdminToken(t, router)
	input := campaignInput()
	campaign := campaignResult(t, sessionRequest(router, http.MethodPost, "/api/v1/campaigns",
		invitationBody(t, input), access, nil), http.StatusCreated)
	path := "/api/v1/campaigns/" + campaign.ID.String()
	old := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &campaign.ID})
	input.Name, input.Description, input.Amount = "  Updated event  ", "Updated purpose", 150_000
	input.Participation = "VOLUNTARY"
	updated := campaignResult(t, sessionRequest(router, http.MethodPut, path,
		invitationBody(t, input), access, nil), http.StatusOK)
	if updated.Name != strings.TrimSpace(input.Name) || updated.Amount == nil || *updated.Amount != input.Amount ||
		updated.Participation != input.Participation || updated.UpdatedAt == nil || updated.UpdatedBy == nil ||
		*updated.UpdatedBy != userID || updated.ID != campaign.ID {
		t.Fatalf("campaign update missing fields or author: %+v", updated)
	}
	stored := paymentResult(t, sessionRequest(router, http.MethodGet, "/api/v1/payments/"+old.Reference,
		"", access, nil), http.StatusOK)
	if stored.Amount != old.Amount || stored.Total != old.Total || stored.Description != old.Description {
		t.Fatal("editing campaign changed an existing checkout")
	}
	wrong := sessionRequest(router, http.MethodPost, "/api/v1/payments",
		invitationBody(t, dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &campaign.ID, Amount: old.Amount}), access, nil)
	if wrong.Code != http.StatusBadRequest {
		t.Fatalf("outdated fixed amount accepted: %d %s", wrong.Code, wrong.Body.String())
	}
	fresh := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &campaign.ID})
	if fresh.Amount != input.Amount || fresh.Description != updated.Name {
		t.Fatal("new checkout did not use updated campaign terms")
	}
	fake.pay(old.Reference, old.Total)
	if w := signedWebhook(router, paystack.EventChargeSuccess, old.Reference); w.Code != http.StatusOK {
		t.Fatalf("old quote webhook: %d %s", w.Code, w.Body.String())
	}
	funded := campaignResult(t, sessionRequest(router, http.MethodGet, path, "", access, nil), http.StatusOK)
	if funded.TotalContributed != old.Amount || funded.ContributionCount != 1 {
		t.Fatal("campaign edit repriced a verified old checkout")
	}
	input.AmountType, input.Amount = "FLEXIBLE", 0
	campaignResult(t, sessionRequest(router, http.MethodPut, path, invitationBody(t, input), access, nil), http.StatusOK)
	flexible := startPayment(t, router, access, dto.CreatePaymentInput{
		Purpose: "SPECIAL_DEPOSIT", CampaignID: &campaign.ID, Amount: 75_000, Description: "My contribution",
	})
	if flexible.Amount != 75_000 || flexible.Description != "My contribution" {
		t.Fatal("mode change or custom description was lost while rechecking the campaign")
	}
}

func TestCampaignClosingCancellationAndRefundFlag(t *testing.T) {
	for _, status := range []string{"CLOSED", "CANCELLED"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router, fake := paymentsTestRouter(t, db, testOwnerAccount)
			userID := seedSessionUser(t, db, []string{"admin", "member"}, true, true)
			access := invitationAdminToken(t, router)
			input := campaignInput()
			campaign := campaignResult(t, sessionRequest(router, http.MethodPost, "/api/v1/campaigns",
				invitationBody(t, input), access, nil), http.StatusCreated)
			path := "/api/v1/campaigns/" + campaign.ID.String()
			payment := startPayment(t, router, access, dto.CreatePaymentInput{Purpose: "SPECIAL_DEPOSIT", CampaignID: &campaign.ID})
			changed := campaignResult(t, sessionRequest(router, http.MethodPatch, path+"/status",
				invitationBody(t, dto.CampaignStatusInput{Status: status, Reason: "  Admin decision  "}), access, nil), http.StatusOK)
			if changed.Status != status || changed.StatusReason == nil || *changed.StatusReason != "Admin decision" ||
				changed.StatusChangedAt == nil || changed.StatusChangedBy == nil || *changed.StatusChangedBy != userID || changed.IsRefunded {
				t.Fatalf("campaign status lacks audit details: %+v", changed)
			}
			for _, req := range []struct{ method, path, body string }{
				{method: http.MethodPost, path: "/api/v1/payments", body: invitationBody(t, dto.CreatePaymentInput{
					Purpose: "SPECIAL_DEPOSIT", CampaignID: &campaign.ID,
				})},
				{method: http.MethodGet, path: "/api/v1/payments/quote?purpose=SPECIAL_DEPOSIT&campaign_id=" + campaign.ID.String()},
				{method: http.MethodPut, path: path, body: invitationBody(t, input)},
				{method: http.MethodPatch, path: path + "/status", body: invitationBody(t, dto.CampaignStatusInput{Status: status, Reason: "Again"})},
			} {
				if w := sessionRequest(router, req.method, req.path, req.body, access, nil); w.Code != http.StatusConflict {
					t.Errorf("terminal campaign %s %s: %d %s", req.method, req.path, w.Code, w.Body.String())
				}
			}
			// Closing or cancellation cannot discard funds from an existing checkout.
			fake.pay(payment.Reference, payment.Total)
			for range 2 {
				if w := signedWebhook(router, paystack.EventChargeSuccess, payment.Reference); w.Code != http.StatusOK {
					t.Fatalf("late webhook: %d %s", w.Code, w.Body.String())
				}
			}
			if status == "CLOSED" {
				if w := sessionRequest(router, http.MethodPut, path+"/resolution", `{"resolution":"A decision"}`, access, nil); w.Code != http.StatusConflict {
					t.Fatalf("closed campaign resolution: %d", w.Code)
				}
			} else {
				resolved := campaignResult(t, sessionRequest(router, http.MethodPut, path+"/resolution",
					`{"resolution":"  Admin will contact contributors  "}`, access, nil), http.StatusOK)
				if resolved.Resolution == nil || *resolved.Resolution != "Admin will contact contributors" ||
					resolved.ResolvedAt == nil || resolved.ResolvedBy == nil || *resolved.ResolvedBy != userID {
					t.Fatalf("resolution not recorded: %+v", resolved)
				}
			}
			for _, refunded := range []bool{true, false} {
				marked := campaignResult(t, sessionRequest(router, http.MethodPatch, path+"/refund-status",
					invitationBody(t, map[string]bool{"is_refunded": refunded}), access, nil), http.StatusOK)
				if marked.IsRefunded != refunded || marked.Status != status || marked.TotalContributed != payment.Amount || marked.ContributionCount != 1 {
					t.Fatalf("refund flag changed contributions or status: %+v", marked)
				}
			}
			if countRows(t, db, "SELECT COUNT(*) FROM transactions WHERE reference = $1", payment.Reference) != 1 {
				t.Fatal("lifecycle changes duplicated or removed the contribution ledger entry")
			}
			var list struct {
				Data dto.ListResponse[dto.Campaign]
			}
			w := sessionRequest(router, http.MethodGet, "/api/v1/campaigns?status="+strings.ToLower(status), "", access, nil)
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil ||
				len(list.Data.Items) != 1 || list.Data.Items[0].ID != campaign.ID {
				t.Fatalf("terminal campaign filter: %d %s", w.Code, w.Body.String())
			}
			if status == "CLOSED" {
				cancelled := campaignResult(t, sessionRequest(router, http.MethodPatch, path+"/status",
					`{"status":"CANCELLED","reason":"Event cancelled after collection ended"}`, access, nil), http.StatusOK)
				if cancelled.Status != "CANCELLED" || cancelled.TotalContributed != payment.Amount {
					t.Fatal("closed campaign cannot be cancelled while preserving its funds")
				}
			}
		})
	}
}

func TestCampaignManagementValidation(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router, _ := paymentsTestRouter(t, db, testOwnerAccount)
	userID := seedSessionUser(t, db, []string{"admin"}, false, false)
	access := invitationAdminToken(t, router)
	id := seedPaymentCampaign(t, db, userID)
	path := "/api/v1/campaigns/" + id.String()
	for _, req := range []struct {
		name, method, suffix, body string
		status                     int
	}{
		{name: "blank close reason", method: http.MethodPatch, suffix: "/status", body: `{"status":"CLOSED","reason":"  "}`, status: 400},
		{name: "unknown status", method: http.MethodPatch, suffix: "/status", body: `{"status":"ACTIVE","reason":"Reopen"}`, status: 400},
		{name: "blank resolution", method: http.MethodPut, suffix: "/resolution", body: `{"resolution":"  "}`, status: 400},
		{name: "open campaign resolution", method: http.MethodPut, suffix: "/resolution", body: `{"resolution":"An explanation"}`, status: 409},
		{name: "missing refund flag", method: http.MethodPatch, suffix: "/refund-status", body: `{}`, status: 400},
		{name: "wrong refund type", method: http.MethodPatch, suffix: "/refund-status", body: `{"is_refunded":"yes"}`, status: 400},
		{name: "open campaign refund flag", method: http.MethodPatch, suffix: "/refund-status", body: `{"is_refunded":true}`, status: 409},
	} {
		t.Run(req.name, func(t *testing.T) {
			w := sessionRequest(router, req.method, path+req.suffix, req.body, access, nil)
			if w.Code != req.status {
				t.Fatalf("want %d, got %d %s", req.status, w.Code, w.Body.String())
			}
		})
	}
	for _, req := range []struct{ method, suffix, body string }{
		{method: http.MethodPut, body: invitationBody(t, campaignInput())},
		{method: http.MethodPatch, suffix: "/status", body: `{"status":"CANCELLED","reason":"Event cancelled"}`},
		{method: http.MethodPut, suffix: "/resolution", body: `{"resolution":"Admin decision"}`},
		{method: http.MethodPatch, suffix: "/refund-status", body: `{"is_refunded":true}`},
	} {
		for _, target := range []struct {
			id     string
			status int
		}{
			{id: "bad", status: 400}, {id: uuid.NewString(), status: 404},
		} {
			w := sessionRequest(router, req.method, "/api/v1/campaigns/"+target.id+req.suffix, req.body, access, nil)
			if w.Code != target.status {
				t.Errorf("%s: want %d, got %d %s", req.suffix, target.status, w.Code, w.Body.String())
			}
		}
	}
	if _, err := db.Exec("UPDATE campaigns SET ends_at = NOW() - INTERVAL '1 minute' WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if w := sessionRequest(router, http.MethodPut, path, invitationBody(t, campaignInput()), access, nil); w.Code != http.StatusConflict {
		t.Fatalf("expired campaign reopened by editing: %d %s", w.Code, w.Body.String())
	}
	marked := campaignResult(t, sessionRequest(router, http.MethodPatch, path+"/refund-status",
		`{"is_refunded":true}`, access, nil), http.StatusOK)
	if !marked.IsRefunded || marked.Status != "CLOSED" {
		t.Fatal("naturally closed campaign could not be marked refunded")
	}
}
