package mcp

import (
	"fmt"
	"strings"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/stats/discovery"

	"github.com/rs/zerolog/log"
)

// handleGetProjectDetails fetches metadata and performs a data probe for a project.
func (s *Server) handleGetProjectDetails(projectKey string) (any, error) {
	// 1. Resolve Source Context (ensures consistent JQL for the project)
	ctx, err := s.resolveSourceContext(projectKey, 0)
	if err != nil {
		// Fallback: If no context exists, create a default project-based JQL
		ctx = &jira.SourceContext{
			ProjectKey: projectKey,
			BoardID:    0,
			JQL:        fmt.Sprintf("project = \"%s\"", projectKey),
		}
	}

	// 2. Anchor Context (Memory Pruning + Metadata Loading)
	if err := s.anchorContext(ctx.ProjectKey, ctx.BoardID); err != nil {
		return nil, err
	}

	// 3. Hydrate Protocol (Synchronous Eager Ingestion)
	if err := s.events.Hydrate(projectKey, ctx.JQL); err != nil {
		log.Error().Err(err).Str("project", projectKey).Msg("Hydration failed")
	}

	// 4. Data Probe (Tier-Neutral Discovery)
	events := s.events.GetEventsInRange(projectKey, time.Time{}, time.Now())
	first, last, total := stats.DiscoverDatasetBoundaries(events)
	sample := stats.ProjectNeutralSample(events, 200)

	summary := discovery.AnalyzeProbe(sample, total)
	summary.Whole.FirstEventAt = first
	summary.Whole.LastEventAt = last

	// 4. Fetch Project Metadata
	var project any
	if strings.ToUpper(projectKey) == "MCSTEST" {
		project = map[string]any{
			"key":  "MCSTEST",
			"name": "Mock Test Project (Synthetic)",
		}
	} else {
		project, err = s.jira.GetProject(projectKey)
		if err != nil {
			return nil, err
		}
	}

	// 5. Return wrapped response
	res := map[string]any{
		"project":      project,
		"data_summary": summary,
	}

	guidance := []string{
		"Data Ingestion Complete: History is loaded and analyzed.",
		"Review the 'data_summary' to understand volume and issue types.",
		"Next Step: Call 'workflow_discover_mapping' to establish the semantic process mapping.",
		"Once mapping is confirmed, use 'guide_diagnostic_roadmap' to plan your analysis.",
	}

	return WrapResponse(res, projectKey, 0, nil, nil, guidance), nil
}
