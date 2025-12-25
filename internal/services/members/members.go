package members

import (
	"context"
	"database/sql"
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
	_ MemberRepository     = (*repository.MemberRepository)(nil)
	_ UserRepository       = (*repository.UserRepository)(nil)
	_ RoleRepository       = (*repository.RoleRepository)(nil)
	_ PermissionRepository = (*repository.PermissionRepository)(nil)
	_ TokenRepository      = (*repository.TokenRepository)(nil)
)

var (
	_ EmailPkg = (*email.Email)(nil)
)

type MemberRepository interface {
	Create(ctx context.Context, member *repository.Member, tx *sqlx.Tx) (*repository.Member, error)
	Get(ctx context.Context, filter repository.MemberRepositoryFilter) (*repository.Member, error)
	MapRepositoryToDTO(member *repository.Member) *dto.Member
}

type UserRepository interface {
	Create(ctx context.Context, user *repository.User, tx *sqlx.Tx) (*repository.User, error)
	Exists(ctx context.Context, filter repository.UserRepositoryFilter) (bool, error)
}

type RoleRepository interface {
	List(ctx context.Context, filter *repository.RoleRepositoryFilter) ([]repository.Role, error)
	AssignToUser(ctx context.Context, userID *uuid.UUID, roleIDs []uuid.UUID, tx *sqlx.Tx) error
}

type PermissionRepository interface {
	List(ctx context.Context, filter *repository.PermissionRepositoryFilter) ([]repository.Permission, error)
	AssignToUser(ctx context.Context, userID *uuid.UUID, permissionIDs []uuid.UUID, tx *sqlx.Tx) error
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
	PermissionRepo   PermissionRepository
	TokenRepo        TokenRepository
	Email            EmailPkg
}

func New(db *sqlx.DB, config *config.Config, memberRepo MemberRepository, userRepo UserRepository, roleRepo RoleRepository, permissionRepo PermissionRepository, tokenRepo TokenRepository, emailPkg EmailPkg) *Member {
	return &Member{
		DB:               db,
		Config:           config,
		MemberRepository: memberRepo,
		UserRepository:   userRepo,
		RoleRepository:   roleRepo,
		PermissionRepo:   permissionRepo,
		TokenRepo:        tokenRepo,
		Email:            emailPkg,
	}
}

func (m Member) Create(ctx context.Context, input dto.CreateMemberInput) (*dto.Member, error) {
	rawToken := helpers.GenerateOTP()
	tokenHash := helpers.HashToken(rawToken)

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

	err = m.AssignDefaultRoleAndPermissions(ctx, user.ID, tx)
	if err != nil {
		return &dto.Member{}, err
	}

	expiresAt := time.Now().Add(30 * time.Minute)
	_, err = m.TokenRepo.Create(ctx, &repository.Token{
		UserID:    user.ID,
		Token:     tokenHash,
		TokenType: token.SetPasswordToken,
		IsValid:   true,
		ExpiresAt: expiresAt,
	}, tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return &dto.Member{}, err
	}

	go func() {
		body := fmt.Sprintf(
			"Hello %s,\n\nYour account has been created.\nUse the following code to set your password:\n\n%s\n\nThis code expires in 15 minutes.",
			input.FirstName,
			rawToken,
		)

		err := m.Email.Send(context.Background(), &email.SendEmailInput{
			To:      input.Email,
			Subject: "Welcome! Verify your account",
			Body:    body,
		})
		if err != nil {
			// Log this error! The user exists but didn't get the code.
			// fmt.Printf("Failed to send email to %s: %v\n", input.Email, err)
		}
	}()

	return m.MemberRepository.MapRepositoryToDTO(member), nil
}

func (m *Member) GetBySlug(ctx context.Context, slug string) (*dto.Member, error) {
	member, err := m.MemberRepository.Get(ctx, repository.MemberRepositoryFilter{
		Slug: &slug,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &svc.APIError{
				Status:  http.StatusNotFound,
				Message: "Member not found",
			}
		}
		return nil, err
	}

	return m.MemberRepository.MapRepositoryToDTO(member), nil
}

func (m *Member) IsMemberActive(ctx context.Context, memberID uuid.UUID) (bool, error) {
	member, err := m.MemberRepository.Get(ctx, repository.MemberRepositoryFilter{
		ID: &memberID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, &svc.APIError{
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

func (m *Member) AssignDefaultRoleAndPermissions(ctx context.Context, userID uuid.UUID, tx *sqlx.Tx) error {
	defaultRole := []string{
		string(constants.RoleMember),
	}

	roles, err := m.RoleRepository.List(ctx, &repository.RoleRepositoryFilter{
		Name: defaultRole,
	})
	if err != nil {
		return err
	}

	roleIDs := lo.Map(roles, func(role repository.Role, _ int) uuid.UUID {
		return role.ID
	})

	err = m.RoleRepository.AssignToUser(ctx, &userID, roleIDs, tx)
	if err != nil {
		return err
	}

	defaultPermmision := []string{
		string(constants.LoanApply),
	}

	permmisions, err := m.PermissionRepo.List(ctx, &repository.PermissionRepositoryFilter{
		Slug: defaultPermmision,
	})
	if err != nil {
		return err
	}

	permissionIDs := lo.Map(permmisions, func(permission repository.Permission, _ int) uuid.UUID {
		return permission.ID
	})

	err = m.PermissionRepo.AssignToUser(ctx, &userID, permissionIDs, tx)
	if err != nil {
		return err
	}

	return nil
}
