package builders

import (
	"context"
	"errors"
	"fmt"
	"runtime"

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

		// Repair the recipe
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

// ValidationFailedError indicates that validation failed after all repair attempts.
type ValidationFailedError struct {
	SandboxResult  *sandbox.SandboxResult
	RepairAttempts int
}

func (e *ValidationFailedError) Error() string {
	return fmt.Sprintf("recipe validation failed after %d repair attempts (exit code %d)",
		e.RepairAttempts, e.SandboxResult.ExitCode)
}
