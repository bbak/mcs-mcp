package eventlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEventStore_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "eventlog-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store1 := NewEventStore()
	sourceID := "test-board"

	events := []IssueEvent{
		{
			IssueKey:   "PROJ-1",
			EventType:  Created,
			Timestamp:  time.Now().Truncate(time.Second).Add(-2 * time.Hour),
			SequenceID: 1,
		},
		{
			IssueKey:   "PROJ-1",
			EventType:  Transitioned,
			ToStatus:   "Doing",
			Timestamp:  time.Now().Truncate(time.Second).Add(-1 * time.Hour),
			SequenceID: 2,
		},
	}

	// Save
	store1.Append(sourceID, events)
	err = store1.Save(tmpDir, sourceID)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	cachePath := filepath.Join(tmpDir, sourceID+".jsonl")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Errorf("Cache file does not exist: %s", cachePath)
	}

	// Load into new store
	store2 := NewEventStore()
	err = store2.Load(tmpDir, sourceID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	loadedEvents := store2.GetEventsForIssue(sourceID, "PROJ-1")
	if len(loadedEvents) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(loadedEvents))
	}
	if loadedEvents[0].IssueKey != "PROJ-1" {
		t.Errorf("Expected IssueKey PROJ-1, got %s", loadedEvents[0].IssueKey)
	}
	if loadedEvents[0].EventType != Created {
		t.Errorf("Expected EventType Created, got %s", loadedEvents[0].EventType)
	}
	if loadedEvents[1].ToStatus != "Doing" {
		t.Errorf("Expected ToStatus Doing, got %s", loadedEvents[1].ToStatus)
	}

	// Verify latest timestamp
	latest := store2.GetLatestTimestamp(sourceID)
	if !latest.Equal(events[1].Timestamp) {
		t.Errorf("Expected latest timestamp %v, got %v", events[1].Timestamp, latest)
	}

	// Verify deduplication after reload and re-append
	store2.Append(sourceID, events) // Same events
	totalAfterReAppend := len(store2.logs[sourceID])
	if totalAfterReAppend != 2 {
		t.Errorf("Expected 2 events after re-append (deduplication), got %d", totalAfterReAppend)
	}
}
