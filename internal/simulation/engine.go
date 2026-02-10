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
	Composition              Composition               `json:"composition"`
	WIPAgeDistribution       map[string]map[string]int `json:"wip_age_distribution,omitempty"`
	ThroughputTrend          ThroughputTrend           `json:"throughput_trend"`
	Insights                 []string                  `json:"insights,omitempty"`
	PercentileLabels         map[string]string         `json:"percentile_labels,omitempty"`
	BackgroundItemsPredicted map[string]int            `json:"background_items_predicted,omitempty"`
	ModelingInsight          string                    `json:"modeling_insight,omitempty"`
	VolatilityAttribution    map[string]string         `json:"volatility_attribution,omitempty"`
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

func (e *Engine) RunMultiTypeDurationSimulation(targets map[string]int, distribution map[string]float64, trials int, expansionEnabled bool) Result {
	if e.histogram == nil || len(e.histogram.Counts) == 0 {
		return Result{}
	}

	// 1. Determine if we should use stratification
	useStratification := false
	if eligible, ok := e.histogram.Meta["stratification_eligible"].(map[string]bool); ok {
		for _, isEligible := range eligible {
			if isEligible {
				useStratification = true
				break
			}
		}
	}

	// 2. Prepare final distribution for pooled fallback
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

	// 3. Parallel Execution Setup
	numGo := 4 // Split into 4 chunks
	if trials < 100 {
		numGo = 1
	}
	trialsPerGo := trials / numGo

	type trialResult struct {
		durations []int
		bgCounts  map[string][]int
	}
	resultsChan := make(chan trialResult, numGo)

	// Calculate Capacity Cap (P95 of total daily throughput)
	capPool := make([]int, len(e.histogram.Counts))
	copy(capPool, e.histogram.Counts)
	sort.Ints(capPool)
	capacityCap := max(capPool[int(float64(len(capPool))*0.95)], 1)

	log.Info().Int("trials", trials).Interface("targets", targets).Bool("stratified", useStratification).Msg("Starting multi-type duration simulation")

	for g := 0; g < numGo; g++ {
		go func(count int, seed int64) {
			rng := rand.New(rand.NewSource(seed))
			res := trialResult{
				durations: make([]int, count),
				bgCounts:  make(map[string][]int),
			}
			for t := range distribution {
				res.bgCounts[t] = make([]int, count)
			}

			for i := range count {
				var duration int
				var bg map[string]int
				if useStratification {
					duration, bg = e.simulateDurationTrialStratified(targets, capacityCap, rng)
				} else {
					duration, bg = e.simulateDurationTrialWithTypeMixLocal(targets, finalDist, rng)
				}
				res.durations[i] = duration
				for t, c := range bg {
					res.bgCounts[t][i] = c
				}
			}
			resultsChan <- res
		}(trialsPerGo, time.Now().UnixNano()+int64(g))
	}

	// Aggregate Results
	durations := make([]int, 0, trials)
	backgroundCounts := make(map[string][]int)
	for t := range distribution {
		backgroundCounts[t] = make([]int, 0, trials)
	}

	for g := 0; g < numGo; g++ {
		res := <-resultsChan
		durations = append(durations, res.durations...)
		for t, counts := range res.bgCounts {
			backgroundCounts[t] = append(backgroundCounts[t], counts...)
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
		PercentileLabels:         getPercentileLabels("duration"),
		BackgroundItemsPredicted: medianBG,
	}

	if insight, ok := e.histogram.Meta["modeling_insight"].(string); ok {
		res.ModelingInsight = insight
	} else {
		res.ModelingInsight = "Pooled: Static model (no dynamic metadata found)"
	}

	// Volatility Attribution
	if vol, ok := e.histogram.Meta["type_volatility"].(map[string]float64); ok {
		res.VolatilityAttribution = make(map[string]string)
		for t, v := range vol {
			if v >= 5.6 {
				res.VolatilityAttribution[t] = fmt.Sprintf("Fat-Tail High-Risk (%.2f)", v)
				res.Insights = append(res.Insights, fmt.Sprintf("Volatility Alert: Item Type '%s' shows chaotic delivery patterns (Ratio %.2f). This type is the primary driver of forecast uncertainty.", t, v))
			}
		}
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

	// Parallel Execution Setup
	numGo := 4
	if trials < 100 {
		numGo = 1
	}
	trialsPerGo := trials / numGo

	resultsChan := make(chan []int, numGo)

	for g := 0; g < numGo; g++ {
		go func(count int, seed int64) {
			rng := rand.New(rand.NewSource(seed))
			res := make([]int, count)
			for i := range count {
				res[i] = e.simulateScopeTrialLocal(days, rng)
			}
			resultsChan <- res
		}(trialsPerGo, time.Now().UnixNano()+int64(g))
	}

	scopes := make([]int, 0, trials)
	for g := 0; g < numGo; g++ {
		res := <-resultsChan
		scopes = append(scopes, res...)
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
		PercentileLabels: getPercentileLabels("scope"),
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
		PercentileLabels: getPercentileLabels("cycle_time"),
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

	// 1. Determine if we should use stratification
	useStratification := false
	if eligible, ok := e.histogram.Meta["stratification_eligible"].(map[string]bool); ok {
		for _, isEligible := range eligible {
			if isEligible {
				useStratification = true
				break
			}
		}
	}

	// Calculate Capacity Cap
	capPool := make([]int, len(e.histogram.Counts))
	copy(capPool, e.histogram.Counts)
	sort.Ints(capPool)
	capacityCap := capPool[int(float64(len(capPool))*0.95)]
	if capacityCap < 1 {
		capacityCap = 1
	}

	// 2. Parallel Execution Setup
	numGo := 4
	if trials < 100 {
		numGo = 1
	}
	trialsPerGo := trials / numGo

	type scopeTrialResult struct {
		scopes   []int
		bgCounts map[string][]int
	}
	resultsChan := make(chan scopeTrialResult, numGo)

	log.Info().Int("days", targetDays).Int("trials", trials).Interface("filter", filterTypes).Bool("stratified", useStratification).Msg("Starting multi-type scope simulation")

	for g := 0; g < numGo; g++ {
		go func(count int, seed int64) {
			rng := rand.New(rand.NewSource(seed))
			res := scopeTrialResult{
				scopes:   make([]int, count),
				bgCounts: make(map[string][]int),
			}
			for t := range distribution {
				res.bgCounts[t] = make([]int, count)
			}

			for i := range count {
				var scope int
				var bg map[string]int
				if useStratification {
					scope, bg = e.simulateScopeTrialStratified(targetDays, filterMap, capacityCap, rng)
				} else {
					scope, bg = e.simulateMultiTypeScopeTrialLocal(targetDays, filterMap, distribution, rng)
				}
				res.scopes[i] = scope
				for t, c := range bg {
					res.bgCounts[t][i] = c
				}
			}
			resultsChan <- res
		}(trialsPerGo, time.Now().UnixNano()+int64(g))
	}

	scopes := make([]int, 0, trials)
	backgroundCounts := make(map[string][]int)
	for t := range distribution {
		backgroundCounts[t] = make([]int, 0, trials)
	}

	for g := 0; g < numGo; g++ {
		res := <-resultsChan
		scopes = append(scopes, res.scopes...)
		for t, counts := range res.bgCounts {
			backgroundCounts[t] = append(backgroundCounts[t], counts...)
		}
	}

	sort.Ints(scopes)

	// Calculate median background items
	medianBG := make(map[string]int)
	for t, counts := range backgroundCounts {
		sort.Ints(counts)
		medianBG[t] = counts[len(counts)/2]
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
		PercentileLabels:         getPercentileLabels("scope"),
		BackgroundItemsPredicted: medianBG,
	}

	e.assessPredictability(&res)

	return res
}

func (e *Engine) simulateScopeTrialStratified(targetDays int, filterMap map[string]bool, capacityCap int, rng *rand.Rand) (int, map[string]int) {
	totalScope := 0
	bgItems := make(map[string]int)

	for range targetDays {
		// 1. Independent Stratified Sampling (with Blending and Dependency Awareness)
		sampled := make(map[string]int)
		totalSampled := 0
		deps, _ := e.histogram.Meta["stratification_dependencies"].(map[string]string)

		for t, counts := range e.histogram.StratifiedCounts {
			h := 0
			if len(counts) < 30 && rng.Float64() < 0.3 {
				if overall, ok := e.histogram.Meta["throughput_overall"].(float64); ok {
					if dist, ok := e.histogram.Meta["type_distribution"].(map[string]float64); ok {
						h = int(math.Round(overall * dist[t]))
					}
				}
			} else {
				idx := rng.Intn(len(counts))
				h = counts[idx]
			}
			sampled[t] = h
			totalSampled += h
		}

		// Dependency Awareness
		for taxer, taxed := range deps {
			if hT, ok := sampled[taxer]; ok && hT > 0 {
				if hD, ok := sampled[taxed]; ok && hD > 0 {
					reduction := int(math.Floor(float64(hT) * 0.5))
					if hD > reduction {
						sampled[taxed] -= reduction
					} else {
						sampled[taxed] = 0
					}
				}
			}
		}

		// Re-calculate
		totalSampled = 0
		for _, h := range sampled {
			totalSampled += h
		}

		// 2. Capacity Coordination (Cap)
		if totalSampled > capacityCap {
			factor := float64(capacityCap) / float64(totalSampled)
			for t, h := range sampled {
				newH := int(math.Floor(float64(h) * factor))
				if newH == 0 && h > 0 && rng.Float64() < factor {
					newH = 1
				}
				sampled[t] = newH
			}
		}

		// 3. Count Scope
		for t, h := range sampled {
			if filterMap[t] || len(filterMap) == 0 {
				totalScope += h
			} else {
				bgItems[t] += h
			}
		}
	}

	return totalScope, bgItems
}

func getPercentileLabels(mode string) map[string]string {
	labels := make(map[string]string)
	switch mode {
	case "duration":
		labels["aggressive"] = "P10 (Aggressive / Best Case)"
		labels["unlikely"] = "P30 (Unlikely / High Risk)"
		labels["coin_toss"] = "P50 (Coin Toss / Median / 50% probability)"
		labels["probable"] = "P70 (Probable)"
		labels["likely"] = "P85 (Likely / Professional Standard / Professional commitment)"
		labels["conservative"] = "P90 (Conservative / Buffer)"
		labels["safe"] = "P95 (Safe Bet / High Confidence)"
		labels["almost_certain"] = "P98 (Limit / Extreme Outlier Boundaries)"
	case "scope":
		labels["aggressive"] = "P10 (10% probability to deliver at least this much)"
		labels["unlikely"] = "P30 (30% probability to deliver at least this much)"
		labels["coin_toss"] = "P50 (Coin Toss / Median / 50% probability)"
		labels["probable"] = "P70 (70% probability to deliver at least this much)"
		labels["likely"] = "P85 (85% probability to deliver at least this much)"
		labels["conservative"] = "P90 (90% probability to deliver at least this much)"
		labels["safe"] = "P95 (95% probability to deliver at least this much)"
		labels["almost_certain"] = "P98 (98% probability to deliver at least this much)"
	case "cycle_time":
		labels["aggressive"] = "P10 (Aggressive / Fast Outliers)"
		labels["unlikely"] = "P30 (Unlikely / Fast Pace)"
		labels["coin_toss"] = "P50 (Coin Toss / Median / 50% probability)"
		labels["probable"] = "P70 (Probable)"
		labels["likely"] = "P85 (Likely / SLE / Service Level Expectation)"
		labels["conservative"] = "P90 (Conservative / Buffer)"
		labels["safe"] = "P95 (Safe Bet)"
		labels["almost_certain"] = "P98 (Limit / Extreme Outliers)"
	}
	return labels
}

func (e *Engine) simulateMultiTypeScopeTrialLocal(targetDays int, filterMap map[string]bool, distribution map[string]float64, rng *rand.Rand) (int, map[string]int) {
	scope := 0
	bgItems := make(map[string]int)

	for range targetDays {
		idx := rng.Intn(len(e.histogram.Counts))
		slots := e.histogram.Counts[idx]
		for range slots {
			r := rng.Float64()
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
				for t := range distribution {
					sampledType = t
					break
				}
			}

			if filterMap[sampledType] || len(filterMap) == 0 {
				scope++
			} else {
				bgItems[sampledType]++
			}
		}
	}

	return scope, bgItems
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
func (e *Engine) AnalyzeWIPStability(res *Result, wipAges map[string][]float64, cycleTimes map[string][]float64) {
	if len(cycleTimes) == 0 {
		return
	}

	// 1. Calculate Pooled Fallback
	allCycleTimes := make([]float64, 0)
	for _, cts := range cycleTimes {
		allCycleTimes = append(allCycleTimes, cts...)
	}
	sort.Float64s(allCycleTimes)
	pn := len(allCycleTimes)
	pP50 := allCycleTimes[int(float64(pn)*0.50)]
	pP85 := allCycleTimes[int(float64(pn)*0.85)]
	pP95 := allCycleTimes[int(float64(pn)*0.95)]

	res.WIPAgeDistribution = make(map[string]map[string]int)
	totalStaleCount := 0
	totalWIPCount := 0

	// 2. Stratified Analysis
	for t, ages := range wipAges {
		res.WIPAgeDistribution[t] = map[string]int{
			"Inconspicuous (within P50)": 0,
			"Aging (P50-P85)":            0,
			"Warning (P85-P95)":          0,
			"Extreme (>P95)":             0,
		}

		// Determine benchmarks for this type
		tP50, tP85, tP95 := pP50, pP85, pP95
		if cts, ok := cycleTimes[t]; ok && len(cts) >= 5 {
			sort.Float64s(cts)
			tn := len(cts)
			tP50 = cts[int(float64(tn)*0.50)]
			tP85 = cts[int(float64(tn)*0.85)]
			tP95 = cts[int(float64(tn)*0.95)]
		}

		for _, age := range ages {
			totalWIPCount++
			if age < tP50 {
				res.WIPAgeDistribution[t]["Inconspicuous (within P50)"]++
			} else if age < tP85 {
				res.WIPAgeDistribution[t]["Aging (P50-P85)"]++
			} else if age < tP95 {
				res.WIPAgeDistribution[t]["Warning (P85-P95)"]++
			} else {
				res.WIPAgeDistribution[t]["Extreme (>P95)"]++
			}

			if age > tP85 {
				totalStaleCount++
			}
		}
	}

	res.StaleWIPCount = totalStaleCount

	if totalWIPCount > 0 {
		staleRate := float64(totalStaleCount) / float64(totalWIPCount)
		if staleRate > 0.3 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%.0f%% of your current WIP is 'stale' relative to type-specific benchmarks. Forecast may be optimistic.", staleRate*100))
		}

		// Find extreme outliers
		extremeCount := 0
		maxAge := 0.0
		for t, dist := range res.WIPAgeDistribution {
			extremeCount += dist["Extreme (>P95)"]
			for _, age := range wipAges[t] {
				if age > maxAge {
					maxAge = age
				}
			}
		}

		if extremeCount > 0 {
			maxAgeDisplay := math.Ceil(maxAge*10) / 10
			res.Insights = append(res.Insights, fmt.Sprintf("Actionable Insight: You have %d extreme outliers (>P95 for their type). Resolving the oldest item (%.1f days) could immediately clarify capacity.", extremeCount, maxAgeDisplay))
		}
	}

	// 2. Little's Law Stability Index (WIP = TH * CT)
	if e.histogram != nil && e.histogram.Meta != nil {
		th, ok1 := e.histogram.Meta["throughput_overall"].(float64)
		sumCT := 0.0
		for _, ct := range allCycleTimes {
			sumCT += ct
		}
		avgCT := sumCT / float64(len(allCycleTimes))

		if ok1 && th > 0 && avgCT > 0 {
			expectedWIP := th * avgCT
			currentWIP := float64(totalWIPCount)
			ratio := currentWIP / expectedWIP
			res.StabilityRatio = math.Round(ratio*100) / 100

			if res.StabilityRatio > 1.3 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Clogged System (Stability Index %.2f): You have %.0f%% more WIP than your historical capacity supports.", res.StabilityRatio, (res.StabilityRatio-1)*100))
			} else if res.StabilityRatio < 0.7 && currentWIP > 0 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("Starving System (Stability Index %.2f): WIP is significantly lower than historical levels.", res.StabilityRatio))
			}
		}
	}
}

func (e *Engine) simulateDurationTrialStratified(targets map[string]int, capacityCap int, rng *rand.Rand) (int, map[string]int) {
	days := 0
	remaining := make(map[string]int)
	totalRemaining := 0
	for t, c := range targets {
		remaining[t] = c
		totalRemaining += c
	}

	background := make(map[string]int)

	for totalRemaining > 0 {
		days++

		// 1. Independent Stratified Sampling (with Bayesian Blending and Dependency Awareness)
		sampled := make(map[string]int)
		totalSampled := 0
		deps, _ := e.histogram.Meta["stratification_dependencies"].(map[string]string)

		for t, counts := range e.histogram.StratifiedCounts {
			// Bayesian Blending: If sparse data, we blend with pooled behavior
			h := 0
			if len(counts) < 30 && rng.Float64() < 0.3 {
				// 30% of the time, fallback to pooled average for this type
				if overall, ok := e.histogram.Meta["throughput_overall"].(float64); ok {
					if dist, ok := e.histogram.Meta["type_distribution"].(map[string]float64); ok {
						h = int(math.Round(overall * dist[t]))
					}
				}
			} else {
				idx := rng.Intn(len(counts))
				h = counts[idx]
			}

			sampled[t] = h
			totalSampled += h
		}

		// Dependency Awareness (Statistical Bug-Tax)
		for taxer, taxed := range deps {
			if hT, ok := sampled[taxer]; ok && hT > 0 {
				if hD, ok := sampled[taxed]; ok && hD > 0 {
					// If taxer is active, we apply a pressure based on its impact
					// High taxer volume squeezes the taxed volume further than just the global cap
					if hT > 0 {
						// Simple heuristic: reduce taxed by a fraction of taxer
						reduction := int(math.Floor(float64(hT) * 0.5))
						if hD > reduction {
							sampled[taxed] -= reduction
						} else {
							sampled[taxed] = 0
						}
					}
				}
			}
		}

		// Re-calculate total after dependency squeeze
		totalSampled = 0
		for _, h := range sampled {
			totalSampled += h
		}

		// 2. Capacity Coordination (Cap)
		// If independent processes exceed historical total p95, we scale them down.
		if totalSampled > capacityCap {
			factor := float64(capacityCap) / float64(totalSampled)
			totalSampled = 0
			for t, h := range sampled {
				// We use Floor to be defensive/conservative
				newH := int(math.Floor(float64(h) * factor))
				if newH == 0 && h > 0 && rng.Float64() < factor {
					// Stochastic rounding to ensure we don't zero out everything on low-cap systems
					newH = 1
				}
				sampled[t] = newH
				totalSampled += newH
			}
		}

		// 3. Consume Targets
		for t, h := range sampled {
			if count, ok := remaining[t]; ok && count > 0 {
				delivered := min(h, count)
				remaining[t] -= delivered
				totalRemaining -= delivered

				// Remainder of capacity for this type goes to background
				if h > delivered {
					background[t] += h - delivered
				}
			} else {
				background[t] += h
			}
		}

		if days >= 20000 {
			break
		}
	}
	return days, background
}

func (e *Engine) simulateDurationTrialWithTypeMixLocal(targets map[string]int, distribution map[string]float64, rng *rand.Rand) (int, map[string]int) {
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
	for t := range distribution {
		totalProb += distribution[t]
	}
	if totalProb == 0 {
		return 20000, background
	}

	for totalRemaining > 0 {
		days++
		idx := rng.Intn(len(e.histogram.Counts))
		slots := e.histogram.Counts[idx]

		for range slots {
			r := rng.Float64()
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
	return e.simulateScopeTrialLocal(targetDays, e.rng)
}

func (e *Engine) simulateScopeTrialLocal(targetDays int, rng *rand.Rand) int {
	totalScope := 0
	for range targetDays {
		idx := rng.Intn(len(e.histogram.Counts))
		totalScope += e.histogram.Counts[idx]
	}
	return totalScope
}
