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
	jira     jira.Client
	store    *EventStore
	cacheDir string
}

// NewLogProvider creates a new LogProvider.
func NewLogProvider(client jira.Client, store *EventStore, cacheDir string) *LogProvider {
	return &LogProvider{
		jira:     client,
		store:    store,
		cacheDir: cacheDir,
	}
}

// EnsureProbe (Stage 1) fetches the most recent items to build discovery metadata.
func (p *LogProvider) EnsureProbe(sourceID string, jql string) error {
	// 1. Try to load from cache first
	if err := p.store.Load(p.cacheDir, sourceID); err != nil {
		log.Warn().Err(err).Str("source", sourceID).Msg("Failed to load cache, proceeding with full probe")
	}

	// 2. Try incremental sync if we have a recent timestamp
	synced, err := p.tryIncrementalSync(sourceID, jql)
	if err != nil {
		log.Warn().Err(err).Str("source", sourceID).Msg("Incremental sync failed, falling back to standard probe")
	}

	if synced {
		return nil
	}

	log.Info().Str("source", sourceID).Msg("Stage 1: Running Discovery Probe")

	// Fetch 200 most recently updated items
	probeJQL := fmt.Sprintf("(%s) ORDER BY updated DESC", jql)
	events, err := p.fetchAll(probeJQL, 200)
	if err != nil {
		return fmt.Errorf("probe failed: %w", err)
	}

	p.store.Append(sourceID, events)
	return p.store.Save(p.cacheDir, sourceID)
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
	events, err := p.fetchAll(wipJQL, 0) // Fetch all WIP
	if err != nil {
		return fmt.Errorf("wip fetch failed: %w", err)
	}

	p.store.Append(sourceID, events)
	return p.store.Save(p.cacheDir, sourceID)
}

// EnsureBaseline (Stage 3) fetches historical resolution events for the baseline.
func (p *LogProvider) EnsureBaseline(sourceID string, jql string, months int) error {
	startTime := time.Now().AddDate(0, -months, 0)
	log.Info().Str("source", sourceID).Time("since", startTime).Msg("Stage 3: Fetching historical baseline")

	baselineJQL := fmt.Sprintf("(%s) AND (resolutiondate >= '%s' OR updated >= '%s')",
		jql, startTime.Format("2006-01-02"), startTime.Format("2006-01-02"))

	events, err := p.fetchAll(baselineJQL, 0) // Fetch all historical
	if err != nil {
		return fmt.Errorf("baseline fetch failed: %w", err)
	}

	p.store.Append(sourceID, events)
	return p.store.Save(p.cacheDir, sourceID)
}

// tryIncrementalSync attempts to fetch only items updated since the last known event.
func (p *LogProvider) tryIncrementalSync(sourceID string, jql string) (bool, error) {
	latest := p.store.GetLatestTimestamp(sourceID)
	if latest.IsZero() {
		return false, nil
	}

	// Safety check: if the last event is too old (e.g. > 30 days), don't trust incremental sync
	// to satisfy stages that might need a wider window.
	if time.Since(latest) > 30*24*time.Hour {
		return false, nil
	}

	log.Info().Str("source", sourceID).Time("since", latest).Msg("Attempting incremental sync")

	// We use >= and subtract 1 second to be safe and avoid missing events exactly at the timestamp
	// Jira's JQL resolution is usually by minute, but some APIs support second.
	// To be truly safe with deduplication, we fetch from the start of the minute.
	sinceStr := latest.Add(-1 * time.Minute).Format("2006-01-02 15:04")
	incJQL := fmt.Sprintf("(%s) AND updated >= '%s'", jql, sinceStr)

	events, err := p.fetchAll(incJQL, 0)
	if err != nil {
		return false, err
	}

	p.store.Append(sourceID, events)
	// We only call Save here if we actually found something
	if len(events) > 0 {
		_ = p.store.Save(p.cacheDir, sourceID)
	}

	return true, nil
}

// fetchAll handles paged fetching of issues and their history.
func (p *LogProvider) fetchAll(jql string, limit int) ([]IssueEvent, error) {
	var allEvents []IssueEvent
	startAt := 0
	maxResults := 50 // Standard Jira page size

	for {
		log.Debug().Str("jql", jql).Int("startAt", startAt).Msg("Fetching page from Jira")
		resp, err := p.jira.SearchIssuesWithHistory(jql, startAt, maxResults)
		if err != nil {
			return nil, err
		}

		for _, dto := range resp.Issues {
			allEvents = append(allEvents, TransformIssue(dto)...)
		}

		startAt += len(resp.Issues)

		// Break if we've reached the total or a specific limit
		if len(resp.Issues) == 0 || startAt >= resp.Total || (limit > 0 && startAt >= limit) {
			break
		}
	}

	return allEvents, nil
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
