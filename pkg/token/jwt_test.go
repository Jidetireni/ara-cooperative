package token_test

import (
	"testing"
	"time"

	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestJwtValidateToken(t *testing.T) {
	t.Parallel()
	svc := token.NewJwt("test-signing-key", false)
	userID := uuid.New()
	pair, err := svc.GenerateTokenPair(&token.TokenPairParams{
		ID:    userID,
		Email: "member@example.com",
		Roles: []string{"member"},
	})
	if err != nil {
		t.Fatal(err)
	}

	sign := func(method jwt.SigningMethod, claims jwt.MapClaims, secret string) string {
		t.Helper()
		raw, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	validClaims := jwt.MapClaims{
		"id": userID, "type": token.AccessTokenType, "exp": time.Now().Add(time.Hour).Unix(),
	}
	tests := []struct {
		name string
		raw  string
		ok   bool
	}{
		{name: "access token", raw: pair.AccessToken, ok: true},
		{name: "refresh token cannot authenticate", raw: pair.RefreshToken},
		{name: "malformed", raw: "not-a-token"},
		{name: "wrong signing key", raw: sign(jwt.SigningMethodHS256, validClaims, "different-key")},
		{name: "wrong signing algorithm", raw: sign(jwt.SigningMethodHS512, validClaims, "test-signing-key")},
		{name: "expired", raw: sign(jwt.SigningMethodHS256, jwt.MapClaims{
			"id": userID, "type": token.AccessTokenType, "exp": time.Now().Add(-time.Hour).Unix(),
		}, "test-signing-key")},
		{name: "expiration required", raw: sign(jwt.SigningMethodHS256, jwt.MapClaims{
			"id": userID, "type": token.AccessTokenType,
		}, "test-signing-key")},
		{name: "legacy untyped token rejected", raw: sign(jwt.SigningMethodHS256, jwt.MapClaims{
			"id": userID, "exp": time.Now().Add(time.Hour).Unix(),
		}, "test-signing-key")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			claims, err := svc.ValidateToken(tt.raw)
			if !tt.ok {
				if err == nil {
					t.Fatal("expected token rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if claims.ID != userID || claims.Type != token.AccessTokenType {
				t.Fatalf("unexpected authenticated identity: %+v", claims)
			}
		})
	}
}
