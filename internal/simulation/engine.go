package simulation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/rs/zerolog/log"
)

// Composition represents the scope breakdown of a forecast.
type Composition struct {
	ExistingBacklog int `json:"existing_backlog"`
	WIP             int `json:"wip"`
	AdditionalItems int `json:"additional_items"`
	Total           int `json:"total"`
}

// ThroughputTrend represents the historical velocity direction.
type ThroughputTrend struct {
	Direction        string  `json:"direction"` // "Increasing", "Declining", "Stable"
	PercentageChange float64 `json:"percentage_change"`
}

// Engine performs the Monte-Carlo simulation.
type Engine struct {
	histogram *Histogram
	rng       *rand.Rand
}

// Percentiles holds the probabilistic outcomes of a simulation.
type Percentiles struct {
	Aggressive    float64 `json:"aggressive"`     // P10
	Unlikely      float64 `json:"unlikely"`       // P30
	CoinToss      float64 `json:"coin_toss"`      // P50
	Probable      float64 `json:"probable"`       // P70
	Likely        float64 `json:"likely"`         // P85
	Conservative  float64 `json:"conservative"`   // P90
	Safe          float64 `json:"safe"`           // P95
	AlmostCertain float64 `json:"almost_certain"` // P98
}

// SpreadMetrics holds statistical dispersion data.
type SpreadMetrics struct {
	IQR     float64 `json:"iqr"`      // P75-P25
	Inner80 float64 `json:"inner_80"` // P90-P10
}

// Result holds the percentiles of a simulation or analysis.
type Result struct {
	Percentiles       Percentiles            `json:"percentiles"`
	Spread            SpreadMetrics          `json:"spread"`
	FatTailRatio      float64                `json:"fat_tail_ratio"`       // P98/P50 (Kanban University heuristic)
	TailToMedianRatio float64                `json:"tail_to_median_ratio"` // P85/P50 (Volatility heuristic)
	Predictability    string                 `json:"predictability"`
	Context           map[string]interface{} `json:"context,omitempty"`
	Warnings          []string               `json:"warnings,omitempty"`
	StabilityRatio    float64                `json:"stability_ratio,omitempty"`
	StaleWIPCount     int                    `json:"stale_wip_count,omitempty"`

	// Advanced Analytics
	Composition              Composition       `json:"composition"`
	WIPAgeDistribution       map[string]int    `json:"wip_age_distribution,omitempty"`
	ThroughputTrend          ThroughputTrend   `json:"throughput_trend"`
	Insights                 []string          `json:"insights,omitempty"`
	PercentileLabels         map[string]string `json:"percentile_labels,omitempty"`
	BackgroundItemsPredicted map[string]int    `json:"background_items_predicted,omitempty"`
}

func NewEngine(h *Histogram) *Engine {
	return &Engine{
		histogram: h,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RunDurationSimulation is a backward-compatible wrapper for simple backlog forecasts.
// It assumes the backlog follows the historical distribution.
func (e *Engine) RunDurationSimulation(backlogSize int, trials int) Result {
	targets := make(map[string]int)
	dist := make(map[string]float64)

	if e.histogram != nil {
		if d, ok := e.histogram.Meta["type_distribution"].(map[string]float64); ok && len(d) > 0 {
			dist = d
			for t, p := range dist {
				targets[t] = int(math.Round(float64(backlogSize) * p))
			}
		}
	}

	// Fallback if no distribution found
	if len(targets) == 0 {
		targets["Unknown"] = backlogSize
		dist["Unknown"] = 1.0
	}

	return e.RunMultiTypeDurationSimulation(targets, dist, trials, true)
}

// RunMultiTypeDurationSimulation predicts how long it takes to finish specific targets while others consume capacity.
func (e *Engine) RunMultiTypeDurationSimulation(targets map[string]int, distribution map[string]float64, trials int, expansionEnabled bool) Result {
	if e.histogram == nil || len(e.histogram.Counts) == 0 {
		return Result{}
	}

	// If expansion is disabled, we re-normalize the distribution to only include types in targets.
	// This models a "Closed System" where 100% of capacity goes to the targets.
	finalDist := make(map[string]float64)
	if !expansionEnabled {
		targetTotalProb := 0.0
		for t := range targets {
			targetTotalProb += distribution[t]
		}
		if targetTotalProb > 0 {
			for t := range targets {
				finalDist[t] = distribution[t] / targetTotalProb
			}
		} else {
			// Fallback: equal distribution if targets aren't in history
			prob := 1.0 / float64(len(targets))
			for t := range targets {
				finalDist[t] = prob
			}
		}
	} else {
		for t, p := range distribution {
			finalDist[t] = p
		}
	}

	durations := make([]int, trials)
	backgroundCounts := make(map[string][]int)
	for t := range finalDist {
		backgroundCounts[t] = make([]int, trials)
	}

	log.Info().Int("trials", trials).Interface("targets", targets).Bool("expansion", expansionEnabled).Msg("Starting multi-type duration simulation")
	for i := 0; i < trials; i++ {
		duration, bg := e.simulateDurationTrialWithTypeMix(targets, finalDist)
		durations[i] = duration
		for t, c := range bg {
			backgroundCounts[t][i] = c
		}
	}

	sort.Ints(durations)

	// Calculate median background items
	medianBG := make(map[string]int)
	for t, counts := range backgroundCounts {
		sort.Ints(counts)
		medianBG[t] = counts[trials/2]
	}

	res := Result{
		Percentiles: Percentiles{
			Aggressive:    float64(durations[int(float64(trials)*0.10)]),
			Unlikely:      float64(durations[int(float64(trials)*0.30)]),
			CoinToss:      float64(durations[int(float64(trials)*0.50)]),
			Probable:      float64(durations[int(float64(trials)*0.70)]),
			Likely:        float64(durations[int(float64(trials)*0.85)]),
			Conservative:  float64(durations[int(float64(trials)*0.90)]),
			Safe:          float64(durations[int(float64(trials)*0.95)]),
			AlmostCertain: float64(durations[int(float64(trials)*0.98)]),
		},
		Spread: SpreadMetrics{
			IQR:     float64(durations[int(float64(trials)*0.75)] - durations[int(float64(trials)*0.25)]),
			Inner80: float64(durations[int(float64(trials)*0.90)] - durations[int(float64(trials)*0.10)]),
		},
		PercentileLabels: map[string]string{
			"aggressive":     "P10 (Aggressive / Best Case)",
			"unlikely":       "P30 (Unlikely / High Risk)",
			"coin_toss":      "P50 (Coin Toss / Median)",
			"probable":       "P70 (Probable)",
			"likely":         "P85 (Likely / Professional Standard)",
			"conservative":   "P90 (Conservative / Buffer)",
			"safe":           "P95 (Safe Bet)",
			"almost_certain": "P98 (Limit / Extreme Outlier Boundaries)",
		},
		BackgroundItemsPredicted: medianBG,
	}

	e.assessPredictability(&res)

	// Check for infinite duration / zero throughput
	isInfinite := true
	for _, c := range e.histogram.Counts {
		if c > 0 {
			isInfinite = false
			break
		}
	}

	if isInfinite {
		log.Warn().Msg("Simulation resulted in infinite duration due to zero throughput")
		res.Warnings = append(res.Warnings, "No historical throughput found for the selected criteria. The duration forecast is theoretically infinite based on current data.")
	} else if durations[int(float64(trials)*0.50)] >= 3650 { // > 10 years
		res.Warnings = append(res.Warnings, "WARNING: Forecast exceeds 10 years. This usually indicates 'Throughput Collapse' due to overly restrictive filters (Issue Types or Resolutions).")
	}

	// Resolution Density Warning
	if dropped, ok := e.histogram.Meta["dropped_by_resolution"].(int); ok && dropped > 0 {
		analyzed := e.histogram.Meta["issues_analyzed"].(int)
		total := analyzed + dropped
		if total > 0 && float64(analyzed)/float64(total) < 0.2 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("CAUTION: Low Resolution Density. %.1f%% of resolved items were excluded by your resolution filter. This may skew results if 'delivered' items are missing. Check your 'resolutions' parameter.", float64(analyzed)/float64(total)*100))
		}
	}

	// Window exclusion warning
	if droppedWindow, ok := e.histogram.Meta["dropped_by_window"].(int); ok && droppedWindow > 0 {
		analyzed := e.histogram.Meta["issues_analyzed"].(int)
		total := analyzed + droppedWindow
		if total > 0 && float64(droppedWindow)/float64(total) > 0.5 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("WARNING: %d items (%d%%) were excluded because they fall outside the analysis time window. This suggests your time window is too narrow or the project is less active recently.", droppedWindow, int(float64(droppedWindow)/float64(total)*100)))
		}
	}

	return res
}

// RunScopeSimulation predicts how many items can be finished within a given number of days.
func (e *Engine) RunScopeSimulation(days int, trials int) Result {
	if e.histogram == nil || len(e.histogram.Counts) == 0 {
		return Result{}
	}

	scopes := make([]int, trials)
	for i := 0; i < trials; i++ {
		scopes[i] = e.simulateScopeTrial(days)
	}

	sort.Ints(scopes)

	res := Result{
		Percentiles: Percentiles{
			Aggressive:    float64(scopes[int(float64(trials)*0.90)]), // 10% chance to deliver AT LEAST this much
			Unlikely:      float64(scopes[int(float64(trials)*0.70)]), // 30% chance to deliver AT LEAST this much
			CoinToss:      float64(scopes[int(float64(trials)*0.50)]),
			Probable:      float64(scopes[int(float64(trials)*0.30)]), // 70% chance to deliver AT LEAST this much
			Likely:        float64(scopes[int(float64(trials)*0.15)]), // 85% chance to deliver AT LEAST this much
			Conservative:  float64(scopes[int(float64(trials)*0.10)]), // 90% chance to deliver AT LEAST this much
			Safe:          float64(scopes[int(float64(trials)*0.05)]), // 95% chance to deliver AT LEAST this much
			AlmostCertain: float64(scopes[int(float64(trials)*0.02)]), // 98% chance to deliver AT LEAST this much
		},
		Spread: SpreadMetrics{
			IQR:     float64(scopes[int(float64(trials)*0.75)] - scopes[int(float64(trials)*0.25)]),
			Inner80: float64(scopes[int(float64(trials)*0.90)] - scopes[int(float64(trials)*0.10)]),
		},
		PercentileLabels: map[string]string{
			"aggressive":     "P10 (10% probability to deliver at least this much)",
			"unlikely":       "P30 (30% probability to deliver at least this much)",
			"coin_toss":      "P50 (Coin Toss / Median)",
			"probable":       "P70 (70% probability to deliver at least this much)",
			"likely":         "P85 (85% probability to deliver at least this much)",
			"conservative":   "P90 (90% probability to deliver at least this much)",
			"safe":           "P95 (95% probability to deliver at least this much)",
			"almost_certain": "P98 (98% probability to deliver at least this much)",
		},
	}
	// Window exclusion warning
	if droppedWindow, ok := e.histogram.Meta["dropped_by_window"].(int); ok && droppedWindow > 0 {
		analyzed := e.histogram.Meta["issues_analyzed"].(int)
		total := analyzed + droppedWindow
		if total > 0 && float64(droppedWindow)/float64(total) > 0.5 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("WARNING: %d items (%d%%) were excluded because they fall outside the analysis time window. This suggests your time window is too narrow or the project is less active recently.", droppedWindow, int(float64(droppedWindow)/float64(total)*100)))
		}
	}

	// For scope, fat-tail detection is slightly different (low scope is the risk)
	// But we use the same formula on the delivery volume distribution for consistency
	// though usually fat-tail refers to Lead/Cycle Time.
	e.assessPredictability(&res)
	return res
}

// RunCycleTimeAnalysis calculates percentiles from a list of historical cycle times (in days).
func (e *Engine) RunCycleTimeAnalysis(cycleTimes []float64) Result {
	if len(cycleTimes) == 0 {
		return Result{}
	}

	sort.Float64s(cycleTimes)
	n := len(cycleTimes)

	res := Result{
		Percentiles: Percentiles{
			Aggressive:    cycleTimes[int(float64(n)*0.10)],
			Unlikely:      cycleTimes[int(float64(n)*0.30)],
			CoinToss:      cycleTimes[int(float64(n)*0.50)],
			Probable:      cycleTimes[int(float64(n)*0.70)],
			Likely:        cycleTimes[int(float64(n)*0.85)],
			Conservative:  cycleTimes[int(float64(n)*0.90)],
			Safe:          cycleTimes[int(float64(n)*0.95)],
			AlmostCertain: cycleTimes[int(float64(n)*0.98)],
		},
		Spread: SpreadMetrics{
			IQR:     cycleTimes[int(float64(n)*0.75)] - cycleTimes[int(float64(n)*0.25)],
			Inner80: cycleTimes[int(float64(n)*0.90)] - cycleTimes[int(float64(n)*0.10)],
		},
		PercentileLabels: map[string]string{
			"aggressive":     "P10 (Aggressive / Fast Outliers)",
			"unlikely":       "P30 (Unlikely / Fast Pace)",
			"coin_toss":      "P50 (Coin Toss / Median)",
			"probable":       "P70 (Probable)",
			"likely":         "P85 (Likely / SLE)",
			"conservative":   "P90 (Conservative / Buffer)",
			"safe":           "P95 (Safe Bet)",
			"almost_certain": "P98 (Limit / Extreme Outliers)",
		},
	}
	e.assessPredictability(&res)
	return res
}

// RunMultiTypeScopeSimulation predicts how many items of specific types can be finished within a given number of days.
func (e *Engine) RunMultiTypeScopeSimulation(targetDays int, trials int, filterTypes []string, distribution map[string]float64, expansionEnabled bool) Result {
	if e.histogram == nil || len(e.histogram.Counts) == 0 {
		return Result{}
	}

	filterMap := make(map[string]bool)
	for _, t := range filterTypes {
		filterMap[t] = true
	}

	scopes := make([]int, trials)
	backgroundCounts := make(map[string][]int)
	for t := range distribution {
		backgroundCounts[t] = make([]int, trials)
	}

	log.Info().Int("days", targetDays).Int("trials", trials).Interface("filter", filterTypes).Bool("expansion", expansionEnabled).Msg("Starting multi-type scope simulation")
	for i := 0; i < trials; i++ {
		scope := 0
		bgItems := make(map[string]int)

		for d := 0; d < targetDays; d++ {
			idx := e.rng.Intn(len(e.histogram.Counts))
			slots := e.histogram.Counts[idx]

			for s := 0; s < slots; s++ {
				// Sample type
				r := e.rng.Float64()
				var sampledType string
				acc := 0.0
				for t, p := range distribution {
					acc += p
					if r <= acc {
						sampledType = t
						break
					}
				}

				if sampledType == "" {
					// Fallback
					for t := range distribution {
						sampledType = t
						break
					}
				}

				// If expansion is disabled, we effectively treat all capacity as target filtered.
				// This is handled by re-normalizing distribution in the handler or here.
				if !expansionEnabled || filterMap[sampledType] || len(filterTypes) == 0 {
					scope++
				} else {
					bgItems[sampledType]++
				}
			}
		}

		scopes[i] = scope
		for t, c := range bgItems {
			backgroundCounts[t][i] += c
		}
	}

	sort.Ints(scopes)

	// Calculate median background items
	medianBG := make(map[string]int)
	for t, counts := range backgroundCounts {
		sort.Ints(counts)
		medianBG[t] = counts[trials/2]
	}

	res := Result{
		Percentiles: Percentiles{
			Aggressive:    float64(scopes[int(float64(trials)*0.90)]),
			Unlikely:      float64(scopes[int(float64(trials)*0.70)]),
			CoinToss:      float64(scopes[int(float64(trials)*0.50)]),
			Probable:      float64(scopes[int(float64(trials)*0.30)]),
			Likely:        float64(scopes[int(float64(trials)*0.15)]),
			Conservative:  float64(scopes[int(float64(trials)*0.10)]),
			Safe:          float64(scopes[int(float64(trials)*0.05)]),
			AlmostCertain: float64(scopes[int(float64(trials)*0.02)]),
		},
		Spread: SpreadMetrics{
			IQR:     float64(scopes[int(float64(trials)*0.75)] - scopes[int(float64(trials)*0.25)]),
			Inner80: float64(scopes[int(float64(trials)*0.90)] - scopes[int(float64(trials)*0.10)]),
		},
		PercentileLabels: map[string]string{
			"aggressive":     "P10 (10% probability to deliver at least this much)",
			"unlikely":       "P30 (30% probability to deliver at least this much)",
			"coin_toss":      "P50 (Coin Toss / Median)",
			"probable":       "P70 (70% probability to deliver at least this much)",
			"likely":         "P85 (85% probability to deliver at least this much)",
			"conservative":   "P90 (90% probability to deliver at least this much)",
			"safe":           "P95 (95% probability to deliver at least this much)",
			"almost_certain": "P98 (98% probability to deliver at least this much)",
		},
		BackgroundItemsPredicted: medianBG,
	}

	e.assessPredictability(&res)
	return res
}

func (e *Engine) assessPredictability(res *Result) {
	if res.Percentiles.CoinToss > 0 {
		res.FatTailRatio = math.Round(res.Percentiles.AlmostCertain/res.Percentiles.CoinToss*100) / 100
		res.TailToMedianRatio = math.Round(res.Percentiles.Likely/res.Percentiles.CoinToss*100) / 100

		predictability := "Stable"
		if res.FatTailRatio >= 5.6 {
			predictability = "Unstable"
			res.Insights = append(res.Insights, fmt.Sprintf("Fat-Tail Warning (Ratio %.2f): Extreme outliers are in control of this process (Kanban heuristic >= 5.6). Your forecasts are high-risk.", res.FatTailRatio))
		}

		if res.TailToMedianRatio > 3.0 {
			if predictability == "Stable" {
				predictability = "Highly Volatile"
			} else {
				predictability = "Unstable & Volatile"
			}
			res.Insights = append(res.Insights, fmt.Sprintf("Heavy-Tail Warning (Ratio %.2f): The process is highly volatile, indicating a significant risk of extreme delay (Volatility heuristic > 3).", res.TailToMedianRatio))
		}
		res.Predictability = predictability
	} else {
		res.Predictability = "Unknown"
	}

	if e.histogram == nil || e.histogram.Meta == nil {
		return
	}

	res.Context = e.histogram.Meta

	// 1. Throughput Trend Warning & Detection
	res.ThroughputTrend.Direction = "Stable"
	if recent, ok := e.histogram.Meta["throughput_recent"].(float64); ok {
		if overall, ok := e.histogram.Meta["throughput_overall"].(float64); ok && overall > 0 {
			diff := (recent - overall) / overall
			res.ThroughputTrend.PercentageChange = math.Round(diff*1000) / 10
			if diff < -0.1 {
				res.ThroughputTrend.Direction = "Declining"
				if diff < -0.3 {
					res.Warnings = append(res.Warnings, fmt.Sprintf("Significant throughput drop recently (%.0f%% below average). WIP may have increased or capacity dropped.", math.Abs(diff)*100))
				}
			} else if diff > 0.1 {
				res.ThroughputTrend.Direction = "Increasing"
				if diff > 0.3 {
					res.Warnings = append(res.Warnings, fmt.Sprintf("Throughput is significantly higher recently (%.0f%% above average). Monitor if this is sustainable.", diff*100))
				}
			}
		}
	}

	if analyzed, ok := e.histogram.Meta["issues_analyzed"].(int); ok && analyzed < 30 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("Simulation based on a small sample size (%d items); results may have limited statistical significance.", analyzed))
	}
}

// AnalyzeWIPStability performs deep analysis of current WIP vs historical performance.
func (e *Engine) AnalyzeWIPStability(res *Result, wipAges []float64, cycleTimes []float64, backlogSize int) {
	if len(cycleTimes) == 0 {
		return
	}

	sort.Float64s(cycleTimes)
	n := len(cycleTimes)
	p50 := cycleTimes[int(float64(n)*0.50)]
	p85 := cycleTimes[int(float64(n)*0.85)]
	p95 := cycleTimes[int(float64(n)*0.95)]

	// 1. WIP Aging Analytics
	res.WIPAgeDistribution = map[string]int{
		"Inconspicuous (within P50)": 0,
		"Aging (P50-P85)":            0,
		"Warning (P85-P95)":          0,
		"Extreme (>P95)":             0,
	}

	staleCount := 0
	for _, age := range wipAges {
		if age < p50 {
			res.WIPAgeDistribution["Inconspicuous (within P50)"]++
		} else if age < p85 {
			res.WIPAgeDistribution["Aging (P50-P85)"]++
		} else if age < p95 {
			res.WIPAgeDistribution["Warning (P85-P95)"]++
		} else {
			res.WIPAgeDistribution["Extreme (>P95)"]++
		}

		if age > p85 {
			staleCount++
		}
	}
	res.StaleWIPCount = staleCount

	if len(wipAges) > 0 {
		staleRate := float64(staleCount) / float64(len(wipAges))
		if staleRate > 0.3 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%.0f%% of your current WIP is 'stale' (older than project P85 of %.1f days). Forecast may be optimistic.", staleRate*100, p85))
		}
		if res.WIPAgeDistribution["Extreme (>P95)"] > 0 {
			maxAge := 0.0
			for _, a := range wipAges {
				if a > maxAge {
					maxAge = a
				}
			}
			maxAgeDisplay := math.Ceil(maxAge*10) / 10
			res.Insights = append(res.Insights, fmt.Sprintf("Actionable Insight: You have %d extreme outliers (>P95). Removing or resolving the oldest item (%.1f days) could immediately clarify your throughput capacity.", res.WIPAgeDistribution["Extreme (>P95)"], maxAgeDisplay))
		}

		// Mode-specific Scope insight
		if res.Composition.Total == 0 && res.Composition.WIP > 0 {
			// Total == 0 is our internal sentinel for "Scope Mode" in this context
			youngWIP := res.WIPAgeDistribution["Inconspicuous (within P50)"]
			if youngWIP > 0 {
				res.Insights = append(res.Insights, fmt.Sprintf("Strategic Insight: To hit your delivery targets, prioritize the %d items that are already in progress and within your median cycle time.", youngWIP))
			}
		}
	}

	// 2. Little's Law Stability Index (WIP = TH * CT)
	if e.histogram != nil && e.histogram.Meta != nil {
		th, ok1 := e.histogram.Meta["throughput_overall"].(float64)
		sumCT := 0.0
		for _, ct := range cycleTimes {
			sumCT += ct
		}
		avgCT := sumCT / float64(len(cycleTimes))

		if ok1 && th > 0 && avgCT > 0 {
			expectedWIP := th * avgCT
			currentWIP := float64(len(wipAges))
			ratio := currentWIP / expectedWIP
			res.StabilityRatio = math.Round(ratio*100) / 100

			if res.StabilityRatio > 1.3 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Clogged System (Stability Index %.2f): You have %.0f%% more WIP than your historical capacity supports. Lead times will likely increase.", res.StabilityRatio, (res.StabilityRatio-1)*100))
			} else if res.StabilityRatio < 0.7 && currentWIP > 0 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Starving System (Stability Index %.2f): You have significantly less WIP than historical levels. Throughput may drop unless more work is started.", res.StabilityRatio))
			}
		}
	}
}

func (e *Engine) simulateDurationTrialWithTypeMix(targets map[string]int, distribution map[string]float64) (int, map[string]int) {
	days := 0
	remaining := make(map[string]int)
	totalRemaining := 0
	for t, c := range targets {
		remaining[t] = c
		totalRemaining += c
	}

	background := make(map[string]int)

	// Early check for infinite loop
	totalProb := 0.0
	for t := range targets {
		totalProb += distribution[t]
	}
	if totalProb == 0 {
		return 20000, background
	}

	for totalRemaining > 0 {
		days++
		idx := e.rng.Intn(len(e.histogram.Counts))
		slots := e.histogram.Counts[idx]

		for s := 0; s < slots; s++ {
			// Sample type
			r := e.rng.Float64()
			var sampledType string
			acc := 0.0
			for t, p := range distribution {
				acc += p
				if r <= acc {
					sampledType = t
					break
				}
			}

			if sampledType == "" {
				// Fallback if float precision issues
				for t := range distribution {
					sampledType = t
					break
				}
			}

			if count, ok := remaining[sampledType]; ok && count > 0 {
				remaining[sampledType]--
				totalRemaining--
			} else {
				background[sampledType]++
			}

			if totalRemaining <= 0 {
				break
			}
		}

		if days >= 20000 {
			break
		}
	}
	return days, background
}

func (e *Engine) simulateScopeTrial(targetDays int) int {
	totalScope := 0
	for d := 0; d < targetDays; d++ {
		idx := e.rng.Intn(len(e.histogram.Counts))
		totalScope += e.histogram.Counts[idx]
	}
	return totalScope
}
