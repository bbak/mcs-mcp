package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Init initializes the global logger with dual sinks: os.Stderr and a rotating file.
func Init(verbose bool) {
	// 0. Load .env from binary directory to ensure LOGS_FOLDER is available.
	// We do this here because Init is called before config.Load.
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		_ = godotenv.Load(filepath.Join(exeDir, ".env"))
	}

	// 1. Determine log level
	level := zerolog.InfoLevel
	if verbose {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)

	// 2. Setup Stderr Writer (Console)
	isTerminal := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		NoColor:    !isTerminal,
	}

	// 3. Setup File Writer (Rotating)
	logDir := os.Getenv("LOGS_FOLDER")
	if logDir == "" {
		if err == nil {
			logDir = filepath.Join(filepath.Dir(exePath), "logs")
		} else {
			logDir = "logs"
		}
	}

	// Ensure log directory exists and is writable
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create log directory %q: %v\n", logDir, err)
		os.Exit(1)
	}

	// Check if we can write to the directory by creating a temp file or just stat'ing it
	// MkdirAll success is a good indicator, but let's be sure.
	testFile := filepath.Join(logDir, ".write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: log directory %q is not writable: %v\n", logDir, err)
		os.Exit(1)
	}
	_ = os.Remove(testFile)

	logFile := filepath.Join(logDir, "mcs-mcp.log")

	fileWriter := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    16,  // megabytes
		MaxBackups: 32,
		MaxAge:     365, // days
		Compress:   true,
	}

	// 4. Combine Writers
	multi := zerolog.MultiLevelWriter(io.Writer(consoleWriter), fileWriter)

	// 5. Set Global Logger
	log.Logger = zerolog.New(multi).
		With().
		Timestamp().
		Logger()
}
