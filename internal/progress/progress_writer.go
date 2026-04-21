package progress

import (
	"io"
	"os"

	"golang.org/x/term"
)

const smallFileThreshold = int64(100 * 1024) // 100 KiB

// IsTerminalFunc is the function used to check if a file descriptor is a terminal.
// It can be overridden in tests.
var IsTerminalFunc = term.IsTerminal

// ShouldShowProgress reports whether progress should be displayed.
// Progress is shown when stdout is a terminal.
func ShouldShowProgress() bool {
	return IsTerminalFunc(int(os.Stdout.Fd()))
}

// ProgressWriter wraps an io.Writer and reports transfer progress via a callback.
// Create one per download; call Reset() before each retry attempt.
type ProgressWriter struct {
	w        io.Writer
	total    int64
	written  int64
	callback func(written, total int64)
}

// NewProgressWriter creates a ProgressWriter that delegates writes to w and
// calls callback after each Write with the running totals.
//
//   - If total <= 0, the download size is unknown; callback receives 0 for total.
//   - If 0 < total < smallFileThreshold, the callback is suppressed entirely so
//     no Status calls are made for small files.
func NewProgressWriter(w io.Writer, total int64, callback func(written, total int64)) *ProgressWriter {
	cb := callback
	if total > 0 && total < smallFileThreshold {
		cb = nil
	}
	return &ProgressWriter{
		w:        w,
		total:    total,
		callback: cb,
	}
}

// Write implements io.Writer. It delegates to the underlying writer, increments
// the written counter, and fires the callback.
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	if n > 0 {
		pw.written += int64(n)
		if pw.callback != nil {
			pw.callback(pw.written, pw.total)
		}
	}
	return n, err
}

// Reset clears the written counter. Call this before each retry attempt so the
// percentage display restarts from zero on the new response body.
func (pw *ProgressWriter) Reset() {
	pw.written = 0
}
