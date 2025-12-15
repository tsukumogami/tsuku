package sandbox

import (
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/executor"
)

func TestDefaultLimits(t *testing.T) {
	t.Parallel()

	limits := DefaultLimits()

	if limits.Memory != "2g" {
		t.Errorf("DefaultLimits().Memory = %q, want %q", limits.Memory, "2g")
	}
	if limits.CPUs != "2" {
		t.Errorf("DefaultLimits().CPUs = %q, want %q", limits.CPUs, "2")
	}
	if limits.PidsMax != 100 {
		t.Errorf("DefaultLimits().PidsMax = %d, want %d", limits.PidsMax, 100)
	}
	if limits.Timeout != 2*time.Minute {
		t.Errorf("DefaultLimits().Timeout = %v, want %v", limits.Timeout, 2*time.Minute)
	}
}

func TestSourceBuildLimits(t *testing.T) {
	t.Parallel()

	limits := SourceBuildLimits()

	if limits.Memory != "4g" {
		t.Errorf("SourceBuildLimits().Memory = %q, want %q", limits.Memory, "4g")
	}
	if limits.CPUs != "4" {
		t.Errorf("SourceBuildLimits().CPUs = %q, want %q", limits.CPUs, "4")
	}
	if limits.PidsMax != 500 {
		t.Errorf("SourceBuildLimits().PidsMax = %d, want %d", limits.PidsMax, 500)
	}
	if limits.Timeout != 15*time.Minute {
		t.Errorf("SourceBuildLimits().Timeout = %v, want %v", limits.Timeout, 15*time.Minute)
	}
}

func TestComputeSandboxRequirements_NilPlan(t *testing.T) {
	t.Parallel()

	reqs := ComputeSandboxRequirements(nil)

	if reqs.RequiresNetwork {
		t.Error("nil plan should not require network")
	}
	if reqs.Image != DefaultSandboxImage {
		t.Errorf("nil plan Image = %q, want %q", reqs.Image, DefaultSandboxImage)
	}
}

func TestComputeSandboxRequirements_EmptyPlan(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{},
	}

	reqs := ComputeSandboxRequirements(plan)

	if reqs.RequiresNetwork {
		t.Error("empty plan should not require network")
	}
	if reqs.Image != DefaultSandboxImage {
		t.Errorf("empty plan Image = %q, want %q", reqs.Image, DefaultSandboxImage)
	}
	if reqs.Resources.Memory != "2g" {
		t.Errorf("empty plan Resources.Memory = %q, want %q", reqs.Resources.Memory, "2g")
	}
}

func TestComputeSandboxRequirements_OfflinePlan(t *testing.T) {
	t.Parallel()

	// Binary installation plan - no network required
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{Action: "download", Params: map[string]interface{}{"url": "https://example.com/tool.tar.gz"}},
			{Action: "extract", Params: map[string]interface{}{}},
			{Action: "install_binaries", Params: map[string]interface{}{}},
		},
	}

	reqs := ComputeSandboxRequirements(plan)

	if reqs.RequiresNetwork {
		t.Error("offline plan should not require network")
	}
	if reqs.Image != DefaultSandboxImage {
		t.Errorf("offline plan Image = %q, want %q", reqs.Image, DefaultSandboxImage)
	}
	if reqs.Resources.Memory != "2g" {
		t.Errorf("offline plan Resources.Memory = %q, want %q", reqs.Resources.Memory, "2g")
	}
	if reqs.Resources.Timeout != 2*time.Minute {
		t.Errorf("offline plan Resources.Timeout = %v, want %v", reqs.Resources.Timeout, 2*time.Minute)
	}
}

func TestComputeSandboxRequirements_NetworkRequired(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		action string
	}{
		{"cargo_build", "cargo_build"},
		{"go_build", "go_build"},
		{"npm_install", "npm_install"},
		{"pip_install", "pip_install"},
		{"gem_install", "gem_install"},
		{"cpan_install", "cpan_install"},
		{"apt_install", "apt_install"},
		{"run_command", "run_command"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			plan := &executor.InstallationPlan{
				Steps: []executor.ResolvedStep{
					{Action: "download", Params: map[string]interface{}{}},
					{Action: tc.action, Params: map[string]interface{}{}},
				},
			}

			reqs := ComputeSandboxRequirements(plan)

			if !reqs.RequiresNetwork {
				t.Errorf("plan with %s should require network", tc.action)
			}
			if reqs.Image != SourceBuildSandboxImage {
				t.Errorf("plan with %s: Image = %q, want %q", tc.action, reqs.Image, SourceBuildSandboxImage)
			}
			if reqs.Resources.Memory != "4g" {
				t.Errorf("plan with %s: Resources.Memory = %q, want %q", tc.action, reqs.Resources.Memory, "4g")
			}
		})
	}
}

func TestComputeSandboxRequirements_BuildActionsUpgradeResources(t *testing.T) {
	t.Parallel()

	// configure_make doesn't require network (source is pre-downloaded)
	// but should still get upgraded resources
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{Action: "download", Params: map[string]interface{}{}},
			{Action: "extract", Params: map[string]interface{}{}},
			{Action: "configure_make", Params: map[string]interface{}{}},
			{Action: "install_binaries", Params: map[string]interface{}{}},
		},
	}

	reqs := ComputeSandboxRequirements(plan)

	// configure_make doesn't implement RequiresNetwork as true,
	// but hasBuildActions should still upgrade resources
	if reqs.Image != SourceBuildSandboxImage {
		t.Errorf("build plan Image = %q, want %q", reqs.Image, SourceBuildSandboxImage)
	}
	if reqs.Resources.Memory != "4g" {
		t.Errorf("build plan Resources.Memory = %q, want %q", reqs.Resources.Memory, "4g")
	}
	if reqs.Resources.CPUs != "4" {
		t.Errorf("build plan Resources.CPUs = %q, want %q", reqs.Resources.CPUs, "4")
	}
	if reqs.Resources.Timeout != 15*time.Minute {
		t.Errorf("build plan Resources.Timeout = %v, want %v", reqs.Resources.Timeout, 15*time.Minute)
	}
}

func TestComputeSandboxRequirements_CMakeBuild(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{Action: "download", Params: map[string]interface{}{}},
			{Action: "extract", Params: map[string]interface{}{}},
			{Action: "cmake_build", Params: map[string]interface{}{}},
		},
	}

	reqs := ComputeSandboxRequirements(plan)

	if reqs.Image != SourceBuildSandboxImage {
		t.Errorf("cmake_build plan Image = %q, want %q", reqs.Image, SourceBuildSandboxImage)
	}
	if reqs.Resources.Memory != "4g" {
		t.Errorf("cmake_build plan Resources.Memory = %q, want %q", reqs.Resources.Memory, "4g")
	}
}

func TestComputeSandboxRequirements_UnknownAction(t *testing.T) {
	t.Parallel()

	// Unknown actions should default to no network (fail closed)
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{Action: "unknown_action", Params: map[string]interface{}{}},
		},
	}

	reqs := ComputeSandboxRequirements(plan)

	if reqs.RequiresNetwork {
		t.Error("unknown action should not require network (fail closed)")
	}
	if reqs.Image != DefaultSandboxImage {
		t.Errorf("unknown action Image = %q, want %q", reqs.Image, DefaultSandboxImage)
	}
}

func TestComputeSandboxRequirements_MixedPlan(t *testing.T) {
	t.Parallel()

	// Plan with both offline and network-requiring actions
	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{
			{Action: "download", Params: map[string]interface{}{}},
			{Action: "extract", Params: map[string]interface{}{}},
			{Action: "cargo_build", Params: map[string]interface{}{}}, // requires network
			{Action: "install_binaries", Params: map[string]interface{}{}},
		},
	}

	reqs := ComputeSandboxRequirements(plan)

	// Should require network due to cargo_build
	if !reqs.RequiresNetwork {
		t.Error("mixed plan with cargo_build should require network")
	}
	if reqs.Image != SourceBuildSandboxImage {
		t.Errorf("mixed plan Image = %q, want %q", reqs.Image, SourceBuildSandboxImage)
	}
}

func TestHasBuildActions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		steps    []executor.ResolvedStep
		expected bool
	}{
		{
			name:     "empty plan",
			steps:    []executor.ResolvedStep{},
			expected: false,
		},
		{
			name: "no build actions",
			steps: []executor.ResolvedStep{
				{Action: "download"},
				{Action: "extract"},
				{Action: "install_binaries"},
			},
			expected: false,
		},
		{
			name: "configure_make",
			steps: []executor.ResolvedStep{
				{Action: "download"},
				{Action: "configure_make"},
			},
			expected: true,
		},
		{
			name: "cmake_build",
			steps: []executor.ResolvedStep{
				{Action: "cmake_build"},
			},
			expected: true,
		},
		{
			name: "cargo_build",
			steps: []executor.ResolvedStep{
				{Action: "cargo_build"},
			},
			expected: true,
		},
		{
			name: "go_build",
			steps: []executor.ResolvedStep{
				{Action: "go_build"},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			plan := &executor.InstallationPlan{Steps: tc.steps}
			got := hasBuildActions(plan)

			if got != tc.expected {
				t.Errorf("hasBuildActions() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	t.Parallel()

	if DefaultSandboxImage != "debian:bookworm-slim" {
		t.Errorf("DefaultSandboxImage = %q, want %q", DefaultSandboxImage, "debian:bookworm-slim")
	}
	if SourceBuildSandboxImage != "ubuntu:22.04" {
		t.Errorf("SourceBuildSandboxImage = %q, want %q", SourceBuildSandboxImage, "ubuntu:22.04")
	}
}
