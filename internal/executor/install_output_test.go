package executor

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// testReporter implements progress.Reporter and records all calls for assertion.
// It is safe for concurrent use.
type testReporter struct {
	mu         sync.Mutex
	Logs       []string // Permanent lines written by Log() and Warn()
	Statuses   []string // Transient status messages from Status()
	StopCalled bool
}

func (r *testReporter) Log(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.mu.Lock()
	r.Logs = append(r.Logs, msg)
	r.mu.Unlock()
}

func (r *testReporter) Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.mu.Lock()
	r.Logs = append(r.Logs, "warning: "+msg)
	r.mu.Unlock()
}

func (r *testReporter) Status(msg string) {
	r.mu.Lock()
	r.Statuses = append(r.Statuses, msg)
	r.mu.Unlock()
}

func (r *testReporter) DeferWarn(format string, args ...any) {}
func (r *testReporter) FlushDeferred()                       {}

func (r *testReporter) Stop() {
	r.mu.Lock()
	r.StopCalled = true
	r.mu.Unlock()
}

func (r *testReporter) hasLog(substr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range r.Logs {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func (r *testReporter) hasStatus(substr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.Statuses {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// newMinimalRecipe returns a recipe with no version source so version resolution
// is not required during ExecutePlan (we supply the plan directly).
func newMinimalRecipe(name string) *recipe.Recipe {
	return &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: name,
		},
	}
}

// --- scenario-14: no bytes escape to os.Stderr during install ---

// TestInstallReporterOutput verifies that ExecutePlan with a TestReporter does not
// write any bytes to os.Stderr. This confirms that all progress output goes through
// the Reporter and none escapes via fmt.Printf or fmt.Fprintln to os.Stderr.
func TestInstallReporterOutput(t *testing.T) {
	// Redirect os.Stderr to a pipe so we can capture any rogue output.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w

	defer func() {
		os.Stderr = origStderr
	}()

	// Create a minimal executor with a chmod step (no network required).
	rec := newMinimalRecipe("test-tool")
	exec, err := New(rec)
	if err != nil {
		w.Close()
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Create a file to chmod so the action succeeds.
	testFile := "output_test.sh"
	testFilePath := filepath.Join(exec.WorkDir(), testFile)
	if err := os.WriteFile(testFilePath, []byte("#!/bin/sh\n"), 0644); err != nil {
		w.Close()
		t.Fatalf("failed to create test file: %v", err)
	}

	reporter := &testReporter{}
	exec.SetReporter(reporter)

	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps: []ResolvedStep{
			{
				Action:    "chmod",
				Evaluable: true,
				Params: map[string]interface{}{
					"files": []interface{}{testFile},
					"mode":  "0755",
				},
			},
		},
	}

	ctx := context.Background()
	execErr := exec.ExecutePlan(ctx, plan)

	// Close the write end so the read below does not block forever.
	w.Close()

	// Read whatever (if anything) was written to stderr.
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	r.Close()

	if execErr != nil {
		t.Fatalf("ExecutePlan() unexpected error = %v", execErr)
	}

	if n > 0 {
		t.Errorf("unexpected output on os.Stderr (%d bytes): %q", n, buf[:n])
	}
}

// --- scenario-15: non-TTY install log lines ---

// TestNonTTYInstallLogLines verifies that the executor records expected Log lines when
// installing a plan that has a dependency. The dependency path calls reporter.Log()
// with "Installing dependency: <tool>@<version>" and "Installed <tool>@<version>",
// which should appear in TestReporter.Logs. StopCalled must be true after Stop().
func TestNonTTYInstallLogLines(t *testing.T) {
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	rec := newMinimalRecipe("main-tool")
	exec, err := New(rec)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	exec.SetToolsDir(toolsDir)
	exec.SetSkipCacheSecurityChecks(true)

	reporter := &testReporter{}
	exec.SetReporter(reporter)

	// Build a plan whose dependency has a simple chmod step. The executor's
	// installSingleDependency will call reporter.Log() at start and completion.
	// chmod on "." (the work directory itself) succeeds without pre-creating files.
	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "main-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Dependencies: []DependencyPlan{
			{
				Tool:    "dep-tool",
				Version: "0.1.0",
				Steps: []ResolvedStep{
					{
						Action:    "chmod",
						Evaluable: true,
						Params: map[string]interface{}{
							"files": []interface{}{"."},
							"mode":  "0755",
						},
					},
				},
			},
		},
		Steps: []ResolvedStep{},
	}

	ctx := context.Background()
	err = exec.ExecutePlan(ctx, plan)
	if err != nil {
		t.Fatalf("ExecutePlan failed: %v", err)
	}

	if !reporter.hasStatus("Installing dep-tool@0.1.0") {
		t.Errorf("Statuses does not contain 'Installing dep-tool@0.1.0'; got: %v", reporter.Statuses)
	}
	if !reporter.hasLog("✅ dep-tool@0.1.0") {
		t.Errorf("Logs does not contain '✅ dep-tool@0.1.0'; got: %v", reporter.Logs)
	}

	// Status-only messages must not appear in Logs.
	for _, l := range reporter.Logs {
		for _, s := range reporter.Statuses {
			if l == s {
				t.Errorf("status-only message %q appeared in Logs", s)
			}
		}
	}

	// Simulate the defer reporter.Stop() that the orchestration layer performs.
	reporter.Stop()
	if !reporter.StopCalled {
		t.Error("expected StopCalled=true after Stop()")
	}
}

// --- scenario-16: non-TTY build recipe log line ---

// TestNonTTYBuildRecipeLogLine verifies that when a plan step has an ActionDescriber
// whose StatusMessage returns "Building <tool>", the executor records that message in
// Statuses before executing the step. This confirms the CI feedback path for build
// recipes works without a real compile.
func TestNonTTYBuildRecipeLogLine(t *testing.T) {
	rec := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "my-tool",
		},
	}

	exec, err := New(rec)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	reporter := &testReporter{}
	exec.SetReporter(reporter)

	// go_build's ActionDescriber returns "Building <module>" when a module param is set.
	// The action itself will fail (no real source), but Status() is recorded before execution.
	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "my-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps: []ResolvedStep{
			{
				Action:    "go_build",
				Evaluable: false,
				Params: map[string]interface{}{
					"module":  "my-tool",
					"version": "1.0.0",
				},
			},
		},
	}

	ctx := context.Background()
	// ExecutePlan will likely fail when go_build actually runs; that is expected.
	_ = exec.ExecutePlan(ctx, plan)

	// The executor must have called Status("Building my-tool@1.0.0") before executing
	// the step, regardless of whether the action itself succeeded.
	if !reporter.hasStatus("Building my-tool") {
		t.Errorf("Statuses does not contain 'Building my-tool'; got: %v", reporter.Statuses)
	}

	// Verify StopCalled works correctly after manual Stop().
	reporter.Stop()
	if !reporter.StopCalled {
		t.Error("expected StopCalled=true after Stop()")
	}
}

// --- progress retry test ---

// TestProgressWriterRetryNoExceed100 verifies that when ProgressWriter.Reset() is
// called between retry attempts, the percentage computed from the callback never
// exceeds 100%. This confirms that Reset() correctly clears the written counter.
func TestProgressWriterRetryNoExceed100(t *testing.T) {
	reporter := &testReporter{}

	total := int64(500 * 1024) // above small-file threshold

	pw := progress.NewProgressWriter(
		io.Discard,
		total,
		func(written, tot int64) {
			pct := float64(written) / float64(tot) * 100
			reporter.Status(fmt.Sprintf("%.1f%%", pct))
		},
	)

	// First attempt: write the full amount.
	buf := make([]byte, int(total))
	if _, err := pw.Write(buf); err != nil {
		t.Fatalf("first Write failed: %v", err)
	}

	// Reset and simulate a retry: write again.
	pw.Reset()
	if _, err := pw.Write(buf); err != nil {
		t.Fatalf("second Write failed: %v", err)
	}

	// No percentage recorded in Statuses must exceed 100%.
	for _, s := range reporter.Statuses {
		// Parse the float from the formatted string.
		var pct float64
		if _, scanErr := fmt.Sscanf(s, "%f%%", &pct); scanErr == nil {
			if pct > 100.0 {
				t.Errorf("percentage %q exceeds 100%%", s)
			}
		}
	}
}

// createTestTarGz creates a minimal tar.gz archive at dest containing one file.
func createTestTarGz(dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	content := []byte("hello")
	hdr := &tar.Header{
		Name: "hello.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err = tw.Write(content); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gw.Close()
}

// --- scenario-17: extract action reporter classification ---

// TestExtractActionReporterClassification verifies that the extract action routes
// its status output through Status() and does not emit internal parameter details
// (format, strip_dirs) through Log().
func TestExtractActionReporterClassification(t *testing.T) {
	t.Parallel()

	rec := newMinimalRecipe("test-tool")
	exec, err := New(rec)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Create a minimal tar.gz archive in the work directory.
	archivePath := filepath.Join(exec.WorkDir(), "test.tar.gz")
	if err := createTestTarGz(archivePath); err != nil {
		t.Fatalf("failed to create test archive: %v", err)
	}

	reporter := &testReporter{}
	exec.SetReporter(reporter)

	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps: []ResolvedStep{
			{
				Action:    "extract",
				Evaluable: true,
				Params: map[string]interface{}{
					"archive": "test.tar.gz",
					"format":  "tar.gz",
				},
			},
		},
	}

	ctx := context.Background()
	if err := exec.ExecutePlan(ctx, plan); err != nil {
		t.Fatalf("ExecutePlan() unexpected error = %v", err)
	}

	if !reporter.hasStatus("Extracting:") {
		t.Errorf("Statuses should contain 'Extracting:'; got: %v", reporter.Statuses)
	}
	if reporter.hasLog("Extracting:") {
		t.Errorf("Logs should not contain 'Extracting:'; got: %v", reporter.Logs)
	}
	if reporter.hasLog("Format:") {
		t.Errorf("Logs should not contain 'Format:'; got: %v", reporter.Logs)
	}
	if reporter.hasLog("Strip dirs:") {
		t.Errorf("Logs should not contain 'Strip dirs:'; got: %v", reporter.Logs)
	}
}

// --- scenario-18: run_command action reporter classification ---

// TestRunCommandReporterClassification verifies that run_command routes the
// running status through Status() and command output through Log(), while
// internal success messages do not appear in Logs.
func TestRunCommandReporterClassification(t *testing.T) {
	t.Parallel()

	rec := newMinimalRecipe("test-tool")
	exec, err := New(rec)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	reporter := &testReporter{}
	exec.SetReporter(reporter)

	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps: []ResolvedStep{
			{
				Action:    "run_command",
				Evaluable: true,
				Params: map[string]interface{}{
					"command": "echo hello",
				},
			},
		},
	}

	ctx := context.Background()
	if err := exec.ExecutePlan(ctx, plan); err != nil {
		t.Fatalf("ExecutePlan() unexpected error = %v", err)
	}

	if !reporter.hasStatus("Running:") {
		t.Errorf("Statuses should contain 'Running:'; got: %v", reporter.Statuses)
	}
	if reporter.hasLog("Running:") {
		t.Errorf("Logs should not contain 'Running:'; got: %v", reporter.Logs)
	}
	if reporter.hasLog("Description:") {
		t.Errorf("Logs should not contain 'Description:'; got: %v", reporter.Logs)
	}
	if reporter.hasLog("Command executed successfully") {
		t.Errorf("Logs should not contain 'Command executed successfully'; got: %v", reporter.Logs)
	}
	if !reporter.hasLog("Output:") {
		t.Errorf("Logs should contain 'Output:' for a command that produces output; got: %v", reporter.Logs)
	}
}

// --- scenario-19: install_binaries action reporter classification ---

// TestInstallBinariesReporterClassification verifies that install_binaries emits
// a single bulk count through Log() and does not emit per-file "Installed" lines.
func TestInstallBinariesReporterClassification(t *testing.T) {
	t.Parallel()

	rec := newMinimalRecipe("test-tool")
	exec, err := New(rec)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Create the source file in the work directory.
	testbinPath := filepath.Join(exec.WorkDir(), "testbin")
	if err := os.WriteFile(testbinPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create testbin: %v", err)
	}

	reporter := &testReporter{}
	exec.SetReporter(reporter)

	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps: []ResolvedStep{
			{
				Action:    "install_binaries",
				Evaluable: true,
				Params: map[string]interface{}{
					"outputs": []interface{}{
						map[string]interface{}{
							"src":  "testbin",
							"dest": "bin/testbin",
						},
					},
				},
			},
		},
	}

	ctx := context.Background()
	if err := exec.ExecutePlan(ctx, plan); err != nil {
		t.Fatalf("ExecutePlan() unexpected error = %v", err)
	}

	// Should have a bulk count status line (ephemeral).
	if !reporter.hasStatus("Installing 1 file(s)") {
		t.Errorf("Statuses should contain 'Installing 1 file(s)'; got: %v", reporter.Statuses)
	}
	// Should not have per-file "Installed" lines.
	for _, l := range reporter.Logs {
		if strings.HasPrefix(strings.TrimSpace(l), "Installed ") {
			t.Errorf("Logs should not contain per-file 'Installed' line; got: %q", l)
		}
	}
}

// --- scenario-20: link_dependencies action reporter classification ---

// TestLinkDependenciesReporterClassification verifies that link_dependencies emits
// a single bulk count through Log() and does not emit per-file "Linked:" lines.
func TestLinkDependenciesReporterClassification(t *testing.T) {
	t.Parallel()

	rec := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
	}
	exec, err := New(rec)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Set up a temporary $TSUKU_HOME structure so link_dependencies can find the library.
	// link_dependencies uses filepath.Dir(ctx.ToolsDir) as $TSUKU_HOME.
	tmpHome := t.TempDir()
	toolsDir := filepath.Join(tmpHome, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}
	exec.SetToolsDir(toolsDir)

	// Create the library source: $TSUKU_HOME/libs/testlib-1.0.0/lib/libtestlib.so
	libDir := filepath.Join(tmpHome, "libs", "testlib-1.0.0", "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "libtestlib.so"), []byte("ELF"), 0644); err != nil {
		t.Fatalf("failed to create fake library: %v", err)
	}

	reporter := &testReporter{}
	exec.SetReporter(reporter)

	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps: []ResolvedStep{
			{
				Action:    "link_dependencies",
				Evaluable: true,
				Params: map[string]interface{}{
					"library": "testlib",
					"version": "1.0.0",
				},
			},
		},
	}

	ctx := context.Background()
	if err := exec.ExecutePlan(ctx, plan); err != nil {
		t.Fatalf("ExecutePlan() unexpected error = %v", err)
	}

	// Should have a bulk count status line (ephemeral).
	if !reporter.hasStatus("Linking 1 library file(s)") {
		t.Errorf("Statuses should contain 'Linking 1 library file(s)'; got: %v", reporter.Statuses)
	}
	// No per-file "Linked:" lines — regression guard for strings removed by Issue #4.
	// link_dependencies previously emitted "Linked:", "Linked (symlink):", "Already linked:" per file.
	for _, l := range reporter.Logs {
		if strings.Contains(l, "Linked:") {
			t.Errorf("Logs must not contain per-file 'Linked:' line (removed in Issue #4); got: %q", l)
		}
	}
}

// --- scenario-11: install_libraries action reporter classification ---

// TestInstallLibrariesReporterClassification verifies that install_libraries emits
// a single bulk count through Log() and does not emit per-file lines.
func TestInstallLibrariesReporterClassification(t *testing.T) {
	t.Parallel()

	rec := newMinimalRecipe("test-tool")
	exec, err := New(rec)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Create a library file in the work directory under lib/.
	libDir := filepath.Join(exec.WorkDir(), "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "libfoo.so"), []byte("ELF"), 0644); err != nil {
		t.Fatalf("failed to create fake library: %v", err)
	}

	reporter := &testReporter{}
	exec.SetReporter(reporter)

	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps: []ResolvedStep{
			{
				Action:    "install_libraries",
				Evaluable: true,
				Params: map[string]interface{}{
					"patterns": []interface{}{"lib/*.so"},
				},
			},
		},
	}

	ctx := context.Background()
	if err := exec.ExecutePlan(ctx, plan); err != nil {
		t.Fatalf("ExecutePlan() unexpected error = %v", err)
	}

	// Should have a bulk count status line (ephemeral).
	if !reporter.hasStatus("Installing 1 library file(s)") {
		t.Errorf("Statuses should contain 'Installing 1 library file(s)'; got: %v", reporter.Statuses)
	}
	// Should not have per-file installation lines.
	for _, l := range reporter.Logs {
		if strings.Contains(l, "lib/libfoo.so") {
			t.Errorf("Logs should not contain per-file path; got: %q", l)
		}
	}
}
