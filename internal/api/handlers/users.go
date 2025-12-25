package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
)

func (h *Handlers) SetPassword(w http.ResponseWriter, r *http.Request) {
	var input dto.SetPasswordInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	authResponse, refreshToken, err := h.factory.Services.User.SetPassword(r.Context(), &input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	setRefreshCookie(w, refreshToken, token.RefreshTokenExpirationTime, h.config.IsDev)
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

	authResponse, refreshToken, err := h.factory.Services.User.Login(r.Context(), &input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	setRefreshCookie(w, refreshToken, token.RefreshTokenExpirationTime, h.config.IsDev)
	err = h.writeJSON(w, http.StatusOK, authResponse, nil)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
}

func (h *Handlers) RefreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(token.RefreshTokenName)
	if err != nil {
		h.errorResponse(w, r, &svc.APIError{
			Status:  http.StatusUnauthorized,
			Message: "No refresh token provided",
		})
		return
	}

	resp, refreshToken, err := h.factory.Services.User.RefreshToken(r.Context(), cookie.Value)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	setRefreshCookie(w, refreshToken, token.RefreshTokenExpirationTime, h.config.IsDev)
	err = h.writeJSON(w, http.StatusOK, resp.AccessToken, nil)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
}
