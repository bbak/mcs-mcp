package stats

import (
	"fmt"
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

// helper: build a synthetic series of ResidenceTimeBucket with controllable L, Lambda, W, D.
func makeSeries(n int, lFunc, lambdaFunc, wFunc func(i int) float64) []ResidenceTimeBucket {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	series := make([]ResidenceTimeBucket, n)
	for i := range n {
		series[i] = ResidenceTimeBucket{
			Date:   start.AddDate(0, 0, i),
			Label:  start.AddDate(0, 0, i).Format("2006-01-02"),
			N:      1,
			L:      lFunc(i),
			Lambda: lambdaFunc(i),
			W:      wFunc(i),
			A:      i + 1,
			D:      i + 1, // 1 departure per day by default
			Theta:  lambdaFunc(i),
			WStar:  wFunc(i) * 0.9,
		}
	}
	return series
}

// makeSeriesWithDepartures builds a series where D (cumulative departures) is explicitly controlled.
func makeSeriesWithDepartures(n int, departuresPerDay func(i int) int) []ResidenceTimeBucket {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	series := make([]ResidenceTimeBucket, n)
	cumulativeD := 0
	for i := range n {
		cumulativeD += departuresPerDay(i)
		series[i] = ResidenceTimeBucket{
			Date:   start.AddDate(0, 0, i),
			Label:  start.AddDate(0, 0, i).Format("2006-01-02"),
			N:      1,
			L:      5.0,
			Lambda: 1.0,
			W:      5.0,
			A:      i + 1,
			D:      cumulativeD,
			Theta:  1.0,
			WStar:  4.5,
		}
	}
	return series
}

func TestEvaluateConvergenceGate_Converging(t *testing.T) {
	summary := ResidenceTimeSummary{
		Convergence: "converging",
		FinalL:      5.0,
		FinalW:      10.0,
		FinalTheta:  0.5,
	}
	cfg := DefaultSPAPipelineConfig()
	status, scaleFactor, warning := EvaluateConvergenceGate(summary, cfg)

	if status != "converging" {
		t.Errorf("expected converging, got %s", status)
	}
	if scaleFactor != 1.0 {
		t.Errorf("expected scale factor 1.0, got %.2f", scaleFactor)
	}
	if warning != "" {
		t.Errorf("expected no warning, got %q", warning)
	}
}

func TestEvaluateConvergenceGate_Diverging(t *testing.T) {
	summary := ResidenceTimeSummary{
		Convergence: "diverging",
		FinalL:      10.0,
		FinalW:      20.0,
		FinalTheta:  0.8,
	}
	cfg := DefaultSPAPipelineConfig()
	status, scaleFactor, warning := EvaluateConvergenceGate(summary, cfg)

	if status != "diverging" {
		t.Errorf("expected diverging, got %s", status)
	}
	// lambda_implied = 10/20 = 0.5, scaleFactor = 0.5/0.8 = 0.625
	if scaleFactor < 0.5 || scaleFactor > 0.7 {
		t.Errorf("expected scale factor ~0.625, got %.3f", scaleFactor)
	}
	if warning == "" {
		t.Error("expected warning for diverging process")
	}
}

func TestEvaluateConvergenceGate_DivergingClamp(t *testing.T) {
	summary := ResidenceTimeSummary{
		Convergence: "diverging",
		FinalL:      1.0,
		FinalW:      100.0,
		FinalTheta:  0.5,
	}
	cfg := DefaultSPAPipelineConfig()
	_, scaleFactor, _ := EvaluateConvergenceGate(summary, cfg)

	// lambda_implied = 1/100 = 0.01, scaleFactor = 0.01/0.5 = 0.02 → clamped to 0.5
	if scaleFactor != 0.5 {
		t.Errorf("expected clamped scale factor 0.5, got %.3f", scaleFactor)
	}
}

func TestDetectRegimeBoundaries_SingleRegime(t *testing.T) {
	// Steadily increasing Lambda and W — no reversals
	series := makeSeries(60, func(i int) float64 {
		return 5.0 + float64(i)*0.1
	}, func(i int) float64 {
		return 0.5 + float64(i)*0.01
	}, func(i int) float64 {
		return 8.0 + float64(i)*0.05
	})

	cfg := DefaultSPAPipelineConfig()
	regimeStart, boundaries := DetectRegimeBoundaries(series, cfg)

	if len(boundaries) != 0 {
		t.Errorf("expected no regime boundaries for monotone series, got %d", len(boundaries))
	}
	if !regimeStart.Equal(series[0].Date) {
		t.Errorf("expected regime start at series start, got %s", regimeStart.Format("2006-01-02"))
	}
}

func TestDetectRegimeBoundaries_TwoRegimes(t *testing.T) {
	// Lambda increases for 40 days, then decreases for 40 days
	series := makeSeries(80, func(i int) float64 {
		return 10.0
	}, func(i int) float64 {
		if i < 40 {
			return 0.5 + float64(i)*0.05 // increasing
		}
		return 2.5 - float64(i-40)*0.05 // decreasing
	}, func(i int) float64 {
		return 10.0
	})

	cfg := DefaultSPAPipelineConfig()
	regimeStart, boundaries := DetectRegimeBoundaries(series, cfg)

	if len(boundaries) == 0 {
		t.Fatal("expected at least one regime boundary")
	}
	// Regime boundary should be around day 40
	if regimeStart.Before(series[25].Date) || regimeStart.After(series[55].Date) {
		t.Errorf("regime boundary at unexpected position: %s", regimeStart.Format("2006-01-02"))
	}
}

func TestFilterOutliersByConvergence_NoOutliers(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	items := make([]ResidenceItem, 20)
	finished := make([]jira.Issue, 20)
	for i := range 20 {
		end := start.AddDate(0, 0, 10+i) // sojourn 10-29 days, tight range
		items[i] = ResidenceItem{
			Key:   "TEST-" + string(rune('A'+i)),
			Start: start,
			End:   &end,
		}
		finished[i] = jira.Issue{Key: items[i].Key}
	}

	cfg := DefaultSPAPipelineConfig()
	window := NewAnalysisWindow(start, start.AddDate(0, 0, 60), "day", time.Time{})

	filteredFinished, removed := FilterOutliersByConvergence(
		items, finished, start, start.AddDate(0, 0, 60),
		"", nil, nil, window, cfg,
	)

	if len(removed) != 0 {
		t.Errorf("expected no outliers removed, got %d: %v", len(removed), removed)
	}
	if len(filteredFinished) != len(finished) {
		t.Errorf("expected all issues retained, got %d/%d", len(filteredFinished), len(finished))
	}
}

func TestComputeWIPScaleFactor_Balanced(t *testing.T) {
	summary := ResidenceTimeSummary{
		FinalL: 5.0,
		FinalW: 10.0, // lambda_implied = 0.5
	}
	cfg := DefaultSPAPipelineConfig()
	factor := ComputeWIPScaleFactor(summary, 0.5, cfg)
	if factor != 1.0 {
		t.Errorf("expected 1.0 for balanced process, got %.3f", factor)
	}
}

func TestComputeWIPScaleFactor_HigherWIP(t *testing.T) {
	summary := ResidenceTimeSummary{
		FinalL: 10.0,
		FinalW: 20.0, // lambda_implied = 0.5
	}
	cfg := DefaultSPAPipelineConfig()
	factor := ComputeWIPScaleFactor(summary, 0.8, cfg)

	// 0.5 / 0.8 = 0.625
	if factor < 0.6 || factor > 0.65 {
		t.Errorf("expected ~0.625, got %.3f", factor)
	}
}

func TestComputeWIPScaleFactor_ZeroGuard(t *testing.T) {
	cfg := DefaultSPAPipelineConfig()

	if f := ComputeWIPScaleFactor(ResidenceTimeSummary{FinalL: 5, FinalW: 0}, 1.0, cfg); f != 1.0 {
		t.Errorf("expected 1.0 for zero W, got %.3f", f)
	}
	if f := ComputeWIPScaleFactor(ResidenceTimeSummary{FinalL: 5, FinalW: 10}, 0, cfg); f != 1.0 {
		t.Errorf("expected 1.0 for zero histogram throughput, got %.3f", f)
	}
}

func TestClampScale(t *testing.T) {
	cfg := DefaultSPAPipelineConfig()

	tests := []struct {
		input    float64
		expected float64
	}{
		{0.01, 0.5},  // clamped to min
		{3.0, 2.0},   // clamped to max
		{0.97, 1.0},  // snapped to 1.0
		{1.04, 1.0},  // snapped to 1.0
		{0.7, 0.7},   // unchanged
		{1.5, 1.5},   // unchanged
		{1.06, 1.06}, // just outside snap zone
		{0.94, 0.94}, // just outside snap zone
	}
	for _, tc := range tests {
		got := clampScale(tc.input, cfg)
		if got != tc.expected {
			t.Errorf("clampScale(%.2f) = %.2f, want %.2f", tc.input, got, tc.expected)
		}
	}
}

func TestConvergenceImproved(t *testing.T) {
	tests := []struct {
		name     string
		baseline convergenceAssessment
		filtered convergenceAssessment
		want     bool
	}{
		{
			"diverging to converging",
			convergenceAssessment{Label: "diverging", Beta1: 5.0},
			convergenceAssessment{Label: "converging", Beta1: 0.1},
			true,
		},
		{
			"same label, lower beta1",
			convergenceAssessment{Label: "metastable", Beta1: 3.0},
			convergenceAssessment{Label: "metastable", Beta1: 1.0},
			true,
		},
		{
			"same label, higher beta1",
			convergenceAssessment{Label: "metastable", Beta1: 1.0},
			convergenceAssessment{Label: "metastable", Beta1: 3.0},
			false,
		},
		{
			"converging to diverging",
			convergenceAssessment{Label: "converging", Beta1: 0.1},
			convergenceAssessment{Label: "diverging", Beta1: 5.0},
			false,
		},
		{
			"insufficient_data baseline",
			convergenceAssessment{Label: "insufficient_data"},
			convergenceAssessment{Label: "insufficient_data"},
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convergenceImproved(tc.baseline, tc.filtered)
			if got != tc.want {
				t.Errorf("convergenceImproved() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunSPAPipeline_NilResult(t *testing.T) {
	finished := []jira.Issue{{Key: "TEST-1"}}
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 90)
	window := NewAnalysisWindow(start, end, "day", time.Time{})
	cfg := DefaultSPAPipelineConfig()

	result := RunSPAPipeline(nil, nil, finished, start, end, "", nil, nil, window, cfg)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.FilteredFinished) != 1 {
		t.Errorf("expected finished issues passed through, got %d", len(result.FilteredFinished))
	}
	if result.DivergenceScaleFactor != 1.0 {
		t.Errorf("expected divergence scale 1.0, got %.2f", result.DivergenceScaleFactor)
	}
}

// --- Adaptive window tests ---

func TestComputeAdaptiveWindow_ReachesThreshold(t *testing.T) {
	// 100 days, 2 departures per day = 200 total. No boundaries.
	// Threshold=50 → met after ~25 days back (day 74). No boundaries → stops.
	series := makeSeriesWithDepartures(100, func(i int) int { return 2 })
	vantage := series[99].Date

	cfg := DefaultSPAPipelineConfig()
	cfg.MinSampleDepartures = 50

	start, departures, boundaryRespected := ComputeAdaptiveWindow(
		series, vantage, nil, nil, nil, cfg,
	)

	if departures < 50 {
		t.Errorf("expected >= 50 departures, got %d", departures)
	}
	if boundaryRespected {
		t.Error("expected no boundary respected")
	}
	// Should have walked back ~25 days (50 departures / 2 per day)
	daysBack := CalendarDaysBetween(start, vantage)
	if daysBack < 20 || daysBack > 30 {
		t.Errorf("expected ~25 days back, got %d", daysBack)
	}
}

func TestComputeAdaptiveWindow_CrossesBoundaryStationaryStops(t *testing.T) {
	// 100 days, 1 departure per day; regime boundary at day 80.
	// Default series has Lambda=Theta (stationary).
	// Phase 1 crosses boundary at day 80 to reach 50 departures (day 49).
	// After threshold: crossed segment is stationary (Λ/Θ = 1.0) → stop at threshold.
	series := makeSeriesWithDepartures(100, func(i int) int { return 1 })
	vantage := series[99].Date
	boundary := series[80].Date

	cfg := DefaultSPAPipelineConfig()
	cfg.MinSampleDepartures = 50

	start, departures, boundaryRespected := ComputeAdaptiveWindow(
		series, vantage, []time.Time{boundary}, nil, nil, cfg,
	)

	if departures < 50 {
		t.Errorf("expected >= 50 departures, got %d", departures)
	}
	// Crossed segment was stationary → stopped at threshold point, no boundary extension
	if boundaryRespected {
		t.Error("expected boundary NOT respected (stationary segment → stop at threshold)")
	}
	_ = start
}

func TestComputeAdaptiveWindow_CrossesBoundaryNonStationaryExtends(t *testing.T) {
	// 200 days, 1 departure per day; boundaries at day 170 and day 120.
	// Lambda >> Theta → non-stationary everywhere.
	// Walking back from day 199: crosses boundary at 170 before reaching 50 (at day 149).
	// Threshold met at day 149 (50 departures). Crossed segment non-stationary →
	// Phase 2 continues to boundary at day 120, stops there.
	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	series := make([]ResidenceTimeBucket, 200)
	cumulativeD := 0
	for i := range 200 {
		cumulativeD++
		series[i] = ResidenceTimeBucket{
			Date:   startDate.AddDate(0, 0, i),
			Label:  startDate.AddDate(0, 0, i).Format("2006-01-02"),
			N:      1,
			L:      5.0,
			Lambda: 2.0, // arrivals much higher than departures
			W:      5.0,
			A:      i + 1,
			D:      cumulativeD,
			Theta:  0.5, // Λ/Θ = 4.0 → non-stationary
			WStar:  4.5,
		}
	}
	vantage := series[199].Date
	boundary1 := series[170].Date // crossed in Phase 1 (before threshold at ~149)
	boundary2 := series[120].Date // Phase 2 stop point

	cfg := DefaultSPAPipelineConfig()
	cfg.MinSampleDepartures = 50

	windowStart, departures, boundaryRespected := ComputeAdaptiveWindow(
		series, vantage, []time.Time{boundary1, boundary2}, nil, nil, cfg,
	)

	if !boundaryRespected {
		t.Error("expected boundary respected (non-stationary → Phase 2 extension)")
	}
	// Should have stopped at boundary2 (day 120)
	if windowStart.Before(boundary2) {
		t.Errorf("window start %s crossed boundary2 %s",
			windowStart.Format("2006-01-02"), boundary2.Format("2006-01-02"))
	}
	if departures < 50 {
		t.Errorf("expected >= 50 departures, got %d", departures)
	}
}

func TestComputeAdaptiveWindow_NoBoundariesStopsAtThreshold(t *testing.T) {
	// 100 days, 3 departures per day; no regime boundaries.
	// Threshold met at ~17 days back. No boundaries crossed → stops immediately.
	series := makeSeriesWithDepartures(100, func(i int) int { return 3 })
	vantage := series[99].Date

	cfg := DefaultSPAPipelineConfig()
	cfg.MinSampleDepartures = 50

	start, departures, boundaryRespected := ComputeAdaptiveWindow(
		series, vantage, nil, nil, nil, cfg,
	)

	if departures < 50 {
		t.Errorf("expected >= 50 departures, got %d", departures)
	}
	if boundaryRespected {
		t.Error("expected no boundary respected with no boundaries")
	}
	// Should stop close to threshold: ~17 days back
	daysBack := CalendarDaysBetween(start, vantage)
	if daysBack > 25 {
		t.Errorf("expected ~17 days back (no boundaries to extend to), got %d", daysBack)
	}
}

func TestComputeAdaptiveWindow_HitsMaxLookback(t *testing.T) {
	// 500 days, but only 0.05 departures/day → very slow board
	// Max lookback is 365, so should stop there
	series := makeSeriesWithDepartures(500, func(i int) int {
		if i%20 == 0 {
			return 1
		}
		return 0
	})
	vantage := series[499].Date

	cfg := DefaultSPAPipelineConfig()
	cfg.MinSampleDepartures = 50
	cfg.MaxLookbackDays = 365

	start, _, _ := ComputeAdaptiveWindow(
		series, vantage, nil, nil, nil, cfg,
	)

	daysBack := CalendarDaysBetween(start, vantage)
	if daysBack > 365 {
		t.Errorf("exceeded max lookback: %d days back", daysBack)
	}
}

func TestComputeAdaptiveWindow_ExcludesOutliers(t *testing.T) {
	// 100 days, 2 departures per day. No boundaries.
	// Without outlier exclusion: 2 dep/day → threshold (50) at ~25 days back.
	// With outlier exclusion (1 per day): effective 1 dep/day → threshold at ~50 days back.
	// Both stop at threshold, but the window with exclusion is wider.
	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	series := makeSeriesWithDepartures(100, func(i int) int { return 2 })
	vantage := series[99].Date

	// Create outlier items: 1 per day for all 100 days
	outlierKeys := make(map[string]bool)
	var items []ResidenceItem
	for i := range 100 {
		key := fmt.Sprintf("OUT-%d", i)
		outlierKeys[key] = true
		end := startDate.AddDate(0, 0, i)
		items = append(items, ResidenceItem{
			Key:   key,
			Start: startDate,
			End:   &end,
		})
	}

	cfg := DefaultSPAPipelineConfig()
	cfg.MinSampleDepartures = 50

	// Without outlier exclusion
	startNoExclude, _, _ := ComputeAdaptiveWindow(
		series, vantage, nil, nil, nil, cfg,
	)
	// With outlier exclusion
	startWithExclude, _, _ := ComputeAdaptiveWindow(
		series, vantage, nil, outlierKeys, items, cfg,
	)

	// With outliers excluded, the window should start earlier (wider window needed)
	if !startWithExclude.Before(startNoExclude) {
		t.Errorf("expected outlier-excluded window to start earlier: without=%s with=%s",
			startNoExclude.Format("2006-01-02"), startWithExclude.Format("2006-01-02"))
	}
}

func TestComputeAdaptiveWindow_InsufficientData(t *testing.T) {
	// Only 10 days, 1 departure per day = 10 total. Threshold is 50.
	series := makeSeriesWithDepartures(10, func(i int) int { return 1 })
	vantage := series[9].Date

	cfg := DefaultSPAPipelineConfig()
	cfg.MinSampleDepartures = 50

	start, departures, _ := ComputeAdaptiveWindow(
		series, vantage, nil, nil, nil, cfg,
	)

	if departures >= 50 {
		t.Errorf("expected < 50 departures, got %d", departures)
	}
	// Should return the earliest bucket
	if !start.Equal(series[0].Date) {
		t.Errorf("expected earliest date, got %s", start.Format("2006-01-02"))
	}
}

func TestComputeAdaptiveWindow_EmptySeries(t *testing.T) {
	vantage := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	cfg := DefaultSPAPipelineConfig()

	start, departures, boundary := ComputeAdaptiveWindow(
		nil, vantage, nil, nil, nil, cfg,
	)

	if !start.Equal(vantage) {
		t.Errorf("expected vantage date returned for empty series, got %s", start.Format("2006-01-02"))
	}
	if departures != 0 {
		t.Errorf("expected 0 departures, got %d", departures)
	}
	if boundary {
		t.Error("expected no boundary respected for empty series")
	}
}
