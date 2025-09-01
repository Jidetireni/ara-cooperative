package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/internal/constants"
	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
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
	permission := []constants.UserPermmisions{constants.MemberReadPermission}
	
	queryOptions := h.getPaginationParams(r)
	options := repository.QueryOptions{}
	if queryOptions != nil {
		options = repository.QueryOptions{
			Limit:  queryOptions.Limit,
			Cursor: queryOptions.Cursor,
			Sort:   queryOptions.Sort,
		}
	}

	savings, err := h.factory.Repositories.Saving.List(r.Context(), repository.SavingRepositoryFilter{
		Confirmed: lo.ToPtr(false),
		Rejected:  lo.ToPtr(false),
		Type:      lo.ToPtr(repository.TransactionTypeDEPOSIT),
	}, options)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, savings, nil)
}
