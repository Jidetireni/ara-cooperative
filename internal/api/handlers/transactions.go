package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	permission := []constants.UserPermissions{constants.MemberWriteALLPermission}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	transactionIDStr := chi.URLParam(r, "id")
	transactionID, err := uuid.Parse(transactionIDStr)
	if err != nil {
		h.errorResponse(w, r, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "Invalid transaction ID",
		})
		return
	}

	var input dto.UpdateTransactionStatusInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.factory.Services.Transactions.UpdateStatus(r.Context(), &transactionID, &input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, result, nil)
}
