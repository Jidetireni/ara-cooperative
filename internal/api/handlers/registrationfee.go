package handlers

import (
	"fmt"
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/transactions"
)

func (h *Handlers) PayRegistrationFee(w http.ResponseWriter, r *http.Request) {
	var input dto.TransactionsInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	if input.Amount != transactions.DefaultRegistrationFee {
		h.errorResponse(w, r, svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("registration fee must be exactly %d", transactions.DefaultRegistrationFee),
		})
		return
	}

	fee, err := h.factory.Services.Transactions.ChargeRegistrationFee(r.Context(), &input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, fee, nil)
}
