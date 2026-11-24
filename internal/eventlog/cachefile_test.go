package eventlog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCachePath_RejectsNamesOutsideCacheDir(t *testing.T) {
	for _, name := range []string{
		"../notes.jsonl",
		"../../proj/notes.jsonl",
		`..\notes.jsonl`,
		"/etc/notes.jsonl",
		`C:\notes.jsonl`,
		"",
	} {
		if _, err := CachePath("cache", name); err == nil {
			t.Errorf("CachePath(%q) = nil error, want rejection", name)
		}
	}
	if _, err := CachePath("cache", "PROJ_0.jsonl"); err != nil {
		t.Errorf("CachePath(PROJ_0.jsonl) = %v, want nil", err)
	}
}

// TestTraversalSourceID_TouchesNothingOutsideCacheDir reproduces the reported
// attack: a source ID with "../" must not let Load, Save or DeleteCache reach
// files next to the cache directory.
func TestTraversalSourceID_TouchesNothingOutsideCacheDir(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	if err := os.Mkdir(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinels := map[string]string{
		filepath.Join(root, "notes.jsonl"):         `{"key":"no ts field"}` + "\n",
		filepath.Join(root, "notes-workflow.json"): `{"keep":"me"}`,
	}
	for path, content := range sentinels {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	const sourceID = "../notes"
	store := NewEventStore(nil)
	if err := store.Load(cacheDir, sourceID); err == nil {
		t.Error("Load with traversal source ID succeeded, want error")
	}
	store.Append(sourceID, []IssueEvent{{IssueKey: "PROJ-1", Timestamp: 1}})
	if err := store.Save(cacheDir, sourceID); err == nil {
		t.Error("Save with traversal source ID succeeded, want error")
	}
	if err := DeleteCache(cacheDir, sourceID); err == nil {
		t.Error("DeleteCache with traversal source ID succeeded, want error")
	}

	for path, want := range sentinels {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("sentinel %s: %v", path, err)
		}
		if string(got) != want {
			t.Errorf("sentinel %s changed: got %q, want %q", path, got, want)
		}
	}
}

func TestOpenCacheFile_MigratesLegacyLowerCaseName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "proj_0.jsonl"), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}

	file, err := OpenCacheFile(dir, "PROJ_0.jsonl")
	if err != nil {
		t.Fatalf("OpenCacheFile: %v", err)
	}
	defer file.Close()

	got, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "legacy" {
		t.Errorf("content = %q, want %q", got, "legacy")
	}

	// On a case-sensitive file system the legacy file must now carry the
	// canonical name. Case-insensitive ones (Windows, default macOS) open it
	// directly, so only the content is checked there.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries, want 1", len(entries))
	}
	if name := entries[0].Name(); name != "PROJ_0.jsonl" && name != "proj_0.jsonl" {
		t.Errorf("unexpected file %q", name)
	}
}

func TestOpenCacheFile_MissingIsNotExist(t *testing.T) {
	_, err := OpenCacheFile(t.TempDir(), "PROJ_0.jsonl")
	if !os.IsNotExist(err) {
		t.Errorf("err = %v, want not-exist", err)
	}
}
