package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maxWebhookBodySize = 1 << 20

// CreatePayment starts a Paystack checkout and returns its authorization_url.
func (h *Handlers) CreatePayment(w http.ResponseWriter, r *http.Request) {
	var input dto.CreatePaymentInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	payment, err := h.factory.Services.Payments.Create(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	if err := h.writeJSON(w, http.StatusCreated, payment, nil); err != nil {
		h.logError(r, err)
	}
}

// PaymentQuote returns the breakdown for ?purpose=&amount=&fine_id= without
// starting a checkout.
func (h *Handlers) PaymentQuote(w http.ResponseWriter, r *http.Request) {
	input, err := parsePaymentQuoteQuery(r)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.validate.Struct(input); err != nil {
		h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest,
			Message: "purpose must be one of REGISTRATION_FEE, SAVINGS, SPECIAL_DEPOSIT, SHARES, FINES"})
		return
	}

	quote, err := h.factory.Services.Payments.Quote(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	if err := h.writeJSON(w, http.StatusOK, quote, nil); err != nil {
		h.logError(r, err)
	}
}

// GetPayment returns the member's stored payment status without verifying it.
func (h *Handlers) GetPayment(w http.ResponseWriter, r *http.Request) {
	payment, err := h.factory.Services.Payments.Get(r.Context(), chi.URLParam(r, "reference"))
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	if err := h.writeJSON(w, http.StatusOK, payment, nil); err != nil {
		h.logError(r, err)
	}
}

// PaystackWebhook receives Paystack events. Handled and ignored events answer
// 200; internal failures answer 500 so Paystack retries (applying is idempotent).
func (h *Handlers) PaystackWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBodySize))
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "unreadable webhook body"})
		return
	}

	if err := h.factory.Services.Payments.HandleWebhook(r.Context(), body, r.Header.Get("X-Paystack-Signature")); err != nil {
		h.errorResponse(w, r, err)
		return
	}

	if err := h.writeJSON(w, http.StatusOK, map[string]bool{"received": true}, nil); err != nil {
		h.logError(r, err)
	}
}

// ListPayments lists Paystack checkouts for admins, newest first.
func (h *Handlers) ListPayments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var filters dto.PaymentFilters
	if v := q.Get("member_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid UUID for 'member_id'"})
			return
		}
		filters.MemberID = &id
	}
	if v := q.Get("campaign_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid UUID for 'campaign_id'"})
			return
		}
		filters.CampaignID = &id
	}
	if v := strings.ToUpper(strings.TrimSpace(q.Get("status"))); v != "" {
		status := dto.PaymentStatus(v)
		filters.Status = &status
	}
	if v := strings.ToUpper(strings.TrimSpace(q.Get("purpose"))); v != "" {
		purpose := dto.LedgerType(v)
		filters.Purpose = &purpose
	}

	result, err := h.factory.Services.Payments.List(r.Context(), filters, h.getPaginationParams(r))
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, result, nil); err != nil {
		h.logError(r, err)
	}
}

// VerifyPayment lets an admin re-check a pending payment with Paystack when
// its webhook never arrived.
func (h *Handlers) VerifyPayment(w http.ResponseWriter, r *http.Request) {
	result, err := h.factory.Services.Payments.Verify(r.Context(), chi.URLParam(r, "reference"))
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, result, nil); err != nil {
		h.logError(r, err)
	}
}

func parsePaymentQuoteQuery(r *http.Request) (dto.CreatePaymentInput, error) {
	q := r.URL.Query()
	input := dto.CreatePaymentInput{Purpose: dto.LedgerType(strings.ToUpper(strings.TrimSpace(q.Get("purpose"))))}

	if v := q.Get("amount"); v != "" {
		amount, err := parsePositiveInt64(v)
		if err != nil {
			return input, &svc.APIError{Status: http.StatusBadRequest, Message: "amount must be a positive whole number of kobo"}
		}
		input.Amount = amount
	}
	if v := q.Get("fine_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return input, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid UUID for 'fine_id'"}
		}
		input.FineID = &id
	}
	if v := q.Get("campaign_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return input, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid UUID for 'campaign_id'"}
		}
		input.CampaignID = &id
	}
	return input, nil
}
