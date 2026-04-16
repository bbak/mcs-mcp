package simulation

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
)

// RunDurationSimulation is a backward-compatible wrapper for simple backlog forecasts.
// It assumes the backlog follows the historical distribution.
func (e *Engine) RunDurationSimulation(backlogSize int, trials int) Result {
	targets := make(map[string]int)
	dist := make(map[string]float64)

	if e.histogram != nil {
		if d, ok := e.histogram.Meta["type_distribution"].(map[string]float64); ok && len(d) > 0 {
			dist = d
			sumProbs := 0.0
			for _, p := range dist {
				sumProbs += p
			}
			if math.Abs(sumProbs-1.0) > 0.05 {
				log.Warn().Float64("sum", sumProbs).Msg("type_distribution probabilities do not sum to ~1.0; forecast target counts may be off")
			}
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

	// 1. Sanitize targets: remove types with no delivery history.
	// A target type absent from the distribution can never be sampled, so its
	// remaining count never reaches zero — causing every trial to run to MaxForecastDays.
	unforecastableTypes := make([]string, 0)
	unforecastableCount := 0
	sanitizedTargets := make(map[string]int, len(targets))
	for t, c := range targets {
		if distribution[t] > 0 {
			sanitizedTargets[t] = c
		} else {
			unforecastableTypes = append(unforecastableTypes, t)
			unforecastableCount += c
		}
	}
	slices.Sort(unforecastableTypes)
	targets = sanitizedTargets
	if len(targets) == 0 {
		res := Result{PercentileLabels: getPercentileLabels("duration")}
		res.Warnings = append(res.Warnings, fmt.Sprintf("CAUTION: %d item(s) of type(s) [%s] have no delivery history and were excluded from the duration forecast. Their completion cannot be estimated.", unforecastableCount, strings.Join(unforecastableTypes, ", ")))
		return res
	}

	// 2. Determine if we should use stratification
	useStratification := false
	if eligible, ok := e.histogram.Meta["stratification_eligible"].(map[string]bool); ok {
		for _, isEligible := range eligible {
			if isEligible {
				useStratification = true
				break
			}
		}
	}

	// 3. Prepare final distribution for pooled fallback
	finalDist := make(map[string]float64)
	if !expansionEnabled || len(distribution) == 0 {
		targetTotalProb := 0.0
		if len(distribution) > 0 {
			for t := range targets {
				targetTotalProb += distribution[t]
			}
		}
		if targetTotalProb > 0 {
			for t := range targets {
				finalDist[t] = distribution[t] / targetTotalProb
			}
		} else {
			prob := 1.0
			if len(targets) > 0 {
				prob = 1.0 / float64(len(targets))
			}
			for t := range targets {
				finalDist[t] = prob
			}
		}
	} else {
		for t, p := range distribution {
			finalDist[t] = p
		}
	}

	// Always ensure we are iterating over a stable distribution for background counts
	// rather than a potentially nil original distribution.
	trackedDistribution := finalDist

	// 4. Parallel Execution Setup
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
	slices.Sort(capPool)
	capacityCap := 1
	if len(capPool) > 0 {
		capacityCap = max(capPool[int(float64(len(capPool))*0.95)], 1)
	}

	log.Info().Int("trials", trials).Interface("targets", targets).Bool("stratified", useStratification).Msg("Starting multi-type duration simulation")

	for g := 0; g < numGo; g++ {
		workerSeed := e.rng.Uint64()
		go func(count int, seed uint64) {
			rng := rand.New(rand.NewPCG(seed, 0))
			res := trialResult{
				durations: make([]int, count),
				bgCounts:  make(map[string][]int),
			}
			for t := range trackedDistribution {
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
		}(trialsPerGo, workerSeed)
	}

	// Aggregate Results
	durations := make([]int, 0, trials)
	backgroundCounts := make(map[string][]int)
	for t := range trackedDistribution {
		backgroundCounts[t] = make([]int, 0, trials)
	}

	for g := 0; g < numGo; g++ {
		res := <-resultsChan
		durations = append(durations, res.durations...)
		for t, counts := range res.bgCounts {
			backgroundCounts[t] = append(backgroundCounts[t], counts...)
		}
	}

	slices.Sort(durations)

	// Calculate median background items
	medianBG := make(map[string]int)
	for t, counts := range backgroundCounts {
		slices.Sort(counts)
		medianBG[t] = counts[trials/2]
	}

	durationsF := intsToFloat64(durations)
	res := Result{
		Percentiles:              percentilesFromSorted(durationsF),
		Spread:                   spreadFromSorted(durationsF),
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
			if v >= FatTailThreshold {
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
	} else if durations[int(float64(trials)*0.50)] >= MaxForecastDays { // > 10 years
		res.Warnings = append(res.Warnings, "WARNING: Forecast exceeds 10 years. This usually indicates 'Throughput Collapse' due to overly restrictive filters (Issue Types or Resolutions).")
	}

	// Outcome Density Warning
	if dropped, ok := e.histogram.Meta["dropped_by_outcome"].(int); ok && dropped > 0 {
		analyzed := e.histogram.Meta["issues_analyzed"].(int)
		total := analyzed + dropped
		if total > 0 && float64(analyzed)/float64(total) < 0.2 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("CAUTION: Low Outcome Density. %.1f%% of resolved items were excluded because they were not 'delivered' (e.g. abandoned). This may skew results if 'delivered' items are missing. Check your 'resolutions' parameter.", float64(analyzed)/float64(total)*100))
		}
	}

	// Window exclusion warning
	if droppedWindow, ok := e.histogram.Meta["dropped_by_window"].(int); ok && droppedWindow > 0 {
		analyzed := e.histogram.Meta["issues_analyzed"].(int)
		total := analyzed + droppedWindow
		if total > 0 && float64(droppedWindow)/float64(total) > DroppedWindowWarnThreshold {
			res.Warnings = append(res.Warnings, fmt.Sprintf("WARNING: %d items (%d%%) were excluded because they fall outside the analysis time window. This suggests your time window is too narrow or the project is less active recently.", droppedWindow, int(float64(droppedWindow)/float64(total)*100)))
		}
	}

	// Unforecastable types warning
	if len(unforecastableTypes) > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("CAUTION: %d item(s) of type(s) [%s] have no delivery history and were excluded from the duration forecast. Their completion cannot be estimated.", unforecastableCount, strings.Join(unforecastableTypes, ", ")))
	}

	slices.Sort(res.Insights)
	slices.Sort(res.Warnings)
	return res
}


func (e *Engine) simulateDurationTrialStratified(targets map[string]int, capacityCap int, rng *rand.Rand) (int, map[string]int) {
	days := 0
	remaining := make(map[string]int)
	totalRemaining := 0
	for t, c := range targets {
		remaining[t] = c
		totalRemaining += c
	}
	originalRemaining := totalRemaining

	background := make(map[string]int)
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

	for totalRemaining > 0 {
		days++

		// 1. Independent Stratified Sampling (with Bayesian Blending and Dependency Awareness)
		sampled := make(map[string]int)
		totalSampled := 0

		for _, t := range types {
			counts := e.histogram.StratifiedCounts[t]
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
				idx := rng.IntN(len(counts))
				h = counts[idx]
			}

			sampled[t] = h
			totalSampled += h
		}

		// Dependency Awareness (Statistical Bug-Tax)
		for _, taxer := range taxers {
			taxed := deps[taxer]
			if hT, ok := sampled[taxer]; ok && hT > 0 {
				if hD, ok := sampled[taxed]; ok && hD > 0 {
					// If taxer is active, we apply a pressure based on its impact
					// High taxer volume squeezes the taxed volume further than just the global cap
					if hT > 0 {
						// Simple heuristic: reduce taxed by a fraction of taxer
						reduction := int(math.Floor(float64(hT) * CapacityTaxRate))
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

			// Deterministic iteration over sampled keys
			sampledTypes := make([]string, 0, len(sampled))
			for t := range sampled {
				sampledTypes = append(sampledTypes, t)
			}
			slices.Sort(sampledTypes)

			for _, t := range sampledTypes {
				h := sampled[t]
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
		for _, t := range types {
			h := sampled[t]
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

		if days >= MaxForecastDays {
			break
		}

		if days%StallCheckInterval == 0 {
			if totalRemaining == originalRemaining {
				return MaxForecastDays, background
			}
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
	originalRemaining := totalRemaining

	background := make(map[string]int)

	// Pre-sort keys for determinism and performance
	distTypes := make([]string, 0, len(distribution))
	for t := range distribution {
		distTypes = append(distTypes, t)
	}
	slices.Sort(distTypes)

	// Early check for infinite loop
	totalProb := 0.0
	for t := range distribution {
		totalProb += distribution[t]
	}
	if totalProb == 0 {
		return 3650, background
	}

	for totalRemaining > 0 {
		days++
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

			if count, ok := remaining[sampledType]; ok && count > 0 {
				remaining[sampledType]--
				totalRemaining--
			} else if sampledType != "" {
				background[sampledType]++
			}

			if totalRemaining <= 0 {
				break
			}
		}

		if days >= MaxForecastDays {
			break
		}

		if days%1000 == 0 {
			// Stall Detection: If we've run 1,000 cycles and delivered nothing, break infinite loop
			if totalRemaining == originalRemaining {
				return 3650, background
			}
		}
	}
	return days, background
}
