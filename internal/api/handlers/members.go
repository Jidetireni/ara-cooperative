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
	permissions := []constants.UserPermissions{constants.MemberWriteALL}
	hasPermission := users.HasAdminPermissions(r.Context(), permissions)
	if !hasPermission {
		h.errorResponse(w, r, svc.AdminForbiddenError(permissions))
		return
	}

	var input dto.CreateMemberInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	invitation, err := h.factory.Services.Member.Create(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	if err := h.writeJSON(w, http.StatusCreated, invitation, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	permissions := []constants.UserPermissions{constants.MemberReadALL}
	if !users.HasAdminPermissions(r.Context(), permissions) {
		h.errorResponse(w, r, svc.AdminForbiddenError(permissions))
		return
	}

	filters, err := h.parseMemberFilters(r)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	options := h.getPaginationParams(r)

	members, err := h.factory.Services.Member.List(r.Context(), filters, options)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}

	if err := h.writeJSON(w, http.StatusOK, members, nil); err != nil {
		h.logError(r, err)
	}
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

func (h *Handlers) CurrentMember(w http.ResponseWriter, r *http.Request) {
	member, err := h.factory.Services.Member.Current(r.Context())
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, member, nil); err != nil {
		h.logError(r, err)
	}
}

func (h *Handlers) UpdateMemberProfile(w http.ResponseWriter, r *http.Request) {
	var input dto.UpdateMemberProfileInput
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	member, err := h.factory.Services.Member.UpdateProfile(r.Context(), input)
	if err != nil {
		h.errorResponse(w, r, err)
		return
	}
	if err := h.writeJSON(w, http.StatusOK, member, nil); err != nil {
		h.logError(r, err)
	}
}
