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
	Mode                   SimulationMode     `json:"mode" jsonschema:"Simulation mode: duration (forecast completion date for a number of work items) or scope (forecast delivery volume)."`
	IncludeExistingBacklog bool               `json:"include_existing_backlog,omitempty" jsonschema:"If true automatically counts and includes all unstarted items (Demand Tier or Backlog) from Jira for this board/filter."`
	IncludeWIP             bool               `json:"include_wip,omitempty" jsonschema:"If true ALSO includes items already in progress (passed the Commitment Point or started)."`
	AdditionalItems        int                `json:"additional_items,omitempty" jsonschema:"Additional items to include (e.g. new initiative/scope not yet in Jira)."`
	TargetDays             int                `json:"target_days,omitempty" jsonschema:"Number of days (required for scope mode)."`
	TargetDate             string             `json:"target_date,omitempty" jsonschema:"Optional: Target date (YYYY-MM-DD). If provided target_days is calculated automatically."`
	StartStatus            string             `json:"start_status,omitempty" jsonschema:"Optional: Start status (Commitment Point)."`
	EndStatus              string             `json:"end_status,omitempty" jsonschema:"Optional: End status (Resolution Point)."`
	IssueTypes             []string           `json:"issue_types,omitempty" jsonschema:"Optional: List of issue types to include (e.g. Story)."`
	HistoryWindowDays      int                `json:"history_window_days,omitempty" jsonschema:"Optional: Lookback window in days for historical baseline."`
	HistoryStartDate       string             `json:"history_start_date,omitempty" jsonschema:"Optional: Explicit start date for the historical baseline (YYYY-MM-DD)."`
	HistoryEndDate         string             `json:"history_end_date,omitempty" jsonschema:"Optional: Explicit end date for the historical baseline (YYYY-MM-DD). Default: Today."`
	Targets                map[string]int     `json:"targets,omitempty" jsonschema:"Optional: Exact counts of items to simulate (e.g. Story 10 Bug 5). If provided additional_items is ignored and the simulation targets these specific counts."`
	MixOverrides           map[string]float64 `json:"mix_overrides,omitempty" jsonschema:"Optional: Shifting the historical capacity distribution (e.g. Bug 0.1). The float values (0.0-1.0) represent the target share of capacity. Remaining capacity is distributed proportionally to other types."`
}

// AnalyzeCycleTimeInput holds arguments for the analyze_cycle_time tool.
type AnalyzeCycleTimeInput struct {
	ProjectKey          string   `json:"project_key" jsonschema:"The project key"`
	BoardID             int      `json:"board_id" jsonschema:"The board ID"`
	IssueTypes  []string `json:"issue_types,omitempty" jsonschema:"Optional: List of issue types to include in the calculation (e.g. Story or Bug)."`
	StartStatus string   `json:"start_status,omitempty" jsonschema:"Optional: Explicit start status (default: Commitment Point)."`
	EndStatus           string   `json:"end_status,omitempty" jsonschema:"Optional: Explicit end status (default: Finished Tier)."`
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
	AgeType    AgeType    `json:"age_type" jsonschema:"Type of age to calculate: total (since creation) or wip (since commitment)."`
	TierFilter TierFilter `json:"tier_filter,omitempty" jsonschema:"Optional: Filter results to specific tiers. WIP (Work-in-process default) excludes Demand and Finished."`
}

// AnalyzeThroughputInput holds arguments for the analyze_throughput tool.
type AnalyzeThroughputInput struct {
	ProjectKey         string `json:"project_key" jsonschema:"The project key"`
	BoardID            int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int    `json:"history_window_weeks,omitempty" jsonschema:"Number of weeks to analyze (default: 26)"`
	IncludeAbandoned   bool   `json:"include_abandoned,omitempty" jsonschema:"If true includes items with abandoned outcome (default: false)."`
	Bucket             string `json:"bucket,omitempty" jsonschema:"Group data by week or month (default: week)."`
}

// AnalyzeProcessStabilityInput holds arguments for the analyze_process_stability tool.
type AnalyzeProcessStabilityInput struct {
	ProjectKey         string `json:"project_key" jsonschema:"The project key"`
	BoardID            int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int    `json:"history_window_weeks,omitempty" jsonschema:"Number of weeks to analyze (default: 26)"`
	IncludeRawSeries   bool   `json:"include_raw_series,omitempty" jsonschema:"If true includes the full Values and MovingRange arrays in the response (default: false)."`
}

// AnalyzeFlowDebtInput holds arguments for the analyze_flow_debt tool.
type AnalyzeFlowDebtInput struct {
	ProjectKey         string `json:"project_key" jsonschema:"The project key"`
	BoardID            int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int    `json:"history_window_weeks,omitempty" jsonschema:"Number of weeks to analyze (default: 26)"`
	BucketSize         string `json:"bucket_size,omitempty" jsonschema:"Group data by week or month (default: week)"`
}

// GenerateCFDDataInput holds arguments for the generate_cfd_data tool.
type GenerateCFDDataInput struct {
	ProjectKey         string      `json:"project_key" jsonschema:"The project key"`
	BoardID            int         `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int         `json:"history_window_weeks,omitempty" jsonschema:"Number of weeks to analyze (default: 26)"`
	Granularity        Granularity `json:"granularity,omitempty" jsonschema:"Time series granularity: daily (default) or weekly. Weekly keeps only the last data point per ISO week reducing payload size."`
}

// AnalyzeWIPStabilityInput holds arguments for the analyze_wip_stability tool.
type AnalyzeWIPStabilityInput struct {
	ProjectKey         string `json:"project_key" jsonschema:"The project key"`
	BoardID            int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int    `json:"history_window_weeks,omitempty" jsonschema:"Number of weeks to analyze (default: 26)"`
}

// AnalyzeWIPAgeStabilityInput holds arguments for the analyze_wip_age_stability tool.
type AnalyzeWIPAgeStabilityInput struct {
	ProjectKey         string `json:"project_key" jsonschema:"The project key"`
	BoardID            int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowWeeks int    `json:"history_window_weeks,omitempty" jsonschema:"Number of weeks to analyze (default: 26)"`
}

// AnalyzeProcessEvolutionInput holds arguments for the analyze_process_evolution tool.
type AnalyzeProcessEvolutionInput struct {
	ProjectKey          string `json:"project_key" jsonschema:"The project key"`
	BoardID             int    `json:"board_id" jsonschema:"The board ID"`
	HistoryWindowMonths int    `json:"history_window_months,omitempty" jsonschema:"Number of months to analyze (default: 12 supports up to 60 for deep history)"`
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
	HistoryWindowWeeks int         `json:"history_window_weeks,omitempty" jsonschema:"Number of weeks to analyze (default: 52)"`
	IssueTypes         []string    `json:"issue_types,omitempty" jsonschema:"Optional: List of issue types to include (e.g. Story Bug)."`
	Granularity        Granularity `json:"granularity,omitempty" jsonschema:"Time series granularity: daily (default) or weekly."`
}

// ImportHistoryExpandInput holds arguments for the import_history_expand tool.
type ImportHistoryExpandInput struct {
	ProjectKey string `json:"project_key" jsonschema:"The project key"`
	BoardID    int    `json:"board_id" jsonschema:"The board ID"`
	Chunks     int    `json:"chunks,omitempty" jsonschema:"Optional: Number of additional batches (300 items each) to fetch. Default: 3"`
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

