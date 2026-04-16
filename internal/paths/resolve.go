// Package paths resolves the shared data directory via a fallback chain:
// explicit DATA_PATH env var, executable directory, system temp, or current
// working directory.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveDataPath returns a writable base directory for logs and cache using a fallback chain:
//  1. DATA_PATH env var (explicit configuration)
//  2. exe directory (next to binary, if writable)
//  3. os.TempDir()/mcs-mcp (system temp, always writable)
//  4. current directory (last resort)
//
// Both logging.Init and config.Load call this with the same exeDir so that
// logs and cache always co-locate under the same root.
func ResolveDataPath(exeDir string) string {
	// 1. Explicit DATA_PATH — trust it as-is (user's responsibility)
	if dp := os.Getenv("DATA_PATH"); dp != "" {
		return dp
	}

	// 2. Exe directory — only if we can create subdirectories
	if exeDir != "" {
		probe := filepath.Join(exeDir, ".mcs-mcp-probe")
		if err := os.MkdirAll(probe, 0755); err == nil {
			os.Remove(probe)
			return exeDir
		}
	}

	// 3. System temp directory
	tmp := filepath.Join(os.TempDir(), "mcs-mcp")
	if err := os.MkdirAll(tmp, 0755); err == nil {
		fmt.Fprintf(os.Stderr, "mcs-mcp: using temp directory %q as data path (binary location is not writable)\n", tmp)
		return tmp
	}

	// 4. Last resort — current directory
	fmt.Fprintf(os.Stderr, "mcs-mcp: all data path candidates failed; falling back to current directory\n")
	return "."
}
