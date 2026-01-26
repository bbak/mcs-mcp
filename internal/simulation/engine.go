package simulation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"
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

// Result holds the percentiles of a simulation or analysis.
type Result struct {
	Aggressive     float64                `json:"aggressive"`     // P10
	Unlikely       float64                `json:"unlikely"`       // P30
	CoinToss       float64                `json:"coin_toss"`      // P50
	Probable       float64                `json:"probable"`       // P70
	Likely         float64                `json:"likely"`         // P85
	Conservative   float64                `json:"conservative"`   // P90
	Safe           float64                `json:"safe"`           // P95
	AlmostCertain  float64                `json:"almost_certain"` // P98
	Ratio          float64                `json:"ratio"`
	Predictability string                 `json:"predictability"`
	Context        map[string]interface{} `json:"context,omitempty"`
	Warnings       []string               `json:"warnings,omitempty"`
	StabilityRatio float64                `json:"stability_ratio,omitempty"`
	StaleWIPCount  int                    `json:"stale_wip_count,omitempty"`

	// Advanced Analytics
	Composition        Composition       `json:"composition"`
	WIPAgeDistribution map[string]int    `json:"wip_age_distribution,omitempty"`
	ThroughputTrend    ThroughputTrend   `json:"throughput_trend"`
	Insights           []string          `json:"insights,omitempty"`
	PercentileLabels   map[string]string `json:"percentile_labels,omitempty"`
}

func NewEngine(h *Histogram) *Engine {
	return &Engine{
		histogram: h,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RunDurationSimulation predicts how many days it will take to finish a backlog.
func (e *Engine) RunDurationSimulation(backlogSize int, trials int) Result {
	if len(e.histogram.Counts) == 0 {
		return Result{}
	}

	durations := make([]int, trials)
	for i := 0; i < trials; i++ {
		durations[i] = e.simulateDurationTrial(backlogSize)
	}

	sort.Ints(durations)

	res := Result{
		Aggressive:    float64(durations[int(float64(trials)*0.10)]),
		Unlikely:      float64(durations[int(float64(trials)*0.30)]),
		CoinToss:      float64(durations[int(float64(trials)*0.50)]),
		Probable:      float64(durations[int(float64(trials)*0.70)]),
		Likely:        float64(durations[int(float64(trials)*0.85)]),
		Conservative:  float64(durations[int(float64(trials)*0.90)]),
		Safe:          float64(durations[int(float64(trials)*0.95)]),
		AlmostCertain: float64(durations[int(float64(trials)*0.98)]),
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
	}
	e.assessPredictability(&res)
	return res
}

// RunScopeSimulation predicts how many items will be finished in a given number of days.
func (e *Engine) RunScopeSimulation(days int, trials int) Result {
	if len(e.histogram.Counts) == 0 {
		return Result{}
	}

	scopes := make([]int, trials)
	for i := 0; i < trials; i++ {
		scopes[i] = e.simulateScopeTrial(days)
	}

	sort.Ints(scopes)

	res := Result{
		Aggressive:    float64(scopes[int(float64(trials)*0.90)]), // 10% chance to deliver AT LEAST this much (Very high items count)
		Unlikely:      float64(scopes[int(float64(trials)*0.70)]), // 30% chance to deliver AT LEAST this much
		CoinToss:      float64(scopes[int(float64(trials)*0.50)]),
		Probable:      float64(scopes[int(float64(trials)*0.30)]), // 70% chance to deliver AT LEAST this much
		Likely:        float64(scopes[int(float64(trials)*0.15)]), // 85% chance to deliver AT LEAST this much
		Conservative:  float64(scopes[int(float64(trials)*0.10)]), // 90% chance to deliver AT LEAST this much
		Safe:          float64(scopes[int(float64(trials)*0.05)]), // 95% chance to deliver AT LEAST this much
		AlmostCertain: float64(scopes[int(float64(trials)*0.02)]), // 98% chance to deliver AT LEAST this much
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
		Aggressive:    cycleTimes[int(float64(n)*0.10)],
		Unlikely:      cycleTimes[int(float64(n)*0.30)],
		CoinToss:      cycleTimes[int(float64(n)*0.50)],
		Probable:      cycleTimes[int(float64(n)*0.70)],
		Likely:        cycleTimes[int(float64(n)*0.85)],
		Conservative:  cycleTimes[int(float64(n)*0.90)],
		Safe:          cycleTimes[int(float64(n)*0.95)],
		AlmostCertain: cycleTimes[int(float64(n)*0.98)],
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

func (e *Engine) assessPredictability(res *Result) {
	if res.CoinToss > 0 {
		res.Ratio = math.Round(res.AlmostCertain/res.CoinToss*100) / 100
		if res.Ratio >= 5.6 {
			res.Predictability = "Unstable"
		} else {
			res.Predictability = "Stable"
		}
	} else {
		res.Predictability = "Unknown"
	}

	if e.histogram.Meta != nil {
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

		// 2. Data Volume Warning
		if analyzed, ok := e.histogram.Meta["issues_analyzed"].(int); ok && analyzed < 30 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Small sample size (%d items). Statistical confidence is low.", analyzed))
		}
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
	if e.histogram.Meta != nil {
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

func (e *Engine) simulateDurationTrial(backlog int) int {
	days := 0
	remaining := backlog

	for remaining > 0 {
		days++
		idx := e.rng.Intn(len(e.histogram.Counts))
		throughput := e.histogram.Counts[idx]
		remaining -= throughput

		if days > 20000 { // Increased safety brake
			break
		}
	}
	return days
}

func (e *Engine) simulateScopeTrial(targetDays int) int {
	totalScope := 0
	for d := 0; d < targetDays; d++ {
		idx := e.rng.Intn(len(e.histogram.Counts))
		totalScope += e.histogram.Counts[idx]
	}
	return totalScope
}
