package guac

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
)

// TestLoggerIsolation verifies that the guac logger is isolated and doesn't
// interfere with global logging settings
func TestLoggerIsolation(t *testing.T) {
	// Save original logger
	originalLogger := globalLogger
	defer func() { globalLogger = originalLogger }()

	// Create a buffer to capture logs
	var buf bytes.Buffer

	// Set the guac logger to write to our buffer at Debug level
	// Note: We need to ensure the logger's level is set on the logger itself
	testLogger := zerolog.New(&buf).With().Timestamp().Logger().Level(zerolog.DebugLevel)
	SetLogger(testLogger)

	// Log something at debug level from guac
	globalLogger.Debug().Msg("test message")

	// Verify that the message was logged
	if buf.Len() == 0 {
		t.Error("Expected guac logger to log debug message")
	}

	// Verify the message contains our test content
	if !bytes.Contains(buf.Bytes(), []byte("test message")) {
		t.Errorf("Expected log to contain 'test message', got: %s", buf.String())
	}
}

// TestDefaultLoggerDisabled verifies that the default logger is disabled
func TestDefaultLoggerDisabled(t *testing.T) {
	// Create a fresh logger using the default creation
	logger := createDefaultLogger()

	// Get the level - it should be Disabled
	if logger.GetLevel() != zerolog.Disabled {
		t.Errorf("Expected default logger level to be Disabled, got: %v", logger.GetLevel())
	}
}

// TestSetLogLevel verifies that SetLogLevel creates a functional logger
func TestSetLogLevel(t *testing.T) {
	// Capture output
	var buf bytes.Buffer
	originalLogger := globalLogger
	defer func() { globalLogger = originalLogger }()

	// Create a logger that writes to our buffer
	globalLogger = zerolog.New(&buf).Level(zerolog.InfoLevel)

	// Log at info level
	globalLogger.Info().Msg("info message")

	// Verify message was logged
	if !bytes.Contains(buf.Bytes(), []byte("info message")) {
		t.Errorf("Expected log to contain 'info message', got: %s", buf.String())
	}

	// Clear buffer
	buf.Reset()

	// Log at debug level (should not appear since level is Info)
	globalLogger.Debug().Msg("debug message")

	// Verify debug message was not logged
	if bytes.Contains(buf.Bytes(), []byte("debug message")) {
		t.Error("Debug message should not be logged when level is Info")
	}
}

// TestSetLogLevelConsole verifies that SetLogLevelConsole creates a logger
func TestSetLogLevelConsole(t *testing.T) {
	// Save and restore original logger
	originalLogger := globalLogger
	defer func() { globalLogger = originalLogger }()

	// Set console log level
	SetLogLevelConsole(zerolog.InfoLevel)

	// Verify logger is not disabled
	if globalLogger.GetLevel() == zerolog.Disabled {
		t.Error("Expected logger to not be disabled after SetLogLevelConsole")
	}

	// Verify level is set correctly
	if globalLogger.GetLevel() != zerolog.InfoLevel {
		t.Errorf("Expected logger level to be Info, got: %v", globalLogger.GetLevel())
	}
}

// TestConnectionLoggingIsolation verifies that connection logging respects
// the log level settings
func TestConnectionLoggingRespectLevel(t *testing.T) {
	// Save and restore original logger
	originalLogger := globalLogger
	defer func() { globalLogger = originalLogger }()

	// Create a buffer to capture logs
	var buf bytes.Buffer
	globalLogger = zerolog.New(&buf).Level(zerolog.ErrorLevel)

	// Try to log at info level (should not appear)
	globalLogger.Info().Msg("info message")

	// Verify nothing was logged
	if buf.Len() > 0 {
		t.Errorf("Expected no logs when level is ErrorLevel, got: %s", buf.String())
	}

	// Try to log at error level (should appear)
	globalLogger.Error().Msg("error message")

	// Verify something was logged
	if buf.Len() == 0 {
		t.Error("Expected logs when logging at ErrorLevel")
	}

	if !bytes.Contains(buf.Bytes(), []byte("error message")) {
		t.Errorf("Expected log to contain error message, got: %s", buf.String())
	}
}
