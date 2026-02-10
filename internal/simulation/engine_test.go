package simulation

import (
	"testing"
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

	if res.Percentiles.CoinToss != 20000 {
		t.Errorf("Expected CoinToss to be safety limit 20000, got %f", res.Percentiles.CoinToss)
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
