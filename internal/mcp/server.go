package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mcs-mcp/internal/config"
	"mcs-mcp/internal/eventlog"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/simulation"
	"mcs-mcp/internal/stats"
	"mcs-mcp/internal/stats/discovery"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

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
	activeEvaluationDate  *time.Time
	activeRegistry        *jira.NameRegistry
	enableMermaidCharts     bool
	commitmentBackflowReset bool
	simulationSeed          int64 // 0 = random (production); non-zero = fixed seed (tests)
	engineRegistry          *simulation.Registry
	engineName              string         // from MCS_ENGINE: "crude", "bbak", "auto"
	engineWeights           map[string]int // from MCS_ENGINE_<NAME>
}

func (s *Server) Clock() time.Time {
	if s.activeEvaluationDate != nil {
		return *s.activeEvaluationDate
	}
	return time.Now()
}

func NewServer(cfg *config.AppConfig, jiraClient jira.Client) *Server {
	reg := simulation.NewRegistry()
	reg.Register(&simulation.CrudeEngine{})
	reg.Register(&simulation.BbakEngine{})

	engineName := cfg.Engine
	if engineName == "" {
		engineName = "crude"
	}
	engineWeights := cfg.EngineWeights
	if engineWeights == nil {
		engineWeights = map[string]int{"crude": 50, "bbak": 50}
	}

	s := &Server{
		jira:                    jiraClient,
		cacheDir:                cfg.CacheDir,
		enableMermaidCharts:     cfg.EnableMermaidCharts,
		commitmentBackflowReset: cfg.CommitmentBackflowReset,
		engineRegistry:          reg,
		engineName:              engineName,
		engineWeights:           engineWeights,
	}

	store := eventlog.NewEventStore(s.Clock)
	s.events = eventlog.NewLogProvider(jiraClient, store, cfg.CacheDir)

	return s
}

// NewMCPServer creates an SDK MCP server wired to all tools on the given app server.
func NewMCPServer(s *Server, version string) (*mcp.Server, error) {
	mcpSrv := mcp.NewServer(&mcp.Implementation{
		Name:    "mcs-mcp",
		Version: version,
	}, nil)
	if err := registerTools(mcpSrv, s); err != nil {
		return nil, fmt.Errorf("register tools: %w", err)
	}
	return mcpSrv, nil
}

// Run starts the MCP server on the stdio transport.
func (s *Server) Run(ctx context.Context, version string) error {
	mcpSrv, err := NewMCPServer(s, version)
	if err != nil {
		return err
	}
	return mcpSrv.Run(ctx, &mcp.StdioTransport{})
}

// handleImportProjects searches for Jira projects, injecting mock data for MCSTEST.
func (s *Server) handleImportProjects(query string) (any, error) {
	var projects []any

	isMockQuery := strings.Contains(strings.ToUpper(query), "MCSTEST") || query == ""
	if isMockQuery {
		projects = append(projects, map[string]any{
			"key":  "MCSTEST",
			"name": "Mock Test Project (Synthetic)",
		})
	}

	if strings.ToUpper(query) != "MCSTEST" {
		jiraProjects, err := s.jira.FindProjects(query)
		if err == nil {
			projects = append(projects, jiraProjects...)
		} else if !isMockQuery {
			return nil, err
		}
	}

	return map[string]any{
		"projects": projects,
		"_guidance": []string{
			"Project located. If you plan to run analytical diagnostics (Aging, Simulations, Stability), you MUST find the project's boards using 'import_boards' next.",
		},
	}, nil
}

// handleImportBoards searches for Jira boards, injecting mock data for MCSTEST.
func (s *Server) handleImportBoards(projectKey, nameFilter string) (any, error) {
	var boards []any

	isMockKey := strings.ToUpper(projectKey) == "MCSTEST" || projectKey == ""
	if isMockKey {
		boards = append(boards, map[string]any{
			"id":   0,
			"name": "Mock Test Board 0 (Synthetic)",
			"type": "kanban",
		})
	}

	if strings.ToUpper(projectKey) != "MCSTEST" {
		jiraBoards, err := s.jira.FindBoards(projectKey, nameFilter)
		if err == nil {
			boards = append(boards, jiraBoards...)
		} else if !isMockKey {
			return nil, err
		}
	}

	return map[string]any{
		"boards": boards,
		"_guidance": []string{
			"Board located. You MUST now call 'import_board_context' to anchor on the data distribution and metadata before performing workflow discovery.",
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
	EvaluationDate  *time.Time                      `json:"evaluation_date,omitempty"`
	NameRegistry    *jira.NameRegistry              `json:"name_registry,omitempty"`
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
		EvaluationDate:  s.activeEvaluationDate,
		NameRegistry:    s.activeRegistry,
	}

	path := filepath.Join(s.cacheDir, fmt.Sprintf("%s_%d_workflow.json", projectKey, boardID))
	tmp := path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(meta); err != nil {
		file.Close()
		os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
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
	s.activeEvaluationDate = meta.EvaluationDate
	s.activeRegistry = meta.NameRegistry

	// Migration: Resolve StatusOrder names to IDs for internal stability
	var resolvedOrder []string
	for _, entry := range s.activeStatusOrder {
		id := s.activeRegistry.GetStatusID(entry)
		if id != "" {
			resolvedOrder = append(resolvedOrder, id)
		} else {
			resolvedOrder = append(resolvedOrder, entry) // Already an ID or unknown
		}
	}
	s.activeStatusOrder = resolvedOrder

	// Migration: Resolve CommitmentPoint name to ID
	if cp := s.activeRegistry.GetStatusID(s.activeCommitmentPoint); cp != "" {
		s.activeCommitmentPoint = cp
	}

	// Migration: If mappings/resolutions are name-based, try to convert them to IDs
	// for internal stability (Analytical Guardrail).
	s.activeMapping = make(map[string]stats.StatusMetadata)
	for k, m := range meta.Mapping {
		id := s.activeRegistry.GetStatusID(k)
		if id != "" {
			// k was a human-readable name; store it and re-key by ID
			m.Name = k
			s.activeMapping[id] = m
		} else if name := s.activeRegistry.GetStatusName(k); name != "" {
			// k was already an ID; heal any corrupted Name field
			m.Name = name
			s.activeMapping[k] = m
		} else {
			// Unknown key — keep as-is
			s.activeMapping[k] = m
		}
	}

	s.activeResolutions = make(map[string]string)
	for k, outcome := range meta.Resolutions {
		id := s.activeRegistry.GetResolutionID(k)
		if id != "" {
			s.activeResolutions[id] = outcome
		} else {
			s.activeResolutions[k] = outcome
		}
	}

	log.Info().Str("path", path).Msg("Loaded and harmonized workflow metadata from disk")
	return len(s.activeMapping) > 0, nil
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
	s.activeEvaluationDate = nil
	s.activeRegistry = nil

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

	window := stats.NewAnalysisWindow(time.Time{}, s.Clock(), "day", time.Time{})
	events := s.events.GetIssuesInRange(sourceID, window.Start, window.End)
	domainIssues, _, _, _ := stats.ProjectScope(events, window, s.activeCommitmentPoint, s.activeMapping, s.activeResolutions, nil)

	finishedMap := make(map[string]bool)
	for name, meta := range s.activeMapping {
		if meta.Tier == "Finished" {
			finishedMap[name] = true
		}
	}
	s.activeDiscoveryCutoff = discovery.CalculateDiscoveryCutoff(domainIssues, finishedMap)
}
