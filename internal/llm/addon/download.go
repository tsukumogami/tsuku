package addon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/tsukumogami/tsuku/internal/progress"
)

const (
	// downloadTimeout is the maximum time for a download attempt
	downloadTimeout = 10 * time.Minute

	// maxRetries is the number of download attempts before giving up
	maxRetries = 3

	// retryDelay is the initial delay between retries (doubles each attempt)
	retryDelay = 2 * time.Second
)

// Download downloads a file from url to destPath.
// It shows progress if stdout is a terminal.
// Returns an error if the download fails after retries.
func Download(ctx context.Context, url, destPath string) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := retryDelay * time.Duration(1<<(attempt-1)) // exponential backoff
			fmt.Printf("   Retrying download in %v (attempt %d/%d)...\n", delay, attempt+1, maxRetries)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		lastErr = downloadOnce(ctx, url, destPath)
		if lastErr == nil {
			return nil
		}

		fmt.Printf("   Download failed: %v\n", lastErr)
	}

	return fmt.Errorf("download failed after %d attempts: %w", maxRetries, lastErr)
}

// downloadOnce performs a single download attempt.
func downloadOnce(ctx context.Context, url, destPath string) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Perform request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Copy with progress if terminal
	if progress.ShouldShowProgress() && resp.ContentLength > 0 {
		pw := progress.NewWriter(out, resp.ContentLength, os.Stdout)
		defer pw.Finish()
		_, err = io.Copy(pw, resp.Body)
	} else {
		_, err = io.Copy(out, resp.Body)
	}

	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
