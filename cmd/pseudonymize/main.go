// pseudonymize replaces real Jira project key prefixes in a cache JSONL file with
// "MOCK", producing anonymized fixture files suitable for committing to the repository.
//
// Usage:
//
//	go run ./cmd/pseudonymize [--out <dir>] <path/to/PROJ_12345.jsonl>
//
// The companion workflow JSON file is auto-detected as <same-dir>/<basename>_workflow.json.
// Outputs are written to --out (default: internal/testdata/golden/).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	outDir := flag.String("out", filepath.Join("internal", "testdata", "golden"),
		"Output directory for anonymized files")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: pseudonymize [--out <dir>] <path/to/SOURCE_BOARDID.jsonl>\n\n")
		fmt.Fprintln(os.Stderr, "Pseudonymizes a Jira cache file by replacing issue key prefixes with MOCK.")
		fmt.Fprintln(os.Stderr, "The companion workflow file is auto-detected as <same-dir>/<base>_workflow.json.")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	eventsPath := flag.Arg(0)

	base := strings.TrimSuffix(filepath.Base(eventsPath), ".jsonl")
	workflowPath := filepath.Join(filepath.Dir(eventsPath), base+"_workflow.json")

	// First pass: discover all project key prefixes
	prefixes, err := discoverPrefixes(eventsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning events: %v\n", err)
		os.Exit(1)
	}

	if len(prefixes) == 0 {
		fmt.Fprintln(os.Stderr, "no real prefixes found — all issue keys may already use MOCK")
	} else {
		sorted := sortedKeys(prefixes)
		if len(sorted) > 1 {
			fmt.Fprintf(os.Stderr,
				"WARNING: multiple project key prefixes found: %v\n"+
					"         MOCK-<N> collision is possible if two projects share an issue number.\n",
				sorted)
		}
		fmt.Printf("replacing prefix(es) %v → MOCK\n", sorted)
	}

	// Second pass: rewrite events
	anonymizedEvents, err := rewriteEvents(eventsPath, sortedKeys(prefixes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error rewriting events: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir: %v\n", err)
		os.Exit(1)
	}

	eventsOut := filepath.Join(*outDir, "simulated_events.jsonl")
	if err := os.WriteFile(eventsOut, anonymizedEvents, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing events: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", eventsOut)

	// Rewrite workflow file if present
	if _, err := os.Stat(workflowPath); err == nil {
		anonymizedWorkflow, err := rewriteWorkflow(workflowPath, sortedKeys(prefixes))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error rewriting workflow: %v\n", err)
			os.Exit(1)
		}
		workflowOut := filepath.Join(*outDir, "simulated_workflow.json")
		if err := os.WriteFile(workflowOut, anonymizedWorkflow, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing workflow: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", workflowOut)
	} else {
		fmt.Fprintf(os.Stderr, "WARNING: workflow file not found at %s, skipping\n", workflowPath)
	}
}

// discoverPrefixes scans the JSONL file and collects all distinct project key prefixes
// found in issueKey values (everything before the first '-'), excluding "MOCK".
func discoverPrefixes(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	prefixes := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		var evt struct {
			IssueKey string `json:"issueKey"`
		}
		if json.Unmarshal(scanner.Bytes(), &evt) != nil || evt.IssueKey == "" {
			continue
		}
		if dash := strings.IndexByte(evt.IssueKey, '-'); dash > 0 {
			prefix := evt.IssueKey[:dash]
			if prefix != "MOCK" {
				prefixes[prefix] = true
			}
		}
	}
	return prefixes, scanner.Err()
}

// rewriteEvents reads the JSONL file and replaces each prefix occurrence
// (e.g. "PROJ-") with "MOCK-" via raw byte replacement.
func rewriteEvents(path string, prefixes []string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		line = replaceAllPrefixes(line, prefixes)
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// rewriteWorkflow parses the workflow JSON, replaces the project-key portion of
// source_id, and re-serialises with stable (sorted) key ordering.
func rewriteWorkflow(path string, prefixes []string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse workflow JSON: %w", err)
	}

	if sourceIDRaw, ok := raw["source_id"]; ok {
		var sourceID string
		if err := json.Unmarshal(sourceIDRaw, &sourceID); err == nil {
			for _, prefix := range prefixes {
				sourceID = strings.ReplaceAll(sourceID, prefix+"_", "MOCK_")
			}
			if updated, err := json.Marshal(sourceID); err == nil {
				raw["source_id"] = updated
			}
		}
	}

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, err
	}
	out = bytes.ReplaceAll(out, []byte("\r\n"), []byte("\n"))
	return append(out, '\n'), nil
}

// replaceAllPrefixes replaces "PREFIX-" with "MOCK-" in raw bytes for each prefix.
func replaceAllPrefixes(data []byte, prefixes []string) []byte {
	for _, prefix := range prefixes {
		data = bytes.ReplaceAll(data, []byte(prefix+"-"), []byte("MOCK-"))
	}
	return data
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
