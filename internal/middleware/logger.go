package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// LoggerMiddleware returns a handler that logs requests using the shared Zap logger
func (m *Middleware) LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		m.Logger.Sugar.Infow("incoming_request",
			"request_id", middleware.GetReqID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration", time.Since(start),
			"ip", r.RemoteAddr,
		)
	})
}
