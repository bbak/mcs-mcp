package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"mcs-mcp/internal/config"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"

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
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

type Server struct {
	jira               jira.Client
	events             *eventlog.LogProvider
	workflowMappings   map[string]map[string]stats.StatusMetadata // sourceID -> statusName -> metadata
	resolutionMappings map[string]map[string]string               // sourceID -> resolutionName -> outcome
	statusOrderings    map[string][]string                        // sourceID -> sorted status names
}

func NewServer(cfg *config.AppConfig, jiraClient jira.Client) *Server {
	store := eventlog.NewEventStore()
	return &Server{
		jira:               jiraClient,
		events:             eventlog.NewLogProvider(jiraClient, store, cfg.CacheDir),
		workflowMappings:   make(map[string]map[string]stats.StatusMetadata),
		resolutionMappings: make(map[string]map[string]string),
		statusOrderings:    make(map[string][]string),
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

		log.Info().
			Str("method", req.Method).
			Interface("id", req.ID).
			Msg("Received JSON-RPC request")

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
	case "find_jira_projects":
		projects, findErr := s.jira.FindProjects(asString(call.Arguments["query"]))
		if findErr == nil {
			data = map[string]interface{}{
				"projects": projects,
				"_guidance": []string{
					"Project located. You MUST now call 'get_project_details' to anchor on the data distribution before planning analysis.",
				},
			}
		}
		err = findErr
	case "find_jira_boards":
		pKey := asString(call.Arguments["project_key"])
		nFilter := asString(call.Arguments["name_filter"])
		boards, findErr := s.jira.FindBoards(pKey, nFilter)
		if findErr == nil {
			data = map[string]interface{}{
				"boards": boards,
				"_guidance": []string{
					"Board located. You MUST now call 'get_board_details' to anchor on the data distribution and metadata before planning analysis.",
				},
			}
		}
		err = findErr
	case "get_project_details":
		key := asString(call.Arguments["project_key"])
		data, err = s.handleGetProjectDetails(key)
	case "get_board_details":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		data, err = s.handleGetBoardDetails(projectKey, boardID)
	case "run_simulation":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		mode := asString(call.Arguments["mode"])
		startStatus := asString(call.Arguments["start_status"])

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

		var res []string
		if r, ok := call.Arguments["resolutions"].([]interface{}); ok {
			for _, v := range r {
				res = append(res, asString(v))
			}
		}
		if len(res) == 0 {
			res = s.getDeliveredResolutions(projectKey, boardID)
		}
		sampleDays := asInt(call.Arguments["sample_days"])
		sampleStart := asString(call.Arguments["sample_start_date"])
		sampleEnd := asString(call.Arguments["sample_end_date"])

		var targets map[string]int
		if t, ok := call.Arguments["targets"].(map[string]interface{}); ok {
			targets = make(map[string]int)
			for k, v := range t {
				targets[k] = asInt(v)
			}
		}

		var mix map[string]float64
		if m, ok := call.Arguments["mix_overrides"].(map[string]interface{}); ok {
			mix = make(map[string]float64)
			for k, v := range m {
				if f, ok := v.(float64); ok {
					mix[k] = f
				} else if i, ok := v.(int); ok {
					mix[k] = float64(i)
				}
			}
		}

		data, err = s.handleRunSimulation(projectKey, boardID, mode, includeExisting, additional, targetDays, targetDate, startStatus, asString(call.Arguments["end_status"]), issueTypes, includeWIP, res, sampleDays, sampleStart, sampleEnd, targets, mix)
	case "get_status_persistence":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		data, err = s.handleGetStatusPersistence(projectKey, boardID)
	case "get_aging_analysis":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		agingType := asString(call.Arguments["aging_type"])
		tierFilter := asString(call.Arguments["tier_filter"])
		data, err = s.handleGetAgingAnalysis(projectKey, boardID, agingType, tierFilter)
	case "get_delivery_cadence":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		data, err = s.handleGetDeliveryCadence(projectKey, boardID)
	case "get_process_stability":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		window := asInt(call.Arguments["window_weeks"])
		if window == 0 {
			window = 26
		}
		data, err = s.handleGetProcessStability(projectKey, boardID)
	case "get_process_evolution":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		window := asInt(call.Arguments["window_months"])
		if window == 0 {
			window = 12
		}
		data, err = s.handleGetProcessEvolution(projectKey, boardID, window)
	case "get_process_yield":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		data, err = s.handleGetProcessYield(projectKey, boardID)
	case "get_workflow_discovery":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		data, err = s.handleGetWorkflowDiscovery(projectKey, boardID)
	case "set_workflow_mapping":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		mapping, _ := call.Arguments["mapping"].(map[string]interface{})
		resolutions, _ := call.Arguments["resolutions"].(map[string]interface{})
		data, err = s.handleSetWorkflowMapping(projectKey, boardID, mapping, resolutions)
	case "set_workflow_order":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		order := []string{}
		if o, ok := call.Arguments["order"].([]interface{}); ok {
			for _, v := range o {
				order = append(order, asString(v))
			}
		}
		data, err = s.handleSetWorkflowOrder(projectKey, boardID, order)
	case "get_cycle_time_assessment":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		startStatus := asString(call.Arguments["start_status"])
		endStatus := asString(call.Arguments["end_status"])

		var issueTypes []string
		if it, ok := call.Arguments["issue_types"].([]interface{}); ok {
			for _, v := range it {
				issueTypes = append(issueTypes, asString(v))
			}
		}

		var analyzeWIP bool
		if w, ok := call.Arguments["analyze_wip_stability"].(bool); ok {
			analyzeWIP = w
		}

		var res []string
		if r, ok := call.Arguments["resolutions"].([]interface{}); ok {
			for _, v := range r {
				res = append(res, asString(v))
			}
		}
		data, err = s.handleGetCycleTimeAssessment(projectKey, boardID, analyzeWIP, startStatus, endStatus, res)
	case "get_diagnostic_roadmap":
		goal := asString(call.Arguments["goal"])
		data, err = s.handleGetDiagnosticRoadmap(goal)
	case "get_item_journey":
		key := asString(call.Arguments["issue_key"])
		data, err = s.handleGetItemJourney(key)
	case "get_forecast_accuracy":
		projectKey := asString(call.Arguments["project_key"])
		boardID := asInt(call.Arguments["board_id"])
		mode := asString(call.Arguments["simulation_mode"])
		items := asInt(call.Arguments["items_to_forecast"])
		horizon := asInt(call.Arguments["forecast_horizon_days"])

		var res []string
		if r, ok := call.Arguments["resolutions"].([]interface{}); ok {
			for _, v := range r {
				res = append(res, asString(v))
			}
		}
		sampleDays := asInt(call.Arguments["sample_days"])
		sampleStart := asString(call.Arguments["sample_start_date"])
		sampleEnd := asString(call.Arguments["sample_end_date"])

		data, err = s.handleGetForecastAccuracy(projectKey, boardID, mode, items, horizon, res, sampleDays, sampleStart, sampleEnd)
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
