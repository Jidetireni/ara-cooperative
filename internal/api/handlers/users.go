package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
)

func (h *Handlers) SetPassword(w http.ResponseWriter, r *http.Request) {
	var input dto.SetPasswordInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	authResponse, err := h.factory.Services.User.SetPassword(r.Context(), w, &input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	err = h.writeJSON(w, http.StatusCreated, authResponse, nil)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var input dto.LoginInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	authResponse, err := h.factory.Services.User.Login(r.Context(), w, &input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	err = h.writeJSON(w, http.StatusOK, authResponse, nil)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
}
