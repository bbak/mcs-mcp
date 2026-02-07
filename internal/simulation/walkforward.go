package simulation

import (
	"fmt"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"sort"
	"time"
)

// WalkForwardConfig defines the parameters for the backtesting analysis.
type WalkForwardConfig struct {
	SourceID        string
	SimulationMode  string   // "duration" or "scope"
	LookbackWindow  int      // Days to look back for the validation (e.g., 90 days)
	StepSize        int      // Days between checkouts (e.g., 14 days)
	ForecastHorizon int      // Days to forecast into the future (only for Scope mode, e.g. 14 days)
	ItemsToForecast int      // Number of items (only for Duration mode)
	IssueTypes      []string // Optional: Filter by issue types
	Resolutions     map[string]string
}

// ValidationCheckpoint represents a single point in the past where we ran a simulation.
type ValidationCheckpoint struct {
	Date          string  `json:"date"`
	ActualValue   float64 `json:"actual_value"` // The real result (Days or ItemCount)
	PredictedP50  float64 `json:"predicted_p50"`
	PredictedP85  float64 `json:"predicted_p85"`
	PredictedP95  float64 `json:"predicted_p95"`
	IsWithinCone  bool    `json:"is_within_cone"` // Is actual between P10 and P95?
	DriftDetected bool    `json:"drift_detected"` // If true, this checkpoint is unreliable due to system change
}

// WalkForwardResult holds the aggregate results of the analysis.
type WalkForwardResult struct {
	AccuracyScore     float64                `json:"accuracy_score"` // % of checkpoints within cone
	Checkpoints       []ValidationCheckpoint `json:"checkpoints"`
	DriftWarning      string                 `json:"drift_warning,omitempty"`
	ValidationMessage string                 `json:"validation_message"`
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

	// 1. Detect System Drift (Three-Way Chart)
	// We analyze the last year of data to find if there was a major process shift.
	// We must reconstruction issues from "Now" to get the full history for stability check.
	// Efficient reconstruction:
	allIssues := w.reconstructAllIssuesAt(time.Now())
	w.analyzedIssues = allIssues

	finishedIssues := make([]jira.Issue, 0)
	for _, i := range allIssues {
		if i.ResolutionDate != nil {
			finishedIssues = append(finishedIssues, i)
		}
	}

	// Sort by resolution date
	sort.Slice(finishedIssues, func(i, j int) bool {
		return finishedIssues[i].ResolutionDate.Before(*finishedIssues[j].ResolutionDate)
	})

	// Get Cycle Times for Stability Analysis
	// We need a helper for cycle times, duplicating minimal logic here for cohesion
	// Ideally we inject a "CycleTimeProvider", but for now we calc simple duration
	cycleTimes := make([]float64, len(finishedIssues))
	for i, issue := range finishedIssues {
		cycleTimes[i] = issue.ResolutionDate.Sub(issue.Created).Hours() / 24.0 // Simplified Total Age for drift
		// Ideally use "Cycle Time" (Commit to Done), but Total Age is often a good proxy for massive drift
	}

	window := stats.NewAnalysisWindow(time.Now().AddDate(-1, 0, 0), time.Now(), "week", time.Time{})
	subgroups := stats.GroupIssuesByBucket(finishedIssues, cycleTimes, window)
	evolution := stats.CalculateThreeWayXmR(subgroups)

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
	now := time.Now()
	startTime := now.AddDate(0, 0, -cfg.LookbackWindow)
	if !driftDate.IsZero() && startTime.Before(driftDate) {
		startTime = driftDate
	}

	activeHits := 0
	totalHits := 0

	for d := now.AddDate(0, 0, -cfg.StepSize); d.After(startTime); d = d.AddDate(0, 0, -cfg.StepSize) {
		// 3. Time Travel: State at 'd'
		// Filter events for Simulation Input (only known at 'd')
		pastEvents := w.sliceEvents(d)

		// Reconstruct issues as they looked at 'd'
		pastIssues := w.reconstructAllIssues(pastEvents, d)

		// Build Histogram (Capability) based on 6 months PRIOR to 'd'
		historyStart := d.AddDate(0, -6, 0) // 6 months rolling window
		h := NewHistogram(pastIssues, historyStart, d, cfg.IssueTypes, w.mappings, w.resolutions)
		engine := NewEngine(h)

		// 4. Run Simulation & Verify
		cp := ValidationCheckpoint{Date: d.Format("2006-01-02")}

		if cfg.SimulationMode == "scope" {
			// FORECAST: "In next 14 days, check how many done"
			simRes := engine.RunScopeSimulation(cfg.ForecastHorizon, 5000)

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
			simRes := engine.RunDurationSimulation(cfg.ItemsToForecast, 5000)

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

		totalHits++
		result.Checkpoints = append(result.Checkpoints, cp)
	}

	if totalHits > 0 {
		result.AccuracyScore = float64(activeHits) / float64(totalHits)
		result.ValidationMessage = fmt.Sprintf("Walk-Forward Analysis: %d/%d (%.0f%%) of actual outcomes fell within the predicted forecast cone (P10-P98).", activeHits, totalHits, result.AccuracyScore*100)
	} else {
		result.ValidationMessage = "Insufficient historical data or drift constraints prevented meaningful backtesting."
	}

	if result.AccuracyScore < 0.7 && totalHits > 3 {
		result.ValidationMessage += " Warning: Low forecast reliability detected."
	}

	return result, nil
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
	// Group events by key
	// This is expensive to do in a loop.
	// Optimization: The engine should probably be initialized with already-grouped events?
	// Or we just accept the hit for the Proof of Concept.

	// Better: The valid full set of issues is ALREADY passed to us?
	// The problem is we need to know the state *At Time T*.
	// `ReconstructIssue` in eventlog takes a list of events.

	// We'll reuse the `reconstructAllIssues` helper below.
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

		// For the purpose of "Finished Map" in ReconstructIssue,
		// we can probably assume the standard Reconstruct logic handles explicit resolution events.
		// Passing empty map means it relies on explicit events, which is fine.
		issue := eventlog.ReconstructIssue(evts, nil, refDate)

		// Force Resolution Date to nil if it happened after refDate?
		// (Already handled by event slicing, but explicit check is good)
		if issue.ResolutionDate != nil && issue.ResolutionDate.After(refDate) {
			issue.ResolutionDate = nil
			issue.Resolution = ""
		}
		issues = append(issues, issue)
	}
	return issues
}

func (w *WalkForwardEngine) countFinishedInWindow(allIssues []jira.Issue, start, end time.Time) int {
	count := 0
	for _, i := range allIssues {
		if i.ResolutionDate != nil {
			if !i.ResolutionDate.Before(start) && (i.ResolutionDate.Before(end) || i.ResolutionDate.Equal(end)) {
				// Semantic Outcome check: only count "delivered" items for scope validation
				isDelivered := true
				if outcome, ok := w.resolutions[i.Resolution]; ok {
					if outcome != "delivered" {
						isDelivered = false
					}
				} else if m, ok := w.mappings[i.Status]; ok && m.Tier == "Finished" {
					if m.Outcome != "" && m.Outcome != "delivered" {
						isDelivered = false
					}
				}

				if isDelivered {
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
		if i.ResolutionDate != nil && i.ResolutionDate.After(start) {
			// Semantic Outcome check: only count "delivered" items for duration validation
			isDelivered := true
			if outcome, ok := w.resolutions[i.Resolution]; ok {
				if outcome != "delivered" {
					isDelivered = false
				}
			} else if m, ok := w.mappings[i.Status]; ok && m.Tier == "Finished" {
				if m.Outcome != "" && m.Outcome != "delivered" {
					isDelivered = false
				}
			}
			if isDelivered {
				resolvedAfter = append(resolvedAfter, *i.ResolutionDate)
			}
		}
	}

	if len(resolvedAfter) < n {
		return -1.0 // Not enough data
	}

	sort.Slice(resolvedAfter, func(i, j int) bool {
		return resolvedAfter[i].Before(resolvedAfter[j])
	})

	nthDate := resolvedAfter[n-1]
	return nthDate.Sub(start).Hours() / 24.0
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
