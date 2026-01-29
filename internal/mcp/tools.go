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
				"description": "Run a Monte-Carlo simulation to forecast project outcomes based on historical throughput (work items / time). Use 'duration' mode to answer 'When will this be done?'. Use 'scope' mode to answer 'How much can we do?'.\n\n" +
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
				"name":        "get_cycle_time_assessment",
				"description": "Calculate Service Level Expectations (SLE) for a single item based on historical cycle times. Use this to answer 'How long does an item (of type X) typically take?'.",
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
				"description": "Analyze how long items spend in each status to identify bottlenecks.\n\n" +
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
				"description": "Identify which items are aging relative to historical norms. Allows choosing between 'WIP Age' (time since commitment) and 'Total Age' (time since creation).\n\n" +
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
				"name":        "get_process_stability",
				"description": "Analyzes process predictability using XmR behavior charts to detect 'Special Cause' variation. Compares the current WIP inventory against historical throughput to identify stability risks. Use this to determine if current forecasts are reliable.",
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
				"name":        "get_process_evolution",
				"description": "Perform a longitudinal 'Strategic Audit' of process behavior over longer time periods using Three-Way Control Charts. Detects systemic shifts, process drift, and long-term capability changes by analyzing monthly subgroups. Use this for deep history analysis or after significant organizational changes.",
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
				"description": "Probe project status categories and residence times to PROPOSE semantic mappings. AI MUST use this to verify the workflow tiers and roles with the user BEFORE performing diagnostics.\n\n" +
					"METAWORKFLOW SEMANTIC GUIDANCE:\n" +
					"- TIERS: 'Demand' (backlog), 'Upstream' (Analysis/Refinement), 'Downstream' (Development/Execution/Testing), 'Finished' (delivered/abandoned/aborted).\n" +
					"- ROLES: 'active' (working), 'queue' (waiting), 'ignore' (non-process noise).\n" +
					"- OUTCOMES: 'delivered' (value reached user), 'abandoned' (work discarded), 'aborted' (work discarded).",
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
				"description": "Store user-confirmed semantic metadata (tier and role) for statuses. This is the mandatory persistence step after the 'Inform & Veto' loop.\n\n" +
					"TIER DEFINITIONS: 'Demand' (Backlog), 'Upstream' (Analysis/Refinement), 'Downstream' (Development/Execution/Testing), 'Finished' (delivered/abandoned/aborted).\n" +
					"ROLE DEFINITIONS: 'active' (Value-adding work), 'queue' (Waiting for someone/something), 'ignore' (Admin/Non-delivery statuses).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id": map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"mapping": map[string]interface{}{
							"type":        "object",
							"description": "A map of status names to metadata (tier and role).",
							"additionalProperties": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"tier": map[string]interface{}{"type": "string", "enum": []string{"Demand", "Upstream", "Downstream", "Finished"}},
									"role": map[string]interface{}{"type": "string", "enum": []string{"active", "queue", "ignore"}},
								},
								"required": []string{"tier", "role"},
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
		},
	}
}
