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
	res := e.RunCycleTimeAnalysis(cycleTimes)

	if res.Aggressive != 2 { // P10 of 10 items
		t.Errorf("Expected Aggressive (P10) to be 2, got %f", res.Aggressive)
	}
	if res.CoinToss != 6 { // P50 of 10 items
		t.Errorf("Expected CoinToss (P50) to be 6, got %f", res.CoinToss)
	}
	if res.Conservative != 10 { // P90 of 10 items
		t.Errorf("Expected Conservative (P90) to be 10, got %f", res.Conservative)
	}
}
