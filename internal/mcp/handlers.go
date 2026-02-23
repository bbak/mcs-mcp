package mcp

import (
	"fmt"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/stats/discovery"
)

func (s *Server) handleGetDiagnosticRoadmap(goal string) (interface{}, error) {
	roadmaps := map[string]interface{}{
		"forecasting": map[string]interface{}{
			"title":       "Analytical Workflow: Professional Forecasting",
			"description": "Recommended sequence to produce reliable delivery dates or volume forecasts.",
			"steps": []interface{}{
				map[string]interface{}{"step": 1, "tool": "workflow_discover_mapping", "description": "Verify the semantic workflow mapping (tiers and roles) and data shape."},
				map[string]interface{}{"step": 2, "tool": "analyze_process_stability", "description": "Verify that the process is predictable (Stable XmR)."},
				map[string]interface{}{"step": 3, "tool": "analyze_cycle_time", "description": "Understand baseline SLE (Service Level Expectations) for different work items."},
				map[string]interface{}{"step": 4, "tool": "analyze_work_item_age", "description": "Check if current WIP is clogging the system."},
				map[string]interface{}{"step": 5, "tool": "forecast_monte_carlo", "description": "Perform Monte-Carlo simulation using the historical baseline."},
				map[string]interface{}{"step": 6, "tool": "forecast_backtest", "description": "Perform a 'Walk-Forward Analysis' to validate the reliability of the forecast model."},
			},
		},
		"bottlenecks": map[string]interface{}{
			"title":       "Analytical Workflow: Bottleneck & Flow Analysis",
			"description": "Recommended sequence to identify systemic delays and batching behavior.",
			"steps": []interface{}{
				map[string]interface{}{"step": 1, "tool": "workflow_discover_mapping", "description": "Map the workflow tiers to differentiate between analysis, execution, and terminal states."},
				map[string]interface{}{"step": 2, "tool": "analyze_status_persistence", "description": "Find where items spend the most time and identify 'High Variance' statuses."},
				map[string]interface{}{"step": 3, "tool": "analyze_throughput", "description": "Analyze throughput pulse to detect batching (uneven delivery) vs. steady flow."},
				map[string]interface{}{"step": 4, "tool": "analyze_yield", "description": "Check for high abandonment rates between tiers."},
				map[string]interface{}{"step": 5, "tool": "analyze_item_journey", "description": "Drill down into specific 'Long Tail' outlier items to see exact path delays."},
			},
		},
		"capacity_planning": map[string]interface{}{
			"title":       "Analytical Workflow: Capacity & Volume Planning",
			"description": "Recommended sequence to determine if the team can take on more scope.",
			"steps": []interface{}{
				map[string]interface{}{"step": 1, "tool": "analyze_throughput", "description": "Determine the current weekly throughput baseline."},
				map[string]interface{}{"step": 2, "tool": "analyze_process_stability", "description": "Compare current WIP against historical capacity (Stability Index)."},
				map[string]interface{}{"step": 3, "tool": "forecast_monte_carlo", "description": "Use 'scope' mode to see how much we can reasonably finish in the next period."},
			},
		},
		"system_health": map[string]interface{}{
			"title":       "Analytical Workflow: Strategic System Health",
			"description": "Recommended sequence for long-term process oversight and strategic shift detection.",
			"steps": []interface{}{
				map[string]interface{}{"step": 1, "tool": "analyze_process_evolution", "description": "Perform a longitudinal audit (Three-Way Control Charts)."},
				map[string]interface{}{"step": 2, "tool": "analyze_yield", "description": "Evaluate long-term conversion efficiency across the entire pipe."},
			},
		},
	}

	res, ok := roadmaps[goal]
	if !ok {
		return nil, fmt.Errorf("unknown goal: %s. Available goals: forecasting, bottlenecks, capacity_planning, system_health", goal)
	}

	return WrapResponse(res, "", 0, nil, nil, nil), nil
}

// Internal shared logic

func (s *Server) getStatusWeights(issues []jira.Issue) map[string]int {
	// Discover the backbone path order and return indexed weights
	order := discovery.DiscoverStatusOrder(issues)
	weights := make(map[string]int)
	for i, name := range order {
		weights[name] = i + 1
	}
	return weights
}

// Note: ApplyBackflowPolicy and RecalculateResidency have been moved to internal/stats/processor.go

func (s *Server) getInferredRange(projectKey string, boardID int, startStatus, endStatus string, issues []jira.Issue) []string {
	if s.activeSourceID == getCombinedID(projectKey, boardID) && len(s.activeStatusOrder) > 0 {
		return s.sliceRange(s.activeStatusOrder, startStatus, endStatus)
	}

	allStatuses := discovery.DiscoverStatusOrder(issues)
	if len(allStatuses) == 0 {
		return []string{}
	}

	return s.sliceRange(allStatuses, startStatus, endStatus)
}

func (s *Server) sliceRange(order []string, start, end string) []string {
	if len(order) == 0 {
		return []string{}
	}
	startIndex := 0
	if start != "" {
		for i, st := range order {
			if st == start {
				startIndex = i
				break
			}
		}
	}

	endIndex := len(order) - 1
	if end != "" {
		for i, st := range order {
			if st == end {
				endIndex = i
				break
			}
		}
	}

	if startIndex > endIndex {
		return []string{order[startIndex]}
	}

	return order[startIndex : endIndex+1]
}

func (s *Server) getEarliestCommitment(projectKey string, boardID int, issues []jira.Issue) (string, bool) {
	if s.activeSourceID != getCombinedID(projectKey, boardID) || s.activeMapping == nil {
		return "", false
	}

	order := s.activeStatusOrder
	if len(order) == 0 {
		order = discovery.DiscoverStatusOrder(issues)
	}

	for _, status := range order {
		if m, ok := stats.GetMetadataRobust(s.activeMapping, "", status); ok && m.Tier == "Downstream" {
			return status, true
		}
	}
	return "", false
}

func (s *Server) getCycleTimes(projectKey string, boardID int, issues []jira.Issue, startStatus, endStatus string, issueTypes []string) ([]float64, []jira.Issue) {
	typeMap := make(map[string]bool)
	for _, t := range issueTypes {
		typeMap[t] = true
	}

	rangeStatuses := s.getInferredRange(projectKey, boardID, startStatus, endStatus, issues)

	var cycleTimes []float64
	var matchedIssues []jira.Issue

	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}
		if s.activeDiscoveryCutoff != nil && issue.ResolutionDate.Before(*s.activeDiscoveryCutoff) {
			continue
		}
		if len(issueTypes) > 0 && !typeMap[issue.IssueType] {
			continue
		}

		// Only count "delivered" work
		if m, ok := stats.GetMetadataRobust(s.activeMapping, issue.StatusID, issue.Status); !ok || m.Outcome != "delivered" {
			continue
		}

		duration := stats.SumRangeDuration(issue, rangeStatuses)
		if duration > 0 {
			cycleTimes = append(cycleTimes, duration)
			matchedIssues = append(matchedIssues, issue)
		}
	}

	return cycleTimes, matchedIssues
}

func (s *Server) getCycleTimesByType(projectKey string, boardID int, issues []jira.Issue, startStatus, endStatus string, issueTypes []string) map[string][]float64 {
	typeMap := make(map[string]bool)
	for _, t := range issueTypes {
		typeMap[t] = true
	}

	rangeStatuses := s.getInferredRange(projectKey, boardID, startStatus, endStatus, issues)

	cycleTimes := make(map[string][]float64)
	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}
		if s.activeDiscoveryCutoff != nil && issue.ResolutionDate.Before(*s.activeDiscoveryCutoff) {
			continue
		}
		if len(issueTypes) > 0 && !typeMap[issue.IssueType] {
			continue
		}

		// Only count "delivered" work
		if m, ok := stats.GetMetadataRobust(s.activeMapping, issue.StatusID, issue.Status); !ok || m.Outcome != "delivered" {
			continue
		}

		duration := stats.SumRangeDuration(issue, rangeStatuses)
		if duration > 0 {
			t := issue.IssueType
			if t == "" {
				t = "Unknown"
			}
			cycleTimes[t] = append(cycleTimes[t], duration)
		}
	}

	return cycleTimes
}
func (s *Server) getQualityWarnings(issues []jira.Issue) []string {
	var warnings []string
	syntheticCount := 0
	var active []jira.Issue

	for _, issue := range issues {
		if issue.HasSyntheticBirth {
			syntheticCount++
		}
		if issue.ResolutionDate == nil {
			active = append(active, issue)
		}
	}

	if syntheticCount > 0 {
		warnings = append(warnings, fmt.Sprintf("DATA INTEGRITY WARNING: %d item(s) are missing their creation events. Cycle Times and Stability metrics for these items are based on the earliest recorded event, which likely understates their true age.", syntheticCount))
	}

	// System Pressure Check (Stability Guardrail)
	if len(active) > 0 {
		pressure := stats.CalculateSystemPressure(active)
		if pressure.PressureRatio >= 0.25 {
			warnings = append(warnings, fmt.Sprintf("SYSTEM PRESSURE WARNING: %.0f%% of your current WIP is currently flagged as blocked. This high level of impediment makes historical throughput a potentially over-optimistic proxy for the future.", pressure.PressureRatio*100))
		}
	}

	return warnings
}
