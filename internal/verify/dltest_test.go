package verify

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

func TestDlopenResult_JSONParsing_Success(t *testing.T) {
	input := `[{"path":"/lib/libc.so.6","ok":true}]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.Path != "/lib/libc.so.6" {
		t.Errorf("Path = %q, want %q", r.Path, "/lib/libc.so.6")
	}
	if !r.OK {
		t.Error("OK = false, want true")
	}
	if r.Error != "" {
		t.Errorf("Error = %q, want empty", r.Error)
	}
}

func TestDlopenResult_JSONParsing_Failure(t *testing.T) {
	input := `[{"path":"/nonexistent.so","ok":false,"error":"cannot open shared object file"}]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.Path != "/nonexistent.so" {
		t.Errorf("Path = %q, want %q", r.Path, "/nonexistent.so")
	}
	if r.OK {
		t.Error("OK = true, want false")
	}
	if r.Error != "cannot open shared object file" {
		t.Errorf("Error = %q, want %q", r.Error, "cannot open shared object file")
	}
}

func TestDlopenResult_JSONParsing_Mixed(t *testing.T) {
	input := `[
		{"path":"/lib/libc.so.6","ok":true},
		{"path":"/lib/libpthread.so.0","ok":true},
		{"path":"/nonexistent.so","ok":false,"error":"not found"}
	]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// First two should be OK
	if !results[0].OK || !results[1].OK {
		t.Error("first two results should be OK")
	}

	// Third should be failure
	if results[2].OK {
		t.Error("third result should be failure")
	}
	if results[2].Error != "not found" {
		t.Errorf("Error = %q, want %q", results[2].Error, "not found")
	}
}

func TestDlopenResult_JSONParsing_Empty(t *testing.T) {
	input := `[]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestEnsureDltest_NotInstalled(t *testing.T) {
	// Create a temp directory for test
	tmpDir, err := os.MkdirTemp("", "tsuku-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config with temp directory
	cfg := &config.Config{
		HomeDir:  tmpDir,
		ToolsDir: filepath.Join(tmpDir, "tools"),
	}

	// Test the state check logic directly instead of calling EnsureDltest,
	// which would try to invoke tsuku to install (causing test recursion).
	// When nothing is installed, GetToolState should return nil (not an error).
	stateManager := install.NewStateManager(cfg)
	toolState, err := stateManager.GetToolState("tsuku-dltest")
	if err != nil {
		t.Fatalf("GetToolState failed unexpectedly: %v", err)
	}
	if toolState != nil {
		t.Error("expected nil toolState for uninstalled tool")
	}

	// Verify the version check logic: when no state exists, installedVersion is empty
	var installedVersion string
	if toolState != nil {
		if toolState.ActiveVersion != "" {
			installedVersion = toolState.ActiveVersion
		} else {
			installedVersion = toolState.Version
		}
	}
	if installedVersion != "" {
		t.Errorf("installedVersion = %q, want empty string", installedVersion)
	}

	// Verify this is NOT the pinned version (so installation would be triggered)
	if installedVersion == pinnedDltestVersion {
		t.Error("empty version should not match pinnedDltestVersion")
	}
}

func TestEnsureDltest_CorrectVersionInstalled(t *testing.T) {
	// Create a temp directory for test
	tmpDir, err := os.MkdirTemp("", "tsuku-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config with temp directory
	cfg := &config.Config{
		HomeDir:  tmpDir,
		ToolsDir: filepath.Join(tmpDir, "tools"),
	}

	// Create the tool directory and binary
	version := pinnedDltestVersion
	binDir := cfg.ToolBinDir("tsuku-dltest", version)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create a fake binary
	binaryPath := filepath.Join(binDir, "tsuku-dltest")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	// Set up state to show tool is installed
	stateManager := install.NewStateManager(cfg)
	if err := stateManager.UpdateTool("tsuku-dltest", func(ts *install.ToolState) {
		ts.ActiveVersion = version
		ts.IsHidden = true
	}); err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// EnsureDltest should return path without trying to install
	path, err := EnsureDltest(cfg)
	if err != nil {
		t.Fatalf("EnsureDltest failed: %v", err)
	}

	if path != binaryPath {
		t.Errorf("path = %q, want %q", path, binaryPath)
	}
}

func TestEnsureDltest_WrongVersionInstalled(t *testing.T) {
	// Skip in dev mode since dev mode accepts any version
	if pinnedDltestVersion == "dev" {
		t.Skip("skipping wrong version test in dev mode (any version accepted)")
	}

	// Create a temp directory for test
	tmpDir, err := os.MkdirTemp("", "tsuku-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config with temp directory
	cfg := &config.Config{
		HomeDir:  tmpDir,
		ToolsDir: filepath.Join(tmpDir, "tools"),
	}

	// Set up state with wrong version
	stateManager := install.NewStateManager(cfg)
	wrongVersion := "v0.0.0-wrong"
	if err := stateManager.UpdateTool("tsuku-dltest", func(ts *install.ToolState) {
		ts.ActiveVersion = wrongVersion
		ts.IsHidden = true
	}); err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// Test the version check logic directly instead of calling EnsureDltest,
	// which would try to invoke tsuku to install (causing test recursion).
	toolState, err := stateManager.GetToolState("tsuku-dltest")
	if err != nil {
		t.Fatalf("GetToolState failed: %v", err)
	}
	if toolState == nil {
		t.Fatal("expected non-nil toolState for installed tool")
	}

	// Verify the version detection logic works
	var installedVersion string
	if toolState.ActiveVersion != "" {
		installedVersion = toolState.ActiveVersion
	} else {
		installedVersion = toolState.Version
	}

	if installedVersion != wrongVersion {
		t.Errorf("installedVersion = %q, want %q", installedVersion, wrongVersion)
	}

	// Verify wrong version does NOT match pinned version (so installation would be triggered)
	if installedVersion == pinnedDltestVersion {
		t.Errorf("wrong version %q should not match pinnedDltestVersion %q",
			installedVersion, pinnedDltestVersion)
	}
}

func TestEnsureDltest_DevMode_AcceptsAnyVersion(t *testing.T) {
	// This test validates dev mode behavior: any installed version is accepted
	if pinnedDltestVersion != "dev" {
		t.Skip("skipping dev mode test when not in dev mode")
	}

	// Create a temp directory for test
	tmpDir, err := os.MkdirTemp("", "tsuku-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config with temp directory
	cfg := &config.Config{
		HomeDir:  tmpDir,
		ToolsDir: filepath.Join(tmpDir, "tools"),
	}

	// Install an arbitrary version (simulating a previous release)
	arbitraryVersion := "v0.3.0"
	binDir := cfg.ToolBinDir("tsuku-dltest", arbitraryVersion)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create a fake binary
	binaryPath := filepath.Join(binDir, "tsuku-dltest")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	// Set up state with the arbitrary version
	stateManager := install.NewStateManager(cfg)
	if err := stateManager.UpdateTool("tsuku-dltest", func(ts *install.ToolState) {
		ts.ActiveVersion = arbitraryVersion
		ts.IsHidden = true
	}); err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// In dev mode, EnsureDltest should accept the arbitrary version
	path, err := EnsureDltest(cfg)
	if err != nil {
		t.Fatalf("EnsureDltest failed: %v", err)
	}

	if path != binaryPath {
		t.Errorf("path = %q, want %q", path, binaryPath)
	}
}

// Tests for batch processing

func TestSplitIntoBatches_Empty(t *testing.T) {
	batches := splitIntoBatches(nil, 50)
	if len(batches) != 0 {
		t.Errorf("got %d batches, want 0", len(batches))
	}
}

func TestSplitIntoBatches_SingleBatch(t *testing.T) {
	paths := []string{"a.so", "b.so", "c.so"}
	batches := splitIntoBatches(paths, 50)

	if len(batches) != 1 {
		t.Fatalf("got %d batches, want 1", len(batches))
	}
	if len(batches[0]) != 3 {
		t.Errorf("batch[0] has %d items, want 3", len(batches[0]))
	}
}

func TestSplitIntoBatches_MultipleBatches(t *testing.T) {
	// Create 75 paths - should split into 2 batches (50 + 25)
	paths := make([]string, 75)
	for i := range paths {
		paths[i] = "lib.so"
	}

	batches := splitIntoBatches(paths, 50)

	if len(batches) != 2 {
		t.Fatalf("got %d batches, want 2", len(batches))
	}
	if len(batches[0]) != 50 {
		t.Errorf("batch[0] has %d items, want 50", len(batches[0]))
	}
	if len(batches[1]) != 25 {
		t.Errorf("batch[1] has %d items, want 25", len(batches[1]))
	}
}

func TestSplitIntoBatches_ExactMultiple(t *testing.T) {
	// Create 100 paths - should split into 2 batches of 50 each
	paths := make([]string, 100)
	for i := range paths {
		paths[i] = "lib.so"
	}

	batches := splitIntoBatches(paths, 50)

	if len(batches) != 2 {
		t.Fatalf("got %d batches, want 2", len(batches))
	}
	if len(batches[0]) != 50 || len(batches[1]) != 50 {
		t.Errorf("expected two batches of 50, got %d and %d", len(batches[0]), len(batches[1]))
	}
}

func TestSplitIntoBatches_SmallBatchSize(t *testing.T) {
	paths := []string{"a.so", "b.so", "c.so", "d.so", "e.so"}
	batches := splitIntoBatches(paths, 2)

	if len(batches) != 3 {
		t.Fatalf("got %d batches, want 3", len(batches))
	}
	// Should be: [a, b], [c, d], [e]
	if len(batches[0]) != 2 || len(batches[1]) != 2 || len(batches[2]) != 1 {
		t.Errorf("unexpected batch sizes: %d, %d, %d", len(batches[0]), len(batches[1]), len(batches[2]))
	}
}

func TestSplitIntoBatches_ZeroBatchSize(t *testing.T) {
	paths := []string{"a.so", "b.so"}
	// Zero batch size should fall back to DefaultBatchSize
	batches := splitIntoBatches(paths, 0)

	if len(batches) != 1 {
		t.Fatalf("got %d batches, want 1 (should use default)", len(batches))
	}
}

// Tests for BatchError

func TestBatchError_Timeout(t *testing.T) {
	err := &BatchError{
		Batch:     []string{"a.so", "b.so"},
		IsTimeout: true,
	}

	msg := err.Error()
	if msg == "" {
		t.Error("expected non-empty error message")
	}
	// Should mention timeout and count
	if !stringContains(msg, "timed out") || !stringContains(msg, "2 libraries") {
		t.Errorf("error message missing expected content: %s", msg)
	}
}

func TestBatchError_Crash(t *testing.T) {
	cause := errors.New("signal: killed")
	err := &BatchError{
		Batch: []string{"crash.so"},
		Cause: cause,
	}

	msg := err.Error()
	if msg == "" {
		t.Error("expected non-empty error message")
	}
	// Should mention failure count and cause
	if !stringContains(msg, "1 libraries") || !stringContains(msg, "signal: killed") {
		t.Errorf("error message missing expected content: %s", msg)
	}

	// Unwrap should return cause
	if errors.Unwrap(err) != cause {
		t.Error("Unwrap did not return cause")
	}
}

func TestBatchError_Unwrap(t *testing.T) {
	cause := &exec.ExitError{}
	err := &BatchError{Batch: []string{"x.so"}, Cause: cause}

	unwrapped := errors.Unwrap(err)
	if unwrapped != cause {
		t.Errorf("Unwrap returned %v, want %v", unwrapped, cause)
	}
}

// Tests for InvokeDltest batching behavior

func TestInvokeDltest_EmptyPaths(t *testing.T) {
	ctx := context.Background()
	results, err := InvokeDltest(ctx, "/nonexistent", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestInvokeDltest_HelperNotFound(t *testing.T) {
	ctx := context.Background()
	_, err := InvokeDltest(ctx, "/nonexistent/helper", []string{"a.so"})

	if err == nil {
		t.Fatal("expected error for missing helper")
	}

	// Should be a BatchError
	var batchErr *BatchError
	if !errors.As(err, &batchErr) {
		t.Errorf("expected BatchError, got %T: %v", err, err)
	}
}

func TestInvokeDltest_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := InvokeDltest(ctx, "/nonexistent", []string{"a.so"})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

// Test with a mock helper script that outputs valid JSON
func TestInvokeDltest_MockHelper_Success(t *testing.T) {
	// Create temp script that outputs valid JSON
	tmpDir := t.TempDir()
	helperPath := filepath.Join(tmpDir, "mock-dltest")

	// Script outputs JSON for all arguments
	script := `#!/bin/sh
echo '['
first=true
for arg in "$@"; do
    if [ "$first" = "true" ]; then
        first=false
    else
        echo ','
    fi
    echo "{\"path\":\"$arg\",\"ok\":true}"
done
echo ']'
exit 0
`
	if err := os.WriteFile(helperPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock helper: %v", err)
	}

	ctx := context.Background()
	paths := []string{"/lib/a.so", "/lib/b.so"}
	results, err := InvokeDltest(ctx, helperPath, paths)
	if err != nil {
		t.Fatalf("InvokeDltest failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for i, r := range results {
		if r.Path != paths[i] {
			t.Errorf("result[%d].Path = %q, want %q", i, r.Path, paths[i])
		}
		if !r.OK {
			t.Errorf("result[%d].OK = false, want true", i)
		}
	}
}

// Test batch splitting with many paths
func TestInvokeDltest_MockHelper_ManyPaths(t *testing.T) {
	// Create temp script that counts arguments
	tmpDir := t.TempDir()
	helperPath := filepath.Join(tmpDir, "mock-dltest")

	// Script outputs JSON for all arguments
	script := `#!/bin/sh
echo '['
first=true
for arg in "$@"; do
    if [ "$first" = "true" ]; then
        first=false
    else
        echo ','
    fi
    echo "{\"path\":\"$arg\",\"ok\":true}"
done
echo ']'
exit 0
`
	if err := os.WriteFile(helperPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock helper: %v", err)
	}

	// Create 75 paths - should be split into 2 batches
	paths := make([]string, 75)
	for i := range paths {
		paths[i] = "/lib/lib" + string(rune('a'+i%26)) + ".so"
	}

	ctx := context.Background()
	results, err := InvokeDltest(ctx, helperPath, paths)
	if err != nil {
		t.Fatalf("InvokeDltest failed: %v", err)
	}

	if len(results) != 75 {
		t.Errorf("got %d results, want 75", len(results))
	}
}

// Test timeout handling - skip if running with short flag
func TestInvokeDltest_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	// Create a helper that uses exec to replace the shell process with sleep
	// This ensures the SIGKILL from context timeout actually kills the process
	tmpDir := t.TempDir()
	helperPath := filepath.Join(tmpDir, "slow-dltest")

	// Use exec to replace shell process - this ensures timeout kills the right process
	script := `#!/bin/sh
exec sleep 10
`
	if err := os.WriteFile(helperPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write slow helper: %v", err)
	}

	ctx := context.Background()
	start := time.Now()
	_, err := InvokeDltest(ctx, helperPath, []string{"a.so"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}

	// Should timeout around 5 seconds, not 10
	if elapsed > 7*time.Second {
		t.Errorf("timeout took too long: %v (expected ~5s)", elapsed)
	}

	var batchErr *BatchError
	if !errors.As(err, &batchErr) {
		t.Errorf("expected BatchError, got %T: %v", err, err)
	} else if !batchErr.IsTimeout {
		t.Error("expected IsTimeout=true")
	}
}

// Test retry on crash - helper exits with unexpected code
func TestInvokeDltest_RetryOnCrash(t *testing.T) {
	tmpDir := t.TempDir()
	counterFile := filepath.Join(tmpDir, "count")
	helperPath := filepath.Join(tmpDir, "crash-dltest")

	// Script that crashes on first call (with 2+ args), succeeds on retry
	// Uses a file to track call count since each invocation is a new process
	script := `#!/bin/sh
count=$(cat "` + counterFile + `" 2>/dev/null || echo 0)
count=$((count + 1))
echo $count > "` + counterFile + `"

# Crash (exit 139 = SIGSEGV) if this is call 1 and we have more than 1 arg
if [ "$count" -eq 1 ] && [ $# -gt 1 ]; then
    exit 139
fi

# Otherwise succeed
echo '['
first=true
for arg in "$@"; do
    if [ "$first" = "true" ]; then
        first=false
    else
        echo ','
    fi
    echo "{\"path\":\"$arg\",\"ok\":true}"
done
echo ']'
exit 0
`
	if err := os.WriteFile(helperPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write helper: %v", err)
	}
	// Initialize counter
	if err := os.WriteFile(counterFile, []byte("0"), 0644); err != nil {
		t.Fatalf("failed to write counter: %v", err)
	}

	ctx := context.Background()
	paths := []string{"a.so", "b.so"}
	results, err := InvokeDltest(ctx, helperPath, paths)

	if err != nil {
		t.Fatalf("InvokeDltest failed (should have retried): %v", err)
	}

	// Should have gotten results for both libraries
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}

	// Check that we actually retried (counter should be > 1)
	countData, _ := os.ReadFile(counterFile)
	countStr := string(countData)
	if countStr == "1\n" || countStr == "1" {
		t.Error("expected retry (counter > 1), but helper was only called once")
	}
}

// Test that exit code 1 (some libraries failed) is not treated as crash
func TestInvokeDltest_ExitCode1_NotCrash(t *testing.T) {
	tmpDir := t.TempDir()
	helperPath := filepath.Join(tmpDir, "fail-dltest")

	// Script exits with code 1 but provides valid JSON
	script := `#!/bin/sh
echo '[{"path":"bad.so","ok":false,"error":"cannot load"}]'
exit 1
`
	if err := os.WriteFile(helperPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write helper: %v", err)
	}

	ctx := context.Background()
	results, err := InvokeDltest(ctx, helperPath, []string{"bad.so"})

	// Should succeed (exit 1 is expected for dlopen failures)
	if err != nil {
		t.Fatalf("unexpected error for exit code 1: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].OK {
		t.Error("expected OK=false")
	}
}

// helper function for string contains
func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
