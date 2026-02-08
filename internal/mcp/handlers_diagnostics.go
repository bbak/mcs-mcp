package mcp

import (
	"fmt"
	"math" // Added for sorting operations
	"time"

	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

func (s *Server) handleGetStatusPersistence(projectKey string, boardID int) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// 2. Project on Demand (6-month historical window for persistence)
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Now().AddDate(0, 0, -180), time.Now(), "day", cutoff)
	events := s.events.GetEventsInRange(sourceID, window.Start, window.End)
	finished, downstream, upstream, demand := eventlog.ProjectScope(events, window, s.activeCommitmentPoint, s.activeMapping, s.activeResolutions, nil)

	// Context for analysis includes historical completions, current WIP, and backlog for residency analysis
	combined := append(finished, append(downstream, append(upstream, demand...)...)...)

	if len(combined) == 0 {
		return nil, fmt.Errorf("no historical data found to analyze status persistence")
	}

	persistence := stats.CalculateStatusPersistence(combined)

	return map[string]interface{}{
		"persistence":   persistence,
		"_data_quality": s.getQualityWarnings(combined),
		"_guidance": []string{
			"This tool uses a robust 6-MONTH historical window, making it the primary source for performance and residency analysis.",
			"Persistence stats (coin_toss, likely, etc.) measure INTERNAL residency time WITHIN one status. They ARE NOT end-to-end completion forecasts.",
			"Inner80 and IQR help distinguish between 'Stable Flow' and 'High Variance' bottlenecks.",
			"Statuses with zero residency for many items might be bypassed in the actual process.",
		},
	}, nil
}

func (s *Server) handleGetAgingAnalysis(projectKey string, boardID int, agingType, tierFilter string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// 2. Project on Demand
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Time{}, time.Now(), "day", cutoff)
	events := s.events.GetEventsInRange(sourceID, window.Start, window.End)
	// handleGetAgingAnalysis doesn't currently take issueTypes as input, keeping as nil for now
	finished, downstream, upstream, demand := eventlog.ProjectScope(events, window, s.activeCommitmentPoint, s.activeMapping, s.activeResolutions, nil)

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, append(finished, append(downstream, append(upstream, demand...)...)...))

	// Using all started issues (Downstream + Upstream) for Nave-aligned aging
	activeIssues := append(downstream, upstream...)

	// Cycle times from history
	cycleTimes := s.getCycleTimes(projectKey, boardID, finished, analysisCtx.CommitmentPoint, "", nil)

	aging := stats.CalculateInventoryAge(activeIssues, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes, agingType)

	// Apply tier filter if requested
	if tierFilter != "All" && tierFilter != "" {
		filtered := make([]stats.InventoryAge, 0)
		for _, a := range aging {
			if tierFilter == "WIP" {
				if a.Tier != "Demand" && a.Tier != "Finished" {
					filtered = append(filtered, a)
				}
			} else if a.Tier == tierFilter {
				filtered = append(filtered, a)
			}
		}
		aging = filtered
	}

	return map[string]interface{}{
		"aging":         aging,
		"_data_quality": s.getQualityWarnings(append(finished, append(downstream, append(upstream, demand...)...)...)),
		"_guidance": []string{
			"Items in 'Demand' or 'Finished' tiers are usually excluded from WIP Age unless explicitly requested.",
			"PercentileRelative helps identify which individual items are 'neglect' risks compared to historical performance.",
			"AgeSinceCommitment reflects time since the LAST commitment (resets on backflow to Demand/Upstream).",
			"Check 'cumulative_wip_days' or the Item Journey if you suspect Nave-alignment discrepancies due to backflows.",
		},
	}, nil
}

func (s *Server) handleGetDeliveryCadence(projectKey string, boardID int, windowWeeks int, includeAbandoned bool) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	if windowWeeks <= 0 {
		windowWeeks = 26
	}

	// 2. Project on Demand
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	// Delivery Cadence uses a window of N weeks, snapped to week boundaries
	window := stats.NewAnalysisWindow(time.Now().AddDate(0, 0, -windowWeeks*7), time.Now(), "week", cutoff)
	events := s.events.GetEventsInRange(sourceID, window.Start, window.End)
	finished, _, _, _ := eventlog.ProjectScope(events, window, s.activeCommitmentPoint, s.activeMapping, s.activeResolutions, nil)

	var delivered []jira.Issue
	if includeAbandoned {
		delivered = finished
	} else {
		delivered = stats.FilterDelivered(finished, s.activeResolutions, s.activeMapping)
	}
	daily := stats.GetDailyThroughput(delivered, window, s.activeResolutions, s.activeMapping)
	weekly := aggregateToWeeks(daily)

	// Build week metadata using the window's subdivision
	weekMetadata := make([]map[string]string, 0)
	buckets := window.Subdivide()
	for i, bucketStart := range buckets {
		if i >= len(weekly) {
			break
		}
		bucketEnd := stats.SnapToEnd(bucketStart, "week")
		weekMetadata = append(weekMetadata, map[string]string{
			"week_index": fmt.Sprintf("%d", i+1),
			"start_date": bucketStart.Format("2006-01-02"),
			"end_date":   bucketEnd.Format("2006-01-02"),
			"label":      window.GenerateLabel(bucketStart),
			"is_partial": fmt.Sprintf("%v", window.IsPartial(bucketStart)),
		})
	}

	return map[string]interface{}{
		"weekly_throughput": weekly,
		"@week_metadata":    weekMetadata,
		"_data_quality":     s.getQualityWarnings(delivered),
		"_guidance": []string{
			"Look for 'Batching' (bursts of delivery followed by silence) vs. 'Steady Flow'.",
			fmt.Sprintf("The current window uses a %d-week historical baseline anchored at %s.", windowWeeks, window.Start.Format("2006-01-02")),
		},
	}, nil
}

func (s *Server) handleGetProcessStability(projectKey string, boardID int) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// 2. Project on Demand
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	// Stability baseline uses 26 weeks by default
	window := stats.NewAnalysisWindow(time.Now().AddDate(0, 0, -26*7), time.Now(), "week", cutoff)
	// Fetch all events up to window end for accurate WIP count
	events := s.events.GetEventsInRange(sourceID, time.Time{}, window.End)
	// handleGetProcessStability doesn't currently take issue_types as input in handleGetProcessStability call itself,
	// but it should probably support it. For now, nil.
	finishedAll, downstream, upstream, demand := eventlog.ProjectScope(events, window, s.activeCommitmentPoint, s.activeMapping, s.activeResolutions, nil)
	delivered := stats.FilterDelivered(finishedAll, s.activeResolutions, s.activeMapping)

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, append(finishedAll, append(downstream, append(upstream, demand...)...)...))

	cycleTimes := s.getCycleTimes(projectKey, boardID, delivered, analysisCtx.CommitmentPoint, "", nil)
	stability := stats.CalculateProcessStability(delivered, cycleTimes, len(downstream), float64(window.ActiveDayCount()))

	return map[string]interface{}{
		"stability":     stability,
		"_data_quality": s.getQualityWarnings(append(finishedAll, append(downstream, append(upstream, demand...)...)...)),
		"_guidance": []string{
			"XmR charts detect 'Special Cause' variation. If stability is low (outliers/shifts), forecasts are unreliable.",
			"Stability Index = (WIP / Throughput) / Average Cycle Time. A ratio > 1.3 indicates a 'Clogged' system.",
			fmt.Sprintf("Baseline calculated from %d delivered items since %s.", len(delivered), window.Start.Format("2006-01-02")),
		},
	}, nil
}

func (s *Server) handleGetProcessEvolution(projectKey string, boardID int, windowMonths int) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	if windowMonths <= 0 {
		windowMonths = 12
	}

	// 2. Project on Demand
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	// Evolution uses monthly buckets by default for strategic audit
	window := stats.NewAnalysisWindow(time.Now().AddDate(0, -windowMonths, 0), time.Now(), "month", cutoff)
	// Fetch all events up to window end for accurate context
	events := s.events.GetEventsInRange(sourceID, time.Time{}, window.End)
	finishedAll, downstream, upstream, demand := eventlog.ProjectScope(events, window, s.activeCommitmentPoint, s.activeMapping, s.activeResolutions, nil)
	delivered := stats.FilterDelivered(finishedAll, s.activeResolutions, s.activeMapping)

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, append(finishedAll, append(downstream, append(upstream, demand...)...)...))

	cycleTimes := s.getCycleTimes(projectKey, boardID, delivered, analysisCtx.CommitmentPoint, "", nil)
	subgroups := stats.GroupIssuesByBucket(delivered, cycleTimes, window)
	evolution := stats.CalculateThreeWayXmR(subgroups)

	return map[string]interface{}{
		"evolution":     evolution,
		"_data_quality": s.getQualityWarnings(delivered),
		"context": map[string]interface{}{
			"window_months":  windowMonths,
			"total_issues":   len(delivered),
			"subgroup_count": len(subgroups),
		},
	}, nil
}

func (s *Server) handleGetProcessYield(projectKey string, boardID int) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// 2. Project on Demand (Yield usually looks at full history to identify abandonment patterns)
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Time{}, time.Now(), "day", cutoff)
	events := s.events.GetEventsInRange(sourceID, window.Start, window.End)
	finished, downstream, upstream, demand := eventlog.ProjectScope(events, window, s.activeCommitmentPoint, s.activeMapping, s.activeResolutions, nil)

	combined := append(finished, append(downstream, append(upstream, demand...)...)...)
	yield := stats.CalculateProcessYield(combined, s.activeMapping, s.getResolutionMap(sourceID))

	return map[string]interface{}{
		"yield":         yield,
		"_data_quality": s.getQualityWarnings(combined),
		"_guidance": []string{
			"High 'Abandoned Upstream' often points to discovery/refinement issues.",
			"High 'Abandoned Downstream' points to execution or commitment issues.",
		},
	}, nil
}

func (s *Server) handleGetItemJourney(projectKey string, boardID int, issueKey string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Try to find in existing memory log for THIS sourceID
	events := s.events.GetEventsForIssue(sourceID, issueKey)

	// 2. Fallback to context-locked hydration if not found
	if len(events) == 0 {
		log.Info().Str("issue", issueKey).Str("source", sourceID).Msg("Issue not found in current source cache, performing context-locked hydration")
		// Context-Locked JQL: Ensure the issue belongs to the board's filter
		lockedJQL := fmt.Sprintf("(%s) AND key = %s", ctx.JQL, issueKey)
		if err := s.events.Hydrate(sourceID, lockedJQL); err != nil {
			return nil, err
		}
		events = s.events.GetEventsForIssue(sourceID, issueKey)
	} else {
		log.Info().Str("issue", issueKey).Str("source", sourceID).Msg("Issue found in existing source cache")
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("issue %s not found on the current Project (%s) and Board (%d)", issueKey, projectKey, boardID)
	}

	// Finished statuses for reconstruction
	finishedMap := make(map[string]bool)
	for status, m := range s.activeMapping {
		if m.Tier == "Finished" {
			finishedMap[status] = true
		}
	}

	issue := eventlog.ReconstructIssue(events, finishedMap, time.Now())
	residency := stats.CalculateResidency(issue.Transitions, issue.Created, issue.ResolutionDate, issue.Status, finishedMap, "", time.Now())

	type JourneyStep struct {
		Status string  `json:"status"`
		Days   float64 `json:"days"`
	}
	var steps []JourneyStep

	// Reconstruct path for display
	if len(issue.Transitions) > 0 {
		birthStatus := issue.Transitions[0].FromStatus
		if birthStatus == "" {
			birthStatus = issue.Transitions[0].ToStatus
		}

		firstDuration := issue.Transitions[0].Date.Sub(issue.Created).Seconds()
		steps = append(steps, JourneyStep{
			Status: birthStatus,
			Days:   math.Round((firstDuration/86400.0)*10) / 10,
		})

		for i := 0; i < len(issue.Transitions)-1; i++ {
			duration := issue.Transitions[i+1].Date.Sub(issue.Transitions[i].Date).Seconds()
			steps = append(steps, JourneyStep{
				Status: issue.Transitions[i].ToStatus,
				Days:   math.Round((duration/86400.0)*10) / 10,
			})
		}

		var finalDate time.Time
		if issue.ResolutionDate != nil {
			finalDate = *issue.ResolutionDate
		} else {
			finalDate = time.Now()
		}
		lastTrans := issue.Transitions[len(issue.Transitions)-1]
		finalDuration := finalDate.Sub(lastTrans.Date).Seconds()
		steps = append(steps, JourneyStep{
			Status: lastTrans.ToStatus,
			Days:   math.Round((finalDuration/86400.0)*10) / 10,
		})
	}
	issue.StatusResidency = residency

	residencyDays := make(map[string]float64)
	for s, sec := range issue.StatusResidency {
		residencyDays[s] = math.Round((float64(sec)/86400.0)*10) / 10
	}

	tierBreakdown := make(map[string]map[string]interface{})
	for status, sec := range issue.StatusResidency {
		tier := "Unknown"
		if s.activeMapping != nil {
			if m, ok := s.activeMapping[status]; ok {
				tier = m.Tier
			}
		}
		if _, ok := tierBreakdown[tier]; !ok {
			tierBreakdown[tier] = map[string]interface{}{
				"days":     0.0,
				"statuses": []string{},
			}
		}
		data := tierBreakdown[tier]
		data["days"] = data["days"].(float64) + math.Round((float64(sec)/86400.0)*10)/10
		data["statuses"] = append(data["statuses"].([]string), status)
		tierBreakdown[tier] = data
	}

	return map[string]interface{}{
		"key":            issue.Key,
		"residency":      residencyDays,
		"path":           steps,
		"tier_breakdown": tierBreakdown,
		"warnings":       []string{},
		"_data_quality":  s.getQualityWarnings([]jira.Issue{issue}),
		"_guidance": []string{
			"The 'path' shows chronological flow, while 'residency' shows cumulative totals.",
		},
	}, nil
}

func aggregateToWeeks(daily []int) []float64 {
	if len(daily) == 0 {
		return nil
	}
	weeks := make([]float64, 0)
	sum := 0
	for i, count := range daily {
		sum += count
		if (i+1)%7 == 0 {
			weeks = append(weeks, float64(sum))
			sum = 0
		}
	}
	if sum > 0 {
		weeks = append(weeks, float64(sum))
	}
	return weeks
}
