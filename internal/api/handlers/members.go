package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/go-chi/chi/v5"
)

func (h *Handlers) CreateMember(w http.ResponseWriter, r *http.Request) {
	permissions := []constants.UserPermissions{constants.MemberWriteALLPermission}
	hasPermission := users.HasAdminPermissions(r.Context(), permissions)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permissions))
		return
	}
	var input dto.CreateMemberInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	createdMember, err := h.factory.Services.Member.Create(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, createdMember, http.Header{})
}

func (h *Handlers) MemberBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	member, err := h.factory.Services.Member.GetBySlug(r.Context(), slug)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, member, nil)
}
