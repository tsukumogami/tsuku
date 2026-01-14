package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/validate"
)

func TestNewExecutor(t *testing.T) {
	t.Parallel()

	detector := validate.NewRuntimeDetector()
	exec := NewExecutor(detector)

	if exec == nil {
		t.Fatal("NewExecutor returned nil")
	}
	if exec.detector != detector {
		t.Error("Executor detector not set correctly")
	}
}

func TestNewExecutor_WithOptions(t *testing.T) {
	t.Parallel()

	detector := validate.NewRuntimeDetector()
	logger := log.NewNoop()

	exec := NewExecutor(detector,
		WithLogger(logger),
		WithTsukuBinary("/usr/bin/tsuku"),
	)

	if exec.logger != logger {
		t.Error("WithLogger option not applied")
	}
	if exec.tsukuBinary != "/usr/bin/tsuku" {
		t.Errorf("WithTsukuBinary option not applied, got %q", exec.tsukuBinary)
	}
}

func TestBuildSandboxScript_OfflineRequirements(t *testing.T) {
	t.Parallel()

	exec := &Executor{}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
	}
	reqs := &SandboxRequirements{
		RequiresNetwork: false,
		Image:           DefaultSandboxImage,
		Resources:       DefaultLimits(),
	}

	script := exec.buildSandboxScript(plan, reqs)

	// Should NOT contain apt-get for offline builds
	if strings.Contains(script, "apt-get update") {
		t.Error("Offline script should not run apt-get update")
	}

	// Should contain TSUKU_HOME setup
	if !strings.Contains(script, "mkdir -p /workspace/tsuku/recipes") {
		t.Error("Script should setup TSUKU_HOME directories")
	}

	// Should run tsuku install --plan
	if !strings.Contains(script, "tsuku install --plan /workspace/plan.json --force") {
		t.Error("Script should run tsuku install --plan")
	}
}

func TestBuildSandboxScript_NoPackageInstallation(t *testing.T) {
	t.Parallel()

	exec := &Executor{}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
	}
	reqs := &SandboxRequirements{
		RequiresNetwork: true,
		Image:           SourceBuildSandboxImage,
		Resources:       SourceBuildLimits(),
	}

	script := exec.buildSandboxScript(plan, reqs)

	// Infrastructure packages are installed via container build, not sandbox script
	if strings.Contains(script, "apt-get") {
		t.Error("Script should not contain apt-get (packages installed via container build)")
	}
	if strings.Contains(script, "dnf ") {
		t.Error("Script should not contain dnf (packages installed via container build)")
	}

	// Should setup TSUKU_HOME and run install
	if !strings.Contains(script, "mkdir -p /workspace/tsuku") {
		t.Error("Script should setup TSUKU_HOME")
	}
	if !strings.Contains(script, "tsuku install --plan") {
		t.Error("Script should run tsuku install --plan")
	}
}

func TestBuildSandboxScript_SetMinusE(t *testing.T) {
	t.Parallel()

	exec := &Executor{}
	plan := &executor.InstallationPlan{}
	reqs := &SandboxRequirements{}

	script := exec.buildSandboxScript(plan, reqs)

	// Script should start with shebang and set -e
	// Uses /bin/sh for portability (Alpine uses ash, not bash)
	if !strings.HasPrefix(script, "#!/bin/sh\nset -e\n") {
		t.Error("Script should start with #!/bin/sh and set -e")
	}
}

func TestSandboxResult_Fields(t *testing.T) {
	t.Parallel()

	result := &SandboxResult{
		Passed:   true,
		Skipped:  false,
		ExitCode: 0,
		Stdout:   "output",
		Stderr:   "errors",
		Error:    nil,
	}

	if !result.Passed {
		t.Error("Passed should be true")
	}
	if result.Skipped {
		t.Error("Skipped should be false")
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Stdout != "output" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "output")
	}
	if result.Stderr != "errors" {
		t.Errorf("Stderr = %q, want %q", result.Stderr, "errors")
	}
}

func TestExecutorConstants(t *testing.T) {
	t.Parallel()

	if TempDirPrefix != "tsuku-sandbox-" {
		t.Errorf("TempDirPrefix = %q, want %q", TempDirPrefix, "tsuku-sandbox-")
	}
	if ContainerLabelPrefix != "io.tsuku.sandbox" {
		t.Errorf("ContainerLabelPrefix = %q, want %q", ContainerLabelPrefix, "io.tsuku.sandbox")
	}
}

func TestResourceLimitsConversion(t *testing.T) {
	t.Parallel()

	// Test that sandbox ResourceLimits can be converted to validate.ResourceLimits
	sandboxLimits := SourceBuildLimits()

	// Verify the limits are what we expect
	if sandboxLimits.Memory != "4g" {
		t.Errorf("Memory = %q, want %q", sandboxLimits.Memory, "4g")
	}
	if sandboxLimits.CPUs != "4" {
		t.Errorf("CPUs = %q, want %q", sandboxLimits.CPUs, "4")
	}
	if sandboxLimits.PidsMax != 500 {
		t.Errorf("PidsMax = %d, want %d", sandboxLimits.PidsMax, 500)
	}
	if sandboxLimits.Timeout != 15*time.Minute {
		t.Errorf("Timeout = %v, want %v", sandboxLimits.Timeout, 15*time.Minute)
	}
}

func TestAugmentWithInfrastructurePackages_DebianFamily(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Platform: executor.Platform{
			OS:          "linux",
			Arch:        "amd64",
			LinuxFamily: "debian",
		},
	}
	reqs := &SandboxRequirements{
		RequiresNetwork: true,
	}

	result := augmentWithInfrastructurePackages(nil, plan, reqs, "debian")

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	pkgs := result.Packages["apt"]
	if len(pkgs) == 0 {
		t.Fatal("Expected apt packages")
	}
	hasCA := false
	for _, p := range pkgs {
		if p == "ca-certificates" {
			hasCA = true
		}
	}
	if !hasCA {
		t.Error("Expected ca-certificates in apt packages")
	}
}

func TestAugmentWithInfrastructurePackages_RhelFamily(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Platform: executor.Platform{
			OS:          "linux",
			Arch:        "amd64",
			LinuxFamily: "rhel",
		},
	}
	reqs := &SandboxRequirements{
		RequiresNetwork: true,
	}

	result := augmentWithInfrastructurePackages(nil, plan, reqs, "rhel")

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	pkgs := result.Packages["dnf"]
	if len(pkgs) == 0 {
		t.Fatal("Expected dnf packages")
	}
	hasCA := false
	for _, p := range pkgs {
		if p == "ca-certificates" {
			hasCA = true
		}
	}
	if !hasCA {
		t.Error("Expected ca-certificates in dnf packages")
	}
}

func TestAugmentWithInfrastructurePackages_ExistingSysReqs(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Platform: executor.Platform{
			OS:          "linux",
			Arch:        "amd64",
			LinuxFamily: "debian",
		},
	}
	reqs := &SandboxRequirements{
		RequiresNetwork: true,
	}
	sysReqs := &SystemRequirements{
		Packages: map[string][]string{
			"apt": {"docker.io"},
		},
	}

	result := augmentWithInfrastructurePackages(sysReqs, plan, reqs, "debian")

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	pkgs := result.Packages["apt"]
	hasDocker := false
	hasCA := false
	for _, p := range pkgs {
		if p == "docker.io" {
			hasDocker = true
		}
		if p == "ca-certificates" {
			hasCA = true
		}
	}
	if !hasDocker {
		t.Error("Expected docker.io in apt packages (original)")
	}
	if !hasCA {
		t.Error("Expected ca-certificates in apt packages (added)")
	}
}

func TestInfrastructurePackages_BuildPackages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pm       string
		expected []string
	}{
		{"apt", []string{"build-essential"}},
		{"dnf", []string{"gcc", "gcc-c++", "make"}},
		{"pacman", []string{"base-devel"}},
		{"apk", []string{"build-base"}},
		{"zypper", []string{"gcc", "gcc-c++", "make"}},
	}

	for _, tt := range tests {
		t.Run(tt.pm, func(t *testing.T) {
			pkgs := infrastructurePackages(tt.pm, "build")
			if len(pkgs) != len(tt.expected) {
				t.Errorf("Expected %d packages, got %d", len(tt.expected), len(pkgs))
			}
			for i, p := range tt.expected {
				if pkgs[i] != p {
					t.Errorf("Expected %q, got %q", p, pkgs[i])
				}
			}
		})
	}
}

func TestSandbox_NoRuntime(t *testing.T) {
	t.Parallel()

	// Create a detector that won't find any runtime
	// (we can't easily mock the detector, so this test is limited)
	detector := validate.NewRuntimeDetector()
	exec := NewExecutor(detector, WithTsukuBinary("/nonexistent/tsuku"))

	plan := &executor.InstallationPlan{
		Tool:    "test",
		Version: "1.0.0",
	}
	reqs := ComputeSandboxRequirements(plan)

	// Detect current system target
	target, err := platform.DetectTarget()
	if err != nil {
		t.Fatalf("Failed to detect target: %v", err)
	}

	// This will either skip (no runtime) or fail (no tsuku binary)
	// Both are acceptable for this test
	result, err := exec.Sandbox(context.Background(), plan, target, reqs)
	if err != nil {
		t.Fatalf("Sandbox returned error: %v", err)
	}

	// Should be skipped since we don't have a valid setup
	if !result.Skipped {
		t.Log("Note: Runtime was found, test may need container environment")
	}
}
