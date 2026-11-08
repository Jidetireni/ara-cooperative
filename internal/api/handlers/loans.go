package handlers

import (
	"net/http"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) CreateLoan(w http.ResponseWriter, r *http.Request) {
	var input dto.LoanInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	loan, err := h.factory.Services.Loans.Create(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusCreated, loan, nil); err != nil {
		h.logError(r, err)
	}
}

func loanID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid loan id"}
	}
	return id, nil
}

func (h *Handlers) GetLoan(w http.ResponseWriter, r *http.Request) {
	id, err := loanID(r)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	loan, err := h.factory.Services.Loans.Get(r.Context(), id)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, loan, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) ListLoans(w http.ResponseWriter, r *http.Request) {
	filters, err := parseLoanFilters(r)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	result, err := h.factory.Services.Loans.List(r.Context(), filters, h.getPaginationParams(r))
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, result, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) ListMyLoans(w http.ResponseWriter, r *http.Request) {
	var filters dto.LoanFilters
	if status := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))); status != "" {
		filters.Status = &status
	}
	result, err := h.factory.Services.Loans.ListMine(r.Context(), filters, h.getPaginationParams(r))
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, result, nil); err != nil {
		h.logError(r, err)
	}
}

func parseLoanFilters(r *http.Request) (dto.LoanFilters, error) {
	var filters dto.LoanFilters
	if value := r.URL.Query().Get("member_id"); value != "" {
		id, err := uuid.Parse(value)
		if err != nil {
			return filters, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid member_id"}
		}
		filters.MemberID = &id
	}
	if status := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))); status != "" {
		filters.Status = &status
	}
	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		filters.Search = &search
	}
	return filters, nil
}

func (h *Handlers) LoanEligibility(w http.ResponseWriter, r *http.Request) {
	id, err := loanID(r)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	checks, err := h.factory.Services.Loans.Eligibility(r.Context(), id)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, checks, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) GetLoanSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.factory.Services.Loans.Settings(r.Context())
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, settings, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) UpdateLoanSettings(w http.ResponseWriter, r *http.Request) {
	var input dto.LoanSettingsInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	settings, err := h.factory.Services.Loans.UpdateSettings(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, settings, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) ApproveLoan(w http.ResponseWriter, r *http.Request) {
	id, err := loanID(r)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	var input dto.LoanApprovalInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	loan, err := h.factory.Services.Loans.Approve(r.Context(), id, input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, loan, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) RejectLoan(w http.ResponseWriter, r *http.Request) {
	id, err := loanID(r)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	var input dto.LoanRejectionInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	loan, err := h.factory.Services.Loans.Reject(r.Context(), id, input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, loan, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) RecordLoanDisbursement(w http.ResponseWriter, r *http.Request) {
	id, err := loanID(r)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	var input dto.LoanDisbursementInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	loan, err := h.factory.Services.Loans.Disburse(r.Context(), id, input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, loan, nil); err != nil {
		h.logError(r, err)
	}
}
