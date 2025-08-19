package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
)

func (h *Handlers) SignUp(w http.ResponseWriter, r *http.Request) {
	var input dto.SignUpInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	authResponse, err := h.factory.Services.User.SignUp(r.Context(), w, &input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, authResponse, nil)
}
