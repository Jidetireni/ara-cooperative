package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
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
