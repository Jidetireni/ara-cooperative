package token

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type UserClaims struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	Roles       []string  `json:"roles"`
	Permissions []string  `json:"scopes"`
	jwt.RegisteredClaims
}

func newUserClaims(params *CreatetokenParams) (*UserClaims, error) {
	tokenID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	return &UserClaims{
		ID:          params.ID,
		Email:       params.Email,
		Roles:       params.Roles,
		Permissions: params.Permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID.String(),
			Subject:   params.Email,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(params.Duration)),
		},
	}, nil
}
