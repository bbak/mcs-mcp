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

		if req.Method == "initialize" {
			s.sendResponse(req.ID, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"serverInfo":      map[string]interface{}{"name": "mcs-mcp", "version": "0.1.0"},
			})
		} else if req.Method == "tools/list" {
			s.sendResponse(req.ID, s.listTools())
		} else if req.Method == "tools/call" {
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
				"description": "Probe a data source (board or filter) to analyze data quality and volume before running a simulation.",
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
				"description": "Run a Monte-Carlo simulation to forecast when a backlog will be finished based on historical throughput.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_id":    map[string]interface{}{"type": "string", "description": "ID of the board or filter"},
						"source_type":  map[string]interface{}{"type": "string", "enum": []string{"board", "filter"}},
						"backlog_size": map[string]interface{}{"type": "integer", "description": "Number of items remaining to be finished"},
						"resolutions":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional list of resolutions like ['Resolved']"},
					},
					"required": []string{"source_id", "source_type", "backlog_size"},
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

	// Probe: Fetch last 200 resolved items to analyze data quality
	probeJQL := fmt.Sprintf("(%s) AND resolution is not EMPTY ORDER BY resolutiondate DESC", jql)
	issues, total, err := s.jira.SearchIssues(probeJQL, 0, 200)
	if err != nil {
		return nil, err
	}

	summary := stats.AnalyzeProbe(issues, total)
	return summary, nil
}

func (s *Server) handleRunSimulation(sourceID, sourceType string, backlogSize int, resolutions []string) (interface{}, error) {
	// 1. Get JQL (Reuse logic from metadata)
	jql, err := s.getJQL(sourceID, sourceType)
	if err != nil {
		return nil, err
	}

	// 2. Adaptive Ingestion: Fetch sufficient history (Last 180 days for now)
	startTime := time.Now().AddDate(0, -6, 0)
	ingestJQL := fmt.Sprintf("(%s) AND resolutiondate >= '%s' ORDER BY resolutiondate ASC",
		jql, startTime.Format("2006-01-02"))

	log.Debug().Str("jql", ingestJQL).Msg("Starting historical ingestion for simulation")

	issues, _, err := s.jira.SearchIssues(ingestJQL, 0, 1000)
	if err != nil {
		return nil, err
	}

	if len(issues) == 0 {
		return nil, fmt.Errorf("no historical data found in the last 6 months to base simulation on")
	}

	// 3. Create Histogram
	endTime := time.Now()
	histogram := simulation.NewHistogram(issues, startTime, endTime, resolutions)

	// 4. Run Monte-Carlo
	engine := simulation.NewEngine(histogram)
	result := engine.Run(backlogSize, 10000)

	return result, nil
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
		data, err = s.jira.FindProjects(call.Arguments["query"].(string))
	case "find_jira_boards":
		pKey, _ := call.Arguments["project_key"].(string)
		nFilter, _ := call.Arguments["name_filter"].(string)
		data, err = s.jira.FindBoards(pKey, nFilter)
	case "get_project_details":
		key := call.Arguments["project_key"].(string)
		data, err = s.jira.GetProject(key)
	case "get_board_details":
		id := int(call.Arguments["board_id"].(float64))
		data, err = s.jira.GetBoard(id)
	case "get_data_metadata":
		id := call.Arguments["source_id"].(string)
		sType := call.Arguments["source_type"].(string)
		data, err = s.handleGetDataMetadata(id, sType)
	case "run_simulation":
		id := call.Arguments["source_id"].(string)
		sType := call.Arguments["source_type"].(string)
		backlog := int(call.Arguments["backlog_size"].(float64))
		var res []string
		if r, ok := call.Arguments["resolutions"].([]interface{}); ok {
			for _, v := range r {
				res = append(res, v.(string))
			}
		}
		data, err = s.handleRunSimulation(id, sType, backlog, res)
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
