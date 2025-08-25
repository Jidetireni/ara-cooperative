package factory

import (
	"github.com/Jidetireni/ara-cooperative.git/internal/config"
	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
	"github.com/Jidetireni/ara-cooperative.git/internal/services/members"
	"github.com/Jidetireni/ara-cooperative.git/internal/services/users"

	"github.com/Jidetireni/ara-cooperative.git/pkg/database"
	"github.com/Jidetireni/ara-cooperative.git/pkg/token"
	"github.com/go-chi/chi"
)

type Repositories struct {
	Member *repository.MemberRepository
	User   *repository.UserRepository
	Role   *repository.RoleRepository
	Token  *repository.TokenRepository
}

type Services struct {
	Member *members.Member
	User   *users.User
}

type Factory struct {
	DB           *database.PostgresDB
	JWTToken     *token.Jwt
	Router       *chi.Mux
	Services     *Services
	Repositories *Repositories
}

func New(cfg *config.Config) (*Factory, func(), error) {
	db, cleanup, err := database.New(cfg.Database.URL, cfg.Database.Type)
	if err != nil {
		return nil, nil, err
	}

	jwtToken := token.NewJwt(cfg.Auth.JWTSecret)

	userRepo := repository.NewUserRepository(db.DB)
	memberRepo := repository.NewMemberRepository(db.DB)
	roleRepo := repository.NewRoleRepository(db.DB)
	tokenRepo := repository.NewTokenRepository(db.DB)

	membersService := members.New(
		db.DB,
		cfg,
		memberRepo,
		userRepo,
		roleRepo,
	)

	usersService := users.New(
		db.DB,
		cfg,
		jwtToken,
		userRepo,
		roleRepo,
		tokenRepo,
	)

	return &Factory{
			DB:       db,
			JWTToken: jwtToken,
			Router:   chi.NewRouter(),
			Services: &Services{
				Member: membersService,
				User:   usersService,
			},
			Repositories: &Repositories{
				Member: memberRepo,
				User:   userRepo,
				Role:   roleRepo,
				Token:  tokenRepo,
			},
		}, func() {
			cleanup()
		}, nil
}
