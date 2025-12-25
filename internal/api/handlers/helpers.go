package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
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
	// Default to 20, clamp to [1,100]
	q := dto.QueryOptions{Limit: 20}

	// Parse & clamp limit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			q.Limit = uint32(n)
		}
	}

	// Directly assign cursor & sort if present
	if v := r.URL.Query().Get("cursor"); v != "" {
		q.Cursor = &v
	}
	if v := r.URL.Query().Get("sort"); v != "" {
		q.Sort = &v
	}

	return &q
}

func (h *Handlers) getTransactionFiltersQuery(r *http.Request) *dto.TransactionFilters {
	filters := dto.TransactionFilters{}

	if v := r.URL.Query().Get("ledger_type"); v != "" {
		filters.LedgerType = &v
	}

	if v := r.URL.Query().Get("type"); v != "" {
		filters.Type = &v
	}

	return &filters
}

// setRefreshCookie handles the specific logic of attaching the cookie
func setRefreshCookie(w http.ResponseWriter, refreshToken string, duration time.Duration, isDev bool) {
	secure := true
	sameSite := http.SameSiteStrictMode

	if isDev {
		secure = false
		sameSite = http.SameSiteLaxMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     token.RefreshTokenName,
		Value:    refreshToken,
		Path:     "/",
		Expires:  time.Now().Add(duration),
		MaxAge:   int(duration.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}
