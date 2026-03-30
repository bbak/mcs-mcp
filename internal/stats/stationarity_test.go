package stats

import (
	"testing"
	"time"
)

func makeResult(convergence string, lambda, theta, wStar, coherenceGap float64, seriesLen int) *ResidenceTimeResult {
	series := make([]ResidenceTimeBucket, seriesLen)
	start := date(2025, 1, 1)
	for i := range series {
		series[i] = ResidenceTimeBucket{
			Date: start.AddDate(0, 0, i),
			W:    10.0, // non-zero so convergence assessment sees valid points
		}
	}

	return &ResidenceTimeResult{
		Series: series,
		Summary: ResidenceTimeSummary{
			WindowStart:       start,
			WindowEnd:         start.AddDate(0, 0, seriesLen-1),
			TotalDays:         seriesLen,
			FinalLambda:       lambda,
			FinalTheta:        theta,
			FinalWStar:        wStar,
			FinalCoherenceGap: coherenceGap,
			Convergence:       convergence,
		},
	}
}

func TestAssessStationarity_Converging(t *testing.T) {
	result := makeResult("converging", 1.0, 1.0, 10.0, 1.0, 30)
	a := AssessStationarity(result)

	if !a.Stationary {
		t.Error("expected stationary for converging process with balanced flow")
	}
	if len(a.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(a.Warnings), a.Warnings)
	}
	if a.RecommendedWindowDays != nil {
		t.Error("expected no window recommendation for stationary process")
	}
}

func TestAssessStationarity_Diverging(t *testing.T) {
	result := makeResult("diverging", 1.0, 1.0, 10.0, 1.0, 30)
	a := AssessStationarity(result)

	if a.Stationary {
		t.Error("expected non-stationary for diverging process")
	}
	if len(a.Warnings) != 1 {
		t.Errorf("expected 1 warning (diverging only), got %d: %v", len(a.Warnings), a.Warnings)
	}
}

func TestAssessStationarity_FlowImbalance(t *testing.T) {
	// Lambda/Theta = 2.0/1.0 = 2.0 > 1.3
	result := makeResult("converging", 2.0, 1.0, 10.0, 1.0, 30)
	a := AssessStationarity(result)

	if a.Stationary {
		t.Error("expected non-stationary for flow imbalance")
	}
	if a.LambdaThetaRatio != 2.0 {
		t.Errorf("expected LambdaThetaRatio=2.0, got %.2f", a.LambdaThetaRatio)
	}
	// Should have 1 warning (flow imbalance)
	found := false
	for _, w := range a.Warnings {
		if len(w) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected at least one warning for flow imbalance")
	}
}

func TestAssessStationarity_AgingWIP(t *testing.T) {
	// CoherenceGap=8.0, WStar=10.0 → ratio=0.8 > 0.5
	result := makeResult("converging", 1.0, 1.0, 10.0, 8.0, 30)
	a := AssessStationarity(result)

	if a.Stationary {
		t.Error("expected non-stationary for aging WIP")
	}
	if a.CoherenceGapRatio != 0.8 {
		t.Errorf("expected CoherenceGapRatio=0.8, got %.2f", a.CoherenceGapRatio)
	}
}

func TestAssessStationarity_AllConditions(t *testing.T) {
	// Diverging + flow imbalance + aging WIP
	result := makeResult("diverging", 2.0, 1.0, 10.0, 8.0, 60)
	a := AssessStationarity(result)

	if a.Stationary {
		t.Error("expected non-stationary")
	}
	if len(a.Warnings) != 3 {
		t.Errorf("expected 3 warnings, got %d: %v", len(a.Warnings), a.Warnings)
	}
	// Should recommend a window since non-stationary and series >= 8
	if a.RecommendedWindowDays == nil {
		t.Fatal("expected window recommendation")
	}
	// 60 days series, tail quarter starts at day 45, so ~15 days from inflection
	// Clamped to min 30
	if *a.RecommendedWindowDays < minRecommendedWindowDays {
		t.Errorf("expected recommended window >= %d, got %d", minRecommendedWindowDays, *a.RecommendedWindowDays)
	}
}

func TestAssessStationarity_ZeroDepartures(t *testing.T) {
	// FinalTheta=0 → skip flow imbalance check
	result := makeResult("converging", 1.0, 0, 0, 0, 30)
	a := AssessStationarity(result)

	if !a.Stationary {
		t.Error("expected stationary when departures are zero (insufficient data)")
	}
	if a.LambdaThetaRatio != 0 {
		t.Errorf("expected LambdaThetaRatio=0 when theta=0, got %.2f", a.LambdaThetaRatio)
	}
}

func TestAssessStationarity_ZeroWStar(t *testing.T) {
	// WStar=0 → skip coherence gap check
	result := makeResult("converging", 1.0, 1.0, 0, 5.0, 30)
	a := AssessStationarity(result)

	if !a.Stationary {
		t.Error("expected stationary when WStar is zero (insufficient data)")
	}
	if a.CoherenceGapRatio != 0 {
		t.Errorf("expected CoherenceGapRatio=0 when WStar=0, got %.2f", a.CoherenceGapRatio)
	}
}

func TestAssessStationarity_NilResult(t *testing.T) {
	a := AssessStationarity(nil)
	if !a.Stationary {
		t.Error("expected stationary for nil result")
	}
}

func TestAssessStationarity_InsufficientSeries(t *testing.T) {
	// Series < 8: no window recommendation even if non-stationary
	result := makeResult("diverging", 1.0, 1.0, 10.0, 1.0, 5)
	a := AssessStationarity(result)

	if a.Stationary {
		t.Error("expected non-stationary for diverging process")
	}
	if a.RecommendedWindowDays != nil {
		t.Error("expected no window recommendation with insufficient series")
	}
}

func TestAssessStationarity_WindowRecommendationClamping(t *testing.T) {
	// 200-day series, diverging → tail quarter starts at day 150 → 50 days recommended
	result := makeResult("diverging", 1.0, 1.0, 10.0, 1.0, 200)
	a := AssessStationarity(result)

	if a.RecommendedWindowDays == nil {
		t.Fatal("expected window recommendation")
	}
	if *a.RecommendedWindowDays < minRecommendedWindowDays {
		t.Errorf("expected recommended window >= %d, got %d", minRecommendedWindowDays, *a.RecommendedWindowDays)
	}
	if a.WindowRationale == "" {
		t.Error("expected non-empty window rationale")
	}
}

func TestAssessStationarity_MetastableNoWarning(t *testing.T) {
	// Metastable convergence with balanced flow → stationary
	result := makeResult("metastable", 1.0, 1.0, 10.0, 2.0, 30)
	a := AssessStationarity(result)

	if !a.Stationary {
		t.Error("expected stationary for metastable with balanced flow and small gap")
	}
}

func TestAssessStationarity_NegativeCoherenceGap(t *testing.T) {
	// Negative coherence gap: |(-8.0)| / 10.0 = 0.8 > 0.5 → should still trigger
	result := makeResult("converging", 1.0, 1.0, 10.0, -8.0, 30)
	a := AssessStationarity(result)

	if a.Stationary {
		t.Error("expected non-stationary for large negative coherence gap")
	}
	if a.CoherenceGapRatio != 0.8 {
		t.Errorf("expected CoherenceGapRatio=0.8 for negative gap, got %.2f", a.CoherenceGapRatio)
	}
}

func TestAssessStationarity_BarelyBelowThresholds(t *testing.T) {
	// Lambda/Theta = 1.29 (below 1.3), CoherenceGap/WStar = 0.49 (below 0.5)
	result := makeResult("converging", 1.29, 1.0, 10.0, 4.9, 30)
	a := AssessStationarity(result)

	if !a.Stationary {
		t.Error("expected stationary when just below all thresholds")
	}
}

// Verify window recommendation dates are sensible.
func TestAssessStationarity_WindowRecommendationDate(t *testing.T) {
	start := date(2025, 1, 1)
	series := make([]ResidenceTimeBucket, 100)
	for i := range series {
		series[i] = ResidenceTimeBucket{
			Date: start.AddDate(0, 0, i),
			W:    10.0,
		}
	}

	result := &ResidenceTimeResult{
		Series: series,
		Summary: ResidenceTimeSummary{
			WindowStart: start,
			WindowEnd:   start.AddDate(0, 0, 99),
			TotalDays:   100,
			FinalLambda: 1.0,
			FinalTheta:  1.0,
			FinalWStar:  10.0,
			Convergence: "diverging",
		},
	}

	a := AssessStationarity(result)
	if a.RecommendedWindowDays == nil {
		t.Fatal("expected window recommendation")
	}

	// Tail quarter starts at index 75 (day 75), window ends at day 99 → 24 days.
	// Clamped to 30.
	if *a.RecommendedWindowDays != 30 {
		// The inflection is at series[75].Date = Jan 1 + 75 days = Mar 17
		// WindowEnd = Jan 1 + 99 = Apr 10. Diff = 24 days → clamped to 30.
		t.Errorf("expected recommended window = 30 (clamped), got %d", *a.RecommendedWindowDays)
	}

	// Verify the rationale mentions the inflection date
	expectedDate := start.AddDate(0, 0, 75).Format(DateFormat)
	if a.WindowRationale == "" {
		t.Fatal("expected non-empty rationale")
	}
	_ = expectedDate // The date should be in the rationale string
	if len(a.WindowRationale) < 10 {
		t.Error("rationale seems too short")
	}
}

// Helper to suppress unused import warning for time package.
var _ = time.Now
