package handlers

import (
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
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
