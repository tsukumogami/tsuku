// Package log provides structured logging for tsuku.
//
// This package defines a Logger interface backed by Go's stdlib slog,
// enabling testable logging throughout the codebase. Subsystems accept
// the Logger via functional options, with a global default for convenience.
//
// Output semantics:
//   - User output (stdout): Command results, progress, success messages
//   - Diagnostic logging (stderr): Debug, Info, Warn, Error messages
//
// Verbosity levels:
//   - ERROR (--quiet): Errors only
//   - WARN (default): Warnings and user output
//   - INFO (--verbose): Operational context
//   - DEBUG (--debug): Internal state and troubleshooting details
package log

import (
	"log/slog"
	"sync"
)

// Logger is the interface for structured logging.
// Methods match slog's signature for easy integration.
type Logger interface {
	// Debug logs at DEBUG level. Use for internal state, cache hits,
	// version resolution details - information only useful for troubleshooting.
	Debug(msg string, args ...any)

	// Info logs at INFO level. Use for operational context like
	// "Using cached asset" or "Connecting to registry".
	Info(msg string, args ...any)

	// Warn logs at WARN level. Use for recoverable issues like
	// "Checksum mismatch, re-downloading".
	Warn(msg string, args ...any)

	// Error logs at ERROR level. Use for failures that prevent
	// the operation from completing.
	Error(msg string, args ...any)

	// With returns a Logger with additional context attributes.
	// The returned Logger includes the given key-value pairs in all
	// subsequent log entries.
	With(args ...any) Logger
}

// slogLogger wraps slog.Logger to implement the Logger interface.
type slogLogger struct {
	l *slog.Logger
}

// New creates a Logger backed by slog with the given handler.
func New(h slog.Handler) Logger {
	return &slogLogger{l: slog.New(h)}
}

func (s *slogLogger) Debug(msg string, args ...any) {
	s.l.Debug(msg, args...)
}

func (s *slogLogger) Info(msg string, args ...any) {
	s.l.Info(msg, args...)
}

func (s *slogLogger) Warn(msg string, args ...any) {
	s.l.Warn(msg, args...)
}

func (s *slogLogger) Error(msg string, args ...any) {
	s.l.Error(msg, args...)
}

func (s *slogLogger) With(args ...any) Logger {
	return &slogLogger{l: s.l.With(args...)}
}

// noopLogger discards all log output.
type noopLogger struct{}

// NewNoop returns a logger that discards all output.
// Useful for testing or when logging should be disabled.
func NewNoop() Logger {
	return noopLogger{}
}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}
func (noopLogger) With(...any) Logger   { return noopLogger{} }

// defaultLogger is the global logger instance.
var (
	defaultLogger Logger = noopLogger{}
	defaultMu     sync.RWMutex
)

// Default returns the global logger configured at startup.
// Returns a noop logger if SetDefault has not been called.
func Default() Logger {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultLogger
}

// SetDefault sets the global logger.
// This should be called once during program initialization,
// typically in main() after parsing verbosity flags.
func SetDefault(l Logger) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultLogger = l
}
