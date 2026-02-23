package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"mcs-mcp/internal/config"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/stats/discovery"

	"github.com/rs/zerolog/log"
)

type JSONRPCRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type JSONRPCResponse struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

type Server struct {
	jira                  jira.Client
	events                *eventlog.LogProvider
	cacheDir              string
	activeSourceID        string
	activeMapping         map[string]stats.StatusMetadata
	activeResolutions     map[string]string
	activeStatusOrder     []string
	activeCommitmentPoint string
	activeDiscoveryCutoff *time.Time
	enableMermaidCharts   bool
}

func NewServer(cfg *config.AppConfig, jiraClient jira.Client) *Server {
	store := eventlog.NewEventStore()
	return &Server{
		jira:                jiraClient,
		events:              eventlog.NewLogProvider(jiraClient, store, cfg.CacheDir),
		cacheDir:            cfg.CacheDir,
		enableMermaidCharts: cfg.EnableMermaidCharts,
	}
}

func (s *Server) Start() {
	decoder := json.NewDecoder(os.Stdin)
	for {
		var req JSONRPCRequest
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				break // End of input
			}
			log.Error().Err(err).Msg("Failed to decode JSON-RPC request")
			continue
		}

		s.dispatch(req)
	}
}

func (s *Server) dispatch(req JSONRPCRequest) {
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			log.Error().
				Interface("panic", r).
				Str("stack", stack).
				Msg("Panic recovered in dispatch")
			s.sendError(req.ID, map[string]interface{}{
				"code":    -32603,
				"message": fmt.Sprintf("Internal error: %v", r),
			})
		}
	}()

	log.Info().
		Str("method", req.Method).
		Interface("id", req.ID).
		Msg("Received JSON-RPC request")

	// Handle notifications (no ID)
	if req.ID == nil {
		switch req.Method {
		case "notifications/initialized":
			log.Info().Msg("Client confirmed initialization")
		default:
			log.Debug().Str("method", req.Method).Msg("Received notification")
		}
		return
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
	default:
		s.sendError(req.ID, map[string]interface{}{
			"code":    -32601,
			"message": fmt.Sprintf("Method not found: %s", req.Method),
		})
	}
}

func (s *Server) sendResponse(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		Jsonrpc: "2.0",
		ID:      id,
		Result:  result,
	}
	out, _ := json.Marshal(resp)
	fmt.Println(string(out))
}

func (s *Server) sendError(id interface{}, err interface{}) {
	resp := JSONRPCResponse{
		Jsonrpc: "2.0",
		ID:      id,
		Error:   err,
	}
	out, _ := json.Marshal(resp)
	fmt.Println(string(out))
}

func (s *Server) callTool(params json.RawMessage) (res interface{}, errRes interface{}) {
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			log.Error().
				Interface("panic", r).
				Str("stack", stack).
				Msg("Panic recovered in callTool")
			errRes = map[string]interface{}{
				"code":    -32603,
				"message": fmt.Sprintf("Internal error: %v\n\nStack trace:\n%s", r, stack),
			}
		}
	}()

	var call struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, map[string]interface{}{"code": -32602, "message": "Invalid params"}
	}

	log.Info().Str("tool", call.Name).Msg("Processing tool call")
	log.Debug().Interface("arguments", call.Arguments).Msg("Tool arguments")

	var data interface{}
	var err error

	switch call.Name {
	case "import_projects":
		query := asString(call.Arguments["query"])
		var projects []interface{}
		var findErr error

		// Intercept MCSTEST (case-insensitive) or broad searches
		isMockQuery := strings.Contains(strings.ToUpper(query), "MCSTEST") || query == ""
		if isMockQuery {
			projects = append(projects, map[string]interface{}{
				"key":  "MCSTEST",
				"name": "Mock Test Project (Synthetic)",
			})
		}

		// Only call Jira if the query isn't JUST "MCSTEST"
		if strings.ToUpper(query) != "MCSTEST" {
			jiraProjects, jErr := s.jira.FindProjects(query)
			if jErr == nil {
				for _, p := range jiraProjects {
					projects = append(projects, p)
				}
			} else if !isMockQuery {
				findErr = jErr
			}
		}

		if findErr == nil {
			data = map[string]interface{}{
				"projects": projects,
				"_guidance": []string{
					"Project located. If you plan to run analytical diagnostics (Aging, Simulations, Stability), you MUST find the project's boards using 'import_boards' next.",
				},
			}
		}
		err = findErr
	case "import_boards":
		pKey := asString(call.Arguments["project_key"])
		nFilter := asString(call.Arguments["name_filter"])
		var boards []interface{}
		var findErr error

		isMockKey := strings.ToUpper(pKey) == "MCSTEST" || pKey == ""
		if isMockKey {
			boards = append(boards, map[string]interface{}{
				"id":   0,
				"name": "Mock Test Board 0 (Synthetic)",
				"type": "kanban",
			})
		}

		if strings.ToUpper(pKey) != "MCSTEST" {
			jiraBoards, jErr := s.jira.FindBoards(pKey, nFilter)
			if jErr == nil {
				for _, b := range jiraBoards {
					boards = append(boards, b)
				}
			} else if !isMockKey {
				findErr = jErr
			}
		}
		if findErr == nil {
			data = map[string]interface{}{
				"boards": boards,
				"_guidance": []string{
					"Board located. You MUST now call 'import_board_context' to anchor on the data distribution and metadata before performing workflow discovery.",
				},
			}
		}
		err = findErr
	case "import_project_context":
		key := asString(call.Arguments["project_key"])
		data, err = s.handleGetProjectDetails(key)
	case "import_board_context":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		data, err = s.handleGetBoardDetails(projectKey, boardID)
	case "forecast_monte_carlo":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		mode := asString(call.Arguments["mode"])
		startStatus := asString(call.Arguments["start_status"])
		endStatus := asString(call.Arguments["end_status"])

		includeExisting := false
		if b, ok := call.Arguments["include_existing_backlog"].(bool); ok {
			includeExisting = b
		}

		additional := asInt(call.Arguments["additional_items"])

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

		sampleDays := asInt(call.Arguments["history_window_days"])
		sampleStart := asString(call.Arguments["history_start_date"])
		sampleEnd := asString(call.Arguments["history_end_date"])

		var targets map[string]int
		if t, ok := call.Arguments["targets"].(map[string]interface{}); ok {
			targets = make(map[string]int)
			for k, v := range t {
				targets[k] = asInt(v)
			}
		}

		var mixOverrides map[string]float64
		if m, ok := call.Arguments["mix_overrides"].(map[string]interface{}); ok {
			mixOverrides = make(map[string]float64)
			for k, v := range m {
				if f, ok := v.(float64); ok {
					mixOverrides[k] = f
				} else if i, ok := v.(int); ok {
					mixOverrides[k] = float64(i)
				}
			}
		}

		data, err = s.handleRunSimulation(projectKey, boardID, mode, includeExisting, additional, targetDays, targetDate, startStatus, endStatus, issueTypes, includeWIP, sampleDays, sampleStart, sampleEnd, targets, mixOverrides)
	case "analyze_cycle_time":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		issueTypes := []string{}
		if types, ok := call.Arguments["issue_types"].([]interface{}); ok {
			for _, t := range types {
				issueTypes = append(issueTypes, asString(t))
			}
		}
		wipStability, _ := call.Arguments["analyze_wip_stability"].(bool)
		startStatus := asString(call.Arguments["start_status"])
		endStatus := asString(call.Arguments["end_status"])
		data, err = s.handleGetCycleTimeAssessment(projectKey, boardID, wipStability, startStatus, endStatus, issueTypes)
	case "analyze_status_persistence":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		data, err = s.handleGetStatusPersistence(projectKey, boardID)
	case "analyze_work_item_age":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		agingType := asString(call.Arguments["age_type"])
		tierFilter := asString(call.Arguments["tier_filter"])
		data, err = s.handleGetAgingAnalysis(projectKey, boardID, agingType, tierFilter)
	case "analyze_throughput":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		weeks := asInt(call.Arguments["history_window_weeks"])
		includeAbandoned, _ := call.Arguments["include_abandoned"].(bool)
		bucket := asString(call.Arguments["bucket"])
		if bucket == "" {
			bucket = "week"
		}
		data, err = s.handleGetDeliveryCadence(projectKey, boardID, weeks, bucket, includeAbandoned)
	case "analyze_process_stability":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		window := asInt(call.Arguments["history_window_weeks"])
		if window == 0 {
			window = 26
		}
		data, err = s.handleGetProcessStability(projectKey, boardID)
	case "analyze_process_evolution":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		window := asInt(call.Arguments["history_window_months"])
		if window == 0 {
			window = 12
		}
		data, err = s.handleGetProcessEvolution(projectKey, boardID, window)
	case "analyze_yield":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		data, err = s.handleGetProcessYield(projectKey, boardID)
	case "workflow_discover_mapping":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		force, _ := call.Arguments["force_refresh"].(bool)
		data, err = s.handleGetWorkflowDiscovery(projectKey, boardID, force)
	case "workflow_set_mapping":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		mapping, _ := call.Arguments["mapping"].(map[string]interface{})
		resolutions, _ := call.Arguments["resolutions"].(map[string]interface{})
		commitmentPoint := asString(call.Arguments["commitment_point"])
		data, err = s.handleSetWorkflowMapping(projectKey, boardID, mapping, resolutions, commitmentPoint)
	case "workflow_set_order":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		order := []string{}
		if o, ok := call.Arguments["order"].([]interface{}); ok {
			for _, v := range o {
				order = append(order, asString(v))
			}
		}
		data, err = s.handleSetWorkflowOrder(projectKey, boardID, order)
	case "guide_diagnostic_roadmap":
		goal := asString(call.Arguments["goal"])
		data, err = s.handleGetDiagnosticRoadmap(goal)
	case "analyze_item_journey":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		issueKey := asString(call.Arguments["issue_key"])
		data, err = s.handleGetItemJourney(projectKey, boardID, issueKey)
	case "forecast_backtest":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		mode := asString(call.Arguments["simulation_mode"])
		items := asInt(call.Arguments["items_to_forecast"])
		horizon := asInt(call.Arguments["forecast_horizon_days"])

		var issueTypes []string
		if it, ok := call.Arguments["issue_types"].([]interface{}); ok {
			for _, v := range it {
				issueTypes = append(issueTypes, asString(v))
			}
		}

		sampleDays := asInt(call.Arguments["history_window_days"])
		sampleStart := asString(call.Arguments["history_start_date"])
		sampleEnd := asString(call.Arguments["history_end_date"])

		data, err = s.handleGetForecastAccuracy(projectKey, boardID, mode, items, horizon, issueTypes, sampleDays, sampleStart, sampleEnd)
	case "import_history_expand":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		chunks := asInt(call.Arguments["chunks"])
		data, err = s.handleCacheExpandHistory(projectKey, boardID, chunks)
	case "import_history_update":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		data, err = s.handleCacheCatchUp(projectKey, boardID)
	default:
		return nil, map[string]interface{}{"code": -32601, "message": "Tool not found"}
	}

	if err != nil {
		log.Error().Err(err).Str("tool", call.Name).Msg("Tool call failed")
		return nil, map[string]interface{}{"code": -32000, "message": err.Error()}
	}

	log.Info().Str("tool", call.Name).Msg("Tool call completed successfully")

	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": s.formatResult(data),
			},
		},
	}, nil
}

type WorkflowMetadata struct {
	SourceID        string                          `json:"source_id"`
	Mapping         map[string]stats.StatusMetadata `json:"mapping"`
	Resolutions     map[string]string               `json:"resolutions,omitempty"`
	StatusOrder     []string                        `json:"status_order,omitempty"`
	CommitmentPoint string                          `json:"commitment_point,omitempty"`
	DiscoveryCutoff *time.Time                      `json:"discovery_cutoff,omitempty"`
}

func (s *Server) saveWorkflow(projectKey string, boardID int) error {
	sourceID := getCombinedID(projectKey, boardID)
	meta := WorkflowMetadata{
		SourceID:        sourceID,
		Mapping:         s.activeMapping,
		Resolutions:     s.activeResolutions,
		StatusOrder:     s.activeStatusOrder,
		CommitmentPoint: s.activeCommitmentPoint,
		DiscoveryCutoff: s.activeDiscoveryCutoff,
	}

	path := filepath.Join(s.cacheDir, fmt.Sprintf("%s_%d_workflow.json", projectKey, boardID))
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(meta)
}

func (s *Server) loadWorkflow(projectKey string, boardID int) (bool, error) {
	path := filepath.Join(s.cacheDir, fmt.Sprintf("%s_%d_workflow.json", projectKey, boardID))
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()

	var meta WorkflowMetadata
	if err := json.NewDecoder(file).Decode(&meta); err != nil {
		return false, err
	}

	s.activeStatusOrder = meta.StatusOrder
	s.activeCommitmentPoint = meta.CommitmentPoint
	s.activeDiscoveryCutoff = meta.DiscoveryCutoff
	s.activeMapping = meta.Mapping
	s.activeResolutions = meta.Resolutions

	log.Info().Str("path", path).Msg("Loaded workflow metadata from disk")
	return true, nil
}

func (s *Server) anchorContext(projectKey string, boardID int) error {
	sourceID := getCombinedID(projectKey, boardID)

	// If already anchored, nothing to do
	if s.activeSourceID == sourceID {
		return nil
	}

	log.Info().Str("prev", s.activeSourceID).Str("next", sourceID).Msg("Switching active context")

	// 1. Clear old active state
	s.activeSourceID = ""
	s.activeMapping = nil
	s.activeResolutions = nil
	s.activeStatusOrder = nil
	s.activeCommitmentPoint = ""

	// 2. Prune EventStore RAM
	s.events.PruneExcept(sourceID)

	// 3. Attempt to load metadata from disk
	found, err := s.loadWorkflow(projectKey, boardID)
	if err != nil {
		log.Warn().Err(err).Str("source", sourceID).Msg("Failed to load workflow metadata from disk")
		// Continue anyway; we'll re-discover it
	}

	if found {
		log.Info().Str("source", sourceID).Msg("Context anchored with existing metadata")
	} else {
		log.Info().Str("source", sourceID).Msg("Context anchored (new, no metadata found)")
	}

	s.activeSourceID = sourceID
	return nil
}

func (s *Server) recalculateDiscoveryCutoff(sourceID string) {
	if s.activeMapping == nil {
		return
	}

	window := stats.NewAnalysisWindow(time.Time{}, time.Now(), "day", time.Time{})
	events := s.events.GetEventsInRange(sourceID, window.Start, window.End)
	domainIssues, _, _, _ := stats.ProjectScope(events, window, s.activeCommitmentPoint, s.activeMapping, s.activeResolutions, nil)

	finishedMap := make(map[string]bool)
	for name, meta := range s.activeMapping {
		if meta.Tier == "Finished" {
			finishedMap[name] = true
		}
	}
	s.activeDiscoveryCutoff = discovery.CalculateDiscoveryCutoff(domainIssues, finishedMap)
}
