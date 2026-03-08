package logging

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"mcs-mcp/internal/paths"

	"github.com/joho/godotenv"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Init initializes the global logger with dual sinks: os.Stderr and a rotating file.
func Init() {
	// 0. Load .env from binary directory to ensure LOGS_FOLDER is available.
	// We do this here because Init is called before config.Load.
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		_ = godotenv.Load(filepath.Join(exeDir, ".env"))
	}

	// 1. Determine log level
	level := zerolog.InfoLevel
	if os.Getenv("VERBOSE") == "true" {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)

	// 2. Setup Stderr Writer (Console)
	isTerminal := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339Nano,
		NoColor:    !isTerminal,
	}

	// 3. Setup File Writer (Rotating) using shared data path resolution
	var exeDir string
	if err == nil {
		exeDir = filepath.Dir(exePath)
	}
	dataPath := paths.ResolveDataPath(exeDir)
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
