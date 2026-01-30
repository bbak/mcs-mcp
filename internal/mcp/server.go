package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

type Server struct {
	jira               jira.Client
	workflowMappings   map[string]map[string]stats.StatusMetadata // sourceID -> statusName -> metadata
	resolutionMappings map[string]map[string]string               // sourceID -> resolutionName -> outcome
	statusOrderings    map[string][]string                        // sourceID -> sorted status names
}

func NewServer(jiraClient jira.Client) *Server {
	return &Server{
		jira:               jiraClient,
		workflowMappings:   make(map[string]map[string]stats.StatusMetadata),
		resolutionMappings: make(map[string]map[string]string),
		statusOrderings:    make(map[string][]string),
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

func (s *Server) callTool(params json.RawMessage) (res interface{}, errRes interface{}) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("Panic recovered in callTool")
			errRes = map[string]interface{}{"code": -32603, "message": fmt.Sprintf("Internal error: %v", r)}
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
					"Before performing analysis on a discovered project, you SHOULD fetch its full details via 'get_project_details' to ensure valid metadata and context.",
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
					"Once a board is identified, you SHOULD fetch its configuration using 'get_board_details' to understand its mapping to projects and statuses.",
				},
			}
		}
		err = findErr
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
			res = s.getDeliveredResolutions(id)
		}
		data, err = s.handleRunSimulation(id, sType, mode, includeExisting, additional, targetDays, targetDate, startStatus, asString(call.Arguments["end_status"]), issueTypes, includeWIP, res)
	case "get_status_persistence":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		data, err = s.handleGetStatusPersistence(id, sType)
	case "get_aging_analysis":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		agingType := asString(call.Arguments["aging_type"])
		tierFilter := asString(call.Arguments["tier_filter"])
		data, err = s.handleGetAgingAnalysis(id, sType, agingType, tierFilter)
	case "get_delivery_cadence":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		window := asInt(call.Arguments["window_weeks"])
		if window == 0 {
			window = 26
		}
		data, err = s.handleGetDeliveryCadence(id, sType)
	case "get_process_stability":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		window := asInt(call.Arguments["window_weeks"])
		if window == 0 {
			window = 26
		}
		data, err = s.handleGetProcessStability(id, sType)
	case "get_process_evolution":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
		window := asInt(call.Arguments["window_months"])
		if window == 0 {
			window = 12
		}
		data, err = s.handleGetProcessEvolution(id, sType, window)
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
		resolutions, _ := call.Arguments["resolutions"].(map[string]interface{})
		data, err = s.handleSetWorkflowMapping(id, mapping, resolutions)
	case "set_workflow_order":
		id := asString(call.Arguments["source_id"])
		order := []string{}
		if o, ok := call.Arguments["order"].([]interface{}); ok {
			for _, v := range o {
				order = append(order, asString(v))
			}
		}
		data, err = s.handleSetWorkflowOrder(id, order)
	case "get_cycle_time_assessment":
		id := asString(call.Arguments["source_id"])
		sType := asString(call.Arguments["source_type"])
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
		data, err = s.handleGetCycleTimeAssessment(id, sType, analyzeWIP, startStatus, endStatus, res)
	case "get_diagnostic_roadmap":
		goal := asString(call.Arguments["goal"])
		data, err = s.handleGetDiagnosticRoadmap(goal)
	case "get_item_journey":
		key := asString(call.Arguments["issue_key"])
		data, err = s.handleGetItemJourney(key)
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
