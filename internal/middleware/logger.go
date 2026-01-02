package middleware

import (
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/go-chi/chi/v5/middleware"
)

// LoggerMiddleware returns a handler that logs requests using your zerolog instance
func (m *Middleware) LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		var userID string
		user, ok := users.FromContext(r.Context())
		if !ok {
			userID = ""
		} else {
			userID = user.ID.String()
		}

		m.Logger.Info().
			Str("request_id", middleware.GetReqID(r.Context())).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", ww.Status()).
			Dur("duration", time.Since(start)).
			Str("ip", r.RemoteAddr).
			Str("user_id", userID).
			Msg("incoming_request")
	})
}
