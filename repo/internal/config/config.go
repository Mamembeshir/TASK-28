package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port            int
	DatabaseURL     string
	DatabaseURLTest string
	SessionSecret   string
	UploadPath      string
	ExportPath      string
	StatementPath   string
	EncryptionKey   string
	FacilityTZ      string
	SecureCookies   bool
}

func Load() (*Config, error) {
	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		var err error
		port, err = strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT: %w", err)
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	// Security-sensitive secrets must be provided explicitly — no insecure
	// compile-time defaults are permitted.  Startup fails fast if either is
	// absent so that misconfigured deployments are caught immediately.
	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET is required and must be set to a strong random value")
	}

	encryptionKey := os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY is required and must be set to a strong random value")
	}
	// AES-256-GCM requires exactly 32 bytes.
	if len(encryptionKey) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes (got %d)", len(encryptionKey))
	}

	// Default to secure cookies unless explicitly disabled.  Non-production
	// environments can set SECURE_COOKIES=false to allow plain HTTP.
	secureCookies := true
	if v := os.Getenv("SECURE_COOKIES"); v == "false" || v == "0" {
		secureCookies = false
	}

	cfg := &Config{
		Port:            port,
		DatabaseURL:     dbURL,
		DatabaseURLTest: os.Getenv("DATABASE_URL_TEST"),
		SessionSecret:   sessionSecret,
		UploadPath:      getEnvOrDefault("UPLOAD_PATH", "./data/uploads"),
		ExportPath:      getEnvOrDefault("EXPORT_PATH", "./data/exports"),
		StatementPath:   getEnvOrDefault("STATEMENT_PATH", "./data/statements"),
		EncryptionKey:   encryptionKey,
		FacilityTZ:      getEnvOrDefault("FACILITY_TIMEZONE", "America/New_York"),
		SecureCookies:   secureCookies,
	}

	return cfg, nil
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
