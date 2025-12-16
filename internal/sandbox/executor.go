package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// TempDirPrefix is the prefix for temporary directories created by the sandbox.
const TempDirPrefix = "tsuku-sandbox-"

// ContainerLabelPrefix is the label added to containers for identification.
const ContainerLabelPrefix = "io.tsuku.sandbox"

// SandboxResult contains the result of a sandbox test.
type SandboxResult struct {
	Passed   bool   // Whether the sandbox test succeeded
	Skipped  bool   // Whether the test was skipped (no runtime)
	ExitCode int    // Container exit code
	Stdout   string // Container stdout
	Stderr   string // Container stderr
	Error    error  // Error if sandbox failed to run
}

// Executor orchestrates container-based sandbox testing.
// It uses SandboxRequirements to configure containers appropriately
// for different types of installations (binary, source build, ecosystem).
type Executor struct {
	detector         *validate.RuntimeDetector
	logger           log.Logger
	tsukuBinary      string // Path to tsuku binary for container execution
	downloadCacheDir string // External download cache directory to mount
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithLogger sets a logger for executor messages.
func WithLogger(logger log.Logger) ExecutorOption {
	return func(e *Executor) {
		e.logger = logger
	}
}

// WithTsukuBinary sets the path to the tsuku binary for container execution.
func WithTsukuBinary(path string) ExecutorOption {
	return func(e *Executor) {
		e.tsukuBinary = path
	}
}

// WithDownloadCacheDir sets the download cache directory to mount into containers.
// This directory should contain pre-downloaded files from plan generation.
func WithDownloadCacheDir(path string) ExecutorOption {
	return func(e *Executor) {
		e.downloadCacheDir = path
	}
}

// NewExecutor creates a new Executor with the given runtime detector.
func NewExecutor(detector *validate.RuntimeDetector, opts ...ExecutorOption) *Executor {
	// Auto-detect tsuku binary path
	tsukuPath := findTsukuBinary()

	e := &Executor{
		detector:    detector,
		logger:      log.NewNoop(),
		tsukuBinary: tsukuPath,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// findTsukuBinary locates a valid tsuku binary for container execution.
func findTsukuBinary() string {
	// Try the current executable first
	if exePath, err := os.Executable(); err == nil {
		baseName := filepath.Base(exePath)
		if baseName == "tsuku" || baseName == "tsuku.exe" {
			return exePath
		}
	}

	// Try to find tsuku in PATH
	if tsukuPath, err := exec.LookPath("tsuku"); err == nil {
		return tsukuPath
	}

	return ""
}

// Sandbox runs an installation plan in an isolated container.
// It uses the provided SandboxRequirements to configure the container
// with appropriate image, network access, and resource limits.
//
// The sandbox process:
// 1. Detect available container runtime
// 2. Write plan JSON to workspace
// 3. Generate sandbox script based on requirements
// 4. Mount tsuku binary, plan, and cache into container
// 5. Run container with configured limits
// 6. Check verification output
func (e *Executor) Sandbox(
	ctx context.Context,
	plan *executor.InstallationPlan,
	reqs *SandboxRequirements,
) (*SandboxResult, error) {
	// Detect container runtime
	runtime, err := e.detector.Detect(ctx)
	if err != nil {
		if err == validate.ErrNoRuntime {
			e.logger.Warn("Container runtime not available. Skipping sandbox test.",
				"hint", "To enable sandbox testing, install Podman or Docker.")
			return &SandboxResult{
				Skipped: true,
			}, nil
		}
		return nil, fmt.Errorf("failed to detect container runtime: %w", err)
	}

	// Check if we have a valid tsuku binary
	if e.tsukuBinary == "" {
		e.logger.Warn("Tsuku binary not found. Skipping sandbox test.",
			"hint", "Ensure tsuku is installed and in PATH, or build with 'go build -o tsuku ./cmd/tsuku'")
		return &SandboxResult{
			Skipped: true,
		}, nil
	}

	// Emit security warning for Docker with group membership (non-rootless)
	if runtime.Name() == "docker" && !runtime.IsRootless() {
		e.logger.Warn("Using Docker with docker group membership.",
			"security", "This grants root-equivalent access on this machine.",
			"recommendation", "Consider configuring Docker rootless mode for better security.",
			"docs", "https://docs.docker.com/engine/security/rootless/")
	}

	e.logger.Debug("Running sandbox test",
		"tool", plan.Tool,
		"runtime", runtime.Name(),
		"image", reqs.Image,
		"network", reqs.RequiresNetwork)

	// Create workspace directory
	workspaceDir, err := os.MkdirTemp("", TempDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspaceDir) }()

	// Use external download cache if provided, otherwise create empty one
	// External cache should contain pre-downloaded files from plan generation
	cacheDir := e.downloadCacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(workspaceDir, "cache", "downloads")
		if err := os.MkdirAll(cacheDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	// Write plan JSON to workspace
	planData, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize plan: %w", err)
	}
	planPath := filepath.Join(workspaceDir, "plan.json")
	if err := os.WriteFile(planPath, planData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write plan file: %w", err)
	}

	// Build the sandbox script
	script := e.buildSandboxScript(plan, reqs)

	// Write script to workspace
	scriptPath := filepath.Join(workspaceDir, "sandbox.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return nil, fmt.Errorf("failed to write sandbox script: %w", err)
	}

	// Configure network mode
	network := "none"
	if reqs.RequiresNetwork {
		network = "host"
	}

	// Convert sandbox.ResourceLimits to validate.ResourceLimits
	limits := validate.ResourceLimits{
		Memory:   reqs.Resources.Memory,
		CPUs:     reqs.Resources.CPUs,
		PidsMax:  reqs.Resources.PidsMax,
		Timeout:  reqs.Resources.Timeout,
		ReadOnly: false, // Need to install packages
	}

	// Build run options
	opts := validate.RunOptions{
		Image:   reqs.Image,
		Command: []string{"/bin/bash", "/workspace/sandbox.sh"},
		Network: network,
		WorkDir: "/workspace",
		Env: []string{
			"TSUKU_SANDBOX=1",
			"TSUKU_HOME=/workspace/tsuku",
			"HOME=/workspace",
			"DEBIAN_FRONTEND=noninteractive",
		},
		Limits: limits,
		Labels: map[string]string{
			ContainerLabelPrefix: "true",
		},
		Mounts: []validate.Mount{
			{
				Source:   workspaceDir,
				Target:   "/workspace",
				ReadOnly: false,
			},
			{
				Source:   cacheDir,
				Target:   "/workspace/tsuku/cache/downloads",
				ReadOnly: true,
			},
		},
	}

	// Mount tsuku binary if available
	if e.tsukuBinary != "" {
		opts.Mounts = append(opts.Mounts, validate.Mount{
			Source:   e.tsukuBinary,
			Target:   "/usr/local/bin/tsuku",
			ReadOnly: true,
		})
	}

	// Run the container
	result, err := runtime.Run(ctx, opts)
	if err != nil {
		return &SandboxResult{
			Passed:   false,
			ExitCode: -1,
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			Error:    err,
		}, nil
	}

	// Check if verification passed (exit code 0 for now)
	passed := result.ExitCode == 0

	return &SandboxResult{
		Passed:   passed,
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
}

// buildSandboxScript creates the shell script for sandbox testing.
// The script sets up the environment and runs tsuku install --plan.
//
// Note: Build tools are NOT installed via apt-get. Instead, tsuku's normal
// dependency resolution handles them via ActionDependencies.InstallTime.
func (e *Executor) buildSandboxScript(
	plan *executor.InstallationPlan,
	reqs *SandboxRequirements,
) string {
	var sb strings.Builder

	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("set -e\n\n")

	// Minimal system setup for network-enabled builds
	if reqs.RequiresNetwork {
		sb.WriteString("# Minimal network setup (ca-certificates for HTTPS)\n")
		sb.WriteString("apt-get update -qq\n")
		sb.WriteString("apt-get install -qq -y ca-certificates curl >/dev/null 2>&1\n\n")
	}

	// Setup TSUKU_HOME directory structure
	sb.WriteString("# Setup TSUKU_HOME\n")
	sb.WriteString("mkdir -p /workspace/tsuku/recipes\n")
	sb.WriteString("mkdir -p /workspace/tsuku/bin\n")
	sb.WriteString("mkdir -p /workspace/tsuku/tools\n\n")

	// Add $TSUKU_HOME/bin to PATH so dependency binaries are available
	// This is needed when plans include dependency steps (e.g., nodejs for npm_exec)
	sb.WriteString("# Add TSUKU_HOME/bin to PATH for dependency binaries\n")
	sb.WriteString("export PATH=/workspace/tsuku/bin:$PATH\n\n")

	// Run tsuku install with pre-generated plan
	// tsuku handles build tool dependencies automatically via ActionDependencies
	sb.WriteString("# Run tsuku install with pre-generated plan\n")
	sb.WriteString("tsuku install --plan /workspace/plan.json --force\n")

	return sb.String()
}
