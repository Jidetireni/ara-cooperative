package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) GrantAdmin(w http.ResponseWriter, r *http.Request) {
	var input dto.GrantAdminInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	account, err := h.factory.Services.User.GrantAdmin(r.Context(), input.Email)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, account, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) RevokeAdmin(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid user ID"})
		return
	}
	account, err := h.factory.Services.User.RevokeAdmin(r.Context(), userID)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, account, nil); err != nil {
		h.logError(r, err)
	}
}
