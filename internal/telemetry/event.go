// Package telemetry provides anonymous usage telemetry for tsuku.
package telemetry

import (
	"runtime"

	"github.com/tsukumogami/tsuku/internal/buildinfo"
)

// Event represents a telemetry event sent to the backend.
type Event struct {
	Action            string `json:"action"`             // "install", "update", or "remove"
	Recipe            string `json:"recipe"`             // Recipe name (e.g., "nodejs")
	VersionConstraint string `json:"version_constraint"` // User's original constraint (e.g., "@LTS", ">=1.0", or empty)
	VersionResolved   string `json:"version_resolved"`   // Actual version installed/updated to
	VersionPrevious   string `json:"version_previous"`   // Previous version (for update/remove)
	OS                string `json:"os"`                 // Operating system ("linux", "darwin")
	Arch              string `json:"arch"`               // CPU architecture ("amd64", "arm64")
	TsukuVersion      string `json:"tsuku_version"`      // Version of tsuku CLI
	IsDependency      bool   `json:"is_dependency"`      // True if installed as a transitive dependency
	SchemaVersion     string `json:"schema_version"`     // Event schema version
}

const schemaVersion = "1"

// newBaseEvent creates an event with common fields pre-filled.
func newBaseEvent() Event {
	return Event{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		TsukuVersion:  buildinfo.Version(),
		SchemaVersion: schemaVersion,
	}
}

// NewInstallEvent creates a telemetry event for an install action.
func NewInstallEvent(recipe, versionConstraint, versionResolved string, isDependency bool) Event {
	e := newBaseEvent()
	e.Action = "install"
	e.Recipe = recipe
	e.VersionConstraint = versionConstraint
	e.VersionResolved = versionResolved
	e.IsDependency = isDependency
	return e
}

// NewUpdateEvent creates a telemetry event for an update action.
func NewUpdateEvent(recipe, versionPrevious, versionResolved string) Event {
	e := newBaseEvent()
	e.Action = "update"
	e.Recipe = recipe
	e.VersionPrevious = versionPrevious
	e.VersionResolved = versionResolved
	return e
}

// NewRemoveEvent creates a telemetry event for a remove action.
func NewRemoveEvent(recipe, versionPrevious string) Event {
	e := newBaseEvent()
	e.Action = "remove"
	e.Recipe = recipe
	e.VersionPrevious = versionPrevious
	return e
}

// LLMEvent represents a telemetry event for LLM operations.
type LLMEvent struct {
	Action        string `json:"action"`                   // Event type (e.g., "llm_generation_started")
	Provider      string `json:"provider,omitempty"`       // LLM provider name (e.g., "claude", "gemini")
	ToolName      string `json:"tool_name,omitempty"`      // Tool being generated
	Repo          string `json:"repo,omitempty"`           // GitHub repository (owner/repo)
	Success       bool   `json:"success,omitempty"`        // Whether the operation succeeded
	DurationMs    int64  `json:"duration_ms,omitempty"`    // Duration in milliseconds
	Attempts      int    `json:"attempts,omitempty"`       // Total number of attempts
	AttemptNumber int    `json:"attempt_number,omitempty"` // Current attempt number
	ErrorCategory string `json:"error_category,omitempty"` // Category of error (for failures)
	Passed        bool   `json:"passed,omitempty"`         // Whether validation passed
	FromProvider  string `json:"from_provider,omitempty"`  // Provider failing over from
	ToProvider    string `json:"to_provider,omitempty"`    // Provider failing over to
	Reason        string `json:"reason,omitempty"`         // Reason for failover/trip
	Failures      int    `json:"failures,omitempty"`       // Number of failures (circuit breaker)
	OS            string `json:"os"`                       // Operating system
	Arch          string `json:"arch"`                     // CPU architecture
	TsukuVersion  string `json:"tsuku_version"`            // Version of tsuku CLI
	SchemaVersion string `json:"schema_version"`           // Event schema version
}

// llmSchemaVersion is the schema version for LLM events.
const llmSchemaVersion = "1"

// newBaseLLMEvent creates an LLMEvent with common fields pre-filled.
func newBaseLLMEvent() LLMEvent {
	return LLMEvent{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		TsukuVersion:  buildinfo.Version(),
		SchemaVersion: llmSchemaVersion,
	}
}

// NewLLMGenerationStartedEvent creates an event for when LLM generation begins.
func NewLLMGenerationStartedEvent(provider, toolName, repo string) LLMEvent {
	e := newBaseLLMEvent()
	e.Action = "llm_generation_started"
	e.Provider = provider
	e.ToolName = toolName
	e.Repo = repo
	return e
}

// NewLLMGenerationCompletedEvent creates an event for when LLM generation ends.
func NewLLMGenerationCompletedEvent(provider, toolName string, success bool, durationMs int64, attempts int) LLMEvent {
	e := newBaseLLMEvent()
	e.Action = "llm_generation_completed"
	e.Provider = provider
	e.ToolName = toolName
	e.Success = success
	e.DurationMs = durationMs
	e.Attempts = attempts
	return e
}

// NewLLMRepairAttemptEvent creates an event for when a repair attempt starts.
func NewLLMRepairAttemptEvent(provider string, attemptNumber int, errorCategory string) LLMEvent {
	e := newBaseLLMEvent()
	e.Action = "llm_repair_attempt"
	e.Provider = provider
	e.AttemptNumber = attemptNumber
	e.ErrorCategory = errorCategory
	return e
}

// NewLLMValidationResultEvent creates an event for when validation completes.
func NewLLMValidationResultEvent(passed bool, errorCategory string, attemptNumber int) LLMEvent {
	e := newBaseLLMEvent()
	e.Action = "llm_validation_result"
	e.Passed = passed
	e.ErrorCategory = errorCategory
	e.AttemptNumber = attemptNumber
	return e
}

// NewLLMCircuitBreakerTripEvent creates an event for when a circuit breaker trips.
func NewLLMCircuitBreakerTripEvent(provider string, failures int) LLMEvent {
	e := newBaseLLMEvent()
	e.Action = "llm_circuit_breaker_trip"
	e.Provider = provider
	e.Failures = failures
	return e
}

// DiscoveryEvent represents a telemetry event for discovery resolver operations.
type DiscoveryEvent struct {
	Action        string `json:"action"`                   // discovery_* action type
	ToolName      string `json:"tool_name"`                // Normalized tool name searched
	Confidence    string `json:"confidence,omitempty"`     // Winning stage: "registry", "ecosystem", "llm"
	Builder       string `json:"builder,omitempty"`        // Builder name (github, cargo, pip, etc.)
	Source        string `json:"source,omitempty"`         // Source identifier (e.g., "sharkdp/bat")
	MatchCount    int    `json:"match_count,omitempty"`    // Number of matches (disambiguation)
	ErrorCategory string `json:"error_category,omitempty"` // Error type on failure
	DurationMs    int64  `json:"duration_ms"`              // Total resolution time in milliseconds
	OS            string `json:"os"`                       // Operating system
	Arch          string `json:"arch"`                     // CPU architecture
	TsukuVersion  string `json:"tsuku_version"`            // Version of tsuku CLI
	SchemaVersion string `json:"schema_version"`           // Event schema version
}

// discoverySchemaVersion is the schema version for discovery events.
const discoverySchemaVersion = "1"

// newBaseDiscoveryEvent creates a DiscoveryEvent with common fields pre-filled.
func newBaseDiscoveryEvent() DiscoveryEvent {
	return DiscoveryEvent{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		TsukuVersion:  buildinfo.Version(),
		SchemaVersion: discoverySchemaVersion,
	}
}

// NewDiscoveryRegistryHitEvent creates an event for a successful registry lookup.
func NewDiscoveryRegistryHitEvent(toolName, builder, source string, durationMs int64) DiscoveryEvent {
	e := newBaseDiscoveryEvent()
	e.Action = "discovery_registry_hit"
	e.ToolName = toolName
	e.Confidence = "registry"
	e.Builder = builder
	e.Source = source
	e.DurationMs = durationMs
	return e
}

// NewDiscoveryEcosystemHitEvent creates an event for a successful ecosystem probe.
func NewDiscoveryEcosystemHitEvent(toolName, builder, source string, durationMs int64) DiscoveryEvent {
	e := newBaseDiscoveryEvent()
	e.Action = "discovery_ecosystem_hit"
	e.ToolName = toolName
	e.Confidence = "ecosystem"
	e.Builder = builder
	e.Source = source
	e.DurationMs = durationMs
	return e
}

// NewDiscoveryLLMHitEvent creates an event for a successful LLM discovery.
func NewDiscoveryLLMHitEvent(toolName, builder, source string, durationMs int64) DiscoveryEvent {
	e := newBaseDiscoveryEvent()
	e.Action = "discovery_llm_hit"
	e.ToolName = toolName
	e.Confidence = "llm"
	e.Builder = builder
	e.Source = source
	e.DurationMs = durationMs
	return e
}

// NewDiscoveryNotFoundEvent creates an event when no stage found a match.
func NewDiscoveryNotFoundEvent(toolName string, durationMs int64) DiscoveryEvent {
	e := newBaseDiscoveryEvent()
	e.Action = "discovery_not_found"
	e.ToolName = toolName
	e.DurationMs = durationMs
	return e
}

// NewDiscoveryDisambiguationEvent creates an event when multiple matches were found.
func NewDiscoveryDisambiguationEvent(toolName, builder, source string, matchCount int, durationMs int64) DiscoveryEvent {
	e := newBaseDiscoveryEvent()
	e.Action = "discovery_disambiguation"
	e.ToolName = toolName
	e.Builder = builder
	e.Source = source
	e.MatchCount = matchCount
	e.DurationMs = durationMs
	return e
}

// NewDiscoveryErrorEvent creates an event when a resolver stage failed.
func NewDiscoveryErrorEvent(toolName, errorCategory string, durationMs int64) DiscoveryEvent {
	e := newBaseDiscoveryEvent()
	e.Action = "discovery_error"
	e.ToolName = toolName
	e.ErrorCategory = errorCategory
	e.DurationMs = durationMs
	return e
}
