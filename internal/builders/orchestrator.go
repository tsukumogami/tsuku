package builders

import (
	"context"
	"errors"
	"fmt"

	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/sandbox"
	"github.com/tsukumogami/tsuku/internal/telemetry"
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
			return nil, fmt.Errorf("repair attempt %d failed: %w", repairAttempts, err)
		}
	}

	return nil, errors.New("unexpected end of validation loop")
}

// validate runs sandbox validation on the given recipe.
func (o *Orchestrator) validate(ctx context.Context, r *recipe.Recipe) (*sandbox.SandboxResult, error) {
	// Generate installation plan from recipe
	plan, err := o.generatePlan(r)
	if err != nil {
		return nil, fmt.Errorf("failed to generate installation plan: %w", err)
	}

	// Compute sandbox requirements from plan
	reqs := sandbox.ComputeSandboxRequirements(plan)

	// Run sandbox
	return o.sandbox.Sandbox(ctx, plan, reqs)
}

// generatePlan creates an installation plan from a recipe for sandbox testing.
func (o *Orchestrator) generatePlan(r *recipe.Recipe) (*executor.InstallationPlan, error) {
	// For sandbox testing, we generate a minimal plan that exercises the recipe
	// The actual version will be resolved at install time
	plan := &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          r.Metadata.Name,
		Version:       "latest", // Will be resolved by sandbox
		Steps:         make([]executor.ResolvedStep, 0),
	}

	// Convert recipe steps to resolved steps
	for _, step := range r.Steps {
		resolved := executor.ResolvedStep{
			Action: step.Action,
			Params: step.Params,
		}
		plan.Steps = append(plan.Steps, resolved)
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
