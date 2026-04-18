package stats

import "slices"

// PercentileIndex returns a safe index into a sorted slice of length n for percentile p (0..1).
// Clamps to [0, n-1] to prevent out-of-bounds panics near p=1.0.
func PercentileIndex(n int, p float64) int {
	i := int(float64(n) * p)
	if i >= n {
		i = n - 1
	}
	if i < 0 {
		i = 0
	}
	return i
}

// PercentileOfSorted returns the value at percentile p (0..1) of an already-sorted ascending slice.
// Returns 0 for an empty slice.
func PercentileOfSorted(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[PercentileIndex(len(sorted), p)]
}

// PercentileOf sorts a copy of values and returns the value at percentile p (0..1).
// Does not mutate the input slice.
func PercentileOf(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	slices.Sort(sorted)
	return PercentileOfSorted(sorted, p)
}
