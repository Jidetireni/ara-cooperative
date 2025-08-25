package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/internal/services"
)

// TODO: seperate some errors to be authomatically handled
// like unique constraint errors, validation errors, etc.

type envelope map[string]any

func (h *Handlers) writeJSON(w http.ResponseWriter, status int, data interface{}, headers http.Header) error {
	for key, value := range headers {
		w.Header()[key] = value
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"data":   data,
		"status": status,
	}); err != nil {
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"message": env["error"],
		"status":  status,
	}); err != nil {
		h.logError(r, fmt.Errorf("failed to write error response: %w", err))
		return
	}

	h.logError(r, fmt.Errorf("%v", message))
}
