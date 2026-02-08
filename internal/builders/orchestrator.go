package builders

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/sandbox"
	"github.com/tsukumogami/tsuku/internal/telemetry"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// DefaultMaxRepairs is the default number of repair attempts before giving up.
const DefaultMaxRepairs = 2

// OrchestratorConfig contains configuration for the Orchestrator.
type OrchestratorConfig struct {
	// MaxRepairs is the maximum number of repair attempts after validation failure.
	// Default is DefaultMaxRepairs (2).
	MaxRepairs int

	// SkipSandbox disables sandbox testing entirely.
	// When true, recipes are returned immediately after generation.
	SkipSandbox bool

	// ToolsDir is the directory containing installed tools ($TSUKU_HOME/tools).
	// Required for plan generation if sandbox testing is enabled.
	ToolsDir string

	// LibsDir is the directory containing installed libraries ($TSUKU_HOME/libs).
	// Required for finding dependencies during build environment setup.
	LibsDir string

	// AppsDir is the directory for macOS .app bundles ($TSUKU_HOME/apps).
	// Used by app_bundle action for GUI application installation.
	AppsDir string

	// CurrentDir is the symlinks directory ($TSUKU_HOME/tools/current).
	// Used by app_bundle action for binary symlinks.
	CurrentDir string

	// DownloadCacheDir is the directory for caching downloads ($TSUKU_HOME/cache/downloads).
	// Required for plan generation if sandbox testing is enabled.
	DownloadCacheDir string

	// KeyCacheDir is the directory for caching PGP public keys ($TSUKU_HOME/cache/keys).
	// Used for PGP signature verification.
	KeyCacheDir string
}

// Orchestrator coordinates the build → sandbox → repair cycle.
// It owns the sandbox executor and controls validation externally,
// allowing builders to focus solely on recipe generation.
//
// The Orchestrator:
// 1. Creates a session from a SessionBuilder
// 2. Calls Generate() to produce an initial recipe
// 3. Runs sandbox validation on the recipe
// 4. If validation fails, calls Repair() with the failure details
// 5. Repeats validation/repair up to MaxRepairs times
// 6. Returns the validated recipe or an error
type Orchestrator struct {
	sandbox         *sandbox.Executor
	telemetryClient *telemetry.Client
	config          OrchestratorConfig
}

// OrchestratorOption configures an Orchestrator.
type OrchestratorOption func(*Orchestrator)

// WithSandboxExecutor sets the sandbox executor for validation.
func WithSandboxExecutor(e *sandbox.Executor) OrchestratorOption {
	return func(o *Orchestrator) {
		o.sandbox = e
	}
}

// WithOrchestratorTelemetry sets the telemetry client.
func WithOrchestratorTelemetry(c *telemetry.Client) OrchestratorOption {
	return func(o *Orchestrator) {
		o.telemetryClient = c
	}
}

// WithOrchestratorConfig sets the orchestrator configuration.
func WithOrchestratorConfig(cfg OrchestratorConfig) OrchestratorOption {
	return func(o *Orchestrator) {
		o.config = cfg
	}
}

// NewOrchestrator creates a new Orchestrator with the given options.
func NewOrchestrator(opts ...OrchestratorOption) *Orchestrator {
	o := &Orchestrator{
		telemetryClient: telemetry.NewClient(),
		config: OrchestratorConfig{
			MaxRepairs: DefaultMaxRepairs,
		},
	}

	for _, opt := range opts {
		opt(o)
	}

	return o
}

// OrchestratorResult contains the result of an orchestrated build.
type OrchestratorResult struct {
	// Recipe is the generated and validated recipe.
	Recipe *recipe.Recipe

	// BuildResult contains the raw result from the builder.
	BuildResult *BuildResult

	// RepairAttempts is the number of repair attempts made.
	RepairAttempts int

	// SandboxSkipped indicates sandbox testing was skipped.
	SandboxSkipped bool
}

// Create generates a recipe using the given builder with sandbox validation.
// It handles the full generate → validate → repair cycle.
func (o *Orchestrator) Create(
	ctx context.Context,
	builder SessionBuilder,
	req BuildRequest,
	opts *SessionOptions,
) (*OrchestratorResult, error) {
	// Create a new session
	session, err := builder.NewSession(ctx, req, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create build session: %w", err)
	}
	defer func() { _ = session.Close() }()

	// Generate initial recipe
	result, err := session.Generate(ctx)
	if err != nil {
		return nil, fmt.Errorf("generation failed: %w", err)
	}

	// If sandbox testing is skipped, return immediately
	if o.config.SkipSandbox || o.sandbox == nil {
		return &OrchestratorResult{
			Recipe:         result.Recipe,
			BuildResult:    result,
			SandboxSkipped: true,
		}, nil
	}

	// Validate and repair loop
	var repairAttempts int
	for attempt := 0; attempt <= o.config.MaxRepairs; attempt++ {
		// Run sandbox validation
		sandboxResult, err := o.validate(ctx, result.Recipe)
		if err != nil {
			return nil, fmt.Errorf("sandbox validation error: %w", err)
		}

		// Check if sandbox testing was skipped (no runtime available)
		if sandboxResult.Skipped {
			return &OrchestratorResult{
				Recipe:         result.Recipe,
				BuildResult:    result,
				RepairAttempts: repairAttempts,
				SandboxSkipped: true,
			}, nil
		}

		// Check if validation passed
		if sandboxResult.Passed {
			return &OrchestratorResult{
				Recipe:         result.Recipe,
				BuildResult:    result,
				RepairAttempts: repairAttempts,
			}, nil
		}

		// Validation failed - attempt repair if we have attempts left
		if attempt >= o.config.MaxRepairs {
			return nil, &ValidationFailedError{
				SandboxResult:  sandboxResult,
				RepairAttempts: repairAttempts,
			}
		}

		// Try deterministic verification self-repair first
		// This can fix common cases (tool doesn't support --version) without LLM
		if repaired, verifyMeta := o.attemptVerifySelfRepair(ctx, result.Recipe, sandboxResult); repaired != nil {
			// Self-repair produced a candidate - validate it
			repairedResult, err := o.validate(ctx, repaired)
			if err != nil {
				return nil, fmt.Errorf("sandbox validation error after self-repair: %w", err)
			}

			if repairedResult.Passed {
				// Self-repair succeeded!
				result.Recipe = repaired
				result.VerifyRepair = verifyMeta

				// Emit telemetry event for successful self-repair
				if o.telemetryClient != nil {
					event := telemetry.NewVerifySelfRepairEvent(
						repaired.Metadata.Name,
						verifyMeta.Method,
						true,
					)
					o.telemetryClient.SendVerifySelfRepair(event)
				}

				return &OrchestratorResult{
					Recipe:         repaired,
					BuildResult:    result,
					RepairAttempts: repairAttempts,
				}, nil
			}
			// Self-repair didn't help - fall through to LLM repair
		}

		// Repair the recipe using LLM
		repairAttempts++
		result, err = session.Repair(ctx, sandboxResult)
		if err != nil {
			// Check if this builder doesn't support repair (ecosystem builders)
			var repairErr *RepairNotSupportedError
			if errors.As(err, &repairErr) {
				// Can't repair deterministic recipes - return validation failure
				return nil, &ValidationFailedError{
					SandboxResult:  sandboxResult,
					RepairAttempts: repairAttempts,
				}
			}
			return nil, fmt.Errorf("repair attempt %d failed: %w", repairAttempts, err)
		}
	}

	return nil, errors.New("unexpected end of validation loop")
}

// validate runs sandbox validation on the given recipe.
func (o *Orchestrator) validate(ctx context.Context, r *recipe.Recipe) (*sandbox.SandboxResult, error) {
	// Generate installation plan from recipe
	plan, err := o.generatePlan(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("failed to generate installation plan: %w", err)
	}

	// Compute sandbox requirements from plan
	reqs := sandbox.ComputeSandboxRequirements(plan)

	// Detect current system target (platform + linux_family)
	target, err := platform.DetectTarget()
	if err != nil {
		return nil, fmt.Errorf("failed to detect target platform: %w", err)
	}

	// Run sandbox
	return o.sandbox.Sandbox(ctx, plan, target, reqs)
}

// generatePlan creates a fully resolved installation plan from a recipe.
// This uses the executor to properly decompose composite actions and set platform info.
func (o *Orchestrator) generatePlan(ctx context.Context, r *recipe.Recipe) (*executor.InstallationPlan, error) {
	exec, err := executor.New(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}
	defer exec.Cleanup()

	// Configure executor with paths
	exec.SetToolsDir(o.config.ToolsDir)
	exec.SetLibsDir(o.config.LibsDir)
	exec.SetAppsDir(o.config.AppsDir)
	exec.SetCurrentDir(o.config.CurrentDir)
	exec.SetDownloadCacheDir(o.config.DownloadCacheDir)
	exec.SetKeyCacheDir(o.config.KeyCacheDir)

	// Create downloader and cache for plan generation
	// These enable action decomposition and pre-downloading for sandbox
	var downloader actions.Downloader
	var downloadCache *actions.DownloadCache
	if o.config.DownloadCacheDir != "" {
		predownloader := validate.NewPreDownloader()
		downloader = validate.NewPreDownloaderAdapter(predownloader)
		downloadCache = actions.NewDownloadCache(o.config.DownloadCacheDir)
	}

	// Generate a fully resolved plan
	plan, err := exec.GeneratePlan(ctx, executor.PlanConfig{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		RecipeSource:  "create",
		Downloader:    downloader,
		DownloadCache: downloadCache,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate plan: %w", err)
	}

	return plan, nil
}

// attemptVerifySelfRepair attempts to deterministically repair a verification failure.
// This handles the common case where a tool doesn't support --version but does output
// help text when given an invalid flag.
//
// The repair strategy has two phases:
// 1. Output detection: Analyze the existing failure output for help-text patterns
// 2. Fallback commands: Try alternative commands (--help, -h) if output analysis is inconclusive
//
// Returns (nil, nil) if self-repair is not applicable or fails.
// Returns (repairedRecipe, metadata) if self-repair produces a candidate.
func (o *Orchestrator) attemptVerifySelfRepair(
	ctx context.Context,
	r *recipe.Recipe,
	failure *sandbox.SandboxResult,
) (*recipe.Recipe, *VerifyRepairMetadata) {
	// Skip if exit code 127 (command not found) - binary is missing, can't self-repair
	if validate.IsNotFoundExitCode(failure.ExitCode) {
		return nil, nil
	}

	originalCommand := r.Verify.Command
	toolName := r.Metadata.Name

	// Phase 1: Analyze the existing failure output for help-text patterns
	combined := failure.Stdout + failure.Stderr
	if len(combined) > 0 {
		analysis := validate.AnalyzeVerifyFailure(failure.Stdout, failure.Stderr, failure.ExitCode, toolName)

		if analysis.Repairable {
			// Output detection succeeded - create repaired recipe
			repairedVerify := recipe.VerifySection{
				Command:  originalCommand, // Keep the same command
				Mode:     analysis.SuggestedMode,
				Pattern:  analysis.SuggestedPattern,
				ExitCode: &analysis.ExitCode,
				Reason:   analysis.SuggestedReason,
			}

			repaired := r.WithVerify(repairedVerify)

			meta := &VerifyRepairMetadata{
				OriginalCommand:  originalCommand,
				RepairedCommand:  originalCommand, // Command stays the same, mode/pattern changed
				Method:           "output_detection",
				DetectedExitCode: analysis.ExitCode,
			}

			return repaired, meta
		}
	}

	// Phase 2: Try fallback commands when output analysis is inconclusive
	// Extract binary name from the original command (first word)
	binaryName := extractBinaryName(originalCommand)
	if binaryName == "" {
		return nil, nil
	}

	// Define fallback commands in priority order
	fallbacks := []struct {
		command string
		method  string
	}{
		{binaryName + " --help", "fallback_help"},
		{binaryName + " -h", "fallback_h"},
	}

	for _, fb := range fallbacks {
		repaired, meta := o.tryFallbackCommand(ctx, r, fb.command, fb.method, originalCommand)
		if repaired != nil {
			return repaired, meta
		}
	}

	return nil, nil
}

// extractBinaryName extracts the binary name from a verify command.
// It handles common formats like "tool --version", "tool -v", "./tool --version".
func extractBinaryName(command string) string {
	if command == "" {
		return ""
	}

	// Split on whitespace and take the first token
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}

	return fields[0]
}

// tryFallbackCommand attempts to validate a recipe with a fallback verify command.
// Returns (repairedRecipe, metadata) if the fallback succeeds, (nil, nil) otherwise.
func (o *Orchestrator) tryFallbackCommand(
	ctx context.Context,
	r *recipe.Recipe,
	fallbackCommand string,
	method string,
	originalCommand string,
) (*recipe.Recipe, *VerifyRepairMetadata) {
	// Create a verify section for the fallback command
	// Use "output" mode with "usage" pattern - help text typically contains "usage"
	fallbackVerify := recipe.VerifySection{
		Command: fallbackCommand,
		Mode:    "output",
		Pattern: "usage",
		Reason:  "verification repaired: using " + method + " fallback",
	}

	// Create candidate recipe with fallback verification
	candidate := r.WithVerify(fallbackVerify)

	// Run sandbox validation on the candidate
	result, err := o.validate(ctx, candidate)
	if err != nil {
		// Validation error - skip this fallback
		return nil, nil
	}

	if result.Skipped {
		// Sandbox was skipped - can't determine if fallback works
		return nil, nil
	}

	if !result.Passed {
		// Fallback command failed - try next one
		// But first, check if the output is analyzable
		analysis := validate.AnalyzeVerifyFailure(result.Stdout, result.Stderr, result.ExitCode, r.Metadata.Name)
		if analysis.Repairable {
			// The fallback produced analyzable help output - create repaired recipe
			repairedVerify := recipe.VerifySection{
				Command:  fallbackCommand,
				Mode:     analysis.SuggestedMode,
				Pattern:  analysis.SuggestedPattern,
				ExitCode: &analysis.ExitCode,
				Reason:   "verification repaired: " + method + " with output detection",
			}

			repaired := r.WithVerify(repairedVerify)

			meta := &VerifyRepairMetadata{
				OriginalCommand:  originalCommand,
				RepairedCommand:  fallbackCommand,
				Method:           method,
				DetectedExitCode: analysis.ExitCode,
			}

			return repaired, meta
		}
		return nil, nil
	}

	// Fallback succeeded!
	meta := &VerifyRepairMetadata{
		OriginalCommand:  originalCommand,
		RepairedCommand:  fallbackCommand,
		Method:           method,
		DetectedExitCode: result.ExitCode,
	}

	// Update the verify section with the actual exit code from the successful run
	exitCode := result.ExitCode
	fallbackVerify.ExitCode = &exitCode
	repaired := r.WithVerify(fallbackVerify)

	return repaired, meta
}

// ValidationFailedError indicates that validation failed after all repair attempts.
type ValidationFailedError struct {
	SandboxResult  *sandbox.SandboxResult
	RepairAttempts int
}

func (e *ValidationFailedError) Error() string {
	return fmt.Sprintf("recipe validation failed after %d repair attempts (exit code %d)",
		e.RepairAttempts, e.SandboxResult.ExitCode)
}
