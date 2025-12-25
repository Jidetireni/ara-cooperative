package factory

import (
	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/middleware"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/internal/services/members"
	"github.com/Jidetireni/ara-cooperative/internal/services/transactions"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"

	"github.com/Jidetireni/ara-cooperative/pkg/database"
	emailpkg "github.com/Jidetireni/ara-cooperative/pkg/email"
	"github.com/Jidetireni/ara-cooperative/pkg/logger"
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
}

type Services struct {
	Member       *members.Member
	User         *users.User
	Transactions *transactions.Transaction
}

type Packages struct {
	DB     *database.PostgresDB
	Email  *emailpkg.Email
	JWTTok *token.Jwt
	Logger *logger.Logger
}

type Factory struct {
	Router       *chi.Mux
	Pkgs         *Packages
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

	logger := logger.New(*cfg)

	userRepo := repository.NewUserRepository(db.DB)
	memberRepo := repository.NewMemberRepository(db.DB)
	roleRepo := repository.NewRoleRepository(db.DB)
	permissionRepo := repository.NewPermissionRepository(db.DB)
	tokenRepo := repository.NewTokenRepository(db.DB)
	transactionRepo := repository.NewTransactionRepository(db.DB)
	shareRepo := repository.NewShareRepository(db.DB)
	fineRepo := repository.NewFineRepository(db.DB)

	membersService := members.New(
		db.DB,
		cfg,
		memberRepo,
		userRepo,
		roleRepo,
		permissionRepo,
		tokenRepo,
		email,
	)

	usersService := users.New(
		db.DB,
		cfg,
		jwtToken,
		userRepo,
		roleRepo,
		permissionRepo,
		tokenRepo,
	)

	transactionService := transactions.New(
		db.DB,
		transactionRepo,
		memberRepo,
		shareRepo,
		fineRepo,
	)

	middleware := middleware.New(jwtToken, logger)

	return &Factory{
			Router: chi.NewRouter(),
			Pkgs: &Packages{
				DB:     db,
				Email:  email,
				JWTTok: jwtToken,
				Logger: logger,
			},
			Services: &Services{
				Member:       membersService,
				User:         usersService,
				Transactions: transactionService,
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
			},
			Middleware: middleware,
		}, func() {
			cleanup()
		}, nil
}
