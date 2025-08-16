package handlers

import "net/http"

func (h *Handlers) CreateMember(w http.ResponseWriter, r *http.Request) {
	var input CreateMemberInput

	if !h.decodeAndValidate(w, r, &input) {
		return
	}

}
