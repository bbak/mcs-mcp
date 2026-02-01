package eventlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// EventStore provides thread-safe, chronological storage for IssueEvents.
type EventStore struct {
	mu   sync.RWMutex
	logs map[string][]IssueEvent // Partitioned by SourceID (Board ID)
}

// NewEventStore creates a new empty EventStore.
func NewEventStore() *EventStore {
	return &EventStore{
		logs: make(map[string][]IssueEvent),
	}
}

// Append adds new events to the log for a given source, ensuring chronological order and deduplication.
func (s *EventStore) Append(sourceID string, events []IssueEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log := s.logs[sourceID]

	// 1. Create a map of existing event "identities" for deduplication
	// Identity = IssueKey + Timestamp + EventType + ToStatus + Resolution
	existing := make(map[string]bool)
	for _, e := range log {
		existing[e.identity()] = true
	}

	// 2. Filter new events
	newCount := 0
	for _, e := range events {
		if !existing[e.identity()] {
			log = append(log, e)
			newCount++
		}
	}

	if newCount == 0 {
		return
	}

	// 3. Sort by Timestamp and then EventType for deterministic ordering
	sort.Slice(log, func(i, j int) bool {
		if log[i].Timestamp != log[j].Timestamp {
			return log[i].Timestamp < log[j].Timestamp
		}
		return log[i].EventType < log[j].EventType
	})

	s.logs[sourceID] = log
}

// Load reads events from a JSONL cache file for the given source.
func (s *EventStore) Load(cacheDir string, sourceID string) error {
	path := filepath.Join(cacheDir, fmt.Sprintf("%s.jsonl", sourceID))
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No cache yet, not an error
		}
		return fmt.Errorf("failed to open cache: %w", err)
	}
	defer file.Close()

	var events []IssueEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var e IssueEvent
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			log.Warn().Err(err).Str("source", sourceID).Msg("Skipping invalid JSON line in cache")
			continue
		}
		events = append(events, e)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading cache: %w", err)
	}

	log.Info().Str("source", sourceID).Int("count", len(events)).Msg("Loaded events from cache")
	s.Append(sourceID, events)
	return nil
}

// Save persists events for the given source to a JSONL cache file.
func (s *EventStore) Save(cacheDir string, sourceID string) error {
	s.mu.RLock()
	logData, ok := s.logs[sourceID]
	s.mu.RUnlock()

	if !ok || len(logData) == 0 {
		return nil
	}

	path := filepath.Join(cacheDir, fmt.Sprintf("%s.jsonl", sourceID))
	tmpPath := path + ".tmp"

	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp cache file: %w", err)
	}

	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)

	for _, e := range logData {
		if err := encoder.Encode(e); err != nil {
			file.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to encode event: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	log.Info().Str("source", sourceID).Int("count", len(logData)).Msg("Persistent events saved to cache")
	return nil
}

// GetLatestTimestamp returns the timestamp of the most recent event for a source.
func (s *EventStore) GetLatestTimestamp(sourceID string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logData, ok := s.logs[sourceID]
	if !ok || len(logData) == 0 {
		return time.Time{}
	}

	// Events are sorted, so the last one is the latest
	return time.UnixMicro(logData[len(logData)-1].Timestamp)
}

// Count returns the number of events in the store for a source.
func (s *EventStore) Count(sourceID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.logs[sourceID])
}

// GetEventsInRange returns a copy of events within the specified time window.
func (s *EventStore) GetEventsInRange(sourceID string, start, end time.Time) []IssueEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logData, ok := s.logs[sourceID]
	if !ok {
		return nil
	}

	startTs := start.UnixMicro()
	endTs := end.UnixMicro()

	var result []IssueEvent
	for _, e := range logData {
		if e.Timestamp >= startTs && (end.IsZero() || e.Timestamp <= endTs) {
			result = append(result, e)
		}
	}
	return result
}

// GetEventsForIssue returns the full event history for a single issue.
func (s *EventStore) GetEventsForIssue(sourceID string, issueKey string) []IssueEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logData, ok := s.logs[sourceID]
	if !ok {
		return nil
	}

	var result []IssueEvent
	for _, e := range logData {
		if e.IssueKey == issueKey {
			result = append(result, e)
		}
	}
	return result
}

// identity computes a unique string identifier for an event to aid deduplication.
func (e IssueEvent) identity() string {
	return fmt.Sprintf("%s|%d|%s|%s|%s",
		e.IssueKey,
		e.Timestamp,
		e.EventType,
		e.ToStatus,
		e.Resolution,
	)
}
