// Package logging initialises zerolog with a dual sink: human-readable
// console output on stderr and a rotating file sink under the shared data
// path.
package logging

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// verboseLevel maps the VERBOSE setting to a log level. Anything unparseable
// counts as off, matching the previous strict comparison, but "1", "TRUE" and
// the other forms strconv accepts now work as users expect.
func verboseLevel(raw string) zerolog.Level {
	if verbose, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil && verbose {
		return zerolog.DebugLevel
	}
	return zerolog.InfoLevel
}

// Init initializes the global logger with dual sinks: os.Stderr and a rotating
// file under dataPath. The caller resolves dataPath via paths.Bootstrap, which
// also loads the .env files, so VERBOSE is already in the environment here.
func Init(dataPath string) {
	// 1. Determine log level
	zerolog.SetGlobalLevel(verboseLevel(os.Getenv("VERBOSE")))

	// 2. Setup Stderr Writer (Console)
	isTerminal := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339Nano,
		NoColor:    !isTerminal,
	}

	// 3. Setup File Writer (Rotating) under the resolved data path
	logDir := filepath.Join(dataPath, "logs")

	var writers []io.Writer
	writers = append(writers, io.Writer(consoleWriter))

	if err := os.MkdirAll(logDir, 0755); err == nil {
		logFile := filepath.Join(logDir, "mcs-mcp.log")
		fileWriter := &lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    16, // megabytes
			MaxBackups: 32,
			MaxAge:     365, // days
			Compress:   true,
		}
		writers = append(writers, fileWriter)
	}

	// 4. Combine Writers
	multi := zerolog.MultiLevelWriter(writers...)

	// 5. Set Global Logger
	log.Logger = zerolog.New(multi).
		With().
		Timestamp().
		Logger()

	if len(writers) == 1 {
		log.Warn().Msg("File logging disabled: could not create log directory")
	} else {
		log.Info().Str("dir", logDir).Msg("Logging initialized")
	}
}
