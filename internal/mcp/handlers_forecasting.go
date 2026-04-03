package mcp

import (
	"fmt"
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
		if t, err := time.Parse(stats.DateFormat, sampleEndDate); err == nil {
			histEnd = t
		} else {
			return nil, fmt.Errorf("invalid sample_end_date format: %w", err)
		}
	}
	histStart := histEnd.AddDate(0, 0, -DefaultForecastSampleDays) // Default 90 days
	if sampleStartDate != "" {
		if t, err := time.Parse(stats.DateFormat, sampleStartDate); err == nil {
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
		Str("engine", s.engineName).
		Msg("tool executed")

	// Resolve target days for scope mode
	finalTargetDays := targetDays
	if mode == "scope" && targetDate != "" {
		t, err := time.Parse(stats.DateFormat, targetDate)
		if err != nil {
			return nil, fmt.Errorf("invalid target_date format: %w", err)
		}
		finalTargetDays = stats.CalendarDaysBetween(s.Clock(), t)
	}

	// Build ForecastRequest
	req := simulation.ForecastRequest{
		Mode:             mode,
		AllIssues:        all,
		Finished:         finished,
		WIP:              wip,
		WindowStart:      window.Start,
		WindowEnd:        window.End,
		DiscoveryCutoff:  cutoff,
		Targets:          actualTargets,
		MixOverrides:     mixOverrides,
		TargetDays:       finalTargetDays,
		IssueTypes:       issueTypes,
		CommitmentPoint:  analysisCtx.CommitmentPoint,
		StatusWeights:    analysisCtx.StatusWeights,
		WorkflowMappings: analysisCtx.WorkflowMappings,
		Resolutions:      s.activeResolutions,
		SimulationSeed:   s.simulationSeed,
		Clock:            s.Clock(),
	}

	// Resolve engine
	selectedEngine, err := s.resolveEngine(req)
	if err != nil {
		return nil, fmt.Errorf("engine resolution failed: %w", err)
	}

	resObj, err := selectedEngine.Run(req)
	if err != nil {
		return nil, fmt.Errorf("simulation failed: %w", err)
	}

	// Post-processing (shared across all engines)
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

	if resObj.Context == nil {
		resObj.Context = make(map[string]any)
	}
	resObj.Context["simulation_mode"] = mode
	if mode == "scope" {
		resObj.Context["target_days"] = finalTargetDays
	}

	if s.enableMermaidCharts {
		resObj.VisualCDF = visuals.GenerateSimulationCDF(resObj.Percentiles, mode)
	}

	warnings := resObj.Warnings
	insights := resObj.Insights
	resObj.Warnings = nil
	resObj.Insights = nil

	return WrapResponse(resObj, projectKey, boardID, nil, warnings, insights), nil
}

// resolveEngine returns the engine to use for a given forecast request.
// For "auto" mode, it runs a walk-forward backtest with all enabled engines
// and selects the best one. For named engines, it does a direct lookup.
func (s *Server) resolveEngine(req simulation.ForecastRequest) (simulation.ForecastEngine, error) {
	if s.engineName != "auto" {
		return s.engineRegistry.Get(s.engineName)
	}

	// Auto mode: backtest all enabled engines and pick the best
	engines := s.engineRegistry.Enabled(s.engineWeights)
	if len(engines) == 0 {
		return nil, fmt.Errorf("no engines enabled (all weights are 0)")
	}
	if len(engines) == 1 {
		return engines[0], nil
	}

	// Defer to multi-engine backtest (Step 8)
	result, err := s.runMultiEngineBacktest(req, engines)
	if err != nil {
		// Fallback to crude on backtest failure
		log.Warn().Err(err).Msg("auto engine selection failed, falling back to crude")
		return s.engineRegistry.Get("crude")
	}

	selected, err := s.engineRegistry.Get(result.Selected)
	if err != nil {
		return nil, err
	}

	log.Info().
		Str("selected_engine", result.Selected).
		Str("reason", result.Reason).
		Msg("auto engine selection complete")

	return selected, nil
}

// runMultiEngineBacktest uses the walk-forward engine to compare multiple engines
// on the currently active source and returns the selection result.
func (s *Server) runMultiEngineBacktest(req simulation.ForecastRequest, engines []simulation.ForecastEngine) (*simulation.MultiEngineResult, error) {
	sourceID := s.activeSourceID

	const walkForwardStepDays = 7
	const walkForwardSteps = 25
	lookbackDays := walkForwardStepDays * walkForwardSteps

	histEnd := req.Clock
	histStart := histEnd.AddDate(0, 0, -lookbackDays)
	eventsStart := histStart.AddDate(0, 0, -DefaultForecastSampleDays)

	if s.activeDiscoveryCutoff != nil && eventsStart.Before(*s.activeDiscoveryCutoff) {
		eventsStart = *s.activeDiscoveryCutoff
	}

	events := s.events.GetIssuesInRange(sourceID, eventsStart, histEnd)
	wfa := simulation.NewWalkForwardEngine(events, s.activeMapping, s.activeResolutions)

	// Use scope mode with 14-day horizon for backtest comparison
	cfg := simulation.WalkForwardConfig{
		SourceID:        sourceID,
		SimulationMode:  "scope",
		LookbackWindow:  lookbackDays,
		StepSize:        walkForwardStepDays,
		ForecastHorizon: 14,
		Resolutions:     s.activeResolutions,
		EvaluationDate:  req.Clock,
		CommitmentPoint: req.CommitmentPoint,
		StatusWeights:   req.StatusWeights,
		SimulationSeed:  s.simulationSeed,
	}

	return wfa.ExecuteMultiEngine(cfg, engines, s.engineWeights)
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
	window := stats.NewAnalysisWindow(s.Clock().AddDate(0, 0, -BaselineWindowWeeks*7), s.Clock(), "day", cutoff)
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
		if t, err := time.Parse(stats.DateFormat, sampleEndDate); err == nil {
			histEnd = t
		}
	}

	// LookbackWindow: how far back the outer iteration runs.
	// Default: stepSize × 25 = 175 days, always yielding 25 checkpoints. User can override.
	lookbackDays := walkForwardDefaultLookback
	histStart := histEnd.AddDate(0, 0, -lookbackDays)
	if sampleStartDate != "" {
		if t, err := time.Parse(stats.DateFormat, sampleStartDate); err == nil {
			histStart = t
			lookbackDays = stats.CalendarDaysBetween(histStart, histEnd)
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
	eventsStart := histStart.AddDate(0, 0, -DefaultForecastSampleDays)
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
