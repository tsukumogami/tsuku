package addon

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tsukumogami/tsuku/internal/progress"
)

// Prompter handles user confirmation before downloads.
// Implementations can prompt interactively, auto-approve for scripting,
// or skip prompts in non-TTY environments.
type Prompter interface {
	// ConfirmDownload asks the user to confirm a download.
	// description explains what will be downloaded (e.g., "tsuku-llm addon").
	// size is the expected download size in bytes (0 if unknown).
	// Returns true if the user approves, false if declined.
	ConfirmDownload(ctx context.Context, description string, size int64) (bool, error)
}

// InteractivePrompter prompts the user via stdin/stdout.
// It only shows prompts when stdout is a terminal; in non-TTY environments
// it declines by default.
type InteractivePrompter struct {
	// In is the input reader (defaults to os.Stdin).
	In io.Reader
	// Out is the output writer (defaults to os.Stderr).
	Out io.Writer
}

// NewInteractivePrompter creates a prompter for interactive terminal use.
func NewInteractivePrompter() *InteractivePrompter {
	return &InteractivePrompter{
		In:  os.Stdin,
		Out: os.Stderr,
	}
}

// ConfirmDownload shows a confirmation prompt with download details.
// In non-TTY environments, it returns false without prompting.
func (p *InteractivePrompter) ConfirmDownload(ctx context.Context, description string, size int64) (bool, error) {
	if !progress.ShouldShowProgress() {
		// Non-TTY: decline silently. Use AutoApprovePrompter for CI.
		return false, nil
	}

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	// Build the prompt message
	if size > 0 {
		fmt.Fprintf(p.Out, "\ntsuku needs to download %s (%s).\n", description, formatSize(size))
	} else {
		fmt.Fprintf(p.Out, "\ntsuku needs to download %s.\n", description)
	}
	fmt.Fprintf(p.Out, "Download now? [Y/n] ")

	scanner := bufio.NewScanner(p.In)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("failed to read input: %w", err)
		}
		// EOF - treat as decline
		return false, nil
	}

	response := strings.TrimSpace(strings.ToLower(scanner.Text()))
	// Empty response (just Enter) defaults to yes
	return response == "" || response == "y" || response == "yes", nil
}

// AutoApprovePrompter always approves downloads.
// Use this when the --yes flag is set or in automated pipelines.
type AutoApprovePrompter struct {
	// Out is the output writer for informational messages (defaults to os.Stderr).
	Out io.Writer
}

// NewAutoApprovePrompter creates a prompter that auto-approves all downloads.
func NewAutoApprovePrompter() *AutoApprovePrompter {
	return &AutoApprovePrompter{Out: os.Stderr}
}

// ConfirmDownload logs the download details and returns true without prompting.
func (p *AutoApprovePrompter) ConfirmDownload(_ context.Context, description string, size int64) (bool, error) {
	out := p.Out
	if out == nil {
		out = os.Stderr
	}
	if size > 0 {
		fmt.Fprintf(out, "Auto-approving download: %s (%s)\n", description, formatSize(size))
	} else {
		fmt.Fprintf(out, "Auto-approving download: %s\n", description)
	}
	return true, nil
}

// NilPrompter declines all downloads silently.
// Use this as a safe default when no prompter is configured.
type NilPrompter struct{}

// ConfirmDownload always returns false.
func (p *NilPrompter) ConfirmDownload(_ context.Context, _ string, _ int64) (bool, error) {
	return false, nil
}

// ErrDownloadDeclined is returned when the user declines a download.
var ErrDownloadDeclined = fmt.Errorf("download declined by user")

// formatSize formats bytes into a human-readable string.
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.0f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.0f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}
