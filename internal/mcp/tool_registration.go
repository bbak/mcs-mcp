package mcp

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

// toolDescriptions holds the long description strings for tools.
// Kept here to avoid cluttering the registration logic.
var toolDescriptions = map[string]string{

	// ── GROUP: Diagnostics — Process, Cycle Time, WIP & Flow ──────────────────

	"analyze_cycle_time": "Measures Cycle Time — also called Lead Time, completion time, elapsed time, duration, or time-to-delivery — for individual work items, and derives Service Level Expectations (SLE) such as P50/P70/P85/P95.\n\n" +
		"WHEN TO USE: User asks 'How long does an item take?', 'What is our cycle time?', 'What is our lead time?', 'How long from commit to done?', 'What is our SLE?', 'What percentile should we commit to?', 'Show the cycle time distribution / histogram / scatterplot', 'How long do stories / bugs typically take?'\n" +
		"WHEN NOT TO USE: Do not use to assess delivery volume stability — use 'analyze_throughput' for that. Do not use to assess predictability of the process — use 'analyze_process_stability' for that. Do not use for residence time / sample-path analysis — use 'analyze_residence_time' for that.\n\n" +
		"PREREQUISITE: Proper workflow mapping/commitment point MUST be confirmed via 'workflow_set_mapping' for accurate results.\n\n" +
		"WINDOWING: Uses the session analysis window (default rolling 26 weeks). Adjust via 'set_analysis_window'.\n\n" +
		"OUTPUT: Per-item cycle times, percentile distribution (P50/P70/P85/P95), Fat-Tail Ratio, scatterplot data, and SLE adherence trend.\n\n" +
		"INTERPRETATION: Primary signals are the Fat-Tail Ratio and P85 (SLE). A Fat-Tail Ratio > 1.5 means the distribution has a long tail — P85 is a more reliable SLE than the mean.",

	"analyze_process_stability": "Measures the predictability of Cycle Times using Wheeler XmR Process Behavior Charts.\n\n" +
		"WHEN TO USE: Use as the FIRST diagnostic step when users ask about forecasting reliability, prediction confidence, or whether historical data is a valid proxy for the future. " +
		"Ask: 'Is our process stable enough to forecast?'\n" +
		"WHEN NOT TO USE: Do not use to measure delivery volume — use 'analyze_throughput' for that. " +
		"Do not use for long-term trend analysis spanning many months — use 'analyze_process_evolution' for that.\n\n" +
		"PREREQUISITE: Proper workflow mapping is required for accurate results.\n\n" +
		"WINDOWING: Uses the session analysis window (default rolling 26 weeks). Adjust via 'set_analysis_window'.\n\n" +
		"INTERPRETATION: Primary signals are UNPL and the total number of signals (outliers + shifts). " +
		"If stability is low (many signals), simulations will produce MISLEADING results. " +
		"Combine with 'analyze_residence_time' when λ/θ > 1.1 to understand why cycle times are unstable.",

	"analyze_process_evolution": "Performs a longitudinal strategic audit of Cycle Time predictability over many months using Three-Way Control Charts.\n\n" +
		"WHEN TO USE: Deep history analysis, post-reorganization audits, quarterly/annual process reviews. " +
		"User asks: 'Has our delivery capability improved over the past year?' or 'When did the process change?'\n" +
		"WHEN NOT TO USE: Not for routine analysis — use 'analyze_process_stability' for that. Throughput-agnostic: does not measure delivery volume.\n\n" +
		"WINDOWING: Long-term trend metric. This tool ignores the session window's range and uses ONLY its End as the right edge. Lookback is FIXED: 12 complete months (bucket='month', default) or 26 complete weeks (bucket='week'). Only complete buckets are included — no partial trailing month/week. To shift the right edge, set the session window's End via 'set_analysis_window'.\n\n" +
		"PARAMETER GUIDANCE:\n" +
		"- bucket: 'month' (default) for quarterly/annual audits or 'week' for tighter regime-shift detection.\n\n" +
		"INTERPRETATION: Primary signals are detected regime shifts (process resets) and the long-term capability trend. " +
		"Subgroup analysis reveals structural drift that short-window XmR charts miss.",

	"analyze_status_persistence": "Analyzes how long completed items spent in each workflow status, revealing bottlenecks and inconsistency hotspots.\n\n" +
		"WHEN TO USE: User asks 'Where do items get stuck?', 'Which status has the most variability?', 'What is causing long cycle times?'\n" +
		"WHEN NOT TO USE: Do not use for active WIP — this tool only analyzes finished items. " +
		"Do not confuse with 'analyze_process_stability', which measures overall Cycle Time predictability, not per-status breakdown.\n\n" +
		"PREREQUISITE: Proper workflow mapping (Upstream/Downstream tiers) is required. Results are SUBPAR if tiers are unmapped.\n\n" +
		"WINDOWING: Uses the session analysis window (default rolling 26 weeks ≈ 6 months). Adjust via 'set_analysis_window'.\n\n" +
		"INTERPRETATION: Primary signal is IQR concentration — a status with high median but low IQR is a consistent queue; " +
		"high IQR indicates unpredictable, variable dwell time worth investigating.",

	"analyze_throughput": "Measures delivery volume — the number of items completed per week or month — and its stability using Wheeler XmR Process Behavior Charts.\n\n" +
		"WHEN TO USE: User asks 'How many items do we deliver per week?', 'Is our delivery cadence stable?', 'Do we have batching or zero-delivery weeks?'\n" +
		"WHEN NOT TO USE: Do not use to measure how long individual items take — use 'analyze_cycle_time' for that. " +
		"Do not use to assess Cycle Time predictability — use 'analyze_process_stability' for that.\n\n" +
		"WINDOWING: Uses the session analysis window (default rolling 26 weeks). Adjust via 'set_analysis_window'.\n\n" +
		"PARAMETER GUIDANCE:\n" +
		"- bucket: Default 'week'. Switch to 'month' for low-volume teams where weekly counts are too sparse to be meaningful.\n\n" +
		"INTERPRETATION: Primary signals are UNPL and zero-count weeks. " +
		"Zero-delivery weeks signal batching or blockage. UNPL breaches signal unusual surges. " +
		"Use 'analyze_flow_debt' as a leading indicator if throughput is declining.",

	"analyze_wip_stability": "Measures Work-In-Progress (WIP) count stability over time using XmR charts and a daily run chart.\n\n" +
		"WHEN TO USE: User asks 'Is our WIP under control?', 'Are we respecting WIP limits?', 'How variable is the number of active items?'\n" +
		"WHEN NOT TO USE: WIP count stability does NOT imply age stability — a stable count of 10 items can still be accumulating age. " +
		"Follow up with 'analyze_wip_age_stability' to check this. Do not use to detect individual aging items — use 'analyze_work_item_age' for that.\n\n" +
		"WINDOWING: Uses the session analysis window (default rolling 26 weeks). Adjust via 'set_analysis_window'.\n\n" +
		"INTERPRETATION: Primary signals are UNPL breaches and the trend direction. " +
		"A rising trend in WIP count, even within limits, is an early warning. Combine with 'analyze_residence_time' when λ/θ > 1.1.",

	"analyze_wip_age_stability": "Measures the cumulative age burden of all active WIP items over time using XmR charts — a leading indicator of future delivery problems.\n\n" +
		"WHEN TO USE: After 'analyze_wip_stability' — stable WIP count does not guarantee stable age. " +
		"User asks: 'Are items stagnating even though count looks fine?', 'Is the total age of WIP growing?'\n" +
		"WHEN NOT TO USE: Do not use to measure WIP count — use 'analyze_wip_stability' for that. " +
		"Do not use to find which specific items are aging — use 'analyze_work_item_age' for that.\n\n" +
		"WINDOWING: Uses the session analysis window (default rolling 26 weeks). Adjust via 'set_analysis_window'.\n\n" +
		"INTERPRETATION: Primary signal is UNPL breaches of total age (not average). " +
		"Growing total WIP age signals trouble before throughput drops. XmR is applied to total age, not the mean.",

	"analyze_work_item_age": "Identifies which active items are aging beyond historical norms, flagging outliers relative to P85 of historical cycle times.\n\n" +
		"WHEN TO USE: User asks 'Which items are taking too long?', 'What is at risk of breaching SLE?', 'Show me aging WIP.'\n" +
		"WHEN NOT TO USE: Do not use to assess overall WIP stability — use 'analyze_wip_stability' or 'analyze_wip_age_stability' for that. " +
		"An aging outlier is NOT necessarily blocked — it simply exceeds historical P85 for its current status.\n\n" +
		"PREREQUISITE: Commitment Point MUST be correctly mapped via 'workflow_set_mapping' for accurate 'WIP Age'. Results are UNRELIABLE otherwise.\n\n" +
		"WINDOWING: Work item age is a POINT-IN-TIME metric, not a range metric. This tool uses ONLY the End of the session analysis window as the as-of snapshot date — Start is intentionally ignored. Default snapshot is today (or the active evaluation date). Move the snapshot via 'set_analysis_window' (only the End matters for this tool).\n\n" +
		"INTERPRETATION: Primary signals are 'stability_index', outlier count, and P85/P95 thresholds. " +
		"Use 'age_type=wip' for standard SLE comparison; use 'age_type=total' to surface items that entered the system long ago but have not yet committed.",

	"analyze_flow_debt": "Measures the systemic imbalance between item arrivals (commitments) and departures (deliveries) — a leading indicator of cycle time inflation.\n\n" +
		"WHEN TO USE: Use before 'forecast_monte_carlo' to validate that WIP is not growing. " +
		"User asks: 'Are we taking on more work than we finish?', 'Is WIP accumulating?', 'Why are cycle times increasing?'\n" +
		"WHEN NOT TO USE: Do not confuse with 'analyze_residence_time' — Flow Debt is a leading indicator (arrival vs. departure counts); " +
		"Residence Time is a Little's Law analysis (L = λ · W) unifying cycle time, WIP age, and flow balance into a single coherent view.\n\n" +
		"WINDOWING: Uses the session analysis window (default rolling 26 weeks). Adjust via 'set_analysis_window'.\n\n" +
		"PARAMETER GUIDANCE:\n" +
		"- bucket_size: Default 'week'. Use 'month' for low-volume teams.\n\n" +
		"INTERPRETATION: Primary signals are 'totalDebt' and the oscillation pattern. " +
		"Sustained positive debt (Arrivals > Departures) mathematically guarantees higher future cycle times (Little's Law). " +
		"Oscillating debt is less concerning than a monotonically growing one.",

	"analyze_residence_time": "Performs a Sample Path Analysis (Little's Law: L = Λ · W) unifying cycle time, WIP age, WIP stability, and flow balance into a single coherent view.\n\n" +
		"WHEN TO USE: When you need to understand *why* a system is non-stationary — connects flow debt, WIP age, and cycle time into one analysis. " +
		"Use when 'analyze_wip_stability' or 'analyze_flow_debt' raises concerns and you want to quantify the severity.\n" +
		"WHEN NOT TO USE: Do not use as a first-line diagnostic — start with 'analyze_process_stability' or 'analyze_throughput'. " +
		"Do not confuse with 'analyze_flow_debt': Flow Debt counts arrivals vs. departures; Residence Time measures how long items actually accumulate in the system.\n\n" +
		"WINDOWING: Uses the session analysis window (default rolling 26 weeks). Adjust via 'set_analysis_window'. " +
		"Sample Path analysis benefits from longer windows — widen via 'set_analysis_window' (e.g. 52 weeks) when convergence requires more path length.\n\n" +
		"INTERPRETATION: Primary signals are the λ/θ ratio, coherence gap, and 'stationary' flag. " +
		"λ/θ > 1.1 means arrivals outpace completions — system is accumulating. " +
		"Coherence gap > 0.5 means active WIP is significantly older than completed items — stalled work is hiding in the system. " +
		"When 'stationary' is false, pass 'recommended_window_days' to 'forecast_monte_carlo' as 'history_window_days'.\n\n" +
		"NOTE: This tool ALWAYS applies backflow reset (uses the LAST commitment date), diverging from configurable backflow reset in other tools.",

	"analyze_yield": "Measures delivery efficiency across workflow tiers — what fraction of committed work reaches delivery vs. abandonment at each stage.\n\n" +
		"WHEN TO USE: User asks 'How much work do we abandon?', 'Where in the funnel do we lose the most?', 'What is our downstream abandonment rate?'\n" +
		"WHEN NOT TO USE: Do not use for throughput volume — use 'analyze_throughput'. " +
		"Do not use for cycle time — use 'analyze_cycle_time'. Yield measures outcome rates, not timing.\n\n" +
		"PREREQUISITE: Workflow tiers (Demand, Upstream, Downstream) and resolution outcomes MUST be verified with the user before interpreting results.\n\n" +
		"WINDOWING: Uses the session analysis window (default rolling 26 weeks). Adjust via 'set_analysis_window'. " +
		"Note: this scopes yield to items active in the window, not all-time. Widen the window for project-lifetime totals.\n\n" +
		"INTERPRETATION: Primary signal is 'overallYieldRate' per tier. " +
		"Downstream abandonment (items that passed the commitment point and were then discarded) is the most severe signal — it represents consumed capacity with no value delivered.",

	"generate_cfd_data": "Calculates daily (or weekly) item counts per status to produce Cumulative Flow Diagram (CFD) data.\n\n" +
		"WHEN TO USE: User asks for a CFD visualization, wants to see WIP accumulation over time by status, or needs to detect stage-level congestion.\n" +
		"WHEN NOT TO USE: This tool returns raw structured data — it is not a standalone diagnostic. " +
		"For overall WIP count stability, use 'analyze_wip_stability'. For arrival/departure imbalance, use 'analyze_flow_debt'.\n\n" +
		"WINDOWING: Uses the session analysis window (default rolling 26 weeks). Adjust via 'set_analysis_window'.\n\n" +
		"PARAMETER GUIDANCE:\n" +
		"- granularity: Default 'daily'. Use 'weekly' to reduce payload size for long windows or low-volume teams.\n\n" +
		"INTERPRETATION: Primary signals are band width changes (widening = accumulation) and which status bands are growing. " +
		"A widening Downstream band with a flat Finished band means delivery is stalling.",

	"analyze_item_journey": "Provides a single-item deep-dive into where one Jira issue spent its time across all workflow steps.\n\n" +
		"WHEN TO USE: User asks about a specific item: 'Why is PROJ-123 taking so long?', 'Where did this ticket get stuck?', 'Show me the history of this item.'\n" +
		"WHEN NOT TO USE: This is NOT a population-level diagnostic. For patterns across many items, use 'analyze_status_persistence' or 'analyze_work_item_age'.",

	// ── GROUP: Forecast & Simulation ─────────────────────────────────────────

	"forecast_monte_carlo": "Runs a Monte-Carlo simulation to forecast project outcomes based on historical throughput.\n\n" +
		"WHEN TO USE:\n" +
		"- mode=duration: 'When will a known set of items be done?' (deadline question — fixed scope, unknown date)\n" +
		"- mode=scope: 'How much will be done by a given date?' (capacity question — fixed date, unknown scope)\n" +
		"WHEN NOT TO USE: Does NOT analyze cycle times or individual item durations — use 'analyze_cycle_time' for that.\n\n" +
		"PARAMETER GUIDANCE:\n" +
		"- history_window_days: Default uses all available history. Narrow to 30–60 days after a process change, or use 'recommended_window_days' from 'analyze_residence_time' when that tool returns a non-stationary signal (λ/θ > 1.1).\n" +
		"- include_wip + include_existing_backlog: Set both to true for real commitment forecasts — this counts ALL outstanding work (started + unstarted). Omitting either understates the total scope.\n\n" +
		"FAILURE HANDLING: If the tool fails or returns zero throughput, do not provide estimated dates or probabilities. " +
		"If the result is unexpectedly far in the future, warn the user that throughput sampling may be too low due to filtered resolutions or issue types.\n\n" +
		"STATIONARITY ASSESSMENT: The result includes 'stationarity_assessment' in the 'context' field. " +
		"When 'stationary' is false, surface the warnings to the user and suggest re-running with 'recommended_window_days'. " +
		"Run 'forecast_backtest' first when stationarity is uncertain.",

	"forecast_backtest": "Validates Monte-Carlo forecast accuracy via Walk-Forward Analysis — reconstructs past system states and checks whether actual outcomes fell within predicted ranges.\n\n" +
		"WHEN TO USE: Before committing to a forecast when stationarity is uncertain. " +
		"User asks: 'How accurate are our forecasts historically?', 'Should we trust the Monte Carlo result?'\n\n" +
		"PARAMETER GUIDANCE:\n" +
		"- simulation_mode: Use the same decision rule as 'forecast_monte_carlo' — duration for deadline questions, scope for capacity questions.\n" +
		"- history_window_days: Controls how many checkpoints are generated (default 175 days = ~25 weekly checkpoints). " +
		"This is NOT the per-checkpoint sampling window — each checkpoint always samples from a fixed 90-day window ending at that point.\n\n" +
		"INTERPRETATION: Key field is 'stationarity_correlation.signal'. " +
		"'predictive' means non-stationary checkpoints miss at >2x the rate of stationary ones — the stationarity guardrail is validated for this project; surface stationarity warnings prominently. " +
		"'not_predictive' means both groups miss at similar rates — stationarity may not be the main accuracy driver here.",

	// ── GROUP: Import & Setup ─────────────────────────────────────────────────
	// Canonical setup sequence is documented in serverInstructions (instructions.go).

	"import_projects": "Searches for Jira projects by name or key.\n\n" +
		"Next step: call 'import_boards' with the project key to find the board ID needed for all analytical tools.",

	"import_boards": "Searches for Agile boards, optionally filtering by project key or name.\n\n" +
		"Next step: call 'import_board_context' with the board ID to anchor the data shape context.",

	"import_board_context": "Returns a Data Shape Anchor — whole dataset volumes vs. sample distributions — for a specific Agile board.\n\n" +
		"MUST be called before 'workflow_discover_mapping'. Next step: call 'workflow_discover_mapping'.",

	"import_project_context": "Returns a Data Shape Anchor for a project (not board-level). Use for general project metadata only.\n\n" +
		"NOTE: All analytical tools require a Board ID. If you plan to run diagnostics or forecasts, use 'import_board_context' instead.",

	"import_history_update": "Incrementally fetches items changed since the last sync to keep the local cache current.\n\n" +
		"WHEN TO USE: At the start of any session to ensure analysis reflects recent Jira changes. This is a lightweight forward-only sync. " +
		"To extend history further back than the current cache, raise INGESTION_CREATED_LOOKBACK / INGESTION_UPDATED_LOOKBACK in .env and re-hydrate via 'import_board_context' (after deleting the existing cache file).",

	"workflow_discover_mapping": "Probes status categories, residence times, and resolution frequencies to propose a semantic workflow mapping for user verification.\n\n" +
		"AI MUST present the proposed tier mapping AND the 'status_order' array to the user for verification. " +
		"After user confirms or corrects BOTH, AI MUST call 'workflow_set_mapping' AND 'workflow_set_order' to persist them. " +
		"Without persisting both, all Diagnostics tools will return subpar or incorrect results.\n\n" +
		"METAWORKFLOW GUIDANCE:\n" +
		"- TIERS: 'Demand' (Backlog), 'Upstream' (Analysis/Refinement), 'Downstream' (Development/Execution/Testing), 'Finished' (Terminal).\n" +
		"- ROLES: 'active' (Value-adding work), 'queue' (Waiting), 'ignore' (Admin). Not applicable for 'Finished' tier.\n" +
		"- OUTCOMES: 'delivered' (Value Provided), 'abandoned' (Work Discarded).\n" +
		"- OUTCOME HIERARCHY: Jira Resolutions (Primary) > Finished-tier Status mapping (Secondary).",

	"workflow_set_mapping": "Persists user-confirmed semantic metadata (tier, role, outcome) for statuses and resolutions.\n\n" +
		"This is the MANDATORY persistence step after the user verifies the mapping from 'workflow_discover_mapping'. " +
		"WITHOUT this step, ALL analytical tools will return subpar or incorrect results.\n\n" +
		"AI MUST verify with the user before calling:\n" +
		"1. Tier assignments (Demand, Upstream, Downstream, Finished) for all statuses.\n" +
		"2. Commitment Point: the first Downstream status where the clock starts.\n" +
		"3. Outcomes: only required for Finished-tier statuses when Jira resolutions are missing or unreliable.\n\n" +
		"METAWORKFLOW GUIDANCE:\n" +
		"- TIERS: 'Demand' (Backlog), 'Upstream' (Analysis/Refinement), 'Downstream' (Development/Execution/Testing), 'Finished' (Terminal).\n" +
		"- ROLES: 'active' (Value-adding work), 'queue' (Waiting), 'ignore' (Admin). Omit for 'Finished' tier.\n" +
		"- OUTCOMES: 'delivered' (Successfully finished with value), 'abandoned' (Work stopped/discarded/cancelled).",

	"workflow_set_order": "Persists the user-confirmed chronological order of workflow statuses.\n\n" +
		"MUST be called after 'workflow_discover_mapping' once the user has verified or corrected the proposed 'status_order'. " +
		"This order drives CFD charts, flow debt analysis, and all range-based analytics. " +
		"If the user accepts the discovered order unchanged, pass it back as-is.",

	"workflow_set_evaluation_date": "Sets a custom evaluation date so all time-based calculations use that date instead of today.\n\n" +
		"WHEN TO USE: Historical scenario analysis, or when the user wants to evaluate the system state as of a specific past date.",

	"set_analysis_window": "Sets the session analysis window — a single [start, end] range that ALL windowed diagnostics use.\n\n" +
		"WHEN TO USE: When the user wants to scope multiple analyses to the same period (e.g. 'analyse Q1', 'look at the last 8 weeks', 'move one month back'). " +
		"Translate relative requests like 'one month back' to absolute dates and call this tool with start_date/end_date or end_date/duration_days.\n\n" +
		"PARAMETER GUIDANCE:\n" +
		"- Provide EITHER start_date OR duration_days, not both. end_date defaults to today (or the active evaluation date).\n" +
		"- reset=true clears the window and restores the default rolling 26-week range.\n\n" +
		"SCOPE: Affects every diagnostic that operates on a historical range (throughput, WIP stability, flow debt, cycle time, etc.). " +
		"analyze_work_item_age uses ONLY the End (point-in-time snapshot). " +
		"analyze_process_evolution uses ONLY the End (anchor for a fixed long-term lookback — 12 months / 26 weeks). " +
		"forecast_monte_carlo and forecast_backtest are NOT affected — forecasting keeps its own sampling window auto-sized by the simulation engine.\n\n" +
		"PERSISTENCE: In-memory only. Resets on board switch and on server restart.",

	"get_analysis_window": "Returns the currently active session analysis window and its source ('session' if explicitly set, 'default' otherwise).\n\n" +
		"WHEN TO USE: To verify which window will scope subsequent diagnostics before running them, or to confirm a 'set_analysis_window' call took effect.",

	"guide_diagnostic_roadmap": "Returns a recommended sequence of analysis steps tailored to a specific analytical goal.\n\n" +
		"WHEN TO USE: At the start of a session when the user's goal is clear but the right tool sequence is not. " +
		"Goals: 'forecasting', 'bottlenecks', 'capacity_planning', 'system_health'.",

	"open_in_browser": "Opens a chart render URL in the system default browser.\n\n" +
		"Use this tool whenever you receive a 'chart_url' from an analysis tool. " +
		"Only localhost render-charts URLs are accepted; all other URLs are rejected. " +
		"Do not use any external browser-control tool for this purpose.",
}

// customSchemas maps Go enum types to their JSON Schema representations.
var customSchemas = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[SimulationMode]():  {Type: "string", Enum: []any{SimModeDuration, SimModeScope}},
	reflect.TypeFor[AgeType]():         {Type: "string", Enum: []any{AgeTypeTotal, AgeTypeWIP}},
	reflect.TypeFor[TierFilter]():      {Type: "string", Enum: []any{TierFilterWIP, TierFilterDemand, TierFilterUpstream, TierFilterDownstream, TierFilterFinished, TierFilterAll}},
	reflect.TypeFor[DiagnosticGoal]():  {Type: "string", Enum: []any{GoalForecasting, GoalBottlenecks, GoalCapacityPlanning, GoalSystemHealth}},
	reflect.TypeFor[Granularity]():     {Type: "string", Enum: []any{GranularityDaily, GranularityWeekly}},
	reflect.TypeFor[WorkflowTier]():    {Type: "string", Enum: []any{TierDemand, TierUpstream, TierDownstream, TierFinished}},
	reflect.TypeFor[WorkflowRole]():    {Type: "string", Enum: []any{RoleActive, RoleQueue, RoleIgnore}},
	reflect.TypeFor[WorkflowOutcome](): {Type: "string", Enum: []any{OutcomeDelivered, OutcomeAbandoned}},
}

// schemaFor infers a JSON Schema for type T with custom enum type mappings.
func schemaFor[T any]() (*jsonschema.Schema, error) {
	s, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: customSchemas})
	if err != nil {
		return nil, fmt.Errorf("schemaFor[%T]: %w", *new(T), err)
	}
	return s, nil
}

// formatToolResult wraps handler output into an MCP CallToolResult with text content.
func formatToolResult(s *Server, data any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: s.formatResult(data)},
		},
	}
}

// formatToolError wraps an error into an MCP CallToolResult with IsError set.
func formatToolError(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: err.Error()},
		},
		IsError: true,
	}
}

// registerTools wires all MCP tools to their handler methods on the Server.
// All tools use the generic mcp.AddTool with typed input structs.
// Enum types are mapped to JSON Schema via customSchemas + schemaFor.
func registerTools(mcpSrv *mcp.Server, s *Server) error {
	var errs []error
	must := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	// GROUP: Import & Setup
	//   import_projects, import_boards, import_board_context, import_project_context,
	//   import_history_update,
	//   workflow_discover_mapping, workflow_set_mapping, workflow_set_order,
	//   workflow_set_evaluation_date, guide_diagnostic_roadmap, open_in_browser

	must(addTool(mcpSrv, s, "import_projects",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportProjectsInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleImportProjects(args.Query)
			return handleResult(s, "import_projects", data, err)
		}))

	must(addTool(mcpSrv, s, "import_boards",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportBoardsInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleImportBoards(args.ProjectKey, args.NameFilter)
			return handleResult(s, "import_boards", data, err)
		}))

	must(addTool(mcpSrv, s, "import_project_context",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportProjectContextInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetProjectDetails(args.ProjectKey)
			return handleResult(s, "import_project_context", data, err)
		}))

	must(addTool(mcpSrv, s, "import_board_context",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportBoardContextInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetBoardDetails(args.ProjectKey, args.BoardID)
			return handleResult(s, "import_board_context", data, err)
		}))

	// GROUP: Forecast & Simulation
	//   forecast_monte_carlo, forecast_backtest

	must(addTool(mcpSrv, s, "forecast_monte_carlo",
		func(_ context.Context, _ *mcp.CallToolRequest, args ForecastMonteCarloInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleRunSimulation(
				args.ProjectKey, args.BoardID, string(args.Mode),
				args.IncludeExistingBacklog, args.AdditionalItems,
				args.TargetDays, args.TargetDate,
				args.StartStatus, args.EndStatus,
				args.IssueTypes, args.IncludeWIP,
				args.HistoryWindowDays, args.HistoryStartDate, args.HistoryEndDate,
				args.Targets, args.MixOverrides,
			)
			return handleResult(s, "forecast_monte_carlo", data, err)
		}))

	// GROUP: Diagnostics — Process, Cycle Time, WIP & Flow
	//   analyze_cycle_time, analyze_process_stability, analyze_process_evolution,
	//   analyze_status_persistence, analyze_throughput, analyze_wip_stability,
	//   analyze_wip_age_stability, analyze_work_item_age, analyze_flow_debt,
	//   analyze_residence_time, analyze_yield, generate_cfd_data, analyze_item_journey

	must(addTool(mcpSrv, s, "analyze_cycle_time",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeCycleTimeInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetCycleTimeAssessment(args.ProjectKey, args.BoardID, args.StartStatus, args.EndStatus, args.IssueTypes, args.SLEPercentile, args.SLEDurationDays)
			return handleResult(s, "analyze_cycle_time", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_status_persistence",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeStatusPersistenceInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetStatusPersistence(args.ProjectKey, args.BoardID)
			return handleResult(s, "analyze_status_persistence", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_work_item_age",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeWorkItemAgeInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetAgingAnalysis(args.ProjectKey, args.BoardID, string(args.AgeType), string(args.TierFilter))
			return handleResult(s, "analyze_work_item_age", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_throughput",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeThroughputInput) (*mcp.CallToolResult, any, error) {
			bucket := args.Bucket
			if bucket == "" {
				bucket = "week"
			}
			data, err := s.handleGetDeliveryCadence(args.ProjectKey, args.BoardID, bucket, args.IncludeAbandoned)
			return handleResult(s, "analyze_throughput", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_process_stability",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeProcessStabilityInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetProcessStability(args.ProjectKey, args.BoardID, args.IncludeRawSeries)
			return handleResult(s, "analyze_process_stability", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_flow_debt",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeFlowDebtInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetFlowDebt(args.ProjectKey, args.BoardID, args.BucketSize)
			return handleResult(s, "analyze_flow_debt", data, err)
		}))

	must(addTool(mcpSrv, s, "generate_cfd_data",
		func(_ context.Context, _ *mcp.CallToolRequest, args GenerateCFDDataInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetCFDData(args.ProjectKey, args.BoardID, string(args.Granularity))
			return handleResult(s, "generate_cfd_data", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_wip_stability",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeWIPStabilityInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleAnalyzeWIPStability(args.ProjectKey, args.BoardID)
			return handleResult(s, "analyze_wip_stability", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_wip_age_stability",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeWIPAgeStabilityInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleAnalyzeWIPAgeStability(args.ProjectKey, args.BoardID)
			return handleResult(s, "analyze_wip_age_stability", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_residence_time",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeResidenceTimeInput) (*mcp.CallToolResult, any, error) {
			granularity := string(args.Granularity)
			if granularity == "weekly" {
				granularity = "week"
			} else if granularity == "" {
				granularity = "day"
			}
			data, err := s.handleAnalyzeResidenceTime(args.ProjectKey, args.BoardID, args.IssueTypes, granularity)
			return handleResult(s, "analyze_residence_time", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_process_evolution",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeProcessEvolutionInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetProcessEvolution(args.ProjectKey, args.BoardID, args.Bucket)
			return handleResult(s, "analyze_process_evolution", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_yield",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeYieldInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetProcessYield(args.ProjectKey, args.BoardID)
			return handleResult(s, "analyze_yield", data, err)
		}))

	must(addTool(mcpSrv, s, "workflow_discover_mapping",
		func(_ context.Context, _ *mcp.CallToolRequest, args WorkflowDiscoverMappingInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetWorkflowDiscovery(args.ProjectKey, args.BoardID, args.ForceRefresh)
			return handleResult(s, "workflow_discover_mapping", data, err)
		}))

	must(addTool(mcpSrv, s, "workflow_set_mapping",
		func(_ context.Context, _ *mcp.CallToolRequest, args WorkflowSetMappingInput) (*mcp.CallToolResult, any, error) {
			// Convert typed structs to map[string]any for the handler interface
			mappingAny := make(map[string]any, len(args.Mapping))
			for k, v := range args.Mapping {
				entry := map[string]any{"tier": string(v.Tier)}
				if v.Role != "" {
					entry["role"] = string(v.Role)
				}
				if v.Outcome != "" {
					entry["outcome"] = string(v.Outcome)
				}
				mappingAny[k] = entry
			}
			var resolutionsAny map[string]any
			if len(args.Resolutions) > 0 {
				resolutionsAny = make(map[string]any, len(args.Resolutions))
				for k, v := range args.Resolutions {
					resolutionsAny[k] = string(v)
				}
			}
			data, err := s.handleSetWorkflowMapping(args.ProjectKey, args.BoardID, mappingAny, resolutionsAny, args.CommitmentPoint)
			return handleResult(s, "workflow_set_mapping", data, err)
		}))

	must(addTool(mcpSrv, s, "workflow_set_order",
		func(_ context.Context, _ *mcp.CallToolRequest, args WorkflowSetOrderInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleSetWorkflowOrder(args.ProjectKey, args.BoardID, args.Order)
			return handleResult(s, "workflow_set_order", data, err)
		}))

	must(addTool(mcpSrv, s, "workflow_set_evaluation_date",
		func(_ context.Context, _ *mcp.CallToolRequest, args WorkflowSetEvaluationDateInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleSetEvaluationDate(args.ProjectKey, args.BoardID, args.Date)
			return handleResult(s, "workflow_set_evaluation_date", data, err)
		}))

	must(addTool(mcpSrv, s, "set_analysis_window",
		func(_ context.Context, _ *mcp.CallToolRequest, args SetAnalysisWindowInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleSetAnalysisWindow(args.StartDate, args.EndDate, args.DurationDays, args.Reset)
			return handleResult(s, "set_analysis_window", data, err)
		}))

	must(addTool(mcpSrv, s, "get_analysis_window",
		func(_ context.Context, _ *mcp.CallToolRequest, _ GetAnalysisWindowInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetAnalysisWindow()
			return handleResult(s, "get_analysis_window", data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_item_journey",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeItemJourneyInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetItemJourney(args.ProjectKey, args.BoardID, args.IssueKey)
			return handleResult(s, "analyze_item_journey", data, err)
		}))

	must(addTool(mcpSrv, s, "guide_diagnostic_roadmap",
		func(_ context.Context, _ *mcp.CallToolRequest, args GuideDiagnosticRoadmapInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetDiagnosticRoadmap(string(args.Goal))
			return handleResult(s, "guide_diagnostic_roadmap", data, err)
		}))

	must(addTool(mcpSrv, s, "forecast_backtest",
		func(_ context.Context, _ *mcp.CallToolRequest, args ForecastBacktestInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetForecastAccuracy(
				args.ProjectKey, args.BoardID, string(args.SimulationMode),
				args.ItemsToForecast, args.ForecastHorizon,
				args.IssueTypes, args.HistoryWindowDays,
				args.HistoryStartDate, args.HistoryEndDate,
			)
			return handleResult(s, "forecast_backtest", data, err)
		}))

	must(addTool(mcpSrv, s, "import_history_update",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportHistoryUpdateInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleCacheCatchUp(args.ProjectKey, args.BoardID)
			return handleResult(s, "import_history_update", data, err)
		}))

	must(addTool(mcpSrv, s, "open_in_browser",
		func(_ context.Context, _ *mcp.CallToolRequest, args OpenInBrowserInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleOpenInBrowser(args.URL)
			return handleResult(s, "open_in_browser", data, err)
		}))

	return errors.Join(errs...)
}

// addTool registers a tool with the SDK using the generic mcp.AddTool API.
// If the input type contains custom enum types, it pre-builds the schema
// with customSchemas to include enum constraints.
func addTool[In any](mcpSrv *mcp.Server, s *Server, name string, handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error)) error {
	schema, err := schemaFor[In]()
	if err != nil {
		return fmt.Errorf("tool %q: %w", name, err)
	}
	desc := toolDescriptions[name]
	tool := &mcp.Tool{
		Name:        name,
		Description: desc,
		InputSchema: schema,
	}
	mcp.AddTool(mcpSrv, tool, withPanicRecovery(name, handler))
	return nil
}

// withPanicRecovery wraps a tool handler with panic recovery and logging.
func withPanicRecovery[In any](name string, handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args In) (result *mcp.CallToolResult, out any, err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				log.Error().
					Interface("panic", r).
					Str("tool", name).
					Str("stack", stack).
					Msg("Panic recovered in tool handler")
				result = &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("Internal error: %v", r)},
					},
					IsError: true,
				}
				err = nil
			}
		}()
		log.Info().Str("tool", name).Msg("Processing tool call")
		result, out, err = handler(ctx, req, args)
		if err == nil {
			log.Info().Str("tool", name).Msg("Tool call completed successfully")
		} else {
			log.Error().Err(err).Str("tool", name).Msg("Tool call failed")
		}
		return
	}
}

// handleResult converts a (data, error) pair to the SDK's 3-return convention.
// For chart-eligible tools, it also pushes the result into the MRU buffer and
// injects a chart_url into the response context. It also injects session_context
// so the agent always sees which analysis window shaped the output.
func handleResult(s *Server, toolName string, data any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return formatToolError(err), nil, nil
	}
	data = s.injectSessionContext(data)
	data = s.injectChartURL(toolName, data)
	return formatToolResult(s, data), nil, nil
}
