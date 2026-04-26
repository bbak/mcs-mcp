package mcp

import (
	"encoding/json"
	"testing"
)

// TestSessionWindow_PropagationAcrossDiagnostics is an end-to-end check of the
// session-window contract: after set_analysis_window, every diagnostic's
// session_context.session_window must echo the same {start, end} that the
// setter accepted. forecast_monte_carlo is exempt — it computes its own
// sample window — so we assert it does NOT inherit the diagnostic window
// (its sampling is independent of session state).
func TestSessionWindow_PropagationAcrossDiagnostics(t *testing.T) {
	srv := newGoldenServer(t)

	wantEnd := srv.Clock().UTC()
	wantStart := wantEnd.AddDate(0, 0, -60)

	if _, err := srv.handleSetAnalysisWindow(
		wantStart.Format("2006-01-02"),
		wantEnd.Format("2006-01-02"),
		0, false,
	); err != nil {
		t.Fatalf("set_analysis_window: %v", err)
	}

	got, err := srv.handleGetAnalysisWindow()
	if err != nil {
		t.Fatalf("get_analysis_window: %v", err)
	}
	gotEnv, ok := got.(ResponseEnvelope)
	if !ok {
		t.Fatalf("get_analysis_window: expected ResponseEnvelope, got %T", got)
	}
	gotData, ok := gotEnv.Data.(map[string]any)
	if !ok {
		t.Fatalf("get_analysis_window: data not map, got %T", gotEnv.Data)
	}
	if gotData["source"] != "session" {
		t.Fatalf("source = %v, want \"session\"", gotData["source"])
	}
	if gotData["start"] != wantStart.Format("2006-01-02") {
		t.Fatalf("start = %v, want %v", gotData["start"], wantStart.Format("2006-01-02"))
	}
	if gotData["end"] != wantEnd.Format("2006-01-02") {
		t.Fatalf("end = %v, want %v", gotData["end"], wantEnd.Format("2006-01-02"))
	}

	diagnostics := []struct {
		name string
		run  func() (any, error)
	}{
		{
			"analyze_throughput",
			func() (any, error) {
				return srv.handleGetDeliveryCadence(testProject, testBoard, "week", false)
			},
		},
		{
			"analyze_wip_stability",
			func() (any, error) {
				return srv.handleAnalyzeWIPStability(testProject, testBoard)
			},
		},
		{
			"analyze_cycle_time",
			func() (any, error) {
				return srv.handleGetCycleTimeAssessment(testProject, testBoard, "", "", nil, 0, 0)
			},
		},
		{
			"analyze_flow_debt",
			func() (any, error) {
				return srv.handleGetFlowDebt(testProject, testBoard, "week")
			},
		},
	}

	wantStartStr := wantStart.Format("2006-01-02")
	wantEndStr := wantEnd.Format("2006-01-02")

	for _, tc := range diagnostics {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.run()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			res = srv.injectSessionContext(res)
			env, ok := res.(ResponseEnvelope)
			if !ok {
				t.Fatalf("%s: expected ResponseEnvelope, got %T", tc.name, res)
			}
			sw, ok := env.Context["session_window"].(map[string]any)
			if !ok {
				t.Fatalf("%s: session_window missing in context", tc.name)
			}
			if sw["start"] != wantStartStr {
				t.Errorf("%s: session_window.start = %v, want %v", tc.name, sw["start"], wantStartStr)
			}
			if sw["end"] != wantEndStr {
				t.Errorf("%s: session_window.end = %v, want %v", tc.name, sw["end"], wantEndStr)
			}
			if sw["source"] != "session" {
				t.Errorf("%s: session_window.source = %v, want \"session\"", tc.name, sw["source"])
			}
		})
	}

	// Forecasting is exempt: the session_context footer still reports the window
	// (so the agent can see what's active), but the simulation's sample window
	// is engine-driven and must NOT equal the diagnostic window. Use a 5-day
	// duration_days session window so that a forecast forced to inherit it
	// would lack throughput data — and inspect the simulation's reported
	// sample range instead.
	if _, err := srv.handleSetAnalysisWindow("", "", 5, false); err != nil {
		t.Fatalf("set_analysis_window narrow: %v", err)
	}

	res, err := srv.handleRunSimulation(
		testProject, testBoard,
		"scope",
		false, 0, 60, "",
		"", "", nil, false,
		0, "", "",
		nil, nil,
	)
	if err != nil {
		t.Fatalf("forecast_monte_carlo: %v", err)
	}
	env, ok := res.(ResponseEnvelope)
	if !ok {
		t.Fatalf("forecast_monte_carlo: expected ResponseEnvelope, got %T", res)
	}
	days, ok := mcsDaysInSample(env.Data)
	if !ok {
		t.Skip("MCS result lacks days_in_sample; skipping forecast-independence assertion")
	}
	if days <= 6 {
		t.Errorf("forecast_monte_carlo inherited the 5-day session window: days_in_sample=%d. forecasting must use its own (larger) sample.", days)
	}
}

// mcsDaysInSample reads days_in_sample from the simulation result. Round-trips
// through JSON to handle typed structs and nested context blocks uniformly.
func mcsDaysInSample(data any) (int, bool) {
	raw, err := json.Marshal(data)
	if err != nil {
		return 0, false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0, false
	}
	candidates := []map[string]any{m}
	if ctx, ok := m["context"].(map[string]any); ok {
		candidates = append(candidates, ctx)
	}
	for _, c := range candidates {
		v, ok := c["days_in_sample"]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case int:
			return n, true
		case int64:
			return int(n), true
		case float64:
			return int(n), true
		}
	}
	return 0, false
}
