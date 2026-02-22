package simulation_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"
)

var update = flag.Bool("update", false, "update golden files")

type SimulationGoldenResult struct {
	ScopeForecast    simulation.Result
	DurationForecast simulation.Result
	MultiForecast    simulation.Result
}

func TestSimulationPipeline_Golden(t *testing.T) {
	// 1. Setup Golden Dataset Paths
	testingDir := filepath.Join("..", "testdata", "golden")
	eventsFile := "simulated_events"
	workflowPath := filepath.Join(testingDir, "simulated_workflow.json")

	// 2. Load the Adversarial Event Log
	store := eventlog.NewEventStore()
	err := store.Load(testingDir, eventsFile)
	if err != nil {
		t.Fatalf("Failed to load simulated events: %v", err)
	}

	// 3. Load Workflow Semantics
	wfData, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("Failed to read workflow JSON: %v", err)
	}

	var wf struct {
		Mapping         map[string]stats.StatusMetadata `json:"mapping"`
		CommitmentPoint string                          `json:"commitment_point"`
		DiscoveryCutoff string                          `json:"discovery_cutoff"`
	}
	if err := json.Unmarshal(wfData, &wf); err != nil {
		t.Fatalf("Failed to parse workflow JSON: %v", err)
	}

	cutoff, _ := time.Parse(time.RFC3339, wf.DiscoveryCutoff)
	latestTS := store.GetLatestTimestamp(eventsFile)
	windowStart := latestTS.AddDate(-1, 0, 0) // 1 year back

	window := stats.NewAnalysisWindow(windowStart, latestTS, "day", cutoff)
	provider := eventlog.NewLogProvider(nil, store, "")
	session := stats.NewAnalysisSession(
		provider,
		eventsFile,
		jira.SourceContext{ProjectKey: "MOCK", FetchedAt: latestTS},
		wf.Mapping,
		map[string]string{"Done": "Done"},
		window,
	)

	delivered := session.GetDelivered()

	// 4. Build Histogram
	h := simulation.NewHistogram(delivered, window.Start, window.End, []string{"Story", "Bug", "Task", "Activity"}, wf.Mapping, map[string]string{"Done": "Done"})

	// 5. Guarantee Determinism via Seed
	engine := simulation.NewEngine(h)
	engine.SetSeed(42) // Fixed seed for stable randomness

	// 6. Run Pipelines
	scope := engine.RunScopeSimulation(14, 10000)
	duration := engine.RunDurationSimulation(50, 10000)

	// Ensure we pass the actual distributions instead of nil
	dist := make(map[string]float64)
	if d, ok := h.Meta["type_distribution"].(map[string]float64); ok {
		dist = d
	}

	multi := engine.RunMultiTypeDurationSimulation(map[string]int{
		"Story": 30,
		"Bug":   15,
		"Task":  5,
	}, dist, 10000, true)

	result := SimulationGoldenResult{
		ScopeForecast:    scope,
		DurationForecast: duration,
		MultiForecast:    multi,
	}

	// 7. Serialize & Golden Compare
	actualJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal golden result: %v", err)
	}

	goldenPath := filepath.Join("..", "testdata", "golden", "simulation_pipeline_golden.json")

	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("Failed to create testdata dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, actualJSON, 0644); err != nil {
			t.Fatalf("Failed to write golden file: %v", err)
		}
		t.Logf("Golden file updated at %s", goldenPath)
		return
	}

	expectedJSON, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Golden file not found at %s. Run tests with -update flag to generate it.", goldenPath)
		}
		t.Fatalf("Failed to read golden file: %v", err)
	}

	if !bytes.Equal(expectedJSON, actualJSON) {
		t.Errorf("Mismatch between actual results and golden file.")
		tmpPath := goldenPath + ".actual"
		os.WriteFile(tmpPath, actualJSON, 0644)
		t.Errorf("Wrote actual output to %s for comparison. If the mathematical change was intentional, re-run with 'go test ./... -update'", tmpPath)
	}
}
