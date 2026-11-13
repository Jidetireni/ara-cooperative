package main

import (
	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) router() {
	s.Factory.Router.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.RequestID)
		r.Use(s.Factory.Middleware.RequestIDResponse)
		r.Use(s.Factory.Middleware.LoggerMiddleware)

		r.Get("/health", s.Handlers.HealthCheckHandler)
		r.Post("/set-password", s.Handlers.SetPassword)
		r.Post("/login", s.Handlers.Login)
		r.Post("/refresh", s.Handlers.RefreshToken)
		r.With(s.Factory.Middleware.RequireAuth).Get("/me", s.Handlers.CurrentUser)
		r.With(
			s.Factory.Middleware.RequireAuth,
			s.Factory.Middleware.RequireRole(constants.RoleOwner),
		).Put("/admins", s.Handlers.GrantAdmin)
		r.With(
			s.Factory.Middleware.RequireAuth,
			s.Factory.Middleware.RequireRole(constants.RoleOwner),
		).Delete("/admins/{user_id}", s.Handlers.RevokeAdmin)

		r.Route("/members", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))

				r.Get("/me", s.Handlers.CurrentMember)
				r.Put("/me", s.Handlers.UpdateMemberProfile)
				r.Post("/me/passport-photo/upload", s.Handlers.PassportPhotoUpload)
				r.Put("/me/passport-photo", s.Handlers.SavePassportPhoto)
				r.Get("/search", s.Handlers.SearchMembers)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))

				r.Post("/", s.Handlers.CreateMember)
				r.Get("/", s.Handlers.ListMembers)
				r.Get("/{slug}/profile", s.Handlers.AdminMemberProfile)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
				r.Use(s.Factory.Middleware.RequireActiveMember)
				r.Get("/{slug}", s.Handlers.MemberBySlug)
			})
		})

		r.Route("/savings", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
				r.Use(s.Factory.Middleware.RequireActiveMember)

				r.Post("/", s.Handlers.DepositSavings)
				r.Get("/me", s.Handlers.SavingsBalance)
			})
		})

		r.Route("/special-deposit", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
				r.Use(s.Factory.Middleware.RequireActiveMember)
				r.Post("/", s.Handlers.SpecialDeposit)
				r.Get("/me", s.Handlers.SpecialDepositBalance)
			})
		})

		r.Route("/transactions", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))

				r.Get("/", s.Handlers.ListTransactions)
				r.Get("/totals", s.Handlers.LedgerTotals)
				r.Patch("/status/{status_id}", s.Handlers.UpdateStatus)
				r.Get("/pending", s.Handlers.ListPendingTransactions)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
				r.Use(s.Factory.Middleware.RequireActiveMember)

				r.Get("/me", s.Handlers.ListMyTransactions)
			})
		})

		r.Route("/shares", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))

				r.Get("/", s.Handlers.ListSharePurchases)
				r.Get("/total", s.Handlers.GetTotalSharesPurchased)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
				r.Use(s.Factory.Middleware.RequireActiveMember)

				r.Get("/unit-price", s.Handlers.GetShareUnitPrice)
				r.Get("/quotes", s.Handlers.GetShareQuote)
				r.Post("/", s.Handlers.BuyShares)
				r.Get("/me/total", s.Handlers.GetMemberTotalSharesPurchased)
			})
		})

		r.Route("/banks", func(r chi.Router) {
			r.Use(s.Factory.Middleware.RequireAuth)
			r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
			r.Get("/", s.Handlers.ListBanks)
			r.Get("/resolve", s.Handlers.ResolveBankAccount)
		})

		r.Route("/loans", func(r chi.Router) {
			r.Use(s.Factory.Middleware.RequireAuth)
			r.Get("/{id}", s.Handlers.GetLoan)
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
				r.Use(s.Factory.Middleware.RequireActiveMember)
				r.Post("/", s.Handlers.CreateLoan)
				r.Get("/me", s.Handlers.ListMyLoans)
				r.Get("/guarantor-requests", s.Handlers.ListGuarantorRequests)
				r.Post("/{id}/guarantee", s.Handlers.RespondAsGuarantor)
			})
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))
				r.Get("/", s.Handlers.ListLoans)
				r.Get("/settings", s.Handlers.GetLoanSettings)
				r.Put("/settings", s.Handlers.UpdateLoanSettings)
				r.Get("/{id}/eligibility", s.Handlers.LoanEligibility)
				r.Post("/{id}/approve", s.Handlers.ApproveLoan)
				r.Post("/{id}/reject", s.Handlers.RejectLoan)
				r.Post("/{id}/disbursement", s.Handlers.RecordLoanDisbursement)
			})
		})

		r.Route("/campaigns", func(r chi.Router) {
			r.Use(s.Factory.Middleware.RequireAuth)
			r.With(s.Factory.Middleware.RequireRole(constants.RoleAdmin)).Post("/", s.Handlers.CreateCampaign)
			r.With(s.Factory.Middleware.RequireRole(constants.RoleAdmin)).Put("/{id}", s.Handlers.UpdateCampaign)
			r.With(s.Factory.Middleware.RequireRole(constants.RoleAdmin)).Patch("/{id}/status", s.Handlers.ChangeCampaignStatus)
			r.With(s.Factory.Middleware.RequireRole(constants.RoleAdmin)).Put("/{id}/resolution", s.Handlers.ResolveCampaign)
			r.With(s.Factory.Middleware.RequireRole(constants.RoleAdmin)).Patch("/{id}/refund-status", s.Handlers.SetCampaignRefundStatus)
			// The service allows admins and active members, and scopes contribution history.
			r.Get("/", s.Handlers.ListCampaigns)
			r.Get("/{id}", s.Handlers.GetCampaign)
			r.Get("/{id}/contributions", s.Handlers.CampaignContributions)
		})

		r.Route("/fines", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))
				r.Post("/", s.Handlers.CreateFine)
				r.Get("/", s.Handlers.ListFines)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.Factory.Middleware.RequireAuth)
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))
				r.Use(s.Factory.Middleware.RequireActiveMember)
				r.Post("/{id}/pay", s.Handlers.PayFine)
				r.Get("/me", s.Handlers.ListFines)
			})
		})

		// Paystack calls this; it authenticates with the X-Paystack-Signature header.
		r.Post("/webhooks/paystack", s.Handlers.PaystackWebhook)

		r.Route("/payments", func(r chi.Router) {
			r.Use(s.Factory.Middleware.RequireAuth)

			r.Group(func(r chi.Router) {
				// Pending members may pay the membership fee; the service requires an
				// active membership for every other purpose.
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleMember))

				r.Post("/", s.Handlers.CreatePayment)
				r.Get("/quote", s.Handlers.PaymentQuote)
				r.Get("/{reference}", s.Handlers.GetPayment)
			})

			r.Group(func(r chi.Router) {
				// Admin recovery for checkouts whose webhook never arrived.
				r.Use(s.Factory.Middleware.RequireRole(constants.RoleAdmin))

				r.Get("/", s.Handlers.ListPayments)
				r.Post("/{reference}/verify", s.Handlers.VerifyPayment)
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
