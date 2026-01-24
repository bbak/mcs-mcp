package simulation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"
)

// Engine performs the Monte-Carlo simulation.
type Engine struct {
	histogram *Histogram
	rng       *rand.Rand
}

// Result holds the percentiles of a simulation or analysis.
type Result struct {
	CoinToss       float64                `json:"coin_toss"`
	Likely         float64                `json:"likely"`
	P85            float64                `json:"p85"`
	P95            float64                `json:"p95"`
	P98            float64                `json:"p98"`
	Ratio          float64                `json:"ratio"`
	Predictability string                 `json:"predictability"`
	Context        map[string]interface{} `json:"context,omitempty"`
	Warnings       []string               `json:"warnings,omitempty"`
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
		CoinToss: float64(durations[int(float64(trials)*0.50)]),
		Likely:   float64(durations[int(float64(trials)*0.70)]),
		P85:      float64(durations[int(float64(trials)*0.85)]),
		P95:      float64(durations[int(float64(trials)*0.95)]),
		P98:      float64(durations[int(float64(trials)*0.98)]),
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
		CoinToss: float64(scopes[int(float64(trials)*0.50)]),
		Likely:   float64(scopes[int(float64(trials)*0.30)]), // 70% sure we do AT LEAST this much
		P85:      float64(scopes[int(float64(trials)*0.15)]), // 85% sure we do AT LEAST this much
		P95:      float64(scopes[int(float64(trials)*0.05)]), // 95% sure we do AT LEAST this much
		P98:      float64(scopes[int(float64(trials)*0.02)]), // 98% sure we do AT LEAST this much
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
		CoinToss: cycleTimes[int(float64(n)*0.50)],
		Likely:   cycleTimes[int(float64(n)*0.70)],
		P85:      cycleTimes[int(float64(n)*0.85)],
		P95:      cycleTimes[int(float64(n)*0.95)],
		P98:      cycleTimes[int(float64(n)*0.98)],
	}
	e.assessPredictability(&res)
	return res
}

func (e *Engine) assessPredictability(res *Result) {
	if res.CoinToss > 0 {
		res.Ratio = math.Round(res.P98/res.CoinToss*100) / 100
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

		// 1. Throughput Trend Warning
		if recent, ok := e.histogram.Meta["throughput_recent"].(float64); ok {
			if overall, ok := e.histogram.Meta["throughput_overall"].(float64); ok && overall > 0 {
				diff := (recent - overall) / overall
				if diff < -0.3 {
					res.Warnings = append(res.Warnings, fmt.Sprintf("Significant throughput drop recently (%.0f%% below average). WIP may have increased or capacity dropped.", math.Abs(diff)*100))
				} else if diff > 0.3 {
					res.Warnings = append(res.Warnings, fmt.Sprintf("Throughput is significantly higher recently (%.0f%% above average). Monitor if this is sustainable.", diff*100))
				}
			}
		}

		// 2. Data Volume Warning
		if analyzed, ok := e.histogram.Meta["issues_analyzed"].(int); ok && analyzed < 30 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Small sample size (%d items). Statistical confidence is low.", analyzed))
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
