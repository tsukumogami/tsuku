package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/containerimages"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// TempDirPrefix is the prefix for temporary directories created by the sandbox.
const TempDirPrefix = "tsuku-sandbox-"

// ContainerLabelPrefix is the label added to containers for identification.
const ContainerLabelPrefix = "io.tsuku.sandbox"

// Marker file names written by the sandbox script and read by readVerifyResults.
const (
	verifyExitMarker   = ".sandbox-verify-exit"
	verifyOutputMarker = ".sandbox-verify-output"
)

// protectedEnvKeys lists environment variable keys that the sandbox hardcodes.
// User-provided ExtraEnv entries matching these keys are silently dropped to
// prevent subverting the sandbox environment.
var protectedEnvKeys = map[string]bool{
	"TSUKU_SANDBOX":              true,
	"TSUKU_HOME":                 true,
	"TSUKU_CARGO_REGISTRY_CACHE": true,
	"HOME":                       true,
	"DEBIAN_FRONTEND":            true,
	"PATH":                       true,
}

// SandboxResult contains the result of a sandbox test.
type SandboxResult struct {
	Passed         bool   // Whether the install succeeded (exit code 0)
	Skipped        bool   // Whether the test was skipped (no runtime)
	ExitCode       int    // Container exit code
	Stdout         string // Container stdout
	Stderr         string // Container stderr
	Error          error  // Error if sandbox failed to run
	Verified       bool   // Whether verify command passed (true if no verify command)
	VerifyExitCode int    // Verify command's exit code (-1 if no verify command)
	DurationMs     int64  // Total execution time in milliseconds
}

// Executor orchestrates container-based sandbox testing.
// It uses SandboxRequirements to configure containers appropriately
// for different types of installations (binary, source build, ecosystem).
type Executor struct {
	detector              *validate.RuntimeDetector
	logger                log.Logger
	tsukuBinary           string // Path to tsuku binary for container execution
	downloadCacheDir      string // External download cache directory to mount
	cargoRegistryCacheDir string // Shared cargo registry cache directory to mount
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithLogger sets a logger for executor messages.
func WithLogger(logger log.Logger) ExecutorOption {
	return func(e *Executor) {
		e.logger = logger
	}
}

// WithTsukuBinary sets the path to the tsuku binary for container execution.
func WithTsukuBinary(path string) ExecutorOption {
	return func(e *Executor) {
		e.tsukuBinary = path
	}
}

// WithDownloadCacheDir sets the download cache directory to mount into containers.
// This directory should contain pre-downloaded files from plan generation.
func WithDownloadCacheDir(path string) ExecutorOption {
	return func(e *Executor) {
		e.downloadCacheDir = path
	}
}

// WithCargoRegistryCacheDir sets a shared cargo registry cache directory.
// When set, this directory is mounted read-write into the container at
// /workspace/cargo-registry-cache. The cargo_build action reads the
// TSUKU_CARGO_REGISTRY_CACHE env var and creates the symlink from
// $CARGO_HOME/registry to this mount, so cargo fetch results are shared
// across Linux families within a single recipe run.
func WithCargoRegistryCacheDir(path string) ExecutorOption {
	return func(e *Executor) {
		e.cargoRegistryCacheDir = path
	}
}

// NewExecutor creates a new Executor with the given runtime detector.
func NewExecutor(detector *validate.RuntimeDetector, opts ...ExecutorOption) *Executor {
	// Auto-detect tsuku binary path
	tsukuPath := findTsukuBinary()

	e := &Executor{
		detector:    detector,
		logger:      log.NewNoop(),
		tsukuBinary: tsukuPath,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// findTsukuBinary locates a valid tsuku binary for container execution.
func findTsukuBinary() string {
	// Try the current executable first (accepts tsuku, tsuku-test, etc.)
	if exePath, err := os.Executable(); err == nil {
		baseName := filepath.Base(exePath)
		if strings.HasPrefix(baseName, "tsuku") {
			return exePath
		}
	}

	// Try to find tsuku in PATH
	if tsukuPath, err := exec.LookPath("tsuku"); err == nil {
		return tsukuPath
	}

	return ""
}

// Sandbox runs an installation plan in an isolated container.
// It uses the provided SandboxRequirements to configure the container
// with appropriate image, network access, and resource limits.
//
// The sandbox process:
// 1. Detect available container runtime
// 2. Write plan JSON to workspace
// 3. Generate sandbox script based on requirements
// 4. Mount tsuku binary, plan, and cache into container
// 5. Run container with configured limits
// 6. Check verification output
func (e *Executor) Sandbox(
	ctx context.Context,
	plan *executor.InstallationPlan,
	target platform.Target,
	reqs *SandboxRequirements,
) (*SandboxResult, error) {
	// Start timing before runtime detection (wall-clock time for the full operation)
	startTime := time.Now()

	// Detect container runtime
	runtime, err := e.detector.Detect(ctx)
	if err != nil {
		if err == validate.ErrNoRuntime {
			e.logger.Warn("Container runtime not available. Skipping sandbox test.",
				"hint", "To enable sandbox testing, install Podman or Docker.")
			return &SandboxResult{
				Skipped:    true,
				DurationMs: time.Since(startTime).Milliseconds(),
			}, nil
		}
		return nil, fmt.Errorf("failed to detect container runtime: %w", err)
	}

	// Check if we have a valid tsuku binary
	if e.tsukuBinary == "" {
		e.logger.Warn("Tsuku binary not found. Skipping sandbox test.",
			"hint", "Ensure tsuku is installed and in PATH, or build with 'go build -o tsuku ./cmd/tsuku'")
		return &SandboxResult{
			Skipped:    true,
			DurationMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// Emit security warning for Docker with group membership (non-rootless)
	if runtime.Name() == "docker" && !runtime.IsRootless() {
		e.logger.Warn("Using Docker with docker group membership.",
			"security", "This grants root-equivalent access on this machine.",
			"recommendation", "Consider configuring Docker rootless mode for better security.",
			"docs", "https://docs.docker.com/engine/security/rootless/")
	}

	// Extract system requirements (packages + repositories) from plan
	// The plan is already filtered for the target platform during plan generation,
	// so we can extract requirements directly without additional filtering.
	sysReqs := ExtractSystemRequirements(plan)

	// Determine the effective linux family: prefer plan's, fall back to target's
	effectiveFamily := plan.Platform.LinuxFamily
	if effectiveFamily == "" {
		effectiveFamily = target.LinuxFamily()
	}

	// Add infrastructure packages needed for sandbox execution
	sysReqs = augmentWithInfrastructurePackages(sysReqs, plan, reqs, effectiveFamily)

	// Determine which image to use
	containerImage := reqs.Image
	if sysReqs != nil {
		// Derive container spec from system requirements
		spec, err := DeriveContainerSpec(sysReqs)
		if err != nil {
			return nil, fmt.Errorf("failed to derive container spec: %w", err)
		}

		// Generate image name for caching
		imageName := ContainerImageName(spec)

		// Check if image already exists
		exists, err := runtime.ImageExists(ctx, imageName)
		if err != nil {
			return nil, fmt.Errorf("failed to check image existence: %w", err)
		}

		// Build image if it doesn't exist
		if !exists {
			e.logger.Debug("Building custom container image",
				"image", imageName,
				"base", spec.BaseImage,
				"family", spec.LinuxFamily)

			if err := runtime.Build(ctx, imageName, spec.BaseImage, spec.BuildCommands); err != nil {
				return nil, fmt.Errorf("failed to build container image: %w", err)
			}
		} else {
			e.logger.Debug("Using cached container image",
				"image", imageName)
		}

		containerImage = imageName
	}

	// Build foundation image if the plan has InstallTime dependencies.
	// FlattenDependencies extracts the dependency tree into a topologically
	// ordered flat list. If non-empty, BuildFoundationImage creates a Docker
	// image with each dependency pre-installed as a cached layer. The
	// container's $TSUKU_HOME filesystem is preserved via targeted mounts
	// (not a broad /workspace mount), so pre-installed tools are found by
	// the executor's skip logic natively.
	deps := FlattenDependencies(plan)
	if len(deps) > 0 {
		foundationImage, err := e.BuildFoundationImage(ctx, runtime, containerImage, effectiveFamily, deps)
		if err != nil {
			return nil, fmt.Errorf("failed to build foundation image: %w", err)
		}
		containerImage = foundationImage
	}

	e.logger.Debug("Running sandbox test",
		"tool", plan.Tool,
		"runtime", runtime.Name(),
		"image", containerImage,
		"network", reqs.RequiresNetwork)

	// Create workspace directory
	workspaceDir, err := os.MkdirTemp("", TempDirPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspaceDir) }()

	// Use external download cache if provided, otherwise create empty one
	// External cache should contain pre-downloaded files from plan generation
	cacheDir := e.downloadCacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(workspaceDir, "cache", "downloads")
		if err := os.MkdirAll(cacheDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	// Create output directory for verification markers
	outputDir := filepath.Join(workspaceDir, "output")
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
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

	// Build the sandbox script
	script := e.buildSandboxScript(plan, reqs)

	// Write script to workspace
	scriptPath := filepath.Join(workspaceDir, "sandbox.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return nil, fmt.Errorf("failed to write sandbox script: %w", err)
	}

	// Configure network mode
	network := "none"
	if reqs.RequiresNetwork {
		network = "host"
	}

	// Convert sandbox.ResourceLimits to validate.ResourceLimits
	limits := validate.ResourceLimits{
		Memory:   reqs.Resources.Memory,
		CPUs:     reqs.Resources.CPUs,
		PidsMax:  reqs.Resources.PidsMax,
		Timeout:  reqs.Resources.Timeout,
		ReadOnly: false, // Need to install packages
	}

	// Build environment: hardcoded sandbox vars first, then filtered user vars
	env := []string{
		"TSUKU_SANDBOX=1",
		"TSUKU_HOME=/workspace/tsuku",
		"HOME=/workspace",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
	}
	if extra := filterExtraEnv(reqs.ExtraEnv); len(extra) > 0 {
		env = append(env, extra...)
	}

	// Signal shared cargo registry cache to the cargo_build action.
	// The action reads TSUKU_CARGO_REGISTRY_CACHE after creating its
	// isolated CARGO_HOME and symlinks $CARGO_HOME/registry to this path.
	if e.cargoRegistryCacheDir != "" {
		env = append(env, "TSUKU_CARGO_REGISTRY_CACHE=/workspace/cargo-registry-cache")
	}

	// Build run options with targeted mounts instead of a single broad
	// /workspace mount. This preserves the container's $TSUKU_HOME filesystem
	// (e.g., tools pre-installed by a foundation image) while exchanging only
	// the specific files the host needs to provide or receive.
	opts := validate.RunOptions{
		Image:   containerImage,
		Command: []string{"/bin/sh", "/workspace/sandbox.sh"},
		Network: network,
		WorkDir: "/workspace",
		Env:     env,
		Limits:  limits,
		Labels: map[string]string{
			ContainerLabelPrefix: "true",
		},
		Mounts: []validate.Mount{
			{
				Source:   planPath,
				Target:   "/workspace/plan.json",
				ReadOnly: true,
			},
			{
				Source:   scriptPath,
				Target:   "/workspace/sandbox.sh",
				ReadOnly: true,
			},
			{
				Source:   cacheDir,
				Target:   "/workspace/tsuku/cache/downloads",
				ReadOnly: true,
			},
			{
				Source:   outputDir,
				Target:   "/workspace/output",
				ReadOnly: false,
			},
		},
	}

	// Mount tsuku binary if available
	if e.tsukuBinary != "" {
		opts.Mounts = append(opts.Mounts, validate.Mount{
			Source:   e.tsukuBinary,
			Target:   "/usr/local/bin/tsuku",
			ReadOnly: true,
		})
	}

	// Mount shared cargo registry cache if configured.
	// The cargo_build action reads TSUKU_CARGO_REGISTRY_CACHE and creates
	// the symlink from $CARGO_HOME/registry to this mount.
	if e.cargoRegistryCacheDir != "" {
		opts.Mounts = append(opts.Mounts, validate.Mount{
			Source:   e.cargoRegistryCacheDir,
			Target:   "/workspace/cargo-registry-cache",
			ReadOnly: false,
		})
	}

	// Run the container
	result, err := runtime.Run(ctx, opts)
	if err != nil {
		return &SandboxResult{
			Passed:         false,
			ExitCode:       -1,
			Stdout:         result.Stdout,
			Stderr:         result.Stderr,
			Error:          err,
			Verified:       false,
			VerifyExitCode: -1,
			DurationMs:     time.Since(startTime).Milliseconds(),
		}, nil
	}

	// If install itself failed, don't check verification
	if result.ExitCode != 0 {
		return &SandboxResult{
			Passed:         false,
			ExitCode:       result.ExitCode,
			Stdout:         result.Stdout,
			Stderr:         result.Stderr,
			Verified:       false,
			VerifyExitCode: -1,
			DurationMs:     time.Since(startTime).Milliseconds(),
		}, nil
	}

	// Install succeeded. Now check verification results.
	// Markers are written to the output directory mount, not workspaceDir directly.
	verified, verifyExitCode := e.readVerifyResults(outputDir, plan)

	return &SandboxResult{
		Passed:         result.ExitCode == 0 && verified,
		ExitCode:       result.ExitCode,
		Stdout:         result.Stdout,
		Stderr:         result.Stderr,
		Verified:       verified,
		VerifyExitCode: verifyExitCode,
		DurationMs:     time.Since(startTime).Milliseconds(),
	}, nil
}

// readVerifyResults reads verification marker files from the output directory
// and evaluates the verify results using executor.CheckPlanVerification.
// The outputDir corresponds to the host-side path mounted at /workspace/output/.
// Returns (verified, verifyExitCode).
// If no verify command exists, returns (true, -1).
func (e *Executor) readVerifyResults(outputDir string, plan *executor.InstallationPlan) (bool, int) {
	// If no verify command, verification is considered passed
	if plan.Verify == nil || plan.Verify.Command == "" {
		return true, -1
	}

	// Read marker files from the output directory
	exitPath := filepath.Join(outputDir, verifyExitMarker)
	outputPath := filepath.Join(outputDir, verifyOutputMarker)

	exitData, err := os.ReadFile(exitPath)
	if err != nil {
		// Marker files don't exist -- something went wrong with the verify step
		e.logger.Debug("Failed to read verify exit marker", "error", err)
		return false, -1
	}

	verifyExitCode, err := strconv.Atoi(strings.TrimSpace(string(exitData)))
	if err != nil {
		e.logger.Debug("Failed to parse verify exit code", "error", err)
		return false, -1
	}

	output := ""
	outputData, err := os.ReadFile(outputPath)
	if err == nil {
		output = string(outputData)
	}

	// Determine expected exit code (default 0)
	expectedExitCode := 0
	if plan.Verify.ExitCode != nil {
		expectedExitCode = *plan.Verify.ExitCode
	}

	verified := executor.CheckPlanVerification(verifyExitCode, output, expectedExitCode, plan.Verify.Pattern)
	return verified, verifyExitCode
}

// augmentWithInfrastructurePackages adds packages needed for sandbox execution
// to the system requirements. These are installed via the container build step
// using the family-appropriate package manager.
func augmentWithInfrastructurePackages(
	sysReqs *SystemRequirements,
	plan *executor.InstallationPlan,
	reqs *SandboxRequirements,
	effectiveFamily string,
) *SystemRequirements {
	// Determine what infrastructure packages are needed
	needsNetwork := reqs.RequiresNetwork
	needsBuild := hasBuildActions(plan)

	if !needsNetwork && !needsBuild {
		return sysReqs
	}

	// Determine the package manager from existing sysReqs or effective linux family
	pm := ""
	if sysReqs != nil && len(sysReqs.Packages) > 0 {
		// Use the package manager already in sysReqs
		for p := range sysReqs.Packages {
			pm = p
			break
		}
	} else if effectiveFamily != "" {
		// Map linux_family to package manager
		switch effectiveFamily {
		case "debian":
			pm = "apt"
		case "rhel":
			pm = "dnf"
		case "arch":
			pm = "pacman"
		case "alpine":
			pm = "apk"
		case "suse":
			pm = "zypper"
		}
	}

	// If no package manager can be determined, return as-is
	if pm == "" {
		return sysReqs
	}

	// Initialize sysReqs if nil
	if sysReqs == nil {
		sysReqs = &SystemRequirements{
			Packages: make(map[string][]string),
		}
	}
	if sysReqs.Packages == nil {
		sysReqs.Packages = make(map[string][]string)
	}

	// Add infrastructure packages from container-images.json config.
	// Core packages are always added — they cover utilities like tar/gzip that
	// most base images include but some (e.g., opensuse/leap) do not.
	var infraPkgs []string
	infraPkgs = append(infraPkgs, containerimages.InfraPackages(effectiveFamily, "core")...)
	if needsNetwork {
		infraPkgs = append(infraPkgs, containerimages.InfraPackages(effectiveFamily, "network")...)
	}
	if needsBuild {
		infraPkgs = append(infraPkgs, containerimages.InfraPackages(effectiveFamily, "build")...)
	}

	sysReqs.Packages[pm] = append(sysReqs.Packages[pm], infraPkgs...)
	return sysReqs
}

// buildSandboxScript creates the shell script for sandbox testing.
// The script sets up the environment and runs tsuku install --plan.
// Infrastructure packages are installed via the container build step,
// not in this script.
// Uses /bin/sh for portability (Alpine uses ash, not bash).
func (e *Executor) buildSandboxScript(
	plan *executor.InstallationPlan,
	reqs *SandboxRequirements,
) string {
	var sb strings.Builder

	sb.WriteString("#!/bin/sh\n")
	sb.WriteString("set -e\n\n")

	// Create TSUKU_HOME structure if not provided by foundation image.
	// When a foundation image pre-installs dependencies, the directory
	// structure (and state.json) already exists on the container's filesystem.
	// Creating it unconditionally would clobber that state.
	sb.WriteString("# Create TSUKU_HOME structure if not provided by foundation image\n")
	sb.WriteString("if [ ! -d /workspace/tsuku/tools ]; then\n")
	sb.WriteString("  mkdir -p /workspace/tsuku/recipes /workspace/tsuku/bin /workspace/tsuku/tools\n")
	sb.WriteString("fi\n\n")

	// Add tools/current and bin to PATH so pre-installed tool binaries are available.
	// tools/current contains symlinks created by install_binaries (e.g., patchelf,
	// cmake). bin is legacy but kept for compatibility.
	sb.WriteString("# Add TSUKU_HOME paths for dependency binaries\n")
	sb.WriteString("export PATH=/workspace/tsuku/tools/current:/workspace/tsuku/bin:$PATH\n\n")

	// NOTE: Cargo registry cache sharing is handled via the
	// TSUKU_CARGO_REGISTRY_CACHE environment variable, which is set in the
	// container env when cargoRegistryCacheDir is configured. The cargo_build
	// action reads this variable after creating CARGO_HOME and symlinks
	// $CARGO_HOME/registry to the shared mount. This works because CARGO_HOME
	// is only known after buildDeterministicCargoEnv() runs (it creates an
	// isolated per-build directory), so a shell-level guard on $CARGO_HOME
	// in this script would never trigger.

	// Run tsuku install with pre-generated plan
	// tsuku handles build tool dependencies automatically via ActionDependencies
	sb.WriteString("# Run tsuku install with pre-generated plan\n")
	sb.WriteString("tsuku install --plan /workspace/plan.json --force\n")

	// Append verify block if the plan has a verify command.
	// Markers are written to /workspace/output/ which is the only read-write mount.
	if plan.Verify != nil && plan.Verify.Command != "" {
		sb.WriteString("\n# Run verify command and capture results to marker files\n")
		sb.WriteString("set +e\n")
		sb.WriteString("export PATH=\"$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH\"\n")
		// Expand {install_dir} to the tool's install path in the sandbox.
		// The sandbox sets TSUKU_HOME=/workspace/tsuku, so the install dir
		// follows the same tool-version convention as the host.
		installDir := fmt.Sprintf("$TSUKU_HOME/tools/%s-%s", plan.Tool, plan.Version)
		verifyCmd := strings.ReplaceAll(plan.Verify.Command, "{install_dir}", installDir)
		sb.WriteString(fmt.Sprintf("%s > /workspace/output/%s 2>&1\n", verifyCmd, verifyOutputMarker))
		sb.WriteString(fmt.Sprintf("echo $? > /workspace/output/%s\n", verifyExitMarker))
	}

	return sb.String()
}

// filterExtraEnv returns the subset of extra env vars that don't collide with
// the hardcoded sandbox environment variables. Each entry is expected in
// KEY=VALUE format. Entries whose key matches a protected key are silently
// dropped. Entries without an '=' separator are treated as KEY-only (the
// caller resolves these to KEY= before calling this function).
func filterExtraEnv(extra []string) []string {
	if len(extra) == 0 {
		return nil
	}
	var filtered []string
	for _, entry := range extra {
		key := entry
		if idx := strings.IndexByte(entry, '='); idx >= 0 {
			key = entry[:idx]
		}
		if protectedEnvKeys[key] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
