package mcp

import (
	"fmt"

	"mcs-mcp/internal/eventlog"

	"github.com/rs/zerolog/log"
)

func (s *Server) handleCacheCatchUp(projectKey string, boardID int) (any, error) {
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Resolve Source Context
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	// 2. Ensure Anchored (required for most-recent-update detection)
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	// 3. CatchUp
	fetched, nmrc, reg, err := s.events.CatchUp(sourceID, projectKey, ctx.JQL, s.activeRegistry)
	if err != nil {
		return nil, err
	}
	s.activeRegistry = reg

	// 4. Re-calculate DiscoveryCutoff (just in case)
	s.recalculateDiscoveryCutoff(sourceID)
	if err := s.saveWorkflow(projectKey, boardID); err != nil {
		log.Warn().Err(err).Msg("Failed to persist workflow metadata to disk")
	}

	res := map[string]any{
		"message": fmt.Sprintf("%d items fetched that were updated since %s", fetched, nmrc.Format(eventlog.DateTimeFormat)),
		"fetched": fetched,
		"nmrc":    nmrc,
	}

	return WrapResponse(res, projectKey, boardID, nil, nil, nil), nil
}

