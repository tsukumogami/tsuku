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
	Downloads   int    // Monthly downloads (0 if unavailable)
	AgeDays     int    // Days since first publish (0 if unavailable)
	Stars       int    // GitHub stars (0 if unavailable)
	Description string // Short description for display
}

// DiscoveryResult describes where a tool can be sourced from.
type DiscoveryResult struct {
	// Builder is the builder name (maps to builders.Registry).
	Builder string

	// Source is the builder-specific source argument (e.g., "owner/repo").
	Source string

	// Confidence indicates which resolver stage produced this result.
	Confidence Confidence

	// Reason is a human-readable explanation for display.
	Reason string

	// Metadata holds optional popularity signals for disambiguation and confirmation UX.
	Metadata Metadata
}

// Resolver resolves a tool name to a discovery result.
// Implementations return (nil, nil) for a soft miss (tool not found at this stage).
// Non-nil errors are either soft (logged, chain continues) or hard (chain stops).
type Resolver interface {
	Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error)
}

// NotFoundError indicates no resolver stage could find the tool.
type NotFoundError struct {
	Tool string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("could not find '%s'. Try tsuku install %s --from github:owner/repo if you know the source", e.Tool, e.Tool)
}

// isFatalError returns true for errors that should stop the resolver chain.
// Context cancellation and budget exhaustion are fatal; everything else is soft.
func isFatalError(err error) bool {
	// For now, only context errors are fatal. Budget/rate-limit errors will
	// be added when the LLM discovery stage is implemented.
	return false
}
