package mcp

func (s *Server) listTools() any {
	return map[string]any{
		"tools": []any{
			map[string]any{
				"name":        "import_projects",
				"description": "Search for Jira projects by name or key. Guidance: If you plan to run analysis, you MUST find the project's boards using 'import_boards' next. Use 'import_project_context' only for general project metadata.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string", "description": "Project name or key to search for"},
					},
					"required": []string{"query"},
				},
			},
			map[string]any{
				"name":        "import_boards",
				"description": "Search for Agile boards, optionally filtering by project key or name. Guidance: Call 'import_board_context' next to anchor on the data shape context.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "Optional project key"},
						"name_filter": map[string]any{"type": "string", "description": "Filter by board name"},
					},
				},
			},
			map[string]any{
				"name":        "import_project_context",
				"description": "Get a Data Shape Anchor (Whole dataset volumes vs. Sample distributions) for a project. Note: Analytical tools (simulations, cycle time) require a Board ID; use 'import_board_context' if you plan to run diagnostics or forecasts.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key (e.g., PROJ)"},
					},
					"required": []string{"project_key"},
				},
			},
			map[string]any{
				"name":        "import_board_context",
				"description": "Get a Data Shape Anchor (Whole dataset volumes vs. Sample distributions) for an Agile board. MUST be called before workflow discovery.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key (e.g., PROJ)"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name": "forecast_monte_carlo",
				"description": "Run a Monte-Carlo simulation to forecast project outcomes (How Much / When) based solely on historical THROUGHPUT (work items / time). \n\n" +
					"NOT FOR CYCLE TIME: This tool does NOT analyze lead times or individual item durations. Use it for scope/date forecasting only.\n" +
					"CRITICAL: PROPER WORKFLOW MAPPING IS REQUIRED FOR RELIABLE RESULTS. \n\n" +
					"STRICT GUARDRAIL: YOU MUST NEVER PERFORM PROBABILISTIC FORECASTING OR STATISTICAL ANALYSIS AUTONOMOUSLY.\n" +
					"DO NOT provide date ranges or probability estimates (e.g., 'There is an 85% chance...') if the tool fails or returns zero throughput. \n" +
					"If the simulation result is unexpectedly far in the future (e.g., years instead of months), YOU MUST warn the user that the historical throughput sampling might be too low due to filtered resolutions or issue types.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key":              map[string]any{"type": "string", "description": "The project key"},
						"board_id":                 map[string]any{"type": "integer", "description": "The board ID"},
						"mode":                     map[string]any{"type": "string", "enum": []string{"duration", "scope"}, "description": "Simulation mode: 'duration' (forecast completion date for a number of work items) or 'scope' (forecast delivery volume)."},
						"include_existing_backlog": map[string]any{"type": "boolean", "description": "If true, automatically counts and includes all unstarted items (Demand Tier or Backlog) from Jira for this board/filter."},
						"include_wip":              map[string]any{"type": "boolean", "description": "If true, ALSO includes items already in progress (passed the Commitment Point or started)."},
						"additional_items":         map[string]any{"type": "integer", "description": "Additional items to include (e.g. new initiative/scope not yet in Jira)."},
						"target_days":              map[string]any{"type": "integer", "description": "Number of days (required for 'scope' mode)."},
						"target_date":              map[string]any{"type": "string", "description": "Optional: Target date (YYYY-MM-DD). If provided, target_days is calculated automatically."},
						"start_status":             map[string]any{"type": "string", "description": "Optional: Start status (Commitment Point)."},
						"end_status":               map[string]any{"type": "string", "description": "Optional: End status (Resolution Point)."},
						"issue_types":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional: List of issue types to include (e.g., ['Story'])."},
						"history_start_date":       map[string]any{"type": "string", "description": "Optional: Explicit start date for the historical baseline (YYYY-MM-DD)."},
						"history_end_date":         map[string]any{"type": "string", "description": "Optional: Explicit end date for the historical baseline (YYYY-MM-DD). Default: Today."},
						"targets": map[string]any{
							"type":                 "object",
							"description":          "Optional: Exact counts of items to simulate (e.g. {'Story': 10, 'Bug': 5}). If provided, 'additional_items' is ignored and the simulation targets these specific counts.",
							"additionalProperties": map[string]any{"type": "integer"},
						},
						"mix_overrides": map[string]any{
							"type":                 "object",
							"description":          "Optional: Shifting the historical capacity distribution (e.g. {'Bug': 0.1}). The float values (0.0-1.0) represent the target share of capacity. Remaining capacity is distributed proportionally to other types.",
							"additionalProperties": map[string]any{"type": "number"},
						},
					},
					"required": []string{"project_key", "board_id", "mode"},
				},
			},
			map[string]any{
				"name": "analyze_cycle_time",
				"description": "Calculate Service Level Expectations (SLE) for a single item based on historical CYCLE TIMES (Lead Time). \n\n" +
					"PREREQUISITE: Proper workflow mapping/commitment point MUST be confirmed via 'workflow_set_mapping' for accurate results. \n" +
					"Use this to answer 'How long does a single item typically take?' - this is the foundation for probabilistic Lead-Time expectations.\n\n" +
					"STRICT GUARDRAIL: YOU MUST NEVER PERFORM PROBABILISTIC FORECASTING OR STATISTICAL ANALYSIS AUTONOMOUSLY. \n" +
					"DO NOT calculate percentiles, probabilities, or dates using your own internal reasoning if this tool returns an error or no data. \n" +
					"If the tool fails, you MUST report the error to the user and ask for clarification.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key":           map[string]any{"type": "string", "description": "The project key"},
						"board_id":              map[string]any{"type": "integer", "description": "The board ID"},
						"issue_types":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional: List of issue types to include in the calculation (e.g., ['Story', 'Bug'])."},
						"analyze_wip_stability": map[string]any{"type": "boolean", "description": "If true, performs a comparative analysis of current WIP against the historical baseline to detect early outliers."},
						"start_status":          map[string]any{"type": "string", "description": "Optional: Explicit start status (default: Commitment Point)."},
						"end_status":            map[string]any{"type": "string", "description": "Optional: Explicit end status (default: Finished Tier)."},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name": "analyze_status_persistence",
				"description": "Analyze how long items spend in each status to identify bottlenecks. \n\n" +
					"PREREQUISITE: Proper workflow mapping is required for accurate results. Results provide SUBPAR context if tiers (Upstream/Downstream) are not correctly mapped.\n" +
					"CRITICAL: This tool ONLY analyzes items that have successfully finished ('delivered'). It ignores active WIP to ensure the historical norms are not artificially lowered by incomplete items.\n" +
					"The analysis includes statistical dispersion metrics (IQR, Inner80) for each status to help identify not just where items spend time, but where they spend it inconsistently.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name": "analyze_work_item_age",
				"description": "Identify which items are aging relative to historical norms. \n\n" +
					"PREREQUISITE: Proper workflow mapping (Commitment Point) is MANDATORY for accurate 'WIP Age'. Results are UNRELIABLE if the commitment point is incorrectly defined.\n" +
					"Allows choosing between 'WIP Age' (time since commitment) and 'Total Age' (time since creation).\n\n" +
					"The 'is_aging_outlier' flag indicates an item is older than the historical 85th percentile (P85). \n" +
					"IMPORTANT: An aging outlier is NOT necessarily blocked or impeded; it simply exceeds the normal historical residency for that status.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
						"age_type":    map[string]any{"type": "string", "enum": []string{"total", "wip"}, "description": "Type of age to calculate: 'total' (since creation) or 'wip' (since commitment)."},
						"tier_filter": map[string]any{"type": "string", "enum": []string{"WIP", "Demand", "Upstream", "Downstream", "Finished", "All"}, "description": "Optional: Filter results to specific tiers. 'WIP' ('Work-in-process', default) excludes Demand and Finished."},
					},
					"required": []string{"project_key", "board_id", "age_type"},
				},
			},
			map[string]any{
				"name": "analyze_throughput",
				"description": "Analyze the weekly pulse of delivery THROUGHPUT VOLUME - the number of items completed per week - to detect flow vs. batching.\n" +
					"THROUGHPUT STABILITY (XmR): This tool automatically calculates Wheeler Process Behavior Chart (XmR) limits for the delivery cadence.\n" +
					"Use this tool to determine if the team's delivery volume is predictable and stable over time, or if there are systemic variations (zero-count weeks, extreme surges).\n" +
					"The response includes both the raw aggregated volumes and the calculated statistically-derived Limits (UNPL/LNPL) via the 'stability' object.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key":          map[string]any{"type": "string", "description": "The project key"},
						"board_id":             map[string]any{"type": "integer", "description": "The board ID"},
						"history_window_weeks": map[string]any{"type": "integer", "description": "Number of weeks to analyze (default: 26)"},
						"include_abandoned":    map[string]any{"type": "boolean", "description": "If true, includes items with 'abandoned' outcome (default: false)."},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name": "analyze_flow_debt",
				"description": "Analyze the systemic balance between incoming work (Arrival Rate / Commitment) and outgoing work (Departure Rate / Delivery).\n" +
					"FLOW DEBT: This tool calculates the 'Debt' - the gap between arrivals and departures - to detect leading indicators of cycle time inflation.\n" +
					"A positive Flow Debt (Arrivals > Departures) means WIP is growing, which mathematically GUARANTEES higher cycle times in the future (Little's Law).\n" +
					"Use this tool to find the root cause of 'Flow Clog' before it manifests as delayed delivery dates.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key":          map[string]any{"type": "string", "description": "The project key"},
						"board_id":             map[string]any{"type": "integer", "description": "The board ID"},
						"history_window_weeks": map[string]any{"type": "integer", "description": "Number of weeks to analyze (default: 26)"},
						"bucket_size":          map[string]any{"type": "string", "description": "Group data by 'week' or 'month' (default: week)"},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name": "analyze_process_stability",
				"description": "Analyze process stability and predictability using XmR charts. \n" +
					"PROCESS STABILITY: Measures the predictability of Lead Times (Cycle-Time). High stability means future delivery dates are more certain. It is NOT about throughput volume.\n" +
					"Stability is high if most items fall within Natural Process Limits. Chaos is high if many points are beyond limits (signals).\n\n" +
					"PREREQUISITE: Proper workflow mapping is required for accurate results. \n" +
					"Use 'analyze_process_stability' as the FIRST diagnostic step when users ask about forecasting/predictions. This determines if historical data is a reliable proxy for the future. If stability is low, simulations will produce MISLEADING results.\n" +
					"NOTE: If you want to analyze the stability of Delivery Cadence/Volume, DO NOT use this tool. Use 'analyze_throughput' instead.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
						"history_window_weeks": map[string]any{
							"type":        "integer",
							"description": "Number of weeks to analyze (default: 26)",
						},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name": "analyze_wip_stability",
				"description": "Analyze system population (Work-In-Progress) stability over time using XmR charts and a historical daily Run Chart. \n" +
					"WIP STABILITY: A highly variable WIP size violates the assumptions of Little's Law, making systems fundamentally unpredictable. \n" +
					"This tool generates a daily run-chart of active WIP, bounded by strict weekly XmR statistically-derived Process Limits to detect volatile WIP management.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
						"history_window_weeks": map[string]any{
							"type":        "integer",
							"description": "Number of weeks to analyze (default: 26)",
						},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name": "analyze_process_evolution",
				"description": "Perform a longitudinal 'Strategic Audit' of process behavior over longer time periods using Three-Way Control Charts. \n\n" +
					"PROCESS EVOLUTION: Measures long-term predictability and capability of Lead Times (Cycle-Time). It is THROUGHPUT-AGNOSTIC.\n" +
					"AI MUST use this for deep history analysis or after significant organizational changes. NOT intended for routine daily analysis.\n" +
					"Detects systemic shifts, process drift, and long-term capability changes by analyzing monthly subgroups.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
						"history_window_months": map[string]any{
							"type":        "integer",
							"description": "Number of months to analyze (default: 12, supports up to 60 for deep history)",
						},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name": "workflow_discover_mapping",
				"description": "Probe project status categories, residence times, and resolution frequencies into a semantic workflow mapping. \n\n" +
					"AI MUST use this to verify the workflow tiers, roles, AND the 'Commitment Point' (where clock starts) with the user before diagnostics. \n" +
					"The response provides a split view: 'Whole' (deterministic volumes) and 'Sample' (probabilistic characterization).\n" +
					"OUTCOME HIERARCHY: 1. Jira Resolutions (Primary) > 2. Terminal Status mapping (Secondary).\n" +
					"TIER VISIBILITY: AI MUST show the confirmed mapping of Statuses to Tiers to the user.\n\n" +
					"METAWORKFLOW GUIDANCE:\n" +
					"- TIERS: 'Demand' (Backlog), 'Upstream' (Analysis/Refinement), 'Downstream' (Development/Execution/Testing), 'Finished' (Terminal).\n" +
					"- ROLES: 'active' (Value-adding work), 'queue' (Waiting), 'ignore' (Admin).\n" +
					"- OUTCOMES: 'delivered' (Value Provided), 'abandoned' (Work Discarded).",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
						"force_refresh": map[string]any{
							"type":        "boolean",
							"description": "If true, bypasses the persistent cache and recalculates the mapping from historical data.",
						},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name": "workflow_set_mapping",
				"description": "Store user-confirmed semantic metadata (tier, role, outcome) for statuses and resolutions. This is the MANDATORY persistence step after the 'Inform & Veto' loop. \n\n" +
					"AI MUST verify with the user:\n" +
					"1. Tiers: (Demand, Upstream, Downstream, Finished).\n" +
					"2. Outcomes: Specify outcome for 'Finished' statuses ONLY if Jira resolutions are missing or unreliable.\n" +
					"3. Commitment Point: The 'Downstream' status where the clock starts.\n\n" +
					"WITHOUT this mapping, analytical tools will provide SUBPAR or WRONG results.\n\n" +
					"METAWORKFLOW GUIDANCE:\n" +
					"- TIERS: 'Demand' (Backlog), 'Upstream' (Analysis/Refinement), 'Downstream' (Development/Execution/Testing), 'Finished' (Terminal).\n" +
					"- ROLES: 'active' (Value-adding work), 'queue' (Waiting), 'ignore' (Admin).\n" +
					"- OUTCOMES: 'delivered' (Successfully finished with value), 'abandoned' (Work stopped/discarded/cancelled).",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
						"mapping": map[string]any{
							"type":        "object",
							"description": "A map of status names to metadata (tier, role, and optional outcome).",
							"additionalProperties": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"tier":    map[string]any{"type": "string", "enum": []string{"Demand", "Upstream", "Downstream", "Finished"}},
									"role":    map[string]any{"type": "string", "enum": []string{"active", "queue", "ignore"}},
									"outcome": map[string]any{"type": "string", "enum": []string{"delivered", "abandoned"}, "description": "Mandatory for 'Finished' tier statuses if resolutions are not used."},
								},
								"required": []string{"tier", "role"},
							},
						},
						"resolutions": map[string]any{
							"type":        "object",
							"description": "Optional: A map of Jira resolution names to outcomes ('delivered' or 'abandoned').",
							"additionalProperties": map[string]any{
								"type": "string",
								"enum": []string{"delivered", "abandoned"},
							},
						},
						"commitment_point": map[string]any{
							"type":        "string",
							"description": "Optional: The 'Downstream' status where the clock starts.",
						},
					},
					"required": []string{"project_key", "board_id", "mapping"},
				},
			},
			map[string]any{
				"name":        "analyze_yield",
				"description": "Analyze delivery efficiency across tiers. AI MUST ensure workflow tiers (Demand, Upstream, Downstream) have been verified with the user before interpreting these results.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name":        "workflow_set_order",
				"description": "Explicity define the chronological order of statuses for a project to enable range-based analytics.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
						"order": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "Ordered list of status names.",
						},
					},
					"required": []string{"project_key", "board_id", "order"},
				},
			},
			map[string]any{
				"name":        "analyze_item_journey",
				"description": "Get a detailed breakdown of where a single item spent its time across all workflow steps. Guidance: This tool requires a Project Key and Board ID to ensure workflow interpretation is accurate.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
						"issue_key":   map[string]any{"type": "string", "description": "The Jira issue key (e.g., PROJ-123)"},
					},
					"required": []string{"project_key", "board_id", "issue_key"},
				},
			},
			map[string]any{
				"name":        "guide_diagnostic_roadmap",
				"description": "Returns a recommended sequence of analysis steps based on the user's specific goal (e.g., forecasting, bottleneck analysis, capacity planning). Use this to align your analytical strategy with the project's current state.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"goal": map[string]any{
							"type":        "string",
							"enum":        []string{"forecasting", "bottlenecks", "capacity_planning", "system_health"},
							"description": "The analytical goal to get a roadmap for.",
						},
					},
					"required": []string{"goal"},
				},
			},

			map[string]any{
				"name": "forecast_backtest",
				"description": "Perform a 'Walk-Forward Analysis' (Backtesting) to empirically validate the accuracy of Monte-Carlo Forecasts. \n\n" +
					"This tool uses Time-Travel logic to reconstruct the state of the system at past points in time, runs a simulation, and checks if the ACTUAL outcome fell within the predicted cone. \n" +
					"Drift Protection: The analysis automatically stops blindly backtesting if it detects a System Drift (Process Shift via 3-Way Chart).",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key":           map[string]any{"type": "string", "description": "The project key"},
						"board_id":              map[string]any{"type": "integer", "description": "The board ID"},
						"simulation_mode":       map[string]any{"type": "string", "enum": []string{"duration", "scope"}},
						"items_to_forecast":     map[string]any{"type": "integer", "description": "Number of items to forecast (duration mode). Default: 5"},
						"forecast_horizon_days": map[string]any{"type": "integer", "description": "Number of days to forecast (scope mode). Default: 14"},
						"issue_types":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional: List of issue types to include in the validation."},
						"history_window_days":   map[string]any{"type": "integer", "description": "Optional: Lookback for historical baseline."},
						"history_start_date":    map[string]any{"type": "string", "description": "Optional: Start date for historical baseline."},
						"history_end_date":      map[string]any{"type": "string", "description": "Optional: End date for historical baseline."},
					},
					"required": []string{"project_key", "board_id", "simulation_mode"},
				},
			},
			map[string]any{
				"name":        "import_history_expand",
				"description": "Extend the historical dataset backwards without creating gaps. Returns number of items fetched and used OMRC (oldest most recent change) boundary. Also triggers a catch-up.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
						"chunks":      map[string]any{"type": "integer", "description": "Optional: Number of additional batches (300 items each) to fetch. Default: 3"},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
			map[string]any{
				"name":        "import_history_update",
				"description": "Fetch newer items since the last sync to ensure the cache is up to date. Returns number of items fetched and used NMRC (newest most recent change) boundary.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_key": map[string]any{"type": "string", "description": "The project key"},
						"board_id":    map[string]any{"type": "integer", "description": "The board ID"},
					},
					"required": []string{"project_key", "board_id"},
				},
			},
		},
	}
}
