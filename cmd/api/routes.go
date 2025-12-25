package main

import (
	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) router() {
	s.Factory.Router.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.RequestID)
		r.Use(s.Factory.Middleware.LoggerMiddleware)

		r.Get("/health", s.Handlers.HealthCheckHandler)
		r.Post("/set-password", s.Handlers.SetPassword)
		r.Post("/login", s.Handlers.Login)
		r.Post("/refresh", s.Handlers.RefreshToken)

		r.Route("/members", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))

				r.Post("/", s.Handlers.CreateMember)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
				r.Route("/me", func(r chi.Router) {
					r.Get("/savings/balance", s.Handlers.SavingsBalance)
				})

				r.Get("/{slug}", s.Handlers.MemberBySlug)
			})
		})

		r.Route("/savings", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))

				r.Post("/", s.Handlers.DepositSavings)
				r.Get("/me", s.Handlers.SavingsBalance)
			})
		})

		r.Route("/special-deposit", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
				r.Post("/", s.Handlers.SpecialDeposit)
				r.Get("/me", s.Handlers.SpecialDepositBalance)
			})
		})

		r.Route("/transactions", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))

				r.Patch("/status/{status_id}", s.Handlers.UpdateStatus)
				r.Get("/pending", s.Handlers.ListPendingTransactions)
			})
		})

		r.Route("/shares", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))

				r.Patch("/unit-price", s.Handlers.SetShareUnitPrice)
				r.Get("/total", s.Handlers.GetTotalSharesPurchased)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))

				r.Get("/unit-price", s.Handlers.GetShareUnitPrice)
				r.Get("/quotes", s.Handlers.GetShareQuote)
				r.Post("/", s.Handlers.BuyShares)
				r.Get("/me/total", s.Handlers.GetMemberTotalSharesPurchased)
			})
		})

		r.Route("/fines", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))
				r.Post("/", s.Handlers.CreateFine)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
				r.Post("/{id}/pay", s.Handlers.PayFine)
			})
		})

		r.Route("/registration-fee", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))
				r.Post("/", s.Handlers.PayRegistrationFee)
			})
		})
	})
}
