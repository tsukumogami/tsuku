package builders

import (
	"context"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
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

// InitOptions contains options for builder initialization.
type InitOptions struct {
	// SkipValidation disables container validation for LLM-generated recipes.
	SkipValidation bool

	// ProgressReporter receives progress updates during build operations.
	// If nil, no progress is reported.
	ProgressReporter ProgressReporter

	// LLMConfig provides access to LLM-related user settings.
	// Required for LLM builders; ignored by ecosystem builders.
	LLMConfig LLMConfig

	// LLMStateTracker provides rate limit checking and cost tracking.
	// Required for LLM builders; ignored by ecosystem builders.
	LLMStateTracker LLMStateTracker

	// ForceInit bypasses rate limit and budget checks.
	// Used when the user confirms they want to proceed despite limits.
	ForceInit bool
}

// CheckLLMPrerequisites validates LLM configuration and rate limits.
// Returns nil if generation is allowed, or an error (possibly ConfirmableError) if not.
// This is a helper for LLM builders to call in their Initialize() method.
func CheckLLMPrerequisites(opts *InitOptions) error {
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

// Builder generates recipes for packages from a specific ecosystem.
// Builders are invoked via "tsuku create" to generate recipes that are
// written to the user's local recipes directory ($TSUKU_HOME/recipes/).
type Builder interface {
	// Name returns the builder identifier (e.g., "crates_io", "rubygems", "github")
	Name() string

	// Initialize performs builder-specific setup.
	// For LLM builders, this creates the LLM factory, validates the API key,
	// and sets up validation executor and progress reporter.
	// For ecosystem builders, this is a no-op.
	// Returns error if initialization fails (e.g., missing API key).
	Initialize(ctx context.Context, opts *InitOptions) error

	// RequiresLLM returns true if this builder uses LLM for recipe generation.
	// Used by CLI to apply LLM-specific behaviors like budget checks,
	// preview prompts, and cost tracking.
	RequiresLLM() bool

	// CanBuild checks if this builder can handle the package.
	// It typically queries the ecosystem's API to verify the package exists.
	CanBuild(ctx context.Context, packageName string) (bool, error)

	// Build generates a recipe for the package.
	// If req.Version is empty, the recipe will use a version provider for dynamic resolution.
	Build(ctx context.Context, req BuildRequest) (*BuildResult, error)
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

	// ValidationSkipped indicates validation was skipped (e.g., no container runtime).
	ValidationSkipped bool

	// Cost is the estimated cost in USD for LLM-based generation.
	// Only populated by LLM-based builders (e.g., GitHubReleaseBuilder).
	Cost float64
}
