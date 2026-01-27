package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mcs-mcp/internal/jira"
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
		config, err := s.jira.GetBoardConfig(id)
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

	// If projectKey is still empty, we'll have to infer it from results later
	// but for now, we return the context
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

func (s *Server) extractProjectKey(issues []jira.Issue) string {
	if len(issues) == 0 {
		return ""
	}
	return issues[0].ProjectKey
}

func (s *Server) getStatusCategories(projectKey string) map[string]string {
	cats := make(map[string]string)
	if projectKey == "" {
		return cats
	}

	if statuses, err := s.jira.GetProjectStatuses(projectKey); err == nil {
		for _, itm := range statuses.([]interface{}) {
			issueTypeMap := itm.(map[string]interface{})
			statusList := issueTypeMap["statuses"].([]interface{})
			for _, sObj := range statusList {
				sMap := sObj.(map[string]interface{})
				name := sMap["name"].(string)
				cat := sMap["statusCategory"].(map[string]interface{})
				cats[name] = fmt.Sprintf("%v", cat["key"])
			}
		}
	}
	return cats
}

func (s *Server) getTotalAges(issues []jira.Issue, resolutions []string) []float64 {
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
			continue
		}

		duration := issue.ResolutionDate.Sub(issue.Created).Hours() / 24.0
		if duration > 0 {
			ages = append(ages, duration)
		}
	}

	return ages
}

func (s *Server) getResolutionMap() map[string]string {
	// Simple heuristic for now. In a real system, we'd fetch this from Jira
	// or allow user configuration.
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
