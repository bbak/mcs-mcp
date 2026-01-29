package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

func (s *Server) resolveSourceContext(sourceID, sourceType string) (*jira.SourceContext, error) {
	var jql string
	var projectKey string

	if sourceType == "board" {
		var id int
		_, err := fmt.Sscanf(sourceID, "%d", &id)
		if err != nil {
			return nil, fmt.Errorf("invalid board ID: %s", sourceID)
		}
		config, err := s.jira.GetBoard(id)
		if err != nil {
			return nil, err
		}
		cMap := config.(map[string]interface{})

		// Extract Project Key from location if available
		if loc, ok := cMap["location"].(map[string]interface{}); ok {
			projectKey = fmt.Sprintf("%v", loc["projectKey"])
		}

		filterObj := cMap["filter"].(map[string]interface{})
		filterID := fmt.Sprintf("%v", filterObj["id"])
		filter, err := s.jira.GetFilter(filterID)
		if err != nil {
			return nil, err
		}
		jql = filter.(map[string]interface{})["jql"].(string)

		// Board-Specific refinement: Exclude sub-tasks
		jql = fmt.Sprintf("(%s) AND issuetype not in subtaskIssueTypes()", jql)
	} else {
		filter, err := s.jira.GetFilter(sourceID)
		if err != nil {
			return nil, err
		}
		jql = filter.(map[string]interface{})["jql"].(string)
	}

	// Strip existing ORDER BY to avoid collision
	jqlLower := strings.ToLower(jql)
	if idx := strings.Index(jqlLower, " order by"); idx != -1 {
		jql = jql[:idx]
	}

	return &jira.SourceContext{
		SourceID:       sourceID,
		SourceType:     sourceType,
		JQL:            jql,
		PrimaryProject: projectKey,
		FetchedAt:      time.Now(),
	}, nil
}

func (s *Server) formatResult(data interface{}) string {
	out, _ := json.MarshalIndent(data, "", "  ")
	return string(out)
}

func (s *Server) extractProjectKeys(issues []jira.Issue) []string {
	keyMap := make(map[string]bool)
	for _, issue := range issues {
		if issue.ProjectKey != "" {
			keyMap[issue.ProjectKey] = true
		}
	}

	keys := make([]string, 0, len(keyMap))
	for k := range keyMap {
		keys = append(keys, k)
	}
	return keys
}

func (s *Server) getStatusCategories(projectKeys []string) map[string]string {
	cats := make(map[string]string)
	for _, projectKey := range projectKeys {
		if projectKey == "" {
			continue
		}

		if statuses, err := s.jira.GetProjectStatuses(projectKey); err == nil {
			for _, itm := range statuses.([]interface{}) {
				issueTypeMap, ok := itm.(map[string]interface{})
				if !ok {
					continue
				}
				statusList, ok := issueTypeMap["statuses"].([]interface{})
				if !ok {
					continue
				}
				for _, sObj := range statusList {
					sMap, ok := sObj.(map[string]interface{})
					if !ok {
						continue
					}
					name := asString(sMap["name"])
					cat, ok := sMap["statusCategory"].(map[string]interface{})
					if ok {
						cats[name] = asString(cat["key"])
					}
				}
			}
		}
	}
	return cats
}

func (s *Server) getTotalAges(sourceID string, issues []jira.Issue, resolutions []string) []float64 {
	mappings := s.workflowMappings[sourceID]
	resMap := make(map[string]bool)
	for _, r := range resolutions {
		resMap[r] = true
	}

	var ages []float64
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

		duration := issue.ResolutionDate.Sub(issue.Created).Hours() / 24.0
		if duration > 0 {
			ages = append(ages, duration)
		}
	}

	return ages
}

func (s *Server) getResolutionMap(sourceID string) map[string]string {
	if rm, ok := s.resolutionMappings[sourceID]; ok && len(rm) > 0 {
		return rm
	}
	// Fallback to defaults
	return map[string]string{
		"Fixed":            "delivered",
		"Done":             "delivered",
		"Complete":         "delivered",
		"Resolved":         "delivered",
		"Approved":         "delivered",
		"Closed":           "delivered",
		"Won't Do":         "abandoned",
		"Discarded":        "abandoned",
		"Obsolete":         "abandoned",
		"Duplicate":        "abandoned",
		"Cannot Reproduce": "abandoned",
		"Declined":         "abandoned",
	}
}

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func asInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case string:
		var res int
		fmt.Sscanf(val, "%d", &res)
		return res
	default:
		return 0
	}
}

func (s *Server) getFinishedStatuses(sourceID string) map[string]bool {
	finished := make(map[string]bool)
	if m, ok := s.workflowMappings[sourceID]; ok {
		for status, meta := range m {
			if meta.Tier == "Finished" {
				finished[status] = true
			}
		}
	}
	return finished
}

func (s *Server) calculateWIPAges(issues []jira.Issue, startStatus string, statusWeights map[string]int, mappings map[string]stats.StatusMetadata, cycleTimes []float64) []float64 {
	var ages []float64
	results := stats.CalculateInventoryAge(issues, startStatus, statusWeights, mappings, cycleTimes, "wip")
	for _, res := range results {
		if res.AgeSinceCommitment != nil {
			ages = append(ages, *res.AgeSinceCommitment)
		}
	}
	return ages
}
