package eventlog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// CachePath returns the path of the file name inside cacheDir. It refuses any
// name that would resolve outside cacheDir (such as "../x" or an absolute
// path), so a source ID can never address a file elsewhere on disk.
func CachePath(cacheDir, name string) (string, error) {
	if !filepath.IsLocal(name) {
		return "", fmt.Errorf("invalid cache file name %q", name)
	}
	return filepath.Join(cacheDir, name), nil
}

// OpenCacheFile opens the file name inside cacheDir for reading. When it does
// not exist but a file with the all-lower-case name does, that legacy file is
// renamed to name first. Older versions accepted lower-case project keys and
// named their cache files after them; project keys are upper case now.
func OpenCacheFile(cacheDir, name string) (*os.File, error) {
	path, err := CachePath(cacheDir, name)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if !errors.Is(err, fs.ErrNotExist) {
		return file, err
	}

	legacy := strings.ToLower(name)
	if legacy == name {
		return nil, err
	}
	legacyPath := filepath.Join(cacheDir, legacy)
	if _, statErr := os.Stat(legacyPath); statErr != nil {
		return nil, err
	}
	if renameErr := os.Rename(legacyPath, path); renameErr != nil {
		return nil, fmt.Errorf("failed to migrate legacy cache file %q: %w", legacy, renameErr)
	}
	log.Info().Str("from", legacy).Str("to", name).Msg("Migrated legacy lower-case cache file")
	return os.Open(path)
}
