package stats

import (
	"mcs-mcp/internal/jira"
	"math"
	"testing"
	"time"
)

// ensure math is used
var _ = math.Abs

func date(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func datePtr(year, month, day int) *time.Time {
	t := date(year, month, day)
	return &t
}

func TestComputeResidenceTimeSeries_Identity(t *testing.T) {
	// 10-day window: Jan 1 - Jan 10, 2025
	window := NewAnalysisWindow(date(2025, 1, 1), date(2025, 1, 10), "day", time.Time{})

	items := []ResidenceItem{
		// Item A: starts day 1, ends day 5 (sojourn = 4 days)
		{Key: "A", Type: "Story", Start: date(2025, 1, 1), End: datePtr(2025, 1, 5), PreWindow: false},
		// Item B: starts day 3, ends day 8 (sojourn = 5 days)
		{Key: "B", Type: "Story", Start: date(2025, 1, 3), End: datePtr(2025, 1, 8), PreWindow: false},
		// Item C: starts day 6, ends day 10 (sojourn = 4 days)
		{Key: "C", Type: "Bug", Start: date(2025, 1, 6), End: datePtr(2025, 1, 10), PreWindow: false},
	}

	result := ComputeResidenceTimeSeries(items, window)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Series) != 10 {
		t.Fatalf("expected 10 buckets, got %d", len(result.Series))
	}

	// Verify L(T) = Λ(T) · w(T) at every point where A(T) > 0
	for i, b := range result.Series {
		if b.A == 0 {
			continue
		}
		product := b.Lambda * b.W
		deviation := math.Abs(b.L - product)
		if deviation > 0.01 {
			t.Errorf("day %d: identity violated: L=%.4f, Λ·w=%.4f (dev=%.6f)",
				i+1, b.L, product, deviation)
		}
	}

	if !result.Validation.IdentityVerified {
		t.Errorf("expected identity to be verified, max_deviation=%.10f", result.Validation.MaxDeviation)
	}

	// Verify final summary
	if result.Summary.TotalItems != 3 {
		t.Errorf("expected 3 total items, got %d", result.Summary.TotalItems)
	}
	if result.Summary.InWindowArrivals != 3 {
		t.Errorf("expected 3 in-window arrivals, got %d", result.Summary.InWindowArrivals)
	}
	if result.Summary.Departures != 3 {
		t.Errorf("expected 3 departures, got %d", result.Summary.Departures)
	}
	if result.Summary.ActiveItems != 0 {
		t.Errorf("expected 0 active items, got %d", result.Summary.ActiveItems)
	}

	// Verify Theta = D(T)/T at final bucket:
	// D=3, T=10 → Theta = 0.3
	last := result.Series[9]
	if math.Abs(last.Theta-0.3) > 0.001 {
		t.Errorf("day 10: expected Theta=0.3, got %.4f", last.Theta)
	}
	if result.Summary.FinalTheta != last.Theta {
		t.Errorf("FinalTheta mismatch: summary=%.4f, last bucket=%.4f", result.Summary.FinalTheta, last.Theta)
	}

	// Verify WPrime = H(T)/D(T) at final bucket (all 3 items departed):
	// Items use end-of-day snap: item departed on day X means NOT active at day-end X.
	// A (Jan1→Jan5): active days 1,2,3,4 (departed Jan 5)
	// B (Jan3→Jan8): active days 3,4,5,6,7 (departed Jan 8)
	// C (Jan6→Jan10): active days 6,7,8,9 (departed Jan 10)
	// N per day: 1,1,2,2,1,2,2,1,1,0 → H(10) = 13; D=3; WPrime = 13/3 ≈ 4.3333
	if math.Abs(last.WPrime-13.0/3.0) > 0.001 {
		t.Errorf("day 10: expected WPrime=%.4f, got %.4f", 13.0/3.0, last.WPrime)
	}
	if result.Summary.FinalWPrime != last.WPrime {
		t.Errorf("FinalWPrime mismatch: summary=%.4f, last bucket=%.4f", result.Summary.FinalWPrime, last.WPrime)
	}
}

func TestComputeResidenceTimeSeries_WPrimeDivergence(t *testing.T) {
	// Unbalanced system: 5 arrive, 1 departs — w(T) should be much smaller than w'(T)
	// because w(T) = H/A (arrival-denominated) and w'(T) = H/D (departure-denominated).
	// With A=5 >> D=1, w'(T) >> w(T).
	window := NewAnalysisWindow(date(2025, 1, 1), date(2025, 1, 5), "day", time.Time{})

	items := []ResidenceItem{
		{Key: "A", Start: date(2025, 1, 1), End: datePtr(2025, 1, 5), PreWindow: false},
		{Key: "B", Start: date(2025, 1, 2), End: nil, PreWindow: false},
		{Key: "C", Start: date(2025, 1, 2), End: nil, PreWindow: false},
		{Key: "D", Start: date(2025, 1, 3), End: nil, PreWindow: false},
		{Key: "E", Start: date(2025, 1, 4), End: nil, PreWindow: false},
	}

	result := ComputeResidenceTimeSeries(items, window)
	last := result.Series[len(result.Series)-1]

	// D=1, A=5 → w'(T) should be much larger than w(T)
	if last.D != 1 {
		t.Errorf("expected D=1, got %d", last.D)
	}
	if last.WPrime <= last.W {
		t.Errorf("expected w'(T)=%.4f > w(T)=%.4f for unbalanced system", last.WPrime, last.W)
	}

	// Theta = 1/5 = 0.2; Lambda = 5/5 = 1.0 → Lambda > Theta (WIP accumulating)
	if math.Abs(last.Theta-0.2) > 0.001 {
		t.Errorf("expected Theta=0.2, got %.4f", last.Theta)
	}
	if last.Lambda <= last.Theta {
		t.Errorf("expected Lambda=%.4f > Theta=%.4f for growing WIP", last.Lambda, last.Theta)
	}
}

func TestComputeResidenceTimeSeries_PreWindowItem(t *testing.T) {
	// 5-day window: Jan 6 - Jan 10, 2025
	window := NewAnalysisWindow(date(2025, 1, 6), date(2025, 1, 10), "day", time.Time{})

	items := []ResidenceItem{
		// Pre-window item: started Dec 20, still active in window, ends Jan 8
		{Key: "PRE", Type: "Story", Start: date(2024, 12, 20), End: datePtr(2025, 1, 8), PreWindow: true},
		// In-window item: starts Jan 7, ends Jan 9
		{Key: "IN", Type: "Story", Start: date(2025, 1, 7), End: datePtr(2025, 1, 9), PreWindow: false},
	}

	result := ComputeResidenceTimeSeries(items, window)

	// PRE should contribute to N(t) but NOT to A(T)
	// Day 1 (Jan 6): PRE active → N=1, A=0 (PRE excluded, IN not started)
	day1 := result.Series[0]
	if day1.N != 1 {
		t.Errorf("day 1: expected N=1 (pre-window item active), got %d", day1.N)
	}
	if day1.A != 0 {
		t.Errorf("day 1: expected A=0 (pre-window excluded from arrivals), got %d", day1.A)
	}

	// Day 2 (Jan 7): PRE + IN both active → N=2, A=1 (only IN counts)
	day2 := result.Series[1]
	if day2.N != 2 {
		t.Errorf("day 2: expected N=2, got %d", day2.N)
	}
	if day2.A != 1 {
		t.Errorf("day 2: expected A=1 (only IN counts as arrival), got %d", day2.A)
	}

	// Summary checks
	if result.Summary.PreWindowItems != 1 {
		t.Errorf("expected 1 pre-window item, got %d", result.Summary.PreWindowItems)
	}
}

func TestComputeResidenceTimeSeries_ActiveItemGrows(t *testing.T) {
	// 5-day window
	window := NewAnalysisWindow(date(2025, 1, 1), date(2025, 1, 5), "day", time.Time{})

	items := []ResidenceItem{
		// Active item: starts day 1, never ends
		{Key: "ACTIVE", Type: "Story", Start: date(2025, 1, 1), End: nil, PreWindow: false},
	}

	result := ComputeResidenceTimeSeries(items, window)

	// N(t) should be 1 for all 5 days
	for i, b := range result.Series {
		if b.N != 1 {
			t.Errorf("day %d: expected N=1 for active item, got %d", i+1, b.N)
		}
	}

	// H(T) should grow linearly: 1, 2, 3, 4, 5
	for i, b := range result.Series {
		expected := float64(i + 1)
		if b.H != expected {
			t.Errorf("day %d: expected H=%.0f, got %.0f", i+1, expected, b.H)
		}
	}

	// w(T) should grow: H(T)/A(T) = T/1 = T
	for i, b := range result.Series {
		expected := float64(i + 1)
		if b.W != expected {
			t.Errorf("day %d: expected w=%.0f, got %.4f", i+1, expected, b.W)
		}
	}

	if result.Summary.ActiveItems != 1 {
		t.Errorf("expected 1 active item, got %d", result.Summary.ActiveItems)
	}
}

func TestComputeResidenceTimeSeries_WeeklyGranularity(t *testing.T) {
	// 4-week window
	window := NewAnalysisWindow(date(2025, 1, 6), date(2025, 2, 2), "week", time.Time{})

	items := []ResidenceItem{
		{Key: "A", Type: "Story", Start: date(2025, 1, 6), End: datePtr(2025, 1, 20), PreWindow: false},
		{Key: "B", Type: "Bug", Start: date(2025, 1, 13), End: nil, PreWindow: false},
	}

	result := ComputeResidenceTimeSeries(items, window)

	if len(result.Series) == 0 {
		t.Fatal("expected non-empty series for weekly granularity")
	}

	// Identity should still hold
	if !result.Validation.IdentityVerified {
		t.Errorf("identity should hold for weekly granularity, max_deviation=%.10f", result.Validation.MaxDeviation)
	}
}

func TestComputeResidenceTimeSeries_EmptyItems(t *testing.T) {
	window := NewAnalysisWindow(date(2025, 1, 1), date(2025, 1, 5), "day", time.Time{})

	result := ComputeResidenceTimeSeries(nil, window)

	if result == nil {
		t.Fatal("expected non-nil result even with no items")
	}

	// All N should be 0
	for i, b := range result.Series {
		if b.N != 0 {
			t.Errorf("day %d: expected N=0, got %d", i+1, b.N)
		}
	}
}

func TestExtractResidenceItems(t *testing.T) {
	commitmentPoint := "10002" // status ID for "In Progress"
	statusWeights := map[string]int{
		"10001": 1, // Backlog (Demand)
		"10002": 2, // In Progress (Downstream)
		"10003": 3, // Done (Finished)
	}
	mappings := map[string]StatusMetadata{
		"10001": {Tier: "Demand"},
		"10002": {Tier: "Downstream"},
		"10003": {Tier: "Finished"},
	}
	windowStart := date(2025, 1, 1)

	issues := []jira.Issue{
		// Issue with transitions: committed Jan 5 (in window)
		{
			Key:       "TEST-1",
			IssueType: "Story",
			Created:   date(2024, 12, 1),
			StatusID:  "10003",
			Status:    "Done",
			OutcomeDate: datePtr(2025, 1, 20),
			Transitions: []jira.StatusTransition{
				{ToStatusID: "10001", FromStatusID: "", Date: date(2024, 12, 1)},
				{ToStatusID: "10002", FromStatusID: "10001", Date: date(2025, 1, 5)},
				{ToStatusID: "10003", FromStatusID: "10002", Date: date(2025, 1, 20)},
			},
		},
		// Issue with backflow: committed twice, should use LAST
		{
			Key:       "TEST-2",
			IssueType: "Bug",
			Created:   date(2024, 11, 1),
			StatusID:  "10002",
			Status:    "In Progress",
			Transitions: []jira.StatusTransition{
				{ToStatusID: "10002", FromStatusID: "10001", Date: date(2024, 12, 1)}, // first commitment
				{ToStatusID: "10001", FromStatusID: "10002", Date: date(2024, 12, 15)}, // backflow
				{ToStatusID: "10002", FromStatusID: "10001", Date: date(2025, 1, 10)}, // re-commitment (LAST)
			},
		},
		// Issue that never reached commitment — should be excluded
		{
			Key:       "TEST-3",
			IssueType: "Story",
			Created:   date(2025, 1, 1),
			StatusID:  "10001",
			Status:    "Backlog",
			Transitions: []jira.StatusTransition{
				{ToStatusID: "10001", Date: date(2025, 1, 1)},
			},
		},
		// Issue currently downstream with no transition crossing commitment boundary —
		// should be excluded (zero residence time, no commitment evidence)
		{
			Key:       "TEST-4",
			IssueType: "Story",
			Created:   date(2024, 6, 1),
			StatusID:  "10002",
			Status:    "In Progress",
		},
	}

	result := ExtractResidenceItems(issues, commitmentPoint, statusWeights, mappings, windowStart)

	if len(result) != 2 {
		t.Fatalf("expected 2 items (TEST-3 and TEST-4 excluded), got %d", len(result))
	}

	// TEST-1: should have Start = Jan 5
	if !result[0].Start.Equal(date(2025, 1, 5)) {
		t.Errorf("TEST-1: expected start Jan 5, got %v", result[0].Start)
	}
	if result[0].PreWindow {
		t.Error("TEST-1: should NOT be pre-window")
	}

	// TEST-2: should have Start = Jan 10 (last commitment, backflow reset)
	if !result[1].Start.Equal(date(2025, 1, 10)) {
		t.Errorf("TEST-2: expected start Jan 10 (backflow reset), got %v", result[1].Start)
	}
}
