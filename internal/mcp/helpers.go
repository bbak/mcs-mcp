package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
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
		cMap, ok := config.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid board config response format from Jira")
		}

		// Extract Project Key from location if available
		if loc, ok := cMap["location"].(map[string]interface{}); ok {
			projectKey = asString(loc["projectKey"])
		}

		filterObj, ok := cMap["filter"].(map[string]interface{})
		if !ok {
			// Fallback: Try Board Configuration for filter info (common for many Jira versions/board types)
			log.Debug().Int("boardId", id).Msg("Filter missing in board metadata, trying board configuration")
			configObj, err := s.jira.GetBoardConfig(id)
			if err == nil {
				if conf, isMap := configObj.(map[string]interface{}); isMap {
					filterObj, ok = conf["filter"].(map[string]interface{})
				}
			}
		}

		if !ok {
			return nil, fmt.Errorf("board config missing filter information (looked in board metadata and config)")
		}
		filterID := asString(filterObj["id"])
		filter, err := s.jira.GetFilter(filterID)
		if err != nil {
			return nil, err
		}
		fMap, ok := filter.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid filter response format from Jira")
		}
		jql = asString(fMap["jql"])

		// STRIP ORDER BY BEFORE WRAPPING
		jql = stripOrderBy(jql)

		// Board-Specific refinement: Exclude sub-tasks
		jql = fmt.Sprintf("(%s) AND issuetype not in subtaskIssueTypes()", jql)
	} else {
		filter, err := s.jira.GetFilter(sourceID)
		if err != nil {
			return nil, err
		}
		fMap, ok := filter.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid filter response format from Jira")
		}
		jql = asString(fMap["jql"])

		// STRIP ORDER BY
		jql = stripOrderBy(jql)
	}

	return &jira.SourceContext{
		SourceID:       sourceID,
		SourceType:     sourceType,
		JQL:            jql,
		PrimaryProject: projectKey,
		FetchedAt:      time.Now(),
	}, nil
}

func stripOrderBy(jql string) string {
	jqlLower := strings.ToLower(jql)
	if idx := strings.Index(jqlLower, " order by"); idx != -1 {
		return jql[:idx]
	}
	return jql
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
			if m, ok := stats.GetMetadataRobust(mappings, issue.StatusID, issue.Status); !ok || m.Outcome != "delivered" {
				continue
			}
		} else if len(resolutions) == 0 {
			if m, ok := stats.GetMetadataRobust(mappings, issue.StatusID, issue.Status); !ok || m.Outcome != "delivered" {
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
		"Closed":           "abandoned",
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

func (s *Server) getActiveStatuses(sourceID string) []string {
	var active []string
	if m, ok := s.workflowMappings[sourceID]; ok {
		for status, meta := range m {
			if meta.Tier != "Finished" && meta.Tier != "Demand" {
				active = append(active, status)
			}
		}
	}
	return active
}

func (s *Server) calculateWIPAges(issues []jira.Issue, startStatus string, statusWeights map[string]int, mappings map[string]stats.StatusMetadata, cycleTimes []float64) []float64 {
	var ages []float64
	// Note: CalculateInventoryAge inside aging.go already handles IDs if we passed them correctly.
	results := stats.CalculateInventoryAge(issues, startStatus, statusWeights, mappings, cycleTimes, "wip")
	for _, res := range results {
		if res.AgeSinceCommitment != nil {
			ages = append(ages, *res.AgeSinceCommitment)
		}
	}
	return ages
}

func (s *Server) filterWIPIssues(issues []jira.Issue, startStatus string, finishedStatuses map[string]bool) []jira.Issue {
	var wip []jira.Issue
	for _, issue := range issues {
		// An issue is WIP if it's not finished AND has passed the commitment point (or startStatus)
		if finishedStatuses[issue.Status] {
			continue
		}

		isCommitted := false
		if issue.Status == startStatus || issue.StatusID == startStatus {
			isCommitted = true
		} else {
			for _, t := range issue.Transitions {
				if t.ToStatus == startStatus || t.ToStatusID == startStatus {
					isCommitted = true
					break
				}
			}
		}

		if isCommitted {
			wip = append(wip, issue)
		}
	}
	return wip
}
