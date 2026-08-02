package commands

import (
	"context"

	"mcs-mcp/internal/charts"
	"mcs-mcp/internal/config"
	"mcs-mcp/internal/httpd"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/logging"
	"mcs-mcp/internal/mcp"
	"mcs-mcp/internal/paths"

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
		// Bootstrap resolves the data path and loads the .env files. It runs
		// before logging so that VERBOSE from those files takes effect, which
		// means its own notes have to be replayed once the sinks exist.
		dataPath, _ := cmd.Flags().GetString("data-path")
		boot, notes := paths.Bootstrap(paths.Options{
			DataPath: dataPath,
			Template: config.Template(),
		})

		logging.Init(boot.DataPath)
		replayNotes(notes)

		// Load configuration
		var err error
		cfg, err = config.Load(boot)
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

// replayNotes emits the records paths.Bootstrap collected before the log sinks
// existed, so they land in the logfile rather than only on stderr.
func replayNotes(notes []paths.Note) {
	for _, n := range notes {
		e := log.Info()
		if n.Level == "warn" {
			e = log.Warn()
		}
		for k, v := range n.Fields {
			e = e.Str(k, v)
		}
		e.Msg(n.Msg)
	}
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().String("data-path", "",
		"Base directory for logs and cache (overrides the DATA_PATH environment variable)")
}
