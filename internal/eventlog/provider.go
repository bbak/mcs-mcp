package eventlog

import (
	"fmt"
	"mcs-mcp/internal/jira"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// LogProvider orchestrates the progressive ingestion of events from Jira.
type LogProvider struct {
	jira  jira.Client
	store *EventStore
}

// NewLogProvider creates a new LogProvider.
func NewLogProvider(client jira.Client, store *EventStore) *LogProvider {
	return &LogProvider{
		jira:  client,
		store: store,
	}
}

// EnsureProbe (Stage 1) fetches the most recent items to build discovery metadata.
func (p *LogProvider) EnsureProbe(sourceID string, jql string) error {
	log.Info().Str("source", sourceID).Msg("Stage 1: Running Discovery Probe")

	// Fetch 200 most recently updated items
	probeJQL := fmt.Sprintf("(%s) ORDER BY updated DESC", jql)
	resp, err := p.jira.SearchIssuesWithHistory(probeJQL, 0, 200)
	if err != nil {
		return fmt.Errorf("probe failed: %w", err)
	}

	var events []IssueEvent
	for _, dto := range resp.Issues {
		events = append(events, TransformIssue(dto)...)
	}

	p.store.Append(sourceID, events)
	return nil
}

// EnsureWIP (Stage 2) ensures all currently active (logical WIP) items are in the log.
func (p *LogProvider) EnsureWIP(sourceID string, jql string, activeStatuses []string) error {
	log.Info().Str("source", sourceID).Msg("Stage 2: Completing WIP population")

	// Fetch all issues in active statuses
	statusJQL := ""
	if len(activeStatuses) > 0 {
		statusJQL = fmt.Sprintf("AND status in (%s)", formatJQLList(activeStatuses))
	} else {
		statusJQL = "AND resolution is EMPTY"
	}

	wipJQL := fmt.Sprintf("(%s) %s", jql, statusJQL)
	resp, err := p.jira.SearchIssuesWithHistory(wipJQL, 0, 500) // Assuming WIP < 500
	if err != nil {
		return fmt.Errorf("wip fetch failed: %w", err)
	}

	var events []IssueEvent
	for _, dto := range resp.Issues {
		events = append(events, TransformIssue(dto)...)
	}

	p.store.Append(sourceID, events)
	return nil
}

// EnsureBaseline (Stage 3) fetches historical resolution events for the baseline.
func (p *LogProvider) EnsureBaseline(sourceID string, jql string, months int) error {
	startTime := time.Now().AddDate(0, -months, 0)
	log.Info().Str("source", sourceID).Time("since", startTime).Msg("Stage 3: Fetching historical baseline")

	baselineJQL := fmt.Sprintf("(%s) AND (resolutiondate >= '%s' OR updated >= '%s')",
		jql, startTime.Format("2006-01-02"), startTime.Format("2006-01-02"))

	// In a real scenario, this would handle pagination properly
	resp, err := p.jira.SearchIssuesWithHistory(baselineJQL, 0, 1000)
	if err != nil {
		return fmt.Errorf("baseline fetch failed: %w", err)
	}

	var events []IssueEvent
	for _, dto := range resp.Issues {
		events = append(events, TransformIssue(dto)...)
	}

	p.store.Append(sourceID, events)
	return nil
}

// GetEventsInRange delegates to the underlying store.
func (p *LogProvider) GetEventsInRange(sourceID string, start, end time.Time) []IssueEvent {
	return p.store.GetEventsInRange(sourceID, start, end)
}

func formatJQLList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var escaped []string
	for _, item := range items {
		escaped = append(escaped, fmt.Sprintf("'%s'", item))
	}
	return strings.Join(escaped, ",")
}
