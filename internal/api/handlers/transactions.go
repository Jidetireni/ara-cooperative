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
	permission := []constants.UserPermissions{constants.MemberWriteALL}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	statusIDStr := chi.URLParam(r, "status_id")
	statusID, err := uuid.Parse(statusIDStr)
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{
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
	permission := []constants.UserPermissions{constants.MemberReadALL}
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

	result, err := h.factory.Repositories.Transaction.ListPopulated(r.Context(), repoFilters, options)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	dtoItems := lo.Map(result.Items, func(item *repository.PopulatedTransaction, _ int) dto.Transactions {
		return *h.factory.Repositories.Transaction.MapRepositoryToDTOModel(item)
	})

	h.writeJSON(w, http.StatusOK, dto.ListResponse[dto.Transactions]{
		Items:      dtoItems,
		NextCursor: result.NextCursor,
	}, nil)
}

// ListTransactions lists every member's transactions for admins. Query:
// member_id, ledger_type, type, status (PENDING|CONFIRMED|REJECTED), search,
// limit, cursor.
func (h *Handlers) ListTransactions(w http.ResponseWriter, r *http.Request) {
	filters, err := parseTransactionListFilters(r)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	result, err := h.factory.Services.Transactions.ListTransactions(r.Context(), filters, h.getPaginationParams(r))
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	if err := h.writeJSON(w, http.StatusOK, result, nil); err != nil {
		h.logError(r, err)
	}
}

// ListMyTransactions lists the signed-in member's own transactions, newest first.
func (h *Handlers) ListMyTransactions(w http.ResponseWriter, r *http.Request) {
	filters, err := parseTransactionListFilters(r)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	result, err := h.factory.Services.Transactions.ListMyTransactions(r.Context(), filters, h.getPaginationParams(r))
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	if err := h.writeJSON(w, http.StatusOK, result, nil); err != nil {
		h.logError(r, err)
	}
}

// LedgerTotals returns confirmed deposits per ledger across all members.
func (h *Handlers) LedgerTotals(w http.ResponseWriter, r *http.Request) {
	totals, err := h.factory.Services.Transactions.LedgerTotals(r.Context())
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	if err := h.writeJSON(w, http.StatusOK, totals, nil); err != nil {
		h.logError(r, err)
	}
}

func parseTransactionListFilters(r *http.Request) (dto.TransactionFilters, error) {
	q := r.URL.Query()
	filters := dto.TransactionFilters{}
	if v := q.Get("member_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return filters, &svc.APIError{Status: http.StatusBadRequest, Message: "invalid UUID for 'member_id'"}
		}
		filters.MemberID = &id
	}
	if v := q.Get("ledger_type"); v != "" {
		filters.LedgerType = &v
	}
	if v := q.Get("type"); v != "" {
		filters.Type = &v
	}
	if v := q.Get("status"); v != "" {
		filters.Status = lo.ToPtr(dto.TransactionStatusType(v))
	}
	if v := q.Get("search"); v != "" {
		filters.Search = &v
	}
	return filters, nil
}
