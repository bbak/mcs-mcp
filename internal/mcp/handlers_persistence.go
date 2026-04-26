package mcp

import (
	"fmt"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

func (s *Server) handleGetStatusPersistence(projectKey string, boardID int) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	// 1. Project using the session analysis window
	winStart, winEnd, _ := s.Window()
	window := stats.NewAnalysisWindow(winStart, winEnd, "day", s.activeCutoff())
	session := s.openSession(hctx, window)

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

	guidance := []string{
		"Status Persistence uses the session analysis window (default rolling 26 weeks). Adjust via 'set_analysis_window'.",
		"Status Persistence EXCLUSIVELY analyzes items that have successfully finished ('delivered') to prevent active WIP from skewing historical norms.",
		"Persistence stats (coin_toss, likely, etc.) measure INTERNAL residency time WITHIN one status. They ARE NOT end-to-end completion forecasts.",
		"Inner80 and IQR help distinguish between 'Stable Flow' and 'High Variance' bottlenecks.",
		"Tier Summary aggregates performance by meta-workflow phase (Demand, Upstream, Downstream).",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(issues), guidance), nil
}

func (s *Server) handleGetAgingAnalysis(projectKey string, boardID int, agingType, tierFilter string) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	// 1. Project — work item age is a point-in-time metric. The session window's
	// End is the snapshot date; its Start is intentionally ignored (an item is
	// "in-flight" at a moment, not over a range).
	_, snapshotEnd, _ := s.Window()
	window := stats.NewAnalysisWindow(time.Time{}, snapshotEnd, "day", s.activeCutoff())
	session := s.openSession(hctx, window)

	all := session.GetAllIssues()
	wip := session.GetWIP()
	delivered := session.GetDelivered()

	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)

	// Cycle times from history
	cycleTimes, _ := s.getCycleTimes(projectKey, boardID, delivered, analysisCtx.CommitmentPoint, "", nil)

	aging := stats.CalculateInventoryAge(wip, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, cycleTimes, agingType, s.commitmentBackflowReset, window.End)

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

	// Compute aggregate summary with risk-band distribution and stability index
	throughput := 0.0
	activeDays := float64(stats.CalendarDaysBetween(window.Start, window.End))
	if activeDays > 0 {
		throughput = float64(len(delivered)) / activeDays
	}
	summary := stats.CalculateAgingSummary(aging, cycleTimes, len(aging), throughput)

	res := map[string]any{
		"aging":   aging,
		"summary": summary,
	}

	guidance := []string{
		"Work item age is a point-in-time metric, NOT a range metric. This tool ignores the session window's Start and uses ONLY its End as the as-of date for in-flight items and age calculation. To analyse 'as of' a different date, set the session window's End via 'set_analysis_window'.",
		"Items in 'Demand' or 'Finished' tiers are usually excluded from WIP Age unless explicitly requested.",
		"PercentileRelative helps identify which individual items are 'neglect' risks compared to historical performance.",
		"AgeSinceCommitment reflects time since the LAST commitment (resets on backflow to Demand/Upstream).",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(all), guidance), nil
}

func (s *Server) handleAnalyzeResidenceTime(projectKey string, boardID int, issueTypes []string, granularity string) (any, error) {
	hctx, err := s.prepareHandler(projectKey, boardID)
	if err != nil {
		return nil, err
	}

	if granularity == "" {
		granularity = "day"
	}

	// 2. Project EVERYTHING from the beginning of time to capture stagnant/pre-window items
	cutoff := s.activeCutoff()
	fullWindow := stats.NewAnalysisWindow(time.Time{}, s.Clock(), "day", cutoff)
	session := s.openSession(hctx, fullWindow)

	all := session.GetAllIssues()
	analysisCtx := s.prepareAnalysisContext(projectKey, boardID, all)

	// 3. Apply issue type filter if provided
	filteredIssues := all
	if len(issueTypes) > 0 {
		typeSet := make(map[string]bool, len(issueTypes))
		for _, t := range issueTypes {
			typeSet[t] = true
		}
		var filtered []jira.Issue
		for _, issue := range all {
			if typeSet[issue.IssueType] {
				filtered = append(filtered, issue)
			}
		}
		filteredIssues = filtered
	}

	// 4. Extract residence items with backflow-reset logic (always-on)
	winStart, winEnd, _ := s.Window()
	displayWindow := stats.NewAnalysisWindow(winStart, winEnd, granularity, cutoff)
	residenceItems := stats.ExtractResidenceItems(filteredIssues, analysisCtx.CommitmentPoint, analysisCtx.StatusWeights, analysisCtx.WorkflowMappings, displayWindow.Start)

	// 5. Compute the sample path time series
	result := stats.ComputeResidenceTimeSeries(residenceItems, displayWindow)

	res := map[string]any{
		"residence_time": result,
	}

	guidance := []string{
		"This is a Sample Path Analysis (Stidham 1972, El-Taha & Stidham 1999) — it tracks the instantaneous count N(t) of active items over the observation window.",
		"Residence time: the time an item accumulates in the system within the observation window. Applies to both completed and still-active items. For active items, residence time grows linearly with the window endpoint T.",
		"Sojourn time (W*): the special case of residence time for completed items — their full duration from commitment to resolution. This is what 'analyze_cycle_time' measures.",
		fmt.Sprintf("The finite Little's Law identity L(T) = Λ(T) · w(T) holds exactly at every point. Identity verified: %v (max deviation: %.2e).", result.Validation.IdentityVerified, result.Validation.MaxDeviation),
		"Flow rate signals: Λ(T) = arrival rate (lambda), Θ(T) = departure rate (theta). When Λ > Θ, WIP is accumulating (more arriving than leaving). When Λ ≈ Θ, the system is balanced.",
		"Residence time decomposition: w(T) = H(T)/A(T) is arrival-denominated; w'(T) = H(T)/D(T) is departure-denominated. When w(T) ≈ w'(T), arrivals and departures are balanced. When w'(T) >> w(T), few departures are inflating the departure-weighted average — a flow imbalance signal.",
		"Coherence gap w(T) - W*(T): the 'end effect' of still-active items. A large gap means active WIP is significantly inflating the average residence time beyond what completed items experienced. The gap w'(T) - W*(T) isolates the empirical residual (path-integral vs arithmetic mean of completed sojourns).",
		fmt.Sprintf("Convergence assessment: %s — assessed via 1/T tail regression on w(T). 'converging' means w(T) is stabilising toward a steady-state value; 'diverging' means it is still climbing; 'metastable' means the tail is noisy but not clearly trending.", result.Summary.Convergence),
		"IMPORTANT: This tool always applies backflow reset (uses the LAST commitment date). This diverges from the configurable commitmentBackflowReset used by other tools like analyze_work_item_age.",
		"POPULATION NOTE: The sample path population includes only items whose transition history shows at least one crossing of the commitment boundary (from a status below the commitment weight to at-or-above it). Items without such a transition have zero residence time and are excluded. D(T) may therefore be lower than throughput from analyze_throughput, which counts all delivered items regardless of commitment evidence.",
	}

	return WrapResponse(res, projectKey, boardID, nil, s.getQualityWarnings(all), guidance), nil
}
