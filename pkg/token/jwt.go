package token

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Jwt struct {
	SecretKey string
	IsDev     bool
}

func NewJwt(secretKey string, isDev bool) *Jwt {
	return &Jwt{
		SecretKey: secretKey,
		IsDev:     isDev,
	}
}

func (j *Jwt) createToken(params *CreatetokenParams) (string, *UserClaims, error) {
	claims, err := newUserClaims(params)
	if err != nil {
		return "", nil, err
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(j.SecretKey))
	if err != nil {
		return "", nil, err
	}

	return tokenString, claims, nil
}

func (j *Jwt) ValidateToken(tokenString string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("invalid token singing method")
		}
		return []byte(j.SecretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token claims")
}

func (j *Jwt) GenerateTokenPair(params *TokenPairParams) (*TokenPair, error) {

	accessExpiry := AccessTokenExpirationTime
	if j.IsDev {
		accessExpiry = time.Hour * 24 * 1 // 1 day
	}

	accessToken, _, err := j.createToken(&CreatetokenParams{
		ID:          params.ID,
		Email:       params.Email,
		Roles:       params.Roles,
		Permissions: params.Permissions,
		Duration:    accessExpiry,
	})
	if err != nil {
		return nil, err
	}

	refreshExpiry := RefreshTokenExpirationTime
	refreshToken, _, err := j.createToken(&CreatetokenParams{
		ID:          params.ID,
		Email:       params.Email,
		Roles:       params.Roles,
		Permissions: params.Permissions,
		Duration:    refreshExpiry,
	})
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
