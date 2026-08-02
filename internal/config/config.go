// Package config loads runtime configuration from the process environment and
// the .env file. It aggregates Jira auth, engine selection, chart buffer
// size, and path resolution fallbacks into a single AppConfig value.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mcs-mcp/internal/chartbuf"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/paths"

	"github.com/rs/zerolog/log"
)

// AppConfig holds the complete application configuration.
type AppConfig struct {
	Jira                    jira.Config
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

// Load assembles the configuration from the environment already populated by
// paths.Bootstrap. Precedence is process environment, then the .env files, then
// the defaults declared in configVars.
func Load(boot paths.Result) (*AppConfig, error) {
	dataPath := boot.DataPath

	logDir := filepath.Join(dataPath, "logs")
	cacheDir := filepath.Join(dataPath, "cache")

	// Ensure directories exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Warn().Err(err).Str("path", logDir).Msg("Failed to create log directory")
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Warn().Err(err).Str("path", cacheDir).Msg("Failed to create cache directory")
	}

	delaySecs := getEnvInt("JIRA_REQUEST_DELAY_SECONDS", defaultRequestDelay)

	chartsBufferSize := getEnvInt("MCS_CHARTS_BUFFER_SIZE", defaultChartsBuffer)
	if chartsBufferSize > chartbuf.MaxBufferSize {
		return nil, fmt.Errorf("MCS_CHARTS_BUFFER_SIZE=%d exceeds maximum %d", chartsBufferSize, chartbuf.MaxBufferSize)
	}

	userEmail := getEnv("JIRA_USER_EMAIL", "")
	tokenType, tokenTypeInferred := resolveTokenType(userEmail)

	cfg := &AppConfig{
		Jira: jira.Config{
			BaseURL:      getEnv("JIRA_URL", ""),
			XsrfToken:    getEnv("JIRA_XSRF_TOKEN", ""),
			SessionID:    getEnv("JIRA_SESSION_ID", ""),
			RememberMe:   getEnv("JIRA_REMEMBERME_COOKIE", ""),
			Token:        getEnv("JIRA_TOKEN", ""),
			TokenType:    tokenType,
			UserEmail:    userEmail,
			GCILB:        getEnv("JIRA_GCILB", ""),
			GCLB:         getEnv("JIRA_GCLB", ""),
			RequestDelay: time.Duration(delaySecs) * time.Second,
		},
		DataPath:                dataPath,
		LogDir:                  logDir,
		CacheDir:                cacheDir,
		CommitmentBackflowReset: getEnvBool("COMMITMENT_POINT_BACKFLOW_RESET_CLOCK", defaultBackflowReset),
		Engine:                  getEnv("MCS_ENGINE", defaultEngine),
		EngineWeights: map[string]int{
			"crude": getEnvInt("MCS_ENGINE_CRUDE", defaultEngineCrude),
			"bbak":  getEnvInt("MCS_ENGINE_BBAK", defaultEngineBBAK),
		},
		ChartsBufferSize: chartsBufferSize,

		IngestionUpdatedLookback: getEnvInt("INGESTION_UPDATED_LOOKBACK", defaultUpdatedLookback),
		IngestionCreatedLookback: getEnvInt("INGESTION_CREATED_LOOKBACK", defaultCreatedLookback),
		IngestionMaxItems:        getEnvInt("INGESTION_MAX_ITEMS", defaultMaxItems),
	}

	// An explicit "api" without an e-mail cannot produce Basic auth, so the
	// client falls through to Bearer and Jira Cloud answers 401. Say so here
	// rather than leaving the user to decode an unexplained rejection.
	if cfg.Jira.TokenType == "api" && userEmail == "" && cfg.Jira.Token != "" {
		log.Warn().Msg("JIRA_TOKEN_TYPE=api requires JIRA_USER_EMAIL; " +
			"falling back to Bearer auth, which Jira Cloud will reject with 401")
	}

	logProvenance(boot, cfg, tokenTypeInferred)

	return cfg, nil
}

// resolveTokenType honours an explicit JIRA_TOKEN_TYPE and otherwise infers it
// from the presence of an account e-mail: Cloud API tokens need one, Data
// Center PATs do not. Reports whether the value was inferred.
func resolveTokenType(userEmail string) (string, bool) {
	raw, explicit := lookupEnv("JIRA_TOKEN_TYPE")
	if explicit {
		switch tt := strings.ToLower(raw); tt {
		case "api", "pat":
			return tt, false
		default:
			log.Warn().Str("value", raw).
				Msg("JIRA_TOKEN_TYPE must be \"api\" or \"pat\"; inferring from JIRA_USER_EMAIL instead")
		}
	}
	if userEmail != "" {
		return "api", true
	}
	return defaultTokenType, true
}

// logProvenance records where each configuration variable came from. Config
// resolves across several layers on machines that cannot be inspected, so the
// source of a surprising value has to be recoverable from the log alone.
// Secrets are reported as present/absent only - never the value, never a
// prefix, never a length, since length fingerprints the token type.
func logProvenance(boot paths.Result, cfg *AppConfig, tokenTypeInferred bool) {
	log.Info().
		Str("key", "DATA_PATH").
		Str("source", string(boot.DataPathSource)).
		Str("value", cfg.DataPath).
		Msg("config")

	for _, v := range configVars {
		if v.Key == "DATA_PATH" {
			continue // resolved during bootstrap, reported above
		}

		source := "default"
		switch {
		case v.Key == "JIRA_TOKEN_TYPE" && tokenTypeInferred:
			source = "inferred"
		case boot.EnvKeys[v.Key]:
			source = "env"
		case os.Getenv(v.Key) != "":
			source = "dotenv"
		}

		e := log.Info().Str("key", v.Key).Str("source", source)
		if v.Secret {
			e.Bool("present", os.Getenv(v.Key) != "")
		} else {
			e.Str("value", effectiveValue(v, cfg))
		}
		e.Msg("config")
	}
}

// effectiveValue reports the value actually in force, which for parsed or
// inferred variables is the one on AppConfig rather than the raw string.
func effectiveValue(v configVar, cfg *AppConfig) string {
	switch v.Key {
	case "JIRA_TOKEN_TYPE":
		return cfg.Jira.TokenType
	case "JIRA_REQUEST_DELAY_SECONDS":
		return strconv.Itoa(int(cfg.Jira.RequestDelay.Seconds()))
	case "MCS_CHARTS_BUFFER_SIZE":
		return strconv.Itoa(cfg.ChartsBufferSize)
	case "MCS_ENGINE":
		return cfg.Engine
	case "MCS_ENGINE_CRUDE":
		return strconv.Itoa(cfg.EngineWeights["crude"])
	case "MCS_ENGINE_BBAK":
		return strconv.Itoa(cfg.EngineWeights["bbak"])
	case "COMMITMENT_POINT_BACKFLOW_RESET_CLOCK":
		return strconv.FormatBool(cfg.CommitmentBackflowReset)
	case "INGESTION_UPDATED_LOOKBACK":
		return strconv.Itoa(cfg.IngestionUpdatedLookback)
	case "INGESTION_CREATED_LOOKBACK":
		return strconv.Itoa(cfg.IngestionCreatedLookback)
	case "INGESTION_MAX_ITEMS":
		return strconv.Itoa(cfg.IngestionMaxItems)
	}
	if value, ok := lookupEnv(v.Key); ok {
		return value
	}
	return v.Default
}

// lookupEnv returns the trimmed value and whether the key is set to a
// non-empty value. A key present but empty (or whitespace) counts as unset,
// which matters because .env files commonly ship keys with empty values.
func lookupEnv(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func getEnv(key, fallback string) string {
	if value, ok := lookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := lookupEnv(key); ok {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
		log.Warn().Str("key", key).Str("value", value).Int("fallback", fallback).
			Msg("Invalid integer in configuration; using default")
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if value, ok := lookupEnv(key); ok {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
		log.Warn().Str("key", key).Str("value", value).Bool("fallback", fallback).
			Msg("Invalid boolean in configuration; using default")
	}
	return fallback
}
