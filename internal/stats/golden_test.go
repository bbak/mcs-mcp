package stats_test

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
	"mcs-mcp/internal/stats"
)

var update = flag.Bool("update", false, "update golden files")

type PipelineGoldenResult struct {
	DeliveryCadence   stats.StratifiedThroughput
	ProcessYield      stats.ProcessYield
	WIPAging          []stats.InventoryAge
	ProcessStability  stats.StabilityResult
	WIPStability      stats.WIPStabilityResult
	ThreeWayXmR       stats.ThreeWayResult
	StatusPersistence []stats.StatusPersistence
	FlowDebt          stats.FlowDebtResult
	CFD               stats.CFDResult
}

func TestAnalyticalPipeline_Golden(t *testing.T) {
	// 1. Setup Golden Dataset Paths
	testingDir := filepath.Join("..", "testdata", "golden")
	eventsFile := "simulated_events" // store.Load appends .jsonl
	workflowPath := filepath.Join(testingDir, "simulated_workflow.json")

	// 2. Load the Adversarial Event Log
	store := eventlog.NewEventStore(nil)
	err := store.Load(testingDir, eventsFile)
	if err != nil {
		t.Fatalf("Failed to load simulated events: %v", err)
	}

	if store.Count(eventsFile) == 0 {
		t.Fatalf("Simulated event log is empty")
	}

	// 3. Load Workflow Semantics
	wfData, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("Failed to read workflow JSON: %v", err)
	}

	var wf struct {
		Mapping         map[string]stats.StatusMetadata `json:"mapping"`
		Resolutions     map[string]string               `json:"resolutions"`
		CommitmentPoint string                          `json:"commitment_point"`
		DiscoveryCutoff string                          `json:"discovery_cutoff"`
	}
	if err := json.Unmarshal(wfData, &wf); err != nil {
		t.Fatalf("Failed to parse workflow JSON: %v", err)
	}

	cutoff, _ := time.Parse(time.RFC3339, wf.DiscoveryCutoff)

	latestTS := store.GetLatestTimestamp(eventsFile)
	windowEnd := latestTS
	windowStart := latestTS.AddDate(-1, 0, 0) // 1 year back

	// Force the Analysis Window
	window := stats.NewAnalysisWindow(windowStart, windowEnd, "day", cutoff)

	// We use a dummy provider since no Jira/Hydrate calls are made
	provider := eventlog.NewLogProvider(nil, store, "")

	// For the session, we filter events relevant to the window
	sessionEvents := provider.GetIssuesInRange(eventsFile, window.Start, window.End)
	session := stats.NewAnalysisSession(
		sessionEvents,
		eventsFile,
		jira.SourceContext{ProjectKey: "MOCK", FetchedAt: latestTS},
		wf.Mapping,
		wf.Resolutions,
		window,
	)

	// 4. Execute the Pipeline
	cadence := stats.GetStratifiedThroughput(session.GetDelivered(), window)
	cadence.XmR = stats.AnalyzeThroughputStability(cadence)

	yield := stats.CalculateProcessYield(session.GetAllIssues(), wf.Mapping, wf.Resolutions)

	// Weights are usually derived dynamically, but we'll supply a flat weight for stability.
	flatWeights := make(map[string]int)
	for id := range wf.Mapping {
		flatWeights[id] = 1
	}
	flatWeights[wf.CommitmentPoint] = 2

	aging := stats.CalculateInventoryAge(session.GetWIP(), wf.CommitmentPoint, flatWeights, wf.Mapping, []float64{10.0, 20.0, 30.0}, "wip", true, window.End)

	var cycleTimes []float64
	for _, issue := range session.GetDelivered() {
		var sumSeconds int64
		for st, secs := range issue.StatusResidency {
			if m, ok := wf.Mapping[st]; ok && m.Tier == "Downstream" {
				sumSeconds += secs
			}
		}
		ct := float64(sumSeconds) / 86400.0
		if ct <= 0 {
			ct = 0.1
		}
		cycleTimes = append(cycleTimes, ct)
	}

	var stability stats.StabilityResult
	if len(session.GetDelivered()) > 2 {
		stability = stats.CalculateProcessStability(session.GetDelivered(), cycleTimes, len(session.GetWIP()), 365.0)
	}

	var threeWay stats.ThreeWayResult
	if len(session.GetDelivered()) > 6 {
		subgroups := stats.GroupIssuesByBucket(session.GetDelivered(), cycleTimes, window)
		threeWay = stats.CalculateThreeWayXmR(subgroups)
	}

	persistence := stats.EnrichStatusPersistence(
		stats.CalculateStatusPersistence(session.GetDelivered()),
		wf.Mapping,
	)

	wipStability := stats.AnalyzeHistoricalWIP(session.GetAllIssues(), window, wf.CommitmentPoint, flatWeights, wf.Mapping)

	flowDebt := stats.CalculateFlowDebt(session.GetAllIssues(), window, wf.CommitmentPoint, flatWeights, wf.Resolutions, wf.Mapping)

	cfd := stats.CalculateCFDData(session.GetAllIssues(), window)

	// 5. Gather Results
	result := PipelineGoldenResult{
		DeliveryCadence:   cadence,
		ProcessYield:      yield,
		WIPAging:          aging,
		ProcessStability:  stability,
		WIPStability:      wipStability,
		ThreeWayXmR:       threeWay,
		StatusPersistence: persistence,
		FlowDebt:          flowDebt,
		CFD:               cfd,
	}

	t.Logf("Golden Test Data Lengths -> All: %d, WIP: %d, Delivered: %d", len(session.GetAllIssues()), len(session.GetWIP()), len(session.GetDelivered()))

	// 6. Serialize & Golden Compare
	actualJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal golden result: %v", err)
	}

	goldenPath := filepath.Join("..", "testdata", "golden", "stats_pipeline_golden.json")

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
		// Log a diff or something readable
		t.Errorf("Mismatch between actual results and golden file.")

		// Optional: write actual to a tmp file for user to diff easily
		tmpPath := goldenPath + ".actual"
		if wErr := os.WriteFile(tmpPath, actualJSON, 0644); wErr == nil {
			t.Errorf("Wrote actual output to %s for comparison. If the mathematical change was intentional, re-run with 'go test ./... -update'", tmpPath)
		}
	}
}
