package services

import (
	"net/http"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
)

type APIError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Errors  any    `json:"errors,omitempty"`
}

func (a *APIError) Error() string {
	return a.Message
}

func AdminForbiddenError(permissions []constants.UserPermissions) *APIError {
	rp := make([]string, len(permissions))
	for i, p := range permissions {
		rp[i] = string(p)
	}

	return &APIError{
		Status:  http.StatusForbidden,
		Message: "admin permissions required: " + strings.Join(rp, ", "),
	}
}

func UnauthenticatedError() *APIError {
	return &APIError{
		Status:  http.StatusUnauthorized,
		Message: "unauthenticated",
	}
}
