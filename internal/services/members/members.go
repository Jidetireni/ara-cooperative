package members

import (
	"context"
	"net/http"
	"strings"

	"github.com/Jidetireni/ara-cooperative.git/internal/api/handlers"
	"github.com/Jidetireni/ara-cooperative.git/internal/config"
	"github.com/Jidetireni/ara-cooperative.git/internal/helpers"
	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative.git/internal/services"
	"github.com/jmoiron/sqlx"
)

var (
	_ MemberRepository = (*repository.MemberRepository)(nil)
	_ UserRepository   = (*repository.UserRepository)(nil)
)

type MemberRepository interface {
	Create(ctx context.Context, member *repository.Member, tx *sqlx.Tx) (*repository.Member, error)
	Exists(ctx context.Context, filter repository.MemberRepositoryFilter) (bool, error)
}

type UserRepository interface {
	Exists(ctx context.Context, filter repository.UserRepositoryFilter) (bool, error)
}

type Member struct {
	DB               *sqlx.DB
	Config           *config.Config
	MemberRepository MemberRepository
	UserRepository   UserRepository
}

func New(db *sqlx.DB, config *config.Config, memberRepo MemberRepository, userRepo UserRepository) *Member {
	return &Member{
		DB:               db,
		Config:           config,
		MemberRepository: memberRepo,
		UserRepository:   userRepo,
	}
}

func (m Member) Create(ctx context.Context, input handlers.CreateMemberInput) (*handlers.Member, error) {
	emailExists, err := m.UserRepository.Exists(ctx, repository.UserRepositoryFilter{
		Email: &input.Email,
	})
	if err != nil {
		return nil, err
	}
	if emailExists {
		return nil, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "Email already exists",
		}
	}

	phoneExists, err := m.MemberRepository.Exists(ctx, repository.MemberRepositoryFilter{
		Phone: &input.Phone,
	})
	if err != nil {
		return nil, err
	}
	if phoneExists {
		return nil, &svc.ApiError{
			Status:  http.StatusConflict,
			Message: "Phone number already exists",
		}
	}

	tx, err := m.DB.BeginTxx(ctx, nil)
	if err != nil {
		return &handlers.Member{}, err
	}
	defer tx.Rollback()

	memberSlug := strings.ToLower(helpers.GenerateRandomString(8))

}
