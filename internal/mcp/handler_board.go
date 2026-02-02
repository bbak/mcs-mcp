package mcp

import (
	"time"

	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

// handleGetBoardDetails fetches metadata and triggers Eager Ingestion (Hydrate).
func (s *Server) handleGetBoardDetails(projectKey string, boardID int) (interface{}, error) {
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Resolve Source Context (ensures consistent JQL and validates board exists)
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	// 2. Hydrate Protocol (Synchronous Eager Ingestion)
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		log.Error().Err(err).Str("source", sourceID).Msg("Hydration failed")
		// Proceed anyway to show board metadata
	}

	// 3. Data Probe (Analysis on existing hydrated data)
	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	domainIssues := s.reconstructIssues(events, sourceID)
	sample := stats.SelectDiscoverySample(domainIssues, 200)
	summary := stats.AnalyzeProbe(sample, len(domainIssues), s.getFinishedStatuses(sourceID))

	// 4. Fetch Board Metadata and Config for the response (uses internal Jira cache)
	board, _ := s.jira.GetBoard(boardID)
	config, _ := s.jira.GetBoardConfig(boardID)

	// 5. Return wrapped response
	return map[string]interface{}{
		"board":         board,
		"configuration": config,
		"data_summary":  summary,
		"_guidance": []string{
			"Data Ingestion Complete: History is loaded and analyzed.",
			"Review the 'data_summary' to understand volume and issue types.",
			"Next Step: Call 'get_workflow_discovery' to establish the semantic process mapping.",
			"Once mapping is confirmed, use 'get_diagnostic_roadmap' to plan your analysis.",
		},
	}, nil
}
