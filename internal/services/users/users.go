package users

import (
	"github.com/Jidetireni/ara-cooperative.git/internal/config"
	"github.com/go-chi/jwtauth/v5"
)

type User struct {
	TokenAuth *jwtauth.JWTAuth
	Config    *config.Config
}
