// Package simulation runs Monte-Carlo forecasts (duration, scope, cycle time)
// over a historical throughput histogram. Supports multiple engines (crude,
// bbak) behind the ForecastEngine interface, walk-forward backtesting, and
// stratified sampling with capacity coordination.
package simulation

import (
	"fmt"
	"math"
	"math/rand/v2"
	"mcs-mcp/internal/stats"
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

// roundFields rounds each of the given float64 pointers to 2 decimal places.
func roundFields(fields ...*float64) {
	for _, f := range fields {
		*f = stats.Round2(*f)
	}
}

// Round rounds all fields to 2 decimal places for output compactness.
func (p *Percentiles) Round() {
	roundFields(&p.Aggressive, &p.Unlikely, &p.CoinToss, &p.Probable,
		&p.Likely, &p.Conservative, &p.Safe, &p.AlmostCertain)
}

// SpreadMetrics holds statistical dispersion data.
type SpreadMetrics struct {
	IQR     float64 `json:"iqr"`      // P75-P25
	Inner80 float64 `json:"inner_80"` // P90-P10
}

// Round rounds all fields to 2 decimal places for output compactness.
func (s *SpreadMetrics) Round() {
	roundFields(&s.IQR, &s.Inner80)
}

// percentilesIdx returns a safe index into a sorted slice of length n for percentile p.
// It clamps to [0, n-1] to prevent out-of-bounds panics near p=1.0.
func percentilesIdx(n int, p float64) int {
	i := int(float64(n) * p)
	if i >= n {
		i = n - 1
	}
	return i
}

// percentilesFromSorted builds a Percentiles struct from a pre-sorted ascending []float64.
// Use for time-based results (duration, cycle time) where higher values are worse.
func percentilesFromSorted(sorted []float64) Percentiles {
	n := len(sorted)
	return Percentiles{
		Aggressive:    sorted[percentilesIdx(n, 0.10)],
		Unlikely:      sorted[percentilesIdx(n, 0.30)],
		CoinToss:      sorted[percentilesIdx(n, 0.50)],
		Probable:      sorted[percentilesIdx(n, 0.70)],
		Likely:        sorted[percentilesIdx(n, 0.85)],
		Conservative:  sorted[percentilesIdx(n, 0.90)],
		Safe:          sorted[percentilesIdx(n, 0.95)],
		AlmostCertain: sorted[percentilesIdx(n, 0.98)],
	}
}

// percentilesFromSortedInverted builds a Percentiles struct from a pre-sorted ascending []float64.
// Use for scope-based results where higher values are better (inverted probability mapping).
func percentilesFromSortedInverted(sorted []float64) Percentiles {
	n := len(sorted)
	return Percentiles{
		Aggressive:    sorted[percentilesIdx(n, 0.90)], // 10% chance to deliver AT LEAST this much
		Unlikely:      sorted[percentilesIdx(n, 0.70)], // 30% chance to deliver AT LEAST this much
		CoinToss:      sorted[percentilesIdx(n, 0.50)],
		Probable:      sorted[percentilesIdx(n, 0.30)], // 70% chance to deliver AT LEAST this much
		Likely:        sorted[percentilesIdx(n, 0.15)], // 85% chance to deliver AT LEAST this much
		Conservative:  sorted[percentilesIdx(n, 0.10)], // 90% chance to deliver AT LEAST this much
		Safe:          sorted[percentilesIdx(n, 0.05)], // 95% chance to deliver AT LEAST this much
		AlmostCertain: sorted[percentilesIdx(n, 0.02)], // 98% chance to deliver AT LEAST this much
	}
}

// spreadFromSorted builds a SpreadMetrics struct from a pre-sorted ascending []float64.
func spreadFromSorted(sorted []float64) SpreadMetrics {
	n := len(sorted)
	return SpreadMetrics{
		IQR:     sorted[percentilesIdx(n, 0.75)] - sorted[percentilesIdx(n, 0.25)],
		Inner80: sorted[percentilesIdx(n, 0.90)] - sorted[percentilesIdx(n, 0.10)],
	}
}

// intsToFloat64 converts a []int slice to []float64.
func intsToFloat64(in []int) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}

// Result holds the percentiles of a simulation or analysis.
type Result struct {
	Percentiles       Percentiles    `json:"percentiles"`
	Spread            SpreadMetrics  `json:"spread"`
	FatTailRatio      float64        `json:"fat_tail_ratio"`       // P98/P50 (Kanban University heuristic)
	TailToMedianRatio float64        `json:"tail_to_median_ratio"` // P85/P50 (Volatility heuristic)
	Predictability    string         `json:"predictability"`
	Context           map[string]any `json:"context,omitempty"`
	Warnings []string `json:"warnings,omitempty"`

	// Advanced Analytics
	Composition *Composition `json:"composition,omitempty"`
	ThroughputTrend          ThroughputTrend           `json:"throughput_trend"`
	Insights                 []string                  `json:"insights,omitempty"`
	PercentileLabels         map[string]string         `json:"percentile_labels,omitempty"`
	BackgroundItemsPredicted map[string]int            `json:"background_items_predicted,omitempty"`
	ModelingInsight          string                    `json:"modeling_insight,omitempty"`
	VolatilityAttribution    map[string]string         `json:"volatility_attribution,omitempty"`
	TypeSLEs                 map[string]Percentiles    `json:"type_sles,omitempty"`
	Scatterplot              []stats.ScatterPoint      `json:"scatterplot,omitempty"`
	SLEAdherence             *stats.SLEAdherenceResult `json:"sle_adherence,omitempty"`
}

// Round rounds all numeric fields to 2 decimal places for output compactness.
// Call at the handler/output boundary — never inside math internals.
func (r *Result) Round() {
	r.Percentiles.Round()
	r.Spread.Round()
	r.FatTailRatio = stats.Round2(r.FatTailRatio)
	r.TailToMedianRatio = stats.Round2(r.TailToMedianRatio)
	r.ThroughputTrend.PercentageChange = stats.Round2(r.ThroughputTrend.PercentageChange)
	for k, p := range r.TypeSLEs {
		p.Round()
		r.TypeSLEs[k] = p
	}
	if decisions, ok := r.Context["stratification_decisions"].([]StratificationDecision); ok {
		for i := range decisions {
			decisions[i].Round()
		}
		r.Context["stratification_decisions"] = decisions
	}
	if v, ok := r.Context["throughput_overall"].(float64); ok {
		r.Context["throughput_overall"] = stats.Round2(v)
	}
	if v, ok := r.Context["throughput_recent"].(float64); ok {
		r.Context["throughput_recent"] = stats.Round2(v)
	}
	if dist, ok := r.Context["type_distribution"].(map[string]float64); ok {
		for k, v := range dist {
			dist[k] = stats.Round2(v)
		}
	}
}

func NewEngine(h *Histogram) *Engine {
	return &Engine{
		histogram: h,
		rng:       rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)),
	}
}

// SetSeed locks the Monte-Carlo simulation to a deterministic RNG sequence for tests.
func (e *Engine) SetSeed(seed int64) {
	e.rng = rand.New(rand.NewPCG(uint64(seed), 0))
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

func (e *Engine) assessPredictability(res *Result) {
	if res.Percentiles.CoinToss > 0 {
		res.FatTailRatio = math.Round(res.Percentiles.AlmostCertain/res.Percentiles.CoinToss*100) / 100
		res.TailToMedianRatio = math.Round(res.Percentiles.Likely/res.Percentiles.CoinToss*100) / 100

		predictability := "Stable"
		if res.FatTailRatio >= FatTailThreshold {
			predictability = "Unstable"
			res.Insights = append(res.Insights, fmt.Sprintf("Fat-Tail Warning (Ratio %.2f): Extreme outliers are in control of this process (Kanban heuristic >= 5.6). Your forecasts are high-risk.", res.FatTailRatio))
		}
		if res.TailToMedianRatio > HeavyTailThreshold {
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

			if diff < -ThroughputChangeThreshold {
				res.ThroughputTrend.Direction = "Declining"
				if diff < -ThroughputSevereDeclineThreshold {
					res.Warnings = append(res.Warnings, fmt.Sprintf("Significant throughput drop recently (%.0f%% below average). WIP may have increased or capacity dropped.", math.Abs(diff)*100))
				}
			} else if diff > ThroughputChangeThreshold {
				res.ThroughputTrend.Direction = "Increasing"
				if diff > ThroughputSevereDeclineThreshold {
					res.Warnings = append(res.Warnings, fmt.Sprintf("Throughput is significantly higher recently (%.0f%% above average). Monitor if this is sustainable.", diff*100))
				}
			}
		}
	}

	if analyzed, ok := e.histogram.Meta["issues_analyzed"].(int); ok && analyzed < 30 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("Simulation based on a small sample size (%d items); results may have limited statistical significance.", analyzed))
	}
}
