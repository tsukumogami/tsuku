package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/actions"
	planexec "github.com/tsukumogami/tsuku/internal/executor"
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
// 2. Generate installation plan on host (downloads cached)
// 3. Mount tsuku binary, plan, and cache into container
// 4. Install required build tools based on build system
// 5. Run tsuku install --plan which executes the build steps
// 6. Verify expected binaries are produced
//
// Note: Source builds still require network access for apt-get and build
// dependencies (cargo, go mod, etc.), but source archives are pre-cached.
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

	// Check if we have a valid tsuku binary
	if e.tsukuBinary == "" {
		e.logger.Warn("Tsuku binary not found. Skipping source build validation.",
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

	e.logger.Debug("Validating source build", "recipe", r.Metadata.Name, "runtime", runtime.Name())

	// Create workspace directory
	workspaceDir, err := os.MkdirTemp("", TempDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}
	defer os.RemoveAll(workspaceDir)

	// Create download cache directory within workspace with secure permissions (0700)
	cacheDir := filepath.Join(workspaceDir, "cache", "downloads")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Generate installation plan on host
	// This downloads source archives to compute checksums and caches them
	exec, err := planexec.New(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create plan executor: %w", err)
	}
	defer exec.Cleanup()

	downloadCache := actions.NewDownloadCache(cacheDir)
	plan, err := exec.GeneratePlan(ctx, planexec.PlanConfig{
		RecipeSource:  "validation",
		Downloader:    NewPreDownloaderAdapter(e.predownloader),
		DownloadCache: downloadCache,
		OnWarning: func(action, message string) {
			e.logger.Debug("Plan generation warning", "action", action, "message", message)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate installation plan: %w", err)
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

	// Serialize recipe to TOML (still needed for verification lookup)
	recipeData, err := r.ToTOML()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize recipe: %w", err)
	}

	// Write recipe to workspace
	recipePath := filepath.Join(workspaceDir, "recipe.toml")
	if err := os.WriteFile(recipePath, recipeData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write recipe file: %w", err)
	}

	// Build the validation script for source builds with plan support
	script := e.buildSourceBuildPlanScript(r)

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
		Network: "host", // Still need network for apt-get and build dependencies
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
			{
				Source:   cacheDir,
				Target:   "/workspace/tsuku/cache/downloads",
				ReadOnly: true, // Cache is read-only in container
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

// buildSourceBuildPlanScript creates a shell script for source build validation
// using a pre-generated plan. It installs build tools and runs tsuku install --plan.
func (e *Executor) buildSourceBuildPlanScript(r *recipe.Recipe) string {
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

	// Copy recipe to tsuku recipes directory (needed for verification lookup)
	sb.WriteString("# Copy recipe to tsuku recipes\n")
	sb.WriteString(fmt.Sprintf("cp /workspace/recipe.toml /workspace/tsuku/recipes/%s.toml\n\n", r.Metadata.Name))

	// Run tsuku install with pre-generated plan
	sb.WriteString("# Run tsuku install with pre-generated plan\n")
	sb.WriteString("tsuku install --plan /workspace/plan.json --force\n")

	return sb.String()
}

// buildSourceBuildScript creates a shell script for source build validation.
// This is the legacy method - see buildSourceBuildPlanScript for the preferred approach.
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
	// Platform-specific steps use the When field (e.g., when.os = ["linux"])
	// but we install tools for all potential steps since validation runs on Linux
	for _, step := range r.Steps {
		// Skip steps that are macOS-only (they won't run in Linux container)
		if step.When != nil && len(step.When.OS) > 0 {
			isDarwinOnly := true
			for _, os := range step.When.OS {
				if os != "darwin" {
					isDarwinOnly = false
					break
				}
			}
			if isDarwinOnly {
				continue
			}
		}
		// Also skip platform-specific darwin steps
		if step.When != nil && len(step.When.Platform) > 0 {
			isDarwinOnly := true
			for _, platform := range step.When.Platform {
				if !strings.HasPrefix(platform, "darwin/") {
					isDarwinOnly = false
					break
				}
			}
			if isDarwinOnly {
				continue
			}
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
