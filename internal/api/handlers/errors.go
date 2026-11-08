package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/internal/services"
)

func (h *Handlers) logError(r *http.Request, err error) {
	h.factory.Pkgs.Logger.Sugar.Errorw("request failed", "path", r.URL.Path, "error", err)
}

func (h *Handlers) errorResponse(w http.ResponseWriter, r *http.Request, message any) {
	status := http.StatusInternalServerError

	resp := map[string]any{
		"status":  status,
		"message": "Internal Server Error",
	}

	// Client-supplied list options that the repository rejected are bad requests.
	if err, ok := message.(error); ok {
		switch {
		case errors.Is(err, repository.ErrInvalidSort):
			message = &services.APIError{Status: http.StatusBadRequest,
				Message: "invalid sort; use created_at:asc or created_at:desc"}
		case errors.Is(err, repository.ErrInvalidCursor):
			message = &services.APIError{Status: http.StatusBadRequest, Message: "invalid cursor"}
		}
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
