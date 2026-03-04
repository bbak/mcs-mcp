package mcp

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcs-mcp/internal/config"
	"mcs-mcp/internal/jira"
	"mcs-mcp/internal/stats"
)

var update = flag.Bool("update", false, "update golden handler baselines")

const (
	testProject  = "MCSTEST"
	testBoard    = 0
	testSourceID = "MCSTEST_0"
)

// GoldenSnapshot captures the analytical payload sent to the AI agent,
// stripped of envelope metadata (context, diagnostics).
type GoldenSnapshot struct {
	Data       any                 `json:"data"`
	Guardrails *ResponseGuardrails `json:"guardrails,omitempty"`
}

func goldenDirPath() string {
	return filepath.Join("..", "testdata", "golden", "mcp")
}

func fixtureDirPath() string {
	return filepath.Join("..", "testdata", "golden")
}

// assertGolden compares the analytical payload of env against the named golden file.
// With -update it writes the actual output as the new baseline.
func assertGolden(t *testing.T, name string, env ResponseEnvelope) {
	t.Helper()

	snapshot := GoldenSnapshot{
		Data:       env.Data,
		Guardrails: env.Guardrails,
	}

	actualJSON, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("assertGolden: marshal: %v", err)
	}
	actualJSON = normalizeNewlines(actualJSON)

	goldenPath := filepath.Join(goldenDirPath(), name+".json")

	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("assertGolden: mkdir: %v", err)
		}
		if err := os.WriteFile(goldenPath, actualJSON, 0644); err != nil {
			t.Fatalf("assertGolden: write: %v", err)
		}
		return
	}

	expectedJSON, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden file missing: %s — run with -update to generate", goldenPath)
		}
		t.Fatalf("assertGolden: read: %v", err)
	}

	if !bytes.Equal(expectedJSON, actualJSON) {
		actualPath := goldenPath + ".actual"
		_ = os.WriteFile(actualPath, actualJSON, 0644)
		t.Errorf("golden mismatch for %q — diff against %s", name, actualPath)
	}
}

func normalizeNewlines(b []byte) []byte {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return b
}

// sha256HexFile returns the hex-encoded SHA-256 digest of a file.
func sha256HexFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checkFixtureHash verifies the events fixture has not changed since the last -update run.
// If it has and -update is set, existing golden JSON files are deleted so they are regenerated.
func checkFixtureHash(t *testing.T) {
	t.Helper()

	eventsPath := filepath.Join(fixtureDirPath(), "simulated_events.jsonl")
	hashPath := filepath.Join(goldenDirPath(), ".fixtures.sha256")

	currentHash, err := sha256HexFile(eventsPath)
	if err != nil {
		t.Fatalf("checkFixtureHash: hash events file: %v", err)
	}

	storedHash := ""
	if raw, err := os.ReadFile(hashPath); err == nil {
		storedHash = strings.TrimSpace(string(raw))
	}

	if currentHash == storedHash {
		return
	}

	if *update {
		entries, _ := filepath.Glob(filepath.Join(goldenDirPath(), "*.json"))
		for _, e := range entries {
			_ = os.Remove(e)
		}
		return // hash written at the end of TestHandlers_Golden
	}

	t.Fatalf(
		"simulated_events.jsonl has changed (hash mismatch).\n"+
			"Run: go test ./internal/mcp/... -run TestHandlers_Golden -update\n"+
			"to regenerate all handler golden baselines.",
	)
}

// writeFixtureHash writes the current SHA-256 of simulated_events.jsonl to the sidecar file.
func writeFixtureHash(t *testing.T) {
	t.Helper()

	eventsPath := filepath.Join(fixtureDirPath(), "simulated_events.jsonl")
	hashPath := filepath.Join(goldenDirPath(), ".fixtures.sha256")

	hash, err := sha256HexFile(eventsPath)
	if err != nil {
		t.Fatalf("writeFixtureHash: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(hashPath), 0755); err != nil {
		t.Fatalf("writeFixtureHash: mkdir: %v", err)
	}
	if err := os.WriteFile(hashPath, []byte(hash+"\n"), 0644); err != nil {
		t.Fatalf("writeFixtureHash: write: %v", err)
	}
}

// newGoldenServer creates a test Server pre-seeded with the canonical fixture data.
// The server clock is pinned to the latest event timestamp in simulated_events.jsonl.
// Server state is pre-anchored to bypass loadWorkflow, using workflow semantics translated
// from simulated_workflow.json with status name→ID mappings derived from the events.
func newGoldenServer(t *testing.T) *Server {
	t.Helper()

	fixDir := fixtureDirPath()
	cacheDir := t.TempDir()

	// 1. Copy events to cache dir as MCSTEST_0.jsonl
	eventsData, err := os.ReadFile(filepath.Join(fixDir, "simulated_events.jsonl"))
	if err != nil {
		t.Fatalf("newGoldenServer: read events: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, testSourceID+".jsonl"), eventsData, 0644); err != nil {
		t.Fatalf("newGoldenServer: write events cache: %v", err)
	}

	// 2. Parse simulated_workflow.json
	wfData, err := os.ReadFile(filepath.Join(fixDir, "simulated_workflow.json"))
	if err != nil {
		t.Fatalf("newGoldenServer: read workflow: %v", err)
	}
	var wf struct {
		Mapping         map[string]stats.StatusMetadata `json:"mapping"`
		Resolutions     map[string]string               `json:"resolutions"`
		CommitmentPoint string                          `json:"commitment_point"`
		DiscoveryCutoff string                          `json:"discovery_cutoff"`
	}
	if err := json.Unmarshal(wfData, &wf); err != nil {
		t.Fatalf("newGoldenServer: parse workflow: %v", err)
	}

	// 3. Scan events to build status name↔ID maps
	nameToID := make(map[string]string)
	idToName := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(eventsData))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var evt struct {
			ToStatus     string `json:"toStatus"`
			ToStatusID   string `json:"toStatusId"`
			FromStatus   string `json:"fromStatus"`
			FromStatusID string `json:"fromStatusId"`
		}
		if json.Unmarshal(scanner.Bytes(), &evt) != nil {
			continue
		}
		if evt.ToStatus != "" && evt.ToStatusID != "" {
			nameToID[evt.ToStatus] = evt.ToStatusID
			idToName[evt.ToStatusID] = evt.ToStatus
		}
		if evt.FromStatus != "" && evt.FromStatusID != "" {
			nameToID[evt.FromStatus] = evt.FromStatusID
			idToName[evt.FromStatusID] = evt.FromStatus
		}
	}

	// 4. Build ID-keyed status mapping (simulated_workflow.json uses name keys)
	idMapping := make(map[string]stats.StatusMetadata)
	for name, m := range wf.Mapping {
		m.Name = name
		if id, ok := nameToID[name]; ok && id != "" {
			idMapping[id] = m
		} else {
			idMapping[name] = m // unknown status name — keep as-is
		}
	}

	// 5. Translate commitment point name → ID
	commitmentPoint := wf.CommitmentPoint
	if id, ok := nameToID[wf.CommitmentPoint]; ok && id != "" {
		commitmentPoint = id
	}

	// 6. Parse discovery cutoff
	var discoveryCutoff *time.Time
	if wf.DiscoveryCutoff != "" {
		if ct, parseErr := time.Parse(time.RFC3339, wf.DiscoveryCutoff); parseErr == nil {
			discoveryCutoff = &ct
		}
	}

	// 7. Pin clock to the last event's timestamp (events are strictly time-ordered)
	latestTS := lastEventTimestamp(t, eventsData)

	// 8. Build NameRegistry from events-derived ID→name map
	registry := &jira.NameRegistry{Statuses: idToName}

	// 9. Create server and pre-anchor state to bypass loadWorkflow
	srv := NewServer(&config.AppConfig{CacheDir: cacheDir, CommitmentBackflowReset: true}, &DummyClient{})
	srv.activeSourceID       = testSourceID
	srv.activeMapping        = idMapping
	srv.activeResolutions    = wf.Resolutions // name-keyed; matches resolution fields in events
	srv.activeCommitmentPoint = commitmentPoint
	srv.activeDiscoveryCutoff = discoveryCutoff
	srv.activeRegistry       = registry
	srv.activeEvaluationDate = &latestTS
	srv.simulationSeed       = 42

	return srv
}

// lastEventTimestamp reads the last non-empty line of a JSONL byte slice and returns
// its "ts" field (Unix microseconds) as a UTC time.Time.
func lastEventTimestamp(t *testing.T, data []byte) time.Time {
	t.Helper()
	data = bytes.TrimRight(data, "\r\n ")
	idx := bytes.LastIndexByte(data, '\n')
	var lastLine []byte
	if idx < 0 {
		lastLine = data
	} else {
		lastLine = data[idx+1:]
	}
	var evt struct {
		Ts int64 `json:"ts"`
	}
	if err := json.Unmarshal(lastLine, &evt); err != nil {
		t.Fatalf("lastEventTimestamp: parse last line: %v", err)
	}
	if evt.Ts == 0 {
		t.Fatal("lastEventTimestamp: ts field is zero in last event line")
	}
	return time.UnixMicro(evt.Ts).UTC()
}
