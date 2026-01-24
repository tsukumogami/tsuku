package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

var (
	// ErrHelperUnavailable indicates the helper could not be obtained
	// but for a recoverable reason (network failure, not installed, etc.).
	// Callers should skip Level 3 verification with a warning.
	ErrHelperUnavailable = errors.New("tsuku-dltest helper unavailable")

	// ErrChecksumMismatch indicates the helper checksum verification failed.
	// This is a security-critical error that should NOT be handled as fallback.
	ErrChecksumMismatch = errors.New("helper binary checksum verification failed")
)

const (
	// DefaultBatchSize is the maximum number of libraries per helper invocation.
	// This keeps command-line length well under ARG_MAX limits (~128KB on Linux).
	DefaultBatchSize = 50

	// BatchTimeout is the timeout for each batch invocation.
	// Library initialization code runs during dlopen; 5 seconds bounds the risk.
	BatchTimeout = 5 * time.Second
)

// BatchError represents a failure during batch processing.
type BatchError struct {
	Batch     []string // The library paths in the failed batch
	Cause     error    // The underlying error
	IsTimeout bool     // True if the error was a timeout
}

func (e *BatchError) Error() string {
	if e.IsTimeout {
		return fmt.Sprintf("batch timed out after %v for %d libraries", BatchTimeout, len(e.Batch))
	}
	return fmt.Sprintf("batch failed for %d libraries: %v", len(e.Batch), e.Cause)
}

func (e *BatchError) Unwrap() error {
	return e.Cause
}

// DlopenResult represents the outcome of a dlopen test for a single library.
type DlopenResult struct {
	// Path is the absolute path to the library that was tested.
	Path string `json:"path"`

	// OK is true if dlopen succeeded for this library.
	OK bool `json:"ok"`

	// Error contains the dlerror() message if OK is false.
	Error string `json:"error,omitempty"`
}

// EnsureDltest checks if the tsuku-dltest helper is installed with the correct
// version, installing it if necessary, and returns the path to the binary.
//
// The helper is installed via tsuku's standard recipe system, which provides:
// - Checksum verification for supply chain security
// - Version tracking in state.json
// - Standard installation patterns
//
// Version behavior:
// - When pinnedDltestVersion is "dev": accept any installed version, or install latest
// - When pinnedDltestVersion is a specific version: require that exact version
func EnsureDltest(cfg *config.Config) (string, error) {
	stateManager := install.NewStateManager(cfg)

	// Check current installation state
	toolState, err := stateManager.GetToolState("tsuku-dltest")
	if err != nil {
		return "", fmt.Errorf("failed to check tsuku-dltest state: %w", err)
	}

	// Determine installed version (handle both old and new state format)
	var installedVersion string
	if toolState != nil {
		if toolState.ActiveVersion != "" {
			installedVersion = toolState.ActiveVersion
		} else {
			installedVersion = toolState.Version
		}
	}

	// Dev mode: accept any installed version
	if pinnedDltestVersion == "dev" {
		if installedVersion != "" {
			// Use whatever version is installed
			dltestPath := filepath.Join(cfg.ToolBinDir("tsuku-dltest", installedVersion), "tsuku-dltest")
			if _, err := os.Stat(dltestPath); err == nil {
				return dltestPath, nil
			}
			// State says installed but binary missing - fall through to install latest
		}
		// Nothing installed, install latest
		if err := installDltest(""); err != nil {
			return "", err
		}
		// Re-check state to get the installed version
		toolState, err = stateManager.GetToolState("tsuku-dltest")
		if err != nil {
			return "", fmt.Errorf("failed to check tsuku-dltest state after install: %w", err)
		}
		if toolState == nil {
			return "", fmt.Errorf("tsuku-dltest install succeeded but no state found")
		}
		if toolState.ActiveVersion != "" {
			installedVersion = toolState.ActiveVersion
		} else {
			installedVersion = toolState.Version
		}
		dltestPath := filepath.Join(cfg.ToolBinDir("tsuku-dltest", installedVersion), "tsuku-dltest")
		if _, err := os.Stat(dltestPath); err != nil {
			return "", fmt.Errorf("tsuku-dltest installed but binary not found at %s", dltestPath)
		}
		return dltestPath, nil
	}

	// Release mode: require exact pinned version
	if installedVersion == pinnedDltestVersion {
		dltestPath := filepath.Join(cfg.ToolBinDir("tsuku-dltest", pinnedDltestVersion), "tsuku-dltest")
		if _, err := os.Stat(dltestPath); err == nil {
			return dltestPath, nil
		}
		// State says installed but binary missing - fall through to reinstall
	}

	// Need to install the pinned version
	if err := installDltest(pinnedDltestVersion); err != nil {
		return "", err
	}

	dltestPath := filepath.Join(cfg.ToolBinDir("tsuku-dltest", pinnedDltestVersion), "tsuku-dltest")
	if _, err := os.Stat(dltestPath); err != nil {
		return "", fmt.Errorf("tsuku-dltest installed but binary not found at %s", dltestPath)
	}

	return dltestPath, nil
}

// installDltest installs tsuku-dltest using the standard recipe flow.
// This invokes tsuku as a subprocess to reuse all installation infrastructure.
// If version is empty, installs the latest available version.
//
// Returns ErrChecksumMismatch if checksum verification fails (security-critical).
// Returns ErrHelperUnavailable for other installation failures (network, etc.).
func installDltest(version string) error {
	// Find tsuku binary - should be in PATH or we can use os.Executable
	tsukuPath, err := os.Executable()
	if err != nil {
		// Fall back to looking in PATH
		tsukuPath, err = exec.LookPath("tsuku")
		if err != nil {
			return fmt.Errorf("%w: cannot find tsuku binary to install helper: %v", ErrHelperUnavailable, err)
		}
	}

	// Build install command - use version spec if provided, otherwise install latest
	var toolSpec string
	if version != "" {
		toolSpec = fmt.Sprintf("tsuku-dltest@%s", version)
	} else {
		toolSpec = "tsuku-dltest"
	}
	cmd := exec.Command(tsukuPath, "install", toolSpec)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run installation
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		// Check for checksum mismatch - security-critical error
		// Only match the specific "checksum mismatch" error message from executor,
		// not generic phrases like "verification failed" which could appear in
		// other error contexts (e.g., "recipe not found").
		lowerMsg := strings.ToLower(errMsg)
		if strings.Contains(lowerMsg, "checksum mismatch") {
			return fmt.Errorf("%w: %s", ErrChecksumMismatch, errMsg)
		}
		// Other failures are recoverable (network, not found, etc.)
		return fmt.Errorf("%w: failed to install %s: %v\nstderr: %s",
			ErrHelperUnavailable, toolSpec, err, errMsg)
	}

	return nil
}

// InvokeDltest calls the tsuku-dltest helper to test dlopen on the given library paths.
// It returns a DlopenResult for each path, preserving order.
//
// Paths are processed in batches of up to 50 to avoid ARG_MAX limits and limit
// the impact of crashes. Each batch has a 5-second timeout. If a batch crashes,
// it is retried with halved batch size until the problematic library is isolated.
//
// Security: All paths are validated to be within $TSUKU_HOME/libs/ before invocation,
// and the helper runs with a sanitized environment (dangerous loader variables stripped).
func InvokeDltest(ctx context.Context, helperPath string, paths []string, tsukuHome string) ([]DlopenResult, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	// Validate all paths before processing
	libsDir := filepath.Join(tsukuHome, "libs")
	if err := validateLibraryPaths(paths, libsDir); err != nil {
		return nil, err
	}

	batches := splitIntoBatches(paths, DefaultBatchSize)
	var allResults []DlopenResult

	for _, batch := range batches {
		results, err := invokeBatchWithRetry(ctx, helperPath, batch, tsukuHome)
		if err != nil {
			return nil, err
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}

// splitIntoBatches divides paths into chunks of at most batchSize.
func splitIntoBatches(paths []string, batchSize int) [][]string {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	var batches [][]string
	for i := 0; i < len(paths); i += batchSize {
		end := i + batchSize
		if end > len(paths) {
			end = len(paths)
		}
		batches = append(batches, paths[i:end])
	}
	return batches
}

// invokeBatchWithRetry executes a batch, retrying with halved size on crash.
func invokeBatchWithRetry(ctx context.Context, helperPath string, batch []string, tsukuHome string) ([]DlopenResult, error) {
	results, err := invokeBatch(ctx, helperPath, batch, tsukuHome)
	if err == nil {
		return results, nil
	}

	// Check if parent context was canceled
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Check if it's a retriable error (crash, not timeout)
	batchErr, ok := err.(*BatchError)
	if !ok || batchErr.IsTimeout {
		return nil, err
	}

	// Don't retry if batch size is 1 - the crash is for this specific library
	if len(batch) == 1 {
		return nil, err
	}

	// Retry with halved batch size
	mid := len(batch) / 2
	firstHalf := batch[:mid]
	secondHalf := batch[mid:]

	var allResults []DlopenResult

	firstResults, err := invokeBatchWithRetry(ctx, helperPath, firstHalf, tsukuHome)
	if err != nil {
		return nil, err
	}
	allResults = append(allResults, firstResults...)

	secondResults, err := invokeBatchWithRetry(ctx, helperPath, secondHalf, tsukuHome)
	if err != nil {
		return nil, err
	}
	allResults = append(allResults, secondResults...)

	return allResults, nil
}

// sanitizeEnvForHelper creates a safe environment for the helper process.
// It strips dangerous loader variables that could enable code injection,
// and prepends $TSUKU_HOME/libs to library search paths.
func sanitizeEnvForHelper(tsukuHome string) []string {
	dangerous := map[string]bool{
		// Linux ld.so injection vectors
		"LD_PRELOAD": true, "LD_AUDIT": true, "LD_DEBUG": true,
		"LD_DEBUG_OUTPUT": true, "LD_PROFILE": true, "LD_PROFILE_OUTPUT": true,
		// macOS dyld injection vectors
		"DYLD_INSERT_LIBRARIES": true, "DYLD_FORCE_FLAT_NAMESPACE": true,
		"DYLD_PRINT_LIBRARIES": true, "DYLD_PRINT_LIBRARIES_POST_LAUNCH": true,
	}

	var env []string
	for _, e := range os.Environ() {
		key := strings.SplitN(e, "=", 2)[0]
		if !dangerous[key] {
			env = append(env, e)
		}
	}

	// Prepend tsuku libs to library search paths
	libsDir := filepath.Join(tsukuHome, "libs")
	env = append(env, fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", libsDir, os.Getenv("LD_LIBRARY_PATH")))
	env = append(env, fmt.Sprintf("DYLD_LIBRARY_PATH=%s:%s", libsDir, os.Getenv("DYLD_LIBRARY_PATH")))

	return env
}

// validateLibraryPaths ensures all paths are within the allowed libs directory.
// It canonicalizes paths via EvalSymlinks before checking the prefix,
// preventing path traversal and symlink escape attacks.
func validateLibraryPaths(paths []string, libsDir string) error {
	// Canonicalize the libs dir itself for consistent prefix checking
	canonicalLibsDir, err := filepath.EvalSymlinks(libsDir)
	if err != nil {
		return fmt.Errorf("libs directory not accessible: %w", err)
	}
	// Ensure trailing separator for correct prefix matching
	if !strings.HasSuffix(canonicalLibsDir, string(filepath.Separator)) {
		canonicalLibsDir += string(filepath.Separator)
	}

	for _, p := range paths {
		canonical, err := filepath.EvalSymlinks(p)
		if err != nil {
			return fmt.Errorf("invalid library path %q: %w", p, err)
		}
		// Path must be within libs directory (or be the libs directory itself)
		if canonical != strings.TrimSuffix(canonicalLibsDir, string(filepath.Separator)) &&
			!strings.HasPrefix(canonical, canonicalLibsDir) {
			return fmt.Errorf("library path %q resolves outside libs directory", p)
		}
	}
	return nil
}

// invokeBatch executes the helper on a single batch with timeout.
// Returns results, or error if timeout/crash occurred.
func invokeBatch(ctx context.Context, helperPath string, batch []string, tsukuHome string) ([]DlopenResult, error) {
	// Create timeout context for this batch
	batchCtx, cancel := context.WithTimeout(ctx, BatchTimeout)
	defer cancel()

	cmd := exec.CommandContext(batchCtx, helperPath, batch...)
	cmd.Env = sanitizeEnvForHelper(tsukuHome)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Check for timeout
	if batchCtx.Err() == context.DeadlineExceeded {
		return nil, &BatchError{Batch: batch, IsTimeout: true}
	}

	// Check for parent context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Check for crash (signal or unexpected exit)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			// Exit codes: 0 = all ok, 1 = some failed, 2 = usage error
			// Anything else (or -1 for signal) indicates crash
			if code != 0 && code != 1 && code != 2 {
				return nil, &BatchError{Batch: batch, Cause: err}
			}
		} else {
			// Non-exit error (couldn't start process, etc.)
			return nil, &BatchError{Batch: batch, Cause: err}
		}
	}

	// Parse JSON
	var results []DlopenResult
	if parseErr := json.Unmarshal(stdout.Bytes(), &results); parseErr != nil {
		if err != nil {
			return nil, &BatchError{Batch: batch, Cause: fmt.Errorf("parse failed: %w (stderr: %s)", parseErr, stderr.String())}
		}
		return nil, &BatchError{Batch: batch, Cause: fmt.Errorf("parse failed: %w", parseErr)}
	}

	return results, nil
}

// DlopenVerificationResult holds the outcome of dlopen verification.
type DlopenVerificationResult struct {
	// Results contains the dlopen test results for each library.
	Results []DlopenResult

	// Skipped is true if Level 3 verification was skipped.
	Skipped bool

	// Warning contains a warning message if verification was skipped
	// due to helper unavailability (not user opt-out).
	Warning string
}

// RunDlopenVerification performs Level 3 dlopen verification with fallback behavior.
//
// Behavior:
//   - If skipDlopen is true, returns immediately with Skipped=true (no warning).
//   - If the helper is unavailable (non-checksum reason), returns Skipped=true with warning.
//   - If the helper checksum fails, returns an error (security-critical, no fallback).
//   - Otherwise, runs dlopen verification and returns results.
func RunDlopenVerification(ctx context.Context, cfg *config.Config, paths []string, skipDlopen bool) (*DlopenVerificationResult, error) {
	// User explicitly requested skip - silent, no warning
	if skipDlopen {
		return &DlopenVerificationResult{Skipped: true}, nil
	}

	// No paths to verify
	if len(paths) == 0 {
		return &DlopenVerificationResult{Skipped: true}, nil
	}

	// Try to get the helper
	helperPath, err := EnsureDltest(cfg)
	if err != nil {
		// Checksum mismatch is security-critical - must error
		if errors.Is(err, ErrChecksumMismatch) {
			return nil, fmt.Errorf("helper binary checksum verification failed, refusing to execute")
		}
		// Other failures (network, not installed, etc.) - skip with warning
		return &DlopenVerificationResult{
			Skipped: true,
			Warning: "Warning: tsuku-dltest helper not available, skipping load test\n  Run 'tsuku install tsuku-dltest' to enable full verification",
		}, nil
	}

	// Run dlopen verification
	results, err := InvokeDltest(ctx, helperPath, paths, cfg.HomeDir)
	if err != nil {
		return nil, err
	}

	return &DlopenVerificationResult{Results: results}, nil
}
