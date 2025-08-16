package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type ServerConfig struct {
	Env  string
	Port string
}

type DataBaseConfig struct {
	URL  string
	Type string
}

type Config struct {
	Server   ServerConfig
	Database DataBaseConfig
}

func validateEnv() {
	environmentVariables := []string{
		// server
		"ENV",
		"PORT",
		// database
		"DB_URL",
		"DB_TYPE",
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
			Env:  os.Getenv("ENV"),
			Port: os.Getenv("PORT"),
		},
		Database: DataBaseConfig{
			URL:  os.Getenv("DB_URL"),
			Type: os.Getenv("DB_TYPE"),
		},
	}
}
