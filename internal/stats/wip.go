package stats

import (
	"math"
	"time"

	"mcs-mcp/internal/jira"
)

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

// CalculateWIPRunChart traces the history of all issues to find active WIP on each day.
// It counts an item as WIP for ANY portion of a day where it resided in one of the activeWipStatuses.
// If an item backflows out of the active statuses, it ceases to be WIP for those days.
func CalculateWIPRunChart(issues []jira.Issue, window AnalysisWindow, commitmentPoint string, weights map[string]int, mappings map[string]StatusMetadata) []WIPRunChartPoint {
	// 1. Build a timeline of all days in the window
	var days []time.Time
	current := SnapToStart(window.Start, "day")
	endBucket := SnapToStart(window.End, "day")

	for !current.After(endBucket) {
		days = append(days, current)
		current = current.AddDate(0, 0, 1)
	}

	// Fast mapping lookup for efficiency
	type interval struct {
		start int64 // Start of day timestamp
		end   int64 // End of day timestamp
	}

	// Create bounds for the days to make checking easier
	var bounds []interval
	for _, d := range days {
		bounds = append(bounds, interval{
			start: d.UnixMicro(),
			end:   SnapToEnd(d, "day").UnixMicro(),
		})
	}

	// 2. Determine active intervals [EnterWIP, ExitWIP] for every issue
	type activeRange struct {
		enterTS int64
		exitTS  int64
	}

	var allActiveRanges [][]activeRange

	commitmentWeight := weights[commitmentPoint]
	if commitmentWeight == 0 {
		commitmentWeight = 999 // safe fallback
	}

	evaluateWipState := func(statusID, status string, currentIsWip bool) bool {
		m, hasMeta := mappings[statusID]

		if hasMeta {
			if m.Tier == "Finished" {
				return false
			}
			if m.Tier == "Demand" {
				return false
			}
		}

		w, hasWeight := weights[status]
		if hasWeight {
			return w >= commitmentWeight
		}

		// Fallback for branching/parallel statuses not in the explicit backbone
		if hasMeta {
			if m.Tier == "Downstream" {
				return true
			}
		}

		// Unknown mapped status, or entirely unmapped. Maintain current WIP state.
		return currentIsWip
	}

	for _, issue := range issues {
		var currentEnter int64 = 0
		var issueRanges []activeRange

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
				issueRanges = append(issueRanges, activeRange{
					enterTS: currentEnter,
					exitTS:  tr.Date.UnixMicro(),
				})
				currentEnter = 0
			}
			isWip = newIsWip
		}

		// Handle items still in Downstream at the end of their recorded timeline
		if currentEnter > 0 {
			var exitTS int64 = math.MaxInt64

			// If it has a resolution date but is still somehow registered as downstream, cap it there
			if issue.ResolutionDate != nil {
				exitTS = issue.ResolutionDate.UnixMicro()
			}

			issueRanges = append(issueRanges, activeRange{
				enterTS: currentEnter,
				exitTS:  exitTS,
			})
		}

		if len(issueRanges) > 0 {
			allActiveRanges = append(allActiveRanges, issueRanges)
		}
	}

	// 3. Count WIP for each day
	chart := make([]WIPRunChartPoint, len(days))

	for i, d := range days {
		b := bounds[i]
		count := 0

		for _, issueRanges := range allActiveRanges {
			// Did this issue occupy ANY part of active WIP during this day?
			isActiveToday := false
			for _, r := range issueRanges {
				// End-Of-Day Snapshot logic with Kanban Same-Day exception
				startsBeforeEnd := r.enterTS <= b.end
				survivesUntilEnd := r.exitTS > b.end
				startedToday := r.enterTS >= b.start

				if startsBeforeEnd {
					// 1. Snapshot: Was it active precisely at midnight (23:59:59) of this bucket?
					if survivesUntilEnd {
						isActiveToday = true
						break
					}
					// 2. Exception: Day 1 Rule - Did it start AND finish completely within this day?
					if startedToday {
						isActiveToday = true
						break
					}
				}
			}

			if isActiveToday {
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
