package config

import (
	"encoding/base64"
	"errors"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const MaxJSONBytes int64 = 1 << 20

type Config struct {
	DatabaseURL               string
	JWTSecret                 string
	JWTIssuer                 string
	QRPepper                  string
	DemoAccessCodeHash        string
	DemoSignupEnabled         bool
	EmailVerificationRequired bool
	PublicAppURL              string
	MailProvider              string
	MailFromAddress           string
	MailFromName              string
	SMTPHost                  string
	SMTPPort                  int
	SMTPUsername              string
	SMTPPassword              string
	SMTPTLSMode               string
	MailPollInterval          time.Duration
	OutboxEncryptionKey       []byte
	AppVersion                string
	GitCommit                 string
	ExpectedSchemaVersion     string
	Port                      string
	TrustedProxyCount         int
	ReadTimeout               time.Duration
	WriteTimeout              time.Duration
	IdleTimeout               time.Duration
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"), JWTSecret: os.Getenv("JWT_SECRET"), JWTIssuer: envDefault("JWT_ISSUER", "puntazo"),
		QRPepper: os.Getenv("QR_PEPPER"), DemoAccessCodeHash: os.Getenv("DEMO_ACCESS_CODE_HASH"),
		DemoSignupEnabled: envBool("DEMO_SIGNUP_ENABLED", false), AppVersion: envDefault("APP_VERSION", "dev"),
		EmailVerificationRequired: envBool("EMAIL_VERIFICATION_REQUIRED", false), PublicAppURL: envDefault("PUBLIC_APP_URL", "http://localhost:8081"),
		MailProvider: strings.ToLower(envDefault("MAIL_PROVIDER", "disabled")), MailFromAddress: strings.TrimSpace(os.Getenv("MAIL_FROM_ADDRESS")), MailFromName: envDefault("MAIL_FROM_NAME", "Puntazo"),
		SMTPHost: strings.TrimSpace(os.Getenv("SMTP_HOST")), SMTPPort: envInt("SMTP_PORT", 587), SMTPUsername: os.Getenv("SMTP_USERNAME"), SMTPPassword: os.Getenv("SMTP_PASSWORD"), SMTPTLSMode: strings.ToLower(envDefault("SMTP_TLS_MODE", "starttls")), MailPollInterval: time.Duration(envInt("MAIL_POLL_INTERVAL_SECONDS", 5)) * time.Second,
		GitCommit: envDefault("GIT_COMMIT", "0000000"), ExpectedSchemaVersion: envDefault("EXPECTED_SCHEMA_VERSION", "0011"),
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
	appURL, err := url.Parse(c.PublicAppURL)
	if err != nil || appURL.Host == "" || (appURL.Scheme != "http" && appURL.Scheme != "https") {
		return Config{}, errors.New("PUBLIC_APP_URL must be an absolute http(s) URL")
	}
	production := strings.EqualFold(os.Getenv("APP_ENV"), "production")
	if encodedKey := strings.TrimSpace(os.Getenv("OUTBOX_ENCRYPTION_KEY")); encodedKey != "" {
		c.OutboxEncryptionKey, err = base64.StdEncoding.DecodeString(encodedKey)
		if err != nil || len(c.OutboxEncryptionKey) != 32 {
			return Config{}, errors.New("OUTBOX_ENCRYPTION_KEY must be base64 for exactly 32 bytes")
		}
	}
	if production && !c.EmailVerificationRequired {
		return Config{}, errors.New("EMAIL_VERIFICATION_REQUIRED=true is required in production")
	}
	if production && (appURL.Scheme != "https" || appURL.Hostname() == "" || appURL.User != nil || appURL.RawQuery != "" || appURL.Fragment != "") {
		return Config{}, errors.New("PUBLIC_APP_URL must be a clean HTTPS origin in production")
	}
	if c.EmailVerificationRequired && len(c.OutboxEncryptionKey) != 32 {
		return Config{}, errors.New("OUTBOX_ENCRYPTION_KEY is required while email verification is required")
	}
	if production && (string(c.OutboxEncryptionKey) == c.JWTSecret || string(c.OutboxEncryptionKey) == c.QRPepper) {
		return Config{}, errors.New("a separate 32-byte OUTBOX_ENCRYPTION_KEY is required in production")
	}
	if c.MailProvider != "disabled" && c.MailProvider != "smtp" {
		return Config{}, errors.New("MAIL_PROVIDER must be disabled or smtp")
	}
	if c.EmailVerificationRequired && c.MailProvider != "smtp" {
		return Config{}, errors.New("MAIL_PROVIDER=smtp is required while email verification is required")
	}
	if c.MailProvider == "smtp" {
		if c.MailFromAddress == "" || c.SMTPHost == "" || c.SMTPPort < 1 || c.SMTPPort > 65535 {
			return Config{}, errors.New("MAIL_FROM_ADDRESS, SMTP_HOST and valid SMTP_PORT are required")
		}
		if c.SMTPTLSMode != "starttls" && c.SMTPTLSMode != "tls" {
			return Config{}, errors.New("SMTP_TLS_MODE must be starttls or tls")
		}
		if (c.SMTPUsername == "") != (c.SMTPPassword == "") {
			return Config{}, errors.New("SMTP_USERNAME and SMTP_PASSWORD must be configured together")
		}
		from, mailErr := mail.ParseAddress(c.MailFromAddress)
		if mailErr != nil || from.Address != c.MailFromAddress || strings.ContainsAny(c.MailFromName, "\r\n") {
			return Config{}, errors.New("mail sender identity is invalid")
		}
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
