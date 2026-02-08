package stats

import (
	"testing"
)

func TestCalculateMedianDiscrete(t *testing.T) {
	tests := []struct {
		name     string
		values   []int
		expected float64
	}{
		{"Empty", []int{}, 0},
		{"SingleItem", []int{5}, 5},
		{"OddCount", []int{1, 3, 2, 4, 5}, 3},
		{"EvenCount", []int{1, 2, 3, 4}, 2.5},
		{"Unsorted", []int{10, 2, 8, 4, 6}, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculateMedianDiscrete(tt.values); got != tt.expected {
				t.Errorf("CalculateMedianDiscrete() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCalculateMedianContinuous(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{"Empty", []float64{}, 0},
		{"SingleItem", []float64{5.5}, 5.5},
		{"OddCount", []float64{1.1, 3.3, 2.2, 4.4, 5.5}, 3.3},
		{"EvenCount", []float64{1.1, 2.2, 3.3, 4.4}, 2.75},
		{"Unsorted", []float64{10.5, 2.5, 8.5, 4.5, 6.5}, 6.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculateMedianContinuous(tt.values); got != tt.expected {
				t.Errorf("CalculateMedianContinuous() = %v, want %v", got, tt.expected)
			}
		})
	}
}
