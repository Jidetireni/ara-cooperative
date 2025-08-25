package users

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/Jidetireni/ara-cooperative.git/internal/constants"
	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
	"github.com/Jidetireni/ara-cooperative.git/pkg/token"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

func (u *User) generateTokenAndSave(ctx context.Context, w http.ResponseWriter, user *repository.User, tx *sqlx.Tx) (*token.TokenPair, error) {
	roles, err := u.RoleRepo.List(ctx, &repository.RoleRepositoryFilter{
		UserID: &user.ID,
	})
	if err != nil {
		return nil, err
	}

	userPermissions := lo.Map(roles, func(role repository.Role, _ int) string {
		return role.Permission
	})

	jwtType := token.JWTTypeMember
	if slices.Contains(userPermissions, string(constants.RoleAssignPermission)) {
		jwtType = token.JWTTypeAdmin
	}

	tokenPairs, err := u.TokenService.GenerateTokenPair(&token.TokenPairParams{
		ID:      user.ID,
		Email:   user.Email,
		Roles:   userPermissions,
		JwtType: token.JWTType(jwtType),
	})
	if err != nil {
		return nil, err
	}

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

	return tokenPairs, nil
}

func (u *User) SetJWTCookie(w http.ResponseWriter, accessToken, refreshToken string, jwtType token.JWTType) error {
	isDevelopmentMode := u.Config.IsDev
	sameSite := http.SameSiteLaxMode

	accessTokenExpirationTime := token.AccessTokenExpirationTime
	if isDevelopmentMode {
		sameSite = http.SameSiteNoneMode
		accessTokenExpirationTime = time.Hour * 24 * 3 // 3 days
	}

	accessCookie := http.Cookie{
		Name:     token.AccessTokenName,
		Value:    accessToken,
		HttpOnly: true,
		Expires:  time.Now().Add(accessTokenExpirationTime),
		Secure:   true,
		SameSite: sameSite,
		Path:     "/",
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

	http.SetCookie(w, &accessCookie)
	http.SetCookie(w, &refreshCookie)

	return nil
}
