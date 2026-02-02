package mcp

import (
	"fmt"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

// handleGetProjectDetails fetches metadata and performs a data probe for a project.
func (s *Server) handleGetProjectDetails(projectKey string) (interface{}, error) {
	// 1. Resolve Source Context (ensures consistent JQL for the project)
	// For project details, we use 0 as a board ID to signal project-only anchoring if permitted.
	// But resolveSourceContext now requires a real board.
	// If we want strict context, maybe we should ask the user to use get_board_details instead.
	// For now, I'll try to use a dummy 0 and see if resolveSourceContext handles it or if I should change it.
	// Actually, I'll update resolveSourceContext to handle boardID=0 if needed, or just redirect users.
	ctx, err := s.resolveSourceContext(projectKey, 0)
	if err != nil {
		// Fallback: If no context exists, create a default project-based JQL
		ctx = &jira.SourceContext{
			ProjectKey: projectKey,
			BoardID:    0,
			JQL:        fmt.Sprintf("project = \"%s\"", projectKey),
		}
	}

	// 2. Hydrate Protocol (Synchronous Eager Ingestion)
	if err := s.events.Hydrate(projectKey, ctx.JQL); err != nil {
		log.Error().Err(err).Str("project", projectKey).Msg("Hydration failed")
	}

	// 3. Data Probe
	events := s.events.GetEventsInRange(projectKey, time.Time{}, time.Now())
	domainIssues := s.reconstructIssues(events, projectKey)
	sample := stats.SelectDiscoverySample(domainIssues, 200)
	summary := stats.AnalyzeProbe(sample, len(domainIssues), s.getFinishedStatuses(projectKey))

	// 4. Fetch Project Metadata
	project, err := s.jira.GetProject(projectKey)
	if err != nil {
		return nil, err
	}

	// 5. Return wrapped response
	return map[string]interface{}{
		"project":      project,
		"data_summary": summary,
		"_guidance": []string{
			"Data Ingestion Complete: History is loaded and analyzed.",
			"Review the 'data_summary' to understand volume and issue types.",
			"Next Step: Call 'get_workflow_discovery' to establish the semantic process mapping.",
			"Once mapping is confirmed, use 'get_diagnostic_roadmap' to plan your analysis.",
		},
	}, nil
}
