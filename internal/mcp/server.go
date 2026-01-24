package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"

	"github.com/rs/zerolog/log"
)

// JSONRPCRequest represents a standard MCP/JSON-RPC request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a standard MCP/JSON-RPC response.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// Server holds the state for the MCP server.
type Server struct {
	jira jira.Client
}

// NewServer creates a new MCP server.
func NewServer(jira jira.Client) *Server {
	return &Server{jira: jira}
}

// Serve starts the JSON-RPC loop over Stdio.
func (s *Server) Serve() error {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal request")
			continue
		}

		s.handleRequest(req)
	}
}

func (s *Server) handleRequest(req JSONRPCRequest) {
	var result interface{}
	var errRes interface{}

	switch req.Method {
	case "initialize":
		result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"serverInfo": map[string]interface{}{
				"name":    "mcs-mcp",
				"version": "0.1.0",
			},
		}
	case "tools/list":
		result = s.listTools()
	case "tools/call":
		result, errRes = s.callTool(req.Params)
	default:
		errRes = map[string]interface{}{
			"code":    -32601,
			"message": fmt.Sprintf("Method %s not found", req.Method),
		}
	}

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   errRes,
	}

	out, _ := json.Marshal(resp)
	fmt.Fprintf(os.Stdout, "%s\n", out)
}

func (s *Server) listTools() interface{} {
	return map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{
				"name":        "find_jira_projects",
				"description": "Search for Jira projects by name or key (at least 3 characters).",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{"type": "string"},
					},
					"required": []string{"query"},
				},
			},
			map[string]interface{}{
				"name":        "find_jira_boards",
				"description": "Search for Jira boards, optionally filtered by project key.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"project_key": map[string]interface{}{"type": "string"},
						"name_filter": map[string]interface{}{"type": "string"},
					},
				},
			},
			map[string]interface{}{
				"name":        "get_project_details",
				"description": "Get details for a specific Jira project by its key.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"project_key": map[string]interface{}{"type": "string"},
					},
					"required": []string{"project_key"},
				},
			},
			map[string]interface{}{
				"name":        "get_board_details",
				"description": "Get details for a specific Jira board by its ID.",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"board_id": map[string]interface{}{"type": "integer"},
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
		},
	}
}

func (s *Server) handleGetDataMetadata(sourceID, sourceType string) (interface{}, error) {
	var jql string

	if sourceType == "board" {
		var id int
		_, err := fmt.Sscanf(sourceID, "%d", &id)
		if err != nil {
			return nil, fmt.Errorf("invalid board ID: %s", sourceID)
		}
		// Fetch board configuration to get the filter ID
		config, err := s.jira.GetBoardConfig(id)
		if err != nil {
			return nil, fmt.Errorf("failed to get board configuration for %d: %w", id, err)
		}

		cMap := config.(map[string]interface{})
		filterObj, ok := cMap["filter"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("board configuration for %d does not contain filter info", id)
		}

		filterID := fmt.Sprintf("%v", filterObj["id"])
		filter, err := s.jira.GetFilter(filterID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch filter %s: %w", filterID, err)
		}
		jql = filter.(map[string]interface{})["jql"].(string)
	} else {
		filter, err := s.jira.GetFilter(sourceID)
		if err != nil {
			return nil, err
		}
		jql = filter.(map[string]interface{})["jql"].(string)
	}

	// Strip existing ORDER BY to avoid collision
	jqlLower := strings.ToLower(jql)
	if idx := strings.Index(jqlLower, " order by"); idx != -1 {
		jql = jql[:idx]
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
		pk, _ := call.Arguments["project_key"].(string)
		nf, _ := call.Arguments["name_filter"].(string)
		data, err = s.jira.FindBoards(pk, nf)
	case "get_project_details":
		data, err = s.jira.GetProject(call.Arguments["project_key"].(string))
	case "get_board_details":
		id := int(call.Arguments["board_id"].(float64))
		data, err = s.jira.GetBoard(id)
	case "get_data_metadata":
		id := call.Arguments["source_id"].(string)
		sType := call.Arguments["source_type"].(string)
		data, err = s.handleGetDataMetadata(id, sType)
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
