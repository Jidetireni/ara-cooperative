package helpers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

func GenerateOTP() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%06d", n.Int64())
}

func HashToken(token string) string {
	hasher := sha256.New()
	hasher.Write([]byte(token))
	return hex.EncodeToString(hasher.Sum(nil))
}

func GenerateSetupToken() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate setup secret: %w", err)
	}
	return hex.EncodeToString(secret), nil
}
