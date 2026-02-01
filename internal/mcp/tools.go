package mcp

func (s *Server) listTools() interface{} {
	return map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{
				"name":        "find_jira_projects",
				"description": "Search for Jira projects by name or key. Uses server-side fuzzy matching and returns up to 30 results.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{"type": "string", "description": "Project name or key to search for"},
					},
					"required": []string{"query"},
				},
			},
			map[string]interface{}{
				"name":        "find_jira_boards",
				"description": "Search for Agile boards, optionally filtering by project key or name. Returns up to 30 results.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"project_key": map[string]interface{}{"type": "string", "description": "Optional project key"},
						"name_filter": map[string]interface{}{"type": "string", "description": "Filter by board name"},
					},
				},
			},
			map[string]interface{}{
				"name":        "get_project_details",
				"description": "Get detailed metadata for a single project by its key.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"project_key": map[string]interface{}{"type": "string", "description": "The project key (e.g., PROJ)"},
					},
					"required": []string{"project_key"},
				},
			},
			map[string]interface{}{
				"name":        "get_board_details",
				"description": "Get metadata for a single Agile board by its ID.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"board_id": map[string]interface{}{"type": "integer", "description": "The board ID"},
					},
					"required": []string{"board_id"},
				},
			},
			map[string]interface{}{
				"name":        "get_data_metadata",
				"description": "Performs a diagnostic probe on a data source (board/filter) to assess volume, health, and distribution. Returns a summary of a 50-item sample (SampleResolvedRatio, inventory counts). This is a tool for data inventory, NOT for team performance metrics.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":   map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type": map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
					},
					"required": []string{"source_id", "source_type"},
				},
			},
			map[string]interface{}{
				"name": "run_simulation",
				"description": "Run a Monte-Carlo simulation to forecast project outcomes based on historical throughput (work items / time). \n\n" +
					"CRITICAL: PROPER WORKFLOW MAPPING IS REQUIRED FOR RELIABLE RESULTS. \n" +
					"AI MUST NOT run simulations autonomously without first clarifying the user's analytical goal (specific scope, date, or scenario) and verifying process stability.\n\n" +
					"DEPENDENCIES (for result quality):\n" +
					"1. Mapping: Ensure 'set_workflow_mapping' has been called and confirmed by the user.\n" +
					"2. Stability: Run 'get_process_stability' first. If the process is unstable (Special Cause variation), simulation results are UNRELIABLE and potentially MISLEADING.\n" +
					"3. Baseline: Run 'get_cycle_time_assessment' to understand historical SLEs.\n\n" +
					"Use 'duration' mode to answer 'When will this be done?'. Use 'scope' mode to answer 'How much can we do?'.\n\n" +
					"The output includes advanced volatility metrics for AI interpretation:\n" +
					"- FatTailRatio (P98/P50): If >= 5.6, the process is Unstable/Out-of-Control (outliers dominate).\n" +
					"- TailToMedianRatio (P85/P50): If > 3.0, the process is Highly Volatile (heavy-tailed risk).\n" +
					"- IQR (P75-P25): Measures the spread of the middle 50% of results.\n" +
					"- Inner80 (P90-P10): Measures the spread of the middle 80% of results.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":                map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type":              map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
						"mode":                     map[string]interface{}{"type": "string", "enum": []string{"duration", "scope"}, "description": "Simulation mode: 'duration' (forecast completion date for a number of work items) or 'scope' (forecast delivery volume)."},
						"include_existing_backlog": map[string]interface{}{"type": "boolean", "description": "If true, automatically counts and includes all unstarted items (Demand Tier or Backlog) from Jira for this board/filter."},
						"include_wip":              map[string]interface{}{"type": "boolean", "description": "If true, ALSO includes items already in progress (passed the Commitment Point or started)."},
						"additional_items":         map[string]interface{}{"type": "integer", "description": "Additional items to include (e.g. new initiative/scope not yet in Jira)."},
						"target_days":              map[string]interface{}{"type": "integer", "description": "Number of days (required for 'scope' mode)."},
						"target_date":              map[string]interface{}{"type": "string", "description": "Optional: Target date (YYYY-MM-DD). If provided, target_days is calculated automatically."},
						"start_status":             map[string]interface{}{"type": "string", "description": "Optional: Start status (Commitment Point)."},
						"end_status":               map[string]interface{}{"type": "string", "description": "Optional: End status (Resolution Point)."},
						"issue_types":              map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional: List of issue types to include (e.g., ['Story'])."},
						"resolutions":              map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional: Resolutions to count as 'Done'."},
					},
					"required": []string{"source_id", "source_type", "mode"},
				},
			},
			map[string]interface{}{
				"name": "get_cycle_time_assessment",
				"description": "Calculate Service Level Expectations (SLE) for a single item based on historical cycle times. \n\n" +
					"PREREQUISITE: Proper workflow mapping/commitment point MUST be confirmed via 'set_workflow_mapping' for accurate results. \n" +
					"Use this to answer 'How long does an item (of type X) typically take?' - this is the foundation for all forecasting.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":             map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type":           map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
						"issue_types":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional: List of issue types to include in the calculation (e.g., ['Story', 'Bug'])."},
						"analyze_wip_stability": map[string]interface{}{"type": "boolean", "description": "If true, performs a comparative analysis of current WIP against the historical baseline to detect early outliers."},
						"start_status":          map[string]interface{}{"type": "string", "description": "Optional: Explicit start status (default: Commitment Point)."},
						"end_status":            map[string]interface{}{"type": "string", "description": "Optional: Explicit end status (default: Finished Tier)."},
						"resolutions":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional: Resolutions to count as 'Done' for the baseline (e.g., ['Fixed'])."},
					},
					"required": []string{"source_id", "source_type"},
				},
			},
			map[string]interface{}{
				"name": "get_status_persistence",
				"description": "Analyze how long items spend in each status to identify bottlenecks. \n\n" +
					"PREREQUISITE: Proper workflow mapping is required for accurate results. Results provide SUBPAR context if tiers (Upstream/Downstream) are not correctly mapped.\n" +
					"The analysis includes statistical dispersion metrics (IQR, Inner80) for each status to help identify not just where items spend time, but where they spend it inconsistently.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":   map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type": map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
					},
					"required": []string{"source_id", "source_type"},
				},
			},
			map[string]interface{}{
				"name": "get_aging_analysis",
				"description": "Identify which items are aging relative to historical norms. \n\n" +
					"PREREQUISITE: Proper workflow mapping (Commitment Point) is MANDATORY for accurate 'WIP Age'. Results are UNRELIABLE if the commitment point is incorrectly defined.\n" +
					"Allows choosing between 'WIP Age' (time since commitment) and 'Total Age' (time since creation).\n\n" +
					"This tool uses the Tail-to-Median and Fat-Tail ratios to determine if the overall system is stable or if individual items are being 'neglected' in the tail.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":   map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type": map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
						"aging_type":  map[string]interface{}{"type": "string", "enum": []string{"total", "wip"}, "description": "Type of age to calculate: 'total' (since creation) or 'wip' (since commitment)."},
						"tier_filter": map[string]interface{}{"type": "string", "enum": []string{"WIP", "Demand", "Upstream", "Downstream", "Finished", "All"}, "description": "Optional: Filter results to specific tiers. 'WIP' ('Work-in-process', default) excludes Demand and Finished."},
					},
					"required": []string{"source_id", "source_type", "aging_type"},
				},
			},
			map[string]interface{}{
				"name":        "get_delivery_cadence",
				"description": "Visualize the weekly pulse of delivery - known as throughput - to detect flow vs. batching.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":    map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type":  map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
						"window_weeks": map[string]interface{}{"type": "integer", "description": "Number of weeks to analyze (default: 26)"},
					},
					"required": []string{"source_id", "source_type"},
				},
			},
			map[string]interface{}{
				"name": "get_process_stability",
				"description": "Analyzes process predictability using XmR behavior charts to detect 'Special Cause' variation. \n\n" +
					"PREREQUISITE: Proper workflow mapping is required for accurate results. \n" +
					"Use 'get_process_stability' as the FIRST diagnostic step when users ask about forecasting/predictions. This determines if current forecasts based on historical data are even reliable. If stability is low, the process is 'Out of Control' and simulations will produce MISLEADING results.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":   map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type": map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
						"window_weeks": map[string]interface{}{
							"type":        "integer",
							"description": "Number of weeks to analyze (default: 26)",
						},
					},
					"required": []string{"source_id", "source_type"},
				},
			},
			map[string]interface{}{
				"name": "get_process_evolution",
				"description": "Perform a longitudinal 'Strategic Audit' of process behavior over longer time periods using Three-Way Control Charts. \n\n" +
					"AI MUST use this for deep history analysis or after significant organizational changes. NOT intended for routine daily analysis.\n" +
					"Detects systemic shifts, process drift, and long-term capability changes by analyzing monthly subgroups.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":   map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type": map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
						"window_months": map[string]interface{}{
							"type":        "integer",
							"description": "Number of months to analyze (default: 12, supports up to 60 for deep history)",
						},
					},
					"required": []string{"source_id", "source_type"},
				},
			},
			map[string]interface{}{
				"name": "get_workflow_discovery",
				"description": "Probe project status categories, residence times, and resolution frequencies to PROPOSE semantic mappings. \n\n" +
					"AI MUST use this to verify the workflow tiers and roles with the user BEFORE performing any other diagnostics. \n" +
					"The proposed mapping is a HEURISTIC and MUST be confirmed by the human via 'set_workflow_mapping'.\n\n" +
					"METAWORKFLOW SEMANTIC GUIDANCE:\n" +
					"- TIERS: 'Demand' (backlog), 'Upstream' (Analysis/Refinement), 'Downstream' (Development/Execution/Testing), 'Finished' (Terminal).\n" +
					"- ROLES: 'active' (working), 'queue' (waiting), 'ignore' (noise).\n" +
					"- OUTCOMES: 'delivered' (Value Provided), 'abandoned' (Work Discarded).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":   map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type": map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
					},
					"required": []string{"source_id", "source_type"},
				},
			},
			map[string]interface{}{
				"name": "set_workflow_mapping",
				"description": "Store user-confirmed semantic metadata (tier, role, outcome) for statuses and resolutions. This is the MANDATORY persistence step after the 'Inform & Veto' loop. \n\n" +
					"WITHOUT this mapping, most analytical tools in this server will provide SUBPAR or even WRONG results.\n\n" +
					"TIER DEFINITIONS: 'Demand' (Backlog), 'Upstream' (Analysis/Refinement), 'Downstream' (Development/Execution/Testing), 'Finished' (Terminal).\n" +
					"ROLE DEFINITIONS: 'active' (Value-adding work), 'queue' (Waiting), 'ignore' (Admin).\n" +
					"OUTCOME DEFINITIONS: 'delivered' (Successfully finished with value), 'abandoned' (Work stopped/discarded/cancelled).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id": map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"mapping": map[string]interface{}{
							"type":        "object",
							"description": "A map of status names to metadata (tier, role, and optional outcome).",
							"additionalProperties": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"tier":    map[string]interface{}{"type": "string", "enum": []string{"Demand", "Upstream", "Downstream", "Finished"}},
									"role":    map[string]interface{}{"type": "string", "enum": []string{"active", "queue", "ignore"}},
									"outcome": map[string]interface{}{"type": "string", "enum": []string{"delivered", "abandoned"}, "description": "Mandatory for 'Finished' tier statuses if resolutions are not used."},
								},
								"required": []string{"tier", "role"},
							},
						},
						"resolutions": map[string]interface{}{
							"type":        "object",
							"description": "Optional: A map of Jira resolution names to outcomes ('delivered' or 'abandoned').",
							"additionalProperties": map[string]interface{}{
								"type": "string",
								"enum": []string{"delivered", "abandoned"},
							},
						},
					},
					"required": []string{"source_id", "mapping"},
				},
			},
			map[string]interface{}{
				"name":        "get_process_yield",
				"description": "Analyze delivery efficiency across tiers. AI MUST ensure workflow tiers (Demand, Upstream, Downstream) have been verified with the user before interpreting these results.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":   map[string]interface{}{"type": "string"},
						"source_type": map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
					},
					"required": []string{"source_id", "source_type"},
				},
			},
			map[string]interface{}{
				"name":        "set_workflow_order",
				"description": "Explicity define the chronological order of statuses for a project to enable range-based analytics.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id": map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"order": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Ordered list of status names.",
						},
					},
					"required": []string{"source_id", "order"},
				},
			},
			map[string]interface{}{
				"name":        "get_item_journey",
				"description": "Get a detailed breakdown of where a single item spent its time across all workflow steps.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"issue_key": map[string]interface{}{"type": "string", "description": "The Jira issue key (e.g., PROJ-123)"},
					},
					"required": []string{"issue_key"},
				},
			},
			map[string]interface{}{
				"name":        "get_diagnostic_roadmap",
				"description": "Returns a recommended sequence of analysis steps based on the user's specific goal (e.g., forecasting, bottleneck analysis, capacity planning). Use this to align your analytical strategy with the project's current state.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"goal": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"forecasting", "bottlenecks", "capacity_planning", "system_health"},
							"description": "The analytical goal to get a roadmap for.",
						},
					},
					"required": []string{"goal"},
				},
			},

			map[string]interface{}{
				"name": "get_forecast_accuracy",
				"description": "Perform a 'Walk-Forward Analysis' (Backtesting) to empirically validate the accuracy of Monte-Carlo Forecasts. \n\n" +
					"This tool uses Time-Travel logic to reconstruct the state of the system at past points in time, runs a simulation, and checks if the ACTUAL outcome fell within the predicted cone. \n" +
					"Drift Protection: The analysis automatically stops blindly backtesting if it detects a System Drift (Process Shift via 3-Way Chart).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":             map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type":           map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
						"simulation_mode":       map[string]interface{}{"type": "string", "enum": []string{"duration", "scope"}},
						"items_to_forecast":     map[string]interface{}{"type": "integer", "description": "Number of items to forecast (duration mode). Default: 5"},
						"forecast_horizon_days": map[string]interface{}{"type": "integer", "description": "Number of days to forecast (scope mode). Default: 14"},
						"resolutions":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional: Resolutions to count as 'Done'."},
					},
					"required": []string{"source_id", "source_type", "simulation_mode"},
				},
			},
		},
	}
}
