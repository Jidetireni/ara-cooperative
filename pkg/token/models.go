package token

import (
	"time"

	"github.com/google/uuid"
)

type JWTType string

const (
	AccessTokenExpirationTime          = time.Minute * 15    // 15 minutes
	RefreshTokenExpirationTime         = time.Hour * 24 * 7  // 7 days
	RefreshTokenExpirationTimeForAdmin = time.Hour * 24 * 14 // 30 days

	RefreshTokenName = "refresh_token"
	AccessTokenName  = "access_token"

	JWTTypeMember = "member"
	JWTTypeAdmin  = "admin"
)

type CreatetokenParams struct {
	ID       uuid.UUID
	Email    string
	Roles    []string
	JwtType  JWTType
	Duration time.Duration
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type TokenPairParams struct {
	ID      uuid.UUID
	Email   string
	Roles   []string
	JwtType JWTType
}

type TokenExpirationConfig struct {
	AccessTokenExpiry       time.Duration
	RefreshTokenExpiry      time.Duration
	AdminRefreshTokenExpiry time.Duration
}
