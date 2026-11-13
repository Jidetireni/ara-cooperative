package factory

import (
	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/middleware"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/internal/services/banks"
	"github.com/Jidetireni/ara-cooperative/internal/services/campaigns"
	"github.com/Jidetireni/ara-cooperative/internal/services/loans"
	"github.com/Jidetireni/ara-cooperative/internal/services/members"
	"github.com/Jidetireni/ara-cooperative/internal/services/payments"
	"github.com/Jidetireni/ara-cooperative/internal/services/transactions"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"

	"github.com/Jidetireni/ara-cooperative/pkg/cloudinary"
	"github.com/Jidetireni/ara-cooperative/pkg/database"
	emailpkg "github.com/Jidetireni/ara-cooperative/pkg/email"
	"github.com/Jidetireni/ara-cooperative/pkg/logger"
	"github.com/Jidetireni/ara-cooperative/pkg/paystack"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/go-chi/chi/v5"
)

type Repositories struct {
	Member      *repository.MemberRepository
	User        *repository.UserRepository
	Role        *repository.RoleRepository
	Permission  *repository.PermissionRepository
	Token       *repository.TokenRepository
	Transaction *repository.TransactionRepository
	Share       *repository.ShareRepository
	Fine        *repository.FineRepository
	Payment     *repository.PaymentRepository
	Campaign    *repository.CampaignRepository
	Loan        *repository.LoanRepository
}

type Services struct {
	Member       *members.Member
	User         *users.User
	Transactions *transactions.Transaction
	Payments     *payments.Payment
	Campaigns    *campaigns.Campaign
	Loans        *loans.Loan
	Banks        *banks.Banks
}

type Packages struct {
	DB       *database.PostgresDB
	Email    *emailpkg.Email
	JWTTok   *token.Jwt
	Logger   *logger.Logger
	Paystack *paystack.Client
}

type Factory struct {
	Router       *chi.Mux
	Pkgs         *Packages
	Services     *Services
	Repositories *Repositories
	Middleware   *middleware.Middleware
}

func New(cfg *config.Config) (*Factory, func(), error) {
	db, dbCleanUp, err := database.New(cfg.Database.URL, cfg.Database.Type)
	if err != nil {
		return nil, nil, err
	}

	jwtToken := token.NewJwt(cfg.Auth.JWTSecret, cfg.IsDev)

	email, err := emailpkg.New(cfg)
	if err != nil {
		return nil, nil, err
	}

	logger, err := logger.New(*cfg)
	if err != nil {
		dbCleanUp()
		return nil, nil, err
	}

	userRepo := repository.NewUserRepository(db.DB)
	memberRepo := repository.NewMemberRepository(db.DB)
	roleRepo := repository.NewRoleRepository(db.DB)
	permissionRepo := repository.NewPermissionRepository(db.DB)
	tokenRepo := repository.NewTokenRepository(db.DB)
	transactionRepo := repository.NewTransactionRepository(db.DB)
	shareRepo := repository.NewShareRepository(db.DB)
	fineRepo := repository.NewFineRepository(db.DB)
	paymentRepo := repository.NewPaymentRepository(db.DB)
	campaignRepo := repository.NewCampaignRepository(db.DB)
	loanRepo := repository.NewLoanRepository(db.DB)
	paystackClient := paystack.New(cfg.Paystack.SecretKey, cfg.Paystack.BaseURL)
	if !paystackClient.Enabled() {
		logger.Sugar.Warnw("PAYSTACK_SECRET_KEY is not set; online payments are disabled")
	} else if cfg.Paystack.OwnerSubaccount == "" {
		logger.Sugar.Warnw("PAYSTACK_OWNER_SUBACCOUNT is not set; new online checkouts are disabled")
	}

	membersService := members.New(
		db.DB,
		cfg,
		memberRepo,
		userRepo,
		roleRepo,
		permissionRepo,
		tokenRepo,
		email,
		cloudinary.New(cfg.Cloudinary.CloudName, cfg.Cloudinary.APIKey, cfg.Cloudinary.APISecret),
		logger,
	)

	usersService := users.New(
		db.DB,
		cfg,
		jwtToken,
		userRepo,
		roleRepo,
		permissionRepo,
		tokenRepo,
		memberRepo,
	)

	transactionService := transactions.New(
		db.DB,
		transactionRepo,
		memberRepo,
		shareRepo,
		fineRepo,
		cfg.Shares.UnitPrice,
		logger,
	)

	paymentsService := payments.New(
		db.DB,
		cfg,
		paymentRepo,
		memberRepo,
		userRepo,
		transactionRepo,
		shareRepo,
		fineRepo,
		campaignRepo,
		transactionService,
		paystackClient,
		logger,
	)

	banksService := banks.New(paystackClient, logger)

	middleware := middleware.New(jwtToken, logger, usersService)

	return &Factory{
			Router: chi.NewRouter(),
			Pkgs: &Packages{
				DB:       db,
				Email:    email,
				JWTTok:   jwtToken,
				Logger:   logger,
				Paystack: paystackClient,
			},
			Services: &Services{
				Member:       membersService,
				User:         usersService,
				Transactions: transactionService,
				Payments:     paymentsService,
				Campaigns:    campaigns.New(db.DB, campaignRepo, memberRepo, paymentRepo),
				Loans: loans.New(db.DB, loanRepo, memberRepo, transactionRepo, shareRepo, fineRepo, banksService,
					email, cfg.Server.FEURL, logger),
				Banks: banksService,
			},
			Repositories: &Repositories{
				Member:      memberRepo,
				User:        userRepo,
				Role:        roleRepo,
				Permission:  permissionRepo,
				Token:       tokenRepo,
				Transaction: transactionRepo,
				Share:       shareRepo,
				Fine:        fineRepo,
				Payment:     paymentRepo,
				Campaign:    campaignRepo,
				Loan:        loanRepo,
			},
			Middleware: middleware,
		}, func() {
			dbCleanUp()
			if err := logger.Sync(); err != nil {
				logger.Sugar.Errorw("failed to flush logs", "error", err)
			}
		}, nil
}
