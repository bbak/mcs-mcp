// Package config loads runtime configuration from the process environment and
// the .env file. It aggregates Jira auth, engine selection, chart buffer
// size, and path resolution fallbacks into a single AppConfig value.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"mcs-mcp/internal/chartbuf"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/paths"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

// AppConfig holds the complete application configuration.
type AppConfig struct {
	Jira                jira.Config
	DataPath                string
	LogDir                  string
	CacheDir                string
	CommitmentBackflowReset bool           // Reset WIP age clock on backflow past the commitment point
	Engine                  string         // MCS_ENGINE: "crude" (default), "bbak", "auto"
	EngineWeights           map[string]int // MCS_ENGINE_<NAME>: 0 = disabled, 1-100 = weight
	ChartsBufferSize        int            // MCS_CHARTS_BUFFER_SIZE: 0 = disabled, 1-100 = enabled

	IngestionUpdatedLookback int // INGESTION_UPDATED_LOOKBACK (months) for initial hydration JQL
	IngestionCreatedLookback int // INGESTION_CREATED_LOOKBACK (months) for initial hydration JQL
	IngestionMaxItems        int // INGESTION_MAX_ITEMS — page-cap for initial hydration
}

// Load loads the configuration from .env files and environment variables.
func Load() (*AppConfig, error) {
	// 1. Try to load from the executable's directory (highest priority for MCP servers)
	exePath, err := os.Executable()
	exeDir := ""
	if err == nil {
		exeDir = filepath.Dir(exePath)
		envPath := filepath.Join(exeDir, ".env")
		if err := godotenv.Load(envPath); err == nil {
			log.Debug().Str("path", envPath).Msg("Loaded configuration from binary directory")
		}
	}

	// 2. Fallback to current working directory (useful for development/go run)
	if err := godotenv.Load(); err != nil {
		log.Debug().Msg("No .env file found in working directory, relying on environment variables or binary-relative .env")
	}

	// 3. Resolve Data Paths (with fallback chain for read-only installations)
	dataPath := paths.ResolveDataPath(exeDir)

	logDir := filepath.Join(dataPath, "logs")
	cacheDir := filepath.Join(dataPath, "cache")

	// Ensure directories exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Warn().Err(err).Str("path", logDir).Msg("Failed to create log directory")
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Warn().Err(err).Str("path", cacheDir).Msg("Failed to create cache directory")
	}

	delayRaw := getEnv("JIRA_REQUEST_DELAY_SECONDS", "10")
	delaySecs, err := strconv.Atoi(delayRaw)
	if err != nil {
		log.Warn().Str("value", delayRaw).Msg("JIRA_REQUEST_DELAY_SECONDS is not a valid integer; defaulting to 10")
		delaySecs = 10
	}

	chartsBufferSize := getEnvInt("MCS_CHARTS_BUFFER_SIZE", 0)
	if chartsBufferSize > chartbuf.MaxBufferSize {
		return nil, fmt.Errorf("MCS_CHARTS_BUFFER_SIZE=%d exceeds maximum %d", chartsBufferSize, chartbuf.MaxBufferSize)
	}

	cfg := &AppConfig{
		Jira: jira.Config{
			BaseURL:      getEnv("JIRA_URL", ""),
			XsrfToken:    getEnv("JIRA_XSRF_TOKEN", ""),
			SessionID:    getEnv("JIRA_SESSION_ID", ""),
			RememberMe:   getEnv("JIRA_REMEMBERME_COOKIE", ""),
			Token:        getEnv("JIRA_TOKEN", ""),
			TokenType:    getEnv("JIRA_TOKEN_TYPE", "pat"),
			UserEmail:    getEnv("JIRA_USER_EMAIL", ""),
			GCILB:        getEnv("JIRA_GCILB", ""),
			GCLB:         getEnv("JIRA_GCLB", ""),
			RequestDelay: time.Duration(delaySecs) * time.Second,
		},
		DataPath:                dataPath,
		LogDir:                  logDir,
		CacheDir:                cacheDir,
		CommitmentBackflowReset: getEnvBool("COMMITMENT_POINT_BACKFLOW_RESET_CLOCK", true),
		Engine:                  getEnv("MCS_ENGINE", "crude"),
		EngineWeights: map[string]int{
			"crude": getEnvInt("MCS_ENGINE_CRUDE", 50),
			"bbak":  getEnvInt("MCS_ENGINE_BBAK", 50),
		},
		ChartsBufferSize: chartsBufferSize,

		IngestionUpdatedLookback: getEnvInt("INGESTION_UPDATED_LOOKBACK", 24),
		IngestionCreatedLookback: getEnvInt("INGESTION_CREATED_LOOKBACK", 36),
		IngestionMaxItems:        getEnvInt("INGESTION_MAX_ITEMS", 5000),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return fallback
}
