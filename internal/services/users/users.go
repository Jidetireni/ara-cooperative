package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/helpers"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"golang.org/x/crypto/bcrypt"
)

var (
	_ UserRepository       = (*repository.UserRepository)(nil)
	_ RoleRepository       = (*repository.RoleRepository)(nil)
	_ PermissionRepository = (*repository.PermissionRepository)(nil)
	_ TokenRepository      = (*repository.TokenRepository)(nil)
	_ MemberRepository     = (*repository.MemberRepository)(nil)
)

var (
	_ TokenPkg = (*token.Jwt)(nil)
)

type UserRepository interface {
	Get(ctx context.Context, filter repository.UserRepositoryFilter, tx *sqlx.Tx) (*repository.User, error)
	Upsert(ctx context.Context, user *repository.User, tx *sqlx.Tx) (*repository.User, error)
}

type MemberRepository interface {
	Get(ctx context.Context, filter repository.MemberRepositoryFilter, tx *sqlx.Tx) (*repository.Member, error)
	MapRepositoryToDTOModel(member *repository.Member) *dto.Member
}

type RoleRepository interface {
	List(ctx context.Context, filter *repository.RoleRepositoryFilter) ([]repository.Role, error)
	AssignToUser(ctx context.Context, userID *uuid.UUID, roleIDs []uuid.UUID, tx *sqlx.Tx) error
	RevokeFromUser(ctx context.Context, userID *uuid.UUID, roleIDs []uuid.UUID, tx *sqlx.Tx) error
}

type PermissionRepository interface {
	List(ctx context.Context, filter *repository.PermissionRepositoryFilter) ([]repository.Permission, error)
}

type TokenRepository interface {
	Create(ctx context.Context, token *repository.Token, tx *sqlx.Tx) (*repository.Token, error)
	Update(ctx context.Context, token *repository.Token, tx *sqlx.Tx) error
	Get(ctx context.Context, filter *repository.TokenRepositoryFilter, tx *sqlx.Tx) (*repository.Token, error)
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
	MemberRepo     MemberRepository
}

func New(
	db *sqlx.DB,
	cfg *config.Config,
	tokenPkg TokenPkg,
	userRepo UserRepository,
	roleRepo RoleRepository,
	permissionRepo PermissionRepository,
	tokenRepo TokenRepository,
	memberRepo MemberRepository,
) *User {
	return &User{
		DB:             db,
		Config:         cfg,
		TokenPkg:       tokenPkg,
		UserRepo:       userRepo,
		RoleRepo:       roleRepo,
		PermissionRepo: permissionRepo,
		TokenRepo:      tokenRepo,
		MemberRepo:     memberRepo,
	}
}

func (u *User) SetPassword(
	ctx context.Context, input *dto.SetPasswordInput,
) (response *dto.AuthResponse, refresh string, err error) {
	if utf8.RuneCountInString(input.Password) < 8 || len(input.Password) > 72 {
		return nil, "", &svc.APIError{Status: http.StatusBadRequest,
			Message: "password must contain at least 8 characters and at most 72 bytes"}
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("hash initial password: %w", err)
	}
	tx, err := u.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("begin password setup: %w", err)
	}
	defer func() {
		// A committed transaction has already released its resources.
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback password setup: %w", rollbackErr))
		}
	}()
	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{Email: &input.Email, ForUpdate: true}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", &svc.APIError{Status: http.StatusBadRequest, Message: "invalid or expired token"}
	}
	if err != nil {
		return nil, "", fmt.Errorf("load password setup account: %w", err)
	}
	if user.PasswordHash.Valid && user.PasswordHash.String != "" {
		return nil, "", &svc.APIError{Status: http.StatusBadRequest, Message: "invalid or expired token"}
	}
	tokenHash := helpers.HashToken(input.Token)
	storedToken, err := u.TokenRepo.Get(ctx, &repository.TokenRepositoryFilter{
		UserID: &user.ID, Token: &tokenHash, TokenType: lo.ToPtr(token.SetPasswordToken), ForUpdate: true,
		IsValid: lo.ToPtr(true), IsDeleted: lo.ToPtr(false), IsExpired: lo.ToPtr(false),
	}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", &svc.APIError{Status: http.StatusBadRequest, Message: "invalid or expired token"}
	}
	if err != nil {
		return nil, "", fmt.Errorf("load password setup token: %w", err)
	}
	if err := u.TokenRepo.Update(ctx, &repository.Token{
		ID: storedToken.ID, UserID: user.ID, TokenType: token.SetPasswordToken, IsValid: false,
	}, tx); err != nil {
		return nil, "", fmt.Errorf("consume password setup token: %w", err)
	}
	upsertUser, err := u.UserRepo.Upsert(ctx, &repository.User{
		ID: user.ID, Email: user.Email,
		PasswordHash:     sql.NullString{String: string(hashedPassword), Valid: true},
		EmailConfirmedAt: sql.NullTime{Time: time.Now(), Valid: true},
	}, tx)
	if err != nil {
		return nil, "", fmt.Errorf("save initial password: %w", err)
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
	input.Email = strings.TrimSpace(input.Email)
	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{
		Email: &input.Email,
	}, nil)
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
	}, nil)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, "", fmt.Errorf("load refresh token: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) || storedToken.ExpiresAt.Before(time.Now()) {
		return nil, "", &svc.APIError{
			Status:  http.StatusUnauthorized,
			Message: "invalid or expired refresh token",
		}
	}

	err = u.TokenRepo.Update(ctx, &repository.Token{
		ID:        storedToken.ID,
		TokenType: token.RefreshTokenName,
		IsValid:   false,
	}, tx)
	if err != nil {
		return nil, "", err
	}

	user, err := u.UserRepo.Get(ctx, repository.UserRepositoryFilter{
		ID: &storedToken.UserID,
	}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", svc.UnauthenticatedError()
	}
	if err != nil {
		return nil, "", fmt.Errorf("load refresh account: %w", err)
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
