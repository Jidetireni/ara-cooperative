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

func GenerateUniqueReference(id uuid.UUID, time time.Time, resource string) string {
	idstr := id.String()
	timeStr := time.Format("2006-01-02T15:04:05")

	referenceStr := fmt.Sprintf("%s|%s|%s", idstr, timeStr, resource)
	reference := base64.RawURLEncoding.EncodeToString([]byte(referenceStr))

	return reference
}
