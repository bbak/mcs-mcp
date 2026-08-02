// Package paths performs process bootstrap: it resolves the shared data
// directory and loads the .env files that populate the process environment,
// before logging and configuration are initialised.
package paths

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// Source identifies where the data path came from, for provenance logging.
type Source string

const (
	SourceFlag    Source = "flag"
	SourceEnv     Source = "env"
	SourceExeDir  Source = "exedir"
	SourceTempDir Source = "tempdir"
	SourceCwd     Source = "cwd"
)

// Options configures Bootstrap.
type Options struct {
	// DataPath is the --data-path flag value. Empty means unset.
	DataPath string
	// Template is the .env content written to the data path on first run.
	// Passed in rather than imported to keep paths free of a config dependency.
	Template []byte
}

// Result is the resolved state produced by Bootstrap.
type Result struct {
	DataPath string
	// DataPathSource records which layer supplied DataPath.
	DataPathSource Source
	// ExeDir is the binary's directory, empty if it could not be determined.
	ExeDir string
	// EnvKeys holds the process environment keys present before any .env file
	// was loaded. config uses it to distinguish "env" from "dotenv" provenance.
	EnvKeys map[string]bool
}

// Note is a log record produced during bootstrap. Bootstrap runs before the
// log sinks exist, so notes are returned and replayed by the caller once
// logging is initialised.
type Note struct {
	Level  string // "info" or "warn"
	Msg    string
	Fields map[string]string
}

// Bootstrap resolves the data path, loads the .env files and returns the
// resolved state along with notes to be replayed once logging is up.
//
// The data path is a bootstrap setting: it must be known before .env can be
// located, so it is taken from the --data-path flag or the DATA_PATH
// environment variable only. DATA_PATH inside a .env file is ignored and
// warned about.
//
// .env files are loaded in precedence order (data path, binary directory,
// working directory). godotenv.Load never overwrites an existing variable, so
// the process environment outranks every file and earlier files outrank later
// ones.
func Bootstrap(opts Options) (Result, []Note) {
	var notes []Note

	exeDir := ""
	if exePath, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exePath)
	}

	dataPath, source := resolveDataPath(opts.DataPath, exeDir, &notes)

	// Snapshot the environment before loading any .env file so that config can
	// tell an inherited variable from a dotenv-supplied one.
	envKeys := make(map[string]bool, len(os.Environ()))
	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok {
			envKeys[k] = true
		}
	}

	// The data path .env is the user-owned config file. Seed it on first run so
	// the documented defaults are visible and editable. Skipped for the cwd
	// last resort (do not litter) and when the environment already carries
	// credentials, which indicates a managed deployment rather than a desktop
	// install.
	dataEnv := filepath.Join(dataPath, ".env")
	if source != SourceCwd && len(opts.Template) > 0 && !credentialsInEnv(envKeys) {
		seedTemplate(dataEnv, opts.Template, &notes)
	}

	_ = godotenv.Load(dataEnv)
	if exeDir != "" && exeDir != dataPath {
		_ = godotenv.Load(filepath.Join(exeDir, ".env"))
	}
	_ = godotenv.Load()

	warnIfDotenvDataPath(dataPath, envKeys, &notes)

	return Result{
		DataPath:       dataPath,
		DataPathSource: source,
		ExeDir:         exeDir,
		EnvKeys:        envKeys,
	}, notes
}

// resolveDataPath returns a writable base directory for logs and cache using a
// fallback chain:
//  1. --data-path flag (explicit configuration)
//  2. DATA_PATH env var (explicit configuration)
//  3. exe directory (next to binary, if writable)
//  4. os.TempDir()/mcs-mcp (system temp, always writable)
//  5. current directory (last resort)
func resolveDataPath(flag, exeDir string, notes *[]Note) (string, Source) {
	// 1. Flag — trust it as-is (user's responsibility)
	if dp := strings.TrimSpace(flag); dp != "" {
		return dp, SourceFlag
	}

	// 2. Explicit DATA_PATH — trust it as-is (user's responsibility)
	if dp := strings.TrimSpace(os.Getenv("DATA_PATH")); dp != "" {
		return dp, SourceEnv
	}

	// 3. Exe directory — only if we can create subdirectories
	if exeDir != "" {
		probe := filepath.Join(exeDir, ".mcs-mcp-probe")
		if err := os.MkdirAll(probe, 0755); err == nil {
			os.Remove(probe)
			return exeDir, SourceExeDir
		}
	}

	// 4. System temp directory
	tmp := filepath.Join(os.TempDir(), "mcs-mcp")
	if err := os.MkdirAll(tmp, 0755); err == nil {
		*notes = append(*notes, Note{
			Level:  "info",
			Msg:    "Using temp directory as data path (binary location is not writable)",
			Fields: map[string]string{"path": tmp},
		})
		return tmp, SourceTempDir
	}

	// 5. Last resort — current directory
	*notes = append(*notes, Note{
		Level: "warn",
		Msg:   "All data path candidates failed; falling back to current directory",
	})
	return ".", SourceCwd
}

// credentialsInEnv reports whether the process environment already supplies
// Jira credentials, which marks a managed deployment (MCPB, container,
// service manager) where writing a config file would be inappropriate.
func credentialsInEnv(envKeys map[string]bool) bool {
	return envKeys["JIRA_TOKEN"] || envKeys["JIRA_SESSION_ID"]
}

// seedTemplate writes the .env template unless the file already exists. The
// O_EXCL create is atomic, so a concurrent start cannot clobber user edits.
// Mode 0600 because users paste a Jira token into this file.
func seedTemplate(path string, template []byte, notes *[]Note) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if !os.IsExist(err) {
			*notes = append(*notes, Note{
				Level:  "warn",
				Msg:    "Could not write configuration template",
				Fields: map[string]string{"path": path, "error": err.Error()},
			})
		}
		return
	}
	defer f.Close()

	if _, err := f.Write(template); err != nil {
		*notes = append(*notes, Note{
			Level:  "warn",
			Msg:    "Could not write configuration template",
			Fields: map[string]string{"path": path, "error": err.Error()},
		})
		return
	}

	*notes = append(*notes, Note{
		Level:  "info",
		Msg:    "Wrote configuration template",
		Fields: map[string]string{"path": path},
	})
}

// warnIfDotenvDataPath reports a DATA_PATH supplied by a .env file. Such a
// value cannot take effect: the data path must be known before .env can be
// located. Escalated when the ignored directory holds an existing cache, since
// that user would otherwise silently re-ingest from Jira.
func warnIfDotenvDataPath(effective string, envKeys map[string]bool, notes *[]Note) {
	if envKeys["DATA_PATH"] {
		return // came from the process environment, already honoured
	}
	ignored := strings.TrimSpace(os.Getenv("DATA_PATH"))
	if ignored == "" || ignored == effective {
		return
	}

	msg := "DATA_PATH set in .env is ignored; pass --data-path or set it as a process environment variable"
	if _, err := os.Stat(filepath.Join(ignored, "cache")); err == nil {
		msg = "DATA_PATH set in .env is ignored and that directory holds an existing cache; " +
			"pass --data-path or set DATA_PATH as a process environment variable to keep using it, " +
			"otherwise Jira will be re-ingested from scratch"
	}

	*notes = append(*notes, Note{
		Level:  "warn",
		Msg:    msg,
		Fields: map[string]string{"dotenv_value": ignored, "effective": effective},
	})
}
