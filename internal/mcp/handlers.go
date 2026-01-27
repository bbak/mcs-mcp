package mcp

import (
	"fmt"
	"math"
	"strings"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

func (s *Server) handleGetDataMetadata(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Probe: Fetch 50 items and history
	response, err := s.jira.SearchIssuesWithHistory(ctx.JQL, 0, 50)
	if err != nil {
		return nil, err
	}

	// Process DTOs into Domain Issues
	issues := make([]jira.Issue, len(response.Issues))
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto)
	}

	summary := stats.AnalyzeProbe(issues, response.Total)

	// Enrich with metadata
	projectKey := ctx.PrimaryProject
	if projectKey == "" && len(issues) > 0 {
		projectKey = issues[0].ProjectKey
	}

	if projectKey != "" {
		statuses, err := s.jira.GetProjectStatuses(projectKey)
		if err != nil {
			log.Error().Err(err).Str("project", projectKey).Msg("Failed to fetch project statuses")
		}
		summary.AvailableStatuses = statuses
		statusWeights := s.getStatusWeights(projectKey)
		summary.CommitmentPointHints = s.getCommitmentPointHints(issues, statusWeights)
	}

	return summary, nil
}

func (s *Server) handleRunSimulation(sourceID, sourceType, mode string, includeExistingBacklog bool, additionalItems int, targetDays int, targetDate string, startStatus, endStatus string, issueTypes []string, includeWIP bool, resolutions []string) (interface{}, error) {
	// 1. Get Source Context
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// 2. Ingestion: Fetch last 6 months of historical data
	startTime := time.Now().AddDate(0, -6, 0)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		ctx.JQL, startTime.Format("2006-01-02"))

	log.Debug().Str("jql", ingestJQL).Msg("Starting historical ingestion for simulation")

	// Use history for all simulations to ensure Stability Analysis has cycle time/residency data
	response, err := s.jira.SearchIssuesWithHistory(ingestJQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	if response.Total == 0 {
		return nil, fmt.Errorf("no historical data found in the last 6 months to base simulation on")
	}

	// Process DTOs into Domain Issues
	issues := make([]jira.Issue, len(response.Issues))
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto)
	}

	// 3. Analytics Context (WIP Aging & Status Weights)
	projectKey := ctx.PrimaryProject
	if projectKey == "" && len(issues) > 0 {
		projectKey = issues[0].ProjectKey
	}
	statusWeights := s.getStatusWeights(projectKey)
	// Override weights with verified mappings if available to ensure correct backflow detection
	if m, ok := s.workflowMappings[sourceID]; ok {
		for name, metadata := range m {
			if metadata.Tier == "Demand" {
				statusWeights[name] = 1
			} else if metadata.Tier == "Downstream" || metadata.Tier == "Finished" {
				if statusWeights[name] < 2 {
					statusWeights[name] = 2
				}
			}
		}
	}

	// Apply Backflow Policy (Discard pre-backflow history)
	cWeight := 2
	if startStatus != "" {
		if w, ok := statusWeights[startStatus]; ok {
			cWeight = w
		}
	}
	issues = s.applyBackflowPolicy(issues, statusWeights, cWeight)
	var wipAges []float64
	wipCount := 0

	// 5. Backlog Ingestion (if needed)
	existingBacklogCount := 0
	if includeExistingBacklog {
		backlogJQL := fmt.Sprintf("(%s) AND resolution is EMPTY", ctx.JQL)
		response, err := s.jira.SearchIssues(backlogJQL, 0, 1000)
		if err != nil {
			return nil, err
		}
		existingBacklogCount = len(response.Issues)
		for _, dto := range response.Issues {
			issues = append(issues, stats.MapIssue(dto))
		}
	}

	// 6. WIP Ingestion (if needed)
	if includeWIP {
		wipJQL := fmt.Sprintf("(%s) AND resolution is EMPTY", ctx.JQL)
		wipIssuesResponse, err := s.jira.SearchIssuesWithHistory(wipJQL, 0, 1000)
		if err == nil {
			wipIssues := make([]jira.Issue, len(wipIssuesResponse.Issues))
			for i, dto := range wipIssuesResponse.Issues {
				wipIssues[i] = stats.MapIssue(dto)
			}
			wipIssues = s.applyBackflowPolicy(wipIssues, statusWeights, cWeight)
			cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, statusWeights, resolutions)
			calcWipAges := stats.CalculateInventoryAge(wipIssues, startStatus, statusWeights, cycleTimes, "wip")
			for _, wa := range calcWipAges {
				if wa.AgeDays != nil {
					wipAges = append(wipAges, *wa.AgeDays)
					wipCount++
				}
			}
		}
	}

	// 4. Mode Selection
	engine := simulation.NewEngine(nil)

	switch mode {
	case "single":
		log.Info().Str("startStatus", startStatus).Msg("Running Cycle Time Analysis (Single Item)")

		projectKey := ""
		if len(issues) > 0 {
			parts := strings.Split(issues[0].Key, "-")
			if len(parts) > 1 {
				projectKey = parts[0]
			}
		}

		statusWeights := s.getStatusWeights(projectKey)
		cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, statusWeights, resolutions)

		if len(cycleTimes) == 0 {
			msg := fmt.Sprintf("no resolved items found that passed the commitment point '%s'.", startStatus)
			hints := s.getCommitmentPointHints(issues, statusWeights)
			if len(hints) > 0 {
				msg += "\n\n💡 Hint: Based on historical reachability, these statuses were frequently used as work started: [" + strings.Join(hints, ", ") + "].\n(⚠️ Note: These are inferred from status categories and transition history; please verify if they represent your actual commitment point.)"
			}
			return nil, fmt.Errorf("%s", msg)
		}
		engine = simulation.NewEngine(&simulation.Histogram{})
		resObj := engine.RunCycleTimeAnalysis(cycleTimes)
		if includeWIP {
			engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, 0)
		}
		return resObj, nil

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

		log.Info().Int("days", finalDays).Any("types", issueTypes).Msg("Running Scope Simulation")
		h := simulation.NewHistogram(issues, startTime, time.Now(), issueTypes, resolutions)
		engine = simulation.NewEngine(h)
		resObj := engine.RunScopeSimulation(finalDays, 10000)

		resObj.Insights = append(resObj.Insights, "Scope Interpretation: Forecast shows total items that will reach 'Done' status, including items currently in progress.")

		if includeWIP {
			cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, statusWeights, resolutions)
			engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, 0)
			resObj.Composition = simulation.Composition{
				WIP:             wipCount,
				ExistingBacklog: existingBacklogCount,
				AdditionalItems: additionalItems,
				Total:           0, // Total doesn't make sense as input in Scope mode
			}
		}

		resObj.Context = map[string]interface{}{
			"forecast_window_days": finalDays,
			"target_date":          targetDate,
		}
		return resObj, nil

	case "duration":
		if !includeExistingBacklog && additionalItems <= 0 && !includeWIP {
			return nil, fmt.Errorf("either include_existing_backlog: true, additional_items > 0, OR include_wip: true must be provided for duration simulation")
		}

		actualBacklog := existingBacklogCount + additionalItems + wipCount
		log.Info().Int("total", actualBacklog).Int("backlog", existingBacklogCount).Int("additional", additionalItems).Int("wip", wipCount).Any("types", issueTypes).Msg("Running Duration Simulation")

		h := simulation.NewHistogram(issues, startTime, time.Now(), issueTypes, resolutions)
		engine = simulation.NewEngine(h)
		resObj := engine.RunDurationSimulation(actualBacklog, 10000)

		// Set Scope Composition for AI transparency
		resObj.Composition = simulation.Composition{
			ExistingBacklog: existingBacklogCount,
			WIP:             wipCount,
			AdditionalItems: additionalItems,
			Total:           actualBacklog,
		}

		// Add Advanced Reliability Analysis
		cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, statusWeights, resolutions)
		engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, existingBacklogCount+additionalItems)

		if (existingBacklogCount+additionalItems) == 0 && includeWIP {
			resObj.Warnings = append(resObj.Warnings, fmt.Sprintf("Note: This forecast ONLY covers the %d items currently in progress. Unstarted backlog items were not included.", wipCount))
		}

		if includeWIP && (existingBacklogCount+additionalItems) > 0 && wipCount > (existingBacklogCount+additionalItems)*3 {
			resObj.Warnings = append(resObj.Warnings, fmt.Sprintf("High operational load: You have %d items in progress, which is significantly larger than the %d unstarted items in this forecast. Lead times for new items may be long.", wipCount, existingBacklogCount+additionalItems))
		}
		return resObj, nil

	default:
		// Auto-detect if mode not explicitly provided
		if targetDays > 0 || targetDate != "" {
			return s.handleRunSimulation(sourceID, sourceType, "scope", false, 0, targetDays, targetDate, "", "", nil, false, resolutions)
		}
		return nil, fmt.Errorf("mode required: 'duration' (backlog forecast) or 'scope' (volume forecast)")
	}
}

func (s *Server) handleGetStatusPersistence(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	response, err := s.jira.SearchIssuesWithHistory(ctx.JQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	issues := make([]jira.Issue, len(response.Issues))
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto)
	}

	return stats.CalculateStatusPersistence(issues), nil
}

func (s *Server) handleGetWorkflowDiscovery(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	response, err := s.jira.SearchIssuesWithHistory(ctx.JQL, 0, 500)
	if err != nil {
		return nil, err
	}

	issues := make([]jira.Issue, len(response.Issues))
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto)
	}

	discovery := s.getWorkflowDiscovery(sourceID, issues)
	return discovery, nil
}

func (s *Server) getWorkflowDiscovery(sourceID string, issues []jira.Issue) interface{} {
	projectKey := s.extractProjectKey(issues)
	statusWeights := s.getStatusWeights(projectKey)
	statusCats := s.getStatusCategories(projectKey)

	discovery := stats.AnalyzeProbe(issues, 0)

	// Enrich with hints for commitment points
	hints := s.getCommitmentPointHints(issues, statusWeights)

	return map[string]interface{}{
		"discovery": discovery,
		"mapping":   s.workflowMappings[sourceID],
		"hints": map[string]interface{}{
			"proposed_commitment_points": hints,
			"categories":                 statusCats,
		},
	}
}

func (s *Server) handleSetWorkflowMapping(sourceID string, mapping map[string]interface{}) (interface{}, error) {
	m := make(map[string]stats.StatusMetadata)
	for k, v := range mapping {
		if vm, ok := v.(map[string]interface{}); ok {
			m[k] = stats.StatusMetadata{
				Tier: asString(vm["tier"]),
				Role: asString(vm["role"]),
			}
		}
	}
	s.workflowMappings[sourceID] = m
	return map[string]string{"status": "success", "message": fmt.Sprintf("Stored workflow mapping for source %s", sourceID)}, nil
}

func (s *Server) handleSetWorkflowOrder(sourceID string, order []string) (interface{}, error) {
	s.statusOrderings[sourceID] = order
	return map[string]string{"status": "success", "message": fmt.Sprintf("Stored workflow order for source %s", sourceID)}, nil
}

func (s *Server) handleGetItemJourney(issueKey string) (interface{}, error) {
	jql := fmt.Sprintf("key = %s", issueKey)
	response, err := s.jira.SearchIssuesWithHistory(jql, 0, 1)
	if err != nil {
		return nil, err
	}

	if response.Total == 0 {
		return nil, fmt.Errorf("issue not found: %s", issueKey)
	}

	issue := stats.MapIssue(response.Issues[0])

	type JourneyStep struct {
		Status string  `json:"status"`
		Days   float64 `json:"days"`
	}
	var steps []JourneyStep

	// Use the transitions to build a chronological journey
	if len(issue.Transitions) > 0 {
		// First segment: from creation to first transition
		firstDuration := issue.Transitions[0].Date.Sub(issue.Created).Seconds()
		steps = append(steps, JourneyStep{
			Status: "Created",
			Days:   math.Round((firstDuration/86400.0)*10) / 10,
		})

		for i := 0; i < len(issue.Transitions)-1; i++ {
			duration := issue.Transitions[i+1].Date.Sub(issue.Transitions[i].Date).Seconds()
			steps = append(steps, JourneyStep{
				Status: issue.Transitions[i].ToStatus,
				Days:   math.Round((duration/86400.0)*10) / 10,
			})
		}

		// Final segment: from last transition to current/resolution
		var finalDate time.Time
		if issue.ResolutionDate != nil {
			finalDate = *issue.ResolutionDate
		} else {
			finalDate = time.Now()
		}
		lastTrans := issue.Transitions[len(issue.Transitions)-1]
		finalDuration := finalDate.Sub(lastTrans.Date).Seconds()
		steps = append(steps, JourneyStep{
			Status: lastTrans.ToStatus,
			Days:   math.Round((finalDuration/86400.0)*10) / 10,
		})
	}

	residencyDays := make(map[string]float64)
	for s, sec := range issue.StatusResidency {
		residencyDays[s] = math.Round((float64(sec)/86400.0)*10) / 10
	}

	return map[string]interface{}{
		"key":       issue.Key,
		"summary":   issue.Summary,
		"residency": residencyDays,
		"path":      steps,
	}, nil
}

func (s *Server) handleGetProcessYield(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Fetch last 6 months of resolved items with history
	startTime := time.Now().AddDate(0, -6, 0)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		ctx.JQL, startTime.Format("2006-01-02"))

	response, err := s.jira.SearchIssuesWithHistory(ingestJQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	issues := make([]jira.Issue, len(response.Issues))
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto)
	}

	mappings := s.workflowMappings[sourceID]
	resolutions := s.getResolutionMap()

	return stats.CalculateProcessYield(issues, mappings, resolutions), nil
}

func (s *Server) handleGetInventoryAgingAnalysis(sourceID, sourceType, agingType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// 1. Ingest historical data for Lead Time baseline (resolved in last 180 days)
	histJQL := fmt.Sprintf("(%s) AND resolution is NOT EMPTY AND resolved >= -180d", ctx.JQL)

	histResponse, err := s.jira.SearchIssuesWithHistory(histJQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	histIssues := make([]jira.Issue, len(histResponse.Issues))
	for i, dto := range histResponse.Issues {
		histIssues[i] = stats.MapIssue(dto)
	}

	// Determine commitment context
	projectKey := ctx.PrimaryProject
	if projectKey == "" && len(histIssues) > 0 {
		projectKey = histIssues[0].ProjectKey
	}
	statusWeights := s.getStatusWeights(projectKey)

	// Override weights with verified mappings if available to ensure correct backflow detection
	if m, ok := s.workflowMappings[sourceID]; ok {
		for name, metadata := range m {
			if metadata.Tier == "Demand" {
				statusWeights[name] = 1
			} else if metadata.Tier == "Downstream" || metadata.Tier == "Finished" {
				// Ensure it's at least weight 2 for commitment
				if statusWeights[name] < 2 {
					statusWeights[name] = 2
				}
			}
		}
	}

	// 1b. Apply Backflow Policy (Discard pre-backflow history)
	commitmentWeight := 2
	histIssues = s.applyBackflowPolicy(histIssues, statusWeights, commitmentWeight)

	// Fetch appropriate baseline
	var baseline []float64
	resolutions := []string{"Fixed", "Done", "Complete", "Resolved"}
	if agingType == "total" {
		baseline = s.getTotalAges(histIssues, resolutions)
	} else {
		baseline = s.getCycleTimes(sourceID, histIssues, "", "", statusWeights, resolutions)
	}

	// 3. Determine Verification Status (for WIP mode)
	verified := false
	if m, ok := s.workflowMappings[sourceID]; ok && len(m) > 0 {
		verified = true
	}

	if agingType == "wip" && !verified {
		// PRECONDITION REFUSAL: Provide discovery data instead of performing expensive WIP calculation
		discovery := s.getWorkflowDiscovery(sourceID, histIssues)
		return map[string]interface{}{
			"status":       "precondition_required",
			"message":      "WIP Aging analysis requires a verified Commitment Point (semantic mapping).",
			"discovery":    discovery,
			"instructions": "The 'WIP' calculation remains invalid until the commitment point is confirmed. Please present the above workflow discovery to the user, propose a mapping (meta-workflow tiers and roles), and confirm it via 'set_workflow_mapping' before re-running this tool.",
		}, nil
	}

	// 2. Get Current WIP (up to 1000 oldest items)
	wipJQL := fmt.Sprintf("(%s) AND resolution is EMPTY ORDER BY created ASC", ctx.JQL)
	wipResponse, err := s.jira.SearchIssuesWithHistory(wipJQL, 0, 1000)
	if err != nil {
		return nil, err
	}
	wipIssues := make([]jira.Issue, len(wipResponse.Issues))
	for i, dto := range wipResponse.Issues {
		wipIssues[i] = stats.MapIssue(dto)
	}
	wipIssues = s.applyBackflowPolicy(wipIssues, statusWeights, commitmentWeight)

	respCtx := map[string]interface{}{
		"aging_type": agingType,
	}

	if agingType == "wip" {
		respCtx["commitment_point_verified"] = verified
	}

	var startStatus string
	if verified {
		startStatus = s.getEarliestCommitment(sourceID)
	}

	// Return neutral wrapped response
	return map[string]interface{}{
		"inventory_aging": stats.CalculateInventoryAge(wipIssues, startStatus, statusWeights, baseline, agingType),
		"context":         respCtx,
	}, nil
}

func (s *Server) handleGetDeliveryCadence(sourceID, sourceType string, windowWeeks int) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Fetch history within the window
	startTime := time.Now().AddDate(0, 0, -windowWeeks*7)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		ctx.JQL, startTime.Format("2006-01-02"))

	response, err := s.jira.SearchIssuesWithHistory(ingestJQL, 0, 2000)
	if err != nil {
		return nil, err
	}

	issues := make([]jira.Issue, len(response.Issues))
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto)
	}

	return stats.CalculateDeliveryCadence(issues, windowWeeks), nil
}

func (s *Server) handleGetProcessStability(sourceID, sourceType string, windowWeeks int) (interface{}, error) {
	if windowWeeks <= 0 {
		windowWeeks = 26
	}

	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// 1. Ingest historical data for Cycle Time baseline (resolved in last N weeks)
	startTime := time.Now().AddDate(0, 0, -windowWeeks*7)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		ctx.JQL, startTime.Format("2006-01-02"))

	response, err := s.jira.SearchIssuesWithHistory(ingestJQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	issues := make([]jira.Issue, len(response.Issues))
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto)
	}

	// 2. Identify context (Start Status)
	projectKey := ctx.PrimaryProject
	if projectKey == "" && len(issues) > 0 {
		projectKey = issues[0].ProjectKey
	}
	statusWeights := s.getStatusWeights(projectKey)
	resNames := []string{"Fixed", "Done", "Complete", "Resolved"} // Default

	startStatus := s.getEarliestCommitment(sourceID)
	// We use applyBackflowPolicy to ensure high-fidelity WIP Age
	issues = s.applyBackflowPolicy(issues, statusWeights, 2)

	// 3. Calculate Historical Cycle Times (The Baseline)
	cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, "", statusWeights, resNames)

	// 4. Get Current WIP Population (The Monitoring group)
	wipJQL := fmt.Sprintf("(%s) AND resolution is EMPTY", ctx.JQL)
	wipResponse, err := s.jira.SearchIssuesWithHistory(wipJQL, 0, 1000)
	var wipAges []float64
	if err == nil {
		wipIssues := make([]jira.Issue, len(wipResponse.Issues))
		for i, dto := range wipResponse.Issues {
			wipIssues[i] = stats.MapIssue(dto)
		}
		wipIssues = s.applyBackflowPolicy(wipIssues, statusWeights, 2)
		calcWipAges := stats.CalculateInventoryAge(wipIssues, startStatus, statusWeights, cycleTimes, "wip")
		for _, wa := range calcWipAges {
			if wa.AgeDays != nil {
				wipAges = append(wipAges, *wa.AgeDays)
			}
		}
	}

	// 5. Calculate Time Stability (Integrated Baseline vs WIP)
	timeStability := stats.AnalyzeTimeStability(cycleTimes, wipAges)

	// 6. Calculate Throughput Stability (Weekly)
	h := simulation.NewHistogram(issues, startTime, time.Now(), nil, resNames)
	weeklyThroughput := aggregateToWeeks(h.Counts)
	throughputStability := stats.CalculateXmR(weeklyThroughput)

	return map[string]interface{}{
		"time_stability":       timeStability,
		"throughput_stability": throughputStability,
		"context": map[string]interface{}{
			"window_weeks": windowWeeks,
			"sample_size":  len(issues),
			"wip_count":    len(wipAges),
		},
	}, nil
}

func aggregateToWeeks(dailyCounts []int) []float64 {
	var weekly []float64
	for i := 0; i < len(dailyCounts); i += 7 {
		end := i + 7
		if end > len(dailyCounts) {
			end = len(dailyCounts)
		}
		weekSum := 0
		for _, c := range dailyCounts[i:end] {
			weekSum += c
		}
		weekly = append(weekly, float64(weekSum))
	}
	return weekly
}

func (s *Server) handleGetProcessEvolution(sourceID, sourceType string, windowMonths int) (interface{}, error) {
	if windowMonths <= 0 {
		windowMonths = 12
	}

	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// 1. Ingest historical data for longitudinal analysis
	startTime := time.Now().AddDate(0, -windowMonths, 0)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		ctx.JQL, startTime.Format("2006-01-02"))

	log.Info().Str("jql", ingestJQL).Int("months", windowMonths).Msg("Performing deep history ingestion for evolution analysis")

	response, err := s.jira.SearchIssuesWithHistory(ingestJQL, 0, 2000) // Increase limit for deep history
	if err != nil {
		return nil, err
	}

	issues := make([]jira.Issue, len(response.Issues))
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto)
	}

	// 2. Identify context
	projectKey := ctx.PrimaryProject
	if projectKey == "" && len(issues) > 0 {
		projectKey = issues[0].ProjectKey
	}
	statusWeights := s.getStatusWeights(projectKey)
	resNames := []string{"Fixed", "Done", "Complete", "Resolved"}
	startStatus := s.getEarliestCommitment(sourceID)

	// Apply backflow policy to ensure clean cycle times
	issues = s.applyBackflowPolicy(issues, statusWeights, 2)

	// 3. Calculate Cycle Times
	cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, "", statusWeights, resNames)

	// 4. Group into Monthly Subgroups
	subgroups := stats.GroupIssuesByMonth(issues, cycleTimes)

	// 5. Calculate Three-Way XmR
	evolution := stats.CalculateThreeWayXmR(subgroups)

	return map[string]interface{}{
		"evolution": evolution,
		"context": map[string]interface{}{
			"window_months":  windowMonths,
			"total_issues":   len(issues),
			"subgroup_count": len(subgroups),
		},
	}, nil
}
