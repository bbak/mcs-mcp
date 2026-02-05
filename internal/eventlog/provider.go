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

// Hydrate ensures the event log is populated with sufficient history for analysis.
func (p *LogProvider) Hydrate(sourceID string, jql string) error {
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

	for {
		resp, err := p.client.SearchIssuesWithHistory(hydrateJQL, totalFetched, BatchSize)
		if err != nil {
			return fmt.Errorf("hydration failed at offset %d: %w", totalFetched, err)
		}

		if len(resp.Issues) == 0 {
			break
		}

		var batchEvents []IssueEvent
		var oldestInBatch int64

		for _, dto := range resp.Issues {
			evts := TransformIssue(dto)
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
			resp, err := p.client.SearchIssuesWithHistory(baselineJQL, baselineOffset, BatchSize)
			if err != nil {
				break
			}
			if len(resp.Issues) == 0 {
				break
			}

			var batchEvents []IssueEvent
			for _, dto := range resp.Issues {
				evts := TransformIssue(dto)
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
	return nil
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
