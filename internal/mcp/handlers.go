package mcp

import (
	"fmt"
	"strings"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

func (s *Server) handleGetDiagnosticRoadmap(goal string) (interface{}, error) {
	roadmaps := map[string]interface{}{
		"forecasting": map[string]interface{}{
			"title":       "Analytical Workflow: Professional Forecasting",
			"description": "Recommended sequence to produce reliable delivery dates or volume forecasts.",
			"steps": []interface{}{
				map[string]interface{}{"step": 1, "tool": "get_workflow_discovery", "description": "Establish the semantic 'Happy Path'. Verify tiers and roles with the user."},
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

func (s *Server) getStatusWeights(projectKeys []string) map[string]int {
	weights := make(map[string]int)
	// Apply common defaults for stability
	weights["To Do"] = 1
	weights["In Progress"] = 2
	weights["Done"] = 3

	// Fetch actual statuses from project to refine if possible
	categories := s.getStatusCategories(projectKeys)
	for name, cat := range categories {
		switch strings.ToLower(cat) {
		case "to-do", "new":
			weights[name] = 1
		case "indeterminate":
			weights[name] = 2
		case "done":
			weights[name] = 3
		}
	}
	return weights
}

func (s *Server) applyBackflowPolicy(issues []jira.Issue, weights map[string]int, commitmentWeight int) []jira.Issue {
	clean := make([]jira.Issue, 0, len(issues))
	for _, issue := range issues {
		lastBackflowIdx := -1
		for j, t := range issue.Transitions {
			if w, ok := weights[t.ToStatus]; ok && w < commitmentWeight {
				lastBackflowIdx = j
			}
		}

		if lastBackflowIdx == -1 {
			clean = append(clean, issue)
			continue
		}

		newIssue := issue
		newIssue.Transitions = stats.FilterTransitions(issue.Transitions, issue.Transitions[lastBackflowIdx].Date)
		newIssue.StatusResidency = s.recalculateResidency(newIssue, issue.Transitions[lastBackflowIdx].ToStatus)
		clean = append(clean, newIssue)
	}
	return clean
}

func (s *Server) recalculateResidency(issue jira.Issue, initialStatus string) map[string]int64 {
	residency := make(map[string]int64)
	if len(issue.Transitions) == 0 {
		var finalDate time.Time
		if issue.ResolutionDate != nil {
			finalDate = *issue.ResolutionDate
		} else {
			finalDate = time.Now()
		}
		duration := int64(finalDate.Sub(issue.Created).Seconds())
		if duration > 0 {
			residency[initialStatus] = duration
		}
		return residency
	}

	firstDuration := int64(issue.Transitions[0].Date.Sub(issue.Created).Seconds())
	if firstDuration > 0 {
		residency[initialStatus] = firstDuration
	}

	for i := 0; i < len(issue.Transitions)-1; i++ {
		duration := int64(issue.Transitions[i+1].Date.Sub(issue.Transitions[i].Date).Seconds())
		if duration > 0 {
			residency[issue.Transitions[i].ToStatus] += duration
		}
	}

	var finalDate time.Time
	if issue.ResolutionDate != nil {
		finalDate = *issue.ResolutionDate
	} else {
		finalDate = time.Now()
	}
	lastTrans := issue.Transitions[len(issue.Transitions)-1]
	finalDuration := int64(finalDate.Sub(lastTrans.Date).Seconds())
	if finalDuration > 0 {
		residency[lastTrans.ToStatus] += finalDuration
	}

	return residency
}

func (s *Server) getInferredRange(sourceID, startStatus, endStatus string, issues []jira.Issue) []string {
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

func (s *Server) getEarliestCommitment(sourceID string, issues []jira.Issue) (string, bool) {
	mapping := s.workflowMappings[sourceID]
	if mapping == nil {
		return "", false
	}

	order := s.statusOrderings[sourceID]
	if len(order) == 0 {
		order = stats.DiscoverStatusOrder(issues)
	}

	for _, status := range order {
		if m, ok := mapping[status]; ok && m.Tier == "Downstream" {
			return status, true
		}
	}
	return "", false
}

func (s *Server) getDeliveredResolutions(sourceID string) []string {
	rm := s.getResolutionMap(sourceID)
	delivered := make([]string, 0)
	for name, outcome := range rm {
		if outcome == "delivered" {
			delivered = append(delivered, name)
		}
	}
	return delivered
}

func (s *Server) getCycleTimes(sourceID string, issues []jira.Issue, startStatus, endStatus string, resolutions []string) []float64 {
	// Reusing logic from mcp package
	return s.calculateCycleTimesList(sourceID, issues, startStatus, endStatus, resolutions)
}

func (s *Server) calculateCycleTimesList(sourceID string, issues []jira.Issue, startStatus, endStatus string, resolutions []string) []float64 {
	mappings := s.workflowMappings[sourceID]
	resMap := make(map[string]bool)
	for _, r := range resolutions {
		resMap[r] = true
	}

	rangeStatuses := s.getInferredRange(sourceID, startStatus, endStatus, issues)

	var cycleTimes []float64
	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}
		if len(resolutions) > 0 && !resMap[issue.Resolution] {
			if m, ok := mappings[issue.Status]; !ok || m.Outcome != "delivered" {
				continue
			}
		} else if len(resolutions) == 0 {
			if m, ok := mappings[issue.Status]; !ok || m.Outcome != "delivered" {
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
