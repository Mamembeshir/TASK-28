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

	cfg := &Config{
		Port:            port,
		DatabaseURL:     dbURL,
		DatabaseURLTest: os.Getenv("DATABASE_URL_TEST"),
		SessionSecret:   getEnvOrDefault("SESSION_SECRET", "change-me-in-production"),
		UploadPath:      getEnvOrDefault("UPLOAD_PATH", "./data/uploads"),
		ExportPath:      getEnvOrDefault("EXPORT_PATH", "./data/exports"),
		StatementPath:   getEnvOrDefault("STATEMENT_PATH", "./data/statements"),
		EncryptionKey:   getEnvOrDefault("ENCRYPTION_KEY", "change-me-32-byte-key-here!!!!!"),
		FacilityTZ:      getEnvOrDefault("FACILITY_TIMEZONE", "America/New_York"),
	}

	return cfg, nil
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
