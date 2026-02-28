package mcp

import (
	"fmt"
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
	_ = s.saveWorkflow(projectKey, boardID)

	res := map[string]any{
		"message": fmt.Sprintf("%d items fetched that were updated since %s", fetched, nmrc.Format("2006-01-02 15:04")),
		"fetched": fetched,
		"nmrc":    nmrc,
	}

	return WrapResponse(res, projectKey, boardID, nil, nil, nil), nil
}

func (s *Server) handleCacheExpandHistory(projectKey string, boardID int, chunks int) (any, error) {
	if chunks <= 0 {
		chunks = 3 // Default
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Resolve Source Context
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	// 2. Ensure Anchored
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	// 3. Expand History
	fetched, omrc, reg, err := s.events.ExpandHistory(sourceID, projectKey, ctx.JQL, chunks, s.activeRegistry)
	if err != nil {
		return nil, err
	}
	s.activeRegistry = reg

	// 4. Re-calculate DiscoveryCutoff
	s.recalculateDiscoveryCutoff(sourceID)
	_ = s.saveWorkflow(projectKey, boardID)

	cutoffStr := "N/A"
	if s.activeDiscoveryCutoff != nil {
		cutoffStr = s.activeDiscoveryCutoff.Format("2006-01-02")
	}

	msg := fmt.Sprintf("%d work items fetched that were updated before %s. Updated DiscoveryCutoff: %s.",
		fetched, omrc.Format("2006-01-02 15:04"), cutoffStr)

	// The ExpandHistory internal call already triggered its own catch-up log, but we return a clean integrated message.
	res := map[string]any{
		"message": msg,
		"fetched": fetched,
		"omrc":    omrc,
	}

	diagnostics := map[string]any{
		"discovery_cutoff": cutoffStr,
	}

	return WrapResponse(res, projectKey, boardID, diagnostics, nil, nil), nil
}
