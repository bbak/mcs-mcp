package simulation

import (
	"fmt"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/stats"
	"testing"
	"time"
)

func TestWalkForwardEngine_Execute_Scope(t *testing.T) {
	// Setup: Use relative dates for reconciliation with engine's time.Now()
	now := time.Now().Truncate(24 * time.Hour)
	t0 := now.AddDate(0, 0, -250)
	events := make([]eventlog.IssueEvent, 0)

	// Consistent 1 item per day for 240 days
	for i := 0; i < 250; i++ {
		key := "PROJ-" + string(rune(i))
		ts := t0.AddDate(0, 0, i)

		events = append(events, eventlog.IssueEvent{
			IssueKey: key, EventType: eventlog.Created, ToStatus: "Open", ToStatusID: "1", Timestamp: ts.UnixMicro(),
		})
		events = append(events, eventlog.IssueEvent{
			IssueKey: key, EventType: eventlog.Change, FromStatus: "Open", FromStatusID: "1", ToStatus: "Dev", ToStatusID: "2", Timestamp: ts.UnixMicro(),
		})
		// Add variance: most take 1 day, some 0 (double on same day), some 2 (gap)
		delta := 1
		if i%5 == 0 {
			delta = 0
		} else if i%7 == 0 {
			delta = 2
		}

		doneTs := t0.AddDate(0, 0, i+delta)
		events = append(events, eventlog.IssueEvent{
			IssueKey: key, EventType: eventlog.Change, FromStatus: "Dev", FromStatusID: "2", ToStatus: "Done", ToStatusID: "3", Resolution: "Fixed", Timestamp: doneTs.UnixMicro(),
		})
	}

	mappings := map[string]stats.StatusMetadata{
		"Done": {Tier: "Finished", Outcome: "delivered"},
	}
	engine := NewWalkForwardEngine(events, mappings, nil)

	cfg := WalkForwardConfig{
		SimulationMode:  "scope",
		LookbackWindow:  30,
		StepSize:        10,
		ForecastHorizon: 10,
	}

	res, err := engine.Execute(cfg)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	if len(res.Checkpoints) == 0 {
		t.Fatalf("Expected checkpoints, got 0")
	}

	t.Logf("Accuracy Score: %.2f", res.AccuracyScore)
	for _, cp := range res.Checkpoints {
		t.Logf("Date: %s, Actual: %.1f, P50: %.1f, P95: %.1f (Within: %v)", cp.Date, cp.ActualValue, cp.PredictedP50, cp.PredictedP95, cp.IsWithinCone)
		if cp.ActualValue < 8 || cp.ActualValue > 12 {
			t.Errorf("Checkpoint %s: Expected actual ~10, got %.1f", cp.Date, cp.ActualValue)
		}
	}

	if res.AccuracyScore < 0.9 {
		t.Errorf("Expected Accuracy Score >= 0.9, got %.2f", res.AccuracyScore)
	}
}

func TestWalkForwardEngine_Driftlimit(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	t0 := now.AddDate(0, 0, -700)
	events := make([]eventlog.IssueEvent, 0)

	for i := 0; i < 350; i++ {
		key := "PROJ-" + string(rune(i))
		ts := t0.AddDate(0, 0, i).UnixMicro()
		events = append(events, eventlog.IssueEvent{IssueKey: key, EventType: eventlog.Created, ToStatus: "Open", ToStatusID: "1", Timestamp: ts})
		doneTs := t0.AddDate(0, 0, i+1).UnixMicro()
		events = append(events, eventlog.IssueEvent{IssueKey: key, EventType: eventlog.Change, ToStatus: "Done", ToStatusID: "3", Resolution: "Fixed", Timestamp: doneTs})
	}

	for i := 350; i < 700; i += 10 {
		key := "PROJ-" + string(rune(i))
		ts := t0.AddDate(0, 0, i).UnixMicro()
		events = append(events, eventlog.IssueEvent{IssueKey: key, EventType: eventlog.Created, ToStatus: "Open", ToStatusID: "1", Timestamp: ts})
		doneTs := t0.AddDate(0, 0, i+5).UnixMicro()
		events = append(events, eventlog.IssueEvent{IssueKey: key, EventType: eventlog.Change, ToStatus: "Done", ToStatusID: "3", Resolution: "Fixed", Timestamp: doneTs})
	}

	mappings := map[string]stats.StatusMetadata{
		"Done": {Tier: "Finished", Outcome: "delivered"},
	}
	engine := NewWalkForwardEngine(events, mappings, nil)

	cfg := WalkForwardConfig{
		SimulationMode:  "scope",
		LookbackWindow:  500,
		StepSize:        30,
		ForecastHorizon: 30,
	}

	res, err := engine.Execute(cfg)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	if res.DriftWarning == "" {
		t.Error("Expected Drift Warning due to process shift, got none")
	}
}

func TestWalkForwardEngine_Execute_Duration(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	t0 := now.AddDate(0, 0, -300)
	events := make([]eventlog.IssueEvent, 0)

	// Throughput of 1 item per day, extending into "future"
	for i := 0; i < 350; i++ {
		key := fmt.Sprintf("PROJ-%d", i)
		// Add some variance: most take 1 day, some 0, some 2
		delta := 1
		if i%10 == 0 {
			delta = 0 // "Double" delivery
		} else if i%15 == 0 {
			delta = 2 // Gap
		}

		createdTs := t0.AddDate(0, 0, i).UnixMicro()
		doneTs := t0.AddDate(0, 0, i+delta).UnixMicro()

		events = append(events, eventlog.IssueEvent{
			IssueKey: key, EventType: eventlog.Created, ToStatus: "Open", ToStatusID: "1", Timestamp: createdTs,
		})
		events = append(events, eventlog.IssueEvent{
			IssueKey: key, EventType: eventlog.Change, FromStatus: "Open", ToStatus: "Done", Resolution: "Fixed", Timestamp: doneTs,
		})
	}

	mappings := map[string]stats.StatusMetadata{
		"Done": {Tier: "Finished", Outcome: "delivered"},
	}
	engine := NewWalkForwardEngine(events, mappings, nil)

	cfg := WalkForwardConfig{
		SimulationMode:  "duration",
		LookbackWindow:  30,
		StepSize:        10,
		ItemsToForecast: 10,
	}

	res, err := engine.Execute(cfg)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	if len(res.Checkpoints) == 0 {
		t.Fatalf("Expected checkpoints, got 0")
	}

	t.Logf("Duration Accuracy Score: %.2f", res.AccuracyScore)
	for _, cp := range res.Checkpoints {
		t.Logf("Date: %s, Actual: %.1f, P50: %.1f, P85: %.1f (Within: %v)", cp.Date, cp.ActualValue, cp.PredictedP50, cp.PredictedP85, cp.IsWithinCone)
		// With 1 item/day throughput, 10 items should take ~10 days
		if cp.ActualValue < 9 || cp.ActualValue > 11 {
			t.Errorf("Checkpoint %s: Expected actual ~10, got %.1f", cp.Date, cp.ActualValue)
		}
	}

	if res.AccuracyScore < 0.7 {
		t.Errorf("Expected Duration Accuracy Score >= 0.7, got %.2f", res.AccuracyScore)
	}
}
