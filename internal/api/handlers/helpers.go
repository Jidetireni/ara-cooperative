package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/internal/services"
)

type envelope map[string]any

func (h *Handlers) writeJSON(w http.ResponseWriter, status int, data interface{}, headers http.Header) error {
	for key, value := range headers {
		w.Header()[key] = value
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		return err
	}

	return nil
}

func (h *Handlers) logError(r *http.Request, err error) {
	fmt.Printf("Server Error for request %s: %v\n", r.URL.Path, err)
}

func (h *Handlers) errorResponse(w http.ResponseWriter, r *http.Request, message any) {
	env := envelope{"error": message}

	status := http.StatusInternalServerError
	if apiErr, ok := message.(*services.ApiError); ok {
		status = apiErr.Status
		env["error"] = apiErr.Message
	}

	if err := h.writeJSON(w, status, env, nil); err != nil {
		h.logError(r, err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
