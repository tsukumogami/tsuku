package builders

import (
	"context"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/sandbox"
)

// LLMStateTracker provides rate limit checking and cost tracking for LLM operations.
// This interface decouples builders from the concrete state management implementation.
type LLMStateTracker interface {
	// CanGenerate checks if generation is allowed given rate limits and budget.
	// Returns (allowed, reason) where reason explains why generation was denied.
	CanGenerate(hourlyLimit int, dailyBudget float64) (bool, string)

	// RecordGeneration records the cost of an LLM generation.
	RecordGeneration(cost float64) error

	// DailySpent returns the total amount spent today.
	DailySpent() float64

	// RecentGenerationCount returns the number of generations in the recent period.
	RecentGenerationCount() int
}

// LLMConfig provides access to LLM-related user configuration.
// This interface decouples builders from the concrete config implementation.
type LLMConfig interface {
	// LLMEnabled returns whether LLM features are enabled.
	LLMEnabled() bool

	// LLMDailyBudget returns the daily budget limit in USD.
	LLMDailyBudget() float64

	// LLMHourlyRateLimit returns the maximum generations per hour.
	LLMHourlyRateLimit() int
}

// ConfirmableError is an error that can be bypassed with user confirmation.
// The CLI checks for this interface to prompt the user before proceeding.
type ConfirmableError interface {
	error
	// ConfirmationPrompt returns the message to display when asking for confirmation.
	ConfirmationPrompt() string
}

// LLMDisabledError indicates LLM features are disabled in user config.
type LLMDisabledError struct{}

func (e *LLMDisabledError) Error() string {
	return "LLM features are disabled. To enable: tsuku config set llm.enabled true"
}

// CheckLLMPrerequisites validates LLM configuration and rate limits.
// Returns nil if generation is allowed, or an error (possibly ConfirmableError) if not.
// This is a helper for LLM builders to call in their NewSession() method.
func CheckLLMPrerequisites(opts *SessionOptions) error {
	if opts == nil || opts.ForceInit {
		return nil
	}

	// Check if LLM is enabled
	if opts.LLMConfig != nil && !opts.LLMConfig.LLMEnabled() {
		return &LLMDisabledError{}
	}

	// Check rate limits and budget
	if opts.LLMConfig != nil && opts.LLMStateTracker != nil {
		hourlyLimit := opts.LLMConfig.LLMHourlyRateLimit()
		dailyBudget := opts.LLMConfig.LLMDailyBudget()

		allowed, reason := opts.LLMStateTracker.CanGenerate(hourlyLimit, dailyBudget)
		if !allowed {
			// Determine if this is a budget or rate limit error
			if strings.Contains(reason, "budget") {
				return &BudgetError{
					Budget:    dailyBudget,
					Spent:     opts.LLMStateTracker.DailySpent(),
					ConfigKey: "llm.daily_budget",
				}
			}
			return &RateLimitError{
				Limit:     hourlyLimit,
				Count:     opts.LLMStateTracker.RecentGenerationCount(),
				ConfigKey: "llm.hourly_rate_limit",
			}
		}
	}

	return nil
}

// RecordLLMCost records the cost of an LLM generation if a state tracker is available.
// Returns any error from recording (callers typically log warnings rather than failing).
func RecordLLMCost(tracker LLMStateTracker, cost float64) error {
	if tracker == nil || cost <= 0 {
		return nil
	}
	return tracker.RecordGeneration(cost)
}

// BuildRequest contains builder-specific parameters for recipe generation.
type BuildRequest struct {
	// Package is the tool name the user wants (e.g., "gh", "ripgrep")
	Package string

	// Version is the optional specific version to install (empty = latest)
	Version string

	// SourceArg is a builder-specific argument passed from the --from flag.
	// The builder is responsible for parsing any builder-specific syntax.
	// Examples:
	// - github builder: "cli/cli" (owner/repo)
	// - homebrew builder: "jq" or "jq:source" (formula with optional :source suffix)
	// - crates.io builder: unused (Package is the crate name)
	SourceArg string
}

// BuildResult contains the generated recipe and metadata about the build process.
type BuildResult struct {
	// Recipe is the generated recipe struct
	Recipe *recipe.Recipe

	// Warnings contains human-readable messages about generation uncertainty.
	// For example: "Could not determine executables from Cargo.toml; using crate name"
	Warnings []string

	// Source identifies where the metadata came from (e.g., "crates.io:ripgrep")
	Source string

	// RepairAttempts is the number of repair attempts made after validation failures.
	// Only populated by builders that support validation (e.g., GitHubReleaseBuilder).
	RepairAttempts int

	// Provider is the LLM provider that generated the recipe (e.g., "claude", "gemini").
	// Only populated by LLM-based builders.
	Provider string

	// SandboxSkipped indicates sandbox testing was skipped (e.g., no container runtime).
	SandboxSkipped bool

	// Cost is the estimated cost in USD for LLM-based generation.
	// Only populated by LLM-based builders (e.g., GitHubReleaseBuilder).
	Cost float64
}

// BuildSession represents an active build session that maintains state across
// multiple generation and repair attempts. This is particularly important for
// LLM builders that maintain conversation history for effective repairs.
//
// Sessions are created by Builder.NewSession() and should be closed when done.
// The Orchestrator uses sessions to control the sandbox validation and repair loop
// externally, rather than having validation embedded in each builder.
type BuildSession interface {
	// Generate produces an initial recipe from the build request.
	// This is the first call after creating a session.
	Generate(ctx context.Context) (*BuildResult, error)

	// Repair attempts to fix the recipe given sandbox failure feedback.
	// The session maintains internal state (e.g., LLM conversation history)
	// so repairs can build on previous context rather than starting fresh.
	// Can be called multiple times for iterative repairs.
	Repair(ctx context.Context, failure *sandbox.SandboxResult) (*BuildResult, error)

	// Close releases resources associated with the session.
	// Should be called when the session is no longer needed.
	Close() error
}

// SessionBuilder is the interface for builders that support session-based generation.
// This extends the basic Builder interface with session creation capabilities.
//
// Session-based builders allow the Orchestrator to control the sandbox validation
// and repair loop externally. This enables:
// - Centralized validation policy (retry counts, different builders, etc.)
// - Cross-builder repair strategies
// - Consistent telemetry and progress reporting
//
// For simple ecosystem builders that don't need stateful repairs, use
// SimpleSessionBuilder which wraps a basic Build() function.
type SessionBuilder interface {
	// Name returns the builder identifier (e.g., "github", "homebrew", "crates_io")
	Name() string

	// RequiresLLM returns true if this builder uses LLM for recipe generation.
	RequiresLLM() bool

	// CanBuild checks if this builder can handle the package/source combination.
	CanBuild(ctx context.Context, req BuildRequest) (bool, error)

	// NewSession creates a new build session for the given request.
	// The session maintains state for iterative generation and repair.
	NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error)
}

// SessionOptions contains options for creating a build session.
type SessionOptions struct {
	// ProgressReporter receives progress updates during build operations.
	ProgressReporter ProgressReporter

	// LLMConfig provides access to LLM-related user settings.
	LLMConfig LLMConfig

	// LLMStateTracker provides rate limit checking and cost tracking.
	LLMStateTracker LLMStateTracker

	// ForceInit bypasses rate limit and budget checks.
	ForceInit bool

	// DeterministicOnly when true, skip LLM fallback and return
	// DeterministicFailedError on failure.
	DeterministicOnly bool
}

// BuildFunc is a function that generates a recipe for a package.
// Used by DeterministicSession to wrap ecosystem builders.
type BuildFunc func(ctx context.Context, req BuildRequest) (*BuildResult, error)

// DeterministicSession wraps a simple Build() function as a BuildSession.
// This is for ecosystem builders (Cargo, Go, PyPI, etc.) that generate
// recipes deterministically from metadata without needing stateful repairs.
//
// Since these builders can't use failure feedback to improve their output,
// Repair() simply returns an error indicating repair is not supported.
type DeterministicSession struct {
	buildFunc  BuildFunc
	req        BuildRequest
	lastResult *BuildResult
}

// NewDeterministicSession creates a session that wraps a build function.
func NewDeterministicSession(buildFunc BuildFunc, req BuildRequest) *DeterministicSession {
	return &DeterministicSession{
		buildFunc: buildFunc,
		req:       req,
	}
}

// Generate calls the underlying build function.
func (s *DeterministicSession) Generate(ctx context.Context) (*BuildResult, error) {
	result, err := s.buildFunc(ctx, s.req)
	if err != nil {
		return nil, err
	}
	s.lastResult = result
	return result, nil
}

// Repair returns an error because ecosystem builders can't use failure feedback.
// Their recipes are deterministic - running again would produce the same result.
func (s *DeterministicSession) Repair(ctx context.Context, failure *sandbox.SandboxResult) (*BuildResult, error) {
	return nil, &RepairNotSupportedError{BuilderType: "ecosystem"}
}

// Close is a no-op for deterministic sessions.
func (s *DeterministicSession) Close() error {
	return nil
}

// RepairNotSupportedError indicates a builder cannot repair its recipes.
type RepairNotSupportedError struct {
	BuilderType string
}

func (e *RepairNotSupportedError) Error() string {
	return e.BuilderType + " builders do not support repair (recipes are deterministic)"
}

// Is enables errors.Is() matching for RepairNotSupportedError.
func (e *RepairNotSupportedError) Is(target error) bool {
	_, ok := target.(*RepairNotSupportedError)
	return ok
}
