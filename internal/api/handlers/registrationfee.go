package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
)

func (h *Handlers) PayRegistrationFee(w http.ResponseWriter, r *http.Request) {
	var input dto.TransactionsInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	fee, err := h.factory.Services.Transactions.ChargeRegistrationFee(r.Context(), &input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, fee, nil)
}
