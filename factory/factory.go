package factory

import (
	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/middleware"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/internal/services/members"
	"github.com/Jidetireni/ara-cooperative/internal/services/savings"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"

	"github.com/Jidetireni/ara-cooperative/pkg/database"
	"github.com/Jidetireni/ara-cooperative/pkg/email"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/go-chi/chi/v5"
)

type Repositories struct {
	Member      *repository.MemberRepository
	User        *repository.UserRepository
	Role        *repository.RoleRepository
	Token       *repository.TokenRepository
	Transaction *repository.TransactionRepository
	Saving      *repository.SavingRepository
}

type Services struct {
	Member  *members.Member
	User    *users.User
	Savings *savings.Saving
}

type Factory struct {
	DB           *database.PostgresDB
	JWTToken     *token.Jwt
	Email        *email.Email
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

	email, err := email.New(cfg)
	if err != nil {
		return nil, nil, err
	}

	userRepo := repository.NewUserRepository(db.DB)
	memberRepo := repository.NewMemberRepository(db.DB)
	roleRepo := repository.NewRoleRepository(db.DB)
	tokenRepo := repository.NewTokenRepository(db.DB)
	transactionRepo := repository.NewTransactionRepository(db.DB)
	savingRepo := repository.NewSavingRepository(db.DB)

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
		savingRepo,
		transactionRepo,
		memberRepo,
	)

	middleware := middleware.New(jwtToken)

	return &Factory{
			DB:       db,
			JWTToken: jwtToken,
			Router:   chi.NewRouter(),
			Email:    email,
			Services: &Services{
				Member:  membersService,
				User:    usersService,
				Savings: savingsService,
			},
			Repositories: &Repositories{
				Member:      memberRepo,
				User:        userRepo,
				Role:        roleRepo,
				Token:       tokenRepo,
				Transaction: transactionRepo,
				Saving:      savingRepo,
			},
			Middleware: middleware,
		}, func() {
			cleanup()
		}, nil
}
