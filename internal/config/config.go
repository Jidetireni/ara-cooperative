package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type ServerConfig struct {
	Env   string
	Port  string
	FEURL string
}

type DataBaseConfig struct {
	URL  string
	Type string
}

type AuthConfig struct {
	JWTSecret string
}

type EmailConfig struct {
	Password string
}

type Config struct {
	Server   ServerConfig
	Database DataBaseConfig
	Auth     AuthConfig
	Email    EmailConfig
	IsDev    bool
}

func validateEnv() {
	environmentVariables := []string{
		// server
		"ENV",
		"PORT",
		"FE_URL",
		// database
		"DB_URL",
		"DB_TYPE",
		// auth
		"JWT_SECRET",
		// email
		"EMAIL_PASSWORD",
	}
	for _, env := range environmentVariables {
		if os.Getenv(env) == "" {
			log.Fatalf("Environment variable %s is not set", env)
		}
	}

}

func New() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	validateEnv()

	return &Config{
		Server: ServerConfig{
			Env:   os.Getenv("ENV"),
			Port:  os.Getenv("PORT"),
			FEURL: os.Getenv("FE_URL"),
		},
		Database: DataBaseConfig{
			URL:  os.Getenv("DB_URL"),
			Type: os.Getenv("DB_TYPE"),
		},
		Auth: AuthConfig{
			JWTSecret: os.Getenv("JWT_SECRET"),
		},
		Email: EmailConfig{
			Password: os.Getenv("EMAIL_PASSWORD"),
		},

		IsDev: os.Getenv("ENV") == "development",
	}
}
