package log

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := New(h)

	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected output to contain 'key=value', got: %s", output)
	}
}

func TestLoggerLevels(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(Logger)
		level    slog.Level
		contains string
	}{
		{
			name:     "Debug",
			logFunc:  func(l Logger) { l.Debug("debug msg") },
			level:    slog.LevelDebug,
			contains: "debug msg",
		},
		{
			name:     "Info",
			logFunc:  func(l Logger) { l.Info("info msg") },
			level:    slog.LevelInfo,
			contains: "info msg",
		},
		{
			name:     "Warn",
			logFunc:  func(l Logger) { l.Warn("warn msg") },
			level:    slog.LevelWarn,
			contains: "warn msg",
		},
		{
			name:     "Error",
			logFunc:  func(l Logger) { l.Error("error msg") },
			level:    slog.LevelError,
			contains: "error msg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
			logger := New(h)

			tt.logFunc(logger)

			output := buf.String()
			if !strings.Contains(output, tt.contains) {
				t.Errorf("expected output to contain %q, got: %s", tt.contains, output)
			}
			if !strings.Contains(output, strings.ToUpper(tt.name)) {
				t.Errorf("expected output to contain level %q, got: %s", tt.name, output)
			}
		})
	}
}

func TestLoggerWith(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := New(h)

	// Create child logger with context
	childLogger := logger.With("tool", "gh", "version", "2.0.0")
	childLogger.Info("installing tool")

	output := buf.String()
	if !strings.Contains(output, "tool=gh") {
		t.Errorf("expected output to contain 'tool=gh', got: %s", output)
	}
	if !strings.Contains(output, "version=2.0.0") {
		t.Errorf("expected output to contain 'version=2.0.0', got: %s", output)
	}
	if !strings.Contains(output, "installing tool") {
		t.Errorf("expected output to contain 'installing tool', got: %s", output)
	}
}

func TestLoggerWithChaining(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := New(h)

	// Chain multiple With calls
	childLogger := logger.With("tool", "gh").With("action", "download")
	childLogger.Debug("starting")

	output := buf.String()
	if !strings.Contains(output, "tool=gh") {
		t.Errorf("expected output to contain 'tool=gh', got: %s", output)
	}
	if !strings.Contains(output, "action=download") {
		t.Errorf("expected output to contain 'action=download', got: %s", output)
	}
}

func TestNewNoop(t *testing.T) {
	logger := NewNoop()

	// These should not panic
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	// With should return a noop logger
	child := logger.With("key", "value")
	child.Info("should not panic")
}

func TestNoopLoggerWith(t *testing.T) {
	logger := NewNoop()

	// With on noop should return noop
	child := logger.With("key", "value")

	// Verify it's still a noop by checking type
	_, ok := child.(noopLogger)
	if !ok {
		t.Error("expected With() on noopLogger to return noopLogger")
	}
}

func TestDefaultLogger(t *testing.T) {
	// Save original default
	original := Default()
	defer SetDefault(original)

	// Default should work (initially noop)
	Default().Info("should not panic")

	// Set a custom default
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	customLogger := New(h)
	SetDefault(customLogger)

	// Verify Default() returns the custom logger
	Default().Info("custom logger message")

	output := buf.String()
	if !strings.Contains(output, "custom logger message") {
		t.Errorf("expected custom logger to be used, got: %s", output)
	}
}

func TestDefaultLoggerConcurrency(t *testing.T) {
	// Save original default
	original := Default()
	defer SetDefault(original)

	// Run concurrent reads and writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				Default().Info("concurrent read")
			}
			done <- true
		}()
		go func() {
			for j := 0; j < 100; j++ {
				SetDefault(NewNoop())
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestLevelFiltering(t *testing.T) {
	// Test that setting handler level filters lower-level messages
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := New(h)

	logger.Debug("debug - should not appear")
	logger.Info("info - should not appear")
	logger.Warn("warn - should appear")
	logger.Error("error - should appear")

	output := buf.String()

	if strings.Contains(output, "debug - should not appear") {
		t.Error("debug message should have been filtered")
	}
	if strings.Contains(output, "info - should not appear") {
		t.Error("info message should have been filtered")
	}
	if !strings.Contains(output, "warn - should appear") {
		t.Errorf("warn message should appear, got: %s", output)
	}
	if !strings.Contains(output, "error - should appear") {
		t.Errorf("error message should appear, got: %s", output)
	}
}

func TestLoggerWithKeyValuePairs(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := New(h)

	// Test various key-value types
	logger.Info("test",
		"string", "value",
		"int", 42,
		"bool", true,
		"float", 3.14,
	)

	output := buf.String()
	if !strings.Contains(output, "string=value") {
		t.Errorf("expected 'string=value' in output: %s", output)
	}
	if !strings.Contains(output, "int=42") {
		t.Errorf("expected 'int=42' in output: %s", output)
	}
	if !strings.Contains(output, "bool=true") {
		t.Errorf("expected 'bool=true' in output: %s", output)
	}
}
