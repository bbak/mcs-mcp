package mcp

import (
	"fmt"
	"time"

	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/visuals"
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

	// 1. Determine Sampling Window
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

	// 2. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// 3. Project using AnalysisSession
	window := stats.NewAnalysisWindow(histStart, histEnd, "day", cutoff)
	session := stats.NewAnalysisSession(s.events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)

	all := session.GetAllIssues()
	wip := session.GetWIP()
	delivered := session.GetDelivered()

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)
	if startStatus == "" {
		startStatus = analysisCtx.CommitmentPoint
	}

	// Apply Backflow Policy weight
	cWeight := 2
	if startStatus != "" {
		if w, ok := analysisCtx.StatusWeights[startStatus]; ok {
			cWeight = w
		}
	}

	actualTargets := make(map[string]int)

	if len(targets) > 0 {
		for k, v := range targets {
			actualTargets[k] = v
		}
	} else {
		if includeExistingBacklog {
			// Backlog items (Demand + Upstream)
			for _, issue := range all {
				if m, ok := stats.GetMetadataRobust(s.activeMapping, issue.StatusID, issue.Status); ok && (m.Tier == "Demand" || m.Tier == "Upstream") {
					actualTargets[issue.IssueType]++
				}
			}
		}

		if includeWIP {
			wipIssues := stats.ApplyBackflowPolicy(wip, analysisCtx.StatusWeights, cWeight)
			for _, issue := range wipIssues {
				actualTargets[issue.IssueType]++
			}
		}

		if additionalItems > 0 {
			if len(issueTypes) == 1 {
				actualTargets[issueTypes[0]] += additionalItems
			} else {
				actualTargets["Unknown"] += additionalItems
			}
		}
	}

	h := simulation.NewHistogram(delivered, window.Start, window.End, issueTypes, analysisCtx.WorkflowMappings, s.activeResolutions)
	engine := simulation.NewEngine(h)

	// Distribution handling
	var dist map[string]float64
	if len(mixOverrides) > 0 {
		dist = mixOverrides
	} else if histDist, ok := h.Meta["type_distribution"].(map[string]float64); ok {
		dist = histDist
	}

	var res interface{}
	var runErr error

	switch mode {
	case "scope":
		finalDays := targetDays
		if targetDate != "" {
			t, err := time.Parse("2006-01-02", targetDate)
			if err != nil {
				return nil, fmt.Errorf("invalid target_date format: %w", err)
			}
			diff := time.Until(t)
			finalDays = int(diff.Hours() / 24)
		}

		resObj := engine.RunMultiTypeScopeSimulation(finalDays, 10000, issueTypes, dist, true)
		resObj.Insights = s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)
		resObj.Warnings = append(resObj.Warnings, s.getQualityWarnings(all)...)
		res = resObj

	case "duration":
		resObj := engine.RunMultiTypeDurationSimulation(actualTargets, dist, 1000, true)
		resObj.Insights = s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)
		resObj.Warnings = append(resObj.Warnings, s.getQualityWarnings(all)...)
		res = resObj
	}

	if s.enableMermaidCharts && res != nil {
		if resObj, ok := res.(simulation.Result); ok {
			resObj.VisualCDF = visuals.GenerateSimulationCDF(resObj.Percentiles, mode)
			res = resObj
		}
	}

	return res, runErr
}

func (s *Server) handleGetCycleTimeAssessment(projectKey string, boardID int, analyzeWIP bool, startStatus, endStatus string, issueTypes []string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Now().AddDate(0, 0, -26*7), time.Now(), "day", cutoff)
	session := stats.NewAnalysisSession(s.events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)

	delivered := session.GetDelivered()
	all := session.GetAllIssues()
	wip := session.GetWIP()

	if len(delivered) == 0 {
		return nil, fmt.Errorf("no historical delivery data found")
	}

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)
	if startStatus == "" {
		startStatus = analysisCtx.CommitmentPoint
	}

	cycleTimes, _ := s.getCycleTimes(projectKey, boardID, delivered, startStatus, endStatus, issueTypes)
	if len(cycleTimes) == 0 {
		return nil, fmt.Errorf("no cycle times found for criteria")
	}

	h := simulation.NewHistogram(delivered, window.Start, window.End, issueTypes, analysisCtx.WorkflowMappings, s.activeResolutions)
	engine := simulation.NewEngine(h)

	ctByType := s.getCycleTimesByType(projectKey, boardID, delivered, startStatus, endStatus, issueTypes)
	resObj := engine.RunCycleTimeAnalysis(cycleTimes, ctByType)

	if analyzeWIP {
		wipAges := s.calculateWIPAges(wip, startStatus, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes)
		engine.AnalyzeWIPStability(&resObj, wipAges, ctByType)
	}

	resObj.Insights = s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)
	resObj.Warnings = append(resObj.Warnings, s.getQualityWarnings(all)...)

	if s.enableMermaidCharts {
		resObj.VisualCDF = visuals.GenerateSimulationCDF(resObj.Percentiles, "duration")
	}

	return resObj, nil
}

func (s *Server) handleGetForecastAccuracy(projectKey string, boardID int, mode string, itemsToForecast, forecastHorizon int, issueTypes []string, sampleDays int, sampleStartDate, sampleEndDate string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	histEnd := time.Now()
	if sampleEndDate != "" {
		if t, err := time.Parse("2006-01-02", sampleEndDate); err == nil {
			histEnd = t
		}
	}

	histStart := histEnd.AddDate(0, -6, 0)
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

	window := stats.NewAnalysisWindow(histStart, histEnd, "day", cutoff)
	events := s.events.GetEventsInRange(sourceID, window.Start, window.End)

	wfa := simulation.NewWalkForwardEngine(events, s.activeMapping, s.activeResolutions)

	if forecastHorizon <= 0 {
		forecastHorizon = 14
	}

	if itemsToForecast <= 0 {
		session := stats.NewAnalysisSession(s.events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)
		delivered := session.GetDelivered()
		// Simple adaptive heuristic
		itemsToForecast = int(float64(len(delivered)) / 10.0 * 2.0)
		if itemsToForecast < 2 {
			itemsToForecast = 2
		}
	}

	cfg := simulation.WalkForwardConfig{
		SourceID:        sourceID,
		SimulationMode:  mode,
		LookbackWindow:  int(histEnd.Sub(histStart).Hours() / 24),
		StepSize:        7,
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
	}, nil
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
