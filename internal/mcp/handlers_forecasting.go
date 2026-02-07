package mcp

import (
	"fmt"
	"math"
	"time"

	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"
)

func (s *Server) handleRunSimulation(projectKey string, boardID int, mode string, includeExistingBacklog bool, additionalItems int, targetDays int, targetDate string, startStatus, endStatus string, issueTypes []string, includeWIP bool, sampleDays int, sampleStartDate, sampleEndDate string, targets map[string]int, mixOverrides map[string]float64) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// Ensure we are anchored before analysis
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// Determine Sampling Window
	histEnd := time.Now()
	if sampleEndDate != "" {
		if t, err := time.Parse("2006-01-02", sampleEndDate); err == nil {
			histEnd = t
		} else {
			return nil, fmt.Errorf("invalid sample_end_date format: %w", err)
		}
	}
	histStart := histEnd.AddDate(0, -6, 0) // Default 6 months
	if sampleStartDate != "" {
		if t, err := time.Parse("2006-01-02", sampleStartDate); err == nil {
			histStart = t
		} else {
			return nil, fmt.Errorf("invalid sample_start_date format: %w", err)
		}
	} else if sampleDays > 0 {
		histStart = histEnd.AddDate(0, 0, -sampleDays)
	}

	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}

	// 2. Project on Demand
	window := stats.NewAnalysisWindow(histStart, histEnd, "day", cutoff)
	// We fetch ALL events up to the end of the window to ensure we reconstruct current state (WIP/Backlog)
	// correctly, even for items that had no activity within the sampling window.
	events := s.events.GetEventsInRange(sourceID, time.Time{}, window.End)
	finished, downstream, upstream, demand := eventlog.ProjectScope(events, window, s.activeCommitmentPoint, s.activeMapping, s.activeResolutions, issueTypes)

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, append(finished, append(downstream, append(upstream, demand...)...)...))
	if startStatus == "" {
		startStatus = analysisCtx.CommitmentPoint
	}

	// Apply Backflow Policy
	cWeight := 2
	if startStatus != "" {
		if w, ok := analysisCtx.StatusWeights[startStatus]; ok {
			cWeight = w
		}
	}

	existingBacklogCount := 0
	wipCount := 0
	var wipAges []float64

	actualTargets := make(map[string]int)
	if includeExistingBacklog {
		// Backlog items are those that exist but have NOT crossed commitment
		// In 4-tier model, this includes Upstream and Demand
		backlog := append(upstream, demand...)
		for _, issue := range backlog {
			existingBacklogCount++
			actualTargets[issue.IssueType]++
		}
	}

	var wipIssues []jira.Issue
	if includeWIP {
		// An issue from ProjectScope is WIP if it's in the downstream slice (already checked for commitment)
		wipIssues = downstream

		wipIssues = stats.ApplyBackflowPolicy(wipIssues, analysisCtx.StatusWeights, cWeight)
		for _, issue := range wipIssues {
			actualTargets[issue.IssueType]++
		}

		cycleTimes := s.getCycleTimes(projectKey, boardID, finished, startStatus, endStatus, issueTypes)
		calcWipAges := s.calculateWIPAges(wipIssues, startStatus, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes)
		wipAges = calcWipAges
		wipCount = len(wipAges)
	}

	var engine *simulation.Engine

	switch mode {
	case "scope":
		finalDays := targetDays
		if targetDate != "" {
			t, err := time.Parse("2006-01-02", targetDate)
			if err != nil {
				return nil, fmt.Errorf("invalid target_date format: %w", err)
			}
			diff := time.Until(t)
			if diff < 0 {
				return nil, fmt.Errorf("target_date must be in the future")
			}
			finalDays = int(diff.Hours() / 24)
		}

		if finalDays <= 0 {
			return nil, fmt.Errorf("target_days must be > 0 (or target_date must be in the future) for scope simulation")
		}

		// Expansion Logic Decision (3 Points)
		expansionEnabled := false
		expansionReason := ""

		if len(targets) > 0 {
			expansionEnabled = true
			expansionReason = "you provided a specific set of target counts"
		} else if len(issueTypes) > 0 {
			expansionEnabled = true
			expansionReason = "you requested a forecast for a specific subset of work item types"
		} else if additionalItems > 0 {
			expansionEnabled = true
			expansionReason = "you added new items to the backlog; we modeled the associated background work"
		}

		h := simulation.NewHistogram(finished, window.Start, window.End, issueTypes, analysisCtx.WorkflowMappings, s.activeResolutions)
		engine = simulation.NewEngine(h)

		// Calculate historical distribution
		dist := make(map[string]float64)
		if d, ok := h.Meta["type_distribution"].(map[string]float64); ok {
			for t, p := range d {
				dist[t] = p
			}
		}

		resObj := engine.RunMultiTypeScopeSimulation(finalDays, 10000, issueTypes, dist, expansionEnabled)

		// Explain Expansion
		totalBG := 0
		for _, c := range resObj.BackgroundItemsPredicted {
			totalBG += c
		}
		if expansionEnabled && totalBG > 0 {
			msg := fmt.Sprintf("Because %s, we added %d background items to match the historical work item distribution.", expansionReason, totalBG)
			resObj.Insights = append(resObj.Insights, msg)
		}

		resObj.Insights = append(resObj.Insights, "Scope Interpretation: Forecast shows total items that will reach 'Done' status, including items currently in progress.")
		resObj.Insights = s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)

		if includeWIP {
			deliveredFiltered := stats.FilterDelivered(finished, s.activeResolutions, s.activeMapping)
			cycleTimes := s.getCycleTimes(projectKey, boardID, deliveredFiltered, startStatus, endStatus, issueTypes)
			engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, 0)
			resObj.Composition = simulation.Composition{
				WIP:             wipCount,
				ExistingBacklog: existingBacklogCount,
				AdditionalItems: additionalItems,
				Total:           wipCount + existingBacklogCount + additionalItems,
			}
		}

		if qReq := s.getQualityWarnings(append(finished, append(downstream, append(upstream, demand...)...)...)); qReq != "" {
			resObj.Warnings = append(resObj.Warnings, qReq)
		}
		return resObj, nil

	case "duration":
		totalBacklog := additionalItems + existingBacklogCount
		if includeWIP {
			totalBacklog += wipCount
		}

		if totalBacklog <= 0 {
			return nil, fmt.Errorf("no items to forecast (additional_items and existing backlog both 0)")
		}

		h := simulation.NewHistogram(finished, window.Start, window.End, issueTypes, analysisCtx.WorkflowMappings, s.activeResolutions)
		engine = simulation.NewEngine(h)

		dist := make(map[string]float64)
		if d, ok := h.Meta["type_distribution"].(map[string]float64); ok {
			for t, p := range d {
				dist[t] = p
			}
		}

		// Prepare targets and distribution
		simTargets := make(map[string]int)

		// Apply Mix Overrides & Re-normalize
		if len(mixOverrides) > 0 {
			newDist := make(map[string]float64)
			overrideSum := 0.0
			for t, p := range mixOverrides {
				newDist[t] = p
				overrideSum += p
			}

			if overrideSum > 1.0 {
				return nil, fmt.Errorf("mix_overrides sum exceeds 1.0 (%.2f)", overrideSum)
			}

			// Distribute remaining probability among non-overridden types
			remainingProb := 1.0 - overrideSum
			histRemainingSum := 0.0
			for t, p := range dist {
				if _, overridden := mixOverrides[t]; !overridden {
					histRemainingSum += p
				}
			}

			for t, p := range dist {
				if _, overridden := mixOverrides[t]; !overridden {
					if histRemainingSum > 0 {
						newDist[t] = (p / histRemainingSum) * remainingProb
					} else {
						// Distribution fallback
						newDist[t] = 0.0
					}
				}
			}
			dist = newDist
		}

		// Populate simTargets
		// Start with actual counts from Jira (High-Fidelity)
		for t, c := range actualTargets {
			simTargets[t] = c
		}

		// Add additional items following historical distribution
		if additionalItems > 0 {
			for t, p := range dist {
				count := int(math.Round(float64(additionalItems) * p))
				if count > 0 {
					simTargets[t] += count
				}
			}
		}

		// Safe fallback if still empty
		if len(simTargets) == 0 {
			simTargets["Unknown"] = totalBacklog
			dist["Unknown"] = 1.0
		}

		// Determine if "Demand Expansion" is appropriate (3 Points).
		expansionEnabled := false
		expansionReason := ""

		if len(targets) > 0 {
			expansionEnabled = true
			expansionReason = "you provided a specific set of target counts"
		} else if len(issueTypes) > 0 {
			expansionEnabled = true
			expansionReason = "you requested a forecast for a specific subset of work item types"
		} else if additionalItems > 0 {
			expansionEnabled = true
			expansionReason = "you added new items to the backlog; we modeled the associated background work"
		}

		resObj := engine.RunMultiTypeDurationSimulation(simTargets, dist, 1000, expansionEnabled)

		// Transparent Expansion Messaging
		totalBG := 0
		for _, c := range resObj.BackgroundItemsPredicted {
			totalBG += c
		}

		if expansionEnabled && totalBG > 0 {
			msg := fmt.Sprintf("Because %s, we added %d background items to the forecast to match your historical work item distribution.", expansionReason, totalBG)
			resObj.Insights = append(resObj.Insights, msg)
		}

		if includeWIP {
			deliveredFiltered := stats.FilterDelivered(finished, s.activeResolutions, s.activeMapping)
			cycleTimes := s.getCycleTimes(projectKey, boardID, deliveredFiltered, startStatus, endStatus, issueTypes)
			engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, totalBacklog)
			resObj.Composition = simulation.Composition{
				WIP:             wipCount,
				ExistingBacklog: existingBacklogCount,
				AdditionalItems: additionalItems,
				Total:           totalBacklog,
			}
		}

		// Add detailed background items predictions to insights if expansion was active
		if expansionEnabled && len(resObj.BackgroundItemsPredicted) > 0 {
			msg := "Background work details: "
			first := true
			for t, c := range resObj.BackgroundItemsPredicted {
				if c > 0 {
					if !first {
						msg += ", "
					}
					msg += fmt.Sprintf("%d %s", c, t)
					first = false
				}
			}
			resObj.Insights = append(resObj.Insights, msg)
		}

		resObj.Insights = s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)
		if qReq := s.getQualityWarnings(append(finished, append(downstream, append(upstream, demand...)...)...)); qReq != "" {
			resObj.Warnings = append(resObj.Warnings, qReq)
		}
		return resObj, nil

	default:
		if targetDays > 0 || targetDate != "" {
			return s.handleRunSimulation(projectKey, boardID, "scope", false, 0, targetDays, targetDate, "", "", nil, false, sampleDays, sampleStartDate, sampleEndDate, targets, mixOverrides)
		}
		return nil, fmt.Errorf("mode required: 'duration' (backlog forecast) or 'scope' (volume forecast)")
	}
}

func (s *Server) addCommitmentInsights(insights []string, analysisCtx *AnalysisContext, explicitStart string) []string {
	if explicitStart != "" {
		insights = append(insights, fmt.Sprintf("Analysis uses EXPLICIT commitment point: '%s'.", explicitStart))
	} else if analysisCtx.CommitmentPointIsDefault {
		insights = append(insights, fmt.Sprintf("IMPORTANT: Analysis uses DEFAULT commitment point: '%s' (First Downstream status).", analysisCtx.CommitmentPoint))
	} else {
		insights = append(insights, "CAUTION: Analysis uses NO commitment point. Lifecycle timing starts from Creation.")
	}
	return insights
}

func (s *Server) handleGetCycleTimeAssessment(projectKey string, boardID int, analyzeWIP bool, startStatus, endStatus string, issueTypes []string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// Ensure we are anchored before analysis
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// 2. Project on Demand
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Time{}, time.Now(), "day", cutoff)
	events := s.events.GetEventsInRange(sourceID, window.Start, window.End)
	finished, downstream, upstream, demand := eventlog.ProjectScope(events, window, s.activeCommitmentPoint, s.activeMapping, s.activeResolutions, issueTypes)
	delivered := stats.FilterDelivered(finished, s.activeResolutions, s.activeMapping)

	if len(delivered) == 0 {
		return nil, fmt.Errorf("no historical delivery data found (items may be abandoned or unmapped)")
	}

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, append(finished, append(downstream, append(upstream, demand...)...)...))
	if startStatus == "" {
		startStatus = analysisCtx.CommitmentPoint
	}

	cWeight := 2
	if startStatus != "" {
		if w, ok := analysisCtx.StatusWeights[startStatus]; ok {
			cWeight = w
		}
	}

	cycleTimes := s.getCycleTimes(projectKey, boardID, delivered, startStatus, endStatus, issueTypes)

	if len(cycleTimes) == 0 {
		return nil, fmt.Errorf("no resolved items found that passed the commitment point '%s'", startStatus)
	}

	var wipAges []float64
	wipIssues := stats.ApplyBackflowPolicy(downstream, analysisCtx.StatusWeights, cWeight)
	wipAges = s.calculateWIPAges(wipIssues, startStatus, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes)

	h := simulation.NewHistogram(finished, window.Start, window.End, issueTypes, analysisCtx.WorkflowMappings, s.activeResolutions)
	engine := simulation.NewEngine(h)
	resObj := engine.RunCycleTimeAnalysis(cycleTimes)
	if analyzeWIP {
		engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, 0)
	}

	resObj.Insights = s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)

	if qReq := s.getQualityWarnings(append(finished, append(downstream, append(upstream, demand...)...)...)); qReq != "" {
		resObj.Warnings = append(resObj.Warnings, qReq)
	}

	return resObj, nil
}

func (s *Server) handleGetForecastAccuracy(projectKey string, boardID int, mode string, itemsToForecast, forecastHorizon int, issueTypes []string, sampleDays int, sampleStartDate, sampleEndDate string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// Ensure we are anchored before analysis
	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// Determine Sampling Window
	histEnd := time.Now()
	if sampleEndDate != "" {
		if t, err := time.Parse("2006-01-02", sampleEndDate); err == nil {
			histEnd = t
		}
	}

	histStart := histEnd.AddDate(0, -6, 0) // Default 6 months
	if sampleStartDate != "" {
		if t, err := time.Parse("2006-01-02", sampleStartDate); err == nil {
			histStart = t
		}
	} else if sampleDays > 0 {
		histStart = histEnd.AddDate(0, 0, -sampleDays)
	}

	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}

	// Apply AnalysisWindow for temporal consistency and boundary snapping
	window := stats.NewAnalysisWindow(histStart, histEnd, "day", cutoff)
	events := s.events.GetEventsInRange(sourceID, window.Start, window.End)

	// Mapping
	// We pass the CURRENT mapping. This assumes mapping hasn't changed drastically.
	// Time-travel mapping is hard, so we stick to current mapping.
	mapping := s.activeMapping

	wfa := simulation.NewWalkForwardEngine(events, mapping, s.activeResolutions)

	// Default Parameters if not provided
	if forecastHorizon <= 0 {
		forecastHorizon = 14
	}
	if itemsToForecast <= 0 {
		itemsToForecast = 5 // Default batch size
	}

	lookback := int(histEnd.Sub(histStart).Hours() / 24)

	cfg := simulation.WalkForwardConfig{
		SourceID:        sourceID,
		SimulationMode:  mode,
		LookbackWindow:  lookback,
		StepSize:        14, // Every 2 weeks
		ForecastHorizon: forecastHorizon,
		ItemsToForecast: itemsToForecast,
		IssueTypes:      issueTypes,
		Resolutions:     s.activeResolutions,
	}

	res, err := wfa.Execute(cfg)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"accuracy":      res,
		"_data_quality": s.getQualityWarnings(wfa.GetAnalyzedIssues()),
		"_guidance": []string{
			"If accuracy is < 70%, users should be cautious with forecasts.",
			"Drift Detection stops the backtest to prevent comparing apples to oranges.",
			"This tool is computationally expensive; cache the result if possible.",
		},
	}, nil
}
