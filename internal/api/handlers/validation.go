package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/go-playground/validator/v10"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (h *Handlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		h.errorResponse(w, r, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("invalid request body: %v", err),
		})
		return false
	}

	if err := h.validate.Struct(dst); err != nil {
		var validationErrors []ValidationError
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			for _, fe := range ve {
				validationErrors = append(validationErrors, ValidationError{
					Field:   fe.Field(),
					Message: fe.Translate(h.trans),
				})
			}
		}

		h.errorResponse(w, r, &svc.APIError{
			Status:  http.StatusBadRequest,
			Message: "Input validation failed",
			Errors:  validationErrors,
		})
		return false
	}

	return true
}
