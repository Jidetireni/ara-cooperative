package handlers

import (
	"fmt"
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) CreateFine(w http.ResponseWriter, r *http.Request) {
	var input dto.FineInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	fine, err := h.factory.Services.Transactions.ChargeFine(r.Context(), &input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, fine, nil)
}

func (h *Handlers) PayFine(w http.ResponseWriter, r *http.Request) {
	var input dto.TransactionsInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	IDStr := chi.URLParam(r, "id")
	fineID, err := uuid.Parse(IDStr)
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("invalid fine ID: %v", err),
		})
		return
	}

	fine, err := h.factory.Services.Transactions.PayFine(r.Context(), fineID, &input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, fine, nil)
}

func (h *Handlers) ListMyFines(w http.ResponseWriter, r *http.Request) {
	filters, err := h.parseFineFilters(r)
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	options := h.getPaginationParams(r)

	fines, err := h.factory.Services.Transactions.ListFines(r.Context(), &filters, options)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, fines, nil)
}

// For Admins: GET /fines/admin?member_id=...
func (h *Handlers) ListAllFines(w http.ResponseWriter, r *http.Request) {
	filters, err := h.parseFineFilters(r)
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	options := h.getPaginationParams(r)

	fines, err := h.factory.Services.Transactions.ListFines(r.Context(), &filters, options)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, fines, nil)
}
