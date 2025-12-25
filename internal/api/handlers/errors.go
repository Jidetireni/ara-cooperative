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

	resp := map[string]any{
		"status":  status,
		"message": "Internal Server Error",
	}

	if apiErr, ok := message.(*services.APIError); ok {
		status = apiErr.Status
		resp["status"] = status
		resp["message"] = apiErr.Message
		if apiErr.Errors != nil {
			resp["errors"] = apiErr.Errors
		}
	} else if err, ok := message.(error); ok {
		h.logError(r, err)
	} else {
		h.logError(r, fmt.Errorf("%v", message))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logError(r, fmt.Errorf("failed to write error response: %w", err))
		return
	}
}
