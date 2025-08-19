package token

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type UserClaims struct {
	ID      uuid.UUID `json:"id"`
	Email   string    `json:"email"`
	Roles   []string  `json:"roles"`
	JwtType JWTType   `json:"jwt_type"`
	jwt.RegisteredClaims
}

func NewUserClaims(params *CreatetokenParams) (*UserClaims, error) {
	tokenID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	return &UserClaims{
		ID:      params.ID,
		Email:   params.Email,
		Roles:   params.Roles,
		JwtType: params.JwtType,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID.String(),
			Subject:   params.Email,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(params.Duration)),
		},
	}, nil
}
