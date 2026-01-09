package guac

import (
	"io"
	"os"

	"github.com/rs/zerolog"
)

var (
	// globalLogger is the package-level logger, isolated from the consuming application
	// By default, it's disabled to prevent logs from appearing unless explicitly configured
	globalLogger = createDefaultLogger()
)

// createDefaultLogger creates a logger that is disabled by default
// This ensures that logs from this package don't interfere with the consuming application
func createDefaultLogger() zerolog.Logger {
	// Create a logger with Disabled level by default so no logs appear
	// unless explicitly configured by the consuming application
	return zerolog.New(io.Discard).Level(zerolog.Disabled)
}

// SetLogger sets a custom zerolog logger for the guac package
// This allows consumers to control log output destination and formatting
// Example:
//
//	logger := zerolog.New(os.Stderr).With().Timestamp().Logger().Level(zerolog.InfoLevel)
//	guac.SetLogger(logger)
func SetLogger(l zerolog.Logger) {
	globalLogger = l
}

// SetLogLevel sets the log level for the guac package logger with JSON output
// This is a convenience method for consumers who want to use standard JSON logging.
// Common levels: zerolog.DebugLevel, zerolog.InfoLevel, zerolog.WarnLevel, zerolog.ErrorLevel
// Example:
//
//	guac.SetLogLevel(zerolog.InfoLevel)
func SetLogLevel(level zerolog.Level) {
	// Create a new logger with JSON output and the specified level
	globalLogger = zerolog.New(os.Stderr).
		With().
		Timestamp().
		Logger().
		Level(level)
}

// SetLogLevelConsole sets the log level for the guac package logger with pretty console output
// This is a convenience method for consumers who want human-readable console logs with colors.
// Common levels: zerolog.DebugLevel, zerolog.InfoLevel, zerolog.WarnLevel, zerolog.ErrorLevel
// Example:
//
//	guac.SetLogLevelConsole(zerolog.InfoLevel)
func SetLogLevelConsole(level zerolog.Level) {
	// Create a new logger with console output and the specified level
	globalLogger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		With().
		Timestamp().
		Logger().
		Level(level)
}

// GetLogger returns the current package logger
func GetLogger() zerolog.Logger {
	return globalLogger
}
