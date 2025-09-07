package services

import (
	"net/http"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/constants"
)

type ApiError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func (a *ApiError) Error() string {
	return a.Message
}

func AdminForbiddenError(permissions []constants.UserPermissions) *ApiError {
	rp := make([]string, len(permissions))
	for i, p := range permissions {
		rp[i] = string(p)
	}

	return &ApiError{
		Status:  http.StatusForbidden,
		Message: "admin permissions required: " + strings.Join(rp, ", "),
	}
}
