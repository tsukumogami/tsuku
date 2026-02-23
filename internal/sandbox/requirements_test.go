package sandbox

import (
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/containerimages"
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

	reqs := ComputeSandboxRequirements(nil, "")

	if reqs.RequiresNetwork {
		t.Error("nil plan should not require network")
	}
	if reqs.Image != containerimages.DefaultImage() {
		t.Errorf("nil plan Image = %q, want %q", reqs.Image, containerimages.DefaultImage())
	}
}

func TestComputeSandboxRequirements_EmptyPlan(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Steps: []executor.ResolvedStep{},
	}

	reqs := ComputeSandboxRequirements(plan, "")

	if reqs.RequiresNetwork {
		t.Error("empty plan should not require network")
	}
	if reqs.Image != containerimages.DefaultImage() {
		t.Errorf("empty plan Image = %q, want %q", reqs.Image, containerimages.DefaultImage())
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

	reqs := ComputeSandboxRequirements(plan, "")

	if reqs.RequiresNetwork {
		t.Error("offline plan should not require network")
	}
	if reqs.Image != containerimages.DefaultImage() {
		t.Errorf("offline plan Image = %q, want %q", reqs.Image, containerimages.DefaultImage())
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

			reqs := ComputeSandboxRequirements(plan, "")

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

	reqs := ComputeSandboxRequirements(plan, "")

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

	reqs := ComputeSandboxRequirements(plan, "")

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

	reqs := ComputeSandboxRequirements(plan, "")

	if reqs.RequiresNetwork {
		t.Error("unknown action should not require network (fail closed)")
	}
	if reqs.Image != containerimages.DefaultImage() {
		t.Errorf("unknown action Image = %q, want %q", reqs.Image, containerimages.DefaultImage())
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

	reqs := ComputeSandboxRequirements(plan, "")

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

func TestComputeSandboxRequirements_TargetFamily(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		targetFamily string
		wantImage    string
	}{
		{"empty defaults to debian", "", containerimages.DefaultImage()},
		{"debian", "debian", "debian:bookworm-slim@sha256:98f4b71de414932439ac6ac690d7060df1f27161073c5036a7553723881bffbe"},
		{"alpine", "alpine", "alpine:3.21@sha256:c3f8e73fdb79deaebaa2037150150191b9dcbfba68b4a46d70103204c53f4709"},
		{"rhel", "rhel", "fedora:41@sha256:f1a3fab47bcb3c3ddf3135d5ee7ba8b7b25f2e809a47440936212a3a50957f3d"},
		{"arch", "arch", "archlinux:base@sha256:e25a13ea0e2a36df12f3593fe4bc1063605cfd2ab46c704f72c9e1c3514138ce"},
		{"suse", "suse", "opensuse/leap:15.6@sha256:045fc29f76266cd8176906ab1d63fcd0f505fe1182c06398631effa8f55e10d0"},
		{"unknown falls back to default", "unknown", containerimages.DefaultImage()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			plan := &executor.InstallationPlan{
				Steps: []executor.ResolvedStep{
					{Action: "download", Params: map[string]interface{}{}},
					{Action: "extract", Params: map[string]interface{}{}},
					{Action: "install_binaries", Params: map[string]interface{}{}},
				},
			}

			reqs := ComputeSandboxRequirements(plan, tc.targetFamily)

			if reqs.Image != tc.wantImage {
				t.Errorf("ComputeSandboxRequirements(plan, %q).Image = %q, want %q",
					tc.targetFamily, reqs.Image, tc.wantImage)
			}
		})
	}
}

func TestComputeSandboxRequirements_TargetFamilyWithBuildActions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		targetFamily string
		wantImage    string
	}{
		{"empty defaults to ubuntu", "", SourceBuildSandboxImage},
		{"alpine uses alpine image", "alpine", "alpine:3.21@sha256:c3f8e73fdb79deaebaa2037150150191b9dcbfba68b4a46d70103204c53f4709"},
		{"suse uses suse image", "suse", "opensuse/leap:15.6@sha256:045fc29f76266cd8176906ab1d63fcd0f505fe1182c06398631effa8f55e10d0"},
		{"rhel uses fedora image", "rhel", "fedora:41@sha256:f1a3fab47bcb3c3ddf3135d5ee7ba8b7b25f2e809a47440936212a3a50957f3d"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			plan := &executor.InstallationPlan{
				Steps: []executor.ResolvedStep{
					{Action: "download", Params: map[string]interface{}{}},
					{Action: "extract", Params: map[string]interface{}{}},
					{Action: "configure_make", Params: map[string]interface{}{}},
					{Action: "install_binaries", Params: map[string]interface{}{}},
				},
			}

			reqs := ComputeSandboxRequirements(plan, tc.targetFamily)

			if reqs.Image != tc.wantImage {
				t.Errorf("ComputeSandboxRequirements(plan, %q).Image = %q, want %q",
					tc.targetFamily, reqs.Image, tc.wantImage)
			}
			// Build actions should always upgrade resources regardless of family
			if reqs.Resources.Memory != "4g" {
				t.Errorf("build plan with family %q: Resources.Memory = %q, want %q",
					tc.targetFamily, reqs.Resources.Memory, "4g")
			}
		})
	}
}

func TestConstants(t *testing.T) {
	t.Parallel()

	if SourceBuildSandboxImage != "ubuntu:22.04" {
		t.Errorf("SourceBuildSandboxImage = %q, want %q", SourceBuildSandboxImage, "ubuntu:22.04")
	}
}
