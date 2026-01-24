package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// DlopenResult represents the outcome of a dlopen test for a single library.
type DlopenResult struct {
	// Path is the absolute path to the library that was tested.
	Path string `json:"path"`

	// OK is true if dlopen succeeded for this library.
	OK bool `json:"ok"`

	// Error contains the dlerror() message if OK is false.
	Error string `json:"error,omitempty"`
}

// EnsureDltest checks if the tsuku-dltest helper is available and returns its path.
// For skeleton implementation, this checks common locations but doesn't download.
func EnsureDltest(tsukuHome string) (string, error) {
	// For skeleton: just look for the helper in PATH
	// Full implementation will download and verify checksum
	path, err := exec.LookPath("tsuku-dltest")
	if err != nil {
		return "", fmt.Errorf("tsuku-dltest helper not found: %w", err)
	}
	return path, nil
}

// InvokeDltest calls the tsuku-dltest helper to test dlopen on the given library paths.
// It returns a DlopenResult for each path, preserving order.
func InvokeDltest(ctx context.Context, helperPath string, paths []string) ([]DlopenResult, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, helperPath, paths...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Check for context cancellation/timeout
	if ctx.Err() != nil {
		return nil, fmt.Errorf("dltest helper: %w", ctx.Err())
	}

	// Parse JSON even on non-zero exit (exit 1 means some libraries failed)
	var results []DlopenResult
	if parseErr := json.Unmarshal(stdout.Bytes(), &results); parseErr != nil {
		// If we can't parse JSON, report the error
		if err != nil {
			return nil, fmt.Errorf("dltest helper failed: %w (stderr: %s)", err, stderr.String())
		}
		return nil, fmt.Errorf("failed to parse dltest output: %w", parseErr)
	}

	return results, nil
}
