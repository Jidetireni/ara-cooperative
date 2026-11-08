package handlers

import (
	"github.com/google/uuid"
	"net/http"
	"strconv"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
)

func (h *Handlers) GetShareUnitPrice(w http.ResponseWriter, r *http.Request) {
	unitPrice, err := h.factory.Services.Transactions.GetSharesUnitPrice(r.Context())
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]int64{"unit_price": unitPrice}, nil)
}

func (h *Handlers) GetShareQuote(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("amount")
	if q == "" {
		h.errorResponse(w, r, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "amount query parameter is required",
		})
		return
	}

	amount, err := strconv.ParseInt(q, 10, 64)
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "amount must be a valid integer",
		})
		return
	}

	quote, err := h.factory.Services.Transactions.GetShareQuote(r.Context(), amount)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, quote, nil)
}

func (h *Handlers) BuyShares(w http.ResponseWriter, r *http.Request) {
	var input dto.BuySharesInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.factory.Services.Transactions.BuyShares(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, result, nil)
}

func (h *Handlers) GetTotalSharesPurchased(w http.ResponseWriter, r *http.Request) {
	permission := []constants.UserPermissions{constants.MemberReadALL}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	total, err := h.factory.Services.Transactions.GetTotalShares(r.Context())
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, total, nil)
}

func (h *Handlers) GetMemberTotalSharesPurchased(w http.ResponseWriter, r *http.Request) {
	total, err := h.factory.Services.Transactions.GetMemberTotalShares(r.Context())
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, total, nil)
}

// ListSharePurchases lists share purchases for admins. Query: member_id,
// limit, cursor.
func (h *Handlers) ListSharePurchases(w http.ResponseWriter, r *http.Request) {
	var memberID *uuid.UUID
	if v := r.URL.Query().Get("member_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid UUID for 'member_id'"})
			return
		}
		memberID = &id
	}

	result, err := h.factory.Services.Transactions.ListSharePurchases(r.Context(), memberID, h.getPaginationParams(r))
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	if err := h.writeJSON(w, http.StatusOK, result, nil); err != nil {
		h.logError(r, err)
	}
}
