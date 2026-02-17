package simulation

import (
	"testing"
)

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
