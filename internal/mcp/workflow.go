package mcp

import (
	"sort"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

func (s *Server) getStatusWeights(projectKey string) map[string]int {
	weights := make(map[string]int)
	if projectKey == "" {
		return weights
	}

	if statuses, err := s.jira.GetProjectStatuses(projectKey); err == nil {
		for _, itm := range statuses.([]interface{}) {
			issueTypeMap := itm.(map[string]interface{})
			statusList := issueTypeMap["statuses"].([]interface{})
			for _, sObj := range statusList {
				sMap := sObj.(map[string]interface{})
				name := sMap["name"].(string)
				cat := sMap["statusCategory"].(map[string]interface{})
				key := cat["key"].(string)

				weight := 1
				switch key {
				case "indeterminate":
					weight = 2
				case "done":
					weight = 3
				}
				weights[name] = weight
			}
		}
	}
	return weights
}

func (s *Server) applyBackflowPolicy(issues []jira.Issue, statusWeights map[string]int, commitmentWeight int) []jira.Issue {
	cleaned := make([]jira.Issue, len(issues))
	for i, issue := range issues {
		lastBackflowIdx := -1
		for j, t := range issue.Transitions {
			if w, ok := statusWeights[t.ToStatus]; ok && w < commitmentWeight {
				lastBackflowIdx = j
			}
		}

		if lastBackflowIdx == -1 {
			cleaned[i] = issue
			continue
		}

		// Keep original Created date to preserve Total Age
		newIssue := issue
		newIssue.Transitions = nil
		if lastBackflowIdx < len(issue.Transitions)-1 {
			newIssue.Transitions = issue.Transitions[lastBackflowIdx+1:]
		}

		// Rebuild residency from the new starting point
		newIssue.StatusResidency = s.rebuildResidency(newIssue, issue.Transitions[lastBackflowIdx].ToStatus)
		cleaned[i] = newIssue
	}
	return cleaned
}

func (s *Server) rebuildResidency(issue jira.Issue, initialStatus string) map[string]int64 {
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

	// 1. Initial status duration
	firstDuration := int64(issue.Transitions[0].Date.Sub(issue.Created).Seconds())
	if firstDuration > 0 {
		residency[initialStatus] = firstDuration
	}

	// 2. Intermediate transitions
	for i := 0; i < len(issue.Transitions)-1; i++ {
		duration := int64(issue.Transitions[i+1].Date.Sub(issue.Transitions[i].Date).Seconds())
		if duration > 0 {
			residency[issue.Transitions[i].ToStatus] += duration
		}
	}

	// 3. Last status duration
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

func (s *Server) getCycleTimes(sourceID string, issues []jira.Issue, startStatus, endStatus string, statusWeights map[string]int, resolutions []string) []float64 {
	resMap := make(map[string]bool)
	for _, r := range resolutions {
		resMap[r] = true
	}

	rangeStatuses := s.getInferredRange(sourceID, startStatus, endStatus, issues, statusWeights)

	var cycleTimes []float64
	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}
		if len(resolutions) > 0 && !resMap[issue.Resolution] {
			continue
		}

		duration := stats.SumRangeDuration(issue, rangeStatuses)
		if duration > 0 {
			cycleTimes = append(cycleTimes, duration)
		}
	}

	return cycleTimes
}

func (s *Server) getInferredRange(sourceID, startStatus, endStatus string, issues []jira.Issue, statusWeights map[string]int) []string {
	// 1. Check if we have a persisted session ordering
	if order, ok := s.statusOrderings[sourceID]; ok {
		return s.sliceRange(order, startStatus, endStatus)
	}

	// 2. Fallback: Inferred order from historical reachability/categories
	// We'll use the statuses present in the issues
	statusMap := make(map[string]bool)
	for _, issue := range issues {
		for st := range issue.StatusResidency {
			statusMap[st] = true
		}
	}
	var allStatuses []string
	for st := range statusMap {
		allStatuses = append(allStatuses, st)
	}

	// Simple heuristic sort: by weight, then by name
	sort.Slice(allStatuses, func(i, j int) bool {
		wi := statusWeights[allStatuses[i]]
		wj := statusWeights[allStatuses[j]]
		if wi != wj {
			return wi < wj
		}
		return allStatuses[i] < allStatuses[j]
	})

	return s.sliceRange(allStatuses, startStatus, endStatus)
}

func (s *Server) getEarliestCommitment(sourceID string) string {
	mappings := s.workflowMappings[sourceID]
	order := s.statusOrderings[sourceID]
	if len(mappings) == 0 {
		return ""
	}

	// Try to find status mapped to 'Downstream'
	// If we have an ordering, use it to find the first one
	if len(order) > 0 {
		for _, status := range order {
			if m, ok := mappings[status]; ok && (m.Tier == "Downstream" || m.Tier == "Finished") {
				return status
			}
		}
	} else {
		// Fallback: search all mappings
		for status, m := range mappings {
			if m.Tier == "Downstream" {
				return status
			}
		}
	}
	return ""
}

func (s *Server) getCommitmentPointHints(issues []jira.Issue, statusWeights map[string]int) []string {
	reachability := make(map[string]int)
	for _, issue := range issues {
		visited := make(map[string]bool)
		for _, trans := range issue.Transitions {
			visited[trans.ToStatus] = true
		}
		for status := range visited {
			reachability[status]++
		}
	}

	type candidate struct {
		name  string
		count int
	}
	var candidates []candidate
	for name, count := range reachability {
		// Prioritize "Indeterminate" (weight 2) categories as commitment point candidates
		if weight, ok := statusWeights[name]; ok && weight == 2 {
			candidates = append(candidates, candidate{name, count})
		}
	}

	// Sort candidates by frequency of usage
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[j].count < candidates[i].count
	})

	var result []string
	for i := 0; i < len(candidates) && i < 3; i++ {
		result = append(result, candidates[i].name)
	}
	return result
}

func (s *Server) sliceRange(order []string, start, end string) []string {
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
		return []string{order[startIndex]} // Fallback to just the start status
	}

	return order[startIndex : endIndex+1]
}
