package stats

import "slices"

// CalculatePercentile returns the value at percentile p (0.0–1.0) from a pre-sorted ascending slice.
// The slice must be sorted before calling. Returns 0 if the slice is empty.
func CalculatePercentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p < 0 {
		p = 0
	}
	idx := int(float64(len(sorted)) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// CalculateMedianDiscrete finds the median value in a slice of integers.
func CalculateMedianDiscrete(values []int) float64 {
	if len(values) == 0 {
		return 0
	}

	// Work on a copy to avoid mutating the original
	temp := make([]int, len(values))
	copy(temp, values)
	slices.Sort(temp)

	n := len(temp)
	if n%2 == 1 {
		return float64(temp[n/2])
	}
	return float64(temp[n/2-1]+temp[n/2]) / 2.0
}

// CalculateMedianContinuous finds the median value in a slice of floats.
func CalculateMedianContinuous(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	temp := make([]float64, len(values))
	copy(temp, values)
	slices.Sort(temp)

	n := len(temp)
	if n%2 == 1 {
		return temp[n/2]
	}
	return (temp[n/2-1] + temp[n/2]) / 2.0
}
