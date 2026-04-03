package chartbuf

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestPushAndGet(t *testing.T) {
	b := NewBuffer(3)

	uuid := b.Push("analyze_throughput", json.RawMessage(`{"x":1}`), json.RawMessage(`{"board_id":1}`))
	if uuid == "" {
		t.Fatal("expected non-empty UUID")
	}

	entry, ok := b.Get(uuid)
	if !ok {
		t.Fatal("expected to find entry")
	}
	if entry.ToolName != "analyze_throughput" {
		t.Errorf("got tool %q, want analyze_throughput", entry.ToolName)
	}
	if string(entry.Data) != `{"x":1}` {
		t.Errorf("got data %s, want {\"x\":1}", entry.Data)
	}
}

func TestGetMissing(t *testing.T) {
	b := NewBuffer(3)
	if _, ok := b.Get("nonexistent"); ok {
		t.Fatal("expected not found for unknown UUID")
	}
}

func TestEviction(t *testing.T) {
	b := NewBuffer(2)

	uuid1 := b.Push("tool_a", json.RawMessage(`{}`), json.RawMessage(`{}`))
	_ = b.Push("tool_b", json.RawMessage(`{}`), json.RawMessage(`{}`))

	// Both should be present.
	if _, ok := b.Get(uuid1); !ok {
		t.Fatal("uuid1 should still be present")
	}

	// Third push evicts uuid1.
	_ = b.Push("tool_c", json.RawMessage(`{}`), json.RawMessage(`{}`))

	if _, ok := b.Get(uuid1); ok {
		t.Fatal("uuid1 should have been evicted")
	}
	if b.Len() != 2 {
		t.Errorf("got len %d, want 2", b.Len())
	}
}

func TestUUIDUniqueness(t *testing.T) {
	b := NewBuffer(100)
	seen := make(map[string]struct{}, 100)

	for i := 0; i < 100; i++ {
		uuid := b.Push("tool", json.RawMessage(`{}`), json.RawMessage(`{}`))
		if _, exists := seen[uuid]; exists {
			t.Fatalf("duplicate UUID: %s", uuid)
		}
		seen[uuid] = struct{}{}
	}
}

func TestConcurrentAccess(t *testing.T) {
	b := NewBuffer(50)
	var wg sync.WaitGroup

	// Concurrent writers.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				b.Push("tool", json.RawMessage(`{}`), json.RawMessage(`{}`))
			}
		}()
	}

	// Concurrent readers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				b.Get("nonexistent")
			}
		}()
	}

	wg.Wait()
	// No panics or races = pass.
}
