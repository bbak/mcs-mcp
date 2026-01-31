package commands

import (
	"mcs-mcp/internal/config"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/logging"
	"mcs-mcp/internal/mcp"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	// Version, Commit, and BuildDate are set at build time via ldflags.
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"

	verbose bool
	cfg     *config.AppConfig

	jiraClient jira.Client
)

var rootCmd = &cobra.Command{
	Use:   "mcs-mcp",
	Short: "MCS-MCP is a Monte-Carlo-Simulation MCP Server for Jira",
	Long: `A specialized MCP Server that provides statistical forecasting (throughput histograms, fat-tail analysis)
using Monte-Carlo-Simulation based on Jira data.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Init(verbose)

		// Load configuration
		var err error
		cfg, err = config.Load()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load configuration")
		}

		// Initialize Jira Client
		jiraClient = jira.NewClient(cfg.Jira)

		log.Info().
			Str("version", Version).
			Str("commit", Commit).
			Str("buildDate", BuildDate).
			Msg("MCS-MCP starting")
	},
	Run: func(cmd *cobra.Command, args []string) {
		log.Info().Msg("MCP Server starting Stdio loop")
		server := mcp.NewServer(cfg, jiraClient)
		server.Start()
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging")
}
