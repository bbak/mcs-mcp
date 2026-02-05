package mcp

import (
	"fmt"
	"math" // Added for sorting operations
	"time"

	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

func (s *Server) handleGetStatusPersistence(projectKey string, boardID int) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events)

	if len(issues) == 0 {
		return nil, fmt.Errorf("no historical data found to analyze status persistence")
	}

	persistence := stats.CalculateStatusPersistence(issues)

	return map[string]interface{}{
		"persistence": persistence,
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

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events)
	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, issues)

	cWeight := 2
	if analysisCtx.CommitmentPoint != "" {
		if w, ok := analysisCtx.StatusWeights[analysisCtx.CommitmentPoint]; ok {
			cWeight = w
		}
	}
	wipIssues := s.filterWIPIssues(issues, analysisCtx.CommitmentPoint, analysisCtx.FinishedStatuses)
	wipIssues = stats.ApplyBackflowPolicy(wipIssues, analysisCtx.StatusWeights, cWeight)

	cycleTimes := s.getCycleTimes(projectKey, boardID, issues, analysisCtx.CommitmentPoint, "", nil)

	aging := stats.CalculateInventoryAge(wipIssues, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes, agingType)

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
		"aging": aging,
		"_guidance": []string{
			"Items in 'Demand' or 'Finished' tiers are usually excluded from WIP Age unless explicitly requested.",
			"PercentileRelative helps identify which individual items are 'neglect' risks compared to historical performance.",
		},
	}, nil
}

func (s *Server) handleGetDeliveryCadence(projectKey string, boardID int, windowWeeks int, includeAbandoned bool) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events)

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, issues)

	if windowWeeks <= 0 {
		windowWeeks = 26
	}

	var cutoff time.Time
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	daily := stats.GetDailyThroughput(issues, windowWeeks*7, analysisCtx.WorkflowMappings, s.activeResolutions, !includeAbandoned, cutoff)
	weekly := aggregateToWeeks(daily)

	// Build week metadata
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	windowDays := windowWeeks * 7
	minDate := today.AddDate(0, 0, -windowDays+1)

	weekMetadata := make([]map[string]string, 0)
	for i := 0; i < len(weekly); i++ {
		weekStart := minDate.AddDate(0, 0, i*7)
		weekEnd := weekStart.AddDate(0, 0, 6)
		weekMetadata = append(weekMetadata, map[string]string{
			"week_index": fmt.Sprintf("%d", i+1),
			"start_date": weekStart.Format("2006-01-02"),
			"end_date":   weekEnd.Format("2006-01-02"),
		})
	}

	return map[string]interface{}{
		"weekly_throughput": weekly,
		"@week_metadata":    weekMetadata,
		"_guidance": []string{
			"Look for 'Batching' (bursts of delivery followed by silence) vs. 'Steady Flow'.",
		},
	}, nil
}

func (s *Server) handleGetProcessStability(projectKey string, boardID int) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events)

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, issues)
	// Stability usually looks at a 26-week window by default
	windowWeeks := 26
	var cutoff time.Time
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	issues = stats.FilterIssuesByResolutionWindow(issues, windowWeeks*7, cutoff)

	cycleTimes := s.getCycleTimes(projectKey, boardID, issues, analysisCtx.CommitmentPoint, "", nil)

	wipIssues := s.filterWIPIssues(issues, analysisCtx.CommitmentPoint, analysisCtx.FinishedStatuses)
	stability := stats.CalculateProcessStability(issues, cycleTimes, len(wipIssues))

	return map[string]interface{}{
		"stability": stability,
		"_guidance": []string{
			"XmR charts detect 'Special Cause' variation. If stability is low, forecasts are unreliable.",
			"Stability Index (WIP/Throughput) > 1.3 indicates a 'Clogged' system.",
		},
	}, nil
}

func (s *Server) handleGetProcessEvolution(projectKey string, boardID int, windowMonths int) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events)

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, issues)

	// Enforce window
	if windowMonths <= 0 {
		windowMonths = 12 // Default
	}
	var cutoff time.Time
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	issues = stats.FilterIssuesByResolutionWindow(issues, windowMonths*30, cutoff)

	cycleTimes := s.getCycleTimes(projectKey, boardID, issues, analysisCtx.CommitmentPoint, "", nil)

	subgroups := stats.GroupIssuesByWeek(issues, cycleTimes)
	evolution := stats.CalculateThreeWayXmR(subgroups)

	return map[string]interface{}{
		"evolution": evolution,
		"context": map[string]interface{}{
			"window_months":  windowMonths,
			"total_issues":   len(issues),
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

	// Ensure we are anchored before analysis
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events)

	yield := stats.CalculateProcessYield(issues, s.activeMapping, s.getResolutionMap(sourceID))

	return map[string]interface{}{
		"yield": yield,
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

	// Ensure we are anchored before analysis
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

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
		return nil, fmt.Errorf("issue %s not found on the current Project (%s) and Board (%d). Other issues cannot be interpreted correctly because the specific workflow context is unknown", issueKey, projectKey, boardID)
	}

	finished := s.getFinishedStatuses(nil, events)
	issue := eventlog.ReconstructIssue(events, finished, time.Now())
	residency := stats.CalculateResidency(issue.Transitions, issue.Created, issue.ResolutionDate, issue.Status, finished, "", time.Now())

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
		"summary":        issue.Summary,
		"residency":      residencyDays,
		"path":           steps,
		"tier_breakdown": tierBreakdown,
		"warnings":       []string{},
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
