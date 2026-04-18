package mcp

import "mcs-mcp/internal/stats"

// --- Enum types ---

// SimulationMode represents the simulation mode for forecasting tools.
type SimulationMode string

const (
	SimModeDuration SimulationMode = "duration"
	SimModeScope    SimulationMode = "scope"
)

// AgeType represents the type of age calculation.
type AgeType string

const (
	AgeTypeTotal AgeType = "total"
	AgeTypeWIP   AgeType = "wip"
)

// TierFilter represents a filter for workflow tiers.
type TierFilter string

const (
	TierFilterWIP        TierFilter = "WIP"
	TierFilterDemand     TierFilter = "Demand"
	TierFilterUpstream   TierFilter = "Upstream"
	TierFilterDownstream TierFilter = "Downstream"
	TierFilterFinished   TierFilter = "Finished"
	TierFilterAll        TierFilter = "All"
)

// DiagnosticGoal represents the analytical goal for the diagnostic roadmap.
type DiagnosticGoal string

const (
	GoalForecasting       DiagnosticGoal = "forecasting"
	GoalBottlenecks       DiagnosticGoal = "bottlenecks"
	GoalCapacityPlanning  DiagnosticGoal = "capacity_planning"
	GoalSystemHealth      DiagnosticGoal = "system_health"
)

// Granularity represents the time series granularity.
type Granularity string

const (
	GranularityDaily  Granularity = "daily"
	GranularityWeekly Granularity = "weekly"
)

// WorkflowTier represents a workflow tier in the semantic mapping.
type WorkflowTier string

const (
	TierDemand     WorkflowTier = WorkflowTier(stats.TierDemand)
	TierUpstream   WorkflowTier = WorkflowTier(stats.TierUpstream)
	TierDownstream WorkflowTier = WorkflowTier(stats.TierDownstream)
	TierFinished   WorkflowTier = WorkflowTier(stats.TierFinished)
)

// WorkflowRole represents a workflow role in the semantic mapping.
type WorkflowRole string

const (
	RoleActive WorkflowRole = "active"
	RoleQueue  WorkflowRole = "queue"
	RoleIgnore WorkflowRole = "ignore"
)

// WorkflowOutcome represents a workflow outcome.
type WorkflowOutcome string

const (
	OutcomeDelivered  WorkflowOutcome = "delivered"
	OutcomeAbandoned  WorkflowOutcome = "abandoned"
)

// StatusMappingEntry holds the semantic metadata for a single workflow status.
type StatusMappingEntry struct {
	Tier    WorkflowTier    `json:"tier"`
	Role    WorkflowRole    `json:"role,omitempty" jsonschema:"Omit for Finished tier."`
	Outcome WorkflowOutcome `json:"outcome,omitempty" jsonschema:"Mandatory for Finished tier statuses if resolutions are not used."`
}

// --- Input structs ---

// ImportProjectsInput holds arguments for the import_projects tool.
type ImportProjectsInput struct {
	Query string `json:"query" jsonschema:"Project name or key to search for"`
}

// ImportBoardsInput holds arguments for the import_boards tool.
type ImportBoardsInput struct {
	ProjectKey string `json:"project_key,omitempty" jsonschema:"Optional project key"`
	NameFilter string `json:"name_filter,omitempty" jsonschema:"Filter by board name"`
}

// ImportProjectContextInput holds arguments for the import_project_context tool.
type ImportProjectContextInput struct {
	ProjectKey string `json:"project_key" jsonschema:"The project key (e.g. PROJ)"`
}

// ImportBoardContextInput holds arguments for the import_board_context tool.
type ImportBoardContextInput struct {
	ProjectKey string `json:"project_key" jsonschema:"The project key (e.g. PROJ)"`
	BoardID    int    `json:"board_id" jsonschema:"The board ID"`
}

// ForecastMonteCarloInput holds arguments for the forecast_monte_carlo tool.
type ForecastMonteCarloInput struct {
	ProjectKey             string             `json:"project_key" jsonschema:"The project key"`
	BoardID                int                `json:"board_id" jsonschema:"The board ID"`
	Mode                   SimulationMode     `json:"mode" jsonschema:"duration: forecast the completion date for a known set of items (deadline question). scope: forecast how many items will be done by a given date (capacity question)."`
	IncludeExistingBacklog bool               `json:"include_existing_backlog,omitempty" jsonschema:"If true automatically counts and includes all unstarted items (Demand Tier or Backlog) from Jira. Set both include_existing_backlog and include_wip to true for real commitment forecasts — omitting either understates total scope."`
	IncludeWIP             bool               `json:"include_wip,omitempty" jsonschema:"If true also includes items already in progress (past the Commitment Point). Set both include_wip and include_existing_backlog to true for real commitment forecasts — omitting either understates total scope."`
	AdditionalItems        int                `json:"additional_items,omitempty" jsonschema:"Additional items to include beyond what Jira contains (e.g. new initiative not yet in Jira). Ignored if targets is provided."`
	TargetDays             int                `json:"target_days,omitempty" jsonschema:"Number of days to forecast into the future (required for scope mode). Calculated automatically if target_date is provided."`
	TargetDate             string             `json:"target_date,omitempty" jsonschema:"Target date (YYYY-MM-DD). If provided target_days is calculated automatically."`
	StartStatus            string             `json:"start_status,omitempty" jsonschema:"Override the Commitment Point status (default: configured commitment point)."`
	EndStatus              string             `json:"end_status,omitempty" jsonschema:"Override the Resolution Point status (default: Finished tier)."`
	IssueTypes             []string           `json:"issue_types,omitempty" jsonschema:"Filter to specific issue types (e.g. Story Bug). If omitted all mapped types are included."`
	HistoryWindowDays      int                `json:"history_window_days,omitempty" jsonschema:"Lookback window in days for the throughput sample. Default: all available history. Narrow to 30–60 days after a process change. Use recommended_window_days from analyze_residence_time when that tool returns a non-stationary signal (λ/θ > 1.1)."`
	HistoryStartDate       string             `json:"history_start_date,omitempty" jsonschema:"Explicit start date for the historical baseline (YYYY-MM-DD). Overrides history_window_days."`
	HistoryEndDate         string             `json:"history_end_date,omitempty" jsonschema:"Explicit end date for the historical baseline (YYYY-MM-DD). Default: today."`
	Targets                map[string]int     `json:"targets,omitempty" jsonschema:"Exact counts of items to simulate per type (e.g. Story:10 Bug:5). If provided additional_items is ignored."`
	MixOverrides           map[string]float64 `json:"mix_overrides,omitempty" jsonschema:"Override the historical capacity distribution per type (e.g. Bug:0.1). Values (0.0–1.0) represent target share of capacity; remaining capacity is distributed proportionally to other types."`
}

// AnalyzeCycleTimeInput holds arguments for the analyze_cycle_time tool.
type AnalyzeCycleTimeInput struct {
	ProjectKey      string   `json:"project_key" jsonschema:"The project key"`
	BoardID         int      `json:"board_id" jsonschema:"The board ID"`
	IssueTypes      []string `json:"issue_types,omitempty" jsonschema:"Optional: List of issue types to include in the calculation (e.g. Story or Bug)."`
	StartStatus     string   `json:"start_status,omitempty" jsonschema:"Optional: Explicit start status (default: Commitment Point)."`
	EndStatus       string   `json:"end_status,omitempty" jsonschema:"Optional: Explicit end status (default: Finished Tier)."`
	SLEPercentile   int      `json:"sle_percentile,omitempty" jsonschema:"Optional: percentile (50, 70, 85, 95) used as the SLE for adherence trending. Default: 85."`
	SLEDurationDays float64  `json:"sle_duration_days,omitempty" jsonschema:"Optional: fixed SLE duration in days. If supplied, adherence is trended against this constant baseline; otherwise the rolling-window percentile is used."`
}

// AnalyzeStatusPersistenceInput holds arguments for the analyze_status_persistence tool.
type AnalyzeStatusPersistenceInput struct {
	ProjectKey string `json:"project_key" jsonschema:"The project key"`
	BoardID    int    `json:"board_id" jsonschema:"The board ID"`
}

// AnalyzeWorkItemAgeInput holds arguments for the analyze_work_item_age tool.
type AnalyzeWorkItemAgeInput struct {
	ProjectKey string     `json:"project_key" jsonschema:"The project key"`
	BoardID    int        `json:"board_id" jsonschema:"The board ID"`
	AgeType    AgeType    `json:"age_type" jsonschema:"'wip': age since commitment point (standard SLE comparison — requires correct commitment point mapping). 'total': age since creation (surfaces items that entered the system long ago but have not yet committed)."`
	TierFilter TierFilter `json:"tier_filter,omitempty" jsonschema:"Filter results to a specific tier. Default 'WIP' excludes Demand and Finished (shows only in-flight items). Use 'Upstream' or 'Downstream' to focus on a specific stage. Use 'All' to include Demand and Finished items."`
}

// AnalyzeThroughputInput holds arguments for the analyze_throughput tool.
type AnalyzeThroughputInput struct {
	ProjectKey         string `json:"project_key" jsonschema:"The project key"`
	BoardID            int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int    `json:"history_window_weeks,omitempty" jsonschema:"Lookback window in weeks. Default: 26 (6 months) for stable systems. Use 8–12 after a process change or team restructuring. Use 4–6 to measure only the current state after a deliberate reset."`
	IncludeAbandoned   bool   `json:"include_abandoned,omitempty" jsonschema:"If true includes items with abandoned outcome. Default: false (delivered items only)."`
	Bucket             string `json:"bucket,omitempty" jsonschema:"Group data by 'week' (default) or 'month'. Use 'month' for low-volume teams where weekly counts are too sparse to be meaningful."`
}

// AnalyzeProcessStabilityInput holds arguments for the analyze_process_stability tool.
type AnalyzeProcessStabilityInput struct {
	ProjectKey         string `json:"project_key" jsonschema:"The project key"`
	BoardID            int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int    `json:"history_window_weeks,omitempty" jsonschema:"Lookback window in weeks. Default: 26 (6 months) for stable systems. Use 8–12 after a process change or team restructuring. Use 4–6 to measure only the current state after a deliberate reset."`
	IncludeRawSeries   bool   `json:"include_raw_series,omitempty" jsonschema:"If true includes the full Values and MovingRange arrays in the response. Default: false. Enable when you need to inspect individual data points or plot the raw series."`
}

// AnalyzeFlowDebtInput holds arguments for the analyze_flow_debt tool.
type AnalyzeFlowDebtInput struct {
	ProjectKey         string `json:"project_key" jsonschema:"The project key"`
	BoardID            int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int    `json:"history_window_weeks,omitempty" jsonschema:"Lookback window in weeks. Default: 26 (6 months) for stable systems. Use 8–12 after a process change or team restructuring. Use 4–6 to measure only the current state after a deliberate reset."`
	BucketSize         string `json:"bucket_size,omitempty" jsonschema:"Group data by 'week' (default) or 'month'. Use 'month' for low-volume teams where weekly counts are too sparse to be meaningful."`
}

// GenerateCFDDataInput holds arguments for the generate_cfd_data tool.
type GenerateCFDDataInput struct {
	ProjectKey         string      `json:"project_key" jsonschema:"The project key"`
	BoardID            int         `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int         `json:"history_window_weeks,omitempty" jsonschema:"Lookback window in weeks. Default: 26 (6 months) for stable systems. Use 8–12 after a process change or team restructuring. Use 4–6 to measure only the current state after a deliberate reset."`
	Granularity        Granularity `json:"granularity,omitempty" jsonschema:"Time series granularity. 'daily' (default) gives the full picture. 'weekly' keeps only the last data point per ISO week — use this to reduce payload size for long windows or low-volume teams."`
}

// AnalyzeWIPStabilityInput holds arguments for the analyze_wip_stability tool.
type AnalyzeWIPStabilityInput struct {
	ProjectKey         string `json:"project_key" jsonschema:"The project key"`
	BoardID            int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int    `json:"history_window_weeks,omitempty" jsonschema:"Lookback window in weeks. Default: 26 (6 months) for stable systems. Use 8–12 after a team size change or WIP limit policy change."`
}

// AnalyzeWIPAgeStabilityInput holds arguments for the analyze_wip_age_stability tool.
type AnalyzeWIPAgeStabilityInput struct {
	ProjectKey         string `json:"project_key" jsonschema:"The project key"`
	BoardID            int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int    `json:"history_window_weeks,omitempty" jsonschema:"Lookback window in weeks. Default: 26 (6 months) for stable systems. Use 8–12 after a process change to measure only the new regime."`
}

// AnalyzeProcessEvolutionInput holds arguments for the analyze_process_evolution tool.
type AnalyzeProcessEvolutionInput struct {
	ProjectKey          string `json:"project_key" jsonschema:"The project key"`
	BoardID             int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowMonths int    `json:"history_window_months,omitempty" jsonschema:"Number of months to analyze. Default: 12. Increase to 24–60 for deep audits or post-reorganization analysis. Raise INGESTION_CREATED_LOOKBACK in .env and re-hydrate if the local cache does not cover the requested range."`
}

// AnalyzeYieldInput holds arguments for the analyze_yield tool.
type AnalyzeYieldInput struct {
	ProjectKey string `json:"project_key" jsonschema:"The project key"`
	BoardID    int    `json:"board_id" jsonschema:"The board ID"`
}

// WorkflowDiscoverMappingInput holds arguments for the workflow_discover_mapping tool.
type WorkflowDiscoverMappingInput struct {
	ProjectKey   string `json:"project_key" jsonschema:"The project key"`
	BoardID      int    `json:"board_id" jsonschema:"The board ID"`
	ForceRefresh bool   `json:"force_refresh,omitempty" jsonschema:"If true bypasses the persistent cache and recalculates the mapping from historical data."`
}

// WorkflowSetMappingInput holds arguments for the workflow_set_mapping tool.
type WorkflowSetMappingInput struct {
	ProjectKey      string                        `json:"project_key" jsonschema:"The project key"`
	BoardID         int                           `json:"board_id" jsonschema:"The board ID"`
	Mapping         map[string]StatusMappingEntry  `json:"mapping" jsonschema:"A map of status names to metadata (tier role and optional outcome)."`
	Resolutions     map[string]WorkflowOutcome     `json:"resolutions,omitempty" jsonschema:"Optional: A map of Jira resolution names to outcomes (delivered or abandoned)."`
	CommitmentPoint string                         `json:"commitment_point,omitempty" jsonschema:"Optional: The Downstream status where the clock starts."`
}

// WorkflowSetOrderInput holds arguments for the workflow_set_order tool.
type WorkflowSetOrderInput struct {
	ProjectKey string   `json:"project_key" jsonschema:"The project key"`
	BoardID    int      `json:"board_id" jsonschema:"The board ID"`
	Order      []string `json:"order" jsonschema:"Ordered list of status names."`
}

// WorkflowSetEvaluationDateInput holds arguments for the workflow_set_evaluation_date tool.
type WorkflowSetEvaluationDateInput struct {
	ProjectKey string `json:"project_key" jsonschema:"The project key"`
	BoardID    int    `json:"board_id" jsonschema:"The board ID"`
	Date       string `json:"date" jsonschema:"Evaluation date (YYYY-MM-DD)"`
}

// AnalyzeItemJourneyInput holds arguments for the analyze_item_journey tool.
type AnalyzeItemJourneyInput struct {
	ProjectKey string `json:"project_key" jsonschema:"The project key"`
	BoardID    int    `json:"board_id" jsonschema:"The board ID"`
	IssueKey   string `json:"issue_key" jsonschema:"The Jira issue key (e.g. PROJ-123)"`
}

// GuideDiagnosticRoadmapInput holds arguments for the guide_diagnostic_roadmap tool.
type GuideDiagnosticRoadmapInput struct {
	Goal DiagnosticGoal `json:"goal" jsonschema:"The analytical goal to get a roadmap for."`
}

// ForecastBacktestInput holds arguments for the forecast_backtest tool.
type ForecastBacktestInput struct {
	ProjectKey        string         `json:"project_key" jsonschema:"The project key"`
	BoardID           int            `json:"board_id" jsonschema:"The board ID"`
	SimulationMode    SimulationMode `json:"simulation_mode" jsonschema:"Simulation mode: duration or scope."`
	ItemsToForecast   int            `json:"items_to_forecast,omitempty" jsonschema:"Number of items to forecast (duration mode). Default: 5"`
	ForecastHorizon   int            `json:"forecast_horizon_days,omitempty" jsonschema:"Number of days to forecast (scope mode). Default: 14"`
	IssueTypes        []string       `json:"issue_types,omitempty" jsonschema:"Optional: List of issue types to include in the validation."`
	HistoryWindowDays int            `json:"history_window_days,omitempty" jsonschema:"Optional: How far back the validation iterates, controlling the number of checkpoints (not the per-checkpoint sampling window). Default: 175 days (25 weekly checkpoints). Each checkpoint always samples from a fixed 90-day window ending at that checkpoint."`
	HistoryStartDate  string         `json:"history_start_date,omitempty" jsonschema:"Optional: Explicit start date for the validation range (YYYY-MM-DD). Overrides history_window_days."`
	HistoryEndDate    string         `json:"history_end_date,omitempty" jsonschema:"Optional: Explicit end date for the validation range (YYYY-MM-DD). Defaults to today."`
}

// AnalyzeResidenceTimeInput holds arguments for the analyze_residence_time tool.
type AnalyzeResidenceTimeInput struct {
	ProjectKey         string      `json:"project_key" jsonschema:"The project key"`
	BoardID            int         `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int         `json:"history_window_weeks,omitempty" jsonschema:"Lookback window in weeks. Default: 52 (longer than other tools — Sample Path analysis needs sufficient path length). Reduce to 26 if only the recent regime is relevant or after a major process reset."`
	IssueTypes         []string    `json:"issue_types,omitempty" jsonschema:"Filter to specific issue types (e.g. Story Bug). If omitted all mapped types are included."`
	Granularity        Granularity `json:"granularity,omitempty" jsonschema:"Time series granularity. 'daily' (default) for full resolution. 'weekly' to reduce payload size for long windows."`
}

// ImportHistoryUpdateInput holds arguments for the import_history_update tool.
type ImportHistoryUpdateInput struct {
	ProjectKey string `json:"project_key" jsonschema:"The project key"`
	BoardID    int    `json:"board_id" jsonschema:"The board ID"`
}

// OpenInBrowserInput holds arguments for the open_in_browser tool.
type OpenInBrowserInput struct {
	URL string `json:"url" jsonschema:"The chart render URL to open (must be a localhost render-charts URL)"`
}

