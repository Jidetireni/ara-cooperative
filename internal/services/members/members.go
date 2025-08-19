package members

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/Jidetireni/ara-cooperative.git/internal/config"
	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
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
	Create(ctx context.Context, user *repository.User, tx *sqlx.Tx) (*repository.User, error)
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

func (m Member) Create(ctx context.Context, input dto.CreateMemberInput) (*dto.Member, error) {
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
		return &dto.Member{}, err
	}
	defer tx.Rollback()

	user, err := m.UserRepository.Create(ctx, &repository.User{
		Email: input.Email,
	}, tx)
	if err != nil {
		return &dto.Member{}, err
	}

	memberSlug := strings.ToLower(helpers.GenerateRandomString(8))
	memberCode := fmt.Sprintf("ARA%06d", helpers.GetNextMemberNumber())

	member, err := m.MemberRepository.Create(ctx, &repository.Member{
		UserID:    user.ID,
		Code:      memberCode,
		Slug:      memberSlug,
		FirstName: input.FirstName,
		LastName:  input.LastName,
		Phone:     input.Phone,
		Address: sql.NullString{
			String: input.Address,
			Valid:  input.Address != "",
		},
		NextOfKinName: sql.NullString{
			String: input.NextOfKinName,
			Valid:  input.NextOfKinName != "",
		},
		NextOfKinPhone: sql.NullString{
			String: input.NextOfKinPhone,
			Valid:  input.NextOfKinPhone != "",
		},
	}, tx)
	if err != nil {
		return &dto.Member{}, err
	}

	// TODO: send email

	return m.mapRepositoryToHandler(*member), nil

}

func (m *Member) mapRepositoryToHandler(member repository.Member) *dto.Member {
	return &dto.Member{
		ID:             member.ID,
		FirstName:      member.FirstName,
		LastName:       member.LastName,
		Slug:           member.Slug,
		Code:           member.Code,
		Address:        member.Address.String,
		NextOfKinName:  member.NextOfKinName.String,
		NextOfKinPhone: member.NextOfKinPhone.String,
	}
}
