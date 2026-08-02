package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcs-mcp/internal/paths"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// isolate clears every configuration variable for the duration of the test so
// that a developer's own .env or shell cannot influence the result. t.Setenv
// registers the restore, and it also fails the test if it is run in parallel.
func isolate(t *testing.T) {
	t.Helper()
	for _, v := range configVars {
		t.Setenv(v.Key, "")
		os.Unsetenv(v.Key)
	}
	// Bootstrap loads a .env from the working directory; run from a scratch dir
	// so the repository's own files are never picked up.
	t.Chdir(t.TempDir())
}

// bootstrapIn runs Bootstrap against a scratch data directory.
func bootstrapIn(t *testing.T, dir string) paths.Result {
	t.Helper()
	boot, _ := paths.Bootstrap(paths.Options{DataPath: dir})
	return boot
}

// captureLogs redirects the global logger into a buffer for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = previous })
	return &buf
}

func TestLoad_Defaults(t *testing.T) {
	isolate(t)
	dir := t.TempDir()

	cfg, err := Load(bootstrapIn(t, dir))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// The values the code actually falls back to.
	if got := int(cfg.Jira.RequestDelay.Seconds()); got != defaultRequestDelay {
		t.Errorf("RequestDelay = %d, want %d", got, defaultRequestDelay)
	}
	if cfg.ChartsBufferSize != defaultChartsBuffer {
		t.Errorf("ChartsBufferSize = %d, want %d", cfg.ChartsBufferSize, defaultChartsBuffer)
	}
	if cfg.Engine != defaultEngine {
		t.Errorf("Engine = %q, want %q", cfg.Engine, defaultEngine)
	}
	if cfg.EngineWeights["crude"] != defaultEngineCrude {
		t.Errorf("crude weight = %d, want %d", cfg.EngineWeights["crude"], defaultEngineCrude)
	}
	if cfg.EngineWeights["bbak"] != defaultEngineBBAK {
		t.Errorf("bbak weight = %d, want %d", cfg.EngineWeights["bbak"], defaultEngineBBAK)
	}
	if cfg.CommitmentBackflowReset != defaultBackflowReset {
		t.Errorf("CommitmentBackflowReset = %v, want %v", cfg.CommitmentBackflowReset, defaultBackflowReset)
	}
	if cfg.IngestionUpdatedLookback != defaultUpdatedLookback {
		t.Errorf("UpdatedLookback = %d, want %d", cfg.IngestionUpdatedLookback, defaultUpdatedLookback)
	}
	if cfg.IngestionCreatedLookback != defaultCreatedLookback {
		t.Errorf("CreatedLookback = %d, want %d", cfg.IngestionCreatedLookback, defaultCreatedLookback)
	}
	if cfg.IngestionMaxItems != defaultMaxItems {
		t.Errorf("MaxItems = %d, want %d", cfg.IngestionMaxItems, defaultMaxItems)
	}
}

// TestDefaults_MatchTemplate closes the divergence loop this package exists to
// prevent: the generated template and the code fallbacks must agree.
func TestDefaults_MatchTemplate(t *testing.T) {
	isolate(t)
	dir := t.TempDir()

	cfg, err := Load(bootstrapIn(t, dir))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	for _, v := range configVars {
		if v.Default == "" {
			continue // no declared default, nothing to compare
		}
		if got := effectiveValue(v, cfg); got != v.Default {
			t.Errorf("%s: code produces %q, template declares %q", v.Key, got, v.Default)
		}
	}
}

func TestEnvExampleMatchesTemplate(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join("..", "..", "conf", ".env-example"))
	if err != nil {
		t.Fatalf("read .env-example: %v", err)
	}
	// Normalize line endings: a clone with autocrlf=true checks the file out
	// with CRLF, which is not a real divergence.
	normalize := func(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }

	if !bytes.Equal(normalize(committed), normalize(Template())) {
		t.Error("conf/.env-example is out of date; regenerate it from config.Template()")
	}
}

func TestPrecedence_EnvBeatsDotenv(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("MCS_ENGINE=crude\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MCS_ENGINE", "bbak")

	cfg, err := Load(bootstrapIn(t, dir))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Engine != "bbak" {
		t.Errorf("Engine = %q, want %q (process environment must outrank .env)", cfg.Engine, "bbak")
	}
}

func TestPrecedence_DotenvBeatsDefault(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("MCS_ENGINE=bbak\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(bootstrapIn(t, dir))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Engine != "bbak" {
		t.Errorf("Engine = %q, want %q", cfg.Engine, "bbak")
	}
}

func TestTokenTypeInference(t *testing.T) {
	tests := []struct {
		name      string
		tokenType string // "" means unset
		setType   bool
		email     string
		want      string
	}{
		{name: "unset with email infers api", email: "user@example.com", want: "api"},
		{name: "unset without email infers pat", want: "pat"},
		{name: "unset with blank email infers pat", email: "   ", want: "pat"},
		{name: "present but empty counts as unset", setType: true, tokenType: "", email: "user@example.com", want: "api"},
		{name: "explicit api honoured without email", setType: true, tokenType: "api", want: "api"},
		{name: "explicit pat honoured with email", setType: true, tokenType: "pat", email: "user@example.com", want: "pat"},
		{name: "explicit uppercase normalized", setType: true, tokenType: "API", want: "api"},
		{name: "explicit padded normalized", setType: true, tokenType: "  pat  ", email: "user@example.com", want: "pat"},
		{name: "garbage falls back to inference", setType: true, tokenType: "API_TOKEN", email: "user@example.com", want: "api"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			if tc.setType {
				t.Setenv("JIRA_TOKEN_TYPE", tc.tokenType)
			}
			t.Setenv("JIRA_USER_EMAIL", tc.email)

			cfg, err := Load(bootstrapIn(t, t.TempDir()))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Jira.TokenType != tc.want {
				t.Errorf("TokenType = %q, want %q", cfg.Jira.TokenType, tc.want)
			}
		})
	}
}

func TestTrimming(t *testing.T) {
	isolate(t)
	t.Setenv("JIRA_URL", "  https://jira.example.com  ")
	t.Setenv("JIRA_USER_EMAIL", "  user@example.com  ")
	t.Setenv("INGESTION_MAX_ITEMS", "  250  ")

	cfg, err := Load(bootstrapIn(t, t.TempDir()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Jira.BaseURL != "https://jira.example.com" {
		t.Errorf("BaseURL = %q, want trimmed", cfg.Jira.BaseURL)
	}
	if cfg.Jira.UserEmail != "user@example.com" {
		t.Errorf("UserEmail = %q, want trimmed", cfg.Jira.UserEmail)
	}
	if cfg.IngestionMaxItems != 250 {
		t.Errorf("MaxItems = %d, want 250 (padded integers must parse)", cfg.IngestionMaxItems)
	}
}

func TestInvalidIntFallsBackAndWarns(t *testing.T) {
	isolate(t)
	buf := captureLogs(t)
	t.Setenv("INGESTION_MAX_ITEMS", "5,000")

	cfg, err := Load(bootstrapIn(t, t.TempDir()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IngestionMaxItems != defaultMaxItems {
		t.Errorf("MaxItems = %d, want default %d", cfg.IngestionMaxItems, defaultMaxItems)
	}
	if !strings.Contains(buf.String(), "Invalid integer") {
		t.Error("malformed integer must be warned about, not silently swallowed")
	}
}

func TestChartsBufferOverMaximumIsFatal(t *testing.T) {
	isolate(t)
	t.Setenv("MCS_CHARTS_BUFFER_SIZE", "5000")

	if _, err := Load(bootstrapIn(t, t.TempDir())); err == nil {
		t.Error("expected an error when MCS_CHARTS_BUFFER_SIZE exceeds the maximum")
	}
}

func TestProvenance_NeverLogsSecrets(t *testing.T) {
	isolate(t)
	buf := captureLogs(t)
	t.Setenv("JIRA_TOKEN", "supersecrettoken")
	t.Setenv("JIRA_SESSION_ID", "secretsession")
	t.Setenv("JIRA_GCLB", "secretgclb")

	if _, err := Load(bootstrapIn(t, t.TempDir())); err != nil {
		t.Fatalf("Load: %v", err)
	}

	for _, secret := range []string{"supersecrettoken", "secretsession", "secretgclb"} {
		if strings.Contains(buf.String(), secret) {
			t.Errorf("secret %q leaked into the log", secret)
		}
	}
	if !strings.Contains(buf.String(), "JIRA_TOKEN") {
		t.Error("JIRA_TOKEN should still be reported as present/absent")
	}
}

func TestProvenance_ReportsSource(t *testing.T) {
	isolate(t)
	buf := captureLogs(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("MCS_ENGINE=bbak\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INGESTION_MAX_ITEMS", "123")

	if _, err := Load(bootstrapIn(t, dir)); err != nil {
		t.Fatalf("Load: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		`"key":"INGESTION_MAX_ITEMS","source":"env"`,
		`"key":"MCS_ENGINE","source":"dotenv"`,
		`"key":"MCS_ENGINE_BBAK","source":"default"`,
		`"key":"JIRA_TOKEN_TYPE","source":"inferred"`,
		`"key":"DATA_PATH","source":"flag"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("provenance log missing %s\ngot: %s", want, out)
		}
	}
}
