package validate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// SourceBuildValidationImage is the container image used for source build validation.
// Uses Ubuntu as it has better package availability for build tools.
const SourceBuildValidationImage = "ubuntu:22.04"

// SourceBuildLimits returns resource limits suitable for source builds.
// Source builds need more time and resources than bottle extraction.
func SourceBuildLimits() ResourceLimits {
	return ResourceLimits{
		Memory:   "4g",
		CPUs:     "4",
		PidsMax:  500,
		ReadOnly: false,
		Timeout:  15 * time.Minute,
	}
}

// ValidateSourceBuild runs a source build recipe in an isolated container.
// Unlike bottle validation, this:
// - Uses a larger image with build tool support
// - Has longer timeouts for compilation
// - Installs build dependencies based on the recipe's build system
//
// The validation process:
// 1. Detect available container runtime
// 2. Serialize recipe to TOML file
// 3. Mount tsuku binary and recipe into container
// 4. Install required build tools based on build system
// 5. Run tsuku install which executes the build steps
// 6. Verify expected binaries are produced
func (e *Executor) ValidateSourceBuild(ctx context.Context, r *recipe.Recipe) (*ValidationResult, error) {
	// Detect container runtime
	runtime, err := e.detector.Detect(ctx)
	if err != nil {
		if err == ErrNoRuntime {
			e.logger.Warn("Container runtime not available. Skipping source build validation.",
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

	e.logger.Debug("Validating source build", "recipe", r.Metadata.Name, "runtime", runtime.Name())

	// Create workspace directory
	workspaceDir, err := os.MkdirTemp("", TempDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}
	defer os.RemoveAll(workspaceDir)

	// Serialize recipe to TOML
	recipeData, err := r.ToTOML()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize recipe: %w", err)
	}

	// Write recipe to workspace
	recipePath := filepath.Join(workspaceDir, "recipe.toml")
	if err := os.WriteFile(recipePath, recipeData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write recipe file: %w", err)
	}

	// Build the validation script for source builds
	script := e.buildSourceBuildScript(r)

	// Create the install script in workspace
	scriptPath := filepath.Join(workspaceDir, "validate.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return nil, fmt.Errorf("failed to write validation script: %w", err)
	}

	// Use source build limits
	limits := SourceBuildLimits()

	opts := RunOptions{
		Image:   SourceBuildValidationImage,
		Command: []string{"/bin/bash", "/workspace/validate.sh"},
		Network: "host", // Need network for downloads and potential dependency fetches
		WorkDir: "/workspace",
		Env: []string{
			"TSUKU_VALIDATION=1",
			"TSUKU_HOME=/workspace/tsuku",
			"HOME=/workspace",
			"DEBIAN_FRONTEND=noninteractive",
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

// buildSourceBuildScript creates a shell script for source build validation.
// It installs build tools based on the actions in the recipe.
func (e *Executor) buildSourceBuildScript(r *recipe.Recipe) string {
	var sb strings.Builder

	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("set -e\n\n")

	// Update package lists and install base requirements
	sb.WriteString("# Update package lists and install base requirements\n")
	sb.WriteString("apt-get update -qq\n")
	sb.WriteString("apt-get install -qq -y ca-certificates curl wget >/dev/null 2>&1\n\n")

	// Install build tools based on recipe actions
	buildTools := e.detectRequiredBuildTools(r)
	if len(buildTools) > 0 {
		sb.WriteString("# Install build tools required by recipe\n")
		sb.WriteString(fmt.Sprintf("apt-get install -qq -y %s >/dev/null 2>&1\n\n", strings.Join(buildTools, " ")))
	}

	// Setup tsuku home directory
	sb.WriteString("# Setup TSUKU_HOME\n")
	sb.WriteString("mkdir -p /workspace/tsuku/recipes\n")
	sb.WriteString("mkdir -p /workspace/tsuku/bin\n")
	sb.WriteString("mkdir -p /workspace/tsuku/tools\n\n")

	// Copy recipe to tsuku recipes directory
	sb.WriteString("# Copy recipe to tsuku recipes\n")
	sb.WriteString(fmt.Sprintf("cp /workspace/recipe.toml /workspace/tsuku/recipes/%s.toml\n\n", r.Metadata.Name))

	// Run tsuku install (which executes the build steps and verification)
	sb.WriteString("# Run tsuku install (executes build steps)\n")
	sb.WriteString(fmt.Sprintf("tsuku install %s --force\n", r.Metadata.Name))

	return sb.String()
}

// detectRequiredBuildTools analyzes recipe steps and returns apt package names for build tools.
// It examines all steps, including platform-specific ones (handled via step.When conditions).
func (e *Executor) detectRequiredBuildTools(r *recipe.Recipe) []string {
	toolsNeeded := make(map[string]bool)

	// Always include basic build essentials
	toolsNeeded["build-essential"] = true

	// Check all recipe steps for specific build systems
	// Platform-specific steps use the When field (e.g., when.os = "linux")
	// but we install tools for all potential steps since validation runs on Linux
	for _, step := range r.Steps {
		// Skip steps that are macOS-only (they won't run in Linux container)
		if os, hasOS := step.When["os"]; hasOS && os == "darwin" {
			continue
		}

		switch step.Action {
		case "configure_make":
			toolsNeeded["autoconf"] = true
			toolsNeeded["automake"] = true
			toolsNeeded["libtool"] = true
			toolsNeeded["pkg-config"] = true
		case "cmake_build":
			toolsNeeded["cmake"] = true
			toolsNeeded["ninja-build"] = true
		case "cargo_build", "cargo_install":
			// Rust toolchain - installed separately
			toolsNeeded["curl"] = true // For rustup
		case "go_build", "go_install":
			// Go toolchain - installed separately
			toolsNeeded["curl"] = true // For Go download
		case "apply_patch":
			toolsNeeded["patch"] = true
		case "cpan_install":
			toolsNeeded["perl"] = true
			toolsNeeded["cpanminus"] = true
		}
	}

	// Convert map to slice
	tools := make([]string, 0, len(toolsNeeded))
	for tool := range toolsNeeded {
		tools = append(tools, tool)
	}

	return tools
}
