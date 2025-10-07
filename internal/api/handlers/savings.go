package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/internal/services/transactions"
)

func (h *Handlers) DepositSavings(w http.ResponseWriter, r *http.Request) {
	var input dto.TransactionsInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	saving, err := h.factory.Services.Transactions.CreateTransaction(
		r.Context(),
		transactions.TransactionParams{
			Input:      input,
			Type:       repository.TransactionTypeDEPOSIT,
			LedgerType: repository.LedgerTypeSAVINGS,
		},
	)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, saving, nil)
}

func (h *Handlers) SavingsBalance(w http.ResponseWriter, r *http.Request) {
	balance, err := h.factory.Services.Transactions.GetBalance(
		r.Context(),
		repository.LedgerTypeSAVINGS,
	)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]int64{"balance": balance}, nil)
}
