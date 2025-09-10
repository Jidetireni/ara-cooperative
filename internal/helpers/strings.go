package helpers

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

func GenerateRandomString(length int) string {
	allowedChars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = allowedChars[rand.Intn(len(allowedChars))]
	}
	return string(b)
}

func GenerateUniqueReference(resource string) string {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	u := uuid.NewString()
	raw := fmt.Sprintf("%s|%s|%s", resource, now, u)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
