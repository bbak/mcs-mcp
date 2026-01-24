package logging

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Init initializes the global logger with dual sinks: os.Stderr and a rotating file.
func Init(verbose bool) {
	// 1. Determine log level
	level := zerolog.InfoLevel
	if verbose {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)

	// 2. Setup Stderr Writer (Console)
	var consoleWriter io.Writer = zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	}

	// 3. Setup File Writer (Rotating)
	logDir := "logs"
	logFile := filepath.Join(logDir, "mcs-mcp.log")

	// Ensure log directory exists
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		_ = os.MkdirAll(logDir, 0755)
	}

	fileWriter := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    16,  // megabytes
		MaxBackups: 32,
		MaxAge:     365, // days
		Compress:   true,
	}

	// 4. Combine Writers
	multi := zerolog.MultiLevelWriter(consoleWriter, fileWriter)

	// 5. Set Global Logger
	log.Logger = zerolog.New(multi).
		With().
		Timestamp().
		Logger()
}
