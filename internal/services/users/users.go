package users

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/Jidetireni/ara-cooperative.git/internal/config"
	"github.com/Jidetireni/ara-cooperative.git/internal/constants"
	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative.git/internal/services"
	"github.com/Jidetireni/ara-cooperative.git/pkg/token"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

var (
	_ UserRepository = (*repository.UserRepository)(nil)
	_ RoleRepository = (*repository.RoleRepository)(nil)
)

var (
	_ TokenService = (*token.Jwt)(nil)
)

type UserRepository interface {
	Get(ctx context.Context, filter repository.UserRepositoryFilter) (*repository.User, error)
	Upsert(ctx context.Context, user *repository.User, tx *sqlx.Tx) (*repository.User, error)
}

type RoleRepository interface {
	GetRoleByPermission(ctx context.Context, permission string) (*repository.Role, error)
	AssignRolesToUser(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID, tx *sqlx.Tx) error
}

type TokenService interface {
	GenerateTokenPair(params *token.TokenPairParams) (*token.TokenPair, error)
}

type User struct {
	DB           *sqlx.DB
	Config       *config.Config
	TokenService TokenService
	UserRepo     UserRepository
	RoleRepo     RoleRepository
}

func New(db *sqlx.DB, cfg *config.Config, tokenService TokenService, userRepo UserRepository, roleRepo RoleRepository) *User {
	return &User{
		DB:           db,
		Config:       cfg,
		TokenService: tokenService,
		UserRepo:     userRepo,
		RoleRepo:     roleRepo,
	}
}

func (u *User) SignUp(ctx context.Context, w http.ResponseWriter, input *dto.SignUpInput) (*dto.AuthResponse, error) {
	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{
		Email: &input.Email,
	})
	if err != nil {
		return nil, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "user does not exist",
		}
	}

	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return &dto.AuthResponse{}, err
	}
	defer tx.Rollback()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	upsertUser, err := u.UserRepo.Upsert(ctx, &repository.User{
		ID:    user.ID,
		Email: user.Email,
		PasswordHash: sql.NullString{
			String: string(hashedPassword),
			Valid:  true,
		},
	}, tx)
	if err != nil {
		return nil, err
	}

	defaultPermissions := []string{
		string(constants.MemberReadOwnPermission),
		string(constants.MemberWriteOwnPermission),
		string(constants.LedgerReadOwnPermission),
		string(constants.LoanApplyPermission),
	}

	roleIDs := make([]uuid.UUID, 0, len(defaultPermissions))
	for _, permission := range defaultPermissions {
		role, err := u.RoleRepo.GetRoleByPermission(ctx, permission)
		if err != nil {
			return nil, err
		}
		roleIDs = append(roleIDs, role.ID)
	}

	err = u.RoleRepo.AssignRolesToUser(ctx, upsertUser.ID, roleIDs, tx)
	if err != nil {
		return nil, err
	}

	tokenPairs, err := u.TokenService.GenerateTokenPair(&token.TokenPairParams{
		ID:      upsertUser.ID,
		Email:   upsertUser.Email,
		Roles:   defaultPermissions,
		JwtType: token.JWTTypeMember,
	})
	if err != nil {
		return nil, err
	}

	// TODO save refresh token to database for later useS

	if err := u.SetJWTCookie(w, tokenPairs.AccessToken, tokenPairs.RefreshToken, token.JWTTypeMember); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &dto.AuthResponse{
		User: &dto.AuthUser{
			ID:    upsertUser.ID,
			Email: upsertUser.Email,
			Roles: defaultPermissions,
		},
		AccessToken:  "",
		RefreshToken: "",
	}, nil
}
