package mcp

import (
	"fmt"
	"math"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/visuals"
)

func (s *Server) handleGetStatusPersistence(projectKey string, boardID int) (any, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// 2. Project on Demand (6-month historical window for persistence)
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Now().AddDate(0, 0, -180), time.Now(), "day", cutoff)
	session := stats.NewAnalysisSession(s.events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)

	issues := session.GetDelivered()
	if len(issues) == 0 {
		return nil, fmt.Errorf("no historical data found to analyze status persistence (must have finished items)")
	}

	persistence := stats.CalculateStatusPersistence(issues)
	persistence = stats.EnrichStatusPersistence(persistence, s.activeMapping)
	stratified := stats.CalculateStratifiedStatusPersistence(issues)
	tierSummary := stats.CalculateTierSummary(issues, s.activeMapping)

	res := map[string]any{
		"persistence":            persistence,
		"stratified_persistence": stratified,
		"tier_summary":           tierSummary,
	}

	if s.enableMermaidCharts {
		res["visual_persistence_bar"] = visuals.GeneratePersistenceChart(persistence)
	}

	guidance := []string{
		"This tool uses a robust 6-MONTH historical window, making it the primary source for performance and residency analysis.",
		"Status Persistence EXCLUSIVELY analyzes items that have successfully finished ('delivered') to prevent active WIP from skewing historical norms.",
		"Persistence stats (coin_toss, likely, etc.) measure INTERNAL residency time WITHIN one status. They ARE NOT end-to-end completion forecasts.",
		"Inner80 and IQR help distinguish between 'Stable Flow' and 'High Variance' bottlenecks.",
		"Tier Summary aggregates performance by meta-workflow phase (Demand, Upstream, Downstream).",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(issues), guidance), nil
}

func (s *Server) handleGetAgingAnalysis(projectKey string, boardID int, agingType, tierFilter string) (any, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// 2. Project
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Time{}, time.Now(), "day", cutoff)
	session := stats.NewAnalysisSession(s.events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)

	all := session.GetAllIssues()
	wip := session.GetWIP()
	delivered := session.GetDelivered()

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)

	// Cycle times from history
	cycleTimes, _ := s.getCycleTimes(projectKey, boardID, delivered, analysisCtx.CommitmentPoint, "", nil)

	aging := stats.CalculateInventoryAge(wip, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes, agingType, window.End)

	// Apply tier filter if requested
	if tierFilter != "All" && tierFilter != "" {
		filtered := make([]stats.InventoryAge, 0)
		for _, a := range aging {
			if tierFilter == "WIP" {
				if a.Tier != "Demand" && a.Tier != "Finished" {
					filtered = append(filtered, a)
				}
			} else if a.Tier == tierFilter {
				filtered = append(filtered, a)
			}
		}
		aging = filtered
	}

	res := map[string]any{
		"aging": aging,
	}

	if s.enableMermaidCharts {
		res["visual_wip_aging"] = visuals.GenerateAgingChart(aging)
	}

	guidance := []string{
		"Items in 'Demand' or 'Finished' tiers are usually excluded from WIP Age unless explicitly requested.",
		"PercentileRelative helps identify which individual items are 'neglect' risks compared to historical performance.",
		"AgeSinceCommitment reflects time since the LAST commitment (resets on backflow to Demand/Upstream).",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(all), guidance), nil
}

func (s *Server) handleGetDeliveryCadence(projectKey string, boardID int, windowWeeks int, bucket string, _ bool) (any, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	if windowWeeks <= 0 {
		windowWeeks = 26
	}

	// 2. Project
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Now().AddDate(0, 0, -windowWeeks*7), time.Now(), bucket, cutoff)
	session := stats.NewAnalysisSession(s.events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)

	delivered := session.GetDelivered()
	throughput := stats.GetStratifiedThroughput(delivered, window, s.activeResolutions, s.activeMapping)

	// Build bucket metadata
	bucketMetadata := make([]map[string]string, 0)
	buckets := window.Subdivide()
	for i, bucketStart := range buckets {
		bucketEnd := stats.SnapToEnd(bucketStart, window.Bucket)
		bucketMetadata = append(bucketMetadata, map[string]string{
			"index":      fmt.Sprintf("%d", i+1),
			"start_date": bucketStart.Format("2006-01-02"),
			"end_date":   bucketEnd.Format("2006-01-02"),
			"label":      window.GenerateLabel(bucketStart),
			"is_partial": fmt.Sprintf("%v", window.IsPartial(bucketStart)),
		})
	}

	res := map[string]any{
		"total_throughput":      throughput.Pooled,
		"stratified_throughput": throughput.ByType,
		"@metadata":             bucketMetadata,
	}

	if s.enableMermaidCharts {
		res["visual_throughput_trend"] = visuals.GenerateThroughputChart(throughput.Pooled, bucketMetadata)
	}

	guidance := []string{
		"Look for 'Batching' (bursts of delivery followed by silence) vs. 'Steady Flow'.",
		fmt.Sprintf("The current window uses a %d-week historical baseline anchored at %s, grouped by %s.", windowWeeks, window.Start.Format("2006-01-02"), bucket),
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(delivered), guidance), nil
}

func (s *Server) handleGetProcessStability(projectKey string, boardID int) (any, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// 2. Project
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Now().AddDate(0, 0, -26*7), time.Now(), "week", cutoff)
	session := stats.NewAnalysisSession(s.events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)

	all := session.GetAllIssues()
	wip := session.GetWIP()
	delivered := session.GetDelivered()

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)
	cycleTimes, matchedIssues := s.getCycleTimes(projectKey, boardID, delivered, analysisCtx.CommitmentPoint, "", nil)

	// Stratified Analysis
	ctByType := s.getCycleTimesByType(projectKey, boardID, delivered, analysisCtx.CommitmentPoint, "", nil)
	wipByType := s.calculateWIPAges(wip, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes)

	issuesByType := make(map[string][]jira.Issue)
	for _, iss := range delivered {
		t := iss.IssueType
		if t == "" {
			t = "Unknown"
		}
		issuesByType[t] = append(issuesByType[t], iss)
	}

	stability := stats.CalculateProcessStability(matchedIssues, cycleTimes, len(wip), float64(window.ActiveDayCount()))
	stratified := stats.CalculateStratifiedStability(issuesByType, ctByType, wipByType, float64(window.ActiveDayCount()))

	res := map[string]any{
		"stability":  stability,
		"stratified": stratified,
	}

	if s.enableMermaidCharts {
		res["visual_stability_xmr"] = visuals.GenerateXmRChart(stability)
	}

	guidance := []string{
		"XmR charts detect 'Special Cause' variation. If stability is low (outliers/shifts), forecasts are unreliable.",
		"Stability Index = (WIP / Throughput) / Average Cycle Time. A ratio > 1.3 indicates a 'Clogged' system.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(all), guidance), nil
}

func (s *Server) handleGetProcessEvolution(projectKey string, boardID int, windowMonths int) (any, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	if windowMonths <= 0 {
		windowMonths = 12
	}

	// 2. Project
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Now().AddDate(0, -windowMonths, 0), time.Now(), "month", cutoff)
	session := stats.NewAnalysisSession(s.events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)

	delivered := session.GetDelivered()
	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, session.GetAllIssues())

	cycleTimes, matchedIssues := s.getCycleTimes(projectKey, boardID, delivered, analysisCtx.CommitmentPoint, "", nil)
	subgroups := stats.GroupIssuesByBucket(matchedIssues, cycleTimes, window)
	evolution := stats.CalculateThreeWayXmR(subgroups)

	res := map[string]any{
		"evolution": evolution,
		"context": map[string]any{
			"window_months":  windowMonths,
			"total_issues":   len(delivered),
			"subgroup_count": len(subgroups),
		},
	}

	if s.enableMermaidCharts {
		res["visual_evolution_xmr"] = visuals.GenerateEvolutionChart(evolution)
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(delivered), nil), nil
}

func (s *Server) handleGetProcessYield(projectKey string, boardID int) (any, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	// 2. Project
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}
	window := stats.NewAnalysisWindow(time.Time{}, time.Now(), "day", cutoff)
	session := stats.NewAnalysisSession(s.events, sourceID, *ctx, s.activeMapping, s.activeResolutions, window)

	all := session.GetAllIssues()
	yield := stats.CalculateProcessYield(all, s.activeMapping, s.getResolutionMap(sourceID))
	stratified := stats.CalculateStratifiedYield(all, s.activeMapping, s.getResolutionMap(sourceID))

	res := map[string]any{
		"yield":      yield,
		"stratified": stratified,
	}

	if s.enableMermaidCharts {
		res["visual_yield_pie"] = visuals.GenerateYieldPie(yield)
	}

	guidance := []string{
		"High 'Abandoned Upstream' often points to discovery/refinement issues.",
		"High 'Abandoned Downstream' points to execution or commitment issues.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(all), guidance), nil
}

func (s *Server) handleGetItemJourney(projectKey string, boardID int, issueKey string) (any, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Try to find in existing memory log
	events := s.events.GetEventsForIssue(sourceID, issueKey)

	// 2. Fallback to context-locked hydration if not found
	if len(events) == 0 {
		lockedJQL := fmt.Sprintf("(%s) AND key = %s", ctx.JQL, issueKey)
		if err := s.events.Hydrate(sourceID, lockedJQL); err != nil {
			return nil, err
		}
		events = s.events.GetEventsForIssue(sourceID, issueKey)
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("issue %s not found on the current Project (%s) and Board (%d)", issueKey, projectKey, boardID)
	}

	// Finished statuses for reconstruction
	finishedMap := make(map[string]bool)
	for status, m := range s.activeMapping {
		if m.Tier == "Finished" {
			finishedMap[status] = true
		}
	}

	issue := stats.MapIssueFromEvents(events, finishedMap, time.Now())

	type JourneyStep struct {
		Status string  `json:"status"`
		Days   float64 `json:"days"`
	}
	var steps []JourneyStep

	// Reconstruct path for display
	if len(issue.Transitions) > 0 {
		birthStatus := issue.BirthStatus
		if birthStatus == "" {
			birthStatus = issue.Transitions[0].FromStatus
		}

		firstDuration := issue.Transitions[0].Date.Sub(issue.Created).Seconds()
		steps = append(steps, JourneyStep{
			Status: birthStatus,
			Days:   math.Round((firstDuration/86400.0)*10) / 10,
		})

		for i := 0; i < len(issue.Transitions)-1; i++ {
			duration := issue.Transitions[i+1].Date.Sub(issue.Transitions[i].Date).Seconds()
			steps = append(steps, JourneyStep{
				Status: issue.Transitions[i].ToStatus,
				Days:   math.Round((duration/86400.0)*10) / 10,
			})
		}

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
	for st, sec := range issue.StatusResidency {
		residencyDays[st] = math.Round((float64(sec)/86400.0)*10) / 10
	}

	blockedDays := make(map[string]float64)
	for st, sec := range issue.BlockedResidency {
		blockedDays[st] = math.Round((float64(sec)/86400.0)*10) / 10
	}

	tierBreakdown := make(map[string]map[string]any)
	for status, sec := range issue.StatusResidency {
		tier := "Unknown"
		if s.activeMapping != nil {
			if m, ok := s.activeMapping[status]; ok {
				tier = m.Tier
			}
		}
		if _, ok := tierBreakdown[tier]; !ok {
			tierBreakdown[tier] = map[string]any{
				"days":         0.0,
				"blocked_days": 0.0,
				"statuses":     []string{},
			}
		}
		data := tierBreakdown[tier]
		data["days"] = data["days"].(float64) + math.Round((float64(sec)/86400.0)*10)/10

		if bSec, ok := issue.BlockedResidency[status]; ok {
			data["blocked_days"] = data["blocked_days"].(float64) + math.Round((float64(bSec)/86400.0)*10)/10
		}

		data["statuses"] = append(data["statuses"].([]string), status)
		tierBreakdown[tier] = data
	}

	res := map[string]any{
		"key":            issue.Key,
		"residency":      residencyDays,
		"blocked_time":   blockedDays,
		"path":           steps,
		"tier_breakdown": tierBreakdown,
		"warnings":       []string{},
	}

	guidance := []string{
		"The 'path' shows chronological flow, while 'residency' shows cumulative totals.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings([]jira.Issue{issue}), guidance), nil
}

func (s *Server) handleAnalyzeWIPStability(projectKey string, boardID int, windowWeeks int) (any, error) {
	ctx, err := s.resolveSourceContext(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := getCombinedID(projectKey, boardID)

	// 1. Hydrate
	if err := s.events.Hydrate(sourceID, ctx.JQL); err != nil {
		return nil, err
	}

	if windowWeeks <= 0 {
		windowWeeks = 26
	}

	// 2. Project EVERYTHING from the beginning of time to capture stagnant WIP
	cutoff := time.Time{}
	if s.activeDiscoveryCutoff != nil {
		cutoff = *s.activeDiscoveryCutoff
	}

	fullWindow := stats.NewAnalysisWindow(time.Time{}, time.Now(), "day", cutoff)
	session := stats.NewAnalysisSession(s.events, sourceID, *ctx, s.activeMapping, s.activeResolutions, fullWindow)

	all := session.GetAllIssues()
	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)

	// 3. Bound the chart output strictly to the requested display window
	displayWindow := stats.NewAnalysisWindow(time.Now().AddDate(0, 0, -windowWeeks*7), time.Now(), "day", cutoff)
	wipStability := stats.AnalyzeHistoricalWIP(all, displayWindow, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings)

	res := map[string]any{
		"wip_stability": wipStability,
	}

	if s.enableMermaidCharts {
		res["visual_wip_run_chart"] = visuals.GenerateWIPRunChart(wipStability)
	}

	guidance := []string{
		"WIP Stability provides a daily historical view of system population.",
		"Signals (Outliers/Shifts) indicate that WIP was not actively managed or constrained, which violates Little's Law.",
		"If the system is 'unstable', flow metrics (Cycle Time, Throughput) will be unpredictable and simulations may fail.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(all), guidance), nil
}
