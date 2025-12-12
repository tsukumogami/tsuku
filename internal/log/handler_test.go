package log

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestNewCLIHandler(t *testing.T) {
	var buf bytes.Buffer
	h := NewCLIHandler(slog.LevelInfo, WithOutput(&buf))

	logger := slog.New(h)
	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "INFO") {
		t.Errorf("expected output to contain 'INFO', got: %s", output)
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("expected output to contain 'test message', got: %s", output)
	}
}

func TestCLIHandler_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	h := NewCLIHandler(slog.LevelWarn, WithOutput(&buf))
	logger := slog.New(h)

	logger.Debug("debug msg")
	logger.Info("info msg")
	logger.Warn("warn msg")
	logger.Error("error msg")

	output := buf.String()

	if strings.Contains(output, "debug msg") {
		t.Error("debug message should have been filtered")
	}
	if strings.Contains(output, "info msg") {
		t.Error("info message should have been filtered")
	}
	if !strings.Contains(output, "warn msg") {
		t.Error("warn message should appear")
	}
	if !strings.Contains(output, "error msg") {
		t.Error("error message should appear")
	}
}

func TestCLIHandler_TimestampOnlyAtDebug(t *testing.T) {
	tests := []struct {
		name          string
		level         slog.Level
		wantTimestamp bool
	}{
		{
			name:          "DEBUG level includes timestamp",
			level:         slog.LevelDebug,
			wantTimestamp: true,
		},
		{
			name:          "INFO level omits timestamp",
			level:         slog.LevelInfo,
			wantTimestamp: false,
		},
		{
			name:          "WARN level omits timestamp",
			level:         slog.LevelWarn,
			wantTimestamp: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := NewCLIHandler(tt.level, WithOutput(&buf))
			logger := slog.New(h)

			// Log at ERROR to ensure it's always shown
			logger.Error("test message")

			output := buf.String()

			// Check for ISO 8601 timestamp pattern (starts with year)
			hasTimestamp := strings.Contains(output, "202") && strings.Contains(output, "T")

			if tt.wantTimestamp && !hasTimestamp {
				t.Errorf("expected timestamp in output, got: %s", output)
			}
			if !tt.wantTimestamp && hasTimestamp {
				t.Errorf("expected no timestamp in output, got: %s", output)
			}
		})
	}
}

func TestCLIHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewCLIHandler(slog.LevelInfo, WithOutput(&buf))
	logger := slog.New(h).With("key1", "value1")

	logger.Info("test message", "key2", "value2")

	output := buf.String()
	if !strings.Contains(output, "key1=value1") {
		t.Errorf("expected 'key1=value1' in output, got: %s", output)
	}
	if !strings.Contains(output, "key2=value2") {
		t.Errorf("expected 'key2=value2' in output, got: %s", output)
	}
}

func TestCLIHandler_LevelStrings(t *testing.T) {
	tests := []struct {
		level    slog.Level
		expected string
	}{
		{slog.LevelDebug, "DEBUG"},
		{slog.LevelInfo, "INFO"},
		{slog.LevelWarn, "WARN"},
		{slog.LevelError, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			var buf bytes.Buffer
			h := NewCLIHandler(slog.LevelDebug, WithOutput(&buf))
			logger := slog.New(h)

			switch tt.level {
			case slog.LevelDebug:
				logger.Debug("msg")
			case slog.LevelInfo:
				logger.Info("msg")
			case slog.LevelWarn:
				logger.Warn("msg")
			case slog.LevelError:
				logger.Error("msg")
			}

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("expected level %q in output, got: %s", tt.expected, output)
			}
		})
	}
}

func TestCLIHandler_AttributeTypes(t *testing.T) {
	var buf bytes.Buffer
	h := NewCLIHandler(slog.LevelInfo, WithOutput(&buf))
	logger := slog.New(h)

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

func TestCLIHandler_QuotesStringsWithSpaces(t *testing.T) {
	var buf bytes.Buffer
	h := NewCLIHandler(slog.LevelInfo, WithOutput(&buf))
	logger := slog.New(h)

	logger.Info("test", "msg", "hello world")

	output := buf.String()
	if !strings.Contains(output, `msg="hello world"`) {
		t.Errorf("expected quoted value for string with spaces, got: %s", output)
	}
}

func TestCLIHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	h := NewCLIHandler(slog.LevelInfo, WithOutput(&buf))
	handlerWithGroup := h.WithGroup("mygroup")

	logger := slog.New(handlerWithGroup)
	logger.Info("test message")

	// The handler should still work with groups
	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected message in output, got: %s", output)
	}
}

func TestCLIHandler_Enabled(t *testing.T) {
	tests := []struct {
		handlerLevel slog.Level
		logLevel     slog.Level
		enabled      bool
	}{
		{slog.LevelDebug, slog.LevelDebug, true},
		{slog.LevelDebug, slog.LevelInfo, true},
		{slog.LevelInfo, slog.LevelDebug, false},
		{slog.LevelInfo, slog.LevelInfo, true},
		{slog.LevelWarn, slog.LevelInfo, false},
		{slog.LevelError, slog.LevelWarn, false},
	}

	for _, tt := range tests {
		var buf bytes.Buffer
		h := NewCLIHandler(tt.handlerLevel, WithOutput(&buf))
		if h.Enabled(context.Background(), tt.logLevel) != tt.enabled {
			t.Errorf("Enabled(%v, %v) = %v, want %v",
				tt.handlerLevel, tt.logLevel, !tt.enabled, tt.enabled)
		}
	}
}

func TestCLIHandler_OutputEndsWithNewline(t *testing.T) {
	var buf bytes.Buffer
	h := NewCLIHandler(slog.LevelInfo, WithOutput(&buf))
	logger := slog.New(h)

	logger.Info("test message")

	output := buf.String()
	if !strings.HasSuffix(output, "\n") {
		t.Errorf("output should end with newline, got: %q", output)
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    slog.Level
		expected string
	}{
		{slog.LevelDebug - 1, "DEBUG"},
		{slog.LevelDebug, "DEBUG"},
		{slog.LevelInfo - 1, "DEBUG"},
		{slog.LevelInfo, "INFO"},
		{slog.LevelWarn - 1, "INFO"},
		{slog.LevelWarn, "WARN"},
		{slog.LevelError - 1, "WARN"},
		{slog.LevelError, "ERROR"},
		{slog.LevelError + 1, "ERROR"},
	}

	for _, tt := range tests {
		result := levelString(tt.level)
		if result != tt.expected {
			t.Errorf("levelString(%v) = %q, want %q", tt.level, result, tt.expected)
		}
	}
}
