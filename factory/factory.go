package factory

import (
	"github.com/Jidetireni/ara-cooperative.git/internal/config"
	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
	"github.com/Jidetireni/ara-cooperative.git/internal/services/members"

	"github.com/Jidetireni/ara-cooperative.git/pkg/database"
	"github.com/go-chi/chi"
)

type Repositories struct {
	Member *repository.MemberRepository
	User   *repository.UserRepository
}

type Services struct {
	Member *members.Member
}

type Factory struct {
	DB           *database.PostgresDB
	Router       *chi.Mux
	Services     *Services
	Repositories *Repositories
}

func New(cfg *config.Config) (*Factory, func(), error) {
	db, cleanup, err := database.New(cfg.Database.URL, cfg.Database.Type)
	if err != nil {
		return nil, nil, err
	}

	userRepo := repository.NewUserRepository(db.DB)
	memberRepo := repository.NewMemberRepository(db.DB)

	membersService := members.New(
		db.DB,
		cfg,
		memberRepo,
		userRepo,
	)

	return &Factory{
			DB:     db,
			Router: chi.NewRouter(),
			Services: &Services{
				Member: membersService,
			},
			Repositories: &Repositories{
				Member: memberRepo,
				User:   userRepo,
			},
		}, func() {
			cleanup()
		}, nil
}
