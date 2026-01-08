package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

// Executor executes action-based recipes
type Executor struct {
	workDir          string
	installDir       string
	downloadCacheDir string // Download cache directory
	keyCacheDir      string // PGP key cache directory
	recipe           *recipe.Recipe
	ctx              *actions.ExecutionContext
	version          string               // Resolved version
	reqVersion       string               // Requested version (optional)
	execPaths        []string             // Additional bin paths for execution (e.g., nodejs for npm tools)
	toolsDir         string               // Tools directory (~/.tsuku/tools/) for finding other installed tools
	libsDir          string               // Libraries directory (~/.tsuku/libs/) for finding installed libraries
	resolvedDeps     actions.ResolvedDeps // Pre-resolved dependencies (from state manager)
}

// New creates a new executor
func New(r *recipe.Recipe) (*Executor, error) {
	// Create temporary work directory
	workDir, err := os.MkdirTemp("", "action-validator-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create work dir: %w", err)
	}

	// Create temporary install directory (use .install to avoid conflicts with archive contents)
	installDir := filepath.Join(workDir, ".install")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create install dir: %w", err)
	}

	return &Executor{
		workDir:    workDir,
		installDir: installDir,
		recipe:     r,
	}, nil
}

// NewWithVersion creates a new executor with a requested version
func NewWithVersion(r *recipe.Recipe, version string) (*Executor, error) {
	exec, err := New(r)
	if err != nil {
		return nil, err
	}
	exec.reqVersion = version
	return exec, nil
}

// SetDownloadCacheDir sets the download cache directory
func (e *Executor) SetDownloadCacheDir(dir string) {
	e.downloadCacheDir = dir
}

// SetKeyCacheDir sets the PGP key cache directory
func (e *Executor) SetKeyCacheDir(dir string) {
	e.keyCacheDir = dir
}

// resolveVersionWith attempts to resolve the latest version for the recipe using the given resolver
func (e *Executor) resolveVersionWith(ctx context.Context, resolver *version.Resolver) (*version.VersionInfo, error) {
	// Use unified provider factory
	factory := version.NewProviderFactory()
	provider, err := factory.ProviderFromRecipe(resolver, e.recipe)
	if err != nil {
		return nil, err
	}

	// Resolve version using provider
	if e.reqVersion != "" {
		return provider.ResolveVersion(ctx, e.reqVersion)
	}
	return provider.ResolveLatest(ctx)
}

// ResolveVersion resolves a version constraint to a concrete version string.
// This is Phase 1 of the two-phase installation model: version resolution runs
// before cache lookup to determine the cache key.
//
// If constraint is empty, resolves to the latest version.
// If constraint is specified, attempts to resolve that specific version.
// Returns the resolved version string (e.g., "14.1.0").
func (e *Executor) ResolveVersion(ctx context.Context, constraint string) (string, error) {
	resolver := version.New()
	factory := version.NewProviderFactory()
	provider, err := factory.ProviderFromRecipe(resolver, e.recipe)
	if err != nil {
		return "", err
	}

	var versionInfo *version.VersionInfo
	if constraint == "" {
		versionInfo, err = provider.ResolveLatest(ctx)
	} else {
		versionInfo, err = provider.ResolveVersion(ctx, constraint)
	}
	if err != nil {
		return "", err
	}

	return versionInfo.Version, nil
}

// shouldExecute checks if a step should be executed based on 'when' conditions.
// Evaluates platform conditions (OS/arch) and package manager availability.
func (e *Executor) shouldExecute(when *recipe.WhenClause) bool {
	if when == nil || when.IsEmpty() {
		return true
	}

	// Check platform conditions (OS and arch)
	// Note: At runtime, linux_family would be detected if needed
	target := recipe.NewMatchTarget(runtime.GOOS, runtime.GOARCH, "")
	if !when.Matches(target) {
		return false
	}

	// Check package_manager condition (stub - always true for validation)
	// In real implementation, would detect system package manager (brew, apt, etc.)
	if when.PackageManager != "" {
		return true
	}

	return true
}

// Cleanup removes temporary directories
func (e *Executor) Cleanup() {
	if e.workDir != "" {
		os.RemoveAll(e.workDir)
	}
}

// Version returns the resolved version
func (e *Executor) Version() string {
	return e.version
}

// WorkDir returns the work directory
func (e *Executor) WorkDir() string {
	return e.workDir
}

// SetExecPaths sets additional bin paths needed for execution (e.g., nodejs for npm tools)
func (e *Executor) SetExecPaths(paths []string) {
	e.execPaths = paths
}

// SetToolsDir sets the tools directory for finding other installed tools
func (e *Executor) SetToolsDir(dir string) {
	e.toolsDir = dir
}

// SetLibsDir sets the libraries directory for finding installed libraries
func (e *Executor) SetLibsDir(dir string) {
	e.libsDir = dir
}

// SetResolvedDeps sets the pre-resolved dependency versions (from state manager).
// When dependencies are installed before generating the plan, the plan's Dependencies
// will be empty. Use this to pass the actual installed dependency versions so they
// can be used in variable expansion (e.g., {deps.openssl.version}).
func (e *Executor) SetResolvedDeps(deps actions.ResolvedDeps) {
	e.resolvedDeps = deps
}

// expandVars replaces {var} placeholders in a string
func expandVars(s string, vars map[string]string) string {
	result := s
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}

// DryRun shows what would be done without actually executing
func (e *Executor) DryRun(ctx context.Context) error {
	// Create version resolver
	resolver := version.New()

	// Resolve version from recipe steps
	versionInfo, err := e.resolveVersionWith(ctx, resolver)
	if err != nil {
		return fmt.Errorf("version resolution failed: %w", err)
	}

	// Store version
	e.version = versionInfo.Version

	// Print dry-run header
	fmt.Printf("Would install: %s@%s\n", e.recipe.Metadata.Name, versionInfo.Version)

	// Print dependencies
	if len(e.recipe.Metadata.Dependencies) > 0 {
		fmt.Printf("  Dependencies: %s\n", strings.Join(e.recipe.Metadata.Dependencies, ", "))
	} else {
		fmt.Printf("  Dependencies: (none)\n")
	}

	// Print actions
	fmt.Printf("  Actions:\n")

	// Build variable map for expansion
	vars := actions.GetStandardVars(versionInfo.Version, "", "", "")

	stepNum := 0
	for _, step := range e.recipe.Steps {
		// Check conditional execution
		if !e.shouldExecute(step.When) {
			continue
		}

		stepNum++
		actionDesc := formatActionDescription(step.Action, step.Params, vars)
		fmt.Printf("    %d. %s: %s\n", stepNum, step.Action, actionDesc)
	}

	// Print verification command
	if e.recipe.Verify.Command != "" {
		fmt.Printf("  Verification: %s\n", e.recipe.Verify.Command)
	}

	return nil
}

// formatActionDescription formats action parameters for dry-run display
func formatActionDescription(action string, params map[string]interface{}, vars map[string]string) string {
	switch action {
	case "download":
		if url, ok := params["url"].(string); ok {
			return actions.ExpandVars(url, vars)
		}
	case "extract":
		if src, ok := params["src"].(string); ok {
			return actions.ExpandVars(src, vars)
		}
	case "install_binaries":
		if bins, ok := params["binaries"].([]interface{}); ok {
			names := make([]string, len(bins))
			for i, b := range bins {
				if name, ok := b.(string); ok {
					names[i] = name
				} else if m, ok := b.(map[string]interface{}); ok {
					if name, ok := m["name"].(string); ok {
						names[i] = name
					}
				}
			}
			return strings.Join(names, ", ")
		}
	case "chmod":
		if file, ok := params["file"].(string); ok {
			mode := "755"
			if m, ok := params["mode"].(string); ok {
				mode = m
			}
			return fmt.Sprintf("%s (mode %s)", actions.ExpandVars(file, vars), mode)
		}
	case "cargo_install", "npm_install", "pipx_install", "gem_install":
		if pkg, ok := params["package"].(string); ok {
			return pkg
		}
	case "run_command":
		if cmd, ok := params["command"].(string); ok {
			expanded := actions.ExpandVars(cmd, vars)
			if len(expanded) > 60 {
				return expanded[:57] + "..."
			}
			return expanded
		}
	}
	return ""
}

// ExecutePlan executes an installation plan, verifying checksums for download steps.
// All downloads are verified against the checksums recorded in the plan.
// Returns ChecksumMismatchError if a download's checksum doesn't match the plan.
// Returns PlanValidationError if the plan contains invalid actions or missing checksums.
func (e *Executor) ExecutePlan(ctx context.Context, plan *InstallationPlan) error {
	// Validate plan before execution (ensures primitives-only, checksums present, etc.)
	if err := ValidatePlan(plan); err != nil {
		return fmt.Errorf("plan validation failed: %w", err)
	}

	// Validate platform compatibility
	if err := validatePlatform(plan); err != nil {
		return fmt.Errorf("platform validation failed: %w", err)
	}

	// Validate resource limits (dependency depth and count)
	if err := validateResourceLimits(plan); err != nil {
		return fmt.Errorf("resource limits exceeded: %w", err)
	}

	fmt.Printf("Executing plan: %s@%s\n", plan.Tool, plan.Version)
	fmt.Printf("   Work directory: %s\n", e.workDir)

	// Count total steps including dependencies
	totalDepSteps := countDependencySteps(plan.Dependencies)
	fmt.Printf("   Total steps: %d (including %d from dependencies)\n",
		len(plan.Steps)+totalDepSteps, totalDepSteps)

	// Install dependencies first (depth-first, each in its own work directory)
	if err := e.installDependencies(ctx, plan.Dependencies, plan.Platform); err != nil {
		return fmt.Errorf("failed to install dependencies: %w", err)
	}

	// Now execute main tool's steps (original flattening approach but only for main tool)
	allSteps := plan.Steps

	// Store version for later use
	e.version = plan.Version

	// When executing from a plan file, the recipe may be nil or have empty Verify.
	// If the plan has verify info, use it to create/update the recipe context.
	recipeForContext := e.recipe
	if plan.Verify != nil && plan.Verify.Command != "" {
		if recipeForContext == nil || recipeForContext.Verify.Command == "" {
			recipeForContext = &recipe.Recipe{
				Metadata: recipe.MetadataSection{
					Name: plan.Tool,
					Type: plan.RecipeType,
				},
				Verify: recipe.VerifySection{
					Command: plan.Verify.Command,
					Pattern: plan.Verify.Pattern,
				},
			}
		}
	}

	// Build resolved dependencies: prefer pre-resolved deps (from state manager)
	// over plan deps. When dependencies are installed before plan generation,
	// plan.Dependencies will be empty but e.resolvedDeps will be populated.
	resolvedDeps := e.resolvedDeps
	if len(resolvedDeps.InstallTime) == 0 && len(resolvedDeps.Runtime) == 0 {
		// Fall back to building from plan if no pre-resolved deps
		resolvedDeps = buildResolvedDepsFromPlan(plan.Dependencies)
	}

	// Create execution context from plan
	execCtx := &actions.ExecutionContext{
		Context:          ctx,
		WorkDir:          e.workDir,
		InstallDir:       e.installDir,
		ToolInstallDir:   "",
		ToolsDir:         e.toolsDir,
		LibsDir:          e.libsDir,
		DownloadCacheDir: e.downloadCacheDir,
		KeyCacheDir:      e.keyCacheDir,
		Version:          plan.Version,
		VersionTag:       plan.Version, // Plan doesn't track tag separately
		OS:               plan.Platform.OS,
		Arch:             plan.Platform.Arch,
		Recipe:           recipeForContext,
		ExecPaths:        e.execPaths,
		Logger:           log.Default(),
		Dependencies:     resolvedDeps,
	}
	e.ctx = execCtx

	fmt.Println()

	// Validate all steps before execution (fail fast)
	for i, step := range allSteps {
		action := actions.Get(step.Action)
		if action == nil {
			return fmt.Errorf("step %d: unknown action '%s'", i+1, step.Action)
		}
		if result := actions.ValidateAction(step.Action, step.Params); result.HasErrors() {
			return fmt.Errorf("step %d (%s): %s", i+1, step.Action, result.ToError())
		}
	}

	// Execute each step (including flattened dependency steps)
	for i, step := range allSteps {
		// Check for context cancellation
		if err := ctx.Err(); err != nil {
			return err
		}

		fmt.Printf("Step %d/%d: %s\n", i+1, len(allSteps), step.Action)

		// Get action
		action := actions.Get(step.Action)
		if action == nil {
			return fmt.Errorf("unknown action: %s", step.Action)
		}

		// For download steps with checksums, verify after download
		if step.Action == "download" && step.Checksum != "" {
			if err := e.executeDownloadWithVerification(ctx, execCtx, step, plan); err != nil {
				return fmt.Errorf("step %d (%s) failed: %w", i+1, step.Action, err)
			}
		} else {
			// Execute (validation already done upfront)
			if err := action.Execute(execCtx, step.Params); err != nil {
				return fmt.Errorf("step %d (%s) failed: %w", i+1, step.Action, err)
			}
		}

		// After install_binaries completes, add the bin directory to ExecPaths
		// so subsequent steps (like npm_exec) can find the installed binaries
		if step.Action == "install_binaries" {
			binDir := filepath.Join(execCtx.InstallDir, "bin")
			if _, err := os.Stat(binDir); err == nil {
				execCtx.ExecPaths = append(execCtx.ExecPaths, binDir)
				fmt.Printf("   Added %s to ExecPaths\n", binDir)
				// Debug: list files in bin directory
				if entries, err := os.ReadDir(binDir); err == nil {
					fmt.Printf("   Contents of %s:\n", binDir)
					for _, e := range entries {
						fmt.Printf("      - %s\n", e.Name())
					}
				}
			} else {
				fmt.Printf("   Warning: bin dir %s does not exist: %v\n", binDir, err)
			}
		}

		fmt.Println()
	}

	return nil
}

// executeDownloadWithVerification downloads a file and verifies its checksum against the plan.
func (e *Executor) executeDownloadWithVerification(
	ctx context.Context,
	execCtx *actions.ExecutionContext,
	step ResolvedStep,
	plan *InstallationPlan,
) error {
	// Execute the download action (validation already done upfront)
	action := actions.Get("download")
	if err := action.Execute(execCtx, step.Params); err != nil {
		return err
	}

	// Determine the destination file path
	destPath := e.resolveDownloadDest(step, execCtx)

	// Compute checksum of downloaded file
	actualChecksum, err := computeFileChecksum(destPath)
	if err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	// Verify checksum matches plan
	expectedChecksum := strings.ToLower(strings.TrimSpace(step.Checksum))
	// Strip algorithm prefix if present (e.g., "sha256:abc123" -> "abc123")
	if idx := strings.Index(expectedChecksum, ":"); idx != -1 {
		expectedChecksum = expectedChecksum[idx+1:]
	}
	if actualChecksum != expectedChecksum {
		return &ChecksumMismatchError{
			Tool:             plan.Tool,
			Version:          plan.Version,
			URL:              step.URL,
			ExpectedChecksum: expectedChecksum,
			ActualChecksum:   actualChecksum,
		}
	}

	fmt.Printf("   Checksum verified\n")
	return nil
}

// resolveDownloadDest determines the destination file path for a download step.
func (e *Executor) resolveDownloadDest(step ResolvedStep, execCtx *actions.ExecutionContext) string {
	// Check for explicit dest in params
	if dest, ok := step.Params["dest"].(string); ok && dest != "" {
		return filepath.Join(execCtx.WorkDir, dest)
	}

	// Fall back to basename of URL
	if step.URL != "" {
		dest := filepath.Base(step.URL)
		// Remove query parameters if present
		if idx := strings.Index(dest, "?"); idx != -1 {
			dest = dest[:idx]
		}
		return filepath.Join(execCtx.WorkDir, dest)
	}

	// Last resort: check url param
	if url, ok := step.Params["url"].(string); ok {
		dest := filepath.Base(url)
		if idx := strings.Index(dest, "?"); idx != -1 {
			dest = dest[:idx]
		}
		return filepath.Join(execCtx.WorkDir, dest)
	}

	return ""
}

// computeFileChecksum computes the SHA256 checksum of a file.
func computeFileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// countDependencySteps counts total steps in all dependencies recursively.
func countDependencySteps(deps []DependencyPlan) int {
	count := 0
	for _, dep := range deps {
		count += len(dep.Steps)
		count += countDependencySteps(dep.Dependencies)
	}
	return count
}

// installDependencies installs all dependencies in depth-first order.
// Each dependency is installed in its own work directory and copied to the final location.
func (e *Executor) installDependencies(ctx context.Context, deps []DependencyPlan, platform Platform) error {
	for _, dep := range deps {
		// First install nested dependencies (depth-first)
		if err := e.installDependencies(ctx, dep.Dependencies, platform); err != nil {
			return err
		}

		// Then install this dependency
		if err := e.installSingleDependency(ctx, &dep, platform); err != nil {
			return fmt.Errorf("failed to install dependency %s: %w", dep.Tool, err)
		}
	}
	return nil
}

// installSingleDependency installs a single dependency to its final location.
func (e *Executor) installSingleDependency(ctx context.Context, dep *DependencyPlan, platform Platform) error {
	// Determine final destination based on recipe type (check before installation)
	var finalDir string
	if dep.RecipeType == "library" {
		// Libraries go to $TSUKU_HOME/libs/{name}-{version}/
		tsukuHome := filepath.Dir(e.toolsDir)
		libsDir := filepath.Join(tsukuHome, "libs")
		finalDir = filepath.Join(libsDir, fmt.Sprintf("%s-%s", dep.Tool, dep.Version))
	} else {
		// Tools go to $TSUKU_HOME/tools/{name}-{version}/
		finalDir = filepath.Join(e.toolsDir, fmt.Sprintf("%s-%s", dep.Tool, dep.Version))
	}

	// Skip if already installed (deduplication)
	if _, err := os.Stat(finalDir); err == nil {
		fmt.Printf("\nSkipping dependency: %s@%s (already installed)\n", dep.Tool, dep.Version)

		// Still add bin directory to exec paths for tools (needed for subsequent steps)
		if dep.RecipeType != "library" {
			binDir := filepath.Join(finalDir, "bin")
			if _, err := os.Stat(binDir); err == nil {
				e.execPaths = append(e.execPaths, binDir)
			}
		}

		return nil
	}

	fmt.Printf("\nInstalling dependency: %s@%s\n", dep.Tool, dep.Version)

	// Create temporary work directory for this dependency
	depWorkDir, err := os.MkdirTemp("", fmt.Sprintf("dep-%s-*", dep.Tool))
	if err != nil {
		return fmt.Errorf("failed to create work dir: %w", err)
	}
	defer os.RemoveAll(depWorkDir)

	// Create install directory within work directory
	depInstallDir := filepath.Join(depWorkDir, ".install")
	if err := os.MkdirAll(depInstallDir, 0755); err != nil {
		return fmt.Errorf("failed to create install dir: %w", err)
	}

	// Build recipe with verify section if available
	depRecipe := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: dep.Tool,
			Type: dep.RecipeType,
		},
	}
	if dep.Verify != nil {
		depRecipe.Verify = recipe.VerifySection{
			Command: dep.Verify.Command,
			Pattern: dep.Verify.Pattern,
		}
	}

	// Build resolved deps from the plan's nested dependencies
	// This uses actual resolved versions (e.g., "3.6.0") rather than
	// constraints from recipe parsing (e.g., "latest")
	depResolvedDeps := buildResolvedDepsFromPlan(dep.Dependencies)

	// Create execution context for this dependency
	execCtx := &actions.ExecutionContext{
		Context:          ctx,
		WorkDir:          depWorkDir,
		InstallDir:       depInstallDir,
		ToolInstallDir:   "",
		ToolsDir:         e.toolsDir,
		LibsDir:          e.libsDir,
		DownloadCacheDir: e.downloadCacheDir,
		KeyCacheDir:      e.keyCacheDir,
		Version:          dep.Version,
		VersionTag:       dep.Version,
		OS:               platform.OS,
		Arch:             platform.Arch,
		Recipe:           depRecipe,
		ExecPaths:        e.execPaths,
		Logger:           log.Default(),
		Dependencies:     depResolvedDeps,
	}

	// Validate all steps before execution (fail fast)
	for i, step := range dep.Steps {
		action := actions.Get(step.Action)
		if action == nil {
			return fmt.Errorf("dependency %s step %d: unknown action '%s'", dep.Tool, i+1, step.Action)
		}
		if result := actions.ValidateAction(step.Action, step.Params); result.HasErrors() {
			return fmt.Errorf("dependency %s step %d (%s): %s", dep.Tool, i+1, step.Action, result.ToError())
		}
	}

	// Execute each step for this dependency
	for i, step := range dep.Steps {
		if err := ctx.Err(); err != nil {
			return err
		}

		fmt.Printf("   Step %d/%d: %s\n", i+1, len(dep.Steps), step.Action)

		action := actions.Get(step.Action)
		if action == nil {
			return fmt.Errorf("unknown action: %s", step.Action)
		}

		// Execute (validation already done upfront)
		if err := action.Execute(execCtx, step.Params); err != nil {
			return fmt.Errorf("step %d (%s) failed: %w", i+1, step.Action, err)
		}
	}

	// Create final directory
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		return fmt.Errorf("failed to create final dir: %w", err)
	}

	// Copy contents from install directory to final location
	fmt.Printf("   Installing to: %s\n", finalDir)
	if err := copyDir(depInstallDir, finalDir); err != nil {
		return fmt.Errorf("failed to copy to final location: %w", err)
	}

	// For tools, add bin directory to exec paths
	if dep.RecipeType != "library" {
		binDir := filepath.Join(finalDir, "bin")
		if _, err := os.Stat(binDir); err == nil {
			e.execPaths = append(e.execPaths, binDir)
		}
	}

	fmt.Printf("   ✓ Installed %s@%s\n", dep.Tool, dep.Version)
	return nil
}

// copyDir recursively copies a directory from src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// Remove existing if present
			os.Remove(dstPath)
			return os.Symlink(target, dstPath)
		}

		// Copy regular file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}

// buildResolvedDepsFromPlan creates a ResolvedDeps from the plan's dependency list.
// This ensures we use actual installed versions (e.g., "3.6.0") rather than
// version constraints (e.g., "latest") from recipe parsing.
func buildResolvedDepsFromPlan(deps []DependencyPlan) actions.ResolvedDeps {
	result := actions.ResolvedDeps{
		InstallTime: make(map[string]string),
		Runtime:     make(map[string]string),
	}

	// Collect all dependencies recursively (flatten the tree)
	var collectDeps func(deps []DependencyPlan)
	collectDeps = func(deps []DependencyPlan) {
		for _, dep := range deps {
			// Use actual resolved version from plan
			result.InstallTime[dep.Tool] = dep.Version
			// Recursively collect nested dependencies
			if len(dep.Dependencies) > 0 {
				collectDeps(dep.Dependencies)
			}
		}
	}

	collectDeps(deps)
	return result
}

// validatePlatform checks if the plan's target platform matches the execution environment.
// Returns an error if the platform mismatch would cause the installation to fail.
func validatePlatform(plan *InstallationPlan) error {
	// OS and architecture must match exactly
	if plan.Platform.OS != runtime.GOOS {
		return fmt.Errorf("plan is for OS %q, but this system is %q", plan.Platform.OS, runtime.GOOS)
	}
	if plan.Platform.Arch != runtime.GOARCH {
		return fmt.Errorf("plan is for architecture %q, but this system is %q", plan.Platform.Arch, runtime.GOARCH)
	}

	// Note: LinuxFamily validation is handled by ValidatePlan in plan.go
	// which checks platform compatibility including family constraints.
	// This function focuses on basic OS/Arch validation for clarity.

	return nil
}

// validateResourceLimits checks that a plan's dependency tree doesn't exceed resource limits.
// This prevents pathological cases like circular dependencies or dependency bombs.
func validateResourceLimits(plan *InstallationPlan) error {
	const maxDepth = 5
	const maxTotalDeps = 100

	// Check depth
	depth := computeDepth(plan.Dependencies)
	if depth > maxDepth {
		return fmt.Errorf("dependency tree depth %d exceeds maximum %d", depth, maxDepth)
	}

	// Check total count
	totalDeps := countTotalDependencies(plan.Dependencies)
	if totalDeps > maxTotalDeps {
		return fmt.Errorf("total dependencies %d exceeds maximum %d", totalDeps, maxTotalDeps)
	}

	return nil
}

// computeDepth calculates the maximum depth of a dependency tree.
func computeDepth(deps []DependencyPlan) int {
	if len(deps) == 0 {
		return 0
	}
	maxChildDepth := 0
	for _, dep := range deps {
		childDepth := computeDepth(dep.Dependencies)
		if childDepth > maxChildDepth {
			maxChildDepth = childDepth
		}
	}
	return 1 + maxChildDepth
}

// countTotalDependencies counts all dependencies recursively.
func countTotalDependencies(deps []DependencyPlan) int {
	count := len(deps)
	for _, dep := range deps {
		count += countTotalDependencies(dep.Dependencies)
	}
	return count
}
