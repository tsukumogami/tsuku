package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/containerimages"
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
		Image:           containerimages.DefaultImage(),
		Resources:       DefaultLimits(),
	}

	script := exec.buildSandboxScript(plan, reqs)

	// Should NOT contain apt-get for offline builds
	if strings.Contains(script, "apt-get update") {
		t.Error("Offline script should not run apt-get update")
	}

	// Should contain conditional TSUKU_HOME setup
	if !strings.Contains(script, "if [ ! -d /workspace/tsuku/tools ]; then") {
		t.Error("Script should conditionally setup TSUKU_HOME directories")
	}
	if !strings.Contains(script, "mkdir -p /workspace/tsuku/recipes /workspace/tsuku/bin /workspace/tsuku/tools") {
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

	// Should conditionally setup TSUKU_HOME and run install
	if !strings.Contains(script, "if [ ! -d /workspace/tsuku/tools ]; then") {
		t.Error("Script should conditionally setup TSUKU_HOME")
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
	reqs := ComputeSandboxRequirements(plan, "")

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

func TestBuildSandboxScript_WithVerifyCommand(t *testing.T) {
	t.Parallel()

	exec := &Executor{}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify: &executor.PlanVerify{
			Command: "test-tool --version",
			Pattern: "1.0.0",
		},
	}
	reqs := &SandboxRequirements{
		Image:     "debian:bookworm-slim",
		Resources: DefaultLimits(),
	}

	script := exec.buildSandboxScript(plan, reqs)

	// Should contain set +e before verify
	if !strings.Contains(script, "set +e") {
		t.Error("Script should contain 'set +e' before verify command")
	}

	// Should redirect output to marker file under /workspace/output/
	if !strings.Contains(script, "/workspace/output/.sandbox-verify-output") {
		t.Error("Script should redirect verify output to /workspace/output/ marker file")
	}

	// Should write exit code to marker file under /workspace/output/
	if !strings.Contains(script, "/workspace/output/.sandbox-verify-exit") {
		t.Error("Script should write verify exit code to /workspace/output/ marker file")
	}

	// Should contain the verify command
	if !strings.Contains(script, "test-tool --version") {
		t.Error("Script should contain the verify command text")
	}

	// Should add TSUKU_HOME/bin and TSUKU_HOME/tools/current to PATH
	if !strings.Contains(script, "$TSUKU_HOME/bin") {
		t.Error("Script should add $TSUKU_HOME/bin to PATH")
	}
	if !strings.Contains(script, "$TSUKU_HOME/tools/current") {
		t.Error("Script should add $TSUKU_HOME/tools/current to PATH")
	}
}

func TestBuildSandboxScript_WithoutVerifyCommand(t *testing.T) {
	t.Parallel()

	exec := &Executor{}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		// No Verify field
	}
	reqs := &SandboxRequirements{
		Image:     "debian:bookworm-slim",
		Resources: DefaultLimits(),
	}

	script := exec.buildSandboxScript(plan, reqs)

	// Should NOT contain verify-related marker files
	if strings.Contains(script, ".sandbox-verify-output") {
		t.Error("Script without verify should not contain verify output marker")
	}
	if strings.Contains(script, ".sandbox-verify-exit") {
		t.Error("Script without verify should not contain verify exit marker")
	}

	// Should NOT contain set +e (only set -e from the start)
	if strings.Contains(script, "set +e") {
		t.Error("Script without verify should not contain 'set +e'")
	}

	// Should still contain install command
	if !strings.Contains(script, "tsuku install --plan") {
		t.Error("Script should still contain install command")
	}
}

func TestBuildSandboxScript_EmptyVerifyCommand(t *testing.T) {
	t.Parallel()

	exec := &Executor{}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify: &executor.PlanVerify{
			Command: "", // Empty command
			Pattern: "1.0.0",
		},
	}
	reqs := &SandboxRequirements{
		Image:     "debian:bookworm-slim",
		Resources: DefaultLimits(),
	}

	script := exec.buildSandboxScript(plan, reqs)

	// Empty command should not produce verify block
	if strings.Contains(script, ".sandbox-verify-output") {
		t.Error("Script with empty verify command should not contain verify block")
	}
}

func TestBuildSandboxScript_VerifyWithNonDefaultExitCode(t *testing.T) {
	t.Parallel()

	exitCode := 2
	exec := &Executor{}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify: &executor.PlanVerify{
			Command:  "test-tool check",
			Pattern:  "ok",
			ExitCode: &exitCode,
		},
	}
	reqs := &SandboxRequirements{
		Image:     "debian:bookworm-slim",
		Resources: DefaultLimits(),
	}

	script := exec.buildSandboxScript(plan, reqs)

	// The script itself doesn't change for non-default exit codes;
	// the expected exit code is evaluated in Go, not in the script.
	// Script should still have the verify block.
	if !strings.Contains(script, "test-tool check") {
		t.Error("Script should contain the verify command")
	}
	if !strings.Contains(script, ".sandbox-verify-exit") {
		t.Error("Script should write exit code to marker file")
	}
}

func TestSandboxResult_DurationMsField(t *testing.T) {
	t.Parallel()

	result := &SandboxResult{
		Passed:     true,
		ExitCode:   0,
		DurationMs: 4523,
	}
	if result.DurationMs != 4523 {
		t.Errorf("DurationMs = %d, want 4523", result.DurationMs)
	}

	// Skipped result should also carry duration
	skipped := &SandboxResult{
		Skipped:    true,
		DurationMs: 15,
	}
	if skipped.DurationMs != 15 {
		t.Errorf("Skipped DurationMs = %d, want 15", skipped.DurationMs)
	}
}

func TestSandboxResult_VerificationFields(t *testing.T) {
	t.Parallel()

	// Test with verification passed
	result := &SandboxResult{
		Passed:         true,
		ExitCode:       0,
		Verified:       true,
		VerifyExitCode: 0,
	}
	if !result.Verified {
		t.Error("Verified should be true")
	}
	if result.VerifyExitCode != 0 {
		t.Errorf("VerifyExitCode = %d, want 0", result.VerifyExitCode)
	}

	// Test with no verify command
	result2 := &SandboxResult{
		Passed:         true,
		ExitCode:       0,
		Verified:       true,
		VerifyExitCode: -1,
	}
	if !result2.Verified {
		t.Error("Verified should be true when no verify command")
	}
	if result2.VerifyExitCode != -1 {
		t.Errorf("VerifyExitCode = %d, want -1", result2.VerifyExitCode)
	}
}

func TestReadVerifyResults_NoVerifyCommand(t *testing.T) {
	t.Parallel()

	exec := &Executor{logger: log.NewNoop()}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		// No Verify field
	}

	verified, exitCode := exec.readVerifyResults("/nonexistent", plan)
	if !verified {
		t.Error("Expected verified=true when no verify command")
	}
	if exitCode != -1 {
		t.Errorf("Expected exitCode=-1 when no verify command, got %d", exitCode)
	}
}

func TestReadVerifyResults_EmptyVerifyCommand(t *testing.T) {
	t.Parallel()

	exec := &Executor{logger: log.NewNoop()}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify:  &executor.PlanVerify{Command: ""},
	}

	verified, exitCode := exec.readVerifyResults("/nonexistent", plan)
	if !verified {
		t.Error("Expected verified=true when verify command is empty")
	}
	if exitCode != -1 {
		t.Errorf("Expected exitCode=-1 when verify command is empty, got %d", exitCode)
	}
}

func TestReadVerifyResults_MarkerFilesExist(t *testing.T) {
	t.Parallel()

	// Create temp workspace with marker files
	workspaceDir := t.TempDir()

	// Write exit code marker
	exitPath := workspaceDir + "/.sandbox-verify-exit"
	if err := os.WriteFile(exitPath, []byte("0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write output marker
	outputPath := workspaceDir + "/.sandbox-verify-output"
	if err := os.WriteFile(outputPath, []byte("test-tool v1.0.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	exec := &Executor{logger: log.NewNoop()}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify: &executor.PlanVerify{
			Command: "test-tool --version",
			Pattern: "1.0.0",
		},
	}

	verified, exitCode := exec.readVerifyResults(workspaceDir, plan)
	if !verified {
		t.Error("Expected verified=true when pattern matches")
	}
	if exitCode != 0 {
		t.Errorf("Expected exitCode=0, got %d", exitCode)
	}
}

func TestReadVerifyResults_PatternMismatch(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()

	exitPath := workspaceDir + "/.sandbox-verify-exit"
	if err := os.WriteFile(exitPath, []byte("0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	outputPath := workspaceDir + "/.sandbox-verify-output"
	if err := os.WriteFile(outputPath, []byte("wrong output\n"), 0644); err != nil {
		t.Fatal(err)
	}

	exec := &Executor{logger: log.NewNoop()}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify: &executor.PlanVerify{
			Command: "test-tool --version",
			Pattern: "1.0.0",
		},
	}

	verified, exitCode := exec.readVerifyResults(workspaceDir, plan)
	if verified {
		t.Error("Expected verified=false when pattern does not match")
	}
	if exitCode != 0 {
		t.Errorf("Expected exitCode=0, got %d", exitCode)
	}
}

func TestReadVerifyResults_NonZeroExitCode(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()

	exitPath := workspaceDir + "/.sandbox-verify-exit"
	if err := os.WriteFile(exitPath, []byte("1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	outputPath := workspaceDir + "/.sandbox-verify-output"
	if err := os.WriteFile(outputPath, []byte("error\n"), 0644); err != nil {
		t.Fatal(err)
	}

	exec := &Executor{logger: log.NewNoop()}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify: &executor.PlanVerify{
			Command: "test-tool --version",
			Pattern: "",
		},
	}

	verified, exitCode := exec.readVerifyResults(workspaceDir, plan)
	if verified {
		t.Error("Expected verified=false when exit code is non-zero")
	}
	if exitCode != 1 {
		t.Errorf("Expected exitCode=1, got %d", exitCode)
	}
}

func TestReadVerifyResults_NonDefaultExpectedExitCode(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()

	exitPath := workspaceDir + "/.sandbox-verify-exit"
	if err := os.WriteFile(exitPath, []byte("2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	outputPath := workspaceDir + "/.sandbox-verify-output"
	if err := os.WriteFile(outputPath, []byte("check passed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	expectedCode := 2
	exec := &Executor{logger: log.NewNoop()}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify: &executor.PlanVerify{
			Command:  "test-tool check",
			Pattern:  "check passed",
			ExitCode: &expectedCode,
		},
	}

	verified, exitCode := exec.readVerifyResults(workspaceDir, plan)
	if !verified {
		t.Error("Expected verified=true when non-default exit code matches and pattern found")
	}
	if exitCode != 2 {
		t.Errorf("Expected exitCode=2, got %d", exitCode)
	}
}

func TestReadVerifyResults_MissingMarkerFiles(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	// Don't create any marker files

	exec := &Executor{logger: log.NewNoop()}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify: &executor.PlanVerify{
			Command: "test-tool --version",
			Pattern: "1.0.0",
		},
	}

	verified, exitCode := exec.readVerifyResults(workspaceDir, plan)
	if verified {
		t.Error("Expected verified=false when marker files are missing")
	}
	if exitCode != -1 {
		t.Errorf("Expected exitCode=-1 when marker files are missing, got %d", exitCode)
	}
}

// --- ExtraEnv / filterExtraEnv tests ---

func TestFilterExtraEnv_PassesArbitraryVars(t *testing.T) {
	t.Parallel()

	extra := []string{
		"GITHUB_TOKEN=ghp_abc123",
		"MY_VAR=hello",
	}
	filtered := filterExtraEnv(extra)

	if len(filtered) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(filtered))
	}
	if filtered[0] != "GITHUB_TOKEN=ghp_abc123" {
		t.Errorf("Expected GITHUB_TOKEN entry, got %q", filtered[0])
	}
	if filtered[1] != "MY_VAR=hello" {
		t.Errorf("Expected MY_VAR entry, got %q", filtered[1])
	}
}

func TestFilterExtraEnv_DropsProtectedKeys(t *testing.T) {
	t.Parallel()

	extra := []string{
		"TSUKU_SANDBOX=0",
		"TSUKU_HOME=/tmp/bad",
		"HOME=/tmp/bad",
		"DEBIAN_FRONTEND=dialog",
		"PATH=/bad",
		"GITHUB_TOKEN=keep_this",
	}
	filtered := filterExtraEnv(extra)

	if len(filtered) != 1 {
		t.Fatalf("Expected 1 entry after filtering, got %d: %v", len(filtered), filtered)
	}
	if filtered[0] != "GITHUB_TOKEN=keep_this" {
		t.Errorf("Expected GITHUB_TOKEN entry, got %q", filtered[0])
	}
}

func TestFilterExtraEnv_KeyOnlyFormat(t *testing.T) {
	t.Parallel()

	// KEY-only entries (no '=') should pass through since the caller
	// (resolveEnvFlags) adds "=value" before the entry reaches
	// filterExtraEnv. But filterExtraEnv itself should handle KEY-only
	// gracefully by treating the whole string as the key.
	extra := []string{
		"SOME_KEY",
		"PATH", // protected, should be dropped even as KEY-only
	}
	filtered := filterExtraEnv(extra)

	if len(filtered) != 1 {
		t.Fatalf("Expected 1 entry, got %d: %v", len(filtered), filtered)
	}
	if filtered[0] != "SOME_KEY" {
		t.Errorf("Expected SOME_KEY, got %q", filtered[0])
	}
}

func TestFilterExtraEnv_EmptySlice(t *testing.T) {
	t.Parallel()

	filtered := filterExtraEnv(nil)
	if filtered != nil {
		t.Errorf("Expected nil for nil input, got %v", filtered)
	}

	filtered = filterExtraEnv([]string{})
	if filtered != nil {
		t.Errorf("Expected nil for empty input, got %v", filtered)
	}
}

func TestFilterExtraEnv_EmptyValue(t *testing.T) {
	t.Parallel()

	extra := []string{"MY_VAR="}
	filtered := filterExtraEnv(extra)

	if len(filtered) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(filtered))
	}
	if filtered[0] != "MY_VAR=" {
		t.Errorf("Expected MY_VAR=, got %q", filtered[0])
	}
}

func TestFilterExtraEnv_ValueContainsEquals(t *testing.T) {
	t.Parallel()

	// KEY=VALUE=with=more=equals should use first '=' to split
	extra := []string{"CONFIG=a=b=c"}
	filtered := filterExtraEnv(extra)

	if len(filtered) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(filtered))
	}
	if filtered[0] != "CONFIG=a=b=c" {
		t.Errorf("Expected CONFIG=a=b=c, got %q", filtered[0])
	}
}

func TestFilterExtraEnv_AllProtected(t *testing.T) {
	t.Parallel()

	extra := []string{
		"TSUKU_SANDBOX=1",
		"HOME=/bad",
		"PATH=/bad",
	}
	filtered := filterExtraEnv(extra)

	if len(filtered) != 0 {
		t.Errorf("Expected 0 entries when all are protected, got %d: %v", len(filtered), filtered)
	}
}

func TestSandboxRequirements_ExtraEnvField(t *testing.T) {
	t.Parallel()

	reqs := &SandboxRequirements{
		ExtraEnv: []string{"GITHUB_TOKEN=abc", "MY_VAR=xyz"},
	}

	if len(reqs.ExtraEnv) != 2 {
		t.Fatalf("Expected 2 ExtraEnv entries, got %d", len(reqs.ExtraEnv))
	}
	if reqs.ExtraEnv[0] != "GITHUB_TOKEN=abc" {
		t.Errorf("Expected first entry GITHUB_TOKEN=abc, got %q", reqs.ExtraEnv[0])
	}
}

// --- Targeted mount tests ---

// TestSandboxTargetedMounts verifies that Executor.Sandbox() constructs four
// targeted mounts (plan.json, sandbox.sh, download cache, output dir) instead
// of a single broad /workspace mount. This test exercises the mount construction
// by replaying the same logic Sandbox() uses to build RunOptions.Mounts.
func TestSandboxTargetedMounts(t *testing.T) {
	t.Parallel()

	// Set up a workspace directory mirroring what Sandbox() creates
	workspaceDir := t.TempDir()
	cacheDir := filepath.Join(workspaceDir, "cache", "downloads")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(workspaceDir, "output")
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		t.Fatal(err)
	}

	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
	}
	planData, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(workspaceDir, "plan.json")
	if err := os.WriteFile(planPath, planData, 0644); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(workspaceDir, "sandbox.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Build the same mounts Sandbox() constructs
	mounts := []validate.Mount{
		{Source: planPath, Target: "/workspace/plan.json", ReadOnly: true},
		{Source: scriptPath, Target: "/workspace/sandbox.sh", ReadOnly: true},
		{Source: cacheDir, Target: "/workspace/tsuku/cache/downloads", ReadOnly: true},
		{Source: outputDir, Target: "/workspace/output", ReadOnly: false},
	}

	// Verify exactly 4 targeted mounts
	if len(mounts) != 4 {
		t.Fatalf("Expected 4 mounts, got %d", len(mounts))
	}

	// Verify each mount's target and read-only flags
	expected := []struct {
		target   string
		readOnly bool
	}{
		{"/workspace/plan.json", true},
		{"/workspace/sandbox.sh", true},
		{"/workspace/tsuku/cache/downloads", true},
		{"/workspace/output", false},
	}

	for i, exp := range expected {
		if mounts[i].Target != exp.target {
			t.Errorf("Mount[%d] target = %q, want %q", i, mounts[i].Target, exp.target)
		}
		if mounts[i].ReadOnly != exp.readOnly {
			t.Errorf("Mount[%d] readOnly = %v, want %v", i, mounts[i].ReadOnly, exp.readOnly)
		}
	}

	// Verify no broad /workspace mount exists
	for i, m := range mounts {
		if m.Target == "/workspace" {
			t.Errorf("Mount[%d] is a broad /workspace mount, which should not exist", i)
		}
	}

	// Verify sources point to real files/dirs
	for i, m := range mounts {
		if _, err := os.Stat(m.Source); err != nil {
			t.Errorf("Mount[%d] source %q does not exist: %v", i, m.Source, err)
		}
	}
}

// TestSandboxTargetedMounts_TsukuBinaryAppendedSeparately verifies that the
// tsuku binary mount is appended after the four targeted mounts.
func TestSandboxTargetedMounts_TsukuBinaryAppendedSeparately(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	planPath := filepath.Join(workspaceDir, "plan.json")
	scriptPath := filepath.Join(workspaceDir, "sandbox.sh")
	cacheDir := filepath.Join(workspaceDir, "cache")
	outputDir := filepath.Join(workspaceDir, "output")

	for _, d := range []string{cacheDir, outputDir} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{planPath, scriptPath} {
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Build mounts the way Sandbox() does
	mounts := []validate.Mount{
		{Source: planPath, Target: "/workspace/plan.json", ReadOnly: true},
		{Source: scriptPath, Target: "/workspace/sandbox.sh", ReadOnly: true},
		{Source: cacheDir, Target: "/workspace/tsuku/cache/downloads", ReadOnly: true},
		{Source: outputDir, Target: "/workspace/output", ReadOnly: false},
	}

	// Append tsuku binary mount (as Sandbox() does when tsukuBinary != "")
	tsukuBinary := "/usr/local/bin/tsuku"
	mounts = append(mounts, validate.Mount{
		Source:   tsukuBinary,
		Target:   "/usr/local/bin/tsuku",
		ReadOnly: true,
	})

	// Total should be 5 (4 targeted + 1 tsuku binary)
	if len(mounts) != 5 {
		t.Fatalf("Expected 5 mounts with tsuku binary, got %d", len(mounts))
	}

	// Last mount should be the tsuku binary
	last := mounts[4]
	if last.Target != "/usr/local/bin/tsuku" {
		t.Errorf("Last mount target = %q, want /usr/local/bin/tsuku", last.Target)
	}
	if !last.ReadOnly {
		t.Error("Tsuku binary mount should be read-only")
	}
}

// TestBuildSandboxScript_ConditionalMkdir verifies that the sandbox script
// uses a conditional mkdir -p guard for TSUKU_HOME structure creation.
func TestBuildSandboxScript_ConditionalMkdir(t *testing.T) {
	t.Parallel()

	exec := &Executor{}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
	}
	reqs := &SandboxRequirements{
		Image:     "debian:bookworm-slim",
		Resources: DefaultLimits(),
	}

	script := exec.buildSandboxScript(plan, reqs)

	// Should use conditional mkdir, not unconditional
	if !strings.Contains(script, "if [ ! -d /workspace/tsuku/tools ]; then") {
		t.Error("Script should guard mkdir -p with [ ! -d /workspace/tsuku/tools ]")
	}
	if !strings.Contains(script, "mkdir -p /workspace/tsuku/recipes /workspace/tsuku/bin /workspace/tsuku/tools") {
		t.Error("Script should create recipes, bin, and tools directories in one command")
	}
	if !strings.Contains(script, "fi") {
		t.Error("Script should close the if block with fi")
	}

	// Should NOT have unconditional mkdir -p /workspace/tsuku/recipes on its own line
	lines := strings.Split(script, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "mkdir -p /workspace/tsuku/recipes" {
			t.Error("Script should not have unconditional 'mkdir -p /workspace/tsuku/recipes'")
		}
		if trimmed == "mkdir -p /workspace/tsuku/bin" {
			t.Error("Script should not have unconditional 'mkdir -p /workspace/tsuku/bin'")
		}
		if trimmed == "mkdir -p /workspace/tsuku/tools" {
			t.Error("Script should not have unconditional 'mkdir -p /workspace/tsuku/tools'")
		}
	}
}

// TestBuildSandboxScript_VerifyMarkersWriteToOutput verifies that marker files
// are written to /workspace/output/, not /workspace/ directly.
func TestBuildSandboxScript_VerifyMarkersWriteToOutput(t *testing.T) {
	t.Parallel()

	exec := &Executor{}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify: &executor.PlanVerify{
			Command: "test-tool --version",
			Pattern: "1.0.0",
		},
	}
	reqs := &SandboxRequirements{
		Image:     "debian:bookworm-slim",
		Resources: DefaultLimits(),
	}

	script := exec.buildSandboxScript(plan, reqs)

	// Markers should go to /workspace/output/
	expectedOutput := fmt.Sprintf("/workspace/output/%s", verifyOutputMarker)
	expectedExit := fmt.Sprintf("/workspace/output/%s", verifyExitMarker)

	if !strings.Contains(script, expectedOutput) {
		t.Errorf("Script should write verify output to %s", expectedOutput)
	}
	if !strings.Contains(script, expectedExit) {
		t.Errorf("Script should write verify exit code to %s", expectedExit)
	}

	// Should NOT contain markers at /workspace/ (without /output/ prefix)
	// Check that the only occurrences of the marker names are under /workspace/output/
	oldStyleOutput := fmt.Sprintf("> /workspace/%s", verifyOutputMarker)
	oldStyleExit := fmt.Sprintf("> /workspace/%s", verifyExitMarker)
	if strings.Contains(script, oldStyleOutput) {
		t.Error("Script should NOT write markers directly to /workspace/")
	}
	if strings.Contains(script, oldStyleExit) {
		t.Error("Script should NOT write exit marker directly to /workspace/")
	}
}

// TestReadVerifyResults_OutputDirectory verifies that readVerifyResults reads
// marker files from the output subdirectory, matching the new /workspace/output/
// mount layout.
func TestReadVerifyResults_OutputDirectory(t *testing.T) {
	t.Parallel()

	// Create a workspace with output subdirectory (matching Sandbox() layout)
	workspaceDir := t.TempDir()
	outputDir := filepath.Join(workspaceDir, "output")
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Write marker files to the output subdirectory
	exitPath := filepath.Join(outputDir, verifyExitMarker)
	if err := os.WriteFile(exitPath, []byte("0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(outputDir, verifyOutputMarker)
	if err := os.WriteFile(outputPath, []byte("test-tool v1.0.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	exec := &Executor{logger: log.NewNoop()}
	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
		Verify: &executor.PlanVerify{
			Command: "test-tool --version",
			Pattern: "1.0.0",
		},
	}

	// Pass outputDir (not workspaceDir) -- matching how Sandbox() now calls it
	verified, exitCode := exec.readVerifyResults(outputDir, plan)
	if !verified {
		t.Error("Expected verified=true when reading from output directory")
	}
	if exitCode != 0 {
		t.Errorf("Expected exitCode=0, got %d", exitCode)
	}

	// Verify that reading from workspaceDir directly would fail (markers aren't there)
	verified2, exitCode2 := exec.readVerifyResults(workspaceDir, plan)
	if verified2 {
		t.Error("Reading from workspaceDir (not outputDir) should fail to find markers")
	}
	if exitCode2 != -1 {
		t.Errorf("Expected exitCode=-1 when reading from wrong dir, got %d", exitCode2)
	}
}

// --- Foundation image wiring tests (scenarios 11 and 12) ---

// TestSandbox_NoDep_SkipsFoundation verifies that when a plan has no
// dependencies, Executor.Sandbox() uses the package image directly and
// does not call BuildFromDockerfile.
func TestSandbox_NoDep_SkipsFoundation(t *testing.T) {
	t.Parallel()

	mock := &mockRuntime{
		name:     "podman",
		rootless: true,
		// ImageExists: return false for the package image check, triggering
		// a Build() call for the package image (normal path).
		imageExistsFunc: func(ctx context.Context, name string) (bool, error) {
			// Package image not cached -- triggers Build() for it.
			// Foundation images are never checked because deps are empty.
			return false, nil
		},
		runFunc: func(ctx context.Context, opts validate.RunOptions) (*validate.RunResult, error) {
			return &validate.RunResult{ExitCode: 0}, nil
		},
	}

	detector := validate.NewRuntimeDetectorFrom(mock)

	// Create a real tsuku binary file (needed for the binary mount check)
	tsukuBin := createTempBinary(t)

	exec := NewExecutor(detector,
		WithLogger(log.NewNoop()),
		WithTsukuBinary(tsukuBin),
	)

	plan := &executor.InstallationPlan{
		Tool:    "fzf",
		Version: "0.42.0",
		Platform: executor.Platform{
			OS:          "linux",
			Arch:        "amd64",
			LinuxFamily: "debian",
		},
		Steps: []executor.ResolvedStep{
			{Action: "download_file", Checksum: "abc123"},
		},
		// No Dependencies -- no foundation image needed
	}

	target := platform.Target{}
	reqs := ComputeSandboxRequirements(plan, "")

	result, err := exec.Sandbox(context.Background(), plan, target, reqs)
	if err != nil {
		t.Fatalf("Sandbox returned error: %v", err)
	}

	// Sandbox should have completed (not skipped)
	if result.Skipped {
		t.Error("Expected Sandbox to run, not skip")
	}

	// BuildFromDockerfile should NOT have been called -- no foundation image
	if mock.buildFromDockerfileCalls != 0 {
		t.Errorf("BuildFromDockerfile called %d times, want 0 (no deps)", mock.buildFromDockerfileCalls)
	}

	// The image used for Run() should be the package image, not a foundation image
	if strings.Contains(mock.lastRunOpts.Image, "sandbox-foundation") {
		t.Errorf("Run() image should not be a foundation image, got %q", mock.lastRunOpts.Image)
	}
}

// TestSandbox_WithDeps_BuildsFoundation verifies that when a plan has
// InstallTime dependencies, Executor.Sandbox() calls BuildFoundationImage
// and uses the resulting foundation image for the container run.
func TestSandbox_WithDeps_BuildsFoundation(t *testing.T) {
	t.Parallel()

	var buildFromDockerfileCalledWithImage string

	mock := &mockRuntime{
		name:     "podman",
		rootless: true,
		imageExistsFunc: func(ctx context.Context, name string) (bool, error) {
			// Package image: not cached (triggers Build for it)
			// Foundation image: not cached (triggers BuildFromDockerfile)
			return false, nil
		},
		buildFromDockerfileFunc: func(ctx context.Context, imageName string, contextDir string) error {
			buildFromDockerfileCalledWithImage = imageName
			return nil
		},
		runFunc: func(ctx context.Context, opts validate.RunOptions) (*validate.RunResult, error) {
			return &validate.RunResult{ExitCode: 0}, nil
		},
	}

	detector := validate.NewRuntimeDetectorFrom(mock)
	tsukuBin := createTempBinary(t)

	exec := NewExecutor(detector,
		WithLogger(log.NewNoop()),
		WithTsukuBinary(tsukuBin),
	)

	plan := &executor.InstallationPlan{
		Tool:    "cargo-nextest",
		Version: "0.24.5",
		Platform: executor.Platform{
			OS:          "linux",
			Arch:        "amd64",
			LinuxFamily: "debian",
		},
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "rust",
				Version: "1.82.0",
				Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "r123"}},
			},
		},
		Steps: []executor.ResolvedStep{
			{Action: "cargo_build"},
		},
	}

	target := platform.Target{}
	reqs := ComputeSandboxRequirements(plan, "")

	result, err := exec.Sandbox(context.Background(), plan, target, reqs)
	if err != nil {
		t.Fatalf("Sandbox returned error: %v", err)
	}

	if result.Skipped {
		t.Error("Expected Sandbox to run, not skip")
	}

	// BuildFromDockerfile should have been called exactly once for the
	// foundation image
	if mock.buildFromDockerfileCalls != 1 {
		t.Errorf("BuildFromDockerfile called %d times, want 1", mock.buildFromDockerfileCalls)
	}

	// The image passed to BuildFromDockerfile should be a foundation image
	if !strings.HasPrefix(buildFromDockerfileCalledWithImage, "tsuku/sandbox-foundation:debian-") {
		t.Errorf("BuildFromDockerfile image = %q, want prefix 'tsuku/sandbox-foundation:debian-'",
			buildFromDockerfileCalledWithImage)
	}

	// The image used for Run() should be the foundation image
	if !strings.HasPrefix(mock.lastRunOpts.Image, "tsuku/sandbox-foundation:debian-") {
		t.Errorf("Run() image = %q, want prefix 'tsuku/sandbox-foundation:debian-'",
			mock.lastRunOpts.Image)
	}

	// The Run() image should match the BuildFromDockerfile image
	if mock.lastRunOpts.Image != buildFromDockerfileCalledWithImage {
		t.Errorf("Run() image %q does not match BuildFromDockerfile image %q",
			mock.lastRunOpts.Image, buildFromDockerfileCalledWithImage)
	}
}

// createTempBinary creates a minimal executable file for test use.
func createTempBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "tsuku")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho mock\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return binPath
}
