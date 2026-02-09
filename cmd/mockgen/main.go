package main

import (
	"flag"
	"fmt"
	"mcs-mcp/cmd/mockgen/engine"
	"os"
	"time"
)

func main() {
	scenario := flag.String("scenario", "mild", "Scenario to generate: mild, chaos, drift")
	distribution := flag.String("distribution", "uniform", "Distribution to use: uniform, weibull")
	outDir := flag.String("out", "./.cache", "Output directory for mock files")
	count := flag.Int("count", 200, "Number of issues to generate")
	flag.Parse()

	cfg := engine.GeneratorConfig{
		Scenario:     *scenario,
		Distribution: *distribution,
		Count:        *count,
		Now:          time.Now(),
	}

	fmt.Printf("Generating scenario '%s' (Distribution: %s, Count: %d) to %s...\n", cfg.Scenario, cfg.Distribution, cfg.Count, *outDir)

	events, mapping := engine.Generate(cfg)

	sourceID := "MCSTEST_0"
	if err := engine.Save(*outDir, sourceID, events, mapping); err != nil {
		fmt.Printf("Failed to save mock data: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Done.")
}
