package simulation

import (
	"mcs-mcp/internal/eventlog"
	"testing"
	"time"
)

func TestWalkForwardEngine_Execute_Scope(t *testing.T) {
	// Setup: Use relative dates for reconciliation with engine's time.Now()
	now := time.Now().Truncate(24 * time.Hour)
	t0 := now.AddDate(0, 0, -250)
	events := make([]eventlog.IssueEvent, 0)

	// Consistent 1 item per day for 240 days
	for i := 0; i < 240; i++ {
		key := "PROJ-" + string(rune(i))
		ts := t0.AddDate(0, 0, i)

		events = append(events, eventlog.IssueEvent{
			IssueKey: key, EventType: eventlog.Created, ToStatus: "Open", ToStatusID: "1", Timestamp: ts.UnixMicro(),
		})
		events = append(events, eventlog.IssueEvent{
			IssueKey: key, EventType: eventlog.Transitioned, FromStatus: "Open", FromStatusID: "1", ToStatus: "Dev", ToStatusID: "2", Timestamp: ts.UnixMicro(),
		})
		doneTs := t0.AddDate(0, 0, i+1)
		events = append(events, eventlog.IssueEvent{
			IssueKey: key, EventType: eventlog.Transitioned, FromStatus: "Dev", FromStatusID: "2", ToStatus: "Done", ToStatusID: "3", Timestamp: doneTs.UnixMicro(),
		})
		events = append(events, eventlog.IssueEvent{
			IssueKey: key, EventType: eventlog.Resolved, ToStatus: "Done", ToStatusID: "3", Resolution: "Fixed", Timestamp: doneTs.UnixMicro(),
		})
	}

	engine := NewWalkForwardEngine(events, nil)

	cfg := WalkForwardConfig{
		SimulationMode:  "scope",
		LookbackWindow:  30,
		StepSize:        10,
		ForecastHorizon: 10,
		Resolutions:     []string{"Fixed"},
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
		events = append(events, eventlog.IssueEvent{IssueKey: key, EventType: eventlog.Resolved, ToStatus: "Done", ToStatusID: "3", Resolution: "Fixed", Timestamp: doneTs})
	}

	for i := 350; i < 700; i += 10 {
		key := "PROJ-" + string(rune(i))
		ts := t0.AddDate(0, 0, i).UnixMicro()
		events = append(events, eventlog.IssueEvent{IssueKey: key, EventType: eventlog.Created, ToStatus: "Open", ToStatusID: "1", Timestamp: ts})
		doneTs := t0.AddDate(0, 0, i+5).UnixMicro()
		events = append(events, eventlog.IssueEvent{IssueKey: key, EventType: eventlog.Resolved, ToStatus: "Done", ToStatusID: "3", Resolution: "Fixed", Timestamp: doneTs})
	}

	engine := NewWalkForwardEngine(events, nil)

	cfg := WalkForwardConfig{
		SimulationMode:  "scope",
		LookbackWindow:  500,
		StepSize:        30,
		ForecastHorizon: 30,
		Resolutions:     []string{"Fixed"},
	}

	res, err := engine.Execute(cfg)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	if res.DriftWarning == "" {
		t.Error("Expected Drift Warning due to process shift, got none")
	}
}
