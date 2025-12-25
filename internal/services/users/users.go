package users

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/helpers"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"golang.org/x/crypto/bcrypt"
)

var (
	_ UserRepository       = (*repository.UserRepository)(nil)
	_ RoleRepository       = (*repository.RoleRepository)(nil)
	_ PermissionRepository = (*repository.PermissionRepository)(nil)
	_ TokenRepository      = (*repository.TokenRepository)(nil)
)

var (
	_ TokenPkg = (*token.Jwt)(nil)
)

type UserRepository interface {
	Get(ctx context.Context, filter repository.UserRepositoryFilter) (*repository.User, error)
	Upsert(ctx context.Context, user *repository.User, tx *sqlx.Tx) (*repository.User, error)
}

type RoleRepository interface {
	List(ctx context.Context, filter *repository.RoleRepositoryFilter) ([]repository.Role, error)
}

type PermissionRepository interface {
	List(ctx context.Context, filter *repository.PermissionRepositoryFilter) ([]repository.Permission, error)
}

type TokenRepository interface {
	Create(ctx context.Context, token *repository.Token, tx *sqlx.Tx) (*repository.Token, error)
	Update(ctx context.Context, token *repository.Token, tx *sqlx.Tx) error
	Get(ctx context.Context, filter *repository.TokenRepositoryFilter) (*repository.Token, error)
	Validate(ctx context.Context, filter *repository.TokenRepositoryFilter) (bool, error)
}

type TokenPkg interface {
	GenerateTokenPair(params *token.TokenPairParams) (*token.TokenPair, error)
	ValidateToken(tokenString string) (*token.UserClaims, error)
}

type User struct {
	DB             *sqlx.DB
	Config         *config.Config
	TokenPkg       TokenPkg
	UserRepo       UserRepository
	RoleRepo       RoleRepository
	PermissionRepo PermissionRepository
	TokenRepo      TokenRepository
}

func New(db *sqlx.DB, cfg *config.Config, tokenPkg TokenPkg, userRepo UserRepository, roleRepo RoleRepository, permissionRepo PermissionRepository, tokenRepo TokenRepository) *User {
	return &User{
		DB:             db,
		Config:         cfg,
		TokenPkg:       tokenPkg,
		UserRepo:       userRepo,
		RoleRepo:       roleRepo,
		PermissionRepo: permissionRepo,
		TokenRepo:      tokenRepo,
	}
}

func (u *User) SetPassword(ctx context.Context, w http.ResponseWriter, input *dto.SetPasswordInput) (*dto.AuthResponse, string, error) {
	incomingTokenHash := helpers.HashToken(input.Token)
	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return &dto.AuthResponse{}, "", err
	}
	defer tx.Rollback()

	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{
		Email: &input.Email,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", &svc.APIError{
				Status:  http.StatusBadRequest,
				Message: "Invalid or expired token",
			}
		}
		return nil, "", err
	}

	storedToken, err := u.TokenRepo.Get(ctx, &repository.TokenRepositoryFilter{
		UserID:    &user.ID,
		Token:     &incomingTokenHash,
		TokenType: lo.ToPtr(string(token.SetPasswordToken)),
		IsValid:   lo.ToPtr(true),
	})
	if err != nil || storedToken.ExpiresAt.Before(time.Now()) {
		return nil, "", &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "Invalid or expired token",
		}
	}

	err = u.TokenRepo.Update(ctx, &repository.Token{
		UserID:  user.ID,
		IsValid: false,
	}, tx)
	if err != nil {
		return nil, "", err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}

	upsertUser, err := u.UserRepo.Upsert(ctx, &repository.User{
		ID:    user.ID,
		Email: input.Email,
		PasswordHash: sql.NullString{
			String: string(hashedPassword),
			Valid:  true,
		},
		EmailConfirmedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}, tx)
	if err != nil {
		return nil, "", err
	}

	dtoUser, refreshToken, err := u.generateUserSession(ctx, upsertUser, tx)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		return nil, "", err
	}

	return dtoUser, refreshToken, nil
}

// Login handles user authentication and token generation.
func (u *User) Login(ctx context.Context, input *dto.LoginInput) (*dto.AuthResponse, string, error) {
	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{
		Email: &input.Email,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", &svc.APIError{
				Status:  http.StatusUnauthorized,
				Message: "invalid email or password",
			}
		}
		return nil, "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), []byte(input.Password)); err != nil {
		return nil, "", &svc.APIError{
			Status:  http.StatusUnauthorized,
			Message: "invalid email or password",
		}
	}

	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()

	dtoUser, refreshToken, err := u.generateUserSession(ctx, user, tx)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		return nil, "", err
	}

	return dtoUser, refreshToken, nil
}

func (u *User) RefreshToken(ctx context.Context, rawRefreshToken string) (*dto.AuthResponse, string, error) {
	incomingHash := helpers.HashToken(rawRefreshToken)
	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()

	storedToken, err := u.TokenRepo.Get(ctx, &repository.TokenRepositoryFilter{
		Token:     &incomingHash,
		TokenType: lo.ToPtr(token.RefreshTokenName),
		IsValid:   lo.ToPtr(true),
		IsDeleted: lo.ToPtr(false),
	})

	if err != nil || storedToken.ExpiresAt.Before(time.Now()) {
		return nil, "", &svc.APIError{
			Status:  http.StatusUnauthorized,
			Message: "Invalid or expired refresh token",
		}
	}

	err = u.TokenRepo.Update(ctx, &repository.Token{
		ID:      storedToken.ID,
		IsValid: false,
	}, tx)
	if err != nil {
		return nil, "", err
	}

	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{
		ID: &storedToken.UserID,
	})
	if err != nil {
		return nil, "", err
	}

	authResponse, newRefreshToken, err := u.generateUserSession(ctx, user, tx)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		return nil, "", err
	}

	return authResponse, newRefreshToken, nil
}
