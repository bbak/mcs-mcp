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
	store := eventlog.NewEventStore()
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

	// Ensure we lock the "Now()" evaluation for the test so Aging/Yield calculations don't drift globally.
	// We'll set the test evaluation time to a fixed point long after the simulated shift.
	// Last simulated event timestamp + some buffer. We know the shift was ~142 days.
	// Actually, stats tools use `time.Now()` internally in some places unless we override it.
	// Wait, stats functions usually take dates or rely on `issue.Updated`.
	// For `CalculateInventoryAge`, we pass `history` but `time.Now()` is NOT used, it subtracts from `issue.Updated`?
	// `CalculateInventoryAge` uses `time.Now().Sub(*item.AgeSinceCommitment)` inside it? No, `processor.go` sets AgeSinceCommitment absolute dates.
	// Let's actually look at it later if it fails on drift.
	// We will supply a fixed window for the projections.

	latestTS := store.GetLatestTimestamp(eventsFile)
	windowEnd := latestTS
	windowStart := latestTS.AddDate(-1, 0, 0) // 1 year back

	// Force the Analysis Window
	window := stats.NewAnalysisWindow(windowStart, windowEnd, "day", cutoff)

	// We use a dummy provider since no Jira/Hydrate calls are made
	provider := eventlog.NewLogProvider(nil, store, "")

	events := provider.GetEventsInRange(eventsFile, time.Time{}, time.Time{})
	nameToID := make(map[string]string)
	for _, e := range events {
		if e.ToStatus != "" && e.ToStatusID != "" {
			nameToID[e.ToStatus] = e.ToStatusID
		}
		if e.FromStatus != "" && e.FromStatusID != "" {
			nameToID[e.FromStatus] = e.FromStatusID
		}
	}

	idMapping := make(map[string]stats.StatusMetadata)
	for name, m := range wf.Mapping {
		if id, ok := nameToID[name]; ok && id != "" {
			idMapping[id] = m
		} else {
			idMapping[name] = m
		}
	}

	session := stats.NewAnalysisSession(
		provider,
		eventsFile,
		jira.SourceContext{ProjectKey: "MOCK", FetchedAt: latestTS},
		idMapping,
		wf.Resolutions,
		window,
	)

	// 4. Execute the Pipeline
	cadence := stats.GetStratifiedThroughput(session.GetDelivered(), window, wf.Resolutions, idMapping)
	cadence.XmR = stats.AnalyzeThroughputStability(cadence)

	yield := stats.CalculateProcessYield(session.GetAllIssues(), idMapping, wf.Resolutions)

	// Weights are usually derived dynamically, but we'll supply a flat weight for stability.
	flatWeights := make(map[string]int)
	for id := range idMapping {
		flatWeights[id] = 1
	}

	// Resolve Commitment Point to ID
	commitmentID := wf.CommitmentPoint
	if id, ok := nameToID[wf.CommitmentPoint]; ok && id != "" {
		commitmentID = id
	}
	flatWeights[commitmentID] = 2

	aging := stats.CalculateInventoryAge(session.GetWIP(), commitmentID, flatWeights, idMapping, []float64{10.0, 20.0, 30.0}, "wip", window.End)

	var cycleTimes []float64
	for _, issue := range session.GetDelivered() {
		var sumSeconds int64
		for st, secs := range issue.StatusResidency {
			if m, ok := idMapping[st]; ok && m.Tier == "Downstream" {
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
		idMapping,
	)

	wipStability := stats.AnalyzeHistoricalWIP(session.GetAllIssues(), window, commitmentID, flatWeights, idMapping)

	flowDebt := stats.CalculateFlowDebt(session.GetAllIssues(), window, commitmentID, flatWeights, wf.Resolutions, idMapping)

	cfd := stats.CalculateCFDData(session.GetAllIssues(), window, idMapping)

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
		os.WriteFile(tmpPath, actualJSON, 0644)
		t.Errorf("Wrote actual output to %s for comparison. If the mathematical change was intentional, re-run with 'go test ./... -update'", tmpPath)
	}
}
