package stats

import (
	"math"
	"time"

	"mcs-mcp/internal/jira"
)

// noCommitmentWeight is a sentinel value used when no commitment point is configured.
// It is intentionally high so that all statuses appear "before" commitment,
// meaning no items are counted as WIP until a real commitment point is set.
const noCommitmentWeight = 999

// WIPRunChartPoint represents the number of active WIP items on a specific date.
type WIPRunChartPoint struct {
	Date  time.Time `json:"date"`
	Count int       `json:"count"`
}

// WIPStabilityResult encapsulates both the daily run chart and the weekly XmR analysis.
type WIPStabilityResult struct {
	RunChart []WIPRunChartPoint `json:"run_chart"`
	XmR      XmRResult          `json:"xmr"` // Limits derived from weekly samples
	Status   string             `json:"status"`
}

// AnalyzeHistoricalWIP generates a daily run chart and calculates weekly-sampled XmR limits.
// It traces issue transitions to determine historical system population.
func AnalyzeHistoricalWIP(issues []jira.Issue, window AnalysisWindow, commitmentPoint string, weights map[string]int, mappings map[string]StatusMetadata) WIPStabilityResult {
	if len(issues) == 0 {
		return WIPStabilityResult{Status: "stable"}
	}

	runChart := CalculateWIPRunChart(issues, window, commitmentPoint, weights, mappings)
	if len(runChart) == 0 {
		return WIPStabilityResult{Status: "stable"}
	}

	// 1. Extract weekly samples to avoid autocorrelation in the limits
	var weeklySamples []float64
	var weeklyDates []time.Time

	currentWeekEnd := SnapToEnd(runChart[0].Date, "week")

	// We iterate through the run chart and pick the last point of each week.
	for i, point := range runChart {
		// If this is the last point in the chart, OR the next point belongs to a new week
		isLastInWeek := false
		if i == len(runChart)-1 {
			isLastInWeek = true
		} else {
			nextPointWeekEnd := SnapToEnd(runChart[i+1].Date, "week")
			if !nextPointWeekEnd.Equal(currentWeekEnd) {
				isLastInWeek = true
				currentWeekEnd = nextPointWeekEnd
			}
		}

		if isLastInWeek {
			weeklySamples = append(weeklySamples, float64(point.Count))
			weeklyDates = append(weeklyDates, point.Date)
		}
	}

	// 2. Calculate XmR limits using only the weekly samples
	var keys []string
	for _, dt := range weeklyDates {
		keys = append(keys, window.GenerateLabel(dt))
	}
	xmr := CalculateXmRWithKeys(weeklySamples, keys)

	// 3. Map the limits back across the *entire* daily run chart to find signals
	// Since XmR calculates signals only on the samples, we need to do a custom sweep
	// on the daily points against the weekly-derived limits.
	var dailySignals []Signal
	for i, point := range runChart {
		key := point.Date.Format("2006-01-02")
		val := float64(point.Count)

		if val > xmr.UNPL {
			dailySignals = append(dailySignals, Signal{
				Index:       i,
				Key:         key,
				Type:        "outlier",
				Description: "WIP count above Upper Natural Process Limit (UNPL)",
			})
		} else if val < xmr.LNPL {
			dailySignals = append(dailySignals, Signal{
				Index:       i,
				Key:         key,
				Type:        "outlier",
				Description: "WIP count below Lower Natural Process Limit (LNPL)",
			})
		}

		// Note: We don't port the 8-point rule to the daily chart seamlessly without recalculating
		// shifts on the daily average. For now, we rely on the limits.
	}

	// Replace the sampled signals with the daily ones to provide context on the UI
	xmr.Signals = dailySignals

	status := "stable"
	if len(dailySignals) > 0 {
		status = "unstable"
	}

	return WIPStabilityResult{
		RunChart: runChart,
		XmR:      xmr,
		Status:   status,
	}
}

// ActiveRange represents a continuous interval where an issue was in WIP state.
// Timestamps are in Unix microseconds.
type ActiveRange struct {
	EnterTS int64
	ExitTS  int64
}

// BuildActiveRanges determines the WIP intervals [enter, exit] for each issue
// based on commitment point weight, transition history, and status mappings.
// WIP membership is defined by the commitment point (weight-based), not by tier alone.
// Returns a slice parallel to the input issues slice; each element contains
// that issue's WIP intervals (possibly empty if the issue was never WIP).
func BuildActiveRanges(issues []jira.Issue, commitmentPoint string, weights map[string]int, mappings map[string]StatusMetadata) [][]ActiveRange {
	allActiveRanges := make([][]ActiveRange, len(issues))

	commitmentWeight := weights[commitmentPoint]
	if commitmentWeight == 0 {
		commitmentWeight = noCommitmentWeight
	}

	evaluateWipState := func(statusID, status string, currentIsWip bool) bool {
		t := DetermineTier(jira.Issue{StatusID: statusID, Status: status}, "", mappings)

		if t == "Finished" || t == "Demand" {
			return false
		}

		w, hasWeight := weights[status]
		if hasWeight {
			return w >= commitmentWeight
		}

		// Fallback for branching/parallel statuses not in the explicit backbone
		if t == "Downstream" {
			return true
		}

		// Unknown mapped status, or entirely unmapped. Maintain current WIP state.
		return currentIsWip
	}

	for idx, issue := range issues {
		var currentEnter int64 = 0
		var issueRanges []ActiveRange

		// Determine starting state
		isWip := evaluateWipState(issue.BirthStatusID, issue.BirthStatus, false)
		if isWip {
			currentEnter = issue.Created.UnixMicro()
		}

		for _, tr := range issue.Transitions {
			newIsWip := evaluateWipState(tr.ToStatusID, tr.ToStatus, isWip)

			if !isWip && newIsWip {
				// Entering active WIP
				currentEnter = tr.Date.UnixMicro()
			} else if isWip && !newIsWip {
				// Exiting active WIP (either backflow or finished)
				issueRanges = append(issueRanges, ActiveRange{
					EnterTS: currentEnter,
					ExitTS:  tr.Date.UnixMicro(),
				})
				currentEnter = 0
			}
			isWip = newIsWip
		}

		// Handle items still in WIP at the end of their recorded timeline
		if currentEnter > 0 {
			var exitTS int64 = math.MaxInt64

			// If it has a resolution date but is still somehow registered as WIP, cap it there
			if issue.ResolutionDate != nil {
				exitTS = issue.ResolutionDate.UnixMicro()
			}

			issueRanges = append(issueRanges, ActiveRange{
				EnterTS: currentEnter,
				ExitTS:  exitTS,
			})
		}

		allActiveRanges[idx] = issueRanges
	}

	return allActiveRanges
}

// dayBounds holds the start and end timestamps (Unix microseconds) for a single day.
type dayBounds struct {
	start int64
	end   int64
}

// buildDayTimeline constructs the sequence of days and their bounds within the analysis window.
func buildDayTimeline(window AnalysisWindow) ([]time.Time, []dayBounds) {
	var days []time.Time
	current := SnapToStart(window.Start, "day")
	endBucket := SnapToStart(window.End, "day")

	for !current.After(endBucket) {
		days = append(days, current)
		current = current.AddDate(0, 0, 1)
	}

	bounds := make([]dayBounds, len(days))
	for i, d := range days {
		bounds[i] = dayBounds{
			start: d.UnixMicro(),
			end:   SnapToEnd(d, "day").UnixMicro(),
		}
	}

	return days, bounds
}

// isActiveOnDay checks whether an issue's active ranges indicate WIP presence on a given day,
// using End-Of-Day Snapshot logic with the Kanban Same-Day exception.
func isActiveOnDay(ranges []ActiveRange, b dayBounds) bool {
	for _, r := range ranges {
		startsBeforeEnd := r.EnterTS <= b.end
		survivesUntilEnd := r.ExitTS > b.end
		startedToday := r.EnterTS >= b.start

		if startsBeforeEnd {
			// 1. Snapshot: Was it active precisely at midnight (23:59:59) of this bucket?
			if survivesUntilEnd {
				return true
			}
			// 2. Exception: Day 1 Rule - Did it start AND finish completely within this day?
			if startedToday {
				return true
			}
		}
	}
	return false
}

// CalculateWIPRunChart traces the history of all issues to find active WIP on each day.
// It counts an item as WIP for ANY portion of a day where it resided in one of the activeWipStatuses.
// If an item backflows out of the active statuses, it ceases to be WIP for those days.
func CalculateWIPRunChart(issues []jira.Issue, window AnalysisWindow, commitmentPoint string, weights map[string]int, mappings map[string]StatusMetadata) []WIPRunChartPoint {
	days, bounds := buildDayTimeline(window)
	allActiveRanges := BuildActiveRanges(issues, commitmentPoint, weights, mappings)

	chart := make([]WIPRunChartPoint, len(days))

	for i, d := range days {
		count := 0
		for _, issueRanges := range allActiveRanges {
			if isActiveOnDay(issueRanges, bounds[i]) {
				count++
			}
		}
		chart[i] = WIPRunChartPoint{
			Date:  d,
			Count: count,
		}
	}

	return chart
}

// WIPAgeRunChartPoint represents the total WIP age across all active items on a specific date.
type WIPAgeRunChartPoint struct {
	Date       time.Time `json:"date"`
	TotalAge   float64   `json:"total_age"`   // Sum of individual WIP ages in days
	Count      int       `json:"count"`        // Number of WIP items (for context)
	AverageAge float64   `json:"average_age"`  // TotalAge / Count (convenience, not used for XmR)
}

// WIPAgeStabilityResult encapsulates the daily run chart and weekly XmR analysis of Total WIP Age.
type WIPAgeStabilityResult struct {
	RunChart []WIPAgeRunChartPoint `json:"run_chart"`
	XmR      XmRResult             `json:"xmr"`    // XmR applied to Total WIP Age (not average)
	Status   string                `json:"status"`
}

// CalculateTotalWIPAgeRunChart traces the history of all issues to compute
// the total WIP age (sum of individual ages) on each day in the window.
// WIP age for an item on day D is the cumulative time it has spent in WIP up to that day.
func CalculateTotalWIPAgeRunChart(issues []jira.Issue, window AnalysisWindow, commitmentPoint string, weights map[string]int, mappings map[string]StatusMetadata) []WIPAgeRunChartPoint {
	days, bounds := buildDayTimeline(window)
	allActiveRanges := BuildActiveRanges(issues, commitmentPoint, weights, mappings)

	const microsecondsPerDay = 86400.0 * 1e6

	chart := make([]WIPAgeRunChartPoint, len(days))

	for i, d := range days {
		b := bounds[i]
		var totalAge float64
		count := 0

		for _, issueRanges := range allActiveRanges {
			if !isActiveOnDay(issueRanges, b) {
				continue
			}
			count++

			// Compute cumulative WIP age for this item as of day D.
			// Sum all time spent in WIP intervals up to the end of this day.
			var ageMicroseconds int64
			for _, r := range issueRanges {
				if r.ExitTS <= b.start {
					// Completed interval entirely before this day — contributes full duration
					ageMicroseconds += r.ExitTS - r.EnterTS
				} else if r.EnterTS <= b.end {
					// Interval overlaps this day — contributes up to end of day
					end := b.end
					if r.ExitTS <= b.end {
						// Same-day completion: use actual exit
						end = r.ExitTS
					}
					ageMicroseconds += end - r.EnterTS
				}
				// Intervals starting after this day are ignored
			}

			totalAge += float64(ageMicroseconds) / microsecondsPerDay
		}

		avgAge := 0.0
		if count > 0 {
			avgAge = totalAge / float64(count)
		}

		chart[i] = WIPAgeRunChartPoint{
			Date:       d,
			TotalAge:   math.Round(totalAge*10) / 10,
			Count:      count,
			AverageAge: math.Round(avgAge*10) / 10,
		}
	}

	return chart
}

// AnalyzeHistoricalWIPAge generates a daily run chart of Total WIP Age and calculates
// weekly-sampled XmR limits. XmR is applied to Total WIP Age (not average).
func AnalyzeHistoricalWIPAge(issues []jira.Issue, window AnalysisWindow, commitmentPoint string, weights map[string]int, mappings map[string]StatusMetadata) WIPAgeStabilityResult {
	if len(issues) == 0 {
		return WIPAgeStabilityResult{Status: "stable"}
	}

	runChart := CalculateTotalWIPAgeRunChart(issues, window, commitmentPoint, weights, mappings)
	if len(runChart) == 0 {
		return WIPAgeStabilityResult{Status: "stable"}
	}

	// 1. Extract weekly samples to avoid autocorrelation in the limits
	var weeklySamples []float64
	var weeklyDates []time.Time

	currentWeekEnd := SnapToEnd(runChart[0].Date, "week")

	for i, point := range runChart {
		isLastInWeek := false
		if i == len(runChart)-1 {
			isLastInWeek = true
		} else {
			nextPointWeekEnd := SnapToEnd(runChart[i+1].Date, "week")
			if !nextPointWeekEnd.Equal(currentWeekEnd) {
				isLastInWeek = true
				currentWeekEnd = nextPointWeekEnd
			}
		}

		if isLastInWeek {
			weeklySamples = append(weeklySamples, point.TotalAge)
			weeklyDates = append(weeklyDates, point.Date)
		}
	}

	// 2. Calculate XmR limits using only the weekly samples
	var keys []string
	for _, dt := range weeklyDates {
		keys = append(keys, window.GenerateLabel(dt))
	}
	xmr := CalculateXmRWithKeys(weeklySamples, keys)

	// 3. Sweep daily values against weekly-derived limits for daily signals
	var dailySignals []Signal
	for i, point := range runChart {
		key := point.Date.Format("2006-01-02")
		val := point.TotalAge

		if val > xmr.UNPL {
			dailySignals = append(dailySignals, Signal{
				Index:       i,
				Key:         key,
				Type:        "outlier",
				Description: "Total WIP Age above Upper Natural Process Limit (UNPL)",
			})
		} else if val < xmr.LNPL {
			dailySignals = append(dailySignals, Signal{
				Index:       i,
				Key:         key,
				Type:        "outlier",
				Description: "Total WIP Age below Lower Natural Process Limit (LNPL)",
			})
		}
	}

	xmr.Signals = dailySignals

	status := "stable"
	if len(dailySignals) > 0 {
		status = "unstable"
	}

	return WIPAgeStabilityResult{
		RunChart: runChart,
		XmR:      xmr,
		Status:   status,
	}
}
