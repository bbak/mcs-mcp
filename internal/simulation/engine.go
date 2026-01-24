package simulation

import (
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
	CoinToss float64 `json:"coin_toss"`
	Likely   float64 `json:"likely"`
	P85      float64 `json:"p85"`
	P95      float64 `json:"p95"`
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

	return Result{
		CoinToss: float64(durations[int(float64(trials)*0.50)]),
		Likely:   float64(durations[int(float64(trials)*0.70)]),
		P85:      float64(durations[int(float64(trials)*0.85)]),
		P95:      float64(durations[int(float64(trials)*0.95)]),
	}
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

	return Result{
		CoinToss: float64(scopes[int(float64(trials)*0.50)]),
		Likely:   float64(scopes[int(float64(trials)*0.30)]), // 70% sure we do AT LEAST this much
		P85:      float64(scopes[int(float64(trials)*0.15)]), // 85% sure we do AT LEAST this much
		P95:      float64(scopes[int(float64(trials)*0.05)]), // 95% sure we do AT LEAST this much
	}
}

// RunCycleTimeAnalysis calculates percentiles from a list of historical cycle times (in days).
func (e *Engine) RunCycleTimeAnalysis(cycleTimes []float64) Result {
	if len(cycleTimes) == 0 {
		return Result{}
	}

	sort.Float64s(cycleTimes)
	n := len(cycleTimes)

	return Result{
		CoinToss: cycleTimes[int(float64(n)*0.50)],
		Likely:   cycleTimes[int(float64(n)*0.70)],
		P85:      cycleTimes[int(float64(n)*0.85)],
		P95:      cycleTimes[int(float64(n)*0.95)],
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
