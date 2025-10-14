package users

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"golang.org/x/crypto/bcrypt"
)

var (
	_ UserRepository  = (*repository.UserRepository)(nil)
	_ RoleRepository  = (*repository.RoleRepository)(nil)
	_ TokenRepository = (*repository.TokenRepository)(nil)
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
	DB        *sqlx.DB
	Config    *config.Config
	TokenPkg  TokenPkg
	UserRepo  UserRepository
	RoleRepo  RoleRepository
	TokenRepo TokenRepository
}

func New(db *sqlx.DB, cfg *config.Config, tokenPkg TokenPkg, userRepo UserRepository, roleRepo RoleRepository, tokenRepo TokenRepository) *User {
	return &User{
		DB:        db,
		Config:    cfg,
		TokenPkg:  tokenPkg,
		UserRepo:  userRepo,
		RoleRepo:  roleRepo,
		TokenRepo: tokenRepo,
	}
}

func (u *User) SetPassword(ctx context.Context, w http.ResponseWriter, input *dto.SetPasswordInput) (*dto.AuthResponse, error) {
	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return &dto.AuthResponse{}, err
	}
	defer tx.Rollback()

	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{
		Email: &input.Email,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &svc.ApiError{
				Status:  http.StatusBadRequest,
				Message: "Invalid email",
			}
		}
		return nil, err
	}

	// TODO validate the token sent through the url,
	// this is to ensure that only users with valid tokens can set their passwords.
	// then invalidate the token after use.
	isValid, err := u.TokenRepo.Validate(ctx, &repository.TokenRepositoryFilter{
		UserID:    &user.ID,
		Token:     &input.Token,
		TokenType: lo.ToPtr(string(token.SetPasswordToken)),
		IsValid:   lo.ToPtr(true),
		IsExpired: lo.ToPtr(false),
		IsDeleted: lo.ToPtr(false),
	})
	if err != nil {
		return nil, err
	}
	if !isValid {
		return nil, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "Invalid or expired token",
		}
	}

	// invalidate token
	err = u.TokenRepo.Update(ctx, &repository.Token{
		UserID:    user.ID,
		TokenType: token.SetPasswordToken,
		IsValid:   false,
	}, tx)
	if err != nil {
		return nil, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	userToUpsert := &repository.User{
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
	}

	upsertUser, err := u.UserRepo.Upsert(ctx, userToUpsert, tx)
	if err != nil {
		return nil, err
	}

	tokenPairs, err := u.generateTokenAndSave(ctx, w, upsertUser, tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &dto.AuthResponse{
		User: &dto.AuthUser{
			ID:    upsertUser.ID,
			Email: upsertUser.Email,
		},
		AccessToken:  tokenPairs.AccessToken,
		RefreshToken: tokenPairs.RefreshToken,
	}, nil
}

// Login handles user authentication and token generation.
func (u *User) Login(ctx context.Context, w http.ResponseWriter, input *dto.LoginInput) (*dto.AuthResponse, error) {
	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{
		Email: &input.Email,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &svc.ApiError{
				Status:  http.StatusUnauthorized,
				Message: "invalid email or password",
			}
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), []byte(input.Password)); err != nil {
		return nil, &svc.ApiError{
			Status:  http.StatusUnauthorized,
			Message: "invalid email or password",
		}
	}

	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	tokenPairs, err := u.generateTokenAndSave(ctx, w, user, tx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Return the authentication response.
	return &dto.AuthResponse{
		User: &dto.AuthUser{
			ID:    user.ID,
			Email: user.Email,
		},
		AccessToken:  tokenPairs.AccessToken,
		RefreshToken: tokenPairs.RefreshToken,
	}, nil
}

func (u *User) RefreshToken(ctx context.Context, w http.ResponseWriter, refreshToken string) (bool, error) {
	_, err := u.TokenPkg.ValidateToken(refreshToken)
	if err != nil {
		return false, &svc.ApiError{
			Status:  http.StatusUnauthorized,
			Message: "Invalid refresh token",
		}
	}

	validatedToken, err := u.TokenRepo.Get(ctx, &repository.TokenRepositoryFilter{
		Token:     &refreshToken,
		IsValid:   lo.ToPtr(true),
		IsExpired: lo.ToPtr(false),
		IsDeleted: lo.ToPtr(false),
		TokenType: lo.ToPtr(token.RefreshTokenName),
	})
	if err != nil {
		return false, err
	}

	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{
		ID: &validatedToken.UserID,
	})
	if err != nil {
		return false, err
	}

	_, err = u.generateTokenAndSave(ctx, w, user, tx)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	return true, nil
}
