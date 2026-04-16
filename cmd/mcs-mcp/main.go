// Command mcs-mcp is the Monte-Carlo Simulation MCP server. It serves analytical
// and forecasting tools to AI agents over the Model Context Protocol.
package main

//go:generate goversioninfo -platform-specific

import (
	"fmt"
	"mcs-mcp/cmd/mcs-mcp/commands"
	"os"
)

func main() {
	if err := commands.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
