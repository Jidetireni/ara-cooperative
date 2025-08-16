package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	svc "github.com/Jidetireni/ara-cooperative.git/internal/services"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
)

var (
	validate *validator.Validate
	trans    ut.Translator
)

func init() {
	validate = validator.New()

	en := en.New()
	uni := ut.New(en, en)
	trans, _ = uni.GetTranslator("en")
	if err := en_translations.RegisterDefaultTranslations(validate, trans); err != nil {
		panic(err)
	}

}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (h *Handlers) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		h.errorResponse(w, r, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
		return false
	}

	if err := validate.Struct(dst); err != nil {
		var validationErrors []ValidationError
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, ValidationError{
				Field:   strings.ToLower(err.Field()),
				Message: err.Translate(trans),
			})
		}
		h.errorResponse(w, r, &svc.ApiError{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Validation failed: %v", validationErrors),
		})
		return false
	}

	return true
}

func (h *Handlers) validateField(fieldName, value, constraint string) error {
	err := validate.Var(value, constraint)
	if err != nil {
		validationErrors := err.(validator.ValidationErrors)
		return fmt.Errorf("%s: %s", fieldName, validationErrors[0].Translate(trans))
	}
	return nil
}
