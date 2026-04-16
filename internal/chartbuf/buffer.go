// Package chartbuf is a thread-safe MRU ring buffer that holds the most recent
// analytical tool results keyed by UUID. The HTTP chart server retrieves
// entries on demand to render interactive charts without re-running the
// analysis.
package chartbuf

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
)

// MaxBufferSize is the hard ceiling for MCS_CHARTS_BUFFER_SIZE.
const MaxBufferSize = 100

// Entry holds a buffered tool result for on-demand chart rendering.
type Entry struct {
	UUID     string
	ToolName string
	Data     json.RawMessage // full ResponseEnvelope JSON
	Workflow json.RawMessage // workflow metadata for template context
}

// Buffer is a fixed-size MRU ring buffer for chart-eligible tool results.
// It is safe for concurrent use.
type Buffer struct {
	mu      sync.Mutex
	entries []Entry
	index   map[string]int // UUID → slot position
	size    int
	cursor  int
}

// NewBuffer creates a buffer with the given capacity.
// The caller must ensure 1 <= size <= MaxBufferSize.
func NewBuffer(size int) *Buffer {
	return &Buffer{
		entries: make([]Entry, size),
		index:   make(map[string]int, size),
		size:    size,
	}
}

// Push stores a tool result and returns its UUID.
// If the buffer is full, the oldest entry is evicted.
func (b *Buffer) Push(toolName string, data, workflow json.RawMessage) string {
	uuid := newUUID()

	b.mu.Lock()
	defer b.mu.Unlock()

	// Evict the entry at the current cursor position if occupied.
	if old := b.entries[b.cursor]; old.UUID != "" {
		delete(b.index, old.UUID)
	}

	b.entries[b.cursor] = Entry{
		UUID:     uuid,
		ToolName: toolName,
		Data:     data,
		Workflow: workflow,
	}
	b.index[uuid] = b.cursor
	b.cursor = (b.cursor + 1) % b.size

	return uuid
}

// Get retrieves an entry by UUID. Returns false if not found or evicted.
func (b *Buffer) Get(uuid string) (Entry, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	pos, ok := b.index[uuid]
	if !ok {
		return Entry{}, false
	}
	return b.entries[pos], true
}

// Len returns the number of entries currently stored.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.index)
}

// newUUID generates a random UUID v4.
func newUUID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
