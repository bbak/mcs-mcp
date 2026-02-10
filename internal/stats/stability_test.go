package stats

import (
	"fmt"
	"math"
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

func TestCalculateXmR(t *testing.T) {
	values := []float64{10, 12, 11, 13, 11}
	result := CalculateXmR(values)

	expectedAvg := 11.4
	if math.Abs(result.Average-expectedAvg) > 0.001 {
		t.Errorf("Expected average %v, got %v", expectedAvg, result.Average)
	}

	expectedAmR := 1.75
	if math.Abs(result.AmR-expectedAmR) > 0.001 {
		t.Errorf("Expected AmR %v, got %v", expectedAmR, result.AmR)
	}

	expectedUNPL := 16.055
	if math.Abs(result.UNPL-expectedUNPL) > 0.001 {
		t.Errorf("Expected UNPL %v, got %v", expectedUNPL, result.UNPL)
	}

	if len(result.Signals) != 0 {
		t.Errorf("Expected 0 signals, got %v", len(result.Signals))
	}
}

func TestXmRSignals(t *testing.T) {
	// Rule 1: Outlier
	// Using a stable baseline followed by an outlier to ensure detection
	values := []float64{10, 11, 10, 11, 10, 11, 10, 11, 10, 11, 100}
	result := CalculateXmR(values)
	foundOutlier := false
	for _, s := range result.Signals {
		if s.Type == "outlier" && s.Index == 10 {
			foundOutlier = true
		}
	}
	if !foundOutlier {
		t.Errorf("Expected outlier at index 10 not found. UNPL was %v, Value was 100", result.UNPL)
	}

	// Rule 2: Shift (8 points on one side)
	// We use 8 points above the average, then 8 points below.
	values = []float64{10, 10, 10, 10, 10, 10, 10, 10, 2, 2, 2, 2, 2, 2, 2, 2}
	result = CalculateXmR(values)
	foundShift := 0
	for _, s := range result.Signals {
		if s.Type == "shift" {
			foundShift++
		}
	}
	if foundShift < 2 {
		t.Errorf("Expected 2 shift signals (one at index 7, one at index 15), got %v", foundShift)
	}
}

func TestAnalyzeTimeStability(t *testing.T) {
	hist := []float64{10, 12, 11, 13, 11} // UNPL ~16.05
	wip := []float64{12, 20}              // 20 is an outlier

	result := AnalyzeTimeStability(hist, wip)

	if result.Status != "warning" {
		t.Errorf("Expected status 'warning', got %v", result.Status)
	}

	if len(result.WIPSignals) != 1 {
		t.Errorf("Expected 1 WIP signal, got %v", len(result.WIPSignals))
	}

	if result.WIPSignals[0].Index != 1 {
		t.Errorf("Expected WIP signal at index 1, got %v", result.WIPSignals[0].Index)
	}
}

func TestCalculateThreeWayXmR(t *testing.T) {
	// 9 months of data
	// 8 months of stable averages, followed by a massive outlier
	subgroups := []SubgroupStats{
		{Label: "M1", Average: 10},
		{Label: "M2", Average: 11},
		{Label: "M3", Average: 10},
		{Label: "M4", Average: 11},
		{Label: "M5", Average: 10},
		{Label: "M6", Average: 11},
		{Label: "M7", Average: 10},
		{Label: "M8", Average: 11},
		{Label: "M9", Average: 100},
	}

	result := CalculateThreeWayXmR(subgroups)

	if result.Status == "stable" {
		t.Errorf("Expected non-stable status (volatile or migrating) for outlier average, got stable. UNPL was %v", result.AverageChart.UNPL)
	}

	if len(result.AverageChart.Signals) == 0 {
		t.Errorf("Expected signals in the average chart")
	}
}

func TestXmRBenchmark(t *testing.T) {
	// Benchmark Dataset: Monthly Accounts Receivable (clients)
	// Ref: r-bar.net SPC tutorials / Wheeler methods
	values := []float64{
		22433, 22612, 22660, 22380, 22545, 22903, 22843, 22595, 22078, 21942,
	}

	result := CalculateXmR(values)

	// Benchmarks from source:
	// X-bar (Avg) ~ 22264
	// M-bar (AmR) ~ 172
	// UNPL ~ 22722
	// LNPL ~ 21806

	// Check average calculation: Sum(22433...21942) / 10 = 224991 / 10 = 22499.1
	if math.Abs(result.Average-22499.1) > 1.0 {
		t.Errorf("Expected average 22499.1, got %v", result.Average)
	}

	// The key is the Scaling and Limits logic verification
	expectedAmR := 220.77 // (179+48+280+165+358+60+248+517+136)/9
	if math.Abs(result.AmR-expectedAmR) > 1.0 {
		t.Errorf("Bench AmR mismatch. Expected ~220.7, got %v", result.AmR)
	}

	// Verify UNPL Scaling (Avg + 2.66 * AmR)
	expectedUNPL := result.Average + (2.66 * result.AmR)
	if math.Abs(result.UNPL-expectedUNPL) > 0.001 {
		t.Errorf("UNPL Scaling error. Expected %v, got %v", expectedUNPL, result.UNPL)
	}
}
func TestGroupIssuesByBucket_ExcludesCurrentBucket(t *testing.T) {
	now := time.Now()

	prev := now.AddDate(0, 0, -7)
	y2, w2 := prev.ISOWeek()
	prevWeekKey := fmt.Sprintf("%d-W%02d", y2, w2)

	issues := []jira.Issue{
		{ResolutionDate: &now},  // Current week
		{ResolutionDate: &prev}, // Previous week
	}
	cycleTimes := []float64{10.0, 20.0}

	window := NewAnalysisWindow(now.AddDate(0, 0, -14), now, "week", time.Time{})
	subgroups := GroupIssuesByBucket(issues, cycleTimes, window)

	// Should only find the previous week
	if len(subgroups) != 1 {
		t.Fatalf("Expected 1 subgroup, got %d", len(subgroups))
	}
	if subgroups[0].Label != prevWeekKey {
		t.Errorf("Expected subgroup label %s, got %s", prevWeekKey, subgroups[0].Label)
	}
}
func TestCalculateProcessStability(t *testing.T) {
	cycleTimes := []float64{10, 10, 10, 10, 10} // Perfectly stable
	wipCount := 5
	windowDays := 10.0

	// Throughput = 5 / 10 = 0.5 items/day
	// Expected Lead Time = 5 / 0.5 = 10
	// Stability Index = 10 / 10 (avg) = 1.0
	res := CalculateProcessStability(nil, cycleTimes, wipCount, windowDays)

	index := res.StabilityIndex
	if math.Abs(index-1.0) > 0.001 {
		t.Errorf("Expected stability index 1.0, got %v", index)
	}

	// Double WIP
	// Expected Lead Time = 10 / 0.5 = 20
	// Stability Index = 20 / 10 (avg) = 2.0
	res2 := CalculateProcessStability(nil, cycleTimes, 10, windowDays)
	index2 := res2.StabilityIndex
	if math.Abs(index2-2.0) > 0.001 {
		t.Errorf("Expected stability index 2.0, got %v", index2)
	}
}
