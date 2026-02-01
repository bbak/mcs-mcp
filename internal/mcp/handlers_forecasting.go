package mcp

import (
	"fmt"
	"time"

	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"
)

func (s *Server) handleRunSimulation(sourceID, sourceType, mode string, includeExistingBacklog bool, additionalItems int, targetDays int, targetDate string, startStatus, endStatus string, issueTypes []string, includeWIP bool, resolutions []string, sampleDays int, sampleStartDate, sampleEndDate string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events, sourceID)

	if len(issues) == 0 {
		return nil, fmt.Errorf("no data found in the event log to base simulation on")
	}

	analysisCtx := s.prepareAnalysisContext(sourceID, issues)
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
	issues = stats.ApplyBackflowPolicy(issues, analysisCtx.StatusWeights, cWeight)

	var wipAges []float64
	wipCount := 0
	existingBacklogCount := 0

	if includeExistingBacklog {
		// Count backlog items (those in 'Demand' tier or not committed)
		for _, issue := range issues {
			if m, ok := analysisCtx.WorkflowMappings[issue.Status]; ok && m.Tier == "Demand" {
				existingBacklogCount++
			}
		}
	}

	if includeWIP {
		wipIssues := s.filterWIPIssues(issues, startStatus, analysisCtx.FinishedStatuses)
		wipIssues = stats.ApplyBackflowPolicy(wipIssues, analysisCtx.StatusWeights, cWeight)
		cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, resolutions)
		calcWipAges := s.calculateWIPAges(wipIssues, startStatus, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes)
		wipAges = calcWipAges
		wipCount = len(wipAges)
	}

	engine := simulation.NewEngine(nil)

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

		startTime := time.Now().AddDate(0, -6, 0)
		h := simulation.NewHistogram(issues, startTime, time.Now(), issueTypes, resolutions)
		engine = simulation.NewEngine(h)
		resObj := engine.RunScopeSimulation(finalDays, 10000)

		resObj.Insights = append(resObj.Insights, "Scope Interpretation: Forecast shows total items that will reach 'Done' status, including items currently in progress.")
		resObj.Insights = s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)

		if includeWIP {
			cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, resolutions)
			engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, 0)
			resObj.Composition = simulation.Composition{
				WIP:             wipCount,
				ExistingBacklog: existingBacklogCount,
				AdditionalItems: additionalItems,
				Total:           0,
			}
		}
		return resObj, nil

	case "duration":
		totalBacklog := additionalItems + existingBacklogCount
		if totalBacklog <= 0 {
			return nil, fmt.Errorf("no items to forecast (additional_items and existing backlog both 0)")
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

		h := simulation.NewHistogram(issues, histStart, histEnd, issueTypes, resolutions)
		engine = simulation.NewEngine(h)

		// Prepare targets and distribution
		targets := make(map[string]int)
		if len(issueTypes) > 0 {
			// Distribution: If multiple types requested, we split the backlog equally for now
			// or we could use the historical ratio within those types.
			// However, usually additionalItems refers to the primary type.
			for _, t := range issueTypes {
				targets[t] = totalBacklog / len(issueTypes)
			}
			// Add remainder to the first one
			if len(issueTypes) > 0 {
				targets[issueTypes[0]] += totalBacklog % len(issueTypes)
			}
		} else {
			// If no types specified, we use the historical distribution to fill the backlog
			// But usually duration simulation is "when will these X items be done".
			// Let's assume the backlog matches the historical distribution if not specified.
			if dist, ok := h.Meta["type_distribution"].(map[string]float64); ok {
				for t, p := range dist {
					targets[t] = int(float64(totalBacklog) * p)
				}
			} else {
				targets["Unknown"] = totalBacklog
			}
		}

		dist, _ := h.Meta["type_distribution"].(map[string]float64)
		resObj := engine.RunMultiTypeDurationSimulation(targets, dist, 10000)

		if includeWIP {
			cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, resolutions)
			engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, totalBacklog)
			resObj.Composition = simulation.Composition{
				WIP:             wipCount,
				ExistingBacklog: existingBacklogCount,
				AdditionalItems: additionalItems,
				Total:           totalBacklog,
			}
		}

		// Add "Approach B" insights
		if len(resObj.BackgroundItemsPredicted) > 0 {
			msg := "Demand Expansion Model: Based on historical distribution, in addition to your targets, the team is forecasted to finish: "
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
		return resObj, nil

	default:
		if targetDays > 0 || targetDate != "" {
			return s.handleRunSimulation(sourceID, sourceType, "scope", false, 0, targetDays, targetDate, "", "", nil, false, resolutions, sampleDays, sampleStartDate, sampleEndDate)
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

func (s *Server) handleGetCycleTimeAssessment(sourceID, sourceType string, analyzeWIP bool, startStatus, endStatus string, resolutions []string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())
	issues := s.reconstructIssues(events, sourceID)

	if len(issues) == 0 {
		return nil, fmt.Errorf("no historical data found in the event log to base assessment on")
	}

	analysisCtx := s.prepareAnalysisContext(sourceID, issues)
	if startStatus == "" {
		startStatus = analysisCtx.CommitmentPoint
	}

	cWeight := 2
	if startStatus != "" {
		if w, ok := analysisCtx.StatusWeights[startStatus]; ok {
			cWeight = w
		}
	}
	issues = stats.ApplyBackflowPolicy(issues, analysisCtx.StatusWeights, cWeight)
	cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, resolutions)

	if len(cycleTimes) == 0 {
		return nil, fmt.Errorf("no resolved items found that passed the commitment point '%s'", startStatus)
	}

	var wipAges []float64
	wipIssues := s.filterWIPIssues(issues, startStatus, analysisCtx.FinishedStatuses)
	wipIssues = stats.ApplyBackflowPolicy(wipIssues, analysisCtx.StatusWeights, cWeight)
	wipAges = s.calculateWIPAges(wipIssues, startStatus, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes)

	engine := simulation.NewEngine(nil)
	resObj := engine.RunCycleTimeAnalysis(cycleTimes)
	if analyzeWIP {
		engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, 0)
	}

	resObj.Insights = s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)

	return resObj, nil
}

func (s *Server) handleGetForecastAccuracy(sourceID, sourceType, mode string, itemsToForecast, forecastHorizon int, resolutions []string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	events := s.events.GetEventsInRange(sourceID, time.Time{}, time.Now())

	// Mapping
	// We pass the CURRENT mapping. This assumes mapping hasn't changed drastically.
	// Time-travel mapping is hard, so we stick to current mapping.
	mapping := s.workflowMappings[sourceID]

	wfa := simulation.NewWalkForwardEngine(events, mapping)

	// Default Parameters if not provided
	if forecastHorizon <= 0 {
		forecastHorizon = 14
	}
	if itemsToForecast <= 0 {
		itemsToForecast = 5 // Default batch size
	}

	if len(resolutions) == 0 {
		resolutions = s.getDeliveredResolutions(sourceID)
	}

	cfg := simulation.WalkForwardConfig{
		SourceID:        sourceID,
		SimulationMode:  mode,
		LookbackWindow:  90, // Check last 3 months
		StepSize:        14, // Every 2 weeks
		ForecastHorizon: forecastHorizon,
		ItemsToForecast: itemsToForecast,
		Resolutions:     resolutions,
	}

	res, err := wfa.Execute(cfg)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"accuracy": res,
		"_guidance": []string{
			"If accuracy is < 70%, users should be cautious with forecasts.",
			"Drift Detection stops the backtest to prevent comparing apples to oranges.",
			"This tool is computationally expensive; cache the result if possible.",
		},
	}, nil
}
