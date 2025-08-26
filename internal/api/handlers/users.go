package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative.git/internal/services"
)

func (h *Handlers) SetPassword(w http.ResponseWriter, r *http.Request) {
	var input dto.SetPasswordInput
	if input.Token == "" {
		input.Token = r.URL.Query().Get("token")
		if input.Token == "" {
			h.errorResponse(w, r, &svc.ApiError{
				Status:  http.StatusBadRequest,
				Message: "token is required",
			})
			return
		}
	}

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

func (h *Handlers) RefreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		h.errorResponse(w, r, &svc.ApiError{
			Status:  http.StatusUnauthorized,
			Message: "refresh token is required",
		})
		return
	}

	refreshed, err := h.factory.Services.User.RefreshToken(r.Context(), w, cookie.Value)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	err = h.writeJSON(w, http.StatusOK, refreshed, nil)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

}
