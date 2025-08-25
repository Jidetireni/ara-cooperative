package members

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/Jidetireni/ara-cooperative.git/internal/config"
	"github.com/Jidetireni/ara-cooperative.git/internal/constants"
	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
	"github.com/Jidetireni/ara-cooperative.git/internal/helpers"
	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative.git/internal/services"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

var (
	_ MemberRepository = (*repository.MemberRepository)(nil)
	_ UserRepository   = (*repository.UserRepository)(nil)
	_ RoleRepository   = (*repository.RoleRepository)(nil)
)

type MemberRepository interface {
	Create(ctx context.Context, member *repository.Member, tx *sqlx.Tx) (*repository.Member, error)
	Exists(ctx context.Context, filter repository.MemberRepositoryFilter) (bool, error)
}

type UserRepository interface {
	Create(ctx context.Context, user *repository.User, tx *sqlx.Tx) (*repository.User, error)
	Exists(ctx context.Context, filter repository.UserRepositoryFilter) (bool, error)
}

type RoleRepository interface {
	List(ctx context.Context, filter *repository.RoleRepositoryFilter) ([]repository.Role, error)
	AssignToUser(ctx context.Context, userID *uuid.UUID, roleIDs []uuid.UUID, tx *sqlx.Tx) error
}

type Member struct {
	DB               *sqlx.DB
	Config           *config.Config
	MemberRepository MemberRepository
	UserRepository   UserRepository
	RoleRepository   RoleRepository
}

func New(db *sqlx.DB, config *config.Config, memberRepo MemberRepository, userRepo UserRepository, roleRepo RoleRepository) *Member {
	return &Member{
		DB:               db,
		Config:           config,
		MemberRepository: memberRepo,
		UserRepository:   userRepo,
		RoleRepository:   roleRepo,
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

	defaultPermissions := []string{
		string(constants.MemberReadOwnPermission),
		string(constants.MemberWriteOwnPermission),
		string(constants.LedgerReadOwnPermission),
		string(constants.LoanApplyPermission),
	}

	roles, err := m.RoleRepository.List(ctx, &repository.RoleRepositoryFilter{
		Permission: defaultPermissions,
	})
	if err != nil {
		return &dto.Member{}, err
	}

	rolesIDs := lo.Map(roles, func(role repository.Role, _ int) uuid.UUID {
		return role.ID
	})

	err = m.RoleRepository.AssignToUser(ctx, &user.ID, rolesIDs, tx)
	if err != nil {
		return &dto.Member{}, err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return &dto.Member{}, err
	}

	// TODO: send email with a stort live url to set password and sign up
	// e.g http://localhost:5000/api/v1/set-password?token=some-token

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
