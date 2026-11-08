package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type ServerConfig struct {
	Env              string
	Port             string
	FEURL            string
	RootUserEmail    string
	RootUserPassword string
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

type SharesConfig struct {
	UnitPrice int64
}

type PaystackConfig struct {
	// SecretKey is sk_test_... or sk_live_.... Online payments are disabled when empty.
	SecretKey string
	// BaseURL is https://api.paystack.co unless overridden (tests).
	BaseURL string
	// CallbackURL is where Paystack sends the member after checkout.
	CallbackURL string
	// OwnerSubaccount (ACCT_...) receives the maintenance fee plus processing
	// charge and bears Paystack's fee. The main cooperative account keeps the principal.
	// Required for new checkouts, including when the maintenance percentage is zero.
	OwnerSubaccount string
}

type PaymentsConfig struct {
	// MaintenanceFeePercent is MAINTENANCE_FEE_PERCENT, a whole percent
	// (5 means 5%) added on top of the underlying amount.
	MaintenanceFeePercent int32
}

type Config struct {
	Server   ServerConfig
	Database DataBaseConfig
	Shares   SharesConfig
	Auth     AuthConfig
	Email    EmailConfig
	Paystack PaystackConfig
	Payments PaymentsConfig
	IsDev    bool
}

const (
	defaultPaystackBaseURL             = "https://api.paystack.co"
	defaultMaintenanceFeePercent       = 5
	DefaultShareUnitPrice        int64 = 50_000
)

func ParseShareUnitPrice(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultShareUnitPrice, nil
	}
	price, err := strconv.ParseInt(value, 10, 64)
	if err != nil || price <= 0 || strings.HasPrefix(value, "+") {
		return 0, fmt.Errorf("SHARE_UNIT_PRICE must be a positive whole number of kobo")
	}
	return price, nil
}

// ParseMaintenanceFeePercent reads a whole percent from 0 to 100; empty
// means the default.
func ParseMaintenanceFeePercent(value string) (int32, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultMaintenanceFeePercent, nil
	}
	percent, err := strconv.ParseInt(value, 10, 32)
	if err != nil || percent < 0 || percent > 100 || strings.HasPrefix(value, "+") {
		return 0, fmt.Errorf("MAINTENANCE_FEE_PERCENT must be a whole number from 0 to 100, got %q", value)
	}
	return int32(percent), nil
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
	}
	if os.Getenv("ENV") != "development" {
		environmentVariables = append(environmentVariables, "EMAIL_PASSWORD")
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

	maintenanceFee, err := ParseMaintenanceFeePercent(os.Getenv("MAINTENANCE_FEE_PERCENT"))
	if err != nil {
		log.Fatal(err)
	}
	sharePrice, err := ParseShareUnitPrice(os.Getenv("SHARE_UNIT_PRICE"))
	if err != nil {
		log.Fatal(err)
	}
	paystackBaseURL := strings.TrimRight(os.Getenv("PAYSTACK_BASE_URL"), "/")
	if paystackBaseURL == "" {
		paystackBaseURL = defaultPaystackBaseURL
	}
	callbackURL := os.Getenv("PAYSTACK_CALLBACK_URL")
	if callbackURL == "" {
		callbackURL = strings.TrimRight(os.Getenv("FE_URL"), "/") + "/payments/callback"
	}

	return &Config{
		Server: ServerConfig{
			Env:              os.Getenv("ENV"),
			Port:             os.Getenv("PORT"),
			FEURL:            os.Getenv("FE_URL"),
			RootUserEmail:    os.Getenv("ROOT_USER_EMAIL"),
			RootUserPassword: os.Getenv("ROOT_USER_PASSWORD"),
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
		Shares: SharesConfig{
			UnitPrice: sharePrice,
		},
		Paystack: PaystackConfig{
			SecretKey:       strings.TrimSpace(os.Getenv("PAYSTACK_SECRET_KEY")),
			BaseURL:         paystackBaseURL,
			CallbackURL:     callbackURL,
			OwnerSubaccount: strings.TrimSpace(os.Getenv("PAYSTACK_OWNER_SUBACCOUNT")),
		},
		Payments: PaymentsConfig{
			MaintenanceFeePercent: maintenanceFee,
		},

		IsDev: os.Getenv("ENV") == "development",
	}
}
