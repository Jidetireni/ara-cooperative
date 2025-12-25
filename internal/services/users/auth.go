package users

import (
	"context"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/helpers"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

func (u *User) generateUserSession(ctx context.Context, user *repository.User, tx *sqlx.Tx) (*dto.AuthResponse, string, error) {
	roles, err := u.RoleRepo.List(ctx, &repository.RoleRepositoryFilter{
		UserID: &user.ID,
	})
	if err != nil {
		return nil, "", err
	}

	roleNames := lo.Map(roles, func(r repository.Role, _ int) string { return r.Name })
	permissions, err := u.PermissionRepo.List(ctx, &repository.PermissionRepositoryFilter{
		UserID: &user.ID,
	})
	if err != nil {
		return nil, "", err
	}

	permSlugs := lo.Map(permissions, func(p repository.Permission, _ int) string { return p.Slug })
	tokenPairs, err := u.TokenPkg.GenerateTokenPair(&token.TokenPairParams{
		ID:          user.ID,
		Email:       user.Email,
		Roles:       roleNames,
		Permissions: permSlugs,
	})
	if err != nil {
		return nil, "", err
	}

	hashedRefreshToken := helpers.HashToken(tokenPairs.RefreshToken)
	expiresAt := time.Now().Add(token.RefreshTokenExpirationTime)
	_, err = u.TokenRepo.Create(ctx, &repository.Token{
		UserID:    user.ID,
		Token:     hashedRefreshToken,
		TokenType: token.RefreshTokenName,
		IsValid:   true,
		ExpiresAt: expiresAt,
	}, tx)
	if err != nil {
		return nil, "", err
	}

	return &dto.AuthResponse{
		User: &dto.AuthUser{
			ID:    user.ID,
			Email: user.Email,
		},
		AccessToken: tokenPairs.AccessToken,
	}, tokenPairs.RefreshToken, nil
}

func (u *User) SetJWTCookie(w http.ResponseWriter, refreshToken string, jwtType token.JWTType) {
	isDevelopmentMode := u.Config.IsDev
	sameSite := http.SameSiteLaxMode

	if isDevelopmentMode {
		sameSite = http.SameSiteNoneMode
	}

	refreshExpiry := token.RefreshTokenExpirationTime
	if jwtType == token.JWTTypeAdmin {
		refreshExpiry = token.RefreshTokenExpirationTimeForAdmin
	}

	refreshCookie := http.Cookie{
		Name:     token.RefreshTokenName,
		Value:    refreshToken,
		HttpOnly: true,
		Expires:  time.Now().Add(refreshExpiry),
		Secure:   true,
		SameSite: sameSite,
		Path:     "/",
	}

	http.SetCookie(w, &refreshCookie)

}
