package addon

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tsukumogami/tsuku/internal/progress"
)

// ErrDownloadDeclined is returned when the user declines a download prompt.
var ErrDownloadDeclined = errors.New("download declined by user")

// Prompter handles user confirmation before large downloads.
type Prompter interface {
	// ConfirmDownload asks the user to confirm a download.
	// description is a human-readable name for the artifact (e.g., "tsuku-llm inference addon").
	// sizeBytes is the estimated download size in bytes. If 0, the size is unknown.
	// Returns true if the user approves, false if declined.
	// Returns ErrDownloadDeclined if the user explicitly declines.
	ConfirmDownload(ctx context.Context, description string, sizeBytes int64) (bool, error)
}

// InteractivePrompter prompts the user via stdin/stderr.
// It checks for TTY availability and falls back to declining if not interactive.
type InteractivePrompter struct {
	// Input is the reader for user input. Defaults to os.Stdin.
	Input io.Reader
	// Output is the writer for prompts. Defaults to os.Stderr.
	Output io.Writer
}

// ConfirmDownload displays a prompt and waits for the user to confirm.
// Defaults to yes on empty input. Non-TTY environments decline automatically.
func (p *InteractivePrompter) ConfirmDownload(ctx context.Context, description string, sizeBytes int64) (bool, error) {
	if !progress.ShouldShowProgress() {
		// Non-interactive: decline with an informative error
		return false, ErrDownloadDeclined
	}

	output := p.Output
	if output == nil {
		output = os.Stderr
	}
	input := p.Input
	if input == nil {
		input = os.Stdin
	}

	// Display the prompt
	if sizeBytes > 0 {
		fmt.Fprintf(output, "\nLocal LLM requires downloading %s (%s).\n", description, FormatSize(sizeBytes))
	} else {
		fmt.Fprintf(output, "\nLocal LLM requires downloading %s.\n", description)
	}
	fmt.Fprint(output, "Continue? [Y/n] ")

	// Read response
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		return false, ErrDownloadDeclined
	}

	response := strings.TrimSpace(strings.ToLower(scanner.Text()))
	switch response {
	case "", "y", "yes":
		return true, nil
	default:
		return false, ErrDownloadDeclined
	}
}

// AutoApprovePrompter automatically approves all download prompts.
// Used when the --yes flag is passed.
type AutoApprovePrompter struct{}

// ConfirmDownload always returns true without prompting.
func (p *AutoApprovePrompter) ConfirmDownload(ctx context.Context, description string, sizeBytes int64) (bool, error) {
	return true, nil
}

// NilPrompter declines all download prompts.
// Used as a safe default when no prompter is configured.
type NilPrompter struct{}

// ConfirmDownload always returns false with ErrDownloadDeclined.
func (p *NilPrompter) ConfirmDownload(ctx context.Context, description string, sizeBytes int64) (bool, error) {
	return false, ErrDownloadDeclined
}

// FormatSize formats a byte count into a human-readable string.
func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
