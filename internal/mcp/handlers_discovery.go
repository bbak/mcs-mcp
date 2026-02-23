package mcp

import (
	"fmt"
	"sort"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/stats/discovery"

	"github.com/rs/zerolog/log"
)

func (s *Server) handleGetWorkflowDiscovery(projectKey string, boardID int, forceRefresh bool) (interface{}, error) {
	// 1. Resolve Source Context (ensures consistent JQL)
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// Anchoring: Mapping Load
	isCachedMapping, err := s.loadWorkflow(projectKey, boardID)
	if err != nil {
		log.Error().Err(err).Str("source", sourceID).Msg("Failed to load workflow mapping")
	}

	// 2. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		log.Error().Err(err).Str("source", sourceID).Msg("Hydration failed")
	}

	// 3. Data Probe (Tier-Neutral Discovery for Summary)
	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	first, last, total := stats.DiscoverDatasetBoundaries(events)

	issues := stats.ProjectNeutralSample(events, 200)

	discoveryResult := discovery.DiscoverWorkflow(events, issues, s.getResolutionMap(sourceID))

	sample := discoveryResult.Sample

	// Check if we have an active mapping from cache and NO force refresh
	discoverySource := "NEWLY_PROPOSED"
	if !forceRefresh && isCachedMapping {
		discoverySource = "LOADED_FROM_CACHE"
	}

	res := s.presentWorkflowMetadata(sourceID, sample, total, first, last, discoverySource)

	// Add is_cached signal to _metadata
	if m, ok := res.(map[string]interface{}); ok {
		if meta, ok := m["_metadata"].(map[string]interface{}); ok {
			meta["is_cached"] = isCachedMapping
		} else {
			m["_metadata"] = map[string]interface{}{
				"is_cached": isCachedMapping,
			}
		}
	}

	return res, nil
}

func (s *Server) presentWorkflowMetadata(sourceID string, sample []jira.Issue, totalCount int, first, last time.Time, discoverySource string) interface{} {
	persistence := stats.CalculateStatusPersistence(sample)

	var mapping map[string]stats.StatusMetadata
	var recommendedCP string
	if discoverySource == "LOADED_FROM_CACHE" {
		mapping = s.activeMapping
	} else {
		mapping, recommendedCP = discovery.ProposeSemantics(sample, persistence)
	}

	// Build a map of significant statuses for quick lookup
	significant := make(map[string]bool)
	for _, p := range persistence {
		significant[p.StatusName] = true
	}

	finalMapping := make(map[string]stats.StatusMetadata)
	for name, meta := range mapping {
		if !significant[name] {
			continue
		}
		finalMapping[name] = meta
	}

	rawOrder := discovery.DiscoverStatusOrder(sample)
	var discoveredOrder []string
	if discoverySource == "LOADED_FROM_CACHE" && len(s.activeStatusOrder) > 0 {
		discoveredOrder = s.activeStatusOrder
	} else {
		for _, st := range rawOrder {
			if significant[st] {
				discoveredOrder = append(discoveredOrder, st)
			}
		}

		// Sort by Tier: Demand < Upstream < Downstream < Finished
		tierWeights := map[string]int{
			"Demand":     1,
			"Upstream":   2,
			"Downstream": 3,
			"Finished":   4,
		}

		sort.SliceStable(discoveredOrder, func(i, j int) bool {
			ti := finalMapping[discoveredOrder[i]].Tier
			tj := finalMapping[discoveredOrder[j]].Tier
			if tierWeights[ti] != tierWeights[tj] {
				return tierWeights[ti] < tierWeights[tj]
			}
			return i < j
		})
	}

	summary := discovery.AnalyzeProbe(sample, totalCount)
	summary.Whole.FirstEventAt = first
	summary.Whole.LastEventAt = last
	if discoverySource == "LOADED_FROM_CACHE" && s.activeCommitmentPoint != "" {
		summary.RecommendedCommitmentPoint = s.activeCommitmentPoint
	} else {
		summary.RecommendedCommitmentPoint = recommendedCP
	}

	res := map[string]interface{}{
		"source_id": sourceID,
		"workflow": map[string]interface{}{
			"status_mapping":    finalMapping,
			"status_order":      discoveredOrder,
			"persistence_stats": persistence,
		},
		"data_summary": summary,
	}

	diagnostics := map[string]interface{}{
		"discovery_source": discoverySource,
	}

	guidance := []string{
		"AI MUST summarize the mapping of ALL statuses to TIERS (Demand, Upstream, Downstream, Finished) for the user in a clear table or list.",
		"AI MUST confirm the 'Outcome Strategy' (Value vs. Abandonment):",
		"  - PRIMARY: Jira Resolutions (if they exist).",
		"  - SECONDARY: Status mapping (only if resolutions are missing).",
		"AI MUST ask the user to confirm the 'Commitment Point' (the status where work officially starts).",
		"PROCESS STABILITY: Understand that Stability measures Cycle-Time predictability, NOT throughput volume.",
	}

	var insights []string
	if discoverySource == "LOADED_FROM_CACHE" {
		insights = append(insights, "PREVIOUSLY VERIFIED: This mapping was LOADED FROM DISK and has been previously confirmed by the user. Treat this as the source of truth for your analysis.")
		insights = append(insights, "AI SHOULD simply present this mapping to reconfirm it with the user, rather than proposing it as a new discovery.")
	} else {
		insights = append(insights, "NOTE: This is a NEW PROPOSAL based on recent data patterns. AI MUST verify this with the user before proceeding to diagnostics.")
	}

	return WrapResponse(res, "", 0, diagnostics, guidance, insights)
}

func (s *Server) handleSetWorkflowMapping(projectKey string, boardID int, mapping map[string]interface{}, resolutions map[string]interface{}, commitmentPoint string) (interface{}, error) {
	sourceID := getCombinedID(projectKey, boardID)

	// Ensure we are anchored
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	m := make(map[string]stats.StatusMetadata)
	for k, v := range mapping {
		if vm, ok := v.(map[string]interface{}); ok {
			m[k] = stats.StatusMetadata{
				Tier:    asString(vm["tier"]),
				Role:    asString(vm["role"]),
				Outcome: asString(vm["outcome"]),
			}
		}
	}
	s.activeMapping = m

	rm := make(map[string]string)
	if len(resolutions) > 0 {
		for k, v := range resolutions {
			rm[k] = asString(v)
		}
	}
	s.activeResolutions = rm
	s.activeCommitmentPoint = commitmentPoint

	// Calculate and persist DiscoveryCutoff based on confirmed mapping
	s.recalculateDiscoveryCutoff(sourceID)

	// Save to disk
	if err := s.saveWorkflow(projectKey, boardID); err != nil {
		log.Error().Err(err).Msg("Failed to save workflow metadata")
		return nil, fmt.Errorf("metadata updated in memory but failed to save to disk: %w", err)
	}

	return WrapResponse(map[string]string{"status": "success", "message": fmt.Sprintf("Stored and PERSISTED workflow mapping for source %s", sourceID)}, projectKey, boardID, nil, nil, nil), nil
}

func (s *Server) handleSetWorkflowOrder(projectKey string, boardID int, order []string) (interface{}, error) {
	sourceID := getCombinedID(projectKey, boardID)

	// Ensure we are anchored
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	s.activeStatusOrder = order

	// Save to disk
	if err := s.saveWorkflow(projectKey, boardID); err != nil {
		log.Error().Err(err).Msg("Failed to save workflow metadata")
		return nil, fmt.Errorf("metadata updated in memory but failed to save to disk: %w", err)
	}

	return WrapResponse(map[string]string{"status": "success", "message": fmt.Sprintf("Stored and PERSISTED workflow order for source %s", sourceID)}, projectKey, boardID, nil, nil, nil), nil
}
