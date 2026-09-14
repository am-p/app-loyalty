package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

const MaxJSONBytes int64 = 1 << 20

type Config struct {
	DatabaseURL           string
	JWTSecret             string
	JWTIssuer             string
	QRPepper              string
	DemoAccessCodeHash    string
	DemoSignupEnabled     bool
	AppVersion            string
	GitCommit             string
	ExpectedSchemaVersion string
	Port                  string
	TrustedProxyCount     int
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"), JWTSecret: os.Getenv("JWT_SECRET"), JWTIssuer: envDefault("JWT_ISSUER", "puntazo"),
		QRPepper: os.Getenv("QR_PEPPER"), DemoAccessCodeHash: os.Getenv("DEMO_ACCESS_CODE_HASH"),
		DemoSignupEnabled: envBool("DEMO_SIGNUP_ENABLED", false), AppVersion: envDefault("APP_VERSION", "dev"),
		GitCommit: envDefault("GIT_COMMIT", "0000000"), ExpectedSchemaVersion: envDefault("EXPECTED_SCHEMA_VERSION", "0006"),
		Port: envDefault("PORT", "8080"), TrustedProxyCount: envInt("TRUSTED_PROXY_COUNT", 0),
		ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 32 {
		return Config{}, errors.New("JWT_SECRET must be at least 32 bytes")
	}
	if c.JWTIssuer == "" {
		return Config{}, errors.New("JWT_ISSUER must not be empty")
	}
	if len(c.QRPepper) < 32 {
		return Config{}, errors.New("QR_PEPPER must be at least 32 bytes")
	}
	if c.JWTSecret == c.QRPepper {
		return Config{}, errors.New("JWT_SECRET and QR_PEPPER must differ")
	}
	if c.DemoSignupEnabled && c.DemoAccessCodeHash == "" {
		return Config{}, errors.New("DEMO_ACCESS_CODE_HASH is required while demo signup is enabled")
	}
	if !fourDigits(c.ExpectedSchemaVersion) {
		return Config{}, errors.New("EXPECTED_SCHEMA_VERSION must have four digits")
	}
	return c, nil
}

func fourDigits(value string) bool {
	if len(value) != 4 {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return v
}

func envInt(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v < 0 {
		return fallback
	}
	return v
}
