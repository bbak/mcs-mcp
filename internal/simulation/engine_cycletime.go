package simulation

import (
	"slices"
)

// RunCycleTimeAnalysis calculates percentiles from a list of historical cycle times (in days).
func (e *Engine) RunCycleTimeAnalysis(cycleTimes []float64, ctByType map[string][]float64) Result {
	if len(cycleTimes) == 0 {
		return Result{}
	}

	// Sort a local copy — never mutate the caller's slice, which may be date-ordered.
	sorted := make([]float64, len(cycleTimes))
	copy(sorted, cycleTimes)
	slices.Sort(sorted)

	res := Result{
		Percentiles:      percentilesFromSorted(sorted),
		Spread:           spreadFromSorted(sorted),
		PercentileLabels: getPercentileLabels("cycle_time"),
	}

	// Stratified Analysis
	if len(ctByType) > 0 {
		res.TypeSLEs = make(map[string]Percentiles)
		for t, cts := range ctByType {
			if len(cts) == 0 {
				continue
			}
			typeSorted := make([]float64, len(cts))
			copy(typeSorted, cts)
			slices.Sort(typeSorted)
			res.TypeSLEs[t] = percentilesFromSorted(typeSorted)
		}
	}

	e.assessPredictability(&res)

	return res
}
