package eventlog

import (
	"fmt"
	"sort"
	"sync"
	"time"
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

	// 3. Sort by Timestamp and then SequenceID for deterministic ordering
	sort.Slice(log, func(i, j int) bool {
		if !log[i].Timestamp.Equal(log[j].Timestamp) {
			return log[i].Timestamp.Before(log[j].Timestamp)
		}
		return log[i].SequenceID < log[j].SequenceID
	})

	s.logs[sourceID] = log
}

// GetEventsInRange returns a copy of events within the specified time window.
func (s *EventStore) GetEventsInRange(sourceID string, start, end time.Time) []IssueEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log, ok := s.logs[sourceID]
	if !ok || len(log) == 0 {
		return nil
	}

	// Use binary search to find the start index
	startIdx := sort.Search(len(log), func(i int) bool {
		return !log[i].Timestamp.Before(start)
	})

	if startIdx == len(log) {
		return nil
	}

	// Find the end index
	endIdx := sort.Search(len(log), func(i int) bool {
		return log[i].Timestamp.After(end)
	})

	// Return a copy to ensure thread safety for the consumer
	result := make([]IssueEvent, endIdx-startIdx)
	copy(result, log[startIdx:endIdx])
	return result
}

// GetEventsForIssue returns the full lifecycle of a single issue within a source.
func (s *EventStore) GetEventsForIssue(sourceID string, issueKey string) []IssueEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log := s.logs[sourceID]
	var result []IssueEvent
	for _, e := range log {
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
		e.Timestamp.UnixNano(),
		e.EventType,
		e.ToStatus,
		e.Resolution,
	)
}
