package users

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/google/uuid"
)

type JWTType string

const (
	AccessTokenExpirationTime = time.Minute * 15 // 15 minutes

	RefreshTokenExpirationTime = time.Hour * 24 * 7 // 7 days

	RefreshTokenExpirationTimeForAdmin = time.Hour * 24 * 14 // 30 days

	RefreshTokenName = "refresh_token"
	AccessTokenName  = "access_token"
	JWTTypeMember    = "member"
	JWTTypeAdmin     = "admin"
)

func (u *User) GenerateTokenPair(userID uuid.UUID, roles []string, jwtType JWTType) (accessToken, refreshToken string, err error) {
	// Generate access token
	accessToken, err = u.generateToken(userID, roles, "access", AccessTokenExpirationTime)
	if err != nil {
		return "", "", err
	}

	// Generate refresh token with longer expiry
	var refreshExpiry time.Duration
	if jwtType == JWTTypeAdmin {
		refreshExpiry = RefreshTokenExpirationTimeForAdmin
	} else {
		refreshExpiry = RefreshTokenExpirationTime
	}

	refreshToken, err = u.generateToken(userID, roles, "refresh", refreshExpiry)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

func (u *User) generateToken(userID uuid.UUID, roles []string, jwtType JWTType, expiry time.Duration) (string, error) {
	claims := map[string]any{
		"user_id":  userID.String(),
		"roles":    roles,
		"jwt_type": jwtType,
	}

	jwtauth.SetExpiry(claims, time.Now().Add(expiry))
	_, tokenString, err := u.TokenAuth.Encode(claims)
	return tokenString, err
}

func (u *User) SetJWTCookie(ctx context.Context, userID uuid.UUID, jwtType JWTType, refreshTokenExpirationTime time.Duration) (http.Cookie, http.Cookie, error) {
	userCtx := FromContext(ctx)
	w := userCtx.Writer

	isDevelopmentMode := u.Config.IsDev
	sameSite := http.SameSiteLaxMode

	accessTokenExpirationTime := AccessTokenExpirationTime
	if isDevelopmentMode {
		sameSite = http.SameSiteNoneMode
		accessTokenExpirationTime = time.Hour * 24 * 3 // 3 days
	}

	accessToken, refreshToken, err := u.GenerateTokenPair(userID, userCtx.Roles, jwtType)
	if err != nil {
		return http.Cookie{}, http.Cookie{}, err
	}

	accessCookie := http.Cookie{
		Name:     AccessTokenName,
		Value:    accessToken,
		HttpOnly: true,
		Expires:  time.Now().Add(accessTokenExpirationTime),
		Secure:   true,
		SameSite: sameSite,
		Path:     "/",
	}

	refreshCookie := http.Cookie{
		Name:     RefreshTokenName,
		Value:    refreshToken,
		HttpOnly: true,
		Expires:  time.Now().Add(refreshTokenExpirationTime),
		Secure:   true,
		SameSite: sameSite,
		Path:     "/",
	}

	http.SetCookie(w, &accessCookie)
	http.SetCookie(w, &refreshCookie)

	return accessCookie, refreshCookie, nil
}
