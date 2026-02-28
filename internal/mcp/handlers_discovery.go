package mcp

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/stats/discovery"

	"github.com/rs/zerolog/log"
)

func (s *Server) handleGetWorkflowDiscovery(projectKey string, boardID int, forceRefresh bool) (any, error) {
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
	reg, err := s.events.Hydrate(sourceID, projectKey, ctx.JQL, s.activeRegistry)
	if err != nil {
		log.Error().Err(err).Str("source", sourceID).Msg("Hydration failed")
	}
	s.activeRegistry = reg
	_ = s.saveWorkflow(projectKey, boardID)

	// 3. Data Probe (Tier-Neutral Discovery for Summary)
	events := s.events.GetEventsInRange(sourceID, time.Time{}, s.Clock())
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
	if m, ok := res.(map[string]any); ok {
		if meta, ok := m["_metadata"].(map[string]any); ok {
			meta["is_cached"] = isCachedMapping
		} else {
			m["_metadata"] = map[string]any{
				"is_cached": isCachedMapping,
			}
		}
	}

	return res, nil
}

func (s *Server) presentWorkflowMetadata(sourceID string, sample []jira.Issue, totalCount int, first, last time.Time, discoverySource string) any {
	persistence := stats.CalculateStatusPersistence(sample)

	var mapping map[string]stats.StatusMetadata
	var recommendedCP string
	if discoverySource == "LOADED_FROM_CACHE" {
		mapping = s.activeMapping
	} else {
		mapping, recommendedCP = discovery.ProposeSemantics(sample, persistence)
	}

	// Build a set of significant statuses keyed by ID for filtering
	significantByID := make(map[string]bool)
	for _, p := range persistence {
		significantByID[stats.PreferID(p.StatusID, p.StatusName)] = true
	}

	finalMapping := make(map[string]stats.StatusMetadata)
	for key, meta := range mapping {
		if !significantByID[key] {
			continue
		}
		finalMapping[key] = meta
	}

	rawOrder := discovery.DiscoverStatusOrder(sample)
	var discoveredOrder []string
	if discoverySource == "LOADED_FROM_CACHE" && len(s.activeStatusOrder) > 0 {
		discoveredOrder = s.activeStatusOrder
	} else {
		for _, st := range rawOrder {
			if significantByID[st] {
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

		slices.SortStableFunc(discoveredOrder, func(a, b string) int {
			ta := finalMapping[a].Tier
			tb := finalMapping[b].Tier
			return cmp.Compare(tierWeights[ta], tierWeights[tb])
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

	res := map[string]any{
		"source_id": sourceID,
		"workflow": map[string]any{
			"status_mapping":    finalMapping,
			"status_order":      discoveredOrder,
			"persistence_stats": persistence,
		},
		"data_summary": summary,
	}

	// Internal metadata: map names back to IDs for the AI to use in set_mapping if needed
	// Actually, we want the AI to send back IDs if it can discover them
	// BUT for now, we'll keep the AI working with names and let the server map them to IDs.

	diagnostics := map[string]any{
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

func (s *Server) handleSetWorkflowMapping(projectKey string, boardID int, mapping map[string]any, resolutions map[string]any, commitmentPoint string) (any, error) {
	sourceID := getCombinedID(projectKey, boardID)

	// Ensure we are anchored
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	// Map names to IDs for internal stability
	m := make(map[string]stats.StatusMetadata)
	for k, v := range mapping {
		if vm, ok := v.(map[string]any); ok {
			sm := stats.StatusMetadata{
				Name:    k, // Store the name for display
				Tier:    asString(vm["tier"]),
				Role:    asString(vm["role"]),
				Outcome: asString(vm["outcome"]),
			}

			// Try to find the ID for this status name
			id := s.activeRegistry.GetStatusID(k)
			if id != "" {
				m[id] = sm
			} else {
				// Fallback: use the name as key (might already be an ID or a custom name)
				m[k] = sm
			}
		}
	}
	s.activeMapping = m

	rm := make(map[string]string)
	if len(resolutions) > 0 {
		for k, v := range resolutions {
			outcome := asString(v)
			id := s.activeRegistry.GetResolutionID(k)
			if id != "" {
				rm[id] = outcome
			} else {
				rm[k] = outcome
			}
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

func (s *Server) handleSetWorkflowOrder(projectKey string, boardID int, order []string) (any, error) {
	sourceID := getCombinedID(projectKey, boardID)

	// Ensure we are anchored
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	// Map incoming names to IDs for internal stability
	var resolvedOrder []string
	for _, entry := range order {
		id := s.activeRegistry.GetStatusID(entry)
		if id != "" {
			resolvedOrder = append(resolvedOrder, id)
		} else {
			resolvedOrder = append(resolvedOrder, entry) // Already an ID or unknown
		}
	}
	s.activeStatusOrder = resolvedOrder

	// Save to disk
	if err := s.saveWorkflow(projectKey, boardID); err != nil {
		log.Error().Err(err).Msg("Failed to save workflow metadata")
		return nil, fmt.Errorf("metadata updated in memory but failed to save to disk: %w", err)
	}

	return WrapResponse(map[string]string{"status": "success", "message": fmt.Sprintf("Stored and PERSISTED workflow order for source %s", sourceID)}, projectKey, boardID, nil, nil, nil), nil
}

func (s *Server) handleSetEvaluationDate(projectKey string, boardID int, dateStr string) (any, error) {
	// Ensure we are anchored before saving
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}
	if dateStr == "" {
		s.activeEvaluationDate = nil
	} else {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, fmt.Errorf("invalid evaluation date format: %w", err)
		}
		s.activeEvaluationDate = &t
	}

	// Save to disk
	if err := s.saveWorkflow(projectKey, boardID); err != nil {
		log.Error().Err(err).Msg("Failed to save workflow metadata")
		return nil, fmt.Errorf("metadata updated in memory but failed to save to disk: %w", err)
	}

	var guidance []string
	msg := "Successfully cleared the evaluation date. Analysis will use real-time time.Now()."
	if s.activeEvaluationDate != nil {
		msg = fmt.Sprintf("Successfully set the evaluation date to %s. All analysis models will be evaluated relative to this date.", s.activeEvaluationDate.Format("2006-01-02"))
		guidance = append(guidance, "If shifting significantly into the past, you may need to use `import_history_expand` to fetch more historical data.")
	}

	return WrapResponse(map[string]string{"status": "success", "message": msg}, projectKey, boardID, nil, nil, guidance), nil
}
