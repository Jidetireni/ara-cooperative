package factory

import (
	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/middleware"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/internal/services/members"
	"github.com/Jidetireni/ara-cooperative/internal/services/savings"
	"github.com/Jidetireni/ara-cooperative/internal/services/shares"
	"github.com/Jidetireni/ara-cooperative/internal/services/transactions"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"

	"github.com/Jidetireni/ara-cooperative/pkg/database"
	emailpkg "github.com/Jidetireni/ara-cooperative/pkg/email"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/go-chi/chi/v5"
)

type Repositories struct {
	Member      *repository.MemberRepository
	User        *repository.UserRepository
	Role        *repository.RoleRepository
	Token       *repository.TokenRepository
	Transaction *repository.TransactionRepository
	Shares      *repository.ShareRepository
}

type Services struct {
	Member       *members.Member
	User         *users.User
	Savings      *savings.Saving
	Transactions *transactions.Transaction
	Shares       *shares.Shares
}

type Factory struct {
	DB           *database.PostgresDB
	JWTToken     *token.Jwt
	Email        *emailpkg.Email
	Router       *chi.Mux
	Services     *Services
	Repositories *Repositories
	Middleware   *middleware.Middleware
}

func New(cfg *config.Config) (*Factory, func(), error) {
	db, cleanup, err := database.New(cfg.Database.URL, cfg.Database.Type)
	if err != nil {
		return nil, nil, err
	}

	jwtToken := token.NewJwt(cfg.Auth.JWTSecret, cfg.IsDev)

	email, err := emailpkg.New(cfg)
	if err != nil {
		return nil, nil, err
	}

	userRepo := repository.NewUserRepository(db.DB)
	memberRepo := repository.NewMemberRepository(db.DB)
	roleRepo := repository.NewRoleRepository(db.DB)
	tokenRepo := repository.NewTokenRepository(db.DB)
	transactionRepo := repository.NewTransactionRepository(db.DB)
	shareRepo := repository.NewShareRepository(db.DB)

	membersService := members.New(
		db.DB,
		cfg,
		memberRepo,
		userRepo,
		roleRepo,
		tokenRepo,
		email,
	)

	usersService := users.New(
		db.DB,
		cfg,
		jwtToken,
		userRepo,
		roleRepo,
		tokenRepo,
	)

	savingsService := savings.New(
		db.DB,
		transactionRepo,
		memberRepo,
	)

	transactionService := transactions.New(
		db.DB,
		transactionRepo,
	)

	sharesService := shares.New(
		db.DB,
		shareRepo,
		memberRepo,
		transactionRepo,
	)

	middleware := middleware.New(jwtToken)

	return &Factory{
			DB:       db,
			JWTToken: jwtToken,
			Router:   chi.NewRouter(),
			Email:    email,
			Services: &Services{
				Member:       membersService,
				User:         usersService,
				Savings:      savingsService,
				Transactions: transactionService,
				Shares:       sharesService,
			},
			Repositories: &Repositories{
				Member:      memberRepo,
				User:        userRepo,
				Role:        roleRepo,
				Token:       tokenRepo,
				Transaction: transactionRepo,
			},
			Middleware: middleware,
		}, func() {
			cleanup()
		}, nil
}
