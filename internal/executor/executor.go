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
	recipe           *recipe.Recipe
	ctx              *actions.ExecutionContext
	version          string   // Resolved version
	reqVersion       string   // Requested version (optional)
	execPaths        []string // Additional bin paths for execution (e.g., nodejs for npm tools)
	toolsDir         string   // Tools directory (~/.tsuku/tools/) for finding other installed tools
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

// shouldExecute checks if a step should be executed based on 'when' conditions
func (e *Executor) shouldExecute(when map[string]string) bool {
	if len(when) == 0 {
		return true
	}

	// Check OS condition
	if osCondition, ok := when["os"]; ok {
		if osCondition != runtime.GOOS {
			return false
		}
	}

	// Check arch condition
	if archCondition, ok := when["arch"]; ok {
		if archCondition != runtime.GOARCH {
			return false
		}
	}

	// Check package_manager condition (stub - always true for validation)
	if _, ok := when["package_manager"]; ok {
		// In real implementation, would detect system package manager
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
	vars := actions.GetStandardVars(versionInfo.Version, "", "")

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

	fmt.Printf("Executing plan: %s@%s\n", plan.Tool, plan.Version)
	fmt.Printf("   Work directory: %s\n", e.workDir)

	// Flatten dependency steps into a single list (depth-first order)
	// This ensures dependencies are installed before the tools that need them
	allSteps := flattenPlanSteps(plan)
	fmt.Printf("   Total steps: %d (including %d from dependencies)\n",
		len(allSteps), len(allSteps)-len(plan.Steps))

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

	// Create execution context from plan
	execCtx := &actions.ExecutionContext{
		Context:          ctx,
		WorkDir:          e.workDir,
		InstallDir:       e.installDir,
		ToolInstallDir:   "",
		ToolsDir:         e.toolsDir,
		DownloadCacheDir: e.downloadCacheDir,
		Version:          plan.Version,
		VersionTag:       plan.Version, // Plan doesn't track tag separately
		OS:               plan.Platform.OS,
		Arch:             plan.Platform.Arch,
		Recipe:           recipeForContext,
		ExecPaths:        e.execPaths,
		Logger:           log.Default(),
	}
	e.ctx = execCtx

	fmt.Println()

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
			// Execute other steps normally
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

// flattenPlanSteps collects all steps from a plan and its dependencies into a single list.
// Steps are collected in depth-first order: dependency steps come before the steps
// that depend on them. This ensures proper installation order.
func flattenPlanSteps(plan *InstallationPlan) []ResolvedStep {
	var allSteps []ResolvedStep

	// First, collect steps from all dependencies (depth-first)
	for _, dep := range plan.Dependencies {
		depSteps := flattenDependencySteps(&dep)
		allSteps = append(allSteps, depSteps...)
	}

	// Then add the main tool's steps
	allSteps = append(allSteps, plan.Steps...)

	return allSteps
}

// flattenDependencySteps recursively collects steps from a dependency and its nested dependencies.
func flattenDependencySteps(dep *DependencyPlan) []ResolvedStep {
	var allSteps []ResolvedStep

	// First, collect steps from nested dependencies (depth-first)
	for _, nestedDep := range dep.Dependencies {
		nestedSteps := flattenDependencySteps(&nestedDep)
		allSteps = append(allSteps, nestedSteps...)
	}

	// Then add this dependency's steps
	allSteps = append(allSteps, dep.Steps...)

	return allSteps
}

// executeDownloadWithVerification downloads a file and verifies its checksum against the plan.
func (e *Executor) executeDownloadWithVerification(
	ctx context.Context,
	execCtx *actions.ExecutionContext,
	step ResolvedStep,
	plan *InstallationPlan,
) error {
	// Execute the download action
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
