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
	finished := s.getFinishedStatuses(sourceID)
	issues := make([]jira.Issue, len(response.Issues))
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto, finished)
	}

	summary := stats.AnalyzeProbe(issues, response.Total, finished)

	// metadata summary is now purely about data health and volume
	return map[string]interface{}{
		"summary": summary,
		"_guidance": []string{
			"This is a DATA PROBE on a 50-item sample. Use it to understand data volume and health.",
			"SampleResolvedRatio is a diagnostic of the sample's completeness, NOT a team performance metric.",
			"CommitmentPointHints are inferred from historical transitions; please verify them.",
			"Inventory counts (WIP/Backlog) are heuristics based on Jira Status Categories and your 'Finished' tier mapping.",
		},
	}, nil
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

	log.Info().Int("count", response.Total).Msg("Historical ingestion complete")

	if response.Total == 0 {
		log.Warn().Str("jql", ingestJQL).Msg("No historical data found for simulation")
		return nil, fmt.Errorf("no historical data found in the last 6 months to base simulation on")
	}

	// Process DTOs into Domain Issues
	finished := s.getFinishedStatuses(sourceID)
	issues := make([]jira.Issue, len(response.Issues))
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto, finished)
	}

	// 3. Analytics Context (WIP Aging & Status Weights)
	projectKeys := s.extractProjectKeys(issues)
	statusWeights := s.getStatusWeights(projectKeys)
	// Override weights with verified mappings if available to ensure correct backflow detection
	if m, ok := s.workflowMappings[sourceID]; ok {
		for name, metadata := range m {
			switch metadata.Tier {
			case "Demand":
				statusWeights[name] = 1
			case "Downstream", "Finished":
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
		finished := s.getFinishedStatuses(sourceID)
		for _, dto := range response.Issues {
			issues = append(issues, stats.MapIssue(dto, finished))
		}
	}

	// 6. WIP Ingestion (if needed)
	if includeWIP {
		wipJQL := fmt.Sprintf("(%s) AND resolution is EMPTY", ctx.JQL)
		wipIssuesResponse, err := s.jira.SearchIssuesWithHistory(wipJQL, 0, 1000)
		if err == nil {
			finished := s.getFinishedStatuses(sourceID)
			wipIssues := make([]jira.Issue, len(wipIssuesResponse.Issues))
			for i, dto := range wipIssuesResponse.Issues {
				wipIssues[i] = stats.MapIssue(dto, finished)
			}
			wipIssues = s.applyBackflowPolicy(wipIssues, statusWeights, cWeight)
			cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, statusWeights, resolutions)
			mappings := s.workflowMappings[sourceID]
			if mappings == nil {
				mappings = make(map[string]stats.StatusMetadata)
			}
			calcWipAges := stats.CalculateInventoryAge(wipIssues, startStatus, statusWeights, mappings, cycleTimes, "wip")
			for _, wa := range calcWipAges {
				if wa.AgeSinceCommitment != nil {
					wipAges = append(wipAges, *wa.AgeSinceCommitment)
					wipCount++
				}
			}
		}
	}

	// 4. Mode Selection
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

func (s *Server) handleGetCycleTimeAssessment(sourceID, sourceType string, issueTypes []string, analyzeWIP bool, startStatus, endStatus string, resolutions []string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// 1. Ingestion: Fetch last 6 months of historical data
	startTime := time.Now().AddDate(0, -6, 0)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		ctx.JQL, startTime.Format("2006-01-02"))

	log.Debug().Str("jql", ingestJQL).Msg("Starting historical ingestion for cycle time assessment")

	response, err := s.jira.SearchIssuesWithHistory(ingestJQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	if response.Total == 0 {
		return nil, fmt.Errorf("no historical data found in the last 6 months to base assessment on")
	}

	// Process DTOs into Domain Issues
	finished := s.getFinishedStatuses(sourceID)
	issues := make([]jira.Issue, 0)
	itMap := make(map[string]bool)
	for _, it := range issueTypes {
		itMap[it] = true
	}

	for _, dto := range response.Issues {
		if len(issueTypes) > 0 && !itMap[dto.Fields.IssueType.Name] {
			continue
		}
		issues = append(issues, stats.MapIssue(dto, finished))
	}

	if len(issues) == 0 {
		return nil, fmt.Errorf("no historical data found for the specified issue types: %v", issueTypes)
	}

	// 2. Analytics Context
	projectKeys := s.extractProjectKeys(issues)
	statusWeights := s.getStatusWeights(projectKeys)
	if m, ok := s.workflowMappings[sourceID]; ok {
		for name, metadata := range m {
			switch metadata.Tier {
			case "Demand":
				statusWeights[name] = 1
			case "Downstream", "Finished":
				if statusWeights[name] < 2 {
					statusWeights[name] = 2
				}
			}
		}
	}

	if startStatus == "" {
		startStatus = s.getEarliestCommitment(sourceID)
	}

	// Apply Backflow Policy
	issues = s.applyBackflowPolicy(issues, statusWeights, 2)
	cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, statusWeights, resolutions)

	if len(cycleTimes) == 0 {
		msg := fmt.Sprintf("no resolved items found that passed the commitment point '%s'.", startStatus)
		hints := s.getCommitmentPointHints(issues, statusWeights)
		if len(hints) > 0 {
			msg += "\n\n💡 Hint: Based on historical reachability, these statuses were frequently used as work started: [" + strings.Join(hints, ", ") + "].\n(⚠️ Note: These are inferred from status categories and transition history; please verify if they represent your actual commitment point.)"
		}
		return nil, fmt.Errorf("%s", msg)
	}

	// 3. WIP Analysis if requested
	var wipAges []float64
	if analyzeWIP {
		wipJQL := fmt.Sprintf("(%s) AND resolution is EMPTY", ctx.JQL)
		wipResponse, err := s.jira.SearchIssuesWithHistory(wipJQL, 0, 1000)
		if err == nil {
			wipIssues := make([]jira.Issue, 0)
			for _, dto := range wipResponse.Issues {
				if len(issueTypes) > 0 && !itMap[dto.Fields.IssueType.Name] {
					continue
				}
				wipIssues = append(wipIssues, stats.MapIssue(dto, finished))
			}
			wipIssues = s.applyBackflowPolicy(wipIssues, statusWeights, 2)
			mappings := s.workflowMappings[sourceID]
			if mappings == nil {
				mappings = make(map[string]stats.StatusMetadata)
			}
			calcWipAges := stats.CalculateInventoryAge(wipIssues, startStatus, statusWeights, mappings, cycleTimes, "wip")
			for _, wa := range calcWipAges {
				if wa.AgeSinceCommitment != nil {
					wipAges = append(wipAges, *wa.AgeSinceCommitment)
				}
			}
		}
	}

	engine := simulation.NewEngine(nil)
	resObj := engine.RunCycleTimeAnalysis(cycleTimes)
	if analyzeWIP {
		engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, 0)
	}

	// 4. Design refined response
	return map[string]interface{}{
		"percentiles":       resObj.Percentiles,
		"percentile_labels": resObj.PercentileLabels,
		"spread":            resObj.Spread,
		"process_statistics": map[string]interface{}{
			"fat_tail_ratio":       resObj.FatTailRatio,
			"tail_to_median_ratio": resObj.TailToMedianRatio,
			"predictability":       resObj.Predictability,
			"sample_size":          len(cycleTimes),
		},
		"wip_stability": resObj.WIPAgeDistribution, // Only populated if includeWIP was true
		"warnings":      resObj.Warnings,
		"insights":      resObj.Insights,
		"_guidance": []string{
			"Individual item assessment shows the Service Level Expectation (SLE) for a single item.",
			"For high-confidence commitments, use the 'Likely' (P85) or 'Safe' (P95) metrics.",
			"If Fat-Tail Ratio >= 5.6, the process is statistically out of control; expect extreme outliers.",
		},
	}, nil
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
	finished := s.getFinishedStatuses(sourceID)
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto, finished)
	}

	mappings := s.workflowMappings[sourceID]
	if mappings == nil {
		mappings = make(map[string]stats.StatusMetadata)
	}

	// 1. Calculate base persistence
	statuses := stats.CalculateStatusPersistence(issues)

	// 2. Enrich with categories/mappings
	categories := make(map[string]string)
	for i := range issues {
		// Categories can be discovered from the issues themselves if not mapped
		categories[issues[i].Status] = "active" // Default, will be refined by Enrich
	}
	statuses = stats.EnrichStatusPersistence(statuses, categories, mappings)

	// 3. Aggregate into Tier Summary
	tierSummary := stats.CalculateTierSummary(issues, mappings)

	return stats.PersistenceResult{
		Statuses:    statuses,
		TierSummary: tierSummary,
		Warnings: []string{
			"Historical metrics for Finished tier are provided for cycle time context; ignore outlier warnings for this tier.",
		},
		Guidance: []string{
			"Status residence times show only time spent IN each status; total cycle time is the sum of these durations.",
			"Prioritize analysis of 'Upstream' and 'Downstream' tiers as they represent your active delivery workflow.",
			"Identify 'in-between' process bottlenecks first, then treat 'Demand' (Backlog) and 'Finished' (Archive) tiers as non-blocking summary context.",
			"High persistence in 'Demand' is expected storage time; persistence in 'Finished' is irrelevant to current flow diagnostics.",
		},
	}, nil
}

func (s *Server) handleGetWorkflowDiscovery(sourceID, sourceType string) (interface{}, error) {
	ctx, err := s.resolveSourceContext(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Hybrid Ingestion Strategy
	// Step 1: Recency Focus (items updated in the last 180 days)
	discoveryJQL := fmt.Sprintf("(%s) AND updated >= -180d ORDER BY updated DESC", ctx.JQL)
	log.Debug().Str("jql", discoveryJQL).Msg("Running Workflow Discovery (Step 1: Recency)")

	response, err := s.jira.SearchIssuesWithHistory(discoveryJQL, 0, 500)
	if err != nil {
		return nil, err
	}

	rawIssues := response.Issues
	issueKeys := make(map[string]bool)
	for _, itm := range rawIssues {
		issueKeys[itm.Key] = true
	}

	// Step 2: Coverage Buffer (ensure at least 100 items for discovery)
	if len(rawIssues) < 100 {
		log.Debug().Int("count", len(rawIssues)).Msg("Insufficient items for discovery, fetching coverage buffer")
		bufferJQL := fmt.Sprintf("(%s) ORDER BY updated DESC", ctx.JQL)
		bufferResponse, err := s.jira.SearchIssuesWithHistory(bufferJQL, 0, 100)
		if err == nil {
			for _, itm := range bufferResponse.Issues {
				if !issueKeys[itm.Key] {
					rawIssues = append(rawIssues, itm)
					issueKeys[itm.Key] = true
				}
			}
		}
	}

	issues := make([]jira.Issue, len(rawIssues))
	finished := s.getFinishedStatuses(sourceID)
	for i, dto := range rawIssues {
		issues[i] = stats.MapIssue(dto, finished)
	}

	discovery := s.getWorkflowDiscovery(sourceID, issues)
	return discovery, nil
}

func (s *Server) getWorkflowDiscovery(sourceID string, issues []jira.Issue) interface{} {
	projectKeys := s.extractProjectKeys(issues)
	statusWeights := s.getStatusWeights(projectKeys)
	statusCats := s.getStatusCategories(projectKeys)
	finished := s.getFinishedStatuses(sourceID)

	// 1. Calculate persistence-based discovery (Tiers/Roles)
	persistence := stats.CalculateStatusPersistence(issues)
	proposedMapping := stats.EnrichStatusPersistence(persistence, statusCats, make(map[string]stats.StatusMetadata))

	// 3. Resolution Discovery
	resFreq := make(map[string]int)
	for _, issue := range issues {
		if issue.Resolution != "" {
			resFreq[issue.Resolution]++
		}
	}

	// 2. Commitment Point Hints
	hints := s.getCommitmentPointHints(issues, statusWeights)

	// 4. Proposed Order
	proposedOrder := s.getInferredRange(sourceID, "", "", issues, statusWeights)

	return map[string]interface{}{
		"proposed_mapping":       proposedMapping,
		"discovered_resolutions": resFreq,
		"proposed_order":         proposedOrder,
		"current_mapping":        s.workflowMappings[sourceID],
		"hints": map[string]interface{}{
			"proposed_commitment_points": hints,
		},
		"data_summary": stats.AnalyzeProbe(issues, 0, finished),
		"_guidance": []string{
			"Confirm the 'proposed_mapping' with the user.",
			"WORKFLOW OUTCOME CALIBRATION: Review 'discovered_resolutions' and 'data_summary.statusAtResolution'.",
			"Ask user to classify each Finished status/resolution into Outcomes: 'delivered' (value), 'abandoned' (waste).",
			"TIERS: Demand (Backlog), Upstream (Refinement), Downstream (Development), Finished (Terminal).",
			"ROLES: active (working), queue (waiting), ignore (noise).",
		},
	}
}

func (s *Server) handleSetWorkflowMapping(sourceID string, mapping map[string]interface{}, resolutions map[string]interface{}) (interface{}, error) {
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
	s.workflowMappings[sourceID] = m

	if len(resolutions) > 0 {
		rm := make(map[string]string)
		for k, v := range resolutions {
			rm[k] = asString(v)
		}
		s.resolutionMappings[sourceID] = rm
	}

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

	// Finding which source this issue belongs to (heuristic)
	var sourceID string
	var mapping map[string]stats.StatusMetadata
	for id, m := range s.workflowMappings {
		if _, ok := m[response.Issues[0].Fields.Status.Name]; ok {
			sourceID = id
			mapping = m
			break
		}
	}
	finished := s.getFinishedStatuses(sourceID)
	issue := stats.MapIssue(response.Issues[0], finished)

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

	// Calculate Tier Breakdown
	tierBreakdown := make(map[string]map[string]interface{})
	// Find which source this issue belongs to (heuristic: use project key to find a mapping)
	for _, m := range s.workflowMappings {
		if _, ok := m[issue.Status]; ok {
			mapping = m
			break
		}
	}

	for status, sec := range issue.StatusResidency {
		tier := "Unknown"
		if mapping != nil {
			if m, ok := mapping[status]; ok {
				tier = m.Tier
			}
		}
		if _, ok := tierBreakdown[tier]; !ok {
			tierBreakdown[tier] = map[string]interface{}{
				"days":     0.0,
				"statuses": []string{},
			}
		}
		data := tierBreakdown[tier]
		data["days"] = data["days"].(float64) + math.Round((float64(sec)/86400.0)*10)/10
		data["statuses"] = append(data["statuses"].([]string), status)
		tierBreakdown[tier] = data
	}

	return map[string]interface{}{
		"key":            issue.Key,
		"summary":        issue.Summary,
		"residency":      residencyDays,
		"path":           steps,
		"tier_breakdown": tierBreakdown,
		"warnings":       []string{},
		"_guidance": []string{
			"The 'path' shows chronological flow, while 'residency' shows cumulative totals.",
			"Tiers represent the meta-workflow (Demand -> Upstream -> Downstream -> Finished).",
		},
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
	finished := s.getFinishedStatuses(sourceID)
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto, finished)
	}

	mappings := s.workflowMappings[sourceID]
	resolutions := s.getResolutionMap(sourceID)

	return stats.CalculateProcessYield(issues, mappings, resolutions), nil
}

func (s *Server) handleGetAgingAnalysis(sourceID, sourceType, agingType, tierFilter string) (interface{}, error) {
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

	finished := s.getFinishedStatuses(sourceID)
	histIssues := make([]jira.Issue, len(histResponse.Issues))
	for i, dto := range histResponse.Issues {
		histIssues[i] = stats.MapIssue(dto, finished)
	}

	// Determine commitment context
	projectKeys := s.extractProjectKeys(histIssues)
	statusWeights := s.getStatusWeights(projectKeys)

	// Override weights with verified mappings if available to ensure correct backflow detection
	if m, ok := s.workflowMappings[sourceID]; ok {
		for name, metadata := range m {
			switch metadata.Tier {
			case "Demand":
				statusWeights[name] = 1
			case "Downstream", "Finished":
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
	resolutions := s.getDeliveredResolutions(sourceID)
	if agingType == "total" {
		baseline = s.getTotalAges(sourceID, histIssues, resolutions)
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

	// 2. Get Current Population (up to 1000 oldest items)
	// We fetch all non-resolved items. Filtering by tier happens after mapping.
	wipJQL := fmt.Sprintf("(%s) AND resolution is EMPTY ORDER BY created ASC", ctx.JQL)
	wipResponse, err := s.jira.SearchIssuesWithHistory(wipJQL, 0, 1000)
	if err != nil {
		return nil, err
	}
	wipIssues := make([]jira.Issue, len(wipResponse.Issues))
	for i, dto := range wipResponse.Issues {
		wipIssues[i] = stats.MapIssue(dto, finished)
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

	mappings := s.workflowMappings[sourceID]
	if mappings == nil {
		mappings = make(map[string]stats.StatusMetadata)
	}

	// 4. Calculate analysis (includes all mapped tiers)
	analyses := stats.CalculateInventoryAge(wipIssues, startStatus, statusWeights, mappings, baseline, agingType)

	// 5. Apply tier_filter
	filtered := make([]stats.InventoryAgeAnalysis, 0)
	target := strings.ToUpper(tierFilter)
	if target == "" {
		target = "WIP" // Default to WIP
	}

	for _, a := range analyses {
		switch target {
		case "ALL":
			filtered = append(filtered, a)
		case "WIP":
			if a.Tier == "Upstream" || a.Tier == "Downstream" {
				filtered = append(filtered, a)
			}
		default:
			if strings.ToUpper(a.Tier) == target {
				filtered = append(filtered, a)
			}
		}
	}

	// Return neutral wrapped response
	return stats.AgingResult{
		Items: filtered,
		Warnings: []string{
			"High operational load: some items have significant cumulative downstream days.",
		},
		Guidance: []string{
			"WIP Age measures time since commitment; Total Age measures time since creation.",
			"For items in the 'Finished' tier, age is fixed (Cycle Time) because the clock stopped upon entering terminal status.",
			"Focus diagnostics on 'Upstream' and 'Downstream' tiers; high Upstream age suggests definition/refinement bottlenecks.",
			"Use 'tier_filter' to focus on 'WIP' (active stages), 'Demand', 'Finished', or specific phases.",
		},
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
	finished := s.getFinishedStatuses(sourceID)
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto, finished)
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
	finished := s.getFinishedStatuses(sourceID)
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto, finished)
	}

	// 2. Identify context (Start Status)
	projectKeys := s.extractProjectKeys(issues)
	statusWeights := s.getStatusWeights(projectKeys)
	resNames := s.getDeliveredResolutions(sourceID)

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
		finished := s.getFinishedStatuses(sourceID)
		wipIssues := make([]jira.Issue, len(wipResponse.Issues))
		for i, dto := range wipResponse.Issues {
			wipIssues[i] = stats.MapIssue(dto, finished)
		}
		wipIssues = s.applyBackflowPolicy(wipIssues, statusWeights, 2)
		mappings := s.workflowMappings[sourceID]
		if mappings == nil {
			mappings = make(map[string]stats.StatusMetadata)
		}
		calcWipAges := stats.CalculateInventoryAge(wipIssues, startStatus, statusWeights, mappings, cycleTimes, "wip")
		for _, wa := range calcWipAges {
			if wa.AgeSinceCommitment != nil {
				wipAges = append(wipAges, *wa.AgeSinceCommitment)
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
		"warnings": []string{
			"Stability trigger: Fat-tail ratio exceeds 5.6, indicating an unpredictable process.",
		},
		"_guidance": []string{
			"Time Stability compares historical cycle times against currently aging items.",
			"Throughput Stability (XmR) identifies 'Special Cause' variation in delivery behavior.",
			"Observe 'FatTailRatio' in Time Stability: if > 5.6, the process is statistically out of control.",
		},
		"context": map[string]interface{}{
			"window_weeks":             windowWeeks,
			"historical_baseline_size": len(issues),
			"current_wip_inventory":    len(wipAges),
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
	finished := s.getFinishedStatuses(sourceID)
	for i, dto := range response.Issues {
		issues[i] = stats.MapIssue(dto, finished)
	}

	// 2. Analytics Context
	projectKeys := s.extractProjectKeys(issues)
	statusWeights := s.getStatusWeights(projectKeys)
	resNames := s.getDeliveredResolutions(sourceID)
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
		"warnings": []string{
			"Significant process shift detected in recent subgroups; historical stability may no longer apply.",
		},
		"_guidance": []string{
			"Evolution analysis identifies long-term shifts in process behavior (Mean/Sigma shift).",
			"Monthly subgroups may have high variation if sample sizes are small.",
		},
		"context": map[string]interface{}{
			"window_months":  windowMonths,
			"total_issues":   len(issues),
			"subgroup_count": len(subgroups),
		},
	}, nil
}
