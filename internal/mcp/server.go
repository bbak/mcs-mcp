package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

type Server struct {
	jira jira.Client
}

func NewServer(jiraClient jira.Client) *Server {
	return &Server{jira: jiraClient}
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
						"start_status":             map[string]interface{}{"type": "string", "description": "Optional: Commitment Point status. Used to identify WIP."},
						"issue_types":              map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional: List of issue types to include (e.g., ['Story'])."},
						"resolutions":              map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional: Resolutions to count as 'Done'."},
					},
					"required": []string{"source_id", "source_type"},
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

func (s *Server) handleRunSimulation(sourceID, sourceType, mode string, includeExistingBacklog bool, additionalItems int, targetDays int, targetDate string, startStatus string, issueTypes []string, includeWIP bool, resolutions []string) (interface{}, error) {
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
			commitmentWeight := 2
			if startStatus != "" {
				if w, ok := statusWeights[startStatus]; ok {
					commitmentWeight = w
				}
			}

			for _, wIssue := range wipIssues {
				isWIP := false
				clockStart := wIssue.Created
				var earliestCommitment *time.Time

				if weight, ok := statusWeights[wIssue.Status]; ok && weight >= commitmentWeight {
					isWIP = true
				} else if startStatus == "" {
					isWIP = true
				}

				if isWIP {
					for _, trans := range wIssue.Transitions {
						weight, ok := statusWeights[trans.ToStatus]
						if (startStatus != "" && trans.ToStatus == startStatus) || (ok && weight >= commitmentWeight) {
							if earliestCommitment == nil || trans.Date.Before(*earliestCommitment) {
								t := trans.Date
								earliestCommitment = &t
							}
						}
					}
					if earliestCommitment != nil {
						clockStart = *earliestCommitment
					}
					wipCount++
					wipAges = append(wipAges, time.Since(clockStart).Hours()/24.0)
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
		cycleTimes := s.getCycleTimes(issues, startStatus, statusWeights, resolutions)

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
			cycleTimes := s.getCycleTimes(issues, startStatus, statusWeights, resolutions)
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
		cycleTimes := s.getCycleTimes(issues, startStatus, statusWeights, resolutions)
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
			return s.handleRunSimulation(sourceID, sourceType, "scope", false, 0, targetDays, targetDate, "", nil, false, resolutions)
		}
		if additionalItems > 0 || includeExistingBacklog {
			return s.handleRunSimulation(sourceID, sourceType, "duration", includeExistingBacklog, additionalItems, 0, "", "", nil, false, resolutions)
		}
		return nil, fmt.Errorf("mode ('duration', 'scope', 'single') or required parameters must be provided")
	}
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
		data, err = s.handleRunSimulation(id, sType, mode, includeExisting, additional, targetDays, targetDate, startStatus, issueTypes, includeWIP, res)
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

func (s *Server) getCycleTimes(issues []jira.Issue, startStatus string, statusWeights map[string]int, resolutions []string) []float64 {
	commitmentWeight := 2
	if startStatus != "" {
		if w, ok := statusWeights[startStatus]; ok {
			commitmentWeight = w
		}
	}

	resMap := make(map[string]bool)
	for _, r := range resolutions {
		resMap[r] = true
	}

	var cycleTimes []float64
	for _, issue := range issues {
		if issue.ResolutionDate == nil {
			continue
		}
		if len(resolutions) > 0 && !resMap[issue.Resolution] {
			continue
		}

		clockStart := issue.Created
		var earliestCommitment *time.Time

		for _, trans := range issue.Transitions {
			weight, ok := statusWeights[trans.ToStatus]
			if (startStatus != "" && trans.ToStatus == startStatus) || (ok && weight >= commitmentWeight) {
				if earliestCommitment == nil || trans.Date.Before(*earliestCommitment) {
					t := trans.Date
					earliestCommitment = &t
				}
			}
		}

		if earliestCommitment != nil {
			clockStart = *earliestCommitment
		} else if startStatus != "" {
			continue
		}

		ct := issue.ResolutionDate.Sub(clockStart).Hours() / 24.0
		if ct >= 0 {
			cycleTimes = append(cycleTimes, ct)
		}
	}
	return cycleTimes
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
