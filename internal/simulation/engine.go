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

// Result holds the percentiles of the simulation.
type Result struct {
	P50 int `json:"p50"`
	P85 int `json:"p85"`
	P95 int `json:"p95"`
}

func NewEngine(h *Histogram) *Engine {
	return &Engine{
		histogram: h,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run performs requested number of simulation trials.
func (e *Engine) Run(backlogSize int, trials int) Result {
	if len(e.histogram.Counts) == 0 {
		return Result{}
	}

	durations := make([]int, trials)

	for i := 0; i < trials; i++ {
		durations[i] = e.simulateTrial(backlogSize)
	}

	sort.Ints(durations)

	return Result{
		P50: durations[int(float64(trials)*0.50)],
		P85: durations[int(float64(trials)*0.85)],
		P95: durations[int(float64(trials)*0.95)],
	}
}

func (e *Engine) simulateTrial(backlog int) int {
	days := 0
	remaining := backlog

	for remaining > 0 {
		days++
		// Randomly sample a day from history
		idx := e.rng.Intn(len(e.histogram.Counts))
		throughput := e.histogram.Counts[idx]
		remaining -= throughput

		if days > 10000 { // Safety brake for 0-throughput histograms
			break
		}
	}

	return days
}
