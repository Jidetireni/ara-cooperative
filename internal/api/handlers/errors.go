package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/services"
)

func (h *Handlers) logError(r *http.Request, err error) {
	fmt.Printf("Server Error for request %s: %v\n", r.URL.Path, err)
}

func (h *Handlers) errorResponse(w http.ResponseWriter, r *http.Request, message any) {
	status := http.StatusInternalServerError
	msg := "internal server error"

	if apiErr, ok := message.(*services.ApiError); ok {
		status = apiErr.Status
		msg = apiErr.Message
	} else if err, ok := message.(error); ok {
		h.logError(r, err)
	} else {
		h.logError(r, fmt.Errorf("%v", message))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"message": msg,
		"status":  status,
	}); err != nil {
		h.logError(r, fmt.Errorf("failed to write error response: %w", err))
		return
	}
}
