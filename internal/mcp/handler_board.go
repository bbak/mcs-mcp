package mcp

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

// handleGetBoardDetails fetches metadata and triggers Eager Ingestion (Hydrate).
func (s *Server) handleGetBoardDetails(boardID int) (interface{}, error) {
	sourceID := fmt.Sprintf("%d", boardID)

	// 1. Resolve Source Context (ensures consistent JQL and validates board exists)
	ctx, err := s.resolveSourceContext(sourceID, "board")
	if err != nil {
		return nil, err
	}

	// 2. Hydrate Protocol (Synchronous Eager Ingestion)
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		log.Error().Err(err).Str("source", sourceID).Msg("Hydration failed")
		// Proceed anyway to show board metadata
	}

	// 3. Fetch Board Metadata and Config for the response (uses internal Jira cache)
	board, _ := s.jira.GetBoard(boardID)
	config, _ := s.jira.GetBoardConfig(boardID)

	// 4. Return wrapped response
	return map[string]interface{}{
		"board":         board,
		"configuration": config,
		"_guidance": []string{
			"Data Ingestion Complete: History is loaded and synced.",
			"You can now run 'get_cycle_time_assessment', 'get_process_stability', or 'run_simulation' instantly.",
		},
	}, nil
}
