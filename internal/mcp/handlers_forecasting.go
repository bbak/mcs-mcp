package mcp

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/rs/zerolog/log"
	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/visuals"
)

func (s *Server) handleRunSimulation(projectKey string, boardID int, mode string, includeExistingBacklog bool, additionalItems int, targetDays int, targetDate string, startStatus, _ string, issueTypes []string, includeWIP bool, sampleDays int, sampleStartDate, sampleEndDate string, targets map[string]int, mixOverrides map[string]float64) (any, error) {
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
	histEnd := s.Clock()
	if sampleEndDate != "" {
		if t, err := time.Parse("2006-01-02", sampleEndDate); err == nil {
			histEnd = t
		} else {
			return nil, fmt.Errorf("invalid sample_end_date format: %w", err)
		}
	}
	histStart := histEnd.AddDate(0, 0, -90) // Default 90 days
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
	reg, err := s.events.Hydrate(sourceID, projectKey, ctx.JQL, s.activeRegistry)
	if err != nil {
		return nil, err
	}
	s.activeRegistry = reg
	if err := s.saveWorkflow(projectKey, boardID); err != nil {
		log.Warn().Err(err).Msg("Failed to persist workflow metadata to disk")
	}

	// 3. Project using AnalysisSession
	window := stats.NewAnalysisWindow(histStart, histEnd, "day", cutoff)
	events := s.events.GetIssuesInRange(sourceID, window.Start, window.End)
	session := stats.NewAnalysisSession(events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)

	all := session.GetAllIssues()
	wip := session.GetWIP()
	finished := session.GetFinished()

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
	var backlogCount, wipCount int

	if len(targets) > 0 {
		for k, v := range targets {
			actualTargets[k] = v
		}
	} else {
		if includeExistingBacklog {
			// Backlog items (Demand + Upstream)
			for _, issue := range all {
				if m, ok := s.activeMapping[issue.StatusID]; ok && (m.Tier == "Demand" || m.Tier == "Upstream") {
					actualTargets[issue.IssueType]++
					backlogCount++
				}
			}
		}

		if includeWIP {
			wipIssues := wip
			if s.commitmentBackflowReset {
				wipIssues = stats.ApplyBackflowPolicy(wip, analysisCtx.StatusWeights, cWeight, s.Clock())
			}
			for _, issue := range wipIssues {
				actualTargets[issue.IssueType]++
				wipCount++
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

	// 4. Stationarity assessment via residence time analysis
	var stationarityAssessment *stats.StationarityAssessment
	if analysisCtx.CommitmentPoint != "" {
		rtItems := stats.ExtractResidenceItems(all, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, window.Start)
		if len(rtItems) > 0 {
			rtResult := stats.ComputeResidenceTimeSeries(rtItems, window)
			stationarityAssessment = stats.AssessStationarity(rtResult)
		}
	}

	log.Info().
		Str("tool", "forecast_monte_carlo").
		Bool("experimental", s.experimentalMode).
		Bool("gate_open", s.allowExperimental).
		Msg("tool executed")

	// [Experimental] SPA Pipeline: Steps 1-4 (pre-histogram)
	var spaDiagnostics *stats.SPAPipelineResult
	if s.experimentalMode && analysisCtx.CommitmentPoint != "" {
		// Run SPA over full history (same approach as handleAnalyzeResidenceTime)
		fullWindow := stats.NewAnalysisWindow(time.Time{}, s.Clock(), "day", cutoff)
		fullEvents := s.events.GetIssuesInRange(sourceID, fullWindow.Start, fullWindow.End)
		fullSession := stats.NewAnalysisSession(fullEvents, sourceID, *ctx, s.activeMapping, s.activeResolutions, fullWindow)
		fullAll := fullSession.GetAllIssues()

		spaItems := stats.ExtractResidenceItems(fullAll, analysisCtx.CommitmentPoint,
			analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, fullWindow.Start)

		if len(spaItems) > 0 {
			spaRT := stats.ComputeResidenceTimeSeries(spaItems, fullWindow)
			spaCfg := stats.DefaultSPAPipelineConfig()

			spaDiagnostics = stats.RunSPAPipeline(
				spaRT, spaItems, finished,
				window.Start, window.End,
				analysisCtx.CommitmentPoint,
				analysisCtx.StatusWeights,
				analysisCtx.WorkflowMappings,
				window, spaCfg,
			)

			// Override finished issues and window with pipeline output
			finished = spaDiagnostics.FilteredFinished
			histStart = spaDiagnostics.EffectiveStart
			window = stats.NewAnalysisWindow(histStart, histEnd, "day", cutoff)

			log.Info().
				Str("tool", "forecast_monte_carlo").
				Time("effective_start", spaDiagnostics.EffectiveStart).
				Int("outliers_removed", spaDiagnostics.OutlierCount).
				Str("convergence", spaDiagnostics.ConvergenceStatus).
				Msg("SPA pipeline applied")
		}
	}

	h := simulation.NewHistogram(finished, window.Start, window.End, issueTypes, analysisCtx.WorkflowMappings, s.activeResolutions)

	// [Experimental] SPA Pipeline: Step 5 (post-histogram WIP/aging resampling)
	if s.experimentalMode && spaDiagnostics != nil {
		spaCfg := stats.DefaultSPAPipelineConfig()
		if tpOverall, ok := h.Meta["throughput_overall"].(float64); ok && tpOverall > 0 {
			wipScale := stats.ComputeWIPScaleFactor(spaDiagnostics.RTSummary, tpOverall, spaCfg)
			combinedFactor := wipScale * spaDiagnostics.DivergenceScaleFactor
			if combinedFactor != 1.0 {
				// Use simulation seed for deterministic resampling in tests
				resampleRNG := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
				if s.simulationSeed != 0 {
					resampleRNG = rand.New(rand.NewPCG(uint64(s.simulationSeed), 1))
				}
				h.Resample(combinedFactor, resampleRNG)
				spaDiagnostics.WIPScaleFactor = combinedFactor
			}
		}
	}

	engine := simulation.NewEngine(h)
	if s.simulationSeed != 0 {
		engine.SetSeed(s.simulationSeed)
	}

	// Distribution handling
	var dist map[string]float64
	if len(mixOverrides) > 0 {
		dist = mixOverrides
	} else if histDist, ok := h.Meta["type_distribution"].(map[string]float64); ok {
		dist = histDist
	}

	var res any
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
		resObj.Round()
		resObj.Insights = s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)
		resObj.Warnings = append(resObj.Warnings, s.getQualityWarnings(all)...)
		s.applyStationarity(&resObj, stationarityAssessment)
		resObj.Composition = &simulation.Composition{
			ExistingBacklog: backlogCount,
			WIP:             wipCount,
			AdditionalItems: additionalItems,
			Total:           backlogCount + wipCount + additionalItems,
		}
		res = resObj

	case "duration":
		resObj := engine.RunMultiTypeDurationSimulation(actualTargets, dist, 1000, true)
		resObj.Round()
		resObj.Insights = s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)
		resObj.Warnings = append(resObj.Warnings, s.getQualityWarnings(all)...)
		s.applyStationarity(&resObj, stationarityAssessment)
		resObj.Composition = &simulation.Composition{
			ExistingBacklog: backlogCount,
			WIP:             wipCount,
			AdditionalItems: additionalItems,
			Total:           backlogCount + wipCount + additionalItems,
		}
		res = resObj
	}

	// [Experimental] Attach SPA pipeline diagnostics to the result
	if s.experimentalMode && spaDiagnostics != nil {
		if resObj, ok := res.(simulation.Result); ok {
			if resObj.Context == nil {
				resObj.Context = make(map[string]any)
			}
			resObj.Context["spa_pipeline"] = spaDiagnostics
			if spaDiagnostics.DivergenceWarning != "" {
				resObj.Warnings = append(resObj.Warnings, spaDiagnostics.DivergenceWarning)
			}
			boundaryStr := "no"
			if spaDiagnostics.RegimeBoundaryRespected {
				boundaryStr = "yes"
			}
			resObj.Insights = append(resObj.Insights, fmt.Sprintf(
				"[Experimental] Adaptive window: %d departures over %d days (window: %s to %s, regime boundary respected: %s), %d outliers removed, convergence: %s, WIP scale: %.2f. Re-run without experimental mode to compare.",
				spaDiagnostics.AdaptiveWindowDepartures,
				spaDiagnostics.AdaptiveWindowDays,
				spaDiagnostics.EffectiveStart.Format("2006-01-02"),
				spaDiagnostics.EffectiveEnd.Format("2006-01-02"),
				boundaryStr,
				spaDiagnostics.OutlierCount,
				spaDiagnostics.ConvergenceStatus,
				spaDiagnostics.WIPScaleFactor,
			))
			res = resObj
		}
	}

	if s.enableMermaidCharts && res != nil {
		if resObj, ok := res.(simulation.Result); ok {
			resObj.VisualCDF = visuals.GenerateSimulationCDF(resObj.Percentiles, mode)
			res = resObj
		}
	}

	var warnings []string
	var insights []string

	if resObj, ok := res.(simulation.Result); ok {
		warnings = resObj.Warnings
		insights = resObj.Insights
		resObj.Warnings = nil
		resObj.Insights = nil
		res = resObj
	}

	return WrapResponse(res, projectKey, boardID, nil, warnings, insights), runErr
}

func (s *Server) handleGetCycleTimeAssessment(projectKey string, boardID int, startStatus, endStatus string, issueTypes []string) (any, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	reg, err := s.events.Hydrate(sourceID, projectKey, ctx.JQL, s.activeRegistry)
	if err != nil {
		return nil, err
	}
	s.activeRegistry = reg
	if err := s.saveWorkflow(projectKey, boardID); err != nil {
		log.Warn().Err(err).Msg("Failed to persist workflow metadata to disk")
	}

	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(s.Clock().AddDate(0, 0, -26*7), s.Clock(), "day", cutoff)
	events := s.events.GetIssuesInRange(sourceID, window.Start, window.End)
	session := stats.NewAnalysisSession(events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)

	delivered := session.GetDelivered()
	finished := session.GetFinished()
	all := session.GetAllIssues()

	if len(delivered) == 0 {
		return nil, fmt.Errorf("no historical delivery data found")
	}

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)
	if startStatus == "" {
		startStatus = analysisCtx.CommitmentPoint
	}

	cycleTimes, matchedIssues := s.getCycleTimes(projectKey, boardID, delivered, startStatus, endStatus, issueTypes)
	if len(cycleTimes) == 0 {
		return nil, fmt.Errorf("no cycle times found for criteria")
	}

	scatterplot := stats.BuildScatterplot(matchedIssues, cycleTimes)

	h := simulation.NewHistogram(finished, window.Start, window.End, issueTypes, analysisCtx.WorkflowMappings, s.activeResolutions)
	engine := simulation.NewEngine(h)
	if s.simulationSeed != 0 {
		engine.SetSeed(s.simulationSeed)
	}

	ctByType := s.getCycleTimesByType(projectKey, boardID, delivered, startStatus, endStatus, issueTypes)
	resObj := engine.RunCycleTimeAnalysis(cycleTimes, ctByType)
	resObj.Round()
	resObj.Scatterplot = scatterplot

	warnings := append(resObj.Warnings, s.getQualityWarnings(all)...)
	insights := s.addCommitmentInsights(resObj.Insights, analysisCtx, startStatus)

	// Clear from the nested object to avoid duplication
	resObj.Warnings = nil
	resObj.Insights = nil

	if s.enableMermaidCharts {
		resObj.VisualCDF = visuals.GenerateSimulationCDF(resObj.Percentiles, "duration")
	}

	return WrapResponse(resObj, projectKey, boardID, nil, warnings, insights), nil
}

func (s *Server) handleGetForecastAccuracy(projectKey string, boardID int, mode string, itemsToForecast, forecastHorizon int, issueTypes []string, sampleDays int, sampleStartDate, sampleEndDate string) (any, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	if err := s.anchorContext(projectKey, boardID); err != nil {
		return nil, err
	}

	reg, err := s.events.Hydrate(sourceID, projectKey, ctx.JQL, s.activeRegistry)
	if err != nil {
		return nil, err
	}
	s.activeRegistry = reg
	if err := s.saveWorkflow(projectKey, boardID); err != nil {
		log.Warn().Err(err).Msg("Failed to persist workflow metadata to disk")
	}

	const walkForwardStepDays = 7
	const walkForwardSteps = 25
	const walkForwardDefaultLookback = walkForwardStepDays * walkForwardSteps // 175 days → 25 checkpoints

	histEnd := s.Clock()
	if sampleEndDate != "" {
		if t, err := time.Parse("2006-01-02", sampleEndDate); err == nil {
			histEnd = t
		}
	}

	// LookbackWindow: how far back the outer iteration runs.
	// Default: stepSize × 25 = 175 days, always yielding 25 checkpoints. User can override.
	lookbackDays := walkForwardDefaultLookback
	histStart := histEnd.AddDate(0, 0, -lookbackDays)
	if sampleStartDate != "" {
		if t, err := time.Parse("2006-01-02", sampleStartDate); err == nil {
			histStart = t
			lookbackDays = int(histEnd.Sub(histStart).Hours() / 24)
		}
	} else if sampleDays > 0 {
		histStart = histEnd.AddDate(0, 0, -sampleDays)
		lookbackDays = sampleDays
	}

	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}

	// Load events from the global maximum lookback so that each checkpoint's
	// 90-day per-checkpoint histogram always has backing data. The earliest
	// checkpoint is at histStart; its histogram reaches back another 90 days.
	eventsStart := histStart.AddDate(0, 0, -90)
	if s.activeDiscoveryCutoff != nil && eventsStart.Before(*s.activeDiscoveryCutoff) {
		eventsStart = *s.activeDiscoveryCutoff
	}
	events := s.events.GetIssuesInRange(sourceID, eventsStart, histEnd)

	wfa := simulation.NewWalkForwardEngine(events, s.activeMapping, s.activeResolutions)

	if forecastHorizon <= 0 {
		forecastHorizon = 14
	}

	if itemsToForecast <= 0 {
		window := stats.NewAnalysisWindow(histStart, histEnd, "day", cutoff)
		session := stats.NewAnalysisSession(events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)
		delivered := session.GetDelivered()
		// Simple adaptive heuristic
		itemsToForecast = int(float64(len(delivered)) / 10.0 * 2.0)
		if itemsToForecast < 2 {
			itemsToForecast = 2
		}
	}

	// Prepare commitment point info for stationarity tracking
	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, nil)

	cfg := simulation.WalkForwardConfig{
		SourceID:         sourceID,
		SimulationMode:   mode,
		LookbackWindow:   lookbackDays,
		StepSize:         walkForwardStepDays,
		ForecastHorizon:  forecastHorizon,
		ItemsToForecast:  itemsToForecast,
		IssueTypes:       issueTypes,
		Resolutions:      s.activeResolutions,
		EvaluationDate:   s.Clock(),
		CommitmentPoint:  analysisCtx.CommitmentPoint,
		StatusWeights:    analysisCtx.StatusWeights,
		ExperimentalMode: s.experimentalMode,
	}

	res, err := wfa.Execute(cfg)
	if err != nil {
		return nil, err
	}

	resMap := map[string]any{
		"accuracy": res,
	}

	return WrapResponse(resMap, projectKey, boardID, nil, s.getQualityWarnings(wfa.GetAnalyzedIssues()), nil), nil
}

// applyStationarity injects stationarity warnings and insights into a simulation result.
func (s *Server) applyStationarity(resObj *simulation.Result, assessment *stats.StationarityAssessment) {
	if assessment == nil {
		return
	}
	resObj.Warnings = append(resObj.Warnings, assessment.Warnings...)
	if assessment.RecommendedWindowDays != nil {
		resObj.Insights = append(resObj.Insights, fmt.Sprintf(
			"RECOMMENDATION: Consider narrowing the sampling window to %d days (%s). Re-run with sample_days=%d.",
			*assessment.RecommendedWindowDays, assessment.WindowRationale, *assessment.RecommendedWindowDays))
	}
	if resObj.Context == nil {
		resObj.Context = make(map[string]any)
	}
	resObj.Context["stationarity_assessment"] = assessment
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
