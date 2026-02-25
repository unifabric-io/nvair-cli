package logging

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Logger provides structured logging with support for verbose mode.
type Logger struct {
	mu      sync.RWMutex
	verbose io.Writer
	quiet   io.Writer
}

// Global logger instance
var globalLogger = &Logger{
	verbose: io.Discard,
	quiet:   os.Stderr,
}

// SetVerbose enables verbose logging to the given writer.
func SetVerbose(w io.Writer) {
	globalLogger.mu.Lock()
	defer globalLogger.mu.Unlock()
	globalLogger.verbose = w
}

// DisableVerbose disables verbose logging.
func DisableVerbose() {
	globalLogger.mu.Lock()
	defer globalLogger.mu.Unlock()
	globalLogger.verbose = io.Discard
}

// Verbose logs a message in verbose mode (shown when --verbose is set).
func Verbose(format string, args ...interface{}) {
	globalLogger.mu.RLock()
	defer globalLogger.mu.RUnlock()
	fmt.Fprintf(globalLogger.verbose, "[DEBUG] [%s] %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		fmt.Sprintf(format, args...))
}

// Info logs an informational message (always shown).
func Info(format string, args ...interface{}) {
	globalLogger.mu.RLock()
	defer globalLogger.mu.RUnlock()
	fmt.Fprintf(globalLogger.quiet, "%s\n", fmt.Sprintf(format, args...))
}

// Warn logs a warning message (always shown).
func Warn(format string, args ...interface{}) {
	globalLogger.mu.RLock()
	defer globalLogger.mu.RUnlock()
	fmt.Fprintf(globalLogger.quiet, "[WARN] %s\n", fmt.Sprintf(format, args...))
}

// Error logs an error message (always shown).
func Error(format string, args ...interface{}) {
	globalLogger.mu.RLock()
	defer globalLogger.mu.RUnlock()
	fmt.Fprintf(globalLogger.quiet, "[ERROR] %s\n", fmt.Sprintf(format, args...))
}
