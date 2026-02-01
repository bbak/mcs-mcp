package eventlog

import (
	"fmt"
	"mcs-mcp/internal/jira"
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
		HardLimit        = 5000
		BatchSize        = 50
	)

	// 1. Try to Load from Cache
	if p.cacheDir != "" {
		if err := p.store.Load(p.cacheDir, sourceID); err == nil {
			log.Debug().Str("source", sourceID).Msg("Hydrate: Loaded from cache")
		}
	}

	// 2. Incremental or Full?
	latest := p.store.GetLatestTimestamp(sourceID)
	// If it's very recent (last 30 mins), skip hydration to save API calls
	if !latest.IsZero() && time.Since(latest) < 30*time.Minute {
		log.Debug().Str("source", sourceID).Msg("Hydrate: Cache is fresh, skipping API sync")
		return nil
	}

	// Stage 1: Recent Activity & WIP
	log.Info().Str("source", sourceID).Msg("Hydrate Stage 1: Fetching activity")

	hydrateJQL := fmt.Sprintf("(%s) ORDER BY updated DESC", jql)
	targetDate := time.Now().AddDate(-1, 0, 0) // 1 year ago

	totalFetched := 0
	resolvedFetched := 0

	for totalFetched < HardLimit {
		resp, err := p.client.SearchIssuesWithHistory(hydrateJQL, totalFetched, BatchSize)
		if err != nil {
			return fmt.Errorf("hydrate stage 1 failed: %w", err)
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

		oldestTime := time.UnixMicro(oldestInBatch)
		if totalFetched >= MinTotalItems && oldestTime.Before(targetDate) {
			break
		}
	}

	// Stage 2: Baseline Depth
	if resolvedFetched < MinResolvedItems && totalFetched < HardLimit {
		log.Info().Int("current", resolvedFetched).Msg("Hydrate Stage 2: Fetching explicit baseline")
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

	// 3. Save to Cache
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

func (p *LogProvider) GetLatestTimestamp(sourceID string) time.Time {
	return p.store.GetLatestTimestamp(sourceID)
}

func (p *LogProvider) GetEventCount(sourceID string) int {
	return p.store.Count(sourceID)
}
