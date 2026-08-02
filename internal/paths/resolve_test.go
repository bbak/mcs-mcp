package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate keeps a developer's own environment out of the test, and undoes the
// leakage Bootstrap itself causes: godotenv writes into the process
// environment, so a .env loaded by one test would otherwise be visible to the
// next one in the same binary.
func isolate(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range append([]string{"DATA_PATH", "JIRA_TOKEN"}, keys...) {
		t.Setenv(k, "") // registers the restore
		os.Unsetenv(k)
	}
	t.Chdir(t.TempDir())
}

func TestDataPathPrecedence(t *testing.T) {
	t.Run("flag wins over env", func(t *testing.T) {
		isolate(t, "MCS_ENGINE", "INGESTION_MAX_ITEMS")
		flagDir, envDir := t.TempDir(), t.TempDir()
		t.Setenv("DATA_PATH", envDir)

		boot, _ := Bootstrap(Options{DataPath: flagDir})
		if boot.DataPath != flagDir {
			t.Errorf("DataPath = %q, want the flag value %q", boot.DataPath, flagDir)
		}
		if boot.DataPathSource != SourceFlag {
			t.Errorf("source = %q, want %q", boot.DataPathSource, SourceFlag)
		}
	})

	t.Run("env wins over fallback chain", func(t *testing.T) {
		isolate(t, "MCS_ENGINE", "INGESTION_MAX_ITEMS")
		envDir := t.TempDir()
		t.Setenv("DATA_PATH", envDir)

		boot, _ := Bootstrap(Options{})
		if boot.DataPath != envDir {
			t.Errorf("DataPath = %q, want %q", boot.DataPath, envDir)
		}
		if boot.DataPathSource != SourceEnv {
			t.Errorf("source = %q, want %q", boot.DataPathSource, SourceEnv)
		}
	})

	t.Run("values are trimmed", func(t *testing.T) {
		isolate(t, "MCS_ENGINE", "INGESTION_MAX_ITEMS")
		dir := t.TempDir()

		boot, _ := Bootstrap(Options{DataPath: "  " + dir + "  "})
		if boot.DataPath != dir {
			t.Errorf("DataPath = %q, want trimmed %q", boot.DataPath, dir)
		}
	})
}

func TestSeedTemplate(t *testing.T) {
	t.Run("writes once and never overwrites", func(t *testing.T) {
		isolate(t, "MCS_ENGINE", "INGESTION_MAX_ITEMS")
		dir := t.TempDir()
		template := []byte("# generated\nMCS_ENGINE=crude\n")

		if _, notes := Bootstrap(Options{DataPath: dir, Template: template}); !hasNote(notes, "Wrote configuration template") {
			t.Error("expected a note recording the template write")
		}

		written, err := os.ReadFile(filepath.Join(dir, ".env"))
		if err != nil {
			t.Fatalf("template not written: %v", err)
		}
		if string(written) != string(template) {
			t.Errorf("written template = %q, want %q", written, template)
		}

		// A second run must leave the user's edits alone.
		edited := []byte("MCS_ENGINE=bbak\n")
		if err := os.WriteFile(filepath.Join(dir, ".env"), edited, 0600); err != nil {
			t.Fatal(err)
		}
		Bootstrap(Options{DataPath: dir, Template: template})

		after, err := os.ReadFile(filepath.Join(dir, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(edited) {
			t.Errorf("existing .env was overwritten: got %q, want %q", after, edited)
		}
	})

	t.Run("skipped when credentials come from the environment", func(t *testing.T) {
		isolate(t, "MCS_ENGINE", "INGESTION_MAX_ITEMS")
		dir := t.TempDir()
		t.Setenv("JIRA_TOKEN", "from-the-environment")

		Bootstrap(Options{DataPath: dir, Template: []byte("# generated\n")})

		if _, err := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(err) {
			t.Error("a managed deployment must not have config written into its data directory")
		}
	})
}

func TestDotenvDataPathIsIgnored(t *testing.T) {
	isolate(t, "MCS_ENGINE", "INGESTION_MAX_ITEMS")
	dir, ignored := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DATA_PATH="+ignored+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	boot, notes := Bootstrap(Options{DataPath: dir})

	if boot.DataPath != dir {
		t.Errorf("DataPath = %q, want %q; a .env must not relocate the data path", boot.DataPath, dir)
	}
	if !hasNote(notes, "DATA_PATH set in .env is ignored") {
		t.Error("expected a warning that the .env DATA_PATH was ignored")
	}
}

func TestDotenvDataPathWithCacheEscalates(t *testing.T) {
	isolate(t, "MCS_ENGINE", "INGESTION_MAX_ITEMS")
	dir, ignored := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(ignored, "cache"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DATA_PATH="+ignored+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, notes := Bootstrap(Options{DataPath: dir})

	if !hasNote(notes, "re-ingested from scratch") {
		t.Error("an ignored DATA_PATH holding an existing cache must warn about re-ingestion")
	}
}

func TestEnvSnapshotSeparatesEnvFromDotenv(t *testing.T) {
	isolate(t, "MCS_ENGINE", "INGESTION_MAX_ITEMS")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("MCS_ENGINE=bbak\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INGESTION_MAX_ITEMS", "123")

	boot, _ := Bootstrap(Options{DataPath: dir})

	if !boot.EnvKeys["INGESTION_MAX_ITEMS"] {
		t.Error("a process environment variable must be recorded in the snapshot")
	}
	if boot.EnvKeys["MCS_ENGINE"] {
		t.Error("a dotenv-supplied variable must not appear in the pre-load snapshot")
	}
}

func hasNote(notes []Note, substring string) bool {
	for _, n := range notes {
		if strings.Contains(n.Msg, substring) {
			return true
		}
	}
	return false
}
