package handlers

import (
	"net/http"
	"strconv"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

func (h *Handlers) SetShareUnitPrice(w http.ResponseWriter, r *http.Request) {
	permission := []constants.UserPermissions{constants.MemberWriteALLPermission}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	var input dto.SetShareUnitPriceInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	_, err := h.factory.Services.Shares.SetUnitPrice(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"message": "updated successfully"}, nil)
}

// GetShareUnitPrice returns the current unit price.
func (h *Handlers) GetShareUnitPrice(w http.ResponseWriter, r *http.Request) {
	price := h.factory.Services.Shares.GetUnitPrice(r.Context())
	h.writeJSON(w, http.StatusOK, map[string]int64{"unit_price": price}, nil)
}

func (h *Handlers) GetShareQuote(w http.ResponseWriter, r *http.Request) {
	if q := r.URL.Query().Get("amount"); q != "" {
		amount, err := strconv.ParseInt(q, 10, 64)
		if err != nil || amount <= 0 {
			h.errorResponse(w, r, &svc.ApiError{
				Status:  http.StatusBadRequest,
				Message: "amount must be a positive integer",
			})
			return
		}

		unitPrice := h.factory.Services.Shares.GetUnitPrice(r.Context())
		if unitPrice <= 0 {
			h.errorResponse(w, r, &svc.ApiError{
				Status:  http.StatusServiceUnavailable,
				Message: "unit price is not set",
			})
			return
		}

		// fractional quote (4 d.p.)
		unitsFloat := float64(amount) / float64(unitPrice)
		unitsFloat = float64(int64(unitsFloat*1e4+0.5)) / 1e4
		remainder := amount % unitPrice

		h.writeJSON(w, http.StatusOK, dto.GetUnitsQuote{
			Units:     unitsFloat,
			Remainder: remainder,
			UnitPrice: unitPrice,
		}, nil)
		return
	}
}

func (h *Handlers) BuyShares(w http.ResponseWriter, r *http.Request) {
	var input dto.BuySharesInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.factory.Services.Shares.BuyShares(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, result, nil)
}

func (h *Handlers) ListPendingSharesTransactions(w http.ResponseWriter, r *http.Request) {
	permission := []constants.UserPermissions{constants.MemberReadALLPermission}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	filters := repository.ShareRepositoryFilter{
		Confirmed:  lo.ToPtr(false),
		Rejected:   lo.ToPtr(false),
		Type:       lo.ToPtr(repository.TransactionTypeDEPOSIT),
		LedgerType: repository.LedgerTypeSHARES,
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

	result, err := h.factory.Repositories.Shares.List(r.Context(), filters, options)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	dtoItems := lo.Map(result.Items, func(item *repository.PopShare, _ int) dto.Shares {
		if mapped := h.factory.Services.Shares.MapRepositoryToDTO(item); mapped != nil {
			return *mapped
		}
		return dto.Shares{}
	})

	h.writeJSON(w, http.StatusOK, dto.ListResponse[dto.Shares]{
		Items:      dtoItems,
		NextCursor: result.NextCursor,
	}, nil)
}

func (h *Handlers) GetTotalSharesPurchased(w http.ResponseWriter, r *http.Request) {
	permission := []constants.UserPermissions{constants.MemberReadALLPermission}
	hasPermission := users.HasAdminPermissions(r.Context(), permission)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permission))
		return
	}

	filters := repository.ShareRepositoryFilter{
		Confirmed:  lo.ToPtr(true),
		Rejected:   lo.ToPtr(false),
		Type:       lo.ToPtr(repository.TransactionTypeDEPOSIT),
		LedgerType: repository.LedgerTypeSHARES,
	}

	total, err := h.factory.Repositories.Shares.CountTotalSharesPurchased(r.Context(), filters)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	units, err := strconv.ParseFloat(total.Units, 64)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, dto.SharesTotal{
		Units:  units,
		Amount: total.Amount,
	}, nil)

}

func (h *Handlers) GetMemberTotalSharesPurchased(w http.ResponseWriter, r *http.Request) {
	userID := users.FromContext(r.Context()).ID
	if userID == uuid.Nil {
		h.errorResponse(w, r, &svc.ApiError{
			Status:  http.StatusUnauthorized,
			Message: "unauthenticated",
		})
		return
	}

	member, err := h.factory.Repositories.Member.Get(r.Context(), repository.MemberRepositoryFilter{
		UserID: &userID,
	})
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	filters := repository.ShareRepositoryFilter{
		Confirmed:  lo.ToPtr(true),
		Rejected:   lo.ToPtr(false),
		Type:       lo.ToPtr(repository.TransactionTypeDEPOSIT),
		MemberID:   &member.ID,
		LedgerType: repository.LedgerTypeSHARES,
	}

	total, err := h.factory.Repositories.Shares.CountTotalSharesPurchased(r.Context(), filters)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	units, err := strconv.ParseFloat(total.Units, 64)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, dto.SharesTotal{
		Units:  units,
		Amount: total.Amount,
	}, nil)
}
