package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// IsTerminalFunc is the function used to check if a file descriptor is a terminal.
// It can be overridden for testing.
var IsTerminalFunc = term.IsTerminal

// Writer wraps an io.Writer with progress tracking and display
type Writer struct {
	writer    io.Writer
	output    io.Writer
	total     int64
	written   int64
	startTime time.Time
	lastPrint time.Time
	mu        sync.Mutex
}

// NewWriter creates a progress writer that displays download progress.
// If total is <= 0, no percentage or ETA can be calculated.
func NewWriter(w io.Writer, total int64, output io.Writer) *Writer {
	return &Writer{
		writer:    w,
		output:    output,
		total:     total,
		startTime: time.Now(),
	}
}

// Write implements io.Writer and updates progress display
func (pw *Writer) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if n > 0 {
		pw.mu.Lock()
		pw.written += int64(n)
		pw.printProgress()
		pw.mu.Unlock()
	}
	return n, err
}

// Finish clears the progress line and prints final status
func (pw *Writer) Finish() {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	// Clear the progress line
	fmt.Fprintf(pw.output, "\r%s\r", strings.Repeat(" ", 80))
}

// printProgress displays the current progress
func (pw *Writer) printProgress() {
	// Rate limit updates to avoid flickering (max 10 updates per second)
	now := time.Now()
	if now.Sub(pw.lastPrint) < 100*time.Millisecond {
		return
	}
	pw.lastPrint = now

	elapsed := now.Sub(pw.startTime).Seconds()
	if elapsed < 0.1 {
		return // Wait a bit before showing progress
	}

	// Calculate speed (bytes per second)
	speed := float64(pw.written) / elapsed

	// Build progress string
	var line string
	if pw.total > 0 {
		// Calculate percentage
		percent := float64(pw.written) / float64(pw.total) * 100
		if percent > 100 {
			percent = 100
		}

		// Calculate ETA
		var etaStr string
		if speed > 0 {
			remaining := float64(pw.total-pw.written) / speed
			if remaining < 0 {
				remaining = 0
			}
			etaStr = formatDuration(remaining)
		} else {
			etaStr = "--:--"
		}

		// Build progress bar
		barWidth := 30
		filled := int(percent / 100 * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
		bar := strings.Repeat("=", filled)
		if filled < barWidth {
			bar += ">"
			bar += strings.Repeat(" ", barWidth-filled-1)
		}

		line = fmt.Sprintf("\r   [%s] %3.0f%% (%s/%s) %s/s ETA: %s",
			bar,
			percent,
			formatBytes(pw.written),
			formatBytes(pw.total),
			formatBytes(int64(speed)),
			etaStr,
		)
	} else {
		// Unknown total - just show downloaded and speed
		line = fmt.Sprintf("\r   Downloaded: %s (%s/s)",
			formatBytes(pw.written),
			formatBytes(int64(speed)),
		)
	}

	// Pad with spaces to clear any remaining characters from previous line
	if len(line) < 80 {
		line += strings.Repeat(" ", 80-len(line))
	}
	_, _ = fmt.Fprint(pw.output, line)
}

// formatBytes formats bytes into human-readable format
func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case b >= GB:
		return fmt.Sprintf("%.1fGB", float64(b)/GB)
	case b >= MB:
		return fmt.Sprintf("%.1fMB", float64(b)/MB)
	case b >= KB:
		return fmt.Sprintf("%.1fKB", float64(b)/KB)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// formatDuration formats seconds into MM:SS or HH:MM:SS format
func formatDuration(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	s := int(seconds)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, (s%3600)/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// ShouldShowProgress returns true if progress should be displayed.
// Progress is shown when stdout is a terminal.
func ShouldShowProgress() bool {
	return IsTerminalFunc(int(os.Stdout.Fd()))
}
