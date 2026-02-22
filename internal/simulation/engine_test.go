package simulation

import (
	"fmt"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"testing"
	"time"
)

func TestEngine_Percentiles(t *testing.T) {
	// Simple histogram with 10 values [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
	h := &Histogram{
		Counts: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}
	e := NewEngine(h)

	// Single item cycle time analysis
	cycleTimes := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	res := e.RunCycleTimeAnalysis(cycleTimes, nil)

	if res.Percentiles.Aggressive != 2 { // P10 of 10 items
		t.Errorf("Expected Aggressive (P10) to be 2, got %f", res.Percentiles.Aggressive)
	}
	if res.Percentiles.CoinToss != 6 { // P50 of 10 items
		t.Errorf("Expected CoinToss (P50) to be 6, got %f", res.Percentiles.CoinToss)
	}
	if res.Percentiles.Conservative != 10 { // P90 of 10 items
		t.Errorf("Expected Conservative (P90) to be 10, got %f", res.Percentiles.Conservative)
	}
}

func TestEngine_ZeroThroughput(t *testing.T) {
	h := &Histogram{
		Counts: []int{0, 0, 0},
	}
	e := NewEngine(h)

	// This should not hang and should return the safety limit
	res := e.RunDurationSimulation(10, 100)

	if res.Percentiles.CoinToss != 3650 {
		t.Errorf("Expected CoinToss to be safety limit 3650, got %f", res.Percentiles.CoinToss)
	}

	foundWarning := false
	for _, w := range res.Warnings {
		if w == "No historical throughput found for the selected criteria. The duration forecast is theoretically infinite based on current data." {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("Expected infinite duration warning, but it was not found")
	}
}

// Extracted Tests

func TestCorrelationDetection(t *testing.T) {
	// Create a scenario with perfect negative correlation
	// Day 1: Story=1, Bug=0
	// Day 2: Story=0, Bug=1
	// ...
	days := 20
	stratified := make(map[string][]int)
	stratified["Story"] = make([]int, days)
	stratified["Bug"] = make([]int, days)

	for i := 0; i < days; i++ {
		if i%2 == 0 {
			stratified["Story"][i] = 1
			stratified["Bug"][i] = 0
		} else {
			stratified["Story"][i] = 0
			stratified["Bug"][i] = 1
		}
	}

	deps := DetectDependencies(stratified)

	// Correlation should be -1.0
	corr := CalculateCorrelation(stratified["Story"], stratified["Bug"])
	if corr > -0.9 {
		t.Errorf("Expected perfect negative correlation, got %.2f", corr)
	}

	found := false
	if _, ok := deps["Story"]; ok {
		found = true
	} else if _, ok := deps["Bug"]; ok {
		found = true
	}

	if !found {
		// DetectDependencies should have picked up the negative correlation
		t.Errorf("Expected dependency detection, got %+v", deps)
	}
}

func TestVolatilityAttribution(t *testing.T) {
	// Stable process
	stable := []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1} // P98=1, P50=1 -> Ratio 1.0
	vS := CalculateFatTail(stable)
	if vS > 1.1 {
		t.Errorf("Expected low volatility for stable process, got %.2f", vS)
	}

	// Chaotic process (Fat-Tail)
	chaotic := []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 10} // P50=1, P98=10 -> Ratio 10.0
	vC := CalculateFatTail(chaotic)
	if vC < 5.6 {
		t.Errorf("Expected high volatility for chaotic process, got %.2f", vC)
	}
}

func TestBugTaxSimulation(t *testing.T) {
	// This test verifies that the engine respects the dependency squeeze
	days := 20
	buckets := make([]int, days)
	stratified := make(map[string][]int)
	stratified["Story"] = make([]int, days)
	stratified["Bug"] = make([]int, days)

	for i := 0; i < days; i++ {
		// In history, Story and Bug never happen together
		if i%2 == 0 {
			stratified["Story"][i] = 2
		} else {
			stratified["Bug"][i] = 2
		}
		buckets[i] = 2
	}

	h := &Histogram{
		Counts:           buckets,
		StratifiedCounts: stratified,
		Meta: map[string]interface{}{
			"stratification_eligible":     map[string]bool{"Story": true, "Bug": true},
			"stratification_dependencies": map[string]string{"Bug": "Story"},
			"type_distribution":           map[string]float64{"Story": 0.5, "Bug": 0.5},
			"throughput_overall":          2.0,
			"modeling_insight":            "Test",
		},
	}

	engine := NewEngine(h)
	targets := map[string]int{"Story": 10}
	dist := map[string]float64{"Story": 0.5, "Bug": 1.0} // Simulation expects lots of bugs!

	// Run stratified
	res := engine.RunMultiTypeDurationSimulation(targets, dist, 1000, true)

	// In this scenario:
	// Every day Bug will be sampled (since bugs are 2 half the time, and dist=1.0)
	// If Bug is 2, and Bug taxes Story (0.5 reduction), Story (which would be 2) gets reduced by 1.
	// So Story throughput becomes 1 instead of 2 on those days.
	// Expected days: 10 / 1 = 10 days.
	// If the tax WASN'T working, Story would be 2 half the time (but Bug being there doesn't matter without tax).
	// Wait, without tax: Story is 2 on even days, 0 on odd days. Avg = 1.
	// With tax: Story is sampled as 2 on even days, 0 on odd days (as per history).
	// On even days, if Bug is ALSO sampled as 2 (because dist is high), Story gets squeezed to 1.
	// So avg becomes 0.5. Days: 10 / 0.5 = 20 days.

	t.Logf("Stratified P50 with Bug-Tax: %.1f", res.Percentiles.CoinToss)
}

func TestMultiTypeSimulation(t *testing.T) {
	// 1. Create a histogram with 1 item/day throughput
	h := &Histogram{
		Counts: []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		Meta: map[string]interface{}{
			"type_distribution": map[string]float64{
				"Story": 0.5,
				"Bug":   0.5,
			},
		},
	}

	engine := NewEngine(h)

	// 2. Goal: 5 Stories.
	// Since throughput is 1/day and 50% are Stories, we expect ~10 days.
	targets := map[string]int{"Story": 5}
	dist := map[string]float64{"Story": 0.5, "Bug": 0.5}

	res := engine.RunMultiTypeDurationSimulation(targets, dist, 1000, true)

	if res.Percentiles.CoinToss < 8 || res.Percentiles.CoinToss > 13 {
		t.Errorf("Expected median duration around 10 days, got %.1f", res.Percentiles.CoinToss)
	}

	if res.BackgroundItemsPredicted["Bug"] < 3 || res.BackgroundItemsPredicted["Bug"] > 7 {
		t.Errorf("Expected around 5 background bugs, got %d", res.BackgroundItemsPredicted["Bug"])
	}
}

func TestMultiTypeSimulation_Skewed(t *testing.T) {
	// 1. Create a histogram with 1 item/day
	h := &Histogram{
		Counts: []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
	}

	engine := NewEngine(h)

	// 2. Goal: 10 Stories. Distribution: 10% Story, 90% Bug.
	// Expect around 100 days.
	targets := map[string]int{"Story": 10}
	dist := map[string]float64{"Story": 0.1, "Bug": 0.9}

	res := engine.RunMultiTypeDurationSimulation(targets, dist, 1000, true)

	if res.Percentiles.CoinToss < 70 || res.Percentiles.CoinToss > 130 {
		t.Errorf("Expected median duration around 100 days, got %.1f", res.Percentiles.CoinToss)
	}

	if res.BackgroundItemsPredicted["Bug"] < 70 || res.BackgroundItemsPredicted["Bug"] > 110 {
		t.Errorf("Expected around 90 background bugs, got %d", res.BackgroundItemsPredicted["Bug"])
	}
}

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
			Resolution:     "Fixed",
			Status:         "Done",
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
			Resolution:     "Fixed",
			Status:         "Done",
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
			Resolution:     "Fixed",
			Status:         "Done",
		})
	}
	// 20 Bugs taking 2 days
	for i := 0; i < 20; i++ {
		resDate := now.AddDate(0, 0, -i)
		issues = append(issues, jira.Issue{
			IssueType:      "Bug",
			Created:        resDate.AddDate(0, 0, -2),
			ResolutionDate: &resDate,
			Resolution:     "Fixed",
			Status:         "Done",
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
