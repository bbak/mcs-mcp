package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

type Server struct {
	jira             jira.Client
	workflowMappings map[string]map[string]stats.StatusMetadata // sourceID -> statusName -> metadata
	statusOrderings  map[string][]string                        // sourceID -> sorted status names
}

func NewServer(jiraClient jira.Client) *Server {
	return &Server{
		jira:             jiraClient,
		workflowMappings: make(map[string]map[string]stats.StatusMetadata),
		statusOrderings:  make(map[string][]string),
	}
}

func (s *Server) Start() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req struct {
			Jsonrpc string          `json:"jsonrpc"`
			ID      interface{}     `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}

		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		switch req.Method {
		case "initialize":
			s.sendResponse(req.ID, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"serverInfo":      map[string]interface{}{"name": "mcs-mcp", "version": "0.1.0"},
			})
		case "tools/list":
			s.sendResponse(req.ID, s.listTools())
		case "tools/call":
			res, err := s.callTool(req.Params)
			if err != nil {
				s.sendError(req.ID, err)
			} else {
				s.sendResponse(req.ID, res)
			}
		}
	}
}

func (s *Server) sendResponse(id interface{}, result interface{}) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	out, _ := json.Marshal(resp)
	fmt.Println(string(out))
}

func (s *Server) sendError(id interface{}, err interface{}) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   err,
	}
	out, _ := json.Marshal(resp)
	fmt.Println(string(out))
}

func (s *Server) listTools() interface{} {
	return map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{
				"name":        "find_jira_projects",
				"description": "Search for Jira projects by name or key (returns up to 20 results).",
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
				"description": "Search for Agile boards, optionally filtering by project key or name.",
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
				"description": "Probe a data source (board or filter) to analyze data quality, volume, and discover project-specific workflow statuses (Commitment Points).",
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
				"name":        "run_simulation",
				"description": "Run a Monte-Carlo simulation or Cycle-Time analysis. Use 'duration' mode to answer 'When will this be done?'. Use 'scope' mode to answer 'How much can we do?'.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":                map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type":              map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
						"mode":                     map[string]interface{}{"type": "string", "enum": []string{"duration", "scope", "single"}, "description": "Simulation mode: 'duration' (forecast completing a backlog), 'scope' (forecast delivery volume), or 'single' (cycle time)."},
						"include_existing_backlog": map[string]interface{}{"type": "boolean", "description": "If true, automatically counts and includes all unstarted items (Backlog) from Jira for this board/filter."},
						"include_wip":              map[string]interface{}{"type": "boolean", "description": "If true, ALSO includes items already in progress (started)."},
						"additional_items":         map[string]interface{}{"type": "integer", "description": "Additional items to include (e.g. new initiative/scope not yet in Jira)."},
						"backlog_size":             map[string]interface{}{"type": "integer", "description": "Alias for additional_items (deprecated, please use additional_items)."},
						"target_days":              map[string]interface{}{"type": "integer", "description": "Number of days (required for 'scope' mode)."},
						"target_date":              map[string]interface{}{"type": "string", "description": "Optional: Target date (YYYY-MM-DD). If provided, target_days is calculated automatically."},
						"start_status":             map[string]interface{}{"type": "string", "description": "Optional: Start status (Commitment Point)."},
						"end_status":               map[string]interface{}{"type": "string", "description": "Optional: End status (Resolution Point)."},
						"issue_types":              map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional: List of issue types to include (e.g., ['Story'])."},
						"resolutions":              map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional: Resolutions to count as 'Done'."},
					},
				},
			},
			map[string]interface{}{
				"name":        "get_status_persistence",
				"description": "Analyze how long items spend in each status to identify bottlenecks.",
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
				"name":        "get_inventory_aging_analysis",
				"description": "Identify which active items are aging relative to historical norms. Allows choosing between 'WIP Age' (time since commitment) and 'Total Age' (time since creation).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":   map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type": map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
						"aging_type":  map[string]interface{}{"type": "string", "enum": []string{"total", "wip"}, "description": "Type of age to calculate: 'total' (since creation) or 'wip' (since commitment)."},
					},
					"required": []string{"source_id", "source_type", "aging_type"},
				},
			},
			map[string]interface{}{
				"name":        "get_delivery_cadence",
				"description": "Visualize the weekly pulse of delivery to detect flow vs. batching.",
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
				"name":        "get_workflow_discovery",
				"description": "Probe project status categories and residence times to propose semantic mappings. AI MUST use this to verify the workflow tiers and roles with the user BEFORE performing diagnostics.",
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
				"name":        "set_workflow_mapping",
				"description": "Store user-confirmed semantic metadata (tier and role) for statuses. This is the mandatory persistence step after the 'Inform & Veto' loop.",
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

func (s *Server) handleGetDataMetadata(sourceID, sourceType string) (interface{}, error) {
	jql, err := s.getJQL(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Probe: Fetch last 200 resolved items to analyze data quality (with history for reachability)
	probeJQL := fmt.Sprintf("(%s) AND resolution is not EMPTY ORDER BY resolutiondate DESC", jql)
	issues, total, err := s.jira.SearchIssuesWithHistory(probeJQL, 0, 200)
	if err != nil {
		return nil, err
	}

	summary := stats.AnalyzeProbe(issues, total)

	// Backlog Discovery: Count unresolved items
	backlogJQL := fmt.Sprintf("(%s) AND resolution is EMPTY", jql)
	_, backlogSize, err := s.jira.SearchIssues(backlogJQL, 0, 0)
	if err == nil {
		summary.BacklogSize = backlogSize
	}

	// Status Discovery
	var projectKey string
	if sourceType == "board" {
		var id int
		if _, err := fmt.Sscanf(sourceID, "%d", &id); err == nil {
			board, err := s.jira.GetBoard(id)
			if err == nil {
				bMap := board.(map[string]interface{})
				if loc, ok := bMap["location"].(map[string]interface{}); ok {
					projectKey = fmt.Sprintf("%v", loc["projectKey"])
				}
			}
		}
	}

	// Fallback/Filter: Get project key from first issue
	if projectKey == "" && len(issues) > 0 {
		keyParts := strings.Split(issues[0].Key, "-")
		if len(keyParts) > 1 {
			projectKey = keyParts[0]
		}
	}

	if projectKey != "" {
		statuses, err := s.jira.GetProjectStatuses(projectKey)
		if err == nil {
			summary.AvailableStatuses = statuses
			statusWeights := s.getStatusWeights(projectKey)
			summary.CommitmentPointHints = s.getCommitmentPointHints(issues, statusWeights)
		}
	}

	return summary, nil
}

func (s *Server) handleRunSimulation(sourceID, sourceType, mode string, includeExistingBacklog bool, additionalItems int, targetDays int, targetDate string, startStatus, endStatus string, issueTypes []string, includeWIP bool, resolutions []string) (interface{}, error) {
	// 1. Get JQL
	jql, err := s.getJQL(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// 2. Ingestion: Fetch last 6 months of historical data
	startTime := time.Now().AddDate(0, -6, 0)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		jql, startTime.Format("2006-01-02"))

	log.Debug().Str("jql", ingestJQL).Msg("Starting historical ingestion for simulation")

	var issues []jira.Issue
	// Use history if needed for cycle time analysis OR WIP aging
	if mode == "single" || startStatus != "" || includeWIP {
		issues, _, err = s.jira.SearchIssuesWithHistory(ingestJQL, 0, 1000)
	} else {
		issues, _, err = s.jira.SearchIssues(ingestJQL, 0, 1000)
	}
	if err != nil {
		return nil, err
	}

	if len(issues) == 0 {
		return nil, fmt.Errorf("no historical data found in the last 6 months to base simulation on")
	}

	// 3. Analytics Context (WIP Aging & Status Weights)
	projectKey := ""
	if len(issues) > 0 {
		parts := strings.Split(issues[0].Key, "-")
		if len(parts) > 1 {
			projectKey = parts[0]
		}
	}
	statusWeights := s.getStatusWeights(projectKey)
	// Override weights with verified mappings if available to ensure correct backflow detection
	if m, ok := s.workflowMappings[sourceID]; ok {
		for name, metadata := range m {
			if metadata.Tier == "Demand" {
				statusWeights[name] = 1
			} else if metadata.Tier == "Downstream" || metadata.Tier == "Finished" {
				if statusWeights[name] < 2 {
					statusWeights[name] = 2
				}
			}
		}
	}

	// Apply Backflow Policy (Discard pre-backflow history)
	cWeight := 2
	if startStatus != "" {
		if w, ok := statusWeights[startStatus]; ok {
			cWeight = w
		}
	}
	issues = s.applyBackflowPolicy(issues, statusWeights, cWeight)
	var wipAges []float64
	wipCount := 0

	existingBacklog := 0
	if includeExistingBacklog {
		backlogJQL := fmt.Sprintf("(%s) AND resolution is EMPTY", jql)
		_, total, err := s.jira.SearchIssues(backlogJQL, 0, 0)
		if err == nil {
			existingBacklog = total
		}
	}

	if includeWIP {
		wipJQL := fmt.Sprintf("(%s) AND resolution is EMPTY", jql)
		wipIssues, _, err := s.jira.SearchIssuesWithHistory(wipJQL, 0, 1000)
		if err == nil {
			wipIssues = s.applyBackflowPolicy(wipIssues, statusWeights, cWeight)
			cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, statusWeights, resolutions)
			calcWipAges := stats.CalculateInventoryAge(wipIssues, startStatus, statusWeights, cycleTimes, "wip")
			for _, wa := range calcWipAges {
				if wa.AgeDays != nil {
					wipAges = append(wipAges, *wa.AgeDays)
					wipCount++
				}
			}
		}
	}

	// 4. Mode Selection
	engine := simulation.NewEngine(nil)

	switch mode {
	case "single":
		log.Info().Str("startStatus", startStatus).Msg("Running Cycle Time Analysis (Single Item)")

		projectKey := ""
		if len(issues) > 0 {
			parts := strings.Split(issues[0].Key, "-")
			if len(parts) > 1 {
				projectKey = parts[0]
			}
		}

		statusWeights := s.getStatusWeights(projectKey)
		cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, statusWeights, resolutions)

		if len(cycleTimes) == 0 {
			msg := fmt.Sprintf("no resolved items found that passed the commitment point '%s'.", startStatus)
			hints := s.getCommitmentPointHints(issues, statusWeights)
			if len(hints) > 0 {
				msg += "\n\n💡 Hint: Based on historical reachability, these statuses were frequently used as work started: [" + strings.Join(hints, ", ") + "].\n(⚠️ Note: These are inferred from status categories and transition history; please verify if they represent your actual commitment point.)"
			}
			return nil, fmt.Errorf("%s", msg)
		}
		engine = simulation.NewEngine(&simulation.Histogram{})
		resObj := engine.RunCycleTimeAnalysis(cycleTimes)
		if includeWIP {
			engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, 0)
		}
		return resObj, nil

	case "scope":
		finalDays := targetDays
		if targetDate != "" {
			t, err := time.Parse("2006-01-02", targetDate)
			if err != nil {
				return nil, fmt.Errorf("invalid target_date format: %w", err)
			}
			diff := time.Until(t)
			if diff < 0 {
				return nil, fmt.Errorf("target_date must be in the future")
			}
			finalDays = int(diff.Hours() / 24)
		}

		if finalDays <= 0 {
			return nil, fmt.Errorf("target_days must be > 0 (or target_date must be in the future) for scope simulation")
		}

		log.Info().Int("days", finalDays).Any("types", issueTypes).Msg("Running Scope Simulation")
		h := simulation.NewHistogram(issues, startTime, time.Now(), issueTypes, resolutions)
		engine = simulation.NewEngine(h)
		resObj := engine.RunScopeSimulation(finalDays, 10000)

		resObj.Insights = append(resObj.Insights, "Scope Interpretation: Forecast shows total items that will reach 'Done' status, including items currently in progress.")

		if includeWIP {
			cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, statusWeights, resolutions)
			engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, 0)
			resObj.Composition = simulation.Composition{
				WIP:             wipCount,
				ExistingBacklog: existingBacklog,
				AdditionalItems: additionalItems,
				Total:           0, // Total doesn't make sense as input in Scope mode
			}
		}

		resObj.Context = map[string]interface{}{
			"forecast_window_days": finalDays,
			"target_date":          targetDate,
		}
		return resObj, nil

	case "duration":
		if !includeExistingBacklog && additionalItems <= 0 && !includeWIP {
			return nil, fmt.Errorf("either include_existing_backlog: true, additional_items > 0, OR include_wip: true must be provided for duration simulation")
		}

		actualBacklog := existingBacklog + additionalItems + wipCount
		log.Info().Int("total", actualBacklog).Int("backlog", existingBacklog).Int("additional", additionalItems).Int("wip", wipCount).Any("types", issueTypes).Msg("Running Duration Simulation")

		h := simulation.NewHistogram(issues, startTime, time.Now(), issueTypes, resolutions)
		engine = simulation.NewEngine(h)
		resObj := engine.RunDurationSimulation(actualBacklog, 10000)

		// Set Scope Composition for AI transparency
		resObj.Composition = simulation.Composition{
			ExistingBacklog: existingBacklog,
			WIP:             wipCount,
			AdditionalItems: additionalItems,
			Total:           actualBacklog,
		}

		// Add Advanced Reliability Analysis
		cycleTimes := s.getCycleTimes(sourceID, issues, startStatus, endStatus, statusWeights, resolutions)
		engine.AnalyzeWIPStability(&resObj, wipAges, cycleTimes, existingBacklog+additionalItems)

		if (existingBacklog+additionalItems) == 0 && includeWIP {
			resObj.Warnings = append(resObj.Warnings, fmt.Sprintf("Note: This forecast ONLY covers the %d items currently in progress. Unstarted backlog items were not included.", wipCount))
		}

		if includeWIP && (existingBacklog+additionalItems) > 0 && wipCount > (existingBacklog+additionalItems)*3 {
			resObj.Warnings = append(resObj.Warnings, fmt.Sprintf("High operational load: You have %d items in progress, which is significantly larger than the %d unstarted items in this forecast. Lead times for new items may be long.", wipCount, existingBacklog+additionalItems))
		}
		return resObj, nil

	default:
		// Auto-detect if mode not explicitly provided
		if targetDays > 0 || targetDate != "" {
			return s.handleRunSimulation(sourceID, sourceType, "scope", false, 0, targetDays, targetDate, "", "", nil, false, resolutions)
		}
		if additionalItems > 0 || includeExistingBacklog {
			return s.handleRunSimulation(sourceID, sourceType, "duration", includeExistingBacklog, additionalItems, 0, "", "", "", nil, false, resolutions)
		}
		return nil, fmt.Errorf("mode ('duration', 'scope', 'single') or required parameters must be provided")
	}
}

func (s *Server) handleGetStatusPersistence(sourceID, sourceType string) (interface{}, error) {
	jql, err := s.getJQL(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Fetch last 6 months of resolved items with history
	startTime := time.Now().AddDate(0, -6, 0)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		jql, startTime.Format("2006-01-02"))

	issues, _, err := s.jira.SearchIssuesWithHistory(ingestJQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	results := stats.CalculateStatusPersistence(issues)

	// Enrich with categories and session mappings
	projectKey := s.extractProjectKey(issues)
	categories := s.getStatusCategories(projectKey)
	mappings := s.workflowMappings[sourceID]

	return stats.EnrichStatusPersistence(results, categories, mappings), nil
}

func (s *Server) handleGetWorkflowDiscovery(sourceID, sourceType string) (interface{}, error) {
	jql, err := s.getJQL(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// 1. Get historical issues for residence time
	startTime := time.Now().AddDate(0, -6, 0)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		jql, startTime.Format("2006-01-02"))

	issues, _, err := s.jira.SearchIssuesWithHistory(ingestJQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	return s.getWorkflowDiscovery(sourceID, issues), nil
}

func (s *Server) getWorkflowDiscovery(sourceID string, issues []jira.Issue) []stats.StatusPersistence {
	results := stats.CalculateStatusPersistence(issues)

	// Enrichment
	projectKey := s.extractProjectKey(issues)
	categories := s.getStatusCategories(projectKey)
	mappings := s.workflowMappings[sourceID]

	enriched := stats.EnrichStatusPersistence(results, categories, mappings)

	// Add "Recommended Roles" for AI
	for i := range enriched {
		res := &enriched[i]
		if res.Role == "" {
			switch strings.ToUpper(res.Category) {
			case "TO DO", "NEW":
				res.Role = "backlog"
			case "IN PROGRESS", "INDETERMINATE":
				res.Role = "active"
			case "DONE":
				res.Role = "done"
			default:
				res.Role = "active" // Default recommendation
			}
		}
	}

	return enriched
}

func (s *Server) handleSetWorkflowMapping(sourceID string, mapping map[string]interface{}) (interface{}, error) {
	if s.workflowMappings[sourceID] == nil {
		s.workflowMappings[sourceID] = make(map[string]stats.StatusMetadata)
	}

	for k, v := range mapping {
		obj, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		s.workflowMappings[sourceID][k] = stats.StatusMetadata{
			Tier: fmt.Sprintf("%v", obj["tier"]),
			Role: fmt.Sprintf("%v", obj["role"]),
		}
	}

	return map[string]string{"status": "success", "message": fmt.Sprintf("Stored %d workflow mappings for source %s", len(mapping), sourceID)}, nil
}

func (s *Server) extractProjectKey(issues []jira.Issue) string {
	if len(issues) == 0 {
		return ""
	}
	parts := strings.Split(issues[0].Key, "-")
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}

func (s *Server) getStatusCategories(projectKey string) map[string]string {
	cats := make(map[string]string)
	if projectKey == "" {
		return cats
	}

	if statuses, err := s.jira.GetProjectStatuses(projectKey); err == nil {
		for _, itm := range statuses.([]interface{}) {
			issueTypeMap := itm.(map[string]interface{})
			statusList := issueTypeMap["statuses"].([]interface{})
			for _, sObj := range statusList {
				sMap := sObj.(map[string]interface{})
				name := sMap["name"].(string)
				cat := sMap["statusCategory"].(map[string]interface{})
				cats[name] = fmt.Sprintf("%v", cat["key"])
			}
		}
	}
	return cats
}

func (s *Server) handleSetWorkflowOrder(sourceID string, order []string) (interface{}, error) {
	s.statusOrderings[sourceID] = order
	return map[string]string{"status": "success", "message": fmt.Sprintf("Stored workflow order for source %s", sourceID)}, nil
}

func (s *Server) handleGetItemJourney(key string) (interface{}, error) {
	// 1. Fetch the issue with history to get reliable residency
	jql := fmt.Sprintf("key = %s", key)
	issues, _, err := s.jira.SearchIssuesWithHistory(jql, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("issue %s not found", key)
	}

	issue := issues[0]

	type JourneyStep struct {
		Status string  `json:"status"`
		Days   float64 `json:"days"`
	}
	var steps []JourneyStep

	// Use the transitions to build a chronological journey
	if len(issue.Transitions) > 0 {
		// First segment: from creation to first transition
		firstDuration := issue.Transitions[0].Date.Sub(issue.Created).Seconds()
		steps = append(steps, JourneyStep{
			Status: "Created",
			Days:   math.Round((firstDuration/86400.0)*10) / 10,
		})

		for i := 0; i < len(issue.Transitions)-1; i++ {
			duration := issue.Transitions[i+1].Date.Sub(issue.Transitions[i].Date).Seconds()
			steps = append(steps, JourneyStep{
				Status: issue.Transitions[i].ToStatus,
				Days:   math.Round((duration/86400.0)*10) / 10,
			})
		}

		// Final segment: from last transition to current/resolution
		var finalDate time.Time
		if issue.ResolutionDate != nil {
			finalDate = *issue.ResolutionDate
		} else {
			finalDate = time.Now()
		}
		lastTrans := issue.Transitions[len(issue.Transitions)-1]
		finalDuration := finalDate.Sub(lastTrans.Date).Seconds()
		steps = append(steps, JourneyStep{
			Status: lastTrans.ToStatus,
			Days:   math.Round((finalDuration/86400.0)*10) / 10,
		})
	}

	residencyDays := make(map[string]float64)
	for s, sec := range issue.StatusResidency {
		residencyDays[s] = math.Round((float64(sec)/86400.0)*10) / 10
	}

	return map[string]interface{}{
		"key":       issue.Key,
		"summary":   issue.Summary,
		"residency": residencyDays,
		"path":      steps,
	}, nil
}

func (s *Server) handleGetProcessYield(sourceID, sourceType string) (interface{}, error) {
	jql, err := s.getJQL(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Fetch last 6 months of resolved items with history
	startTime := time.Now().AddDate(0, -6, 0)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		jql, startTime.Format("2006-01-02"))

	issues, _, err := s.jira.SearchIssuesWithHistory(ingestJQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	mappings := s.workflowMappings[sourceID]
	resolutions := s.getResolutionMap()

	return stats.CalculateProcessYield(issues, mappings, resolutions), nil
}

func (s *Server) getResolutionMap() map[string]string {
	// Simple heuristic for now. In a real system, we'd fetch this from Jira
	// or allow user configuration.
	return map[string]string{
		"Fixed":            "delivered",
		"Done":             "delivered",
		"Complete":         "delivered",
		"Resolved":         "delivered",
		"Duplicate":        "abandoned",
		"Won't Do":         "abandoned",
		"Cannot Reproduce": "abandoned",
		"Obsolete":         "abandoned",
		"Incomplete":       "abandoned",
		"Abandoned":        "abandoned",
		"Withdrawn":        "abandoned",
	}
}

func (s *Server) handleGetInventoryAgingAnalysis(sourceID, sourceType, agingType string) (interface{}, error) {
	jql, err := s.getJQL(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// 1. Get History for Baseline Metrics
	startTime := time.Now().AddDate(0, -6, 0)
	histJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		jql, startTime.Format("2006-01-02"))

	histIssues, _, err := s.jira.SearchIssuesWithHistory(histJQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	// Determine commitment context
	projectKey := s.extractProjectKey(histIssues)
	statusWeights := s.getStatusWeights(projectKey)

	// Override weights with verified mappings if available to ensure correct backflow detection
	if m, ok := s.workflowMappings[sourceID]; ok {
		for name, metadata := range m {
			if metadata.Tier == "Demand" {
				statusWeights[name] = 1
			} else if metadata.Tier == "Downstream" || metadata.Tier == "Finished" {
				// Ensure it's at least weight 2 for commitment
				if statusWeights[name] < 2 {
					statusWeights[name] = 2
				}
			}
		}
	}

	// 1b. Apply Backflow Policy (Discard pre-backflow history)
	commitmentWeight := 2
	histIssues = s.applyBackflowPolicy(histIssues, statusWeights, commitmentWeight)

	// Fetch appropriate baseline
	var baseline []float64
	resolutions := []string{"Fixed", "Done", "Complete", "Resolved"}
	if agingType == "total" {
		baseline = s.getTotalAges(histIssues, resolutions)
	} else {
		baseline = s.getCycleTimes(sourceID, histIssues, "", "", statusWeights, resolutions)
	}

	// 3. Determine Verification Status (for WIP mode)
	verified := false
	if m, ok := s.workflowMappings[sourceID]; ok && len(m) > 0 {
		verified = true
	}

	if agingType == "wip" && !verified {
		// PRECONDITION REFUSAL: Provide discovery data instead of performing expensive WIP calculation
		discovery := s.getWorkflowDiscovery(sourceID, histIssues)
		return map[string]interface{}{
			"status":       "precondition_required",
			"message":      "WIP Aging analysis requires a verified Commitment Point (semantic mapping).",
			"discovery":    discovery,
			"instructions": "The 'WIP' calculation remains invalid until the commitment point is confirmed. Please present the above workflow discovery to the user, propose a mapping (meta-workflow tiers and roles), and confirm it via 'set_workflow_mapping' before re-running this tool.",
		}, nil
	}

	// 2. Get Current WIP (up to 1000 oldest items)
	wipJQL := fmt.Sprintf("(%s) AND resolution is EMPTY ORDER BY created ASC", jql)
	wipIssues, _, err := s.jira.SearchIssuesWithHistory(wipJQL, 0, 1000)
	if err != nil {
		return nil, err
	}
	wipIssues = s.applyBackflowPolicy(wipIssues, statusWeights, commitmentWeight)

	ctx := map[string]interface{}{
		"aging_type": agingType,
	}

	if agingType == "wip" {
		ctx["commitment_point_verified"] = verified
	}

	var startStatus string
	if verified {
		startStatus = s.getEarliestCommitment(sourceID)
	}

	// Return neutral wrapped response
	return map[string]interface{}{
		"inventory_aging": stats.CalculateInventoryAge(wipIssues, startStatus, statusWeights, baseline, agingType),
		"context":         ctx,
	}, nil
}

func (s *Server) handleGetDeliveryCadence(sourceID, sourceType string, windowWeeks int) (interface{}, error) {
	jql, err := s.getJQL(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// Fetch history within the window
	startTime := time.Now().AddDate(0, 0, -windowWeeks*7)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		jql, startTime.Format("2006-01-02"))

	issues, _, err := s.jira.SearchIssues(ingestJQL, 0, 2000)
	if err != nil {
		return nil, err
	}

	return stats.CalculateDeliveryCadence(issues, windowWeeks), nil
}

func (s *Server) getJQL(sourceID, sourceType string) (string, error) {
	var jql string
	if sourceType == "board" {
		var id int
		_, err := fmt.Sscanf(sourceID, "%d", &id)
		if err != nil {
			return "", fmt.Errorf("invalid board ID: %s", sourceID)
		}
		config, err := s.jira.GetBoardConfig(id)
		if err != nil {
			return "", err
		}
		cMap := config.(map[string]interface{})
		filterObj := cMap["filter"].(map[string]interface{})
		filterID := fmt.Sprintf("%v", filterObj["id"])
		filter, err := s.jira.GetFilter(filterID)
		if err != nil {
			return "", err
		}
		jql = filter.(map[string]interface{})["jql"].(string)
	} else {
		filter, err := s.jira.GetFilter(sourceID)
		if err != nil {
			return "", err
		}
		jql = filter.(map[string]interface{})["jql"].(string)
	}

	// Strip existing ORDER BY to avoid collision
	jqlLower := strings.ToLower(jql)
	if idx := strings.Index(jqlLower, " order by"); idx != -1 {
		jql = jql[:idx]
	}
	return jql, nil
}

func (s *Server) callTool(params json.RawMessage) (interface{}, interface{}) {
	var call struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, map[string]interface{}{"code": -32602, "message": "Invalid params"}
	}

	var data interface{}
	var err error

	switch call.Name {
	case "find_jira_projects":
		data, err = s.jira.FindProjects(asString(call.Arguments["query"]))
	case "find_jira_boards":
		pKey := asString(call.Arguments["project_key"])
		nFilter := asString(call.Arguments["name_filter"])
		data, err = s.jira.FindBoards(pKey, nFilter)
	case "get_project_details":
		key := asString(call.Arguments["project_key"])
		data, err = s.jira.GetProject(key)
	case "get_board_details":
		id := asInt(call.Arguments["board_id"])
		data, err = s.jira.GetBoard(id)
	case "get_data_metadata":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		data, err = s.handleGetDataMetadata(id, sType)
	case "run_simulation":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		mode := asString(call.Arguments["mode"])
		startStatus := asString(call.Arguments["start_status"])

		includeExisting := false
		if b, ok := call.Arguments["include_existing_backlog"].(bool); ok {
			includeExisting = b
		}

		additional := asInt(call.Arguments["additional_items"])
		if additional == 0 {
			additional = asInt(call.Arguments["backlog_size"]) // Compatibility/Alias
		}

		targetDays := asInt(call.Arguments["target_days"])
		targetDate := asString(call.Arguments["target_date"])

		var issueTypes []string
		if it, ok := call.Arguments["issue_types"].([]interface{}); ok {
			for _, v := range it {
				issueTypes = append(issueTypes, asString(v))
			}
		}

		var includeWIP bool
		if w, ok := call.Arguments["include_wip"].(bool); ok {
			includeWIP = w
		}

		var res []string
		if r, ok := call.Arguments["resolutions"].([]interface{}); ok {
			for _, v := range r {
				res = append(res, asString(v))
			}
		}
		data, err = s.handleRunSimulation(id, sType, mode, includeExisting, additional, targetDays, targetDate, startStatus, asString(call.Arguments["end_status"]), issueTypes, includeWIP, res)
	case "get_status_persistence":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		data, err = s.handleGetStatusPersistence(id, sType)
	case "get_inventory_aging_analysis":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		agingType := asString(call.Arguments["aging_type"])
		data, err = s.handleGetInventoryAgingAnalysis(id, sType, agingType)
	case "get_delivery_cadence":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		window := asInt(call.Arguments["window_weeks"])
		if window == 0 {
			window = 26
		}
		data, err = s.handleGetDeliveryCadence(id, sType, window)
	case "get_process_yield":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		data, err = s.handleGetProcessYield(id, sType)
	case "get_workflow_discovery":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		data, err = s.handleGetWorkflowDiscovery(id, sType)
	case "set_workflow_mapping":
		id := asString(call.Arguments["source_id"])
		mapping, _ := call.Arguments["mapping"].(map[string]interface{})
		data, err = s.handleSetWorkflowMapping(id, mapping)
	case "set_workflow_order":
		id := asString(call.Arguments["source_id"])
		order := []string{}
		if o, ok := call.Arguments["order"].([]interface{}); ok {
			for _, v := range o {
				order = append(order, asString(v))
			}
		}
		data, err = s.handleSetWorkflowOrder(id, order)
	case "get_item_journey":
		key := asString(call.Arguments["issue_key"])
		data, err = s.handleGetItemJourney(key)
	default:
		return nil, map[string]interface{}{"code": -32601, "message": "Tool not found"}
	}

	if err != nil {
		return nil, map[string]interface{}{"code": -32000, "message": err.Error()}
	}

	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": s.formatResult(data),
			},
		},
	}, nil
}

func (s *Server) formatResult(data interface{}) string {
	out, _ := json.MarshalIndent(data, "", "  ")
	return string(out)
}

func (s *Server) getStatusWeights(projectKey string) map[string]int {
	weights := make(map[string]int)
	if projectKey == "" {
		return weights
	}

	if statuses, err := s.jira.GetProjectStatuses(projectKey); err == nil {
		for _, itm := range statuses.([]interface{}) {
			issueTypeMap := itm.(map[string]interface{})
			statusList := issueTypeMap["statuses"].([]interface{})
			for _, sObj := range statusList {
				sMap := sObj.(map[string]interface{})
				name := sMap["name"].(string)
				cat := sMap["statusCategory"].(map[string]interface{})
				key := cat["key"].(string)

				weight := 1
				switch key {
				case "indeterminate":
					weight = 2
				case "done":
					weight = 3
				}
				weights[name] = weight
			}
		}
	}
	return weights
}

func (s *Server) getTotalAges(issues []jira.Issue, resolutions []string) []float64 {
	resMap := make(map[string]bool)
	for _, r := range resolutions {
		resMap[r] = true
	}

	var ages []float64
	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}
		if len(resolutions) > 0 && !resMap[issue.Resolution] {
			continue
		}

		duration := issue.ResolutionDate.Sub(issue.Created).Hours() / 24.0
		if duration > 0 {
			ages = append(ages, duration)
		}
	}

	return ages
}

func (s *Server) getCycleTimes(sourceID string, issues []jira.Issue, startStatus, endStatus string, statusWeights map[string]int, resolutions []string) []float64 {
	resMap := make(map[string]bool)
	for _, r := range resolutions {
		resMap[r] = true
	}

	rangeStatuses := s.getInferredRange(sourceID, startStatus, endStatus, issues, statusWeights)

	var cycleTimes []float64
	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}
		if len(resolutions) > 0 && !resMap[issue.Resolution] {
			continue
		}

		duration := stats.SumRangeDuration(issue, rangeStatuses)
		if duration > 0 {
			cycleTimes = append(cycleTimes, duration)
		}
	}

	return cycleTimes
}

func (s *Server) getInferredRange(sourceID, startStatus, endStatus string, issues []jira.Issue, statusWeights map[string]int) []string {
	// 1. Check if we have a persisted session ordering
	if order, ok := s.statusOrderings[sourceID]; ok {
		return s.sliceRange(order, startStatus, endStatus)
	}

	// 2. Fallback: Inferred order from historical reachability/categories
	// We'll use the statuses present in the issues
	statusMap := make(map[string]bool)
	for _, issue := range issues {
		for st := range issue.StatusResidency {
			statusMap[st] = true
		}
	}
	var allStatuses []string
	for st := range statusMap {
		allStatuses = append(allStatuses, st)
	}

	// Simple heuristic sort: by weight, then by name
	sort.Slice(allStatuses, func(i, j int) bool {
		wi := statusWeights[allStatuses[i]]
		wj := statusWeights[allStatuses[j]]
		if wi != wj {
			return wi < wj
		}
		return allStatuses[i] < allStatuses[j]
	})

	return s.sliceRange(allStatuses, startStatus, endStatus)
}

func (s *Server) sliceRange(order []string, start, end string) []string {
	startIndex := 0
	if start != "" {
		for i, st := range order {
			if st == start {
				startIndex = i
				break
			}
		}
	}

	endIndex := len(order) - 1
	if end != "" {
		for i, st := range order {
			if st == end {
				endIndex = i
				break
			}
		}
	}

	if startIndex > endIndex {
		return []string{order[startIndex]} // Fallback to just the start status
	}

	return order[startIndex : endIndex+1]
}

func (s *Server) getEarliestCommitment(sourceID string) string {
	mappings := s.workflowMappings[sourceID]
	order := s.statusOrderings[sourceID]
	if len(mappings) == 0 {
		return ""
	}

	// Try to find status mapped to 'Downstream'
	// If we have an ordering, use it to find the first one
	if len(order) > 0 {
		for _, status := range order {
			if m, ok := mappings[status]; ok && (m.Tier == "Downstream" || m.Tier == "Finished") {
				return status
			}
		}
	} else {
		// Fallback: search all mappings
		for status, m := range mappings {
			if m.Tier == "Downstream" {
				return status
			}
		}
	}
	return ""
}

func (s *Server) getCommitmentPointHints(issues []jira.Issue, statusWeights map[string]int) []string {
	reachability := make(map[string]int)
	for _, issue := range issues {
		visited := make(map[string]bool)
		for _, trans := range issue.Transitions {
			visited[trans.ToStatus] = true
		}
		for status := range visited {
			reachability[status]++
		}
	}

	type candidate struct {
		name  string
		count int
	}
	var candidates []candidate
	for name, count := range reachability {
		// Prioritize "Indeterminate" (weight 2) categories as commitment point candidates
		if weight, ok := statusWeights[name]; ok && weight == 2 {
			candidates = append(candidates, candidate{name, count})
		}
	}

	// Sort candidates by frequency of usage
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].count > candidates[i].count {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	var result []string
	for i := 0; i < len(candidates) && i < 3; i++ {
		result = append(result, candidates[i].name)
	}
	return result
}

func (s *Server) applyBackflowPolicy(issues []jira.Issue, statusWeights map[string]int, commitmentWeight int) []jira.Issue {
	cleaned := make([]jira.Issue, len(issues))
	for i, issue := range issues {
		lastBackflowIdx := -1
		for j, t := range issue.Transitions {
			if w, ok := statusWeights[t.ToStatus]; ok && w < commitmentWeight {
				lastBackflowIdx = j
			}
		}

		if lastBackflowIdx == -1 {
			cleaned[i] = issue
			continue
		}

		// Keep original Created date to preserve Total Age
		newIssue := issue
		newIssue.Transitions = nil
		if lastBackflowIdx < len(issue.Transitions)-1 {
			newIssue.Transitions = issue.Transitions[lastBackflowIdx+1:]
		}

		// Rebuild residency from the new starting point
		newIssue.StatusResidency = s.rebuildResidency(newIssue, issue.Transitions[lastBackflowIdx].ToStatus)
		cleaned[i] = newIssue
	}
	return cleaned
}

func (s *Server) rebuildResidency(issue jira.Issue, initialStatus string) map[string]int64 {
	residency := make(map[string]int64)
	if len(issue.Transitions) == 0 {
		var finalDate time.Time
		if issue.ResolutionDate != nil {
			finalDate = *issue.ResolutionDate
		} else {
			finalDate = time.Now()
		}
		duration := int64(finalDate.Sub(issue.Created).Seconds())
		if duration > 0 {
			residency[initialStatus] = duration
		}
		return residency
	}

	// 1. Initial status duration
	firstDuration := int64(issue.Transitions[0].Date.Sub(issue.Created).Seconds())
	if firstDuration > 0 {
		residency[initialStatus] = firstDuration
	}

	// 2. Intermediate transitions
	for i := 0; i < len(issue.Transitions)-1; i++ {
		duration := int64(issue.Transitions[i+1].Date.Sub(issue.Transitions[i].Date).Seconds())
		if duration > 0 {
			residency[issue.Transitions[i].ToStatus] += duration
		}
	}

	// 3. Last status duration
	var finalDate time.Time
	if issue.ResolutionDate != nil {
		finalDate = *issue.ResolutionDate
	} else {
		finalDate = time.Now()
	}
	lastTrans := issue.Transitions[len(issue.Transitions)-1]
	finalDuration := int64(finalDate.Sub(lastTrans.Date).Seconds())
	if finalDuration > 0 {
		residency[lastTrans.ToStatus] += finalDuration
	}

	return residency
}

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func asInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case string:
		var res int
		fmt.Sscanf(val, "%d", &res)
		return res
	default:
		return 0
	}
}
