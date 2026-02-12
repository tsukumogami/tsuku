package discover

import (
	"context"
	"fmt"
)

// Confidence indicates how the tool source was determined.
type Confidence string

const (
	ConfidenceRegistry  Confidence = "registry"
	ConfidenceEcosystem Confidence = "ecosystem"
	ConfidenceLLM       Confidence = "llm"
)

// Metadata holds optional popularity and provenance signals for disambiguation UX.
type Metadata struct {
	Downloads           int    // Monthly downloads (0 if unavailable)
	VersionCount        int    // Number of published versions (0 if unavailable)
	HasRepository       bool   // Whether the package has a linked source repository
	AgeDays             int    // Days since first publish (0 if unavailable)
	Stars               int    // GitHub stars (0 if unavailable)
	Description         string // Short description for display
	IsFork              bool   // True if repository is a fork
	ParentRepo          string // Parent repository full_name (e.g., "owner/repo") if fork
	ParentStars         int    // Parent repository star count (0 if unavailable)
	CreatedAt           string // Creation date in YYYY-MM-DD format (empty if unavailable)
	LastCommitDays      int    // Days since last commit (0 if unavailable)
	OwnerName           string // Repository owner login name
	OwnerType           string // Owner type: "User" or "Organization"
	VerificationSkipped bool   // True if GitHub API verification was skipped (e.g., rate limited)
	VerificationWarning string // Warning message when verification was skipped
}

// DiscoveryResult describes where a tool can be sourced from.
type DiscoveryResult struct {
	// Builder is the builder name (maps to builders.Registry).
	Builder string

	// Source is the builder-specific source argument (e.g., "owner/repo").
	Source string

	// Confidence indicates which resolver stage produced this result.
	Confidence Confidence

	// ConfidenceScore is the LLM-provided confidence score (0-100) for ranking.
	// Only set for LLM discovery results. Used to rank multiple candidates.
	ConfidenceScore int

	// Reason is a human-readable explanation for display.
	Reason string

	// Metadata holds optional popularity signals for disambiguation and confirmation UX.
	Metadata Metadata

	// LLMMetrics holds cost and usage metrics from LLM discovery.
	// Only set for results from the LLM discovery stage.
	LLMMetrics *LLMMetrics
}

// LLMMetrics contains cost and usage metrics from an LLM discovery session.
type LLMMetrics struct {
	InputTokens  int     // Total input tokens used
	OutputTokens int     // Total output tokens used
	Cost         float64 // Estimated cost in USD
	Provider     string  // LLM provider name (e.g., "claude", "gemini")
	Turns        int     // Number of LLM conversation turns
}

// Resolver resolves a tool name to a discovery result.
// Implementations return (nil, nil) for a soft miss (tool not found at this stage).
// Non-nil errors are either soft (logged, chain continues) or hard (chain stops).
type Resolver interface {
	Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error)
}

// Suggester is implemented by errors that provide actionable guidance.
type Suggester interface {
	Suggestion() string
}

// NotFoundError indicates no resolver stage could find the tool.
type NotFoundError struct {
	Tool string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("could not find '%s'", e.Tool)
}

func (e *NotFoundError) Suggestion() string {
	return fmt.Sprintf("Try tsuku install %s --from github:owner/repo if you know the source.", e.Tool)
}

// ConfigurationError indicates discovery couldn't complete due to missing configuration.
type ConfigurationError struct {
	Tool   string
	Reason string // "no_api_key" or "deterministic_only"
}

func (e *ConfigurationError) Error() string {
	switch e.Reason {
	case "no_api_key":
		return fmt.Sprintf("no match for '%s' in registry or ecosystems", e.Tool)
	case "deterministic_only":
		return fmt.Sprintf("no deterministic source found for '%s'", e.Tool)
	default:
		return fmt.Sprintf("configuration error for '%s': %s", e.Tool, e.Reason)
	}
}

func (e *ConfigurationError) Suggestion() string {
	switch e.Reason {
	case "no_api_key":
		return "Set ANTHROPIC_API_KEY to enable web search discovery, or use --from to specify the source directly."
	case "deterministic_only":
		return "Remove --deterministic-only to enable LLM discovery, or use --from to specify the source."
	default:
		return ""
	}
}

// BuilderRequiresLLMError indicates the resolved builder requires LLM but deterministic mode is set.
type BuilderRequiresLLMError struct {
	Tool    string
	Builder string
	Source  string
}

func (e *BuilderRequiresLLMError) Error() string {
	return fmt.Sprintf("'%s' resolved to %s releases (%s), which requires LLM for recipe generation",
		e.Tool, e.Builder, e.Source)
}

func (e *BuilderRequiresLLMError) Suggestion() string {
	return "Remove --deterministic-only or wait for a recipe to be contributed."
}

// AmbiguousMatchError indicates multiple ecosystem matches with no clear winner.
// Returned when popularity data is missing or matches are within 10x of each other.
// Downstream handlers format this error with --from suggestions.
type AmbiguousMatchError struct {
	Tool    string           // The requested tool name
	Matches []DiscoveryMatch // Ranked matches for display
}

// DiscoveryMatch represents a single ecosystem match for disambiguation display.
type DiscoveryMatch struct {
	Builder       string // e.g., "crates.io", "npm"
	Source        string // e.g., "sharkdp/bat", "bat-cli"
	Downloads     int    // 0 if unavailable
	VersionCount  int    // 0 if unavailable
	HasRepository bool
}

// ProbeMatch represents a single ecosystem match for interactive disambiguation.
// Used as callback input to ConfirmDisambiguationFunc.
type ProbeMatch struct {
	Builder       string // e.g., "crates.io", "npm"
	Source        string // e.g., "sharkdp/bat", "bat-cli"
	Downloads     int    // Monthly downloads (0 if unavailable)
	VersionCount  int    // Number of published versions
	HasRepository bool   // Whether package has linked source repo
}

// ConfirmDisambiguationFunc prompts the user to select from multiple matches.
// Returns the selected index (0-based) or an error if cancelled.
type ConfirmDisambiguationFunc func(matches []ProbeMatch) (int, error)

func (e *AmbiguousMatchError) Error() string {
	return fmt.Sprintf("multiple sources found for '%s': use --from to specify", e.Tool)
}

// isFatalError returns true for errors that should stop the resolver chain.
// Context cancellation and budget exhaustion are fatal; everything else is soft.
func isFatalError(err error) bool {
	// For now, only context errors are fatal. Budget/rate-limit errors will
	// be added when the LLM discovery stage is implemented.
	return false
}
