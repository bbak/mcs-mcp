package config

import (
	"bytes"
	"fmt"
	"strings"
)

// Default values for every configuration variable. Referenced by Load and by
// configVars so the code and the generated template cannot drift.
const (
	defaultTokenType       = "pat"
	defaultRequestDelay    = 10
	defaultBackflowReset   = true
	defaultEngine          = "crude"
	defaultEngineCrude     = 50
	defaultEngineBBAK      = 50
	defaultChartsBuffer    = 20
	defaultUpdatedLookback = 24
	defaultCreatedLookback = 36
	defaultMaxItems        = 5000
)

// configVar describes one configuration variable. The registry below is the
// authoritative list: it generates the .env template and drives startup
// provenance logging, so a variable cannot appear in one and not the other.
type configVar struct {
	Key string
	// Default is the value the Go code falls back to, rendered into the
	// template. Empty means "no default" (the variable is simply unset).
	Default string
	// Placeholder is example text shown in the comment block for variables
	// that have no meaningful default, such as JIRA_URL.
	Placeholder string
	// Doc is the comment block written above the variable.
	Doc string
	// Secret suppresses the value in provenance logging.
	Secret bool
	// Commented ships the assignment commented out, for optional variables.
	Commented bool
	// Section starts a new banner comment before this variable.
	Section string
}

var configVars = []configVar{
	{
		Key:         "JIRA_URL",
		Placeholder: "https://your-domain.atlassian.net",
		Section:     "Jira instance to connect to",
		Doc:         "Base URL of your Jira instance.",
	},
	{
		Key:     "JIRA_TOKEN_TYPE",
		Section: "Authentication: 3 options",
		Doc: "Token type. Leave unset to infer it: JIRA_USER_EMAIL set means \"api\"\n" +
			"(Atlassian Cloud), otherwise \"pat\" (Data Center/Server).\n" +
			"Set explicitly to override:\n" +
			"  pat - Personal Access Token, Data Center/Server only\n" +
			"  api - API Token, Atlassian Cloud only",
		Commented: true,
	},
	{
		Key:    "JIRA_TOKEN",
		Doc:    "The token itself.",
		Secret: true,
	},
	{
		Key: "JIRA_USER_EMAIL",
		Doc: "Your Jira account e-mail. Required for Atlassian Cloud API tokens,\n" +
			"leave empty for Data Center/Server with a PAT.",
	},
	{
		Key:       "JIRA_XSRF_TOKEN",
		Doc:       "3rd option: session cookies, for locked-down Data Center instances.",
		Secret:    true,
		Commented: true,
	},
	{Key: "JIRA_SESSION_ID", Secret: true, Commented: true},
	{Key: "JIRA_REMEMBERME_COOKIE", Secret: true, Commented: true},
	{
		Key:       "JIRA_GCILB",
		Doc:       "Optional cookies for a Google Cloud Load Balancer.",
		Secret:    true,
		Commented: true,
	},
	{Key: "JIRA_GCLB", Secret: true, Commented: true},
	{
		Key:     "JIRA_REQUEST_DELAY_SECONDS",
		Default: fmt.Sprint(defaultRequestDelay),
		Section: "Jira REST API behaviour",
		Doc:     "Enforced delay between requests to the Jira REST API.",
	},
	{
		Key:     "VERBOSE",
		Default: "false",
		Section: "Diagnostics",
		Doc:     "Writes considerably more detail to the logfile.",
	},
	{
		Key:     "DATA_PATH",
		Section: "Data location",
		Doc: "Base folder for logs and cache.\n" +
			"NOTE: setting this here has NO EFFECT. The data path must be known\n" +
			"before this file can be found, so it is read only from the\n" +
			"--data-path flag or the DATA_PATH environment variable.\n" +
			"Listed here for discoverability only.",
		Commented: true,
	},
	{
		Key:     "COMMITMENT_POINT_BACKFLOW_RESET_CLOCK",
		Default: "true",
		Section: "Flow metrics",
		Doc: "Reset the clock when an item backflows past the commitment point.\n" +
			"Applies to both Cycle Time and WIP Age to keep them in sync.\n" +
			"When true, backflowed items have their age recalculated from the\n" +
			"last backflow date forward. Set to false to keep cumulative age.",
	},
	{
		Key:     "MCS_ENGINE",
		Default: defaultEngine,
		Section: "Simulation engine",
		Doc: "Engine selection: \"crude\", \"bbak\" (regime-aware sampling), or\n" +
			"\"auto\" (backtest-driven). See docs/architecture.md 4.6-4.8.",
		Commented: true,
	},
	{
		Key:     "MCS_ENGINE_CRUDE",
		Default: fmt.Sprint(defaultEngineCrude),
		Doc: "Per-engine weights (0 = disabled, 1-100 = enabled with tiebreak\n" +
			"priority). Used by \"auto\" to break ties when two engines have\n" +
			"identical backtest accuracy.",
		Commented: true,
	},
	{Key: "MCS_ENGINE_BBAK", Default: fmt.Sprint(defaultEngineBBAK), Commented: true},
	{
		Key:     "MCS_CHARTS_BUFFER_SIZE",
		Default: fmt.Sprint(defaultChartsBuffer),
		Section: "Chart rendering",
		Doc: "Number of recent tool results kept for on-demand chart rendering.\n" +
			"0 = disabled (no HTTP server). 1-100 = enabled. Values above 100\n" +
			"are rejected at startup. When enabled the server listens on a\n" +
			"local port for chart requests.",
	},
	{
		Key:     "INGESTION_UPDATED_LOOKBACK",
		Default: fmt.Sprint(defaultUpdatedLookback),
		Section: "Initial Jira hydration",
		Doc: "Hydration window (months). The hydration JQL fetches all issues\n" +
			"updated within INGESTION_UPDATED_LOOKBACK months OR created within\n" +
			"INGESTION_CREATED_LOOKBACK months. The created clause keeps\n" +
			"long-lived items whose last update fell outside the updated window.",
	},
	{Key: "INGESTION_CREATED_LOOKBACK", Default: fmt.Sprint(defaultCreatedLookback)},
	{
		Key:     "INGESTION_MAX_ITEMS",
		Default: fmt.Sprint(defaultMaxItems),
		Doc: "Hard ceiling on items pulled during initial hydration. Paging stops\n" +
			"once the running total reaches this value. Forward catch-up\n" +
			"(import_history_update) is not subject to this cap.",
	},
}

const templateHeader = `# MCS-MCP configuration
#
# Generated from internal/config/template.go - do not edit by hand.
# This file is created once and never overwritten, so your edits are safe.
#
# Precedence, highest first:
#   1. process environment
#   2. this file
#   3. .env next to the binary
#   4. .env in the working directory
#   5. built-in defaults (shown below)
`

// Template renders the .env template from configVars. It is written to the
// data path on first run and committed as conf/.env-example.
func Template() []byte {
	var b bytes.Buffer
	b.WriteString(templateHeader)

	for _, v := range configVars {
		if v.Section != "" {
			fmt.Fprintf(&b, "\n#\n# %s\n#\n", v.Section)
		} else {
			b.WriteByte('\n')
		}

		if v.Doc != "" {
			for _, line := range strings.Split(v.Doc, "\n") {
				if line == "" {
					b.WriteString("#\n")
					continue
				}
				fmt.Fprintf(&b, "# %s\n", line)
			}
		}
		if v.Placeholder != "" {
			fmt.Fprintf(&b, "# Example: %s\n", v.Placeholder)
		}

		prefix := ""
		if v.Commented {
			prefix = "# "
		}
		fmt.Fprintf(&b, "%s%s=%s\n", prefix, v.Key, v.Default)
	}

	return b.Bytes()
}
