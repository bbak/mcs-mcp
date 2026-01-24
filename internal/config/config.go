package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"mcs-mcp/internal/jira"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

// AppConfig holds the complete application configuration.
type AppConfig struct {
	Jira jira.Config
}

// Load loads the configuration from .env files and environment variables.
func Load() (*AppConfig, error) {
	// 1. Try to load from the executable's directory (highest priority for MCP servers)
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		envPath := filepath.Join(exeDir, ".env")
		if err := godotenv.Load(envPath); err == nil {
			log.Debug().Str("path", envPath).Msg("Loaded configuration from binary directory")
		}
	}

	// 2. Fallback to current working directory (useful for development/go run)
	if err := godotenv.Load(); err != nil {
		log.Debug().Msg("No .env file found in working directory, relying on environment variables or binary-relative .env")
	}

	delaySecs, _ := strconv.Atoi(getEnv("JIRA_REQUEST_DELAY_SECONDS", "10"))

	cfg := &AppConfig{
		Jira: jira.Config{
			BaseURL:      getEnv("JIRA_URL", ""),
			XsrfToken:    getEnv("JIRA_XSRF_TOKEN", ""),
			SessionID:    getEnv("JIRA_SESSION_ID", ""),
			RememberMe:   getEnv("JIRA_REMEMBERME_COOKIE", ""),
			GCILB:        getEnv("JIRA_GCILB", ""),
			GCLB:         getEnv("JIRA_GCLB", ""),
			RequestDelay: time.Duration(delaySecs) * time.Second,
		},
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
