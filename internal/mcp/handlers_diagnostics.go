package mcp

import (
	"fmt"
	"math"
	"time"

	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/stats"
)

func (s *Server) handleGetStatusPersistence(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Stage 3: Baseline Completion
	if err := s.events.EnsureBaseline(sourceID, ctx.JQL, 6); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events, sourceID)

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

func (s *Server) handleGetAgingAnalysis(sourceID, sourceType, agingType, tierFilter string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Stage 3 & 2 Completion
	if err := s.events.EnsureBaseline(sourceID, ctx.JQL, 6); err != nil {
		return nil, err
	}
	active := s.getActiveStatuses(sourceID)
	if err := s.events.EnsureWIP(sourceID, ctx.JQL, active); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events, sourceID)
	analysisCtx := s.prepareAnalysisContext(sourceID, issues)

	cWeight := 2
	if analysisCtx.CommitmentPoint != "" {
		if w, ok := analysisCtx.StatusWeights[analysisCtx.CommitmentPoint]; ok {
			cWeight = w
		}
	}
	wipIssues := s.filterWIPIssues(issues, analysisCtx.CommitmentPoint, analysisCtx.FinishedStatuses)
	wipIssues = s.applyBackflowPolicy(wipIssues, analysisCtx.StatusWeights, cWeight)

	deliveredResolutions := s.getDeliveredResolutions(sourceID)
	cycleTimes := s.getCycleTimes(sourceID, issues, analysisCtx.CommitmentPoint, "", deliveredResolutions)

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

func (s *Server) handleGetDeliveryCadence(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Stage 3: Baseline Completion
	if err := s.events.EnsureBaseline(sourceID, ctx.JQL, 6); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events, sourceID)

	daily := stats.GetDailyThroughput(issues)
	weekly := aggregateToWeeks(daily)

	return map[string]interface{}{
		"weekly_throughput": weekly,
		"_guidance": []string{
			"Look for 'Batching' (bursts of delivery followed by silence) vs. 'Steady Flow'.",
		},
	}, nil
}

func (s *Server) handleGetProcessStability(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Stage 3 & 2 Completion
	if err := s.events.EnsureBaseline(sourceID, ctx.JQL, 6); err != nil {
		return nil, err
	}
	active := s.getActiveStatuses(sourceID)
	if err := s.events.EnsureWIP(sourceID, ctx.JQL, active); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events, sourceID)

	analysisCtx := s.prepareAnalysisContext(sourceID, issues)
	deliveredResolutions := s.getDeliveredResolutions(sourceID)
	cycleTimes := s.getCycleTimes(sourceID, issues, analysisCtx.CommitmentPoint, "", deliveredResolutions)

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

func (s *Server) handleGetProcessEvolution(sourceID, sourceType string, windowMonths int) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Stage 3 Completion
	if err := s.events.EnsureBaseline(sourceID, ctx.JQL, windowMonths); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events, sourceID)

	analysisCtx := s.prepareAnalysisContext(sourceID, issues)
	deliveredResolutions := s.getDeliveredResolutions(sourceID)

	issues = s.applyBackflowPolicy(issues, analysisCtx.StatusWeights, 2)
	cycleTimes := s.getCycleTimes(sourceID, issues, analysisCtx.CommitmentPoint, "", deliveredResolutions)

	subgroups := stats.GroupIssuesByMonth(issues, cycleTimes)
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

func (s *Server) handleGetProcessYield(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Stage 3: Baseline Completion
	if err := s.events.EnsureBaseline(sourceID, ctx.JQL, 6); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events, sourceID)

	yield := stats.CalculateProcessYield(issues, s.workflowMappings[sourceID], s.getResolutionMap(sourceID))

	return map[string]interface{}{
		"yield": yield,
		"_guidance": []string{
			"High 'Abandoned Upstream' often points to discovery/refinement issues.",
			"High 'Abandoned Downstream' points to execution or commitment issues.",
		},
	}, nil
}

func (s *Server) handleGetItemJourney(issueKey string) (interface{}, error) {
	// SINGLE ITEM probe
	jql := fmt.Sprintf("key = %s", issueKey)
	if err := s.events.EnsureProbe(issueKey, jql); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(issueKey, time.Time{}, time.Now())
	if len(events) == 0 {
		return nil, fmt.Errorf("issue not found in event log: %s", issueKey)
	}

	var sourceID string
	var mapping map[string]stats.StatusMetadata
	for id, m := range s.workflowMappings {
		sourceID = id
		mapping = m
		break
	}

	finished := s.getFinishedStatuses(sourceID)
	issue := eventlog.ReconstructIssue(events, finished)

	type JourneyStep struct {
		Status string  `json:"status"`
		Days   float64 `json:"days"`
	}
	var steps []JourneyStep

	if len(issue.Transitions) > 0 {
		birthStatus := issue.Transitions[0].FromStatus
		if birthStatus == "" {
			// Fallback to the status it moved into if From is empty (shouldn't happen with new transformer but good to have)
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

	residencyDays := make(map[string]float64)
	for s, sec := range issue.StatusResidency {
		residencyDays[s] = math.Round((float64(sec)/86400.0)*10) / 10
	}

	tierBreakdown := make(map[string]map[string]interface{})
	for status, sec := range issue.StatusResidency {
		tier := "Unknown"
		if mapping != nil {
			if m, ok := mapping[status]; ok {
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
