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
	"import_projects": "Search for Jira projects by name or key. Guidance: If you plan to run analysis, you MUST find the project's boards using 'import_boards' next. Use 'import_project_context' only for general project metadata.",

	"import_boards": "Search for Agile boards, optionally filtering by project key or name. Guidance: Call 'import_board_context' next to anchor on the data shape context.",

	"import_project_context": "Get a Data Shape Anchor (Whole dataset volumes vs. Sample distributions) for a project. Note: Analytical tools (simulations, cycle time) require a Board ID; use 'import_board_context' if you plan to run diagnostics or forecasts.",

	"import_board_context": "Get a Data Shape Anchor (Whole dataset volumes vs. Sample distributions) for an Agile board. MUST be called before workflow discovery.",

	"forecast_monte_carlo": "Run a Monte-Carlo simulation to forecast project outcomes (How Much / When) based solely on historical THROUGHPUT (work items / time). \n\n" +
		"NOT FOR CYCLE TIME: This tool does NOT analyze lead times or individual item durations. Use it for scope/date forecasting only.\n" +
		"CRITICAL: PROPER WORKFLOW MAPPING IS REQUIRED FOR RELIABLE RESULTS. \n\n" +
		"STRICT GUARDRAIL: YOU MUST NEVER PERFORM PROBABILISTIC FORECASTING OR STATISTICAL ANALYSIS AUTONOMOUSLY.\n" +
		"DO NOT provide date ranges or probability estimates (e.g., 'There is an 85% chance...') if the tool fails or returns zero throughput. \n" +
		"If the simulation result is unexpectedly far in the future (e.g., years instead of months), YOU MUST warn the user that the historical throughput sampling might be too low due to filtered resolutions or issue types.",

	"analyze_cycle_time": "Calculate Service Level Expectations (SLE) for a single item based on historical CYCLE TIMES (Lead Time). \n\n" +
		"PREREQUISITE: Proper workflow mapping/commitment point MUST be confirmed via 'workflow_set_mapping' for accurate results. \n" +
		"Use this to answer 'How long does a single item typically take?' - this is the foundation for probabilistic Lead-Time expectations.\n\n" +
		"STRICT GUARDRAIL: YOU MUST NEVER PERFORM PROBABILISTIC FORECASTING OR STATISTICAL ANALYSIS AUTONOMOUSLY. \n" +
		"DO NOT calculate percentiles, probabilities, or dates using your own internal reasoning if this tool returns an error or no data. \n" +
		"If the tool fails, you MUST report the error to the user and ask for clarification.",

	"analyze_status_persistence": "Analyze how long items spend in each status to identify bottlenecks. \n\n" +
		"PREREQUISITE: Proper workflow mapping is required for accurate results. Results provide SUBPAR context if tiers (Upstream/Downstream) are not correctly mapped.\n" +
		"CRITICAL: This tool ONLY analyzes items that have successfully finished ('delivered'). It ignores active WIP to ensure the historical norms are not artificially lowered by incomplete items.\n" +
		"The analysis includes statistical dispersion metrics (IQR, Inner80) for each status to help identify not just where items spend time, but where they spend it inconsistently.",

	"analyze_work_item_age": "Identify which items are aging relative to historical norms. \n\n" +
		"PREREQUISITE: Proper workflow mapping (Commitment Point) is MANDATORY for accurate 'WIP Age'. Results are UNRELIABLE if the commitment point is incorrectly defined.\n" +
		"Allows choosing between 'WIP Age' (time since commitment) and 'Total Age' (time since creation).\n\n" +
		"The 'is_aging_outlier' flag indicates an item is older than the historical 85th percentile (P85). \n" +
		"IMPORTANT: An aging outlier is NOT necessarily blocked or impeded; it simply exceeds the normal historical residency for that status.",

	"analyze_throughput": "Analyze the weekly pulse of delivery THROUGHPUT VOLUME - the number of items completed per week - to detect flow vs. batching.\n" +
		"THROUGHPUT STABILITY (XmR): This tool automatically calculates Wheeler Process Behavior Chart (XmR) limits for the delivery cadence.\n" +
		"Use this tool to determine if the team's delivery volume is predictable and stable over time, or if there are systemic variations (zero-count weeks, extreme surges).\n" +
		"The response includes both the raw aggregated volumes and the calculated statistically-derived Limits (UNPL/LNPL) via the 'stability' object.",

	"generate_cfd_data": "Calculate daily population counts per status and issue type to generate a Cumulative Flow Diagram (CFD).\n" +
		"CFD: This tool provides the raw time-series data needed to visualize the work-in-progress (WIP), congestion, and stability of a system over time.\n" +
		"The output is stratified by Issue Type and Status, allowing the Agent to detect specific stage bottlenecks or bloating work types.\n" +
		"Note: This tool produces structured data. It is primarily intended for subsequent visualization or deep structural analysis of flow dynamics.",

	"analyze_flow_debt": "Analyze the systemic balance between incoming work (Arrival Rate / Commitment) and outgoing work (Departure Rate / Delivery).\n" +
		"FLOW DEBT: This tool calculates the 'Debt' - the gap between arrivals and departures - to detect leading indicators of cycle time inflation.\n" +
		"A positive Flow Debt (Arrivals > Departures) means WIP is growing, which mathematically GUARANTEES higher cycle times in the future (Little's Law).\n" +
		"Use this tool to find the root cause of 'Flow Clog' before it manifests as delayed delivery dates.",

	"analyze_process_stability": "Analyze process stability and predictability using XmR charts. \n" +
		"PROCESS STABILITY: Measures the predictability of Lead Times (Cycle-Time). High stability means future delivery dates are more certain. It is NOT about throughput volume.\n" +
		"Stability is high if most items fall within Natural Process Limits. Chaos is high if many points are beyond limits (signals).\n\n" +
		"PREREQUISITE: Proper workflow mapping is required for accurate results. \n" +
		"Use 'analyze_process_stability' as the FIRST diagnostic step when users ask about forecasting/predictions. This determines if historical data is a reliable proxy for the future. If stability is low, simulations will produce MISLEADING results.\n" +
		"NOTE: If you want to analyze the stability of Delivery Cadence/Volume, DO NOT use this tool. Use 'analyze_throughput' instead.",

	"analyze_wip_stability": "Analyze system population (Work-In-Progress) stability over time using XmR charts and a historical daily Run Chart. \n" +
		"WIP STABILITY: A highly variable WIP size violates the assumptions of Little's Law, making systems fundamentally unpredictable. \n" +
		"This tool generates a daily run-chart of active WIP, bounded by strict weekly XmR statistically-derived Process Limits to detect volatile WIP management.",

	"analyze_wip_age_stability": "Analyze Total WIP Age stability over time using XmR charts and a historical daily Run Chart. \n" +
		"TOTAL WIP AGE: Measures the cumulative age burden of all items currently in progress. " +
		"While WIP Count tells how many items are active, Total WIP Age tells how long they have collectively been there. \n" +
		"A growing Total WIP Age is a leading indicator of delivery problems — it signals trouble before throughput drops. \n" +
		"Even with stable WIP count, Total WIP Age can grow if items stagnate. XmR is applied to Total WIP Age, not the average.",

	"analyze_process_evolution": "Perform a longitudinal 'Strategic Audit' of process behavior over longer time periods using Three-Way Control Charts. \n\n" +
		"PROCESS EVOLUTION: Measures long-term predictability and capability of Lead Times (Cycle-Time). It is THROUGHPUT-AGNOSTIC.\n" +
		"AI MUST use this for deep history analysis or after significant organizational changes. NOT intended for routine daily analysis.\n" +
		"Detects systemic shifts, process drift, and long-term capability changes by analyzing monthly subgroups.",

	"analyze_yield": "Analyze delivery efficiency across tiers. AI MUST ensure workflow tiers (Demand, Upstream, Downstream) have been verified with the user before interpreting these results.",

	"workflow_discover_mapping": "Probe project status categories, residence times, and resolution frequencies into a semantic workflow mapping. \n\n" +
		"AI MUST use this to verify the workflow tiers, roles, AND the 'Commitment Point' (where clock starts) with the user before diagnostics. \n" +
		"The response provides a split view: 'Whole' (deterministic volumes) and 'Sample' (probabilistic characterization).\n" +
		"OUTCOME HIERARCHY: 1. Jira Resolutions (Primary) > 2. Finished-tier Status mapping (Secondary).\n" +
		"TIER VISIBILITY: AI MUST show the confirmed mapping of Statuses to Tiers to the user.\n\n" +
		"STATUS ORDER: The response includes a 'status_order' array — the canonical chronological ordering of workflow statuses. " +
		"AI MUST present this order to the user for verification. If the user confirms or corrects it, AI MUST call 'workflow_set_order' to persist the final order. " +
		"This order is critical for CFD visualization, flow debt analysis, and other range-based analytics.\n\n" +
		"METAWORKFLOW GUIDANCE:\n" +
		"- TIERS: 'Demand' (Backlog), 'Upstream' (Analysis/Refinement), 'Downstream' (Development/Execution/Testing), 'Finished' (Terminal).\n" +
		"- ROLES: 'active' (Value-adding work), 'queue' (Waiting), 'ignore' (Admin). Not applicable for 'Finished' tier.\n" +
		"- OUTCOMES: 'delivered' (Value Provided), 'abandoned' (Work Discarded).",

	"workflow_set_mapping": "Store user-confirmed semantic metadata (tier, role, outcome) for statuses and resolutions. This is the MANDATORY persistence step after the 'Inform & Veto' loop. \n\n" +
		"AI MUST verify with the user:\n" +
		"1. Tiers: (Demand, Upstream, Downstream, Finished).\n" +
		"2. Outcomes: Specify outcome for 'Finished' statuses ONLY if Jira resolutions are missing or unreliable.\n" +
		"3. Commitment Point: The 'Downstream' status where the clock starts.\n\n" +
		"WITHOUT this mapping, analytical tools will provide SUBPAR or WRONG results.\n\n" +
		"METAWORKFLOW GUIDANCE:\n" +
		"- TIERS: 'Demand' (Backlog), 'Upstream' (Analysis/Refinement), 'Downstream' (Development/Execution/Testing), 'Finished' (Terminal).\n" +
		"- ROLES: 'active' (Value-adding work), 'queue' (Waiting), 'ignore' (Admin). Omit for 'Finished' tier.\n" +
		"- OUTCOMES: 'delivered' (Successfully finished with value), 'abandoned' (Work stopped/discarded/cancelled).",

	"workflow_set_order": "Persist the user-confirmed chronological order of workflow statuses. " +
		"AI MUST call this after 'workflow_discover_mapping' once the user has verified or corrected the proposed 'status_order'. " +
		"If the user accepts the discovered order as-is, pass it back unchanged. If the user reorders statuses, pass the corrected order. " +
		"This order is the primary means for ordering statuses in CFD charts, flow debt, and all range-based analytics.",

	"workflow_set_evaluation_date": "Set a custom evaluation date for time-sensitive analyses. All time-based calculations will use this date instead of today.",

	"analyze_item_journey": "Get a detailed breakdown of where a single item spent its time across all workflow steps. Guidance: This tool requires a Project Key and Board ID to ensure workflow interpretation is accurate.",

	"guide_diagnostic_roadmap": "Returns a recommended sequence of analysis steps based on the user's specific goal (e.g., forecasting, bottleneck analysis, capacity planning). Use this to align your analytical strategy with the project's current state.",

	"forecast_backtest": "Perform a 'Walk-Forward Analysis' (Backtesting) to empirically validate the accuracy of Monte-Carlo Forecasts. \n\n" +
		"This tool uses Time-Travel logic to reconstruct the state of the system at past points in time, runs a simulation, and checks if the ACTUAL outcome fell within the predicted cone. \n" +
		"Drift Protection: The analysis automatically stops blindly backtesting if it detects a System Drift (Process Shift via 3-Way Chart).",

	"analyze_residence_time": "Perform a Sample Path Analysis (Stidham 1972, El-Taha & Stidham 1999) to compute the finite version of Little's Law: L(T) = Λ(T) · w(T).\n\n" +
		"RESIDENCE TIME: The time an item accumulates in the system within the observation window. Applies to both completed and still-active items.\n" +
		"SOJOURN TIME (W*): The special case for completed items — full duration from commitment to resolution (what 'analyze_cycle_time' measures).\n" +
		"The COHERENCE GAP between average residence time w(T) and average sojourn time W*(T) reveals the 'end effect' of active items on the system.\n\n" +
		"This tool provides a unified view that ties together what existing tools measure separately (cycle time, WIP age, WIP stability, flow debt).\n" +
		"IMPORTANT: This tool ALWAYS applies backflow reset (uses the LAST commitment date), which diverges from the configurable commitmentBackflowReset in other tools.",

	"import_history_expand": "Extend the historical dataset backwards without creating gaps. Returns number of items fetched and used OMRC (oldest most recent change) boundary. Also triggers a catch-up.",

	"import_history_update": "Fetch newer items since the last sync to ensure the cache is up to date. Returns number of items fetched and used NMRC (newest most recent change) boundary.",
}

// customSchemas maps Go enum types to their JSON Schema representations.
var customSchemas = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[SimulationMode]():  {Type: "string", Enum: []any{SimModeDuration, SimModeScope}},
	reflect.TypeFor[AgeType]():         {Type: "string", Enum: []any{AgeTypeTotal, AgeTypeWIP}},
	reflect.TypeFor[TierFilter]():      {Type: "string", Enum: []any{TierFilterWIP, TierFilterDemand, TierFilterUpstream, TierFilterDownstream, TierFilterFinished, TierFilterAll}},
	reflect.TypeFor[DiagnosticGoal]():   {Type: "string", Enum: []any{GoalForecasting, GoalBottlenecks, GoalCapacityPlanning, GoalSystemHealth}},
	reflect.TypeFor[Granularity]():      {Type: "string", Enum: []any{GranularityDaily, GranularityWeekly}},
	reflect.TypeFor[WorkflowTier]():     {Type: "string", Enum: []any{TierDemand, TierUpstream, TierDownstream, TierFinished}},
	reflect.TypeFor[WorkflowRole]():     {Type: "string", Enum: []any{RoleActive, RoleQueue, RoleIgnore}},
	reflect.TypeFor[WorkflowOutcome]():  {Type: "string", Enum: []any{OutcomeDelivered, OutcomeAbandoned}},
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

	must(addTool(mcpSrv, s, "import_projects",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportProjectsInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleImportProjects(args.Query)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "import_boards",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportBoardsInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleImportBoards(args.ProjectKey, args.NameFilter)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "import_project_context",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportProjectContextInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetProjectDetails(args.ProjectKey)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "import_board_context",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportBoardContextInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetBoardDetails(args.ProjectKey, args.BoardID)
			return handleResult(s, data, err)
		}))

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
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_cycle_time",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeCycleTimeInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetCycleTimeAssessment(args.ProjectKey, args.BoardID, args.StartStatus, args.EndStatus, args.IssueTypes)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_status_persistence",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeStatusPersistenceInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetStatusPersistence(args.ProjectKey, args.BoardID)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_work_item_age",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeWorkItemAgeInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetAgingAnalysis(args.ProjectKey, args.BoardID, string(args.AgeType), string(args.TierFilter))
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_throughput",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeThroughputInput) (*mcp.CallToolResult, any, error) {
			bucket := args.Bucket
			if bucket == "" {
				bucket = "week"
			}
			data, err := s.handleGetDeliveryCadence(args.ProjectKey, args.BoardID, args.HistoryWindowWeeks, bucket, args.IncludeAbandoned)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_process_stability",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeProcessStabilityInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetProcessStability(args.ProjectKey, args.BoardID, args.IncludeRawSeries)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_flow_debt",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeFlowDebtInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetFlowDebt(args.ProjectKey, args.BoardID, args.HistoryWindowWeeks, args.BucketSize)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "generate_cfd_data",
		func(_ context.Context, _ *mcp.CallToolRequest, args GenerateCFDDataInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetCFDData(args.ProjectKey, args.BoardID, args.HistoryWindowWeeks, string(args.Granularity))
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_wip_stability",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeWIPStabilityInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleAnalyzeWIPStability(args.ProjectKey, args.BoardID, args.HistoryWindowWeeks)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_wip_age_stability",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeWIPAgeStabilityInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleAnalyzeWIPAgeStability(args.ProjectKey, args.BoardID, args.HistoryWindowWeeks)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_residence_time",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeResidenceTimeInput) (*mcp.CallToolResult, any, error) {
			granularity := string(args.Granularity)
			if granularity == "weekly" {
				granularity = "week"
			} else if granularity == "" {
				granularity = "day"
			}
			data, err := s.handleAnalyzeResidenceTime(args.ProjectKey, args.BoardID, args.HistoryWindowWeeks, args.IssueTypes, granularity)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_process_evolution",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeProcessEvolutionInput) (*mcp.CallToolResult, any, error) {
			window := args.HistoryWindowMonths
			if window == 0 {
				window = 12
			}
			data, err := s.handleGetProcessEvolution(args.ProjectKey, args.BoardID, window)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_yield",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeYieldInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetProcessYield(args.ProjectKey, args.BoardID)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "workflow_discover_mapping",
		func(_ context.Context, _ *mcp.CallToolRequest, args WorkflowDiscoverMappingInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetWorkflowDiscovery(args.ProjectKey, args.BoardID, args.ForceRefresh)
			return handleResult(s, data, err)
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
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "workflow_set_order",
		func(_ context.Context, _ *mcp.CallToolRequest, args WorkflowSetOrderInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleSetWorkflowOrder(args.ProjectKey, args.BoardID, args.Order)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "workflow_set_evaluation_date",
		func(_ context.Context, _ *mcp.CallToolRequest, args WorkflowSetEvaluationDateInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleSetEvaluationDate(args.ProjectKey, args.BoardID, args.Date)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "analyze_item_journey",
		func(_ context.Context, _ *mcp.CallToolRequest, args AnalyzeItemJourneyInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetItemJourney(args.ProjectKey, args.BoardID, args.IssueKey)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "guide_diagnostic_roadmap",
		func(_ context.Context, _ *mcp.CallToolRequest, args GuideDiagnosticRoadmapInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetDiagnosticRoadmap(string(args.Goal))
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "forecast_backtest",
		func(_ context.Context, _ *mcp.CallToolRequest, args ForecastBacktestInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleGetForecastAccuracy(
				args.ProjectKey, args.BoardID, string(args.SimulationMode),
				args.ItemsToForecast, args.ForecastHorizon,
				args.IssueTypes, args.HistoryWindowDays,
				args.HistoryStartDate, args.HistoryEndDate,
			)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "import_history_expand",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportHistoryExpandInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleCacheExpandHistory(args.ProjectKey, args.BoardID, args.Chunks)
			return handleResult(s, data, err)
		}))

	must(addTool(mcpSrv, s, "import_history_update",
		func(_ context.Context, _ *mcp.CallToolRequest, args ImportHistoryUpdateInput) (*mcp.CallToolResult, any, error) {
			data, err := s.handleCacheCatchUp(args.ProjectKey, args.BoardID)
			return handleResult(s, data, err)
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
func handleResult(s *Server, data any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return formatToolError(err), nil, nil
	}
	return formatToolResult(s, data), nil, nil
}
