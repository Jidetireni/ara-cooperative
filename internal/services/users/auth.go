package users

import (
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative.git/pkg/token"
)

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
