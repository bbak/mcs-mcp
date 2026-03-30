package mcp

import (
	"fmt"
	"strings"
	"time"

	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/stats/discovery"

	"github.com/rs/zerolog/log"
)

// handleGetBoardDetails fetches metadata and triggers Eager Ingestion (Hydrate).
func (s *Server) handleGetBoardDetails(projectKey string, boardID int) (any, error) {
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Resolve Source Context (ensures consistent JQL and validates board exists)
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	// 2. Anchor Context (Memory Pruning + Metadata Loading)
	if err := s.anchorContext(ctx.ProjectKey, ctx.BoardID); err != nil {
		return nil, err
	}

	// 3. Hydrate Protocol (Synchronous Eager Ingestion)
	reg, err := s.events.Hydrate(sourceID, projectKey, ctx.JQL, s.activeRegistry)
	if err != nil {
		log.Error().Err(err).Str("source", sourceID).Msg("Hydration failed")
		// Proceed anyway to show board metadata
	}
	s.activeRegistry = reg
	if err := s.saveWorkflow(projectKey, boardID); err != nil {
		log.Warn().Err(err).Msg("Failed to persist workflow metadata to disk")
	}

	// 4. Data Probe (Tier-Neutral Discovery)
	events := s.events.GetIssuesInRange(sourceID, time.Time{}, s.Clock())
	first, last, total := stats.DiscoverDatasetBoundaries(events)
	sample := stats.ProjectNeutralSample(events, DataProbeSampleSize)

	summary := discovery.AnalyzeProbe(sample, total)
	summary.Whole.FirstEventAt = first
	summary.Whole.LastEventAt = last

	// 5. Fetch Board Metadata for the response (uses internal Jira cache)
	var board any
	if strings.ToUpper(projectKey) == "MCSTEST" {
		board = map[string]any{
			"id":   boardID,
			"name": fmt.Sprintf("Mock Test Board %d (Synthetic)", boardID),
			"type": "kanban",
		}
	} else {
		var boardErr error
		board, boardErr = s.jira.GetBoard(boardID)
		if boardErr != nil {
			log.Warn().Err(boardErr).Int("boardID", boardID).Msg("Failed to fetch board metadata from Jira")
		}
	}

	// 6. Return wrapped response
	res := map[string]any{
		"board":        board,
		"data_summary": summary,
	}

	guidance := []string{
		"Data Ingestion Complete: History is loaded and analyzed.",
		"Review the 'data_summary' to understand volume and issue types.",
		"Next Step: Call 'workflow_discover_mapping' to establish the semantic process mapping.",
		"Once mapping is confirmed, use 'guide_diagnostic_roadmap' to plan your analysis.",
	}

	return WrapResponse(res, projectKey, boardID, nil, nil, guidance), nil
}
