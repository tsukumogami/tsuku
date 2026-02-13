package validate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// TestEvalPlanCacheFlow verifies that plan generation caches downloads and
// the container can use the cache without network access.
//
// This test:
// 1. Generates an installation plan for a known recipe
// 2. Verifies downloads are cached during plan generation
// 3. Runs the plan in a container with network=none
// 4. Confirms the container can access cached downloads
func TestEvalPlanCacheFlow(t *testing.T) {
	// Check if container runtime is available
	detector := NewRuntimeDetector()
	ctx := context.Background()
	runtime, err := detector.Detect(ctx)
	if err == ErrNoRuntime {
		t.Skip("Container runtime not available")
	}
	if err != nil {
		t.Fatalf("Failed to detect runtime: %v", err)
	}

	// Check if tsuku binary is available
	tsukuPath := findTsukuBinary()
	if tsukuPath == "" {
		t.Skip("tsuku binary not found in PATH or as os.Executable()")
	}

	// Create a minimal recipe that downloads a small file
	// Use serve which has a simple github_file action
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          "serve-test",
			Description:   "Test recipe for cache validation",
			Homepage:      "https://github.com/syntaqx/serve",
			VersionFormat: "semver",
		},
		Version: recipe.VersionSection{
			Source:     "github_releases",
			GitHubRepo: "syntaqx/serve",
			TagPrefix:  "v",
		},
		Steps: []recipe.Step{
			{
				Action: "github_file",
				Params: map[string]interface{}{
					"repo":          "syntaqx/serve",
					"asset_pattern": "serve_{version}_{os}_{arch}.tar.gz",
					"binary":        "serve",
					"os_mapping":    map[string]interface{}{"darwin": "macos", "linux": "linux"},
					"arch_mapping":  map[string]interface{}{"amd64": "x86_64", "arm64": "arm64"},
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: "serve version",
			Pattern: "{version}",
		},
	}

	// Create workspace directory
	workspaceDir, err := os.MkdirTemp("", TempDirPrefix)
	if err != nil {
		t.Fatalf("Failed to create workspace directory: %v", err)
	}
	defer os.RemoveAll(workspaceDir)

	// Create download cache directory with secure permissions
	cacheDir := filepath.Join(workspaceDir, "cache", "downloads")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatalf("Failed to create cache directory: %v", err)
	}

	// Create plan executor
	exec, err := executor.New(r)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer exec.Cleanup()

	// Generate plan with download caching
	predownloader := NewPreDownloader()
	downloadCache := actions.NewDownloadCache(cacheDir)

	plan, err := exec.GeneratePlan(ctx, executor.PlanConfig{
		RecipeSource:  "test",
		Downloader:    NewPreDownloaderAdapter(predownloader),
		DownloadCache: downloadCache,
		OnWarning: func(action, message string) {
			t.Logf("Warning: %s: %s", action, message)
		},
	})
	if err != nil {
		// This integration test requires network access to GitHub to:
		// 1. Resolve the latest version via GitHub API
		// 2. Download the release asset to compute checksum
		// Skip gracefully if network is unavailable
		t.Skipf("Skipping integration test (network required): %v", err)
	}

	t.Logf("Generated plan for %s@%s", plan.Tool, plan.Version)

	// Verify cache has content
	cacheInfo, err := downloadCache.Info()
	if err != nil {
		t.Fatalf("Failed to get cache info: %v", err)
	}
	if cacheInfo.EntryCount == 0 {
		t.Fatal("Expected downloads to be cached, but cache is empty")
	}
	t.Logf("Cache contains %d entries (%d bytes)", cacheInfo.EntryCount, cacheInfo.TotalSize)

	// Write plan JSON to workspace
	planData, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("Failed to serialize plan: %v", err)
	}
	planPath := filepath.Join(workspaceDir, "plan.json")
	if err := os.WriteFile(planPath, planData, 0644); err != nil {
		t.Fatalf("Failed to write plan file: %v", err)
	}

	// Serialize recipe to TOML
	recipeData, err := r.ToTOML()
	if err != nil {
		t.Fatalf("Failed to serialize recipe: %v", err)
	}
	recipePath := filepath.Join(workspaceDir, "recipe.toml")
	if err := os.WriteFile(recipePath, recipeData, 0644); err != nil {
		t.Fatalf("Failed to write recipe file: %v", err)
	}

	// Build validation script
	script := buildTestValidationScript(r)
	scriptPath := filepath.Join(workspaceDir, "validate.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to write validation script: %v", err)
	}

	// Run container with network=none (the key test)
	opts := RunOptions{
		Image:   DefaultValidationImage,
		Command: []string{"/bin/sh", "/workspace/validate.sh"},
		Network: "none", // This is the critical part - no network access!
		WorkDir: "/workspace",
		Env: []string{
			"TSUKU_VALIDATION=1",
			"TSUKU_HOME=/workspace/tsuku",
			"HOME=/workspace",
		},
		Limits: ResourceLimits{
			Memory:  "2g",
			CPUs:    "2",
			PidsMax: 100,
		},
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
			{
				Source:   tsukuPath,
				Target:   "/usr/local/bin/tsuku",
				ReadOnly: true,
			},
		},
	}

	result, err := runtime.Run(ctx, opts)
	if err != nil {
		t.Fatalf("Container run failed: %v\nStdout: %s\nStderr: %s", err, result.Stdout, result.Stderr)
	}

	// Log output for debugging
	t.Logf("Container stdout:\n%s", result.Stdout)
	if result.Stderr != "" {
		t.Logf("Container stderr:\n%s", result.Stderr)
	}

	// Check result
	if result.ExitCode != 0 {
		t.Errorf("Container exited with code %d, expected 0\nStdout: %s\nStderr: %s",
			result.ExitCode, result.Stdout, result.Stderr)
	}

	// Verify the installation worked by checking for version pattern
	if !strings.Contains(result.Stdout, plan.Version) {
		t.Errorf("Expected version %q in output, got: %s", plan.Version, result.Stdout)
	}
}

// buildTestValidationScript creates the shell script for container validation.
func buildTestValidationScript(r *recipe.Recipe) string {
	var sb strings.Builder

	sb.WriteString("#!/bin/sh\n")
	sb.WriteString("set -e\n\n")

	// Setup tsuku home directory
	sb.WriteString("# Setup TSUKU_HOME\n")
	sb.WriteString("mkdir -p /workspace/tsuku/recipes\n")
	sb.WriteString("mkdir -p /workspace/tsuku/bin\n")
	sb.WriteString("mkdir -p /workspace/tsuku/tools\n\n")

	// Copy recipe to tsuku recipes directory
	sb.WriteString("# Copy recipe to tsuku recipes\n")
	sb.WriteString("cp /workspace/recipe.toml /workspace/tsuku/recipes/" + r.Metadata.Name + ".toml\n\n")

	// Run tsuku install with plan
	sb.WriteString("# Run tsuku install with pre-generated plan (offline mode)\n")
	sb.WriteString("tsuku install --plan /workspace/plan.json --force\n\n")

	// Run verify command
	if r.Verify.Command != "" {
		sb.WriteString("# Run verify command\n")
		sb.WriteString("export PATH=\"/workspace/tsuku/tools/current:$PATH\"\n")
		sb.WriteString(r.Verify.Command + "\n")
	}

	return sb.String()
}
