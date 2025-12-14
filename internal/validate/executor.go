package validate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// DefaultValidationImage is the container image used for validation.
// Using Debian because the tsuku binary is dynamically linked against glibc.
const DefaultValidationImage = "debian:bookworm-slim"

// findTsukuBinary locates a valid tsuku binary for container execution.
// It first checks os.Executable(), verifying the binary name looks correct.
// If running in a test context (binary ends in .test), it looks for tsuku in PATH.
// Returns empty string if no valid binary is found.
func findTsukuBinary() string {
	// Try the current executable first
	if exePath, err := os.Executable(); err == nil {
		baseName := filepath.Base(exePath)
		// Check if this looks like the real tsuku binary (not a test binary)
		if baseName == "tsuku" || baseName == "tsuku.exe" {
			return exePath
		}
	}

	// Current executable is not tsuku (likely a test binary)
	// Try to find tsuku in PATH
	if tsukuPath, err := exec.LookPath("tsuku"); err == nil {
		return tsukuPath
	}

	// No valid tsuku binary found - validation will skip or fail gracefully
	return ""
}

// ValidationResult contains the result of a recipe validation.
type ValidationResult struct {
	Passed   bool   // Whether validation succeeded
	Skipped  bool   // Whether validation was skipped (no runtime)
	ExitCode int    // Container exit code
	Stdout   string // Container stdout
	Stderr   string // Container stderr
	Error    error  // Error if validation failed to run
}

// Executor orchestrates container-based recipe validation.
// It combines runtime detection, asset pre-download, and isolated container execution.
type Executor struct {
	detector      *RuntimeDetector
	predownloader *PreDownloader
	logger        log.Logger
	image         string
	limits        ResourceLimits
	tsukuBinary   string // Path to tsuku binary for container execution
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithExecutorLogger sets a logger for executor warnings.
func WithExecutorLogger(logger log.Logger) ExecutorOption {
	return func(e *Executor) {
		e.logger = logger
	}
}

// WithValidationImage sets the container image for validation.
func WithValidationImage(image string) ExecutorOption {
	return func(e *Executor) {
		e.image = image
	}
}

// WithResourceLimits sets resource limits for validation containers.
func WithResourceLimits(limits ResourceLimits) ExecutorOption {
	return func(e *Executor) {
		e.limits = limits
	}
}

// WithTsukuBinary sets the path to the tsuku binary for container execution.
func WithTsukuBinary(path string) ExecutorOption {
	return func(e *Executor) {
		e.tsukuBinary = path
	}
}

// NewExecutor creates a new Executor with the given dependencies.
func NewExecutor(detector *RuntimeDetector, predownloader *PreDownloader, opts ...ExecutorOption) *Executor {
	// Auto-detect tsuku binary path
	tsukuPath := findTsukuBinary()

	e := &Executor{
		detector:      detector,
		predownloader: predownloader,
		logger:        log.NewNoop(),
		image:         DefaultValidationImage,
		tsukuBinary:   tsukuPath,
		limits: ResourceLimits{
			Memory:   "2g",
			CPUs:     "2",
			PidsMax:  100,
			ReadOnly: true,
		},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Validate runs a recipe in an isolated container and checks the verification command.
// It returns a ValidationResult indicating whether the recipe installed correctly.
//
// The validation process:
// 1. Detect available container runtime
// 2. Serialize recipe to TOML file
// 3. Mount tsuku binary and recipe into container
// 4. Run tsuku install in isolated container
// 5. Run verification command
// 6. Check output against expected pattern
func (e *Executor) Validate(ctx context.Context, r *recipe.Recipe, assetURL string) (*ValidationResult, error) {
	// Detect container runtime
	runtime, err := e.detector.Detect(ctx)
	if err != nil {
		if err == ErrNoRuntime {
			e.logger.Warn("Container runtime not available. Skipping recipe validation.",
				"hint", "To enable validation, install Podman or Docker.")
			return &ValidationResult{
				Skipped: true,
			}, nil
		}
		return nil, fmt.Errorf("failed to detect container runtime: %w", err)
	}

	// Check if we have a valid tsuku binary
	if e.tsukuBinary == "" {
		e.logger.Warn("Tsuku binary not found. Skipping recipe validation.",
			"hint", "Ensure tsuku is installed and in PATH, or build with 'go build -o tsuku ./cmd/tsuku'")
		return &ValidationResult{
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

	e.logger.Debug("Using container runtime", "runtime", runtime.Name(), "rootless", runtime.IsRootless())

	// Create workspace directory
	workspaceDir, err := os.MkdirTemp("", TempDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}
	defer os.RemoveAll(workspaceDir)

	// Serialize recipe to TOML using custom method
	recipeData, err := r.ToTOML()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize recipe: %w", err)
	}

	// Write recipe to workspace
	recipePath := filepath.Join(workspaceDir, "recipe.toml")
	if err := os.WriteFile(recipePath, recipeData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write recipe file: %w", err)
	}

	// Build the validation script that runs tsuku install
	script := e.buildTsukuInstallScript(r)

	// Create the install script in workspace
	scriptPath := filepath.Join(workspaceDir, "validate.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return nil, fmt.Errorf("failed to write validation script: %w", err)
	}

	// Build run options
	// Override ReadOnly to false since we need to install packages
	limits := e.limits
	limits.ReadOnly = false

	opts := RunOptions{
		Image:   e.image,
		Command: []string{"/bin/sh", "/workspace/validate.sh"},
		Network: "host", // Need network for downloads
		WorkDir: "/workspace",
		Env: []string{
			"TSUKU_VALIDATION=1",
			"TSUKU_HOME=/workspace/tsuku",
			"HOME=/workspace",
		},
		Limits: limits,
		Labels: map[string]string{
			ContainerLabelPrefix: "true",
		},
		Mounts: []Mount{
			{
				Source:   workspaceDir,
				Target:   "/workspace",
				ReadOnly: false,
			},
		},
	}

	// Mount tsuku binary if available
	if e.tsukuBinary != "" {
		opts.Mounts = append(opts.Mounts, Mount{
			Source:   e.tsukuBinary,
			Target:   "/usr/local/bin/tsuku",
			ReadOnly: true,
		})
	}

	// Run the container
	result, err := runtime.Run(ctx, opts)
	if err != nil {
		return &ValidationResult{
			Passed:   false,
			ExitCode: -1,
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			Error:    err,
		}, nil
	}

	// Check if verification passed
	passed := e.checkVerification(r, result)

	return &ValidationResult{
		Passed:   passed,
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
}

// buildTsukuInstallScript creates a shell script that runs tsuku install with the recipe.
func (e *Executor) buildTsukuInstallScript(r *recipe.Recipe) string {
	var sb strings.Builder

	sb.WriteString("#!/bin/sh\n")
	sb.WriteString("set -e\n\n")

	// Install ca-certificates for HTTPS downloads
	sb.WriteString("# Install required packages\n")
	sb.WriteString("apt-get update -qq && apt-get install -qq -y ca-certificates >/dev/null 2>&1 || true\n\n")

	// Setup tsuku home directory
	sb.WriteString("# Setup TSUKU_HOME\n")
	sb.WriteString("mkdir -p /workspace/tsuku/recipes\n")
	sb.WriteString("mkdir -p /workspace/tsuku/bin\n")
	sb.WriteString("mkdir -p /workspace/tsuku/tools\n\n")

	// Copy recipe to tsuku recipes directory
	sb.WriteString("# Copy recipe to tsuku recipes\n")
	sb.WriteString(fmt.Sprintf("cp /workspace/recipe.toml /workspace/tsuku/recipes/%s.toml\n\n", r.Metadata.Name))

	// Run tsuku install
	sb.WriteString("# Run tsuku install\n")
	sb.WriteString(fmt.Sprintf("tsuku install %s --force\n\n", r.Metadata.Name))

	// Run the verify command explicitly to capture its output for pattern matching.
	// The install command doesn't print verify output, so we need to run it separately.
	// Binaries are symlinked to $TSUKU_HOME/tools/current (/workspace/tsuku/tools/current).
	if r.Verify.Command != "" {
		sb.WriteString("# Run verify command to capture output for pattern matching\n")
		sb.WriteString("export PATH=\"/workspace/tsuku/tools/current:$PATH\"\n")
		sb.WriteString(fmt.Sprintf("%s\n", r.Verify.Command))
	}

	return sb.String()
}

// checkVerification checks if the verification output matches expectations.
func (e *Executor) checkVerification(r *recipe.Recipe, result *RunResult) bool {
	// If exit code is non-zero, verification failed
	expectedExitCode := 0
	if r.Verify.ExitCode != nil {
		expectedExitCode = *r.Verify.ExitCode
	}
	if result.ExitCode != expectedExitCode {
		return false
	}

	// If no pattern specified, just check exit code
	if r.Verify.Pattern == "" {
		return true
	}

	// Check if pattern appears in stdout or stderr
	output := result.Stdout + result.Stderr
	return strings.Contains(output, r.Verify.Pattern)
}

// GetAssetChecksum returns the SHA256 checksum of a downloaded asset.
// This is useful for embedding checksums in generated recipes.
func (e *Executor) GetAssetChecksum(ctx context.Context, url string) (string, error) {
	result, err := e.predownloader.Download(ctx, url)
	if err != nil {
		return "", err
	}
	defer func() { _ = result.Cleanup() }()
	return result.Checksum, nil
}
