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
	client   jira.Client
	store    *EventStore
	cacheDir string
}

func NewLogProvider(client jira.Client, store *EventStore, cacheDir string) *LogProvider {
	return &LogProvider{
		client:   client,
		store:    store,
		cacheDir: cacheDir,
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

// Hydrate ensures the event log is populated with sufficient history for analysis.
func (p *LogProvider) Hydrate(sourceID string, projectKey string, jql string, reg *jira.NameRegistry) (*jira.NameRegistry, error) {
	const (
		MinTotalItems    = 1000
		MinResolvedItems = 200
		BatchSize        = 300
		HardLimit        = 8 * BatchSize // 2400 items
	)

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
			// Trigger workflow cache wipe via side effect?
			// Better if we have a way to signal this.
			// For now, we'll delete the respective workflow file if it exists.
			workflowPath := filepath.Join(p.cacheDir, fmt.Sprintf("%s-workflow.json", sourceID))
			_ = os.Remove(workflowPath)
		}
		latest = time.Time{} // Treat as fresh
	}

	// 3. Identification: Is this an Incremental Sync or Initial Hydration?
	isIncremental := !latest.IsZero()

	// 4. Hydration / Sync Loop
	log.Info().Str("source", sourceID).Bool("incremental", isIncremental).Msg("Starting hydration process")

	totalFetched := 0
	resolvedFetched := 0
	targetDate := time.Now().AddDate(-1, 0, 0) // 1 year ago for initial baseline

	// Determine JQL and Ordering
	var hydrateJQL string
	if isIncremental {
		// Incremental Sync: Fetch EVERYTHING between latest and now
		// Order by ASC to ensure we process changes in chronological sequence
		tsStr := latest.Format("2006-01-02 15:04")
		hydrateJQL = fmt.Sprintf("(%s) AND updated >= \"%s\" ORDER BY updated ASC", jql, tsStr)
	} else {
		// Initial Hydration: Fetch enough for a robust baseline, but bounded by time and volume
		hydrateJQL = fmt.Sprintf("(%s) AND updated >= startOfDay(\"-24M\") ORDER BY updated DESC", jql)
	}

	registry := reg
	if registry == nil {
		registry = p.getRegistryHelper(projectKey)
	}

	for {
		resp, err := p.client.SearchIssues(hydrateJQL, totalFetched, BatchSize)
		if err != nil {
			return registry, fmt.Errorf("hydration failed at offset %d: %w", totalFetched, err)
		}

		if len(resp.Issues) == 0 {
			break
		}

		var batchEvents []IssueEvent
		var oldestInBatch int64

		for _, dto := range resp.Issues {
			evts := TransformIssue(dto, registry)
			batchEvents = append(batchEvents, evts...)

			if dto.Fields.ResolutionDate != "" || dto.Fields.Resolution.Name != "" {
				resolvedFetched++
			}

			if upd, err := jira.ParseTime(dto.Fields.Updated); err == nil {
				if upd.UnixMicro() < oldestInBatch || oldestInBatch == 0 {
					oldestInBatch = upd.UnixMicro()
				}
			}
		}

		p.store.Append(sourceID, batchEvents)
		totalFetched += len(resp.Issues)

		// Exit Conditions
		if isIncremental {
			// Incremental Mode: Never break early until all paged results are processed
			if len(resp.Issues) < BatchSize {
				break
			}
		} else {
			// Initial Mode: Apply baseline heuristics to avoid infinite initial ingestion
			oldestTime := time.UnixMicro(oldestInBatch)
			if totalFetched >= HardLimit {
				break
			}
			if totalFetched >= MinTotalItems && oldestTime.Before(targetDate) {
				break
			}
			if len(resp.Issues) < BatchSize {
				break
			}
		}
	}

	// 5. Stage 2 Baseline Depth (Only for Initial Hydration if needed)
	if !isIncremental && resolvedFetched < MinResolvedItems && totalFetched < HardLimit {
		log.Info().Int("current", resolvedFetched).Msg("Initial Hydrate Stage 2: Fetching explicit baseline")
		baselineJQL := fmt.Sprintf("(%s) AND resolution is not EMPTY ORDER BY resolutiondate DESC", jql)

		baselineOffset := 0
		for totalFetched < HardLimit && resolvedFetched < MinResolvedItems {
			resp, err := p.client.SearchIssues(baselineJQL, baselineOffset, BatchSize)
			if err != nil {
				break
			}
			if len(resp.Issues) == 0 {
				break
			}

			var batchEvents []IssueEvent
			for _, dto := range resp.Issues {
				evts := TransformIssue(dto, registry)
				batchEvents = append(batchEvents, evts...)
				resolvedFetched++
			}
			p.store.Append(sourceID, batchEvents)
			totalFetched += len(resp.Issues)
			baselineOffset += len(resp.Issues)

			if len(resp.Issues) < BatchSize {
				break
			}
		}
	}

	// 6. Save to Cache
	if p.cacheDir != "" {
		if err := p.store.Save(p.cacheDir, sourceID); err != nil {
			log.Warn().Err(err).Str("source", sourceID).Msg("Hydrate: Failed to save cache")
		}
	}

	log.Info().Int("total", totalFetched).Int("resolved", resolvedFetched).Msg("Hydration complete")
	return registry, nil
}

func (p *LogProvider) GetEventsInRange(sourceID string, start, end time.Time) []IssueEvent {
	return p.store.GetEventsInRange(sourceID, start, end)
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

// GetMostRecentUpdates returns OMRC and NMRC for a source.
func (p *LogProvider) GetMostRecentUpdates(sourceID string) (time.Time, time.Time) {
	return p.store.GetMostRecentUpdates(sourceID)
}

// CatchUp fetches new items since the last sync.
func (p *LogProvider) CatchUp(sourceID string, projectKey string, jql string, reg *jira.NameRegistry) (int, time.Time, *jira.NameRegistry, error) {
	_, nmrc := p.store.GetMostRecentUpdates(sourceID)
	if nmrc.IsZero() {
		return 0, time.Time{}, nil, fmt.Errorf("cannot catch up: no existing cache for %s", sourceID)
	}

	const BatchSize = 300
	totalFetched := 0

	tsStr := nmrc.Format("2006-01-02 15:04")
	catchUpJQL := fmt.Sprintf("(%s) AND updated > \"%s\" ORDER BY updated ASC", jql, tsStr)

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
			evts := TransformIssue(dto, registry)
			batchEvents = append(batchEvents, evts...)
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

// ExpandHistory fetches older items for a source.
func (p *LogProvider) ExpandHistory(sourceID string, projectKey string, jql string, chunks int, reg *jira.NameRegistry) (int, time.Time, *jira.NameRegistry, error) {
	omrc, _ := p.store.GetMostRecentUpdates(sourceID)
	if omrc.IsZero() {
		return 0, time.Time{}, nil, fmt.Errorf("cannot expand history: no existing cache for %s", sourceID)
	}

	const BatchSize = 300
	totalFetched := 0
	limit := chunks * BatchSize

	tsStr := omrc.Format("2006-01-02 15:04")
	expandJQL := fmt.Sprintf("(%s) AND updated <= \"%s\" ORDER BY updated DESC", jql, tsStr)

	registry := reg
	if registry == nil {
		registry = p.getRegistryHelper(projectKey)
	}

	log.Info().Str("source", sourceID).Time("omrc", omrc).Int("limit", limit).Msg("Starting history expansion")

	for totalFetched < limit {
		resp, err := p.client.SearchIssues(expandJQL, totalFetched, BatchSize)
		if err != nil {
			return totalFetched, omrc, registry, fmt.Errorf("expansion failed at offset %d: %w", totalFetched, err)
		}

		if len(resp.Issues) == 0 {
			break
		}

		var batchEvents []IssueEvent
		for _, dto := range resp.Issues {
			evts := TransformIssue(dto, registry)
			batchEvents = append(batchEvents, evts...)
		}

		p.store.Merge(sourceID, batchEvents)
		totalFetched += len(resp.Issues)

		if len(resp.Issues) < BatchSize {
			break
		}
	}

	// Always trigger catch-up to ensure consistency
	var err error
	_, _, registry, err = p.CatchUp(sourceID, projectKey, jql, registry)
	if err != nil {
		log.Warn().Err(err).Msg("Expansion followed by catch-up failed")
	}

	if totalFetched > 0 && p.cacheDir != "" {
		_ = p.store.Save(p.cacheDir, sourceID)
	}

	log.Info().Int("fetched", totalFetched).Msg("History expansion complete")
	return totalFetched, omrc, registry, nil
}
