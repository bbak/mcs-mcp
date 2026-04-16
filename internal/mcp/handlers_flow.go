package mcp

import (
	"fmt"
	"time"

	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/visuals"
)

func (s *Server) handleGetDeliveryCadence(projectKey string, boardID int, windowWeeks int, bucket string, _ bool) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	if windowWeeks <= 0 {
		windowWeeks = 26
	}

	// 2. Project
	window := stats.NewAnalysisWindow(s.Clock().AddDate(0, 0, -windowWeeks*7), s.Clock(), bucket, s.activeCutoff())
	session := s.openSession(hctx, window)

	delivered := session.GetDelivered()
	throughput := stats.GetStratifiedThroughput(delivered, window)
	throughput.XmR = stats.AnalyzeThroughputStability(throughput)

	// Build bucket metadata
	bucketMetadata := make([]map[string]string, 0)
	buckets := window.Subdivide()
	for i, bucketStart := range buckets {
		bucketEnd := stats.SnapToEnd(bucketStart, window.Bucket)
		bucketMetadata = append(bucketMetadata, map[string]string{
			"index":      fmt.Sprintf("%d", i+1),
			"start_date": bucketStart.Format(stats.DateFormat),
			"end_date":   bucketEnd.Format(stats.DateFormat),
			"label":      window.GenerateLabel(bucketStart),
			"is_partial": fmt.Sprintf("%v", window.IsPartial(bucketStart)),
		})
	}

	res := map[string]any{
		"total_throughput":      throughput.Pooled,
		"stratified_throughput": throughput.ByType,
		"@metadata":             bucketMetadata,
	}

	if throughput.XmR != nil {
		throughput.XmR.Round()
		res["stability"] = throughput.XmR
	}

	if s.enableMermaidCharts {
		res["visual_throughput_trend"] = visuals.GenerateThroughputChart(throughput.Pooled, bucketMetadata, throughput.XmR)
	}

	guidance := []string{
		"Look for 'Batching' (bursts of delivery followed by silence) vs. 'Steady Flow'.",
		fmt.Sprintf("The current window uses a %d-week historical baseline anchored at %s, grouped by %s.", windowWeeks, window.Start.Format(stats.DateFormat), bucket),
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(delivered), guidance), nil
}

func (s *Server) handleAnalyzeWIPStability(projectKey string, boardID int, windowWeeks int) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	if windowWeeks <= 0 {
		windowWeeks = 26
	}

	// 2. Project EVERYTHING from the beginning of time to capture stagnant WIP
	cutoff := s.activeCutoff()
	fullWindow := stats.NewAnalysisWindow(time.Time{}, s.Clock(), "day", cutoff)
	session := s.openSession(hctx, fullWindow)

	all := session.GetAllIssues()
	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)

	// 3. Bound the chart output strictly to the requested display window
	displayWindow := stats.NewAnalysisWindow(s.Clock().AddDate(0, 0, -windowWeeks*7), s.Clock(), "day", cutoff)
	wipStability := stats.AnalyzeHistoricalWIP(all, displayWindow, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings)
	wipStability.XmR.Round()

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

func (s *Server) handleAnalyzeWIPAgeStability(projectKey string, boardID int, windowWeeks int) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	if windowWeeks <= 0 {
		windowWeeks = 26
	}

	// 2. Project EVERYTHING from the beginning of time to capture stagnant WIP
	cutoff := s.activeCutoff()
	fullWindow := stats.NewAnalysisWindow(time.Time{}, s.Clock(), "day", cutoff)
	session := s.openSession(hctx, fullWindow)

	all := session.GetAllIssues()
	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)

	// 3. Bound the chart output strictly to the requested display window
	displayWindow := stats.NewAnalysisWindow(s.Clock().AddDate(0, 0, -windowWeeks*7), s.Clock(), "day", cutoff)
	wipAgeStability := stats.AnalyzeHistoricalWIPAge(all, displayWindow, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings)
	wipAgeStability.XmR.Round()

	res := map[string]any{
		"wip_age_stability": wipAgeStability,
	}

	if s.enableMermaidCharts {
		res["visual_wip_age_run_chart"] = visuals.GenerateWIPAgeRunChart(wipAgeStability)
	}

	guidance := []string{
		"Total WIP Age reveals the cumulative age burden on the system.",
		"While WIP Count tells how many items are in progress, Total WIP Age tells how long they have collectively been there.",
		"A growing Total WIP Age means items are aging without being delivered — it is a leading indicator of delivery problems.",
		"Even with stable WIP count, Total WIP Age can grow if items stagnate.",
		"Natural behavior: Total WIP Age grows by (WIP count x 1 day) per day when no items enter or exit.",
		"Average WIP Age is provided for convenience but is less informative — it can mask individual outliers and assumes nothing about the distribution shape.",
		"The XmR analysis on Total WIP Age is the most defensible signal — it detects process changes without distribution assumptions.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(all), guidance), nil
}

func (s *Server) handleGetFlowDebt(projectKey string, boardID int, windowWeeks int, bucket string) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	if windowWeeks <= 0 {
		windowWeeks = 26
	}
	if bucket == "" {
		bucket = "week"
	}

	// 2. Project
	now := s.Clock()
	window := stats.NewAnalysisWindow(now.AddDate(0, 0, -windowWeeks*7), now, bucket, s.activeCutoff())
	session := s.openSession(hctx, window)

	all := session.GetAllIssues()
	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)

	flowDebt := stats.CalculateFlowDebt(all, window, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, s.activeResolutions, s.activeMapping)

	res := map[string]any{
		"flow_debt": flowDebt,
	}

	guidance := []string{
		"Positive Flow Debt (Arrivals > Departures) is a leading indicator of cycle time inflation.",
		"Zero or Negative Flow Debt indicates a stable or improving system throughput-to-workload ratio.",
		fmt.Sprintf("The current window uses a %d-week historical baseline with Commitment Point: %s.", windowWeeks, analysisCtx.CommitmentPoint),
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(all), guidance), nil
}

func (s *Server) handleGetCFDData(projectKey string, boardID int, windowWeeks int, granularity string) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}
	sourceID := hctx.SourceID

	if windowWeeks <= 0 {
		windowWeeks = 26
	}

	// 2. Prepare Window and Reconstruction Context
	now := s.Clock()
	window := stats.NewAnalysisWindow(now.AddDate(0, 0, -windowWeeks*7), now, "day", time.Time{})

	// GetIssuesInRange returns events for all issues active in the window, including full history.
	events := s.events.GetIssuesInRange(sourceID, window.Start, window.End)

	finished, downstream, upstream, demand := stats.ProjectScope(events, window, "", s.activeMapping, s.activeResolutions, nil)
	allIssues := append(finished, append(downstream, append(upstream, demand...)...)...)

	// 3. Calculate CFD Data
	cfd := stats.CalculateCFDData(allIssues, window)

	// 4. Translate status IDs to human-readable names (API boundary translation)
	if s.activeRegistry != nil {
		for i, id := range cfd.Statuses {
			if name := s.activeRegistry.GetStatusName(id); name != "" {
				cfd.Statuses[i] = name
			}
		}
		for i := range cfd.Buckets {
			for t, statusCounts := range cfd.Buckets[i].ByIssueType {
				translated := make(map[string]int, len(statusCounts))
				for id, count := range statusCounts {
					key := id
					if name := s.activeRegistry.GetStatusName(id); name != "" {
						key = name
					}
					translated[key] = count
				}
				cfd.Buckets[i].ByIssueType[t] = translated
			}
		}
	}

	// Weekly downsampling: keep only the last bucket per ISO week (CFD is cumulative)
	if granularity == "weekly" && len(cfd.Buckets) > 0 {
		var weekly []stats.CFDBucket
		for i, b := range cfd.Buckets {
			isLast := i == len(cfd.Buckets)-1
			if !isLast {
				nextYear, nextWeek := cfd.Buckets[i+1].Date.ISOWeek()
				curYear, curWeek := b.Date.ISOWeek()
				if nextYear == curYear && nextWeek == curWeek {
					continue // Not the last day of this ISO week
				}
			}
			weekly = append(weekly, b)
		}
		cfd.Buckets = weekly
	}

	res := map[string]any{
		"cfd_data": cfd,
	}

	guidance := []string{
		"CFD (Cumulative Flow Diagram) provides a snapshot of work items by status and issue type.",
		"The visualization agent should use this data to render a stacked area chart.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(allIssues), guidance), nil
}
