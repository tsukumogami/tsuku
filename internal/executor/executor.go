package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tsuku-dev/tsuku/internal/actions"
	"github.com/tsuku-dev/tsuku/internal/recipe"
	"github.com/tsuku-dev/tsuku/internal/version"
)

// Executor executes action-based recipes
type Executor struct {
	workDir    string
	installDir string
	recipe     *recipe.Recipe
	ctx        *actions.ExecutionContext
	version    string   // Resolved version
	reqVersion string   // Requested version (optional)
	execPaths  []string // Additional bin paths for execution (e.g., nodejs for npm tools)
	toolsDir   string   // Tools directory (~/.tsuku/tools/) for finding other installed tools
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

// Execute runs the recipe
func (e *Executor) Execute(ctx context.Context) error {
	fmt.Printf("📦 Executing action-based recipe: %s\n", e.recipe.Metadata.Name)
	fmt.Printf("   Work directory: %s\n", e.workDir)

	// Create version resolver (reused across all steps)
	resolver := version.New()

	// Resolve version from recipe steps
	versionInfo, err := e.resolveVersionWith(ctx, resolver)
	if err != nil {
		fmt.Printf("⚠️  Version resolution failed: %v\n", err)
		fmt.Printf("   Proceeding with default version 'dev' for local/test recipes\n")
		versionInfo = &version.VersionInfo{
			Version: "dev",
			Tag:     "dev",
		}
	}

	fmt.Printf("   Latest version: %s\n", versionInfo.Version)

	// Store version for later use
	e.version = versionInfo.Version

	// Create execution context
	e.ctx = &actions.ExecutionContext{
		Context:        ctx, // Pass context for cancellation and timeouts
		WorkDir:        e.workDir,
		InstallDir:     e.installDir,
		ToolInstallDir: "", // Set by composite actions when install_mode="directory" is used
		ToolsDir:       e.toolsDir,
		Version:        versionInfo.Version,
		VersionTag:     versionInfo.Tag,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Recipe:         e.recipe,
		ExecPaths:      e.execPaths, // Include execution dependency paths
		Resolver:       resolver,    // Pass resolver for asset resolution
	}

	fmt.Println()

	// Execute each step
	for i, step := range e.recipe.Steps {
		// Check conditional execution
		if !e.shouldExecute(step.When) {
			fmt.Printf("Step %d/%d: %s (skipped - condition not met)\n", i+1, len(e.recipe.Steps), step.Action)
			continue
		}

		fmt.Printf("Step %d/%d: %s\n", i+1, len(e.recipe.Steps), step.Action)

		// Get action
		action := actions.Get(step.Action)
		if action == nil {
			return fmt.Errorf("unknown action: %s", step.Action)
		}

		// Execute action
		if err := action.Execute(e.ctx, step.Params); err != nil {
			return fmt.Errorf("step %d (%s) failed: %w", i+1, step.Action, err)
		}

		fmt.Println()
	}

	// Verify installation
	fmt.Println("🔍 Verifying installation")
	if err := e.verify(); err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	return nil
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

// verify runs the verification command
func (e *Executor) verify() error {
	// Expand variables in command
	vars := map[string]string{
		"version":     e.ctx.Version,
		"install_dir": e.installDir,
		"binary":      filepath.Join(e.installDir, "bin", e.recipe.Metadata.Name),
	}

	command := expandVars(e.recipe.Verify.Command, vars)
	pattern := expandVars(e.recipe.Verify.Pattern, vars)

	fmt.Printf("   Running: %s\n", command)

	// Set up PATH to include install directory and execution dependencies
	binDir := filepath.Join(e.installDir, "bin")
	pathDirs := []string{binDir}

	// Add execution paths (e.g., nodejs bin for npm tools)
	pathDirs = append(pathDirs, e.ctx.ExecPaths...)

	// Build PATH with all directories
	pathValue := strings.Join(pathDirs, ":") + ":" + os.Getenv("PATH")

	// Build environment with updated PATH (filter out existing PATH to avoid duplicates)
	env := []string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "PATH=") {
			env = append(env, e)
		}
	}
	env = append(env, "PATH="+pathValue)

	// Run command
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}

	outputStr := strings.TrimSpace(string(output))
	fmt.Printf("   Output: %s\n", outputStr)

	// Check pattern
	if pattern != "" {
		if !strings.Contains(outputStr, pattern) {
			return fmt.Errorf("output does not match pattern\n  Expected: %s\n  Got: %s", pattern, outputStr)
		}
		fmt.Printf("   Pattern matched: %s ✓\n", pattern)
	}

	// Run additional verifications
	for i, additional := range e.recipe.Verify.Additional {
		additionalCmd := expandVars(additional.Command, vars)
		additionalPattern := expandVars(additional.Pattern, vars)

		fmt.Printf("   Additional verification %d: %s\n", i+1, additionalCmd)

		cmd := exec.Command("sh", "-c", additionalCmd)
		cmd.Env = env
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("additional verification %d failed: %w\nOutput: %s", i+1, err, string(output))
		}

		outputStr := strings.TrimSpace(string(output))
		if additionalPattern != "" && !strings.Contains(outputStr, additionalPattern) {
			return fmt.Errorf("additional verification %d: output does not match pattern\n  Expected: %s\n  Got: %s",
				i+1, additionalPattern, outputStr)
		}
		fmt.Printf("   ✓ Verification %d passed\n", i+1)
	}

	return nil
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
