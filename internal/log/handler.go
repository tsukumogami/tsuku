package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// HandlerOption configures a CLIHandler.
type HandlerOption func(*cliHandler)

// WithOutput sets the output writer for the handler.
// Defaults to os.Stderr.
func WithOutput(w io.Writer) HandlerOption {
	return func(h *cliHandler) {
		h.out = w
	}
}

// cliHandler is a slog.Handler that formats output for CLI use.
// It writes human-readable output to stderr (by default).
type cliHandler struct {
	level  slog.Level
	out    io.Writer
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

// NewCLIHandler creates a slog.Handler for CLI output.
//   - Writes to stderr by default
//   - Human-readable format (not JSON)
//   - Includes timestamp only at DEBUG level
//   - Omits source location unless DEBUG level
func NewCLIHandler(level slog.Level, opts ...HandlerOption) slog.Handler {
	h := &cliHandler{
		level: level,
		out:   os.Stderr,
		mu:    &sync.Mutex{},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Enabled reports whether the handler handles records at the given level.
func (h *cliHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle handles the Record.
func (h *cliHandler) Handle(_ context.Context, r slog.Record) error {
	var buf strings.Builder

	// Timestamp only at DEBUG level
	if h.level <= slog.LevelDebug {
		buf.WriteString(r.Time.Format(time.RFC3339))
		buf.WriteString(" ")
	}

	// Level
	buf.WriteString(levelString(r.Level))
	buf.WriteString(" ")

	// Message
	buf.WriteString(r.Message)

	// Attributes from WithAttrs
	for _, attr := range h.attrs {
		writeAttr(&buf, attr)
	}

	// Attributes from record
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&buf, a)
		return true
	})

	// Source location only at DEBUG level
	if h.level <= slog.LevelDebug && r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		if f.File != "" {
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("[%s:%d]", f.File, f.Line))
		}
	}

	buf.WriteString("\n")

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.out, buf.String())
	return err
}

// WithAttrs returns a new Handler with the given attributes added.
func (h *cliHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &cliHandler{
		level:  h.level,
		out:    h.out,
		attrs:  newAttrs,
		groups: h.groups,
		mu:     h.mu,
	}
}

// WithGroup returns a new Handler with the given group name prepended.
func (h *cliHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name
	return &cliHandler{
		level:  h.level,
		out:    h.out,
		attrs:  h.attrs,
		groups: newGroups,
		mu:     h.mu,
	}
}

// levelString returns a compact level representation.
func levelString(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "DEBUG"
	case level < slog.LevelWarn:
		return "INFO"
	case level < slog.LevelError:
		return "WARN"
	default:
		return "ERROR"
	}
}

// writeAttr writes a single attribute in key=value format.
func writeAttr(buf *strings.Builder, a slog.Attr) {
	if a.Key == "" {
		return
	}
	buf.WriteString(" ")
	buf.WriteString(a.Key)
	buf.WriteString("=")
	buf.WriteString(formatValue(a.Value))
}

// formatValue formats an slog.Value for display.
func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		// Quote if contains spaces or special characters
		if strings.ContainsAny(s, " \t\n\"") {
			return fmt.Sprintf("%q", s)
		}
		return s
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindGroup:
		// For groups, format each attr
		var buf strings.Builder
		buf.WriteString("{")
		attrs := v.Group()
		for i, attr := range attrs {
			if i > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString(attr.Key)
			buf.WriteString("=")
			buf.WriteString(formatValue(attr.Value))
		}
		buf.WriteString("}")
		return buf.String()
	default:
		return v.String()
	}
}
