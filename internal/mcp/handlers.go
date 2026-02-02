package mcp

import (
	"fmt"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

func (s *Server) handleGetDiagnosticRoadmap(goal string) (interface{}, error) {
	roadmaps := map[string]interface{}{
		"forecasting": map[string]interface{}{
			"title":       "Analytical Workflow: Professional Forecasting",
			"description": "Recommended sequence to produce reliable delivery dates or volume forecasts.",
			"steps": []interface{}{
				map[string]interface{}{"step": 1, "tool": "get_workflow_discovery", "description": "Verify the semantic workflow mapping (tiers and roles) and data shape."},
				map[string]interface{}{"step": 2, "tool": "get_process_stability", "description": "Verify that the process is predictable (Stable XmR)."},
				map[string]interface{}{"step": 3, "tool": "get_cycle_time_assessment", "description": "Understand baseline SLE (Service Level Expectations) for different work items."},
				map[string]interface{}{"step": 4, "tool": "get_aging_analysis", "description": "Check if current WIP is clogging the system."},
				map[string]interface{}{"step": 5, "tool": "run_simulation", "description": "Perform Monte-Carlo simulation using the historical baseline."},
			},
		},
		"bottlenecks": map[string]interface{}{
			"title":       "Analytical Workflow: Bottleneck & Flow Analysis",
			"description": "Recommended sequence to identify systemic delays and batching behavior.",
			"steps": []interface{}{
				map[string]interface{}{"step": 1, "tool": "get_workflow_discovery", "description": "Map the workflow tiers to differentiate between analysis, execution, and terminal states."},
				map[string]interface{}{"step": 2, "tool": "get_status_persistence", "description": "Find where items spend the most time and identify 'High Variance' statuses."},
				map[string]interface{}{"step": 3, "tool": "get_delivery_cadence", "description": "Analyze throughput pulse to detect batching (uneven delivery) vs. steady flow."},
				map[string]interface{}{"step": 4, "tool": "get_process_yield", "description": "Check for high abandonment rates between tiers."},
			},
		},
		"capacity_planning": map[string]interface{}{
			"title":       "Analytical Workflow: Capacity & Volume Planning",
			"description": "Recommended sequence to determine if the team can take on more scope.",
			"steps": []interface{}{
				map[string]interface{}{"step": 1, "tool": "get_delivery_cadence", "description": "Determine the current weekly throughput baseline."},
				map[string]interface{}{"step": 2, "tool": "get_process_stability", "description": "Compare current WIP against historical capacity (Stability Index)."},
				map[string]interface{}{"step": 3, "tool": "run_simulation", "description": "Use 'scope' mode to see how much we can reasonably finish in the next period."},
			},
		},
		"system_health": map[string]interface{}{
			"title":       "Analytical Workflow: Strategic System Health",
			"description": "Recommended sequence for long-term process oversight and strategic shift detection.",
			"steps": []interface{}{
				map[string]interface{}{"step": 1, "tool": "get_process_evolution", "description": "Perform a longitudinal audit (Three-Way Control Charts)."},
				map[string]interface{}{"step": 2, "tool": "get_process_yield", "description": "Evaluate long-term conversion efficiency across the entire pipe."},
			},
		},
	}

	res, ok := roadmaps[goal]
	if !ok {
		return nil, fmt.Errorf("unknown goal: %s. Available goals: forecasting, bottlenecks, capacity_planning, system_health", goal)
	}

	return res, nil
}

// Internal shared logic

func (s *Server) getStatusWeights(issues []jira.Issue) map[string]int {
	// Discover the backbone path order and return indexed weights
	order := stats.DiscoverStatusOrder(issues)
	weights := make(map[string]int)
	for i, name := range order {
		weights[name] = i + 1
	}
	return weights
}

// Note: ApplyBackflowPolicy and RecalculateResidency have been moved to internal/stats/processor.go

func (s *Server) getInferredRange(projectKey string, boardID int, startStatus, endStatus string, issues []jira.Issue) []string {
	sourceID := getCombinedID(projectKey, boardID)
	if order, ok := s.statusOrderings[sourceID]; ok {
		return s.sliceRange(order, startStatus, endStatus)
	}

	allStatuses := stats.DiscoverStatusOrder(issues)
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

func (s *Server) getCommitmentPointHints(issues []jira.Issue, weights map[string]int) []string {
	hints := make(map[string]bool)
	for _, issue := range issues {
		for _, t := range issue.Transitions {
			if weights[t.ToStatus] >= 2 && weights[t.FromStatus] < 2 {
				hints[t.ToStatus] = true
			}
		}
	}
	res := make([]string, 0, len(hints))
	for h := range hints {
		res = append(res, h)
	}
	return res
}

func (s *Server) getEarliestCommitment(projectKey string, boardID int, issues []jira.Issue) (string, bool) {
	sourceID := getCombinedID(projectKey, boardID)
	mapping := s.workflowMappings[sourceID]
	if mapping == nil {
		return "", false
	}

	order := s.statusOrderings[sourceID]
	if len(order) == 0 {
		order = stats.DiscoverStatusOrder(issues)
	}

	for _, status := range order {
		// status might be a name from DiscoverStatusOrder, but GetMetadataRobust handles that
		if m, ok := stats.GetMetadataRobust(mapping, "", status); ok && m.Tier == "Downstream" {
			return status, true
		}
	}
	return "", false
}

func (s *Server) getDeliveredResolutions(projectKey string, boardID int) []string {
	sourceID := getCombinedID(projectKey, boardID)
	rm := s.getResolutionMap(sourceID)
	delivered := make([]string, 0)
	for name, outcome := range rm {
		if outcome == "delivered" {
			delivered = append(delivered, name)
		}
	}
	return delivered
}

func (s *Server) getCycleTimes(projectKey string, boardID int, issues []jira.Issue, startStatus, endStatus string, resolutions []string) []float64 {
	// Reusing logic from mcp package
	return s.calculateCycleTimesList(projectKey, boardID, issues, startStatus, endStatus, resolutions)
}

func (s *Server) calculateCycleTimesList(projectKey string, boardID int, issues []jira.Issue, startStatus, endStatus string, resolutions []string) []float64 {
	sourceID := getCombinedID(projectKey, boardID)
	mappings := s.workflowMappings[sourceID]
	resMap := make(map[string]bool)
	for _, r := range resolutions {
		resMap[r] = true
	}

	rangeStatuses := s.getInferredRange(projectKey, boardID, startStatus, endStatus, issues)

	var cycleTimes []float64
	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}
		if len(resolutions) > 0 && !resMap[issue.Resolution] {
			if m, ok := stats.GetMetadataRobust(mappings, issue.StatusID, issue.Status); !ok || m.Outcome != "delivered" {
				continue
			}
		} else if len(resolutions) == 0 {
			if m, ok := stats.GetMetadataRobust(mappings, issue.StatusID, issue.Status); !ok || m.Outcome != "delivered" {
				continue
			}
		}

		duration := stats.SumRangeDuration(issue, rangeStatuses)
		if duration > 0 {
			cycleTimes = append(cycleTimes, duration)
		}
	}

	return cycleTimes
}
