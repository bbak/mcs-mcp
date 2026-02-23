package mcp

import (
	"fmt"
)

func (s *Server) handleCacheCatchUp(projectKey string, boardID int) (interface{}, error) {
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
	fetched, nmrc, err := s.events.CatchUp(sourceID, ctx.JQL)
	if err != nil {
		return nil, err
	}

	// 4. Re-calculate DiscoveryCutoff (just in case)
	s.recalculateDiscoveryCutoff(sourceID)
	_ = s.saveWorkflow(projectKey, boardID)

	res := map[string]interface{}{
		"message": fmt.Sprintf("%d items fetched that were updated since %s", fetched, nmrc.Format("2006-01-02 15:04")),
		"fetched": fetched,
		"nmrc":    nmrc,
	}

	return WrapResponse(res, projectKey, boardID, nil, nil, nil), nil
}

func (s *Server) handleCacheExpandHistory(projectKey string, boardID int, chunks int) (interface{}, error) {
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
	fetched, omrc, err := s.events.ExpandHistory(sourceID, ctx.JQL, chunks)
	if err != nil {
		return nil, err
	}

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
	res := map[string]interface{}{
		"message": msg,
		"fetched": fetched,
		"omrc":    omrc,
	}

	diagnostics := map[string]interface{}{
		"discovery_cutoff": cutoffStr,
	}

	return WrapResponse(res, projectKey, boardID, diagnostics, nil, nil), nil
}
