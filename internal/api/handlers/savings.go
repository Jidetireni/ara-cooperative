package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

func (h *Handlers) DepositSavings(w http.ResponseWriter, r *http.Request) {
	var input dto.SavingsDepositInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	createdSavings, err := h.factory.Services.Savings.Deposit(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, createdSavings, nil)
}

func (h *Handlers) ListPendingDeposits(w http.ResponseWriter, r *http.Request) {
	permission := []constants.UserPermissions{constants.MemberReadALLPermission}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	filters := repository.SavingRepositoryFilter{
		Confirmed: lo.ToPtr(false),
		Rejected:  lo.ToPtr(false),
		Type:      lo.ToPtr(repository.TransactionTypeDEPOSIT),
	}

	queryOptions := h.getPaginationParams(r)
	options := repository.QueryOptions{}
	if queryOptions != nil {
		options = repository.QueryOptions{
			Limit:  queryOptions.Limit,
			Cursor: queryOptions.Cursor,
			Sort:   queryOptions.Sort,
		}
	}

	result, err := h.factory.Repositories.Saving.List(r.Context(), filters, options)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	dtoItems := lo.Map(result.Items, func(item *repository.Saving, _ int) dto.Savings {
		return *h.factory.Services.Savings.MapRepositoryToDTO(item)
	})

	h.writeJSON(w, http.StatusOK, dto.ListResponse[dto.Savings]{
		Items:      dtoItems,
		NextCursor: result.NextCursor,
	}, nil)
}

func (h *Handlers) ConfirmSavings(w http.ResponseWriter, r *http.Request) {
	permission := []constants.UserPermissions{constants.MemberWriteALLPermission}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	transactionIDStr := chi.URLParam(r, "transaction_id")
	transactionID, err := uuid.Parse(transactionIDStr)
	if err != nil {
		h.errorResponse(w, r, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "Invalid transaction ID",
		})
		return
	}

	confirmed, err := h.factory.Services.Savings.Confirm(r.Context(), &transactionID)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]bool{"confirmed": confirmed}, nil)
}

func (h *Handlers) RejectSavings(w http.ResponseWriter, r *http.Request) {
	permission := []constants.UserPermissions{constants.MemberWriteALLPermission}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	transactionIDStr := chi.URLParam(r, "transaction_id")
	transactionID, err := uuid.Parse(transactionIDStr)
	if err != nil {
		h.errorResponse(w, r, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "Invalid transaction ID",
		})
		return
	}

	rejected, err := h.factory.Services.Savings.Reject(r.Context(), &transactionID)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]bool{"rejected": rejected}, nil)
}

func (h *Handlers) SavingsBalance(w http.ResponseWriter, r *http.Request) {
	balance, err := h.factory.Services.Savings.GetBalance(r.Context())
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]int64{"balance": balance}, nil)
}
