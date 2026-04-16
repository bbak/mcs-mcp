package simulation

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/rs/zerolog/log"
)

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
		workerSeed := e.rng.Uint64()
		go func(count int, seed uint64) {
			rng := rand.New(rand.NewPCG(seed, 0))
			res := make([]int, count)
			for i := range count {
				res[i] = e.simulateScopeTrialLocal(days, rng)
			}
			resultsChan <- res
		}(trialsPerGo, workerSeed)
	}

	scopes := make([]int, 0, trials)
	for g := 0; g < numGo; g++ {
		res := <-resultsChan
		scopes = append(scopes, res...)
	}

	slices.Sort(scopes)

	scopesF := intsToFloat64(scopes)
	res := Result{
		Percentiles:      percentilesFromSortedInverted(scopesF),
		Spread:           spreadFromSorted(scopesF),
		PercentileLabels: getPercentileLabels("scope"),
	}

	// Window exclusion warning
	if droppedWindow, ok := e.histogram.Meta["dropped_by_window"].(int); ok && droppedWindow > 0 {
		analyzed := e.histogram.Meta["issues_analyzed"].(int)
		total := analyzed + droppedWindow
		if total > 0 && float64(droppedWindow)/float64(total) > DroppedWindowWarnThreshold {
			res.Warnings = append(res.Warnings, fmt.Sprintf("WARNING: %d items (%d%%) were excluded because they fall outside the analysis time window. This suggests your time window is too narrow or the project is less active recently.", droppedWindow, int(float64(droppedWindow)/float64(total)*100)))
		}
	}

	// For scope, fat-tail detection is slightly different (low scope is the risk)
	// But we use the same formula on the delivery volume distribution for consistency
	// though usually fat-tail refers to Lead/Cycle Time.
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
	slices.Sort(capPool)
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
		workerSeed := e.rng.Uint64()
		go func(count int, seed uint64) {
			rng := rand.New(rand.NewPCG(seed, 0))
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
		}(trialsPerGo, workerSeed)
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

	slices.Sort(scopes)

	// Calculate median background items
	medianBG := make(map[string]int)
	for t, counts := range backgroundCounts {
		slices.Sort(counts)
		medianBG[t] = counts[len(counts)/2]
	}

	scopesF2 := intsToFloat64(scopes)
	res := Result{
		Percentiles:              percentilesFromSortedInverted(scopesF2),
		Spread:                   spreadFromSorted(scopesF2),
		PercentileLabels:         getPercentileLabels("scope"),
		BackgroundItemsPredicted: medianBG,
	}

	e.assessPredictability(&res)

	return res
}

func (e *Engine) simulateScopeTrialStratified(targetDays int, filterMap map[string]bool, capacityCap int, rng *rand.Rand) (int, map[string]int) {
	totalScope := 0
	bgItems := make(map[string]int)
	deps, _ := e.histogram.Meta["stratification_dependencies"].(map[string]string)

	// Pre-sort keys for determinism and performance
	types := make([]string, 0, len(e.histogram.StratifiedCounts))
	for t := range e.histogram.StratifiedCounts {
		types = append(types, t)
	}
	slices.Sort(types)

	taxers := make([]string, 0, len(deps))
	for taxer := range deps {
		taxers = append(taxers, taxer)
	}
	slices.Sort(taxers)

	for range targetDays {
		// 1. Independent Stratified Sampling (with Blending and Dependency Awareness)
		sampled := make(map[string]int)
		totalSampled := 0

		// Keys are pre-sorted for determinism outside the loop
		for _, t := range types {
			counts := e.histogram.StratifiedCounts[t]
			h := 0
			if len(counts) < 30 && rng.Float64() < 0.3 {
				if overall, ok := e.histogram.Meta["throughput_overall"].(float64); ok {
					if dist, ok := e.histogram.Meta["type_distribution"].(map[string]float64); ok {
						h = int(math.Round(overall * dist[t]))
					}
				}
			} else {
				idx := rng.IntN(len(counts))
				h = counts[idx]
			}
			sampled[t] = h
			totalSampled += h
		}

		// Dependency Awareness
		// Keys are pre-sorted for determinism outside the loop
		for _, taxer := range taxers { // Use pre-sorted taxers
			taxed := deps[taxer]
			if hT, ok := sampled[taxer]; ok && hT > 0 {
				if hD, ok := sampled[taxed]; ok && hD > 0 {
					reduction := int(math.Floor(float64(hT) * CapacityTaxRate))
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
			// Sort keys for deterministic RNG check
			typesCap := make([]string, 0, len(sampled))
			for t := range sampled {
				typesCap = append(typesCap, t)
			}
			slices.Sort(typesCap)

			for _, t := range typesCap {
				h := sampled[t]
				newH := int(math.Floor(float64(h) * factor))
				if newH == 0 && h > 0 && rng.Float64() < factor {
					newH = 1
				}
				sampled[t] = newH
			}
		}

		// 3. Count Scope
		for _, t := range types {
			h := sampled[t]
			if filterMap[t] || len(filterMap) == 0 {
				totalScope += h
			} else {
				bgItems[t] += h
			}
		}
	}

	return totalScope, bgItems
}

func (e *Engine) simulateMultiTypeScopeTrialLocal(targetDays int, filterMap map[string]bool, distribution map[string]float64, rng *rand.Rand) (int, map[string]int) {
	scope := 0
	bgItems := make(map[string]int)

	for range targetDays {
		idx := rng.IntN(len(e.histogram.Counts))
		slots := e.histogram.Counts[idx]
		for range slots {
			r := rng.Float64()
			var sampledType string
			acc := 0.0

			// Deterministic iteration over distribution
			distTypes := make([]string, 0, len(distribution))
			for t := range distribution {
				distTypes = append(distTypes, t)
			}
			slices.Sort(distTypes)

			for _, t := range distTypes {
				p := distribution[t]
				acc += p
				if r <= acc {
					sampledType = t
					break
				}
			}

			if sampledType == "" && len(distTypes) > 0 {
				sampledType = distTypes[0]
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

func (e *Engine) simulateScopeTrialLocal(targetDays int, rng *rand.Rand) int {
	totalScope := 0
	for range targetDays {
		idx := rng.IntN(len(e.histogram.Counts))
		totalScope += e.histogram.Counts[idx]
	}
	return totalScope
}
