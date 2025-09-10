package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
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
	if err := json.NewEncoder(w).Encode(map[string]any{
		"data":   data,
		"status": status,
	}); err != nil {
		return err
	}

	return nil
}

func (h *Handlers) getPaginationParams(r *http.Request) *dto.QueryOptions {
	// Default limit
	var query dto.QueryOptions
	query.Limit = 20
	// Parse limit from query parameters
	limitParam := r.URL.Query().Get("limit")
	if limitParam != "" {
		var limit int
		_, err := fmt.Sscanf(limitParam, "%d", &limit)
		if err == nil && limit > 0 {
			fmt.Println("Parsed Limit:", limit)
			query.Limit = uint32(limit)
		}
	}

	cursorParam := r.URL.Query().Get("cursor")
	if cursorParam != "" {
		var cursor string
		_, err := fmt.Sscanf(cursorParam, "%s", &cursor)
		if err == nil && cursor != "" {
			query.Cursor = &cursor
		}
	}

	sortParam := r.URL.Query().Get("sort")
	if sortParam != "" {
		var sort string
		_, err := fmt.Sscanf(sortParam, "%s", &sort)
		if err == nil && sort != "" {
			query.Sort = &sort
		}
	}

	return &query
}
