package validate

import (
	"context"
	"errors"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// mockRuntime is a mock Runtime for testing.
type mockRuntime struct {
	name     string
	rootless bool
	runFunc  func(ctx context.Context, opts RunOptions) (*RunResult, error)
}

func (m *mockRuntime) Name() string {
	return m.name
}

func (m *mockRuntime) IsRootless() bool {
	return m.rootless
}

func (m *mockRuntime) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, opts)
	}
	return &RunResult{ExitCode: 0}, nil
}

// testLogger captures log messages for testing.
type testLogger struct {
	warnings []string
	debugs   []string
}

func (l *testLogger) Warn(msg string, args ...any) {
	l.warnings = append(l.warnings, msg)
}

func (l *testLogger) Debug(msg string, args ...any) {
	l.debugs = append(l.debugs, msg)
}

func TestExecutor_Validate_NoRuntime(t *testing.T) {
	// Create detector that returns no runtime
	detector := NewRuntimeDetector()
	detector.lookPath = func(string) (string, error) {
		return "", errors.New("not found")
	}

	logger := &testLogger{}
	executor := NewExecutor(detector, NewPreDownloader(), WithExecutorLogger(logger))

	r := &recipe.Recipe{
		Verify: recipe.VerifySection{
			Command: "test --version",
		},
	}

	result, err := executor.Validate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !result.Skipped {
		t.Error("expected validation to be skipped")
	}

	// Check warning was logged
	if len(logger.warnings) == 0 {
		t.Error("expected warning about missing runtime")
	}
}

func TestExecutor_Validate_DockerGroupWarning(t *testing.T) {
	// Create mock docker runtime (non-rootless)
	mockDocker := &mockRuntime{
		name:     "docker",
		rootless: false,
		runFunc: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			return &RunResult{
				ExitCode: 0,
				Stdout:   "test 1.0.0",
			}, nil
		},
	}

	// Create detector that returns docker
	detector := NewRuntimeDetector()
	detector.lookPath = func(name string) (string, error) {
		if name == "docker" {
			return "/usr/bin/docker", nil
		}
		return "", errors.New("not found")
	}
	detector.cmdRun = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		// Not rootless
		return []byte("no-rootless"), nil
	}

	// We need to manually set the detected runtime since our mock detector
	// won't properly detect our mock runtime
	detector.detected = mockDocker
	detector.checked = true

	logger := &testLogger{}
	executor := NewExecutor(detector, NewPreDownloader(), WithExecutorLogger(logger))

	r := &recipe.Recipe{
		Verify: recipe.VerifySection{
			Command: "test --version",
			Pattern: "1.0.0",
		},
	}

	_, err := executor.Validate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check docker group warning was logged
	found := false
	for _, w := range logger.warnings {
		if w == "Using Docker with docker group membership." {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected docker group warning, got warnings: %v", logger.warnings)
	}
}

func TestExecutor_Validate_Success(t *testing.T) {
	mockPodman := &mockRuntime{
		name:     "podman",
		rootless: true,
		runFunc: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			// Verify options - network=host is needed for downloads, ReadOnly=false for package installs
			if opts.Network != "host" {
				t.Errorf("expected network=host, got %s", opts.Network)
			}
			if opts.Limits.ReadOnly {
				t.Error("expected writable filesystem for tsuku install")
			}
			return &RunResult{
				ExitCode: 0,
				Stdout:   "mytool version 1.2.3",
			}, nil
		},
	}

	detector := NewRuntimeDetector()
	detector.detected = mockPodman
	detector.checked = true

	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Verify: recipe.VerifySection{
			Command: "mytool --version",
			Pattern: "1.2.3",
		},
	}

	result, err := executor.Validate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Passed {
		t.Error("expected validation to pass")
	}
	if result.Skipped {
		t.Error("expected validation not to be skipped")
	}
}

func TestExecutor_Validate_VerificationFails(t *testing.T) {
	mockPodman := &mockRuntime{
		name:     "podman",
		rootless: true,
		runFunc: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			return &RunResult{
				ExitCode: 1,
				Stderr:   "command not found",
			}, nil
		},
	}

	detector := NewRuntimeDetector()
	detector.detected = mockPodman
	detector.checked = true

	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Verify: recipe.VerifySection{
			Command: "mytool --version",
			Pattern: "1.2.3",
		},
	}

	result, err := executor.Validate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Passed {
		t.Error("expected validation to fail")
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
}

func TestExecutor_Validate_PatternMismatch(t *testing.T) {
	mockPodman := &mockRuntime{
		name:     "podman",
		rootless: true,
		runFunc: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			return &RunResult{
				ExitCode: 0,
				Stdout:   "mytool version 2.0.0",
			}, nil
		},
	}

	detector := NewRuntimeDetector()
	detector.detected = mockPodman
	detector.checked = true

	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Verify: recipe.VerifySection{
			Command: "mytool --version",
			Pattern: "1.2.3", // Looking for 1.2.3 but output has 2.0.0
		},
	}

	result, err := executor.Validate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Passed {
		t.Error("expected validation to fail due to pattern mismatch")
	}
}

func TestExecutor_Validate_ContainerError(t *testing.T) {
	mockPodman := &mockRuntime{
		name:     "podman",
		rootless: true,
		runFunc: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			return &RunResult{
				ExitCode: -1,
				Stderr:   "container failed to start",
			}, errors.New("container execution failed")
		},
	}

	detector := NewRuntimeDetector()
	detector.detected = mockPodman
	detector.checked = true

	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Verify: recipe.VerifySection{
			Command: "mytool --version",
		},
	}

	result, err := executor.Validate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Passed {
		t.Error("expected validation to fail")
	}
	if result.Error == nil {
		t.Error("expected error to be set")
	}
}

func TestExecutor_Validate_CustomExitCode(t *testing.T) {
	mockPodman := &mockRuntime{
		name:     "podman",
		rootless: true,
		runFunc: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			return &RunResult{
				ExitCode: 2,
				Stdout:   "expected output",
			}, nil
		},
	}

	detector := NewRuntimeDetector()
	detector.detected = mockPodman
	detector.checked = true

	executor := NewExecutor(detector, NewPreDownloader())

	exitCode := 2
	r := &recipe.Recipe{
		Verify: recipe.VerifySection{
			Command:  "mytool --check",
			ExitCode: &exitCode, // Expect exit code 2
			Pattern:  "expected",
		},
	}

	result, err := executor.Validate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Passed {
		t.Error("expected validation to pass with custom exit code")
	}
}

func TestExecutor_Validate_NoPattern(t *testing.T) {
	mockPodman := &mockRuntime{
		name:     "podman",
		rootless: true,
		runFunc: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			return &RunResult{
				ExitCode: 0,
				Stdout:   "any output",
			}, nil
		},
	}

	detector := NewRuntimeDetector()
	detector.detected = mockPodman
	detector.checked = true

	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Verify: recipe.VerifySection{
			Command: "mytool --version",
			// No pattern - just check exit code
		},
	}

	result, err := executor.Validate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Passed {
		t.Error("expected validation to pass with no pattern requirement")
	}
}

func TestExecutor_Validate_MountsAndLabels(t *testing.T) {
	var capturedOpts RunOptions

	mockPodman := &mockRuntime{
		name:     "podman",
		rootless: true,
		runFunc: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			capturedOpts = opts
			return &RunResult{ExitCode: 0}, nil
		},
	}

	detector := NewRuntimeDetector()
	detector.detected = mockPodman
	detector.checked = true

	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Verify: recipe.VerifySection{
			Command: "test",
		},
	}

	_, err := executor.Validate(context.Background(), r, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that workspace mount exists
	foundWorkspace := false
	for _, m := range capturedOpts.Mounts {
		if m.Target == "/workspace" {
			foundWorkspace = true
			if m.ReadOnly {
				t.Error("workspace should not be read-only")
			}
		}
	}
	if !foundWorkspace {
		t.Error("expected workspace mount")
	}

	// Check label
	if capturedOpts.Labels[ContainerLabelPrefix] != "true" {
		t.Error("expected container label for cleanup")
	}
}

func TestExecutor_WithOptions(t *testing.T) {
	detector := NewRuntimeDetector()
	predownloader := NewPreDownloader()
	logger := &testLogger{}

	executor := NewExecutor(detector, predownloader,
		WithExecutorLogger(logger),
		WithValidationImage("custom:image"),
		WithResourceLimits(ResourceLimits{
			Memory:  "4g",
			CPUs:    "4",
			PidsMax: 200,
		}),
	)

	if executor.image != "custom:image" {
		t.Errorf("expected custom:image, got %s", executor.image)
	}
	if executor.limits.Memory != "4g" {
		t.Errorf("expected 4g memory, got %s", executor.limits.Memory)
	}
	if executor.limits.CPUs != "4" {
		t.Errorf("expected 4 cpus, got %s", executor.limits.CPUs)
	}
	if executor.limits.PidsMax != 200 {
		t.Errorf("expected 200 pids, got %d", executor.limits.PidsMax)
	}
}
