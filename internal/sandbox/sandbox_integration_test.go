package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/sandbox"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// TestSandboxIntegration tests the sandbox executor with a real container runtime.
// This test automatically skips when no container runtime (Docker/Podman) is available,
// making it safe to run in CI while still exercising the full sandbox flow locally.
func TestSandboxIntegration(t *testing.T) {
	// Check if container runtime is available - skip if not
	detector := validate.NewRuntimeDetector()
	ctx := context.Background()
	runtime, err := detector.Detect(ctx)
	if err != nil {
		t.Skipf("No container runtime available: %v", err)
	}
	t.Logf("Using container runtime: %s (rootless: %v)", runtime.Name(), runtime.IsRootless())

	// Create executor
	exec := sandbox.NewExecutor(detector)

	t.Run("simple_binary_install", func(t *testing.T) {
		// Test a simple binary installation plan
		// This simulates what would happen when validating a recipe
		// Note: We use download+extract which don't require network (pre-computed checksums)
		plan := &executor.InstallationPlan{
			FormatVersion: 2,
			Tool:          "test-sandbox",
			Version:       "1.0.0",
			Steps: []executor.ResolvedStep{
				{
					Action: "download",
					Params: map[string]any{
						"url":    "https://example.com/file.tar.gz",
						"output": "/workspace/file.tar.gz",
					},
				},
			},
		}

		reqs := sandbox.ComputeSandboxRequirements(plan)

		// download action doesn't require network (checksum pre-computed at eval time)
		if reqs.RequiresNetwork {
			t.Log("Note: RequiresNetwork=true, which is unexpected for download-only plan")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		target := platform.Target{
			Platform: plan.Platform.OS + "/" + plan.Platform.Arch,
		}

		result, err := exec.Sandbox(ctx, plan, target, reqs)
		if err != nil {
			t.Fatalf("Sandbox() returned error: %v", err)
		}

		if result.Skipped {
			t.Skip("Sandbox test was skipped (no runtime or tsuku binary)")
		}

		t.Logf("Exit code: %d", result.ExitCode)
		if result.Stdout != "" {
			t.Logf("Stdout:\n%s", result.Stdout)
		}
		if result.Stderr != "" {
			t.Logf("Stderr:\n%s", result.Stderr)
		}

		if !result.Passed {
			t.Errorf("Sandbox test failed: exit code %d, error: %v", result.ExitCode, result.Error)
		}
	})

	t.Run("network_required_plan", func(t *testing.T) {
		// Test a plan that requires network access
		// npm_install is an action that truly requires network
		plan := &executor.InstallationPlan{
			FormatVersion: 2,
			Tool:          "test-network",
			Version:       "1.0.0",
			Steps: []executor.ResolvedStep{
				{
					Action: "npm_install",
					Params: map[string]any{
						"package": "typescript",
					},
				},
			},
		}

		reqs := sandbox.ComputeSandboxRequirements(plan)

		// npm_install action should require network
		if !reqs.RequiresNetwork {
			t.Error("Expected RequiresNetwork=true for plan with npm_install action")
		}

		// Network-requiring plans should use the source build image
		if reqs.Image != sandbox.SourceBuildSandboxImage {
			t.Errorf("Expected image=%s for network plan, got %s",
				sandbox.SourceBuildSandboxImage, reqs.Image)
		}

		// We don't actually run this test as it would take too long
		// This just validates that requirements are computed correctly
		t.Logf("Requirements: network=%v, image=%s", reqs.RequiresNetwork, reqs.Image)
	})

	t.Run("source_build_plan", func(t *testing.T) {
		// Test requirements for a source build plan
		// cargo_build is a network-requiring action that also needs build resources
		plan := &executor.InstallationPlan{
			FormatVersion: 2,
			Tool:          "test-source",
			Version:       "1.0.0",
			Steps: []executor.ResolvedStep{
				{
					Action: "cargo_build",
					Params: map[string]any{
						"manifest_path": "Cargo.toml",
					},
				},
			},
		}

		reqs := sandbox.ComputeSandboxRequirements(plan)

		// cargo_build should require network
		if !reqs.RequiresNetwork {
			t.Error("Expected RequiresNetwork=true for plan with cargo_build action")
		}

		// Source builds should use the source build image
		if reqs.Image != sandbox.SourceBuildSandboxImage {
			t.Errorf("Expected image=%s for source build, got %s",
				sandbox.SourceBuildSandboxImage, reqs.Image)
		}

		// Source builds should have higher resource limits
		sourceLimits := sandbox.SourceBuildLimits()
		if reqs.Resources.Memory != sourceLimits.Memory {
			t.Errorf("Expected memory=%s for source build, got %s",
				sourceLimits.Memory, reqs.Resources.Memory)
		}

		t.Logf("Requirements: network=%v, image=%s, memory=%s, cpus=%s",
			reqs.RequiresNetwork, reqs.Image, reqs.Resources.Memory, reqs.Resources.CPUs)
	})
}

// TestSandboxRequirementsComputation tests that requirements are computed correctly
// for various plan types. This test runs everywhere (including CI) since it doesn't
// need a container runtime.
func TestSandboxRequirementsComputation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		plan            *executor.InstallationPlan
		wantNetwork     bool
		wantSourceImage bool
	}{
		{
			name: "empty_plan",
			plan: &executor.InstallationPlan{
				FormatVersion: 2,
				Tool:          "test",
				Version:       "1.0.0",
			},
			wantNetwork:     false,
			wantSourceImage: false,
		},
		{
			name: "download_only",
			plan: &executor.InstallationPlan{
				FormatVersion: 2,
				Tool:          "test",
				Version:       "1.0.0",
				Steps: []executor.ResolvedStep{
					// download action doesn't implement RequiresNetwork (uses BaseAction default)
					// so it's considered offline - this is correct for pre-downloaded binaries
					{Action: "download", Params: map[string]any{"url": "https://example.com/file"}},
				},
			},
			wantNetwork:     false, // download doesn't require network (pre-computed checksum)
			wantSourceImage: false,
		},
		{
			name: "run_command_requires_network",
			plan: &executor.InstallationPlan{
				FormatVersion: 2,
				Tool:          "test",
				Version:       "1.0.0",
				Steps: []executor.ResolvedStep{
					// run_command conservatively returns RequiresNetwork=true
					{Action: "run_command", Params: map[string]any{"command": "curl https://example.com"}},
				},
			},
			wantNetwork:     true,
			wantSourceImage: true, // network-requiring steps upgrade to source build image
		},
		{
			name: "pipx_install_action",
			plan: &executor.InstallationPlan{
				FormatVersion: 2,
				Tool:          "test",
				Version:       "1.0.0",
				Steps: []executor.ResolvedStep{
					{Action: "pipx_install", Params: map[string]any{"package": "requests"}},
				},
			},
			wantNetwork:     true,
			wantSourceImage: true, // network-requiring steps upgrade to source build image
		},
		{
			name: "npm_install_action",
			plan: &executor.InstallationPlan{
				FormatVersion: 2,
				Tool:          "test",
				Version:       "1.0.0",
				Steps: []executor.ResolvedStep{
					{Action: "npm_install", Params: map[string]any{"package": "typescript"}},
				},
			},
			wantNetwork:     true,
			wantSourceImage: true, // network-requiring steps upgrade to source build image
		},
		{
			name: "cargo_install_action",
			plan: &executor.InstallationPlan{
				FormatVersion: 2,
				Tool:          "test",
				Version:       "1.0.0",
				Steps: []executor.ResolvedStep{
					{Action: "cargo_install", Params: map[string]any{"crate": "ripgrep"}},
				},
			},
			wantNetwork:     true,
			wantSourceImage: true,
		},
		{
			name: "go_install_action",
			plan: &executor.InstallationPlan{
				FormatVersion: 2,
				Tool:          "test",
				Version:       "1.0.0",
				Steps: []executor.ResolvedStep{
					{Action: "go_install", Params: map[string]any{"package": "golang.org/x/tools/gopls@latest"}},
				},
			},
			wantNetwork:     true,
			wantSourceImage: true,
		},
		{
			name: "cargo_build_action",
			plan: &executor.InstallationPlan{
				FormatVersion: 2,
				Tool:          "test",
				Version:       "1.0.0",
				Steps: []executor.ResolvedStep{
					{Action: "cargo_build", Params: map[string]any{"manifest_path": "Cargo.toml"}},
				},
			},
			wantNetwork:     true,
			wantSourceImage: true,
		},
		{
			name: "go_build_action",
			plan: &executor.InstallationPlan{
				FormatVersion: 2,
				Tool:          "test",
				Version:       "1.0.0",
				Steps: []executor.ResolvedStep{
					{Action: "go_build", Params: map[string]any{"package": "./cmd/app"}},
				},
			},
			wantNetwork:     true,
			wantSourceImage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reqs := sandbox.ComputeSandboxRequirements(tt.plan)

			if reqs.RequiresNetwork != tt.wantNetwork {
				t.Errorf("RequiresNetwork = %v, want %v", reqs.RequiresNetwork, tt.wantNetwork)
			}

			wantImage := sandbox.DefaultSandboxImage
			if tt.wantSourceImage {
				wantImage = sandbox.SourceBuildSandboxImage
			}
			if reqs.Image != wantImage {
				t.Errorf("Image = %s, want %s", reqs.Image, wantImage)
			}
		})
	}
}
