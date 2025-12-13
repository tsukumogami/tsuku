package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// PipInstallAction installs Python packages using pip with deterministic configuration.
// This is an ecosystem primitive that achieves determinism through pip's hash-checking
// mode and controlled environment variables.
type PipInstallAction struct{}

// Name returns the action name
func (a *PipInstallAction) Name() string {
	return "pip_install"
}

// Execute installs Python packages via pip with deterministic flags.
//
// Parameters:
//   - source_dir (optional): Directory containing setup.py/pyproject.toml
//   - requirements (optional): Path to requirements.txt file (should contain hashes when use_hashes=true)
//   - constraints (optional): Path to constraints.txt file
//   - python_version (required): Required Python version (e.g., "3.11")
//   - use_hashes (optional): Require hash checking (default: false)
//   - output_dir (optional): Installation target directory (defaults to venv in install_dir)
//   - python_path (optional): Path to Python interpreter (auto-detects python-standalone)
//
// Environment Setup:
//   - SOURCE_DATE_EPOCH is set to ensure reproducible timestamps
//   - PYTHONDONTWRITEBYTECODE=1 to avoid .pyc variations
//   - PYTHONHASHSEED=0 for deterministic hash ordering
//
// Deterministic Flags (when use_hashes is true):
//   - --require-hashes: Verify all packages against hashes in requirements
//   - --only-binary :all:: Only install wheels (no sdist compilation)
//   - --no-deps: Don't install dependencies (assumes requirements.txt is complete)
func (a *PipInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get python version (required)
	pythonVersion, ok := GetString(params, "python_version")
	if !ok {
		return fmt.Errorf("pip_install action requires 'python_version' parameter")
	}

	// Get optional parameters
	sourceDir, _ := GetString(params, "source_dir")
	requirements, _ := GetString(params, "requirements")
	constraints, _ := GetString(params, "constraints")
	outputDir, _ := GetString(params, "output_dir")
	useHashes, _ := GetBool(params, "use_hashes")

	// At least one of source_dir or requirements must be specified
	if sourceDir == "" && requirements == "" {
		return fmt.Errorf("pip_install action requires either 'source_dir' or 'requirements' parameter")
	}

	// Get python interpreter path
	pythonPath, _ := GetString(params, "python_path")
	if pythonPath == "" {
		pythonPath = ResolvePythonStandalone()
		if pythonPath == "" {
			// Fall back to python3 in PATH
			pythonPath = "python3"
		}
	}

	// Validate Python version
	if err := validatePythonVersion(pythonPath, pythonVersion); err != nil {
		return fmt.Errorf("python version validation failed: %w", err)
	}

	// Set up output directory (venv location)
	if outputDir == "" {
		outputDir = filepath.Join(ctx.InstallDir, "venv")
	}

	// Resolve relative paths
	if sourceDir != "" && !filepath.IsAbs(sourceDir) {
		sourceDir = filepath.Join(ctx.WorkDir, sourceDir)
	}
	if requirements != "" && !filepath.IsAbs(requirements) {
		requirements = filepath.Join(ctx.WorkDir, requirements)
	}
	if constraints != "" && !filepath.IsAbs(constraints) {
		constraints = filepath.Join(ctx.WorkDir, constraints)
	}

	// Display configuration to user
	fmt.Printf("   Python version: %s\n", pythonVersion)
	fmt.Printf("   Use hashes: %v\n", useHashes)
	fmt.Printf("   Output directory: %s\n", outputDir)
	if requirements != "" {
		fmt.Printf("   Requirements: %s\n", requirements)
	}
	if sourceDir != "" {
		fmt.Printf("   Source directory: %s\n", sourceDir)
	}

	// Step 1: Create virtual environment
	fmt.Printf("   Creating virtual environment...\n")
	if err := createVirtualEnv(pythonPath, outputDir); err != nil {
		return fmt.Errorf("failed to create virtual environment: %w", err)
	}

	// Step 2: Build pip install command
	pipPath := filepath.Join(outputDir, "bin", "pip")
	args := buildPipInstallArgs(sourceDir, requirements, constraints, useHashes)

	fmt.Printf("   Installing packages: pip %s\n", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx.Context, pipPath, args...)
	cmd.Dir = ctx.WorkDir

	// Set up deterministic environment
	env := os.Environ()
	env = append(env, fmt.Sprintf("SOURCE_DATE_EPOCH=%d", getSourceDateEpoch()))
	env = append(env, "PYTHONDONTWRITEBYTECODE=1")
	env = append(env, "PYTHONHASHSEED=0") // Deterministic hash ordering

	// Add any extra exec paths (e.g., for python-standalone)
	if len(ctx.ExecPaths) > 0 {
		pathVal := os.Getenv("PATH")
		for _, p := range ctx.ExecPaths {
			pathVal = p + ":" + pathVal
		}
		env = append(env, fmt.Sprintf("PATH=%s", pathVal))
	}
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip install failed: %w\nOutput: %s", err, string(output))
	}

	// Show output if debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		ctx.Log().Debug("pip_install: pip output", "output", outputStr)
	}

	fmt.Printf("   pip install completed successfully\n")

	return nil
}

// validatePythonVersion checks that the Python interpreter matches the required version.
func validatePythonVersion(pythonPath, requiredVersion string) error {
	cmd := exec.Command(pythonPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get Python version: %w", err)
	}

	// Output format: "Python 3.11.7"
	versionStr := strings.TrimSpace(string(output))
	parts := strings.Split(versionStr, " ")
	if len(parts) != 2 {
		return fmt.Errorf("unexpected Python version output: %s", versionStr)
	}

	actualVersion := parts[1]

	// Check if the actual version starts with the required version
	// This allows "3.11" to match "3.11.7"
	if !strings.HasPrefix(actualVersion, requiredVersion) {
		return fmt.Errorf("Python version mismatch: requires %s, found %s", requiredVersion, actualVersion)
	}

	return nil
}

// createVirtualEnv creates a Python virtual environment at the specified path.
func createVirtualEnv(pythonPath, venvPath string) error {
	cmd := exec.Command(pythonPath, "-m", "venv", venvPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("venv creation failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// buildPipInstallArgs constructs the pip install command arguments.
func buildPipInstallArgs(sourceDir, requirements, constraints string, useHashes bool) []string {
	args := []string{"install"}

	// Deterministic flags when using hashes (per ecosystem_pip.md spec)
	if useHashes {
		args = append(args, "--require-hashes")
		args = append(args, "--no-deps")
		args = append(args, "--only-binary", ":all:")
	}

	// Disable pip version check (reduces non-determinism)
	args = append(args, "--disable-pip-version-check")

	// Add constraints file if specified
	if constraints != "" {
		args = append(args, "-c", constraints)
	}

	// Add requirements file or source directory
	if requirements != "" {
		args = append(args, "-r", requirements)
	} else if sourceDir != "" {
		args = append(args, sourceDir)
	}

	return args
}

// getSourceDateEpoch returns a reproducible timestamp for builds.
// Uses the value from SOURCE_DATE_EPOCH env var if set, otherwise uses a fixed epoch.
func getSourceDateEpoch() int64 {
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		if val, err := strconv.ParseInt(epoch, 10, 64); err == nil {
			return val
		}
	}
	// Use a fixed, reproducible epoch (2024-01-01 00:00:00 UTC)
	return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
}
