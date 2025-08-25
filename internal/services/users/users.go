package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative.git/internal/config"
	"github.com/Jidetireni/ara-cooperative.git/internal/dto"
	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative.git/internal/services"
	"github.com/Jidetireni/ara-cooperative.git/pkg/token"
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
	_ TokenService = (*token.Jwt)(nil)
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
	TokenRepo    TokenRepository
}

func New(db *sqlx.DB, cfg *config.Config, tokenService TokenService, userRepo UserRepository, roleRepo RoleRepository, tokenRepo TokenRepository) *User {
	return &User{
		DB:           db,
		Config:       cfg,
		TokenService: tokenService,
		UserRepo:     userRepo,
		RoleRepo:     roleRepo,
		TokenRepo:    tokenRepo,
	}
}

func (u *User) SetPassword(ctx context.Context, w http.ResponseWriter, input *dto.SetPasswordInput) (*dto.AuthResponse, error) {
	fmt.Printf("Setting password for email: %s\n", input.Email)
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
	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return &dto.AuthResponse{}, err
	}
	defer tx.Rollback()

	fmt.Println("Hashing password")
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

	fmt.Println("Upserting user with new password")
	upsertUser, err := u.UserRepo.Upsert(ctx, userToUpsert, tx)
	if err != nil {
		return nil, err
	}

	roles, err := u.RoleRepo.List(ctx, &repository.RoleRepositoryFilter{
		UserID: &upsertUser.ID,
	})
	if err != nil {
		return nil, err
	}

	permissions := lo.Map(roles, func(role repository.Role, _ int) string {
		return role.Permission
	})

	tokenPairs, err := u.TokenService.GenerateTokenPair(&token.TokenPairParams{
		ID:      upsertUser.ID,
		Email:   upsertUser.Email,
		Roles:   permissions,
		JwtType: token.JWTTypeMember,
	})
	if err != nil {
		return nil, err
	}

	// TODO save refresh token to database for later useS
	expiresAt := time.Now().Add(token.RefreshTokenExpirationTime)
	fmt.Println("Creating refresh token in database")
	_, err = u.TokenRepo.Create(ctx, &repository.Token{
		UserID:    upsertUser.ID,
		Token:     tokenPairs.RefreshToken,
		TokenType: token.RefreshTokenName,
		IsValid:   true,
		ExpiresAt: expiresAt,
	}, tx)
	if err != nil {
		return nil, err
	}

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
		},
		AccessToken:  "",
		RefreshToken: "",
	}, nil
}

// Login handles user authentication and token generation.
func (u *User) Login(ctx context.Context, w http.ResponseWriter, input *dto.LoginInput) (*dto.AuthResponse, error) {
	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{
		Email: &input.Email,
	})
	if err != nil {
		// Use a generic error message to prevent leaking information about whether a user exists.
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

	roles, err := u.RoleRepo.List(ctx, &repository.RoleRepositoryFilter{
		UserID: &user.ID,
	})
	if err != nil {
		return nil, err
	}

	userPermissions := lo.Map(roles, func(role repository.Role, _ int) string {
		return role.Permission
	})

	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return &dto.AuthResponse{}, err
	}
	defer tx.Rollback()

	tokenPairs, err := u.TokenService.GenerateTokenPair(&token.TokenPairParams{
		ID:      user.ID,
		Email:   user.Email,
		Roles:   userPermissions,
		JwtType: token.JWTTypeMember,
	})
	if err != nil {
		return nil, err
	}

	u.TokenRepo.Update(ctx, &repository.Token{}, tx)
	expiresAt := time.Now().Add(token.RefreshTokenExpirationTime)
	_, err = u.TokenRepo.Create(ctx, &repository.Token{
		UserID:    user.ID,
		Token:     tokenPairs.RefreshToken,
		TokenType: token.RefreshTokenName,
		IsValid:   true,
		ExpiresAt: expiresAt,
	}, tx)
	if err != nil {
		return nil, err
	}

	if err := u.SetJWTCookie(w, tokenPairs.AccessToken, tokenPairs.RefreshToken, token.JWTTypeMember); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// 8. Return the authentication response.
	return &dto.AuthResponse{
		User: &dto.AuthUser{
			ID:    user.ID,
			Email: user.Email,
		},
		AccessToken:  "",
		RefreshToken: "",
	}, nil
}
