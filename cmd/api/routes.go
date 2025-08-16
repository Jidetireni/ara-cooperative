package main

import (
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) router() {
	s.Factory.Router.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.Logger)

		r.Get("/healthz", s.Handlers.HealthCheckHandler)
	})
}
