package mcp

import (
	"fmt"
	"time"

	"mcs-mcp/internal/simulation"
)

func (s *Server) handleRunSimulation(sourceID, sourceType, mode string, includeExistingBacklog bool, additionalItems int, targetDays int, targetDate string, startStatus, endStatus string, issueTypes []string, includeWIP bool, resolutions []string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	issues, _, err := s.ingestHistoricalIssues(sourceID, ctx, 6)
	if err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("no historical data found in the last 6 months to base simulation on")
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
	issues = s.applyBackflowPolicy(issues, analysisCtx.StatusWeights, cWeight)

	var wipAges []float64
	wipCount := 0
	existingBacklogCount := 0

	if includeExistingBacklog {
		backlogIssues, err := s.ingestWIPIssues(sourceID, ctx, false)
		if err == nil {
			existingBacklogCount = len(backlogIssues)
			issues = append(issues, backlogIssues...)
		}
	}

	if includeWIP {
		wipIssues, err := s.ingestWIPIssues(sourceID, ctx, true)
		if err == nil {
			wipIssues = s.applyBackflowPolicy(wipIssues, analysisCtx.StatusWeights, cWeight)
			cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, analysisCtx.StatusWeights, resolutions)
			calcWipAges := s.calculateWIPAges(wipIssues, startStatus, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes)
			wipAges = calcWipAges
			wipCount = len(wipAges)
		}
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

		if includeWIP {
			cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, analysisCtx.StatusWeights, resolutions)
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

		startTime := time.Now().AddDate(0, -6, 0)
		h := simulation.NewHistogram(issues, startTime, time.Now(), issueTypes, resolutions)
		engine = simulation.NewEngine(h)
		resObj := engine.RunDurationSimulation(totalBacklog, 10000)

		if includeWIP {
			cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, analysisCtx.StatusWeights, resolutions)
			engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, totalBacklog)
			resObj.Composition = simulation.Composition{
				WIP:             wipCount,
				ExistingBacklog: existingBacklogCount,
				AdditionalItems: additionalItems,
				Total:           totalBacklog,
			}
		}
		return resObj, nil

	default:
		if targetDays > 0 || targetDate != "" {
			return s.handleRunSimulation(sourceID, sourceType, "scope", false, 0, targetDays, targetDate, "", "", nil, false, resolutions)
		}
		return nil, fmt.Errorf("mode required: 'duration' (backlog forecast) or 'scope' (volume forecast)")
	}
}

func (s *Server) handleGetCycleTimeAssessment(sourceID, sourceType string, issueTypes []string, analyzeWIP bool, startStatus, endStatus string, resolutions []string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	issues, _, err := s.ingestHistoricalIssues(sourceID, ctx, 6)
	if err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("no historical data found in the last 6 months to base assessment on")
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
	issues = s.applyBackflowPolicy(issues, analysisCtx.StatusWeights, cWeight)
	cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, analysisCtx.StatusWeights, resolutions)

	if len(cycleTimes) == 0 {
		return nil, fmt.Errorf("no resolved items found that passed the commitment point '%s'", startStatus)
	}

	var wipAges []float64
	if analyzeWIP {
		wipIssues, err := s.ingestWIPIssues(sourceID, ctx, true)
		if err == nil {
			wipIssues = s.applyBackflowPolicy(wipIssues, analysisCtx.StatusWeights, cWeight)
			wipAges = s.calculateWIPAges(wipIssues, startStatus, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes)
		}
	}

	engine := simulation.NewEngine(nil)
	resObj := engine.RunCycleTimeAnalysis(cycleTimes)
	if analyzeWIP {
		engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, 0)
	}

	return resObj, nil
}
