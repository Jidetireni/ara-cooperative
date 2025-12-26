package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
)

func (h *Handlers) SpecialDeposit(w http.ResponseWriter, r *http.Request) {
	var input dto.TransactionsInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	saving, err := h.factory.Services.Transactions.DepositSpecial(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, saving, nil)
}

func (h *Handlers) SpecialDepositBalance(w http.ResponseWriter, r *http.Request) {
	balance, err := h.factory.Services.Transactions.GetSpecialDepositBalance(r.Context())
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]int64{"balance": balance}, nil)
}
