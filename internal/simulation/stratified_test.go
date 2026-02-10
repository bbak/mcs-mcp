package simulation

import (
	"mcs-mcp/internal/jira"
	"testing"
	"time"
)

func TestAssessStratificationNeeds(t *testing.T) {
	now := time.Now()

	// 1. Case: Low volume
	issues := make([]jira.Issue, 10)
	for i := 0; i < 10; i++ {
		resDate := now.AddDate(0, 0, -i)
		issues[i] = jira.Issue{
			IssueType:      "Story",
			Created:        resDate.AddDate(0, 0, -10),
			ResolutionDate: &resDate,
		}
	}

	decisions := AssessStratificationNeeds(issues, nil, nil)
	if len(decisions) != 1 || decisions[0].Eligible {
		t.Errorf("Expected Story to be ineligible due to low volume, got %+v", decisions)
	}

	// 2. Case: High volume but low variance
	issues = make([]jira.Issue, 40)
	for i := 0; i < 40; i++ {
		resDate := now.AddDate(0, 0, -i)
		issues[i] = jira.Issue{
			IssueType:      "Story",
			Created:        resDate.AddDate(0, 0, -10),
			ResolutionDate: &resDate,
		}
	}
	decisions = AssessStratificationNeeds(issues, nil, nil)
	if len(decisions) != 1 || decisions[0].Eligible {
		t.Errorf("Expected Story to be ineligible due to low variance, got %+v", decisions)
	}

	// 3. Case: High volume and high variance
	issues = make([]jira.Issue, 0)
	// 20 Stories taking 40 days
	for i := 0; i < 20; i++ {
		resDate := now.AddDate(0, 0, -i)
		issues = append(issues, jira.Issue{
			IssueType:      "Story",
			Created:        resDate.AddDate(0, 0, -40),
			ResolutionDate: &resDate,
		})
	}
	// 20 Bugs taking 2 days
	for i := 0; i < 20; i++ {
		resDate := now.AddDate(0, 0, -i)
		issues = append(issues, jira.Issue{
			IssueType:      "Bug",
			Created:        resDate.AddDate(0, 0, -2),
			ResolutionDate: &resDate,
		})
	}

	decisions = AssessStratificationNeeds(issues, nil, nil)
	eligibleCount := 0
	for _, d := range decisions {
		if d.Eligible {
			eligibleCount++
		}
	}
	if eligibleCount < 1 {
		t.Errorf("Expected at least one type to be eligible, got %+v", decisions)
	}
}

func TestStratifiedSimulation(t *testing.T) {
	// Create a histogram where Stories are 1/day and Bugs are 1/day but on different days
	// This creates a "perfectly disjoint" process.
	days := 60
	buckets := make([]int, days)
	stratified := make(map[string][]int)
	stratified["Story"] = make([]int, days)
	stratified["Bug"] = make([]int, days)

	for i := 0; i < days; i++ {
		if i%2 == 0 {
			stratified["Story"][i] = 1
			buckets[i] = 1
		} else {
			stratified["Bug"][i] = 1
			buckets[i] = 1
		}
	}

	h := &Histogram{
		Counts:           buckets,
		StratifiedCounts: stratified,
		Meta: map[string]interface{}{
			"stratification_eligible": map[string]bool{
				"Story": true,
				"Bug":   true,
			},
			"type_distribution": map[string]float64{
				"Story": 0.5,
				"Bug":   0.5,
			},
		},
	}

	engine := NewEngine(h)

	// Goal: 10 Stories.
	// Since Story throughput is 0.5/day (1 every 2 days), we expect ~20 days.
	targets := map[string]int{"Story": 10}
	dist := map[string]float64{"Story": 0.5, "Bug": 0.5}

	// 1. Run Stratified
	resStr := engine.RunMultiTypeDurationSimulation(targets, dist, 1000, true)
	// Insight: Stratified simulation with a shared cap will result in LONGER durations
	// if independent processes clash (Capacity Fallacy).
	if resStr.Percentiles.CoinToss < 24 || resStr.Percentiles.CoinToss > 29 {
		t.Errorf("Stratified: Expected median duration around 26 days (due to capacity clashes), got %.1f", resStr.Percentiles.CoinToss)
	}

	// 2. Compare with Pooled (if we disable eligibility)
	h.Meta["stratification_eligible"] = map[string]bool{"Story": false, "Bug": false}
	resPool := engine.RunMultiTypeDurationSimulation(targets, dist, 1000, true)

	t.Logf("INSIGHT: Capacity Fallacy observed. Stratified P50: %.1f (clashes), Pooled P50: %.1f (perfect sharing)", resStr.Percentiles.CoinToss, resPool.Percentiles.CoinToss)
	t.Logf("Stratified Spread (Inner80): %.1f, Pooled Spread: %.1f", resStr.Spread.Inner80, resPool.Spread.Inner80)
}

func TestParallelPerformance(t *testing.T) {
	// Simple smoke test to ensure parallel execution doesn't crash or hang
	buckets := []int{1, 0, 2, 1, 0, 1, 3}
	h := &Histogram{
		Counts: buckets,
		Meta: map[string]interface{}{
			"type_distribution": map[string]float64{"Story": 1.0},
		},
	}
	engine := NewEngine(h)

	// Run with high number of trials
	start := time.Now()
	res := engine.RunDurationSimulation(100, 10000)
	duration := time.Since(start)

	if res.Percentiles.CoinToss == 0 {
		t.Error("Expected non-zero result")
	}
	t.Logf("10k trials took %v", duration)
}
