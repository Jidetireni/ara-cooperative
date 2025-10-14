package members

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/helpers"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/pkg/email"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

var (
	_ MemberRepository = (*repository.MemberRepository)(nil)
	_ UserRepository   = (*repository.UserRepository)(nil)
	_ RoleRepository   = (*repository.RoleRepository)(nil)
	_ TokenRepository  = (*repository.TokenRepository)(nil)
)

var (
	_ EmailPkg = (*email.Email)(nil)
)

type MemberRepository interface {
	Create(ctx context.Context, member *repository.Member, tx *sqlx.Tx) (*repository.Member, error)
	Exists(ctx context.Context, filter repository.MemberRepositoryFilter) (bool, error)
	Get(ctx context.Context, filter repository.MemberRepositoryFilter) (*repository.Member, error)
}

type UserRepository interface {
	Create(ctx context.Context, user *repository.User, tx *sqlx.Tx) (*repository.User, error)
	Exists(ctx context.Context, filter repository.UserRepositoryFilter) (bool, error)
}

type RoleRepository interface {
	List(ctx context.Context, filter *repository.RoleRepositoryFilter) ([]repository.Role, error)
	AssignToUser(ctx context.Context, userID *uuid.UUID, roleIDs []uuid.UUID, tx *sqlx.Tx) error
}

type TokenRepository interface {
	Create(ctx context.Context, token *repository.Token, tx *sqlx.Tx) (*repository.Token, error)
}

type EmailPkg interface {
	Send(ctx context.Context, input *email.SendEmailInput) error
}

type Member struct {
	DB               *sqlx.DB
	Config           *config.Config
	MemberRepository MemberRepository
	UserRepository   UserRepository
	RoleRepository   RoleRepository
	TokenRepo        TokenRepository
	Email            EmailPkg
}

func New(db *sqlx.DB, config *config.Config, memberRepo MemberRepository, userRepo UserRepository, roleRepo RoleRepository, tokenRepo TokenRepository, emailPkg EmailPkg) *Member {
	return &Member{
		DB:               db,
		Config:           config,
		MemberRepository: memberRepo,
		UserRepository:   userRepo,
		RoleRepository:   roleRepo,
		TokenRepo:        tokenRepo,
		Email:            emailPkg,
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
		PasswordHash: sql.NullString{
			String: "",
			Valid:  false,
		},
	}, tx)
	if err != nil {
		return &dto.Member{}, err
	}

	memberSlug := strings.ToLower(fmt.Sprintf("ara%06d", helpers.GetNextMemberNumber()))
	member, err := m.MemberRepository.Create(ctx, &repository.Member{
		UserID:    user.ID,
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

	tokenStr := lo.RandomString(12, lo.AlphanumericCharset)
	tokenID := base64.URLEncoding.EncodeToString([]byte(tokenStr))
	expiresAt := time.Now().Add(15 * time.Minute)
	_, err = m.TokenRepo.Create(ctx, &repository.Token{
		UserID:    user.ID,
		Token:     tokenID,
		TokenType: token.SetPasswordToken,
		IsValid:   true,
		ExpiresAt: expiresAt,
	}, tx)
	if err != nil {
		return nil, err
	}

	// TODO: send email with a stort live url to set password and sign up
	// e.g http://localhost:5000/api/v1/set-password?token=some-token
	setPasswordURL := fmt.Sprintf("%s/set-password?token=%s", m.Config.Server.FEURL, tokenID)
	body := fmt.Sprintf("click here to set password: %s\n", setPasswordURL)

	err = m.Email.Send(ctx, &email.SendEmailInput{
		To:      input.Email,
		Subject: "Set your password",
		Body:    body,
	})
	if err != nil {
		return nil, err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return &dto.Member{}, err
	}

	return m.mapRepositoryToHandler(member), nil

}

func (m *Member) GetBySlug(ctx context.Context, slug string) (*dto.Member, error) {
	member, err := m.MemberRepository.Get(ctx, repository.MemberRepositoryFilter{
		Slug: &slug,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &svc.ApiError{
				Status:  http.StatusNotFound,
				Message: "Member not found",
			}
		}
		return nil, err
	}

	return m.mapRepositoryToHandler(member), nil
}

func (m *Member) IsMemberActive(ctx context.Context, memberID uuid.UUID) (bool, error) {
	member, err := m.MemberRepository.Get(ctx, repository.MemberRepositoryFilter{
		ID: &memberID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, &svc.ApiError{
				Status:  http.StatusNotFound,
				Message: "member not found",
			}
		}
		return false, err
	}

	isActive := false
	if member.ActivatedAt.Valid {
		isActive = true
	}

	return isActive, nil
}

func (m *Member) mapRepositoryToHandler(member *repository.Member) *dto.Member {
	isActive := false
	if member.ActivatedAt.Valid {
		isActive = true
	}
	return &dto.Member{
		ID:             member.ID,
		FirstName:      member.FirstName,
		LastName:       member.LastName,
		Slug:           member.Slug,
		Address:        member.Address.String,
		NextOfKinName:  member.NextOfKinName.String,
		NextOfKinPhone: member.NextOfKinPhone.String,
		IsActive:       isActive,
	}
}
