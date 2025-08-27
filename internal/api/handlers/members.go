package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
	"github.com/go-chi/chi/v5"
)

func (h *Handlers) CreateMember(w http.ResponseWriter, r *http.Request) {
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
