package simulation

import (
	"testing"
)

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

	res := engine.RunMultiTypeDurationSimulation(targets, dist, 1000)

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

	res := engine.RunMultiTypeDurationSimulation(targets, dist, 1000)

	if res.Percentiles.CoinToss < 70 || res.Percentiles.CoinToss > 130 {
		t.Errorf("Expected median duration around 100 days, got %.1f", res.Percentiles.CoinToss)
	}

	if res.BackgroundItemsPredicted["Bug"] < 70 || res.BackgroundItemsPredicted["Bug"] > 110 {
		t.Errorf("Expected around 90 background bugs, got %d", res.BackgroundItemsPredicted["Bug"])
	}
}
