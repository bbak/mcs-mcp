package stats

import (
	"math"
	"mcs-mcp/internal/jira"
	"sort"
	"time"
)

// SPAPipelineConfig holds tunable parameters for the SPA pipeline.
// All fields have sensible defaults via DefaultSPAPipelineConfig().
type SPAPipelineConfig struct {
	// Regime detection (internal, used by adaptive window)
	MinRegimeBuckets int // Minimum sustained buckets for a trend reversal to count (default: 14)

	// Outlier filtering
	IQRMultiplier float64 // Tukey fence multiplier for sojourn time outliers (default: 1.5)

	// WIP scaling
	ScaleClampMin float64 // Lower bound for WIP scale factor (default: 0.5)
	ScaleClampMax float64 // Upper bound for WIP scale factor (default: 2.0)
	ScaleSnapZone float64 // Snap to 1.0 if within this fraction of 1.0 (default: 0.05)

	// Adaptive window (replaces mixing period and regime-start windowing)
	MinSampleDepartures int // Minimum completed items for a reliable distribution (default: 50)
	MaxLookbackDays     int // Hard ceiling — never sample older than this (default: 365)
}

// DefaultSPAPipelineConfig returns production defaults for the SPA pipeline.
func DefaultSPAPipelineConfig() SPAPipelineConfig {
	return SPAPipelineConfig{
		MinRegimeBuckets:    14,
		IQRMultiplier:       1.5,
		ScaleClampMin:       0.5,
		ScaleClampMax:       2.0,
		ScaleSnapZone:       0.05,
		MinSampleDepartures: 50,
		MaxLookbackDays:     365,
	}
}

// SPAPipelineResult carries all intermediate outputs for diagnostics and handler use.
type SPAPipelineResult struct {
	// Convergence gate
	ConvergenceStatus     string  `json:"convergence_status"`
	DivergenceScaleFactor float64 `json:"divergence_scale_factor"`
	DivergenceWarning     string  `json:"divergence_warning,omitempty"`

	// Regime detection (retained for diagnostics and adaptive window)
	RegimeBoundaries []time.Time `json:"regime_boundaries,omitempty"`

	// Outlier filtering
	OutliersRemoved []string `json:"outliers_removed,omitempty"`
	OutlierCount    int      `json:"outlier_count"`

	// Adaptive window
	AdaptiveWindowDepartures int  `json:"adaptive_window_departures"`
	AdaptiveWindowDays       int  `json:"adaptive_window_days"`
	RegimeBoundaryRespected  bool `json:"regime_boundary_respected"`

	// WIP/aging adjustment (populated post-histogram)
	WIPScaleFactor float64 `json:"wip_scale_factor"`

	// Effective window (set by adaptive window)
	EffectiveStart time.Time `json:"effective_start"`
	EffectiveEnd   time.Time `json:"effective_end"`

	// For handler use — not serialized
	FilteredFinished []jira.Issue         `json:"-"`
	RTSummary        ResidenceTimeSummary `json:"-"`
}

// RunSPAPipeline orchestrates the pre-histogram pipeline phase.
// Flow: convergence gate → regime detection → outlier filtering → adaptive window.
// WIP scaling (post-histogram) is separate because it needs the built histogram.
func RunSPAPipeline(
	rtResult *ResidenceTimeResult,
	items []ResidenceItem,
	finished []jira.Issue,
	windowStart, windowEnd time.Time,
	commitmentPoint string,
	statusWeights map[string]int,
	mappings map[string]StatusMetadata,
	window AnalysisWindow,
	cfg SPAPipelineConfig,
) *SPAPipelineResult {
	if rtResult == nil || len(rtResult.Series) == 0 {
		return &SPAPipelineResult{
			EffectiveStart:        windowStart,
			EffectiveEnd:          windowEnd,
			FilteredFinished:      finished,
			DivergenceScaleFactor: 1.0,
			WIPScaleFactor:        1.0,
		}
	}

	result := &SPAPipelineResult{
		EffectiveEnd:          windowEnd,
		RTSummary:             rtResult.Summary,
		DivergenceScaleFactor: 1.0,
		WIPScaleFactor:        1.0,
	}

	// 1. Convergence gate
	status, scaleFactor, warning := EvaluateConvergenceGate(rtResult.Summary, cfg)
	result.ConvergenceStatus = status
	result.DivergenceScaleFactor = scaleFactor
	result.DivergenceWarning = warning

	// 2. Regime boundary detection (internal, for adaptive window)
	_, boundaries := DetectRegimeBoundaries(rtResult.Series, cfg)
	result.RegimeBoundaries = boundaries

	// 3. Outlier filtering (over full series, before adaptive window)
	filtered, removedKeys := FilterOutliersByConvergence(
		items, finished, windowStart, windowEnd,
		commitmentPoint, statusWeights, mappings, window, cfg,
	)
	result.FilteredFinished = filtered
	result.OutliersRemoved = removedKeys
	result.OutlierCount = len(removedKeys)

	// Build outlier key set for adaptive window
	outlierKeys := make(map[string]bool, len(removedKeys))
	for _, key := range removedKeys {
		outlierKeys[key] = true
	}

	// 4. Adaptive window: walk backwards through departures
	adaptiveStart, departures, boundaryRespected := ComputeAdaptiveWindow(
		rtResult.Series, windowEnd, boundaries, outlierKeys, items, cfg,
	)
	result.AdaptiveWindowDepartures = departures
	result.AdaptiveWindowDays = CalendarDaysBetween(adaptiveStart, windowEnd)
	result.RegimeBoundaryRespected = boundaryRespected

	// Don't go earlier than the original window start
	effectiveStart := adaptiveStart
	if effectiveStart.Before(windowStart) {
		effectiveStart = windowStart
	}
	result.EffectiveStart = effectiveStart

	return result
}

// ComputeAdaptiveWindow walks backwards through the SPA series from the vantage
// date, accumulating departures. The algorithm:
//
// Phase 1: Walk back until MinSampleDepartures is reached, ignoring regime
// boundaries. This ensures a minimum sample size. Track which boundaries
// were crossed and record the series index where the threshold was met.
//
// Phase 2 (after threshold): Check if any crossed regime segment was stationary
// (Λ/Θ < flowImbalanceThreshold). If so, the 50 departures are representative
// enough — stop at the threshold point. If all crossed segments were
// non-stationary, continue walking back to the next regime boundary for a
// cleaner, regime-aligned window edge.
//
// The hard ceiling MaxLookbackDays applies throughout.
// Outlier departures are excluded from the count.
func ComputeAdaptiveWindow(
	series []ResidenceTimeBucket,
	vantageDate time.Time,
	regimeBoundaries []time.Time,
	outlierKeys map[string]bool,
	items []ResidenceItem,
	cfg SPAPipelineConfig,
) (windowStart time.Time, departureCount int, regimeBoundaryRespected bool) {
	n := len(series)
	if n == 0 {
		return vantageDate, 0, false
	}

	// Build outlier departures per day: how many outlier items departed on each date
	outlierDeparturesPerDay := make(map[string]int)
	if len(outlierKeys) > 0 {
		for _, item := range items {
			if item.End != nil && outlierKeys[item.Key] {
				dayKey := item.End.Format("2006-01-02")
				outlierDeparturesPerDay[dayKey]++
			}
		}
	}

	// Sort regime boundaries descending for efficient lookup during walk-back
	sortedBoundaries := make([]time.Time, len(regimeBoundaries))
	copy(sortedBoundaries, regimeBoundaries)
	sort.Slice(sortedBoundaries, func(i, j int) bool {
		return sortedBoundaries[i].After(sortedBoundaries[j])
	})

	// Find the starting index: last bucket at or before vantageDate
	startIdx := n - 1
	for startIdx > 0 && series[startIdx].Date.After(vantageDate) {
		startIdx--
	}

	maxLookback := time.Duration(cfg.MaxLookbackDays) * 24 * time.Hour
	cumulativeDepartures := 0
	windowStart = series[startIdx].Date

	// Track boundary crossings during Phase 1
	nextBoundaryIdx := 0
	for nextBoundaryIdx < len(sortedBoundaries) && sortedBoundaries[nextBoundaryIdx].After(vantageDate) {
		nextBoundaryIdx++
	}
	boundariesCrossedInPhase1 := 0

	// Phase 1 state
	thresholdMet := false
	thresholdWindowStart := windowStart
	thresholdDepartures := 0

	for i := startIdx; i >= 0; i-- {
		bucketDate := series[i].Date

		// Hard ceiling: don't go back more than MaxLookbackDays
		if vantageDate.Sub(bucketDate) > maxLookback {
			break
		}

		// Check if we're at or crossing a boundary
		atBoundary := false
		if nextBoundaryIdx < len(sortedBoundaries) {
			boundary := sortedBoundaries[nextBoundaryIdx]
			if !boundary.Before(bucketDate) {
				atBoundary = true
			}
		}

		if thresholdMet && atBoundary {
			// Phase 2: stop at this boundary
			regimeBoundaryRespected = true
			return windowStart, cumulativeDepartures, regimeBoundaryRespected
		}

		// Consume boundary (Phase 1 or Phase 2 walk)
		if atBoundary {
			if !thresholdMet {
				boundariesCrossedInPhase1++
			}
			nextBoundaryIdx++
		}

		// Compute per-bucket departures
		var bucketDepartures int
		if i > 0 {
			bucketDepartures = series[i].D - series[i-1].D
		} else {
			bucketDepartures = series[0].D
		}

		// Subtract outlier departures for this bucket's date
		if outlierCount, ok := outlierDeparturesPerDay[bucketDate.Format("2006-01-02")]; ok {
			bucketDepartures -= outlierCount
			if bucketDepartures < 0 {
				bucketDepartures = 0
			}
		}

		cumulativeDepartures += bucketDepartures
		windowStart = bucketDate

		// Threshold just reached — decide whether to enter Phase 2
		if !thresholdMet && cumulativeDepartures >= cfg.MinSampleDepartures {
			thresholdMet = true
			thresholdWindowStart = windowStart
			thresholdDepartures = cumulativeDepartures

			// If no boundaries were crossed, the 50 departures are within the
			// current regime — stop immediately.
			if boundariesCrossedInPhase1 == 0 {
				return thresholdWindowStart, thresholdDepartures, false
			}

			// Check if any crossed regime segment was stationary (Λ/Θ balanced).
			// If so, the data is representative enough — stop at the threshold point.
			if anyCrossedSegmentStationary(series, sortedBoundaries, startIdx, i, vantageDate) {
				return thresholdWindowStart, thresholdDepartures, false
			}

			// All crossed segments were non-stationary — enter Phase 2:
			// continue to the next regime boundary for a cleaner edge.
		}
	}

	// Ran out of data or hit max lookback.
	// If we were in Phase 2 and never found a boundary, return what we have.
	return windowStart, cumulativeDepartures, regimeBoundaryRespected
}

// anyCrossedSegmentStationary checks whether any regime segment between
// bucketIdx and startIdx contains a stationary period (Λ/Θ < 1.3).
// It examines the Λ/Θ ratio at the end of each segment (the bucket just
// before each boundary).
func anyCrossedSegmentStationary(
	series []ResidenceTimeBucket,
	sortedBoundaries []time.Time,
	startIdx, endIdx int,
	vantageDate time.Time,
) bool {
	for _, boundary := range sortedBoundaries {
		if boundary.After(vantageDate) {
			continue
		}
		// Find the bucket at the boundary
		for j := startIdx; j >= endIdx; j-- {
			if !series[j].Date.After(boundary) {
				// This is the bucket at or just after the boundary — check Λ/Θ
				if series[j].Theta > 0 {
					ratio := series[j].Lambda / series[j].Theta
					if ratio <= flowImbalanceThreshold {
						return true
					}
				}
				break
			}
		}
	}
	return false
}

// EvaluateConvergenceGate checks whether forecasting is meaningful by examining
// the convergence status from the sample path. If diverging, it computes a
// scaling factor from the implied throughput vs. observed departure rate.
func EvaluateConvergenceGate(summary ResidenceTimeSummary, cfg SPAPipelineConfig) (string, float64, string) {
	status := summary.Convergence
	scaleFactor := 1.0
	warning := ""

	if status == "diverging" {
		// Implied throughput from Little's Law: λ_implied = L(T) / w(T)
		// Compare to observed departure rate Θ(T)
		if summary.FinalW > 0 && summary.FinalTheta > 0 {
			lambdaImplied := summary.FinalL / summary.FinalW
			scaleFactor = lambdaImplied / summary.FinalTheta
			scaleFactor = clampScale(scaleFactor, cfg)
		}
		warning = "CAUTION: Process is diverging — residence time is still climbing. " +
			"The throughput histogram has been scaled to reflect implied capacity constraints."
	}

	return status, scaleFactor, warning
}

// DetectRegimeBoundaries identifies sustained trend reversals in Λ(T) and w(T)
// to find regime boundaries. Returns the start of the current (most recent) regime
// and all detected boundaries.
func DetectRegimeBoundaries(series []ResidenceTimeBucket, cfg SPAPipelineConfig) (time.Time, []time.Time) {
	n := len(series)
	if n < cfg.MinRegimeBuckets*2 {
		// Not enough data for regime detection
		return series[0].Date, nil
	}

	// Compute first differences of Lambda and W
	type diff struct {
		dLambda float64
		dW      float64
		date    time.Time
	}
	diffs := make([]diff, n-1)
	for i := 1; i < n; i++ {
		diffs[i-1] = diff{
			dLambda: series[i].Lambda - series[i-1].Lambda,
			dW:      series[i].W - series[i-1].W,
			date:    series[i].Date,
		}
	}

	// Detect sign reversals in the smoothed trend of Lambda and W.
	// We use a running sign over MinRegimeBuckets to filter transient noise.
	var boundaries []time.Time

	prevLambdaSign := signOfSum(diffs, 0, cfg.MinRegimeBuckets, func(d diff) float64 { return d.dLambda })
	prevWSign := signOfSum(diffs, 0, cfg.MinRegimeBuckets, func(d diff) float64 { return d.dW })

	for i := cfg.MinRegimeBuckets; i <= len(diffs)-cfg.MinRegimeBuckets; i++ {
		curLambdaSign := signOfSum(diffs, i, cfg.MinRegimeBuckets, func(d diff) float64 { return d.dLambda })
		curWSign := signOfSum(diffs, i, cfg.MinRegimeBuckets, func(d diff) float64 { return d.dW })

		lambdaReversed := prevLambdaSign != 0 && curLambdaSign != 0 && prevLambdaSign != curLambdaSign
		wReversed := prevWSign != 0 && curWSign != 0 && prevWSign != curWSign

		if lambdaReversed || wReversed {
			boundaries = append(boundaries, diffs[i].date)
			prevLambdaSign = curLambdaSign
			prevWSign = curWSign
		} else {
			if curLambdaSign != 0 {
				prevLambdaSign = curLambdaSign
			}
			if curWSign != 0 {
				prevWSign = curWSign
			}
		}
	}

	if len(boundaries) == 0 {
		return series[0].Date, nil
	}

	// Current regime starts at the most recent boundary
	return boundaries[len(boundaries)-1], boundaries
}

// FilterOutliersByConvergence identifies items with extreme sojourn times
// (Q3 + IQRMultiplier * IQR) and removes them as a batch if doing so
// improves convergence quality (lower Beta1 in the 1/T regression).
func FilterOutliersByConvergence(
	items []ResidenceItem,
	finished []jira.Issue,
	effectiveStart, effectiveEnd time.Time,
	commitmentPoint string,
	statusWeights map[string]int,
	mappings map[string]StatusMetadata,
	window AnalysisWindow,
	cfg SPAPipelineConfig,
) ([]jira.Issue, []string) {
	// Build a map of issue key → sojourn time from residence items within the effective window
	sojournTimes := make(map[string]float64)
	for _, item := range items {
		if item.End == nil {
			continue // active items have no sojourn time
		}
		if item.Start.Before(effectiveStart) && !item.PreWindow {
			continue // outside effective window
		}
		sojourn := item.End.Sub(item.Start).Hours() / 24.0
		if sojourn > 0 {
			sojournTimes[item.Key] = sojourn
		}
	}

	if len(sojournTimes) < 4 {
		return finished, nil // not enough data for IQR
	}

	// Compute Q1, Q3, IQR
	vals := make([]float64, 0, len(sojournTimes))
	for _, v := range sojournTimes {
		vals = append(vals, v)
	}
	sort.Float64s(vals)

	q1 := percentileOfSorted(vals, 25)
	q3 := percentileOfSorted(vals, 75)
	iqr := q3 - q1
	fence := q3 + cfg.IQRMultiplier*iqr

	// Identify outlier keys
	outlierKeys := make(map[string]bool)
	for key, sojourn := range sojournTimes {
		if sojourn > fence {
			outlierKeys[key] = true
		}
	}

	if len(outlierKeys) == 0 {
		return finished, nil
	}

	// Compute baseline convergence with all items
	baselineCA := assessConvergence(computeSeriesForItems(items, window))

	// Compute convergence without outliers
	filteredItems := make([]ResidenceItem, 0, len(items))
	for _, item := range items {
		if !outlierKeys[item.Key] {
			filteredItems = append(filteredItems, item)
		}
	}
	filteredCA := assessConvergence(computeSeriesForItems(filteredItems, window))

	// Compare: does removing outliers improve convergence?
	if !convergenceImproved(baselineCA, filteredCA) {
		return finished, nil // routine variation — keep everything
	}

	// Exceptional variation — remove from finished issues
	filteredFinished := make([]jira.Issue, 0, len(finished))
	var removedKeys []string
	for _, issue := range finished {
		if outlierKeys[issue.Key] {
			removedKeys = append(removedKeys, issue.Key)
		} else {
			filteredFinished = append(filteredFinished, issue)
		}
	}

	return filteredFinished, removedKeys
}

// computeSeriesForItems is a thin wrapper around ComputeResidenceTimeSeries
// that extracts the series from a set of residence items.
func computeSeriesForItems(items []ResidenceItem, window AnalysisWindow) []ResidenceTimeBucket {
	if len(items) == 0 {
		return nil
	}
	result := ComputeResidenceTimeSeries(items, window)
	if result == nil {
		return nil
	}
	return result.Series
}

// ComputeWIPScaleFactor computes the ratio of Little's Law implied throughput
// to the histogram's observed mean throughput. Used for post-histogram scaling.
func ComputeWIPScaleFactor(summary ResidenceTimeSummary, histogramMeanThroughput float64, cfg SPAPipelineConfig) float64 {
	if summary.FinalW <= 0 || histogramMeanThroughput <= 0 {
		return 1.0
	}

	lambdaImplied := summary.FinalL / summary.FinalW
	scaleFactor := lambdaImplied / histogramMeanThroughput
	return clampScale(scaleFactor, cfg)
}

// convergenceImproved returns true if the filtered assessment is strictly better
// than the baseline. "Better" means: a more favorable label, or the same label
// with a smaller |Beta1| (tighter 1/T fit).
func convergenceImproved(baseline, filtered convergenceAssessment) bool {
	labelRank := map[string]int{
		"insufficient_data": 0,
		"diverging":         1,
		"metastable":        2,
		"converging":        3,
	}

	baseRank := labelRank[baseline.Label]
	filtRank := labelRank[filtered.Label]

	if filtRank > baseRank {
		return true // label improved
	}
	if filtRank == baseRank && baseRank >= 1 {
		// Same label — compare |Beta1|: smaller is better (less slope)
		return math.Abs(filtered.Beta1) < math.Abs(baseline.Beta1)
	}
	return false
}

// signOfSum returns +1, -1, or 0 for the sign of the sum of values in
// diffs[start:start+length] extracted by the given function.
func signOfSum[T any](diffs []T, start, length int, extract func(T) float64) int {
	if start+length > len(diffs) {
		return 0
	}
	var sum float64
	for i := start; i < start+length; i++ {
		sum += extract(diffs[i])
	}
	if sum > 0 {
		return 1
	}
	if sum < 0 {
		return -1
	}
	return 0
}

// percentileOfSorted returns the p-th percentile of a sorted float64 slice.
func percentileOfSorted(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := p / 100.0 * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := rank - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// clampScale applies bounds and snap-to-1.0 logic to a scale factor.
func clampScale(factor float64, cfg SPAPipelineConfig) float64 {
	if factor < cfg.ScaleClampMin {
		factor = cfg.ScaleClampMin
	}
	if factor > cfg.ScaleClampMax {
		factor = cfg.ScaleClampMax
	}
	if math.Abs(factor-1.0) <= cfg.ScaleSnapZone {
		factor = 1.0
	}
	return factor
}
