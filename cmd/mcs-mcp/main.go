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
