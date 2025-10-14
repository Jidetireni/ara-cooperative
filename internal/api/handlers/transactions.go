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

func (h *Handlers) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	permission := []constants.UserPermissions{constants.MemberWriteALLPermission}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	statusIDStr := chi.URLParam(r, "status_id")
	statusID, err := uuid.Parse(statusIDStr)
	if err != nil {
		h.errorResponse(w, r, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "Invalid status ID",
		})
		return
	}

	var input dto.UpdateTransactionStatusInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.factory.Services.Transactions.UpdateStatus(
		r.Context(),
		&statusID,
		&input,
	)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, result, nil)
}

func (h *Handlers) ListPendingTransactions(w http.ResponseWriter, r *http.Request) {
	permission := []constants.UserPermissions{constants.MemberReadALLPermission}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	filters := h.getTransactionFiltersQuery(r)
	repoFilters := repository.TransactionRepositoryFilter{}
	repoFilters.Confirmed = lo.ToPtr(false)
	repoFilters.Rejected = lo.ToPtr(false)

	if filters.LedgerType != nil {
		repoFilters.LedgerType = lo.ToPtr(repository.LedgerType(*filters.LedgerType))
	}
	if filters.Type != nil {
		repoFilters.Type = lo.ToPtr(repository.TransactionType(*filters.Type))
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

	result, err := h.factory.Repositories.Transaction.List(r.Context(), repoFilters, options)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	dtoItems := lo.Map(result.Items, func(item *repository.PopTransaction, _ int) dto.Transactions {
		return *h.factory.Services.Transactions.MapPopTransactionToDTO(item)
	})

	h.writeJSON(w, http.StatusOK, dto.ListResponse[dto.Transactions]{
		Items:      dtoItems,
		NextCursor: result.NextCursor,
	}, nil)
}
