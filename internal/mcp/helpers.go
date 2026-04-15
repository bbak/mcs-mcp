package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mcs-mcp/internal/charts"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

func getCombinedID(projectKey string, boardID int) string {
	return fmt.Sprintf("%s_%d", projectKey, boardID)
}

// handlerContext holds the resolved context for a standard handler invocation.
type handlerContext struct {
	SourceID string
	Ctx      *jira.SourceContext
}

// prepareHandler performs the standard handler setup: anchor context, resolve source,
// hydrate the event log, and persist workflow metadata.
func (s *Server) prepareHandler(projectKey string, boardID int) (*handlerContext, error) {
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)
	oldestUpdated, reg, err := s.events.Hydrate(sourceID, projectKey, ctx.JQL, s.activeRegistry)
	if err != nil {
		return nil, err
	}
	s.activeRegistry = reg
	if !oldestUpdated.IsZero() {
		s.activeOldestUpdated = &oldestUpdated
	}
	if err := s.saveWorkflow(projectKey, boardID); err != nil {
		log.Warn().Err(err).Msg("Failed to persist workflow metadata to disk")
	}
	return &handlerContext{SourceID: sourceID, Ctx: ctx}, nil
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
		if name := asString(loc["projectName"]); name != "" {
			s.activeProjectName = name
		}
	}
	if name := asString(cMap["name"]); name != "" {
		s.activeBoardName = name
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
	Context     map[string]any      `json:"context,omitempty"`
	Data        any                 `json:"data"`
	Diagnostics map[string]any      `json:"diagnostics,omitempty"`
	Guardrails  *ResponseGuardrails `json:"guardrails,omitempty"`
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

// injectChartURL pushes the tool result into the MRU buffer and returns the
// data with a chart_url injected into the ResponseEnvelope's Context.
// If the tool has no chart template or charting is disabled, data is returned unchanged.
func (s *Server) injectChartURL(toolName string, data any) any {
	if s.chartBuf == nil || !charts.HasTemplate(toolName) {
		return data
	}

	envelope, ok := data.(ResponseEnvelope)
	if !ok {
		return data
	}

	// Serialize the full envelope for the buffer (the HTTP handler will
	// extract .data and .guardrails from it).
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		log.Warn().Err(err).Str("tool", toolName).Msg("Failed to serialize envelope for chart buffer")
		return data
	}

	workflowJSON := s.workflowJSON()
	uuid := s.chartBuf.Push(toolName, envelopeJSON, workflowJSON)

	if envelope.Context == nil {
		envelope.Context = map[string]any{}
	}
	envelope.Context["chart_url"] = fmt.Sprintf("http://localhost:%d/render-charts/%s", s.httpPort, uuid)

	return envelope
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


func (s *Server) getFinishedStatuses() map[string]bool {
	finished := make(map[string]bool)

	// activeMapping is keyed by ID (Phase 2+). StatusMetadata.Name is populated.
	for key, meta := range s.activeMapping {
		if meta.Tier == "Finished" {
			finished[key] = true // ID
			if meta.Name != "" {
				finished[meta.Name] = true // Name for legacy matching
			}
		}
	}
	return finished
}

func (s *Server) detectMappingStaleness(events []eventlog.IssueEvent) string {
	// Collect unique status IDs observed in event data
	observed := make(map[string]bool)
	for i := range events {
		if events[i].ToStatusID != "" {
			observed[events[i].ToStatusID] = true
		}
		if events[i].FromStatusID != "" {
			observed[events[i].FromStatusID] = true
		}
	}

	// Find statuses in events but missing from mapping
	var unmapped []string
	for id := range observed {
		if _, ok := s.activeMapping[id]; !ok {
			name := ""
			if s.activeRegistry != nil {
				name = s.activeRegistry.GetStatusName(id)
			}
			if name != "" {
				unmapped = append(unmapped, name)
			} else {
				unmapped = append(unmapped, id)
			}
		}
	}

	if len(unmapped) > 0 {
		return fmt.Sprintf("Board configuration may have changed: %d status(es) found in data but not in cached mapping (%s). Consider running with force_refresh=true.", len(unmapped), strings.Join(unmapped, ", "))
	}
	return ""
}

func (s *Server) calculateWIPAges(issues []jira.Issue, startStatus string, statusWeights map[string]int, mappings map[string]stats.StatusMetadata, cycleTimes []float64) map[string][]float64 {
	ages := make(map[string][]float64)
	results := stats.CalculateInventoryAge(issues, startStatus, statusWeights, mappings, cycleTimes, "wip", s.commitmentBackflowReset, s.Clock())
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

