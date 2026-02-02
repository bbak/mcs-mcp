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

func getCombinedID(projectKey string, boardID int) string {
	return fmt.Sprintf("%s_%d", projectKey, boardID)
}

func (s *Server) resolveSourceContext(projectKey string, boardID int) (*jira.SourceContext, error) {
	if boardID == 0 {
		return &jira.SourceContext{
			ProjectKey: projectKey,
			BoardID:    0,
			JQL:        fmt.Sprintf("project = \"%s\"", projectKey),
			FetchedAt:  time.Now(),
		}, nil
	}

	config, err := s.jira.GetBoard(boardID)
	if err != nil {
		return nil, err
	}
	cMap, ok := config.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid board config response format from Jira")
	}

	// Extract and Verify Project Key from location
	boardProjectKey := ""
	if loc, ok := cMap["location"].(map[string]interface{}); ok {
		boardProjectKey = asString(loc["projectKey"])
	}

	if projectKey != "" && boardProjectKey != "" && projectKey != boardProjectKey {
		return nil, fmt.Errorf("context mismatch: provided project %s does not match board project %s", projectKey, boardProjectKey)
	}

	// Use provided or board-implicit key
	finalProjectKey := projectKey
	if finalProjectKey == "" {
		finalProjectKey = boardProjectKey
	}

	if finalProjectKey == "" {
		return nil, fmt.Errorf("could not determine project key; please provide it explicitly")
	}

	filterObj, ok := cMap["filter"].(map[string]interface{})
	if !ok {
		// Fallback: Try Board Configuration
		log.Debug().Int("boardId", boardID).Msg("Filter missing in board metadata, trying board configuration")
		configObj, err := s.jira.GetBoardConfig(boardID)
		if err == nil {
			if conf, isMap := configObj.(map[string]interface{}); isMap {
				filterObj, ok = conf["filter"].(map[string]interface{})
			}
		}
	}

	if !ok {
		return nil, fmt.Errorf("board config missing filter information")
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
	jql := asString(fMap["jql"])

	// Strip and Normalize
	jql = stripOrderBy(jql)

	// Anchoring: Ensure JQL is scoped to the project
	if !strings.Contains(strings.ToLower(jql), "project =") && !strings.Contains(strings.ToLower(jql), "project in") {
		jql = fmt.Sprintf("(%s) AND project = \"%s\"", jql, finalProjectKey)
	}

	// Exclude sub-tasks
	jql = fmt.Sprintf("(%s) AND issuetype not in subtaskIssueTypes()", jql)

	return &jira.SourceContext{
		ProjectKey: finalProjectKey,
		BoardID:    boardID,
		JQL:        jql,
		FetchedAt:  time.Now(),
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
