package commands

import (
	"context"

	"mcs-mcp/internal/charts"
	"mcs-mcp/internal/config"
	"mcs-mcp/internal/httpd"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/logging"
	"mcs-mcp/internal/mcp"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var (
	// Version, Commit, and BuildDate are set at build time via ldflags.
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"

	cfg *config.AppConfig

	jiraClient jira.Client
)

var rootCmd = &cobra.Command{
	Use:   "mcs-mcp",
	Short: "MCS-MCP is a Monte-Carlo-Simulation MCP Server for Jira",
	Long: `A specialized MCP Server that provides statistical forecasting (throughput histograms, fat-tail analysis)
using Monte-Carlo-Simulation based on Jira data.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Init()

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
		server := mcp.NewServer(cfg, jiraClient)

		// If chart rendering is enabled, start the HTTP server alongside stdio.
		if server.ChartBuf() != nil {
			httpSrv, err := httpd.New(server.ChartBuf(), charts.RenderChart)
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to start chart HTTP server")
			}
			server.SetHTTPPort(httpSrv.Port())

			g, ctx := errgroup.WithContext(context.Background())
			g.Go(func() error { return httpSrv.Start(ctx) })
			g.Go(func() error { return server.Run(ctx, Version) })

			if err := g.Wait(); err != nil {
				log.Fatal().Err(err).Msg("Server exited with error")
			}
		} else {
			log.Info().Msg("MCP Server starting Stdio loop (chart rendering disabled)")
			if err := server.Run(context.Background(), Version); err != nil {
				log.Fatal().Err(err).Msg("MCP server exited with error")
			}
		}
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
}
