package mcp

import (
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

func (s *Server) handleGetProcessStability(projectKey string, boardID int, includeRawSeries bool) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	// 2. Project
	window := s.AnalysisWindow("week")
	session := s.openSession(hctx, window)

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

	// Scatterplot: chart-ready data with dates, cycle times, pooled moving ranges, and issue types.
	scatterplot := stats.BuildScatterplot(matchedIssues, cycleTimes)

	// Round all numeric output to 2 decimal places (post-math, output boundary only).
	stability.Round()
	for k, sr := range stratified {
		sr.Round()
		stratified[k] = sr
	}

	// Always strip stratified raw series (scatterplot supersedes them).
	for k, sr := range stratified {
		sr.XmR.Values = nil
		sr.XmR.MovingRange = nil
		stratified[k] = sr
	}

	if !includeRawSeries {
		stability.XmR.Values = nil
		stability.XmR.MovingRange = nil
	}

	res := map[string]any{
		"stability":   stability,
		"stratified":  stratified,
		"scatterplot": scatterplot,
	}

	guidance := []string{
		"XmR charts detect 'Special Cause' variation. If stability is low (outliers/shifts), forecasts are unreliable.",
		"Stability Index = (WIP / Throughput) / Average Cycle Time. A ratio > 1.3 indicates a 'Clogged' system.",
		"The 'scatterplot' array contains one entry per delivered work item with cycle time (value), date (the work item's outcome date), pooled moving range, and issue type. " +
			"Render a Cycle Time Scatterplot: X=date, Y=value. " +
			"Reference lines from stability.xmr: average (center), upper_natural_process_limit, lower_natural_process_limit. " +
			"For type-specific limits, use stratified[type].xmr.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(all), guidance), nil
}

func (s *Server) handleGetProcessEvolution(projectKey string, boardID int, bucket string) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	if bucket != "week" {
		bucket = "month"
	}

	// Process evolution is a long-term trend metric. It anchors at the session
	// window's End (capped at s.Clock() — never extrapolates past "now") and
	// looks back a fixed horizon: 12 complete months or 26 complete weeks.
	// Window Start is intentionally ignored: short ranges defeat the point of
	// trend detection.
	_, sessionEnd, _ := s.Window()
	rightEdge := sessionEnd
	if rightEdge.After(s.Clock()) {
		rightEdge = s.Clock()
	}
	end := stats.LastCompleteBucketEnd(rightEdge, bucket)

	var start time.Time
	if bucket == "month" {
		start = stats.SnapToStart(end.AddDate(0, -11, 0), "month") // 12 buckets inclusive
	} else {
		start = stats.SnapToStart(end.AddDate(0, 0, -7*25), "week") // 26 buckets inclusive
	}

	window := stats.NewAnalysisWindow(start, end, bucket, s.activeCutoff())
	session := s.openSession(hctx, window)

	delivered := session.GetDelivered()
	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, session.GetAllIssues())

	cycleTimes, matchedIssues := s.getCycleTimes(projectKey, boardID, delivered, analysisCtx.CommitmentPoint, "", nil)
	subgroups := stats.GroupIssuesByBucket(matchedIssues, cycleTimes, window)
	evolution := stats.CalculateThreeWayXmR(subgroups)
	evolution.Round()

	res := map[string]any{
		"evolution": evolution,
		"context": map[string]any{
			"bucket":         bucket,
			"window_start":   window.Start.Format(stats.DateFormat),
			"window_end":     window.End.Format(stats.DateFormat),
			"total_issues":   len(delivered),
			"subgroup_count": len(subgroups),
		},
	}

	guidance := []string{
		"Process evolution is a long-term trend metric. This tool ignores the session window's Start and uses ONLY its End as the right edge. Lookback is fixed: 12 complete months (bucket='month') or 26 complete weeks (bucket='week'). To shift the trend's right edge, set the session window's End via 'set_analysis_window'.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(delivered), guidance), nil
}

func (s *Server) handleGetProcessYield(projectKey string, boardID int) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := hctx.SourceID

	// 2. Project using the session analysis window
	window := s.AnalysisWindow("day")
	session := s.openSession(hctx, window)

	all := session.GetAllIssues()
	yield := stats.CalculateProcessYield(all, s.activeMapping, s.getResolutionMap(sourceID))
	yield.Round()
	stratified := stats.CalculateStratifiedYield(all, s.activeMapping, s.getResolutionMap(sourceID))
	for k, sy := range stratified {
		sy.Round()
		stratified[k] = sy
	}

	res := map[string]any{
		"yield":      yield,
		"stratified": stratified,
	}

	guidance := []string{
		"High 'Abandoned Upstream' often points to discovery/refinement issues.",
		"High 'Abandoned Downstream' points to execution or commitment issues.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(all), guidance), nil
}
