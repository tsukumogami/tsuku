package validate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// DefaultValidationImage is the container image used for validation.
const DefaultValidationImage = "alpine:latest"

// ValidationResult contains the result of a recipe validation.
type ValidationResult struct {
	Passed   bool   // Whether validation succeeded
	Skipped  bool   // Whether validation was skipped (no runtime)
	ExitCode int    // Container exit code
	Stdout   string // Container stdout
	Stderr   string // Container stderr
	Error    error  // Error if validation failed to run
}

// ExecutorLogger defines the logging interface for executor warnings.
type ExecutorLogger interface {
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)
}

// noopExecutorLogger is a logger that discards all messages.
type noopExecutorLogger struct{}

func (noopExecutorLogger) Warn(string, ...any)  {}
func (noopExecutorLogger) Debug(string, ...any) {}

// Executor orchestrates container-based recipe validation.
// It combines runtime detection, asset pre-download, and isolated container execution.
type Executor struct {
	detector      *RuntimeDetector
	predownloader *PreDownloader
	logger        ExecutorLogger
	image         string
	limits        ResourceLimits
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithExecutorLogger sets a logger for executor warnings.
func WithExecutorLogger(logger ExecutorLogger) ExecutorOption {
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

// NewExecutor creates a new Executor with the given dependencies.
func NewExecutor(detector *RuntimeDetector, predownloader *PreDownloader, opts ...ExecutorOption) *Executor {
	e := &Executor{
		detector:      detector,
		predownloader: predownloader,
		logger:        noopExecutorLogger{},
		image:         DefaultValidationImage,
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
// 2. Download assets with checksums (if assetURL provided)
// 3. Run recipe steps in isolated container
// 4. Run verification command
// 5. Check output against expected pattern
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

	// Emit security warning for Docker with group membership (non-rootless)
	if runtime.Name() == "docker" && !runtime.IsRootless() {
		e.logger.Warn("Using Docker with docker group membership.",
			"security", "This grants root-equivalent access on this machine.",
			"recommendation", "Consider configuring Docker rootless mode for better security.",
			"docs", "https://docs.docker.com/engine/security/rootless/")
	}

	e.logger.Debug("Using container runtime", "runtime", runtime.Name(), "rootless", runtime.IsRootless())

	// Download asset if URL provided
	var assetPath string
	var downloadResult *DownloadResult
	if assetURL != "" {
		downloadResult, err = e.predownloader.Download(ctx, assetURL)
		if err != nil {
			return &ValidationResult{
				Passed: false,
				Error:  fmt.Errorf("failed to download asset: %w", err),
			}, nil
		}
		defer downloadResult.Cleanup()
		assetPath = filepath.Dir(downloadResult.AssetPath)
		e.logger.Debug("Downloaded asset", "path", downloadResult.AssetPath, "checksum", downloadResult.Checksum)
	}

	// Create workspace directory
	workspaceDir, err := os.MkdirTemp("", TempDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}
	defer os.RemoveAll(workspaceDir)

	// Build the validation script
	script := e.buildValidationScript(r)

	// Create the install script in workspace
	scriptPath := filepath.Join(workspaceDir, "validate.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return nil, fmt.Errorf("failed to write validation script: %w", err)
	}

	// Build run options
	opts := RunOptions{
		Image:   e.image,
		Command: []string{"/bin/sh", "/workspace/validate.sh"},
		Network: "none",
		WorkDir: "/workspace",
		Env:     []string{"TSUKU_VALIDATION=1"},
		Limits:  e.limits,
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

	// Mount assets if available
	if assetPath != "" {
		opts.Mounts = append(opts.Mounts, Mount{
			Source:   assetPath,
			Target:   "/assets",
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

// buildValidationScript creates a shell script that runs recipe steps and verification.
func (e *Executor) buildValidationScript(r *recipe.Recipe) string {
	var sb strings.Builder

	sb.WriteString("#!/bin/sh\n")
	sb.WriteString("set -e\n\n")

	// Add environment setup
	sb.WriteString("# Setup environment\n")
	sb.WriteString("export PATH=\"/workspace/bin:$PATH\"\n")
	sb.WriteString("mkdir -p /workspace/bin\n\n")

	// For simplicity in slice 2, we handle the most common case:
	// Download action with a binary that needs to be made executable
	sb.WriteString("# Copy asset to workspace if available\n")
	sb.WriteString("if [ -d /assets ]; then\n")
	sb.WriteString("  cp /assets/* /workspace/ 2>/dev/null || true\n")
	sb.WriteString("fi\n\n")

	// Make binaries executable
	sb.WriteString("# Make binaries executable\n")
	sb.WriteString("chmod +x /workspace/* 2>/dev/null || true\n\n")

	// Run verification command
	sb.WriteString("# Run verification\n")
	if r.Verify.Command != "" {
		// Handle the verification command
		// The command may reference the binary directly
		sb.WriteString(fmt.Sprintf("%s\n", r.Verify.Command))
	} else {
		sb.WriteString("echo 'No verification command specified'\n")
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
	defer result.Cleanup()
	return result.Checksum, nil
}
