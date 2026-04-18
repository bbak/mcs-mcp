package eventlog

import (
	"fmt"
	"mcs-mcp/internal/jira"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

// LogProvider orchestrates data ingestion and event retrieval.
type LogProvider struct {
	client           jira.Client
	store            *EventStore
	cacheDir         string
	updatedLookbackM int
	createdLookbackM int
	maxItems         int
}

func NewLogProvider(client jira.Client, store *EventStore, cacheDir string, updatedLookbackM, createdLookbackM, maxItems int) *LogProvider {
	return &LogProvider{
		client:           client,
		store:            store,
		cacheDir:         cacheDir,
		updatedLookbackM: updatedLookbackM,
		createdLookbackM: createdLookbackM,
		maxItems:         maxItems,
	}
}

// getRegistryHelper fetches the name registry for a given project.
func (p *LogProvider) getRegistryHelper(projectKey string) *jira.NameRegistry {
	if p.client == nil || projectKey == "" {
		return nil
	}

	reg, err := p.client.GetRegistry(projectKey)
	if err != nil {
		log.Warn().Err(err).Str("project", projectKey).Msg("Failed to fetch name registry, proceeding without stable labels")
		return nil
	}
	return reg
}

// Hydrate ensures the event log is populated with sufficient history for
// analysis. Initial hydration uses a single generous JQL bounded by the
// configured updated/created lookback windows and capped at maxItems.
// Incremental sync (when a cache exists) fetches everything updated since
// the latest cached timestamp.
func (p *LogProvider) Hydrate(sourceID string, projectKey string, jql string, reg *jira.NameRegistry) (*jira.NameRegistry, error) {
	const BatchSize = 300

	// 1. Try to Load from Cache
	if p.cacheDir != "" {
		if err := p.store.Load(p.cacheDir, sourceID); err == nil {
			log.Debug().Str("source", sourceID).Msg("Hydrate: Loaded from cache")
		}
	}

	// 1.5. Intercept MCSTEST: Never query Jira for this source.
	// We rely purely on what was just loaded from cache.
	if sourceID == "MCSTEST_0" || filepath.Base(sourceID) == "MCSTEST" {
		log.Info().Str("source", sourceID).Msg("Hydrate: MCSTEST detected, skipping Jira sync")
		return reg, nil
	}

	latest := p.store.GetLatestTimestamp(sourceID)

	// 2. Validate Cache Recency (2-month rule)
	if !latest.IsZero() && time.Since(latest) > (60*24*time.Hour) {
		log.Info().Str("source", sourceID).Time("latest", latest).Msg("Cache is older than 2 months, evicting and performing full re-ingestion")
		p.store.Clear(sourceID)
		if p.cacheDir != "" {
			_ = DeleteCache(p.cacheDir, sourceID)
			workflowPath := filepath.Join(p.cacheDir, fmt.Sprintf("%s-workflow.json", sourceID))
			_ = os.Remove(workflowPath)
		}
		latest = time.Time{} // Treat as fresh
	}

	// 3. Identification: Is this an Incremental Sync or Initial Hydration?
	isIncremental := !latest.IsZero()

	log.Info().Str("source", sourceID).Bool("incremental", isIncremental).Msg("Starting hydration process")

	var hydrateJQL string
	if isIncremental {
		// Incremental Sync: process changes in chronological order
		tsStr := latest.Format(DateTimeFormat)
		hydrateJQL = fmt.Sprintf(`(%s) AND updated >= "%s" ORDER BY updated ASC`, jql, tsStr)
	} else {
		// Initial Hydration: wide OR predicate captures both recently-updated
		// items AND long-lived items born in the window. Newest first so the
		// max-items cap evicts the oldest tail rather than the active head.
		hydrateJQL = fmt.Sprintf(
			`(%s) AND (updated >= startOfDay("-%dM") OR created >= startOfDay("-%dM")) ORDER BY updated DESC`,
			jql, p.updatedLookbackM, p.createdLookbackM)
	}

	registry := reg
	if registry == nil {
		registry = p.getRegistryHelper(projectKey)
	}

	totalFetched := 0
	for {
		resp, err := p.client.SearchIssues(hydrateJQL, totalFetched, BatchSize)
		if err != nil {
			return registry, fmt.Errorf("hydration failed at offset %d: %w", totalFetched, err)
		}

		if len(resp.Issues) == 0 {
			break
		}

		var batchEvents []IssueEvent
		for _, dto := range resp.Issues {
			batchEvents = append(batchEvents, TransformIssue(dto, registry)...)
		}

		p.store.Append(sourceID, batchEvents)
		totalFetched += len(resp.Issues)

		if isIncremental {
			if len(resp.Issues) < BatchSize {
				break
			}
		} else {
			if totalFetched >= p.maxItems {
				log.Info().Int("total", totalFetched).Int("cap", p.maxItems).Msg("Initial hydration reached INGESTION_MAX_ITEMS cap")
				break
			}
			if len(resp.Issues) < BatchSize {
				break
			}
		}
	}

	// 4. Save to Cache
	if p.cacheDir != "" {
		if err := p.store.Save(p.cacheDir, sourceID); err != nil {
			log.Warn().Err(err).Str("source", sourceID).Msg("Hydrate: Failed to save cache")
		}
	}

	log.Info().Int("total", totalFetched).Bool("incremental", isIncremental).Msg("Hydration complete")
	return registry, nil
}

func (p *LogProvider) GetIssuesInRange(sourceID string, start, end time.Time) []IssueEvent {
	return p.store.GetIssuesInRange(sourceID, start, end)
}

func (p *LogProvider) GetEventsForIssue(sourceID, issueKey string) []IssueEvent {
	return p.store.GetEventsForIssue(sourceID, issueKey)
}

func (p *LogProvider) GetEventsForIssueInAllSources(issueKey string) (string, []IssueEvent) {
	return p.store.FindIssueInAllSources(issueKey)
}

func (p *LogProvider) GetLatestTimestamp(sourceID string) time.Time {
	return p.store.GetLatestTimestamp(sourceID)
}

func (p *LogProvider) GetEventCount(sourceID string) int {
	return p.store.Count(sourceID)
}

func (p *LogProvider) PruneExcept(keepSourceID string) {
	p.store.PruneExcept(keepSourceID)
}

// CatchUp fetches new items since the last sync (NMRC).
func (p *LogProvider) CatchUp(sourceID string, projectKey string, jql string, reg *jira.NameRegistry) (int, time.Time, *jira.NameRegistry, error) {
	_, nmrc := p.store.GetMostRecentUpdates(sourceID)
	if nmrc.IsZero() {
		return 0, time.Time{}, nil, fmt.Errorf("cannot catch up: no existing cache for %s", sourceID)
	}

	const BatchSize = 300
	totalFetched := 0

	tsStr := nmrc.Format(DateTimeFormat)
	catchUpJQL := fmt.Sprintf(`(%s) AND updated > "%s" ORDER BY updated ASC`, jql, tsStr)

	registry := reg
	if registry == nil {
		registry = p.getRegistryHelper(projectKey)
	}

	log.Info().Str("source", sourceID).Time("nmrc", nmrc).Msg("Starting catch-up process")

	for {
		resp, err := p.client.SearchIssues(catchUpJQL, totalFetched, BatchSize)
		if err != nil {
			return totalFetched, nmrc, registry, fmt.Errorf("catch-up failed at offset %d: %w", totalFetched, err)
		}

		if len(resp.Issues) == 0 {
			break
		}

		var batchEvents []IssueEvent
		for _, dto := range resp.Issues {
			batchEvents = append(batchEvents, TransformIssue(dto, registry)...)
		}

		p.store.Merge(sourceID, batchEvents)
		totalFetched += len(resp.Issues)

		if len(resp.Issues) < BatchSize {
			break
		}
	}

	if totalFetched > 0 && p.cacheDir != "" {
		_ = p.store.Save(p.cacheDir, sourceID)
	}

	log.Info().Int("fetched", totalFetched).Msg("Catch-up complete")
	return totalFetched, nmrc, registry, nil
}
