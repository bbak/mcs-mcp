package simulation

import (
	"fmt"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/stats/discovery"
	"slices"
	"time"
)

// WalkForwardConfig defines the parameters for the backtesting analysis.
type WalkForwardConfig struct {
	SourceID        string
	SimulationMode  string   // "duration" or "scope"
	LookbackWindow  int      // Days to look back for the validation (e.g., 90 days)
	StepSize        int      // Days between checkouts (e.g., 7 days)
	ForecastHorizon int      // Days to forecast into the future (only for Scope mode, e.g. 14 days)
	ItemsToForecast int      // Number of items (only for Duration mode)
	IssueTypes      []string // Optional: Filter by issue types
	Resolutions     map[string]string
	EvaluationDate  time.Time

	// Stationarity tracking (Phase 2): required for per-checkpoint residence time analysis.
	CommitmentPoint string
	StatusWeights   map[string]int

	// SimulationSeed, when non-zero, seeds each checkpoint's RNG deterministically
	// as (SimulationSeed + checkpointIndex), making walk-forward results reproducible.
	SimulationSeed int64
}

// ValidationCheckpoint represents a single point in the past where we ran a simulation.
type ValidationCheckpoint struct {
	Date          string  `json:"date"`
	ActualValue   float64 `json:"actual_value"` // The real result (Days or ItemCount)
	PredictedP50  float64 `json:"predicted_p50"`
	PredictedP85  float64 `json:"predicted_p85"`
	PredictedP95  float64 `json:"predicted_p95"`
	IsWithinCone  bool    `json:"is_within_cone"`  // Is actual between P10 and P95?
	DriftDetected bool    `json:"drift_detected"`  // If true, this checkpoint is unreliable due to system change
	IsDegenerate  bool    `json:"is_degenerate,omitempty"` // If true, near-zero throughput made this forecast meaningless
	Convergence   string  `json:"convergence,omitempty"`    // Residence time convergence at this checkpoint
	NonStationary bool    `json:"non_stationary,omitempty"` // True if stationarity assessment fired warnings
}

// StationarityCorrelation summarizes whether non-stationarity correlates with forecast misses.
type StationarityCorrelation struct {
	NonStationaryCheckpoints int     `json:"non_stationary_checkpoints"`
	NonStationaryMissRate    float64 `json:"non_stationary_miss_rate"`  // Fraction of non-stationary checkpoints that missed
	StationaryMissRate       float64 `json:"stationary_miss_rate"`      // Fraction of stationary checkpoints that missed
	Signal                   string  `json:"signal"`                    // "predictive", "not_predictive", "insufficient_data"
}

// WalkForwardResult holds the aggregate results of the analysis.
type WalkForwardResult struct {
	AccuracyScore           float64                  `json:"accuracy_score"` // % of checkpoints within cone
	DegenerateCheckpoints   int                      `json:"degenerate_checkpoints,omitempty"`
	Checkpoints             []ValidationCheckpoint   `json:"checkpoints"`
	DriftWarning            string                   `json:"drift_warning,omitempty"`
	ValidationMessage       string                   `json:"validation_message"`
	StationarityCorrelation *StationarityCorrelation `json:"stationarity_correlation,omitempty"`
}

// WalkForwardEngine orchestrates the time-travel validation.
type WalkForwardEngine struct {
	events         []eventlog.IssueEvent
	mappings       map[string]stats.StatusMetadata
	resolutions    map[string]string
	analyzedIssues []jira.Issue
}

func NewWalkForwardEngine(events []eventlog.IssueEvent, mappings map[string]stats.StatusMetadata, resolutions map[string]string) *WalkForwardEngine {
	return &WalkForwardEngine{
		events:      events,
		mappings:    mappings,
		resolutions: resolutions,
	}
}

func (w *WalkForwardEngine) GetAnalyzedIssues() []jira.Issue {
	return w.analyzedIssues
}

// Execute performs the walk-forward analysis.
func (w *WalkForwardEngine) Execute(cfg WalkForwardConfig) (WalkForwardResult, error) {
	result := WalkForwardResult{
		Checkpoints: make([]ValidationCheckpoint, 0),
	}

	if cfg.EvaluationDate.IsZero() {
		cfg.EvaluationDate = time.Now()
	}

	// 1. Detect System Drift (Three-Way Chart)
	// We analyze the last year of data to find if there was a major process shift.
	// We must reconstruction issues from the evaluation date to get the full history for stability check.
	// Efficient reconstruction:
	allIssues := w.reconstructAllIssuesAt(cfg.EvaluationDate)
	w.analyzedIssues = allIssues

	finishedIssues := make([]jira.Issue, 0)
	for _, i := range allIssues {
		if i.ResolutionDate != nil {
			finishedIssues = append(finishedIssues, i)
		}
	}

	// Sort by resolution date
	slices.SortFunc(finishedIssues, func(a, b jira.Issue) int {
		return a.ResolutionDate.Compare(*b.ResolutionDate)
	})

	// Get Cycle Times for Stability Analysis
	// We need a helper for cycle times, duplicating minimal logic here for cohesion
	// Ideally we inject a "CycleTimeProvider", but for now we calc simple duration
	cycleTimes := make([]float64, len(finishedIssues))
	for i, issue := range finishedIssues {
		cycleTimes[i] = issue.ResolutionDate.Sub(issue.Created).Hours() / 24.0 // Simplified Total Age for drift
		// Ideally use "Cycle Time" (Commit to Done), but Total Age is often a good proxy for massive drift
	}

	window := stats.NewAnalysisWindow(cfg.EvaluationDate.AddDate(-1, 0, 0), cfg.EvaluationDate, "week", time.Time{})
	subgroups := stats.GroupIssuesByBucket(finishedIssues, cycleTimes, window)
	evolution := stats.CalculateThreeWayXmR(subgroups)

	// 1.1 Calculate Discovery Cutoff for the whole dataset
	finishedMap := make(map[string]bool)
	for name, m := range w.mappings {
		if m.Tier == "Finished" {
			finishedMap[name] = true
		}
	}
	globalCutoff := discovery.CalculateDiscoveryCutoff(allIssues, finishedMap)

	driftDate := time.Time{}
	if evolution.Status == "migrating" {
		// Find the first shift signal
		for _, s := range evolution.AverageChart.Signals {
			if s.Type == "shift" {
				// Rough approximation of when the shift started (subgroup index)
				if s.Index < len(subgroups) {
					// ISO Week label: "2024-W12"
					label := subgroups[s.Index].Label
					var year, week int
					if _, err := fmt.Sscanf(label, "%d-W%d", &year, &week); err == nil {
						// Convert ISO week to rough date (approximate start of week)
						driftDate = isoWeekToDate(year, week)
						result.DriftWarning = fmt.Sprintf("Systemic Process Drift detected around %s. Backtesting capped at this date.", driftDate.Format("2006-01-02"))
					}
				}
				break
			}
		}
	}

	// 2. Iterate Backwards
	now := cfg.EvaluationDate.Truncate(24 * time.Hour)
	startTime := now.AddDate(0, 0, -cfg.LookbackWindow)
	if !driftDate.IsZero() && startTime.Before(driftDate) {
		startTime = driftDate
	}

	activeHits := 0
	totalHits := 0
	checkpointIndex := 0

	for d := now.AddDate(0, 0, -cfg.StepSize); d.After(startTime); d = d.AddDate(0, 0, -cfg.StepSize) {
		// 3. Time Travel: State at 'd'
		// Filter events for Simulation Input (only known at 'd')
		pastEvents := w.sliceEvents(d)

		// Reconstruct issues as they looked at 'd'
		pastIssues := w.reconstructAllIssues(pastEvents, d)

		// 3.5. Stationarity assessment at this checkpoint (if commitment point available).
		// Runs before histogram construction so the experimental branch can narrow historyStart.
		historyStart := d.AddDate(0, 0, -90) // 90-day rolling window (aligned with MCS default)
		if globalCutoff != nil && historyStart.Before(*globalCutoff) {
			historyStart = *globalCutoff
			// If history range is too short (e.g. project just started),
			// the simulation will naturally have lower confidence/high spread.
		}

		var cpStationary bool = true
		var cpConvergence string
		var assessment *stats.StationarityAssessment
		if cfg.CommitmentPoint != "" && len(cfg.StatusWeights) > 0 {
			rtWindow := stats.NewAnalysisWindow(historyStart, d, "day", time.Time{})
			rtItems := stats.ExtractResidenceItems(pastIssues, cfg.CommitmentPoint, cfg.StatusWeights, w.mappings, historyStart)
			if len(rtItems) > 0 {
				rtResult := stats.ComputeResidenceTimeSeries(rtItems, rtWindow)
				assessment = stats.AssessStationarity(rtResult)
				cpStationary = assessment.Stationary
				cpConvergence = assessment.Convergence
			}
		}

		// Build Histogram (Capability) using historyStart.
		h := NewHistogram(pastIssues, historyStart, d, cfg.IssueTypes, w.mappings, w.resolutions)

		engine := NewEngine(h)
		if cfg.SimulationSeed != 0 {
			engine.SetSeed(cfg.SimulationSeed + int64(checkpointIndex))
		}
		checkpointIndex++

		// 4. Run Simulation & Verify
		cp := ValidationCheckpoint{
			Date:          d.Format("2006-01-02"),
			Convergence:   cpConvergence,
			NonStationary: !cpStationary,
		}

		if cfg.SimulationMode == "scope" {
			// FORECAST: "In next 14 days, check how many done"
			simRes := engine.RunScopeSimulation(cfg.ForecastHorizon, DefaultTrials)

			// ACTUAL: Look at 'fullHistory' (which contains future relative to 'd')
			// Count how many items finished between d and d + ForecastHorizon
			actualCount := w.countFinishedInWindow(allIssues, d, d.AddDate(0, 0, cfg.ForecastHorizon))

			cp.ActualValue = float64(actualCount)
			cp.PredictedP50 = simRes.Percentiles.CoinToss
			cp.PredictedP85 = simRes.Percentiles.Likely
			cp.PredictedP95 = simRes.Percentiles.Safe

			// In Scope Mode, Aggressive is high volume, AlmostCertain is low volume.
			minV := simRes.Percentiles.AlmostCertain
			maxV := simRes.Percentiles.Aggressive
			if cp.ActualValue >= minV-0.1 && cp.ActualValue <= maxV+0.1 {
				cp.IsWithinCone = true
				activeHits++
			}

		} else if cfg.SimulationMode == "duration" {
			// FORECAST: "How long to finish next N items?"
			if cfg.ItemsToForecast <= 0 {
				continue // Can't forecast 0 items
			}
			simRes := engine.RunDurationSimulation(cfg.ItemsToForecast, DefaultTrials)

			// ACTUAL: Find the Nth item resolved AFTER 'd'
			actualDays := w.measureDurationForNItems(allIssues, d, cfg.ItemsToForecast)

			if actualDays < 0 {
				// Not enough items finished yet in real history to verify this batch
				continue
			}

			cp.ActualValue = actualDays
			cp.PredictedP50 = simRes.Percentiles.CoinToss
			cp.PredictedP85 = simRes.Percentiles.Likely
			cp.PredictedP95 = simRes.Percentiles.Safe

			// Verification
			// In Duration Mode, Aggressive is fast (Low Days), AlmostCertain is slow (High Days)
			minV := simRes.Percentiles.Aggressive
			maxV := simRes.Percentiles.AlmostCertain
			if cp.ActualValue >= minV-0.1 && cp.ActualValue <= maxV+0.1 {
				cp.IsWithinCone = true
				activeHits++
			}
		}

		// Detect degenerate checkpoints (near-zero throughput → meaningless forecast)
		if cfg.SimulationMode == "duration" && (cp.PredictedP50 >= MaxForecastDays || cp.PredictedP85 >= MaxForecastDays || cp.PredictedP95 >= MaxForecastDays) {
			cp.IsDegenerate = true
		} else if cfg.SimulationMode == "scope" && cp.PredictedP50 == 0 && cp.PredictedP85 == 0 && cp.PredictedP95 == 0 {
			cp.IsDegenerate = true
		}

		if cp.IsDegenerate {
			result.DegenerateCheckpoints++
		} else {
			totalHits++
		}
		result.Checkpoints = append(result.Checkpoints, cp)
	}

	if totalHits > 0 {
		result.AccuracyScore = float64(activeHits) / float64(totalHits)
		result.ValidationMessage = fmt.Sprintf("Walk-Forward Analysis: %d/%d (%.0f%%) of actual outcomes fell within the predicted forecast cone (P10-P98).", activeHits, totalHits, result.AccuracyScore*100)
	} else {
		result.ValidationMessage = "Insufficient historical data or drift constraints prevented meaningful backtesting."
	}

	if result.DegenerateCheckpoints > 0 {
		result.ValidationMessage += fmt.Sprintf(" %d checkpoint(s) excluded due to near-zero throughput (degenerate).", result.DegenerateCheckpoints)
	}

	if result.AccuracyScore < 0.7 && totalHits > 3 {
		result.ValidationMessage += " Warning: Low forecast reliability detected."
	}

	// Compute stationarity correlation (only if we have stationarity data)
	result.StationarityCorrelation = computeStationarityCorrelation(result.Checkpoints)

	return result, nil
}

// computeStationarityCorrelation partitions checkpoints into stationary vs non-stationary
// and compares miss rates to determine if the stationarity signal is predictive.
func computeStationarityCorrelation(checkpoints []ValidationCheckpoint) *StationarityCorrelation {
	var stationaryTotal, stationaryMisses int
	var nonStationaryTotal, nonStationaryMisses int

	for _, cp := range checkpoints {
		if cp.IsDegenerate {
			continue
		}
		if cp.Convergence == "" {
			// No stationarity data for this checkpoint
			continue
		}
		if cp.NonStationary {
			nonStationaryTotal++
			if !cp.IsWithinCone {
				nonStationaryMisses++
			}
		} else {
			stationaryTotal++
			if !cp.IsWithinCone {
				stationaryMisses++
			}
		}
	}

	// Need data in both groups to be meaningful
	if stationaryTotal < 3 || nonStationaryTotal < 3 {
		if stationaryTotal+nonStationaryTotal == 0 {
			return nil // No stationarity data at all
		}
		return &StationarityCorrelation{
			NonStationaryCheckpoints: nonStationaryTotal,
			Signal:                   "insufficient_data",
		}
	}

	stationaryMissRate := float64(stationaryMisses) / float64(stationaryTotal)
	nonStationaryMissRate := float64(nonStationaryMisses) / float64(nonStationaryTotal)

	signal := "not_predictive"
	if nonStationaryMissRate > 2*stationaryMissRate && nonStationaryMissRate > 0.1 {
		signal = "predictive"
	}

	return &StationarityCorrelation{
		NonStationaryCheckpoints: nonStationaryTotal,
		NonStationaryMissRate:    stats.Round2(nonStationaryMissRate),
		StationaryMissRate:       stats.Round2(stationaryMissRate),
		Signal:                   signal,
	}
}

func (w *WalkForwardEngine) sliceEvents(cutoff time.Time) []eventlog.IssueEvent {
	// Optimization: Since events are chronological (mostly), we could binary search.
	// But simple loop is safer for now.
	limit := cutoff.UnixMicro()
	res := make([]eventlog.IssueEvent, 0, len(w.events))
	for _, e := range w.events {
		if e.Timestamp <= limit {
			res = append(res, e)
		}
	}
	return res
}

// reconstructAllIssuesAt reconstructs the state of ALL issues at a specific time.
// This duplicates some logic from LogProvider/Server but is necessary for the isolated engine.
func (w *WalkForwardEngine) reconstructAllIssuesAt(refDate time.Time) []jira.Issue {
	return w.reconstructAllIssues(w.events, refDate)
}

func (w *WalkForwardEngine) reconstructAllIssues(events []eventlog.IssueEvent, refDate time.Time) []jira.Issue {
	groups := make(map[string][]eventlog.IssueEvent)
	for _, e := range events {
		if e.Timestamp <= refDate.UnixMicro() {
			groups[e.IssueKey] = append(groups[e.IssueKey], e)
		}
	}

	issues := make([]jira.Issue, 0, len(groups))
	// We need a dummy "Finished" map.
	// For "Time Travel", we should infer finished status based on resolution event presence in the partial log.

	for _, evts := range groups {

		// For the purpose of "Finished Map" in MapIssueFromEvents,
		// Passing empty map means it relies on explicit events, which is fine.
		issue := eventlog.ReconstructIssue(evts, refDate)

		// Force Resolution Date to nil if it happened after refDate?
		// (Already handled by event slicing, but explicit check is good)
		if issue.ResolutionDate != nil && issue.ResolutionDate.After(refDate) {
			issue.ResolutionDate = nil
			issue.Resolution = ""
		}

		// NEW: Determine Outcome centrally using the engine's active mappings
		stats.DetermineOutcome(&issue, w.resolutions, w.mappings)
		issues = append(issues, issue)
	}
	return issues
}

func (w *WalkForwardEngine) countFinishedInWindow(allIssues []jira.Issue, start, end time.Time) int {
	count := 0
	for _, i := range allIssues {
		if i.OutcomeDate != nil {
			if !i.OutcomeDate.Before(start) && (i.OutcomeDate.Before(end) || i.OutcomeDate.Equal(end)) {
				if stats.IsDelivered(i) {
					count++
				}
			}
		}
	}
	return count
}

func (w *WalkForwardEngine) measureDurationForNItems(allIssues []jira.Issue, start time.Time, n int) float64 {
	// Find all items resolved AFTER start
	resolvedAfter := make([]time.Time, 0)
	for _, i := range allIssues {
		if i.OutcomeDate != nil && i.OutcomeDate.After(start) {
			if stats.IsDelivered(i) {
				resolvedAfter = append(resolvedAfter, *i.OutcomeDate)
			}
		}
	}

	if len(resolvedAfter) < n {
		return -1.0 // Not enough data
	}

	slices.SortFunc(resolvedAfter, func(a, b time.Time) int {
		return a.Compare(b)
	})

	nthDate := resolvedAfter[n-1]
	return nthDate.Sub(start).Hours() / 24.0
}

// MultiEngineResult holds per-engine backtest results for auto mode selection.
type MultiEngineResult struct {
	PerEngine map[string]WalkForwardResult `json:"per_engine"`
	Selected  string                        `json:"selected_engine"`
	Reason    string                        `json:"selection_reason"`
}

// ExecuteMultiEngine runs a walk-forward backtest using multiple ForecastEngines
// at each checkpoint and returns per-engine accuracy scores plus the selected engine.
func (w *WalkForwardEngine) ExecuteMultiEngine(cfg WalkForwardConfig, engines []ForecastEngine, weights map[string]int) (*MultiEngineResult, error) {
	if len(engines) == 0 {
		return nil, fmt.Errorf("no engines provided")
	}

	if cfg.EvaluationDate.IsZero() {
		cfg.EvaluationDate = time.Now()
	}

	allIssues := w.reconstructAllIssuesAt(cfg.EvaluationDate)

	// Discovery cutoff
	finishedMap := make(map[string]bool)
	for name, m := range w.mappings {
		if m.Tier == "Finished" {
			finishedMap[name] = true
		}
	}
	globalCutoff := discovery.CalculateDiscoveryCutoff(allIssues, finishedMap)

	// Drift detection (same as Execute)
	finishedIssues := make([]jira.Issue, 0)
	for _, i := range allIssues {
		if i.ResolutionDate != nil {
			finishedIssues = append(finishedIssues, i)
		}
	}
	slices.SortFunc(finishedIssues, func(a, b jira.Issue) int {
		return a.ResolutionDate.Compare(*b.ResolutionDate)
	})
	cycleTimes := make([]float64, len(finishedIssues))
	for i, issue := range finishedIssues {
		cycleTimes[i] = issue.ResolutionDate.Sub(issue.Created).Hours() / 24.0
	}
	window := stats.NewAnalysisWindow(cfg.EvaluationDate.AddDate(-1, 0, 0), cfg.EvaluationDate, "week", time.Time{})
	subgroups := stats.GroupIssuesByBucket(finishedIssues, cycleTimes, window)
	evolution := stats.CalculateThreeWayXmR(subgroups)

	driftDate := time.Time{}
	if evolution.Status == "migrating" {
		for _, s := range evolution.AverageChart.Signals {
			if s.Type == "shift" && s.Index < len(subgroups) {
				label := subgroups[s.Index].Label
				var year, week int
				if _, err := fmt.Sscanf(label, "%d-W%d", &year, &week); err == nil {
					driftDate = isoWeekToDate(year, week)
				}
				break
			}
		}
	}

	now := cfg.EvaluationDate.Truncate(24 * time.Hour)
	startTime := now.AddDate(0, 0, -cfg.LookbackWindow)
	if !driftDate.IsZero() && startTime.Before(driftDate) {
		startTime = driftDate
	}

	// Per-engine tracking
	type engineTracker struct {
		hits  int
		total int
	}
	trackers := make(map[string]*engineTracker)
	for _, e := range engines {
		trackers[e.Name()] = &engineTracker{}
	}

	checkpointIndex := 0
	for d := now.AddDate(0, 0, -cfg.StepSize); d.After(startTime); d = d.AddDate(0, 0, -cfg.StepSize) {
		pastEvents := w.sliceEvents(d)
		pastIssues := w.reconstructAllIssues(pastEvents, d)

		historyStart := d.AddDate(0, 0, -90)
		if globalCutoff != nil && historyStart.Before(*globalCutoff) {
			historyStart = *globalCutoff
		}

		cutoff := time.Time{}
		if globalCutoff != nil {
			cutoff = *globalCutoff
		}

		// Build ForecastRequest for this checkpoint
		req := ForecastRequest{
			Mode:             cfg.SimulationMode,
			AllIssues:        pastIssues,
			Finished:         pastIssues, // Engines filter internally
			WindowStart:      historyStart,
			WindowEnd:        d,
			DiscoveryCutoff:  cutoff,
			IssueTypes:       cfg.IssueTypes,
			CommitmentPoint:  cfg.CommitmentPoint,
			StatusWeights:    cfg.StatusWeights,
			WorkflowMappings: w.mappings,
			Resolutions:      w.resolutions,
			SimulationSeed:   cfg.SimulationSeed + int64(checkpointIndex),
			Clock:            d,
		}

		// Set mode-specific fields
		if cfg.SimulationMode == "scope" {
			req.TargetDays = cfg.ForecastHorizon
		} else if cfg.SimulationMode == "duration" {
			req.Targets = map[string]int{"_all": cfg.ItemsToForecast}
		}

		// Determine actual outcome
		var actualValue float64
		var hasActual bool

		if cfg.SimulationMode == "scope" {
			actualValue = float64(w.countFinishedInWindow(allIssues, d, d.AddDate(0, 0, cfg.ForecastHorizon)))
			hasActual = true
		} else if cfg.SimulationMode == "duration" && cfg.ItemsToForecast > 0 {
			days := w.measureDurationForNItems(allIssues, d, cfg.ItemsToForecast)
			if days >= 0 {
				actualValue = days
				hasActual = true
			}
		}

		if !hasActual {
			checkpointIndex++
			continue
		}

		// Run each engine and evaluate
		for _, eng := range engines {
			result, err := eng.Run(req)
			if err != nil {
				checkpointIndex++
				continue
			}

			tracker := trackers[eng.Name()]

			// Check if degenerate
			isDegenerate := false
			if cfg.SimulationMode == "duration" && (result.Percentiles.CoinToss >= MaxForecastDays || result.Percentiles.Likely >= MaxForecastDays) {
				isDegenerate = true
			} else if cfg.SimulationMode == "scope" && result.Percentiles.CoinToss == 0 && result.Percentiles.Likely == 0 {
				isDegenerate = true
			}

			if isDegenerate {
				continue
			}

			tracker.total++

			// Check if within cone
			var minV, maxV float64
			if cfg.SimulationMode == "scope" {
				minV = result.Percentiles.AlmostCertain
				maxV = result.Percentiles.Aggressive
			} else {
				minV = result.Percentiles.Aggressive
				maxV = result.Percentiles.AlmostCertain
			}
			if actualValue >= minV-0.1 && actualValue <= maxV+0.1 {
				tracker.hits++
			}
		}
		checkpointIndex++
	}

	// Select the best engine
	res := &MultiEngineResult{
		PerEngine: make(map[string]WalkForwardResult),
	}

	bestScore := -1.0
	bestWeight := -1
	for _, eng := range engines {
		t := trackers[eng.Name()]
		score := 0.0
		if t.total > 0 {
			score = float64(t.hits) / float64(t.total)
		}
		res.PerEngine[eng.Name()] = WalkForwardResult{
			AccuracyScore:     score,
			ValidationMessage: fmt.Sprintf("%s: %d/%d (%.0f%%)", eng.Name(), t.hits, t.total, score*100),
		}

		w := weights[eng.Name()]
		if score > bestScore || (score == bestScore && w > bestWeight) {
			bestScore = score
			bestWeight = w
			res.Selected = eng.Name()
		}
	}

	// Build reason string
	var parts []string
	for _, eng := range engines {
		t := trackers[eng.Name()]
		score := 0.0
		if t.total > 0 {
			score = float64(t.hits) / float64(t.total)
		}
		parts = append(parts, fmt.Sprintf("%s: %.0f%%", eng.Name(), score*100))
	}
	res.Reason = fmt.Sprintf("accuracy: %s", fmt.Sprintf("%v", parts))

	return res, nil
}

func isoWeekToDate(year, week int) time.Time {
	// Move to the first Monday of the year (ISO weeks start on Monday)
	// We use the property that the first ISO week of the year contains Jan 4th.
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	isoYear, isoWeek := jan4.ISOWeek()
	for isoYear != year || isoWeek != 1 {
		jan4 = jan4.AddDate(0, 0, -1)
		isoYear, isoWeek = jan4.ISOWeek()
	}

	// Move forward to the requested week
	return jan4.AddDate(0, 0, (week-1)*7)
}
