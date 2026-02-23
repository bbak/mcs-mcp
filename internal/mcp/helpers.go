package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

func getCombinedID(projectKey string, boardID int) string {
	return fmt.Sprintf("%s_%d", projectKey, boardID)
}

func (s *Server) resolveSourceContext(projectKey string, boardID int) (*jira.SourceContext, error) {
	if projectKey == "MCSTEST" {
		return &jira.SourceContext{
			ProjectKey: "MCSTEST",
			BoardID:    boardID,
			JQL:        "project = \"MCSTEST\"",
			FetchedAt:  time.Now(),
		}, nil
	}

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
	cMap, ok := config.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid board config response format from Jira")
	}

	// Extract and Verify Project Key from location
	boardProjectKey := ""
	if loc, ok := cMap["location"].(map[string]any); ok {
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

	filterObj, ok := cMap["filter"].(map[string]any)
	if !ok {
		// Fallback: Try Board Configuration
		log.Debug().Int("boardId", boardID).Msg("Filter missing in board metadata, trying board configuration")
		configObj, err := s.jira.GetBoardConfig(boardID)
		if err == nil {
			if conf, isMap := configObj.(map[string]any); isMap {
				filterObj, ok = conf["filter"].(map[string]any)
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
	fMap, ok := filter.(map[string]any)
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

func (s *Server) formatResult(data any) string {
	out, _ := json.MarshalIndent(data, "", "  ")
	return string(out)
}

// ResponseEnvelope represents the standardized JSON structure for all MCP tool returns.
type ResponseEnvelope struct {
	Context     map[string]any `json:"context,omitempty"`
	Data        any            `json:"data"`
	Diagnostics map[string]any `json:"diagnostics,omitempty"`
	Guardrails  *ResponseGuardrails    `json:"guardrails,omitempty"`
}

type ResponseGuardrails struct {
	Insights []string `json:"insights"`
	Warnings []string `json:"warnings"`
}

// WrapResponse constructs standard ResponseEnvelope for tools.
func WrapResponse(data any, proj string, board int, diagnostics map[string]any, warnings []string, insights []string) ResponseEnvelope {
	ctx := map[string]any{}
	if proj != "" {
		ctx["project_key"] = proj
	}
	if board != 0 {
		ctx["board_id"] = board
	}

	if warnings == nil {
		warnings = []string{}
	}
	if insights == nil {
		insights = []string{}
	}

	return ResponseEnvelope{
		Context:     ctx,
		Data:        data,
		Diagnostics: diagnostics,
		Guardrails: &ResponseGuardrails{
			Insights: insights,
			Warnings: warnings,
		},
	}
}

func (s *Server) getTotalAges(issues []jira.Issue) []float64 {
	var ages []float64
	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}

		// Only count "delivered" work
		if m, ok := stats.GetMetadataRobust(s.activeMapping, issue.StatusID, issue.Status); !ok || m.Outcome != "delivered" {
			continue
		}

		duration := issue.ResolutionDate.Sub(issue.Created).Hours() / 24.0
		if duration > 0 {
			ages = append(ages, duration)
		}
	}

	return ages
}

func (s *Server) getResolutionMap(sourceID string) map[string]string {
	if s.activeSourceID == sourceID && len(s.activeResolutions) > 0 {
		return s.activeResolutions
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

func asString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func asInt(v any) int {
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
		if _, err := fmt.Sscanf(val, "%d", &res); err != nil {
			return 0
		}
		return res
	default:
		return 0
	}
}

func (s *Server) getFinishedStatuses(issues []jira.Issue, events []eventlog.IssueEvent) map[string]bool {
	finished := make(map[string]bool)
	nameToID := make(map[string]string)

	// Extract from issues if available
	for _, issue := range issues {
		if issue.Status != "" && issue.StatusID != "" {
			nameToID[issue.Status] = issue.StatusID
		}
	}

	// Extract from events (history)
	for _, e := range events {
		if e.ToStatus != "" && e.ToStatusID != "" {
			nameToID[e.ToStatus] = e.ToStatusID
		}
		if e.FromStatus != "" && e.FromStatusID != "" {
			nameToID[e.FromStatus] = e.FromStatusID
		}
	}

	for status, meta := range s.activeMapping {
		if meta.Tier == "Finished" {
			finished[status] = true
			lowerName := strings.ToLower(status)
			for name, id := range nameToID {
				if strings.ToLower(name) == lowerName {
					finished[id] = true
				}
			}
		}
	}
	return finished
}

func (s *Server) getActiveStatuses() []string {
	var active []string
	for status, meta := range s.activeMapping {
		if meta.Tier != "Finished" && meta.Tier != "Demand" {
			active = append(active, status)
		}
	}
	return active
}

func (s *Server) calculateWIPAges(issues []jira.Issue, startStatus string, statusWeights map[string]int, mappings map[string]stats.StatusMetadata, cycleTimes []float64) map[string][]float64 {
	ages := make(map[string][]float64)
	// Note: CalculateInventoryAge inside aging.go already handles IDs if we passed them correctly.
	results := stats.CalculateInventoryAge(issues, startStatus, statusWeights, mappings, cycleTimes, "wip", time.Now())
	for _, res := range results {
		if res.AgeSinceCommitment != nil {
			t := res.Type
			if t == "" {
				t = "Unknown"
			}
			ages[t] = append(ages[t], *res.AgeSinceCommitment)
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
