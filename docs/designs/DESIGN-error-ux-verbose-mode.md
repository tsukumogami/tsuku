---
status: Accepted
problem: |
  The discovery resolver produces only a generic "could not find" message regardless
  of why discovery failed. Users don't know if they need an API key, if the tool
  doesn't exist, or if they hit a rate limit. They also can't see which resolver
  stages ran during discovery, making debugging difficult. The design specifies
  8 distinct error scenarios that need different messages and guidance.
decision: |
  Create a structured error type hierarchy with context-aware messages, and add
  INFO-level resolver chain logging that's visible with --verbose. Each error
  scenario produces a specific message with actionable guidance. Verbose output
  shows stage attempts and outcomes without exposing internal types.
rationale: |
  The existing logging infrastructure (--verbose at INFO level) and error type
  pattern (NotFoundError with custom Error() method) provide the foundation.
  Extending this with additional error types and INFO logging keeps the code
  consistent while providing the user feedback the design requires. The error
  types encode enough context to produce different messages without needing
  complex state tracking.
---

# DESIGN: Error UX and Verbose Mode

## Status

Accepted

## Upstream Design Reference

This design implements Phase 6 (Error UX and Verbose Mode) of [DESIGN-discovery-resolver.md](DESIGN-discovery-resolver.md). It addresses the design questions raised in [issue #1322](https://github.com/tsukumogami/tsuku/issues/1322).

## Context and Problem Statement

The discovery resolver has three stages: registry lookup, ecosystem probe, and LLM discovery. Each stage can fail in different ways, and different failures require different user actions. Right now, all failures produce the same message:

```
could not find 'foo'. Try tsuku install foo --from github:owner/repo if you know the source
```

This message is accurate but unhelpful. A user who lacks an API key sees the same message as one who mistyped a tool name. The parent design specifies 8 distinct error scenarios, each with an actionable message:

| Scenario | Required Message |
|----------|-----------------|
| No match anywhere | `Could not find 'foo'. Try tsuku install foo --from github:owner/repo if you know the source.` |
| LLM not configured | `No match in registry or ecosystems. Set ANTHROPIC_API_KEY to enable web search discovery, or use --from to specify the source directly.` |
| `--deterministic-only`, no ecosystem match | `No deterministic source found for 'foo'. Remove --deterministic-only to enable LLM discovery, or use --from to specify the source.` |
| `--deterministic-only`, builder requires LLM | `'foo' resolved to GitHub releases (owner/repo), which requires LLM for recipe generation. Remove --deterministic-only or wait for a recipe to be contributed.` |
| Ecosystem probe timeout | (Silently fall through to LLM; show timeout warning in verbose mode) |
| LLM rate limit/budget | `LLM discovery unavailable (rate limit). Try --from <source> to specify the source directly.` |
| Non-ASCII input (homoglyph) | `Tool name 'foo' contains non-ASCII character 'x' — possible homoglyph attack. Use ASCII characters only.` (already implemented in normalize.go) |
| Ecosystem ambiguity | (Handled by #1321 Disambiguation, out of scope) |

Beyond error messages, users need visibility into what's happening. The `--verbose` flag should show which stages ran and what they found, without exposing internal terminology like "ChainResolver" or "EcosystemProbe."

**Important**: These errors apply only to the discovery path (when no `--from` flag is provided). When users explicitly specify `--from github:owner/repo`, they bypass discovery entirely and the existing builder error handling applies.

### Scope

**In scope:**
- Error type hierarchy for distinct failure scenarios in the discovery path
- Message formatting with context (tool name, source, builder)
- Verbose output showing resolver chain progress at INFO level
- Integration with existing logging infrastructure
- Adding logging infrastructure to ecosystem probe for timeout warnings

**Out of scope:**
- Ecosystem ambiguity errors (handled by #1321 Disambiguation)
- Typosquatting warnings (handled by #1649 as part of M81)
- Homoglyph detection (deferred; ASCII gate is already implemented)
- New CLI flags (using existing `--verbose`)
- Errors from direct `--from` usage (existing builder error handling)

## Decision Drivers

- **Actionable messages**: Each failure scenario needs guidance on what the user can do next
- **Consistent terminology**: Messages use user-facing terms ("web search discovery") not internal types ("LLMDiscovery")
- **Minimal new code**: Reuse existing Logger interface and error patterns
- **Testable**: Error types should be inspectable for testing
- **No breaking changes**: `NotFoundError` behavior preserved for callers that inspect error types

## Considered Options

### Decision 1: Error Type Architecture

The resolver chain needs to communicate different failure scenarios to the CLI layer, which then formats appropriate messages. The question is how to structure the error types: should each scenario have its own type, or should we use a single type with variant fields?

Go's error handling patterns support both approaches. Individual types are more idiomatic and allow `errors.As()` inspection. A single type with fields is simpler but requires callers to check variant fields. The chain resolver currently uses `NotFoundError` as a sentinel, so compatibility with existing callers matters.

#### Chosen: Individual Error Types with Suggestion Method

Define individual error types for each distinct scenario. Each type implements `error` plus a `Suggestion() string` method that returns actionable guidance. This keeps error messages separate from suggestions, allowing the CLI to format them appropriately.

```go
// ConfigurationError indicates LLM discovery is unavailable due to missing configuration.
type ConfigurationError struct {
    Tool   string
    Reason string // "no_api_key" or "deterministic_only"
}

func (e *ConfigurationError) Error() string {
    return fmt.Sprintf("no configuration for LLM discovery: %s", e.Reason)
}

func (e *ConfigurationError) Suggestion() string {
    switch e.Reason {
    case "no_api_key":
        return "Set ANTHROPIC_API_KEY to enable web search discovery, or use --from to specify the source directly."
    case "deterministic_only":
        return fmt.Sprintf("Remove --deterministic-only to enable LLM discovery, or use --from to specify the source.")
    default:
        return ""
    }
}
```

The CLI layer can then:
1. Display `err.Error()` as the main message
2. Display `err.Suggestion()` as guidance
3. Use `errors.As()` to identify error types for testing or special handling

#### Alternatives Considered

**Single NotFoundError with context fields**: Extend `NotFoundError` with fields like `HasAPIKey bool`, `DeterministicOnly bool`, `TriedEcosystem bool`. The `Error()` method would check fields to produce different messages.

Rejected because it conflates "not found" with "not configured." A missing API key isn't a "not found" error; it's a configuration error. Distinct types make the error semantics clearer and allow callers to handle them differently (e.g., a wrapper script might want to set an API key automatically).

**Return error + suggestion as separate values**: Change `Resolve()` to return `(result, suggestion, error)` where suggestion is always populated.

Rejected because it changes the interface signature and complicates callers. The `Suggestion()` method on error types achieves the same goal without interface changes.

### Decision 2: Verbose Output Format

When `--verbose` is set, users should see resolver chain progress. The question is how to format this output and at what log level.

The existing logging system maps `--verbose` to INFO level. Messages at INFO are visible with `-v` but hidden by default. The format should be human-readable without exposing internal types.

#### Chosen: Stage Announcement at INFO Level

Log a single INFO message when each stage starts, with outcome information. Use user-facing terminology.

```
INFO  Checking discovery registry for 'ripgrep'...
INFO  Not in registry, probing package ecosystems...
INFO  Found in crates.io (45K downloads, 87 versions)
```

Or for failures:
```
INFO  Checking discovery registry for 'foo'...
INFO  Not in registry, probing package ecosystems...
INFO  No ecosystem match, trying web search...
WARN  Web search timed out after 15s
INFO  Could not find 'foo' in any source
```

The chain resolver calls `logger.Info()` at stage transitions. Stage implementations log their own progress at DEBUG level for finer detail.

#### Alternatives Considered

**DEBUG level only**: Put all resolver progress at DEBUG level, require `--debug` to see it.

Rejected because `--debug` also enables timestamps and source locations, cluttering output for users who just want to see what's happening. INFO strikes the right balance: visible with `-v`, clean format, no debugging noise.

**Structured progress events**: Emit structured events that a progress bar or TUI could consume.

Rejected as premature. The current CLI doesn't have a TUI, and adding one isn't in scope. Plain text logging works with existing infrastructure and can be enhanced later.

**Stage timing in output**: Show elapsed time for each stage (e.g., "Registry lookup: 2ms, Ecosystem probe: 1.2s").

Deferred. Timing is useful for debugging but adds complexity. Can be added to DEBUG level output without changing the INFO format.

### Decision 3: Where Error Context Lives

Error messages need context (tool name, builder, source) to be actionable. The question is where this context is assembled: in the error type constructor, in the chain resolver, or in the CLI layer.

#### Chosen: Error Types Own Their Context

Each error type stores the context it needs in its fields. The chain resolver (or stage implementations) creates errors with full context. The CLI layer just calls `Error()` and `Suggestion()`.

```go
// BuilderRequiresLLMError indicates the resolved builder requires LLM but --deterministic-only was set.
type BuilderRequiresLLMError struct {
    Tool    string
    Builder string
    Source  string
}

func (e *BuilderRequiresLLMError) Error() string {
    return fmt.Sprintf("'%s' resolved to %s (%s), which requires LLM for recipe generation",
        e.Tool, e.Builder, e.Source)
}

func (e *BuilderRequiresLLMError) Suggestion() string {
    return "Remove --deterministic-only or wait for a recipe to be contributed."
}
```

This keeps the CLI layer simple and allows error messages to be tested independently of the full resolver chain.

#### Alternatives Considered

**CLI layer formats messages**: Error types return raw data, CLI layer has message templates.

Rejected because it spreads error message logic across packages. When a message needs updating, you'd need to find and change the CLI code, not the error definition. Error types owning their messages keeps related code together.

**Chain resolver wraps all errors**: Stage implementations return generic errors, chain resolver wraps them with context.

Rejected because the chain resolver doesn't always have the right context. For `BuilderRequiresLLMError`, the context (builder, source) comes from the discovery result, which the stage implementation has but the chain resolver only receives as part of a successful result.

### Uncertainties

- **Message wording**: The exact wording from the parent design may need adjustment based on user feedback. The types support changing messages without changing callers.
- **Soft error visibility**: Currently, soft errors (stage timeouts, API errors) are logged at WARN. Should they also produce INFO messages? Starting with WARN for errors, INFO for progress seems right, but may need tuning.
- **Rate limit recovery**: The `RateLimitError` includes `ResetTime`. Should the message show when rate limits reset? The current implementation does; this design preserves that behavior.
- **Error wrapping**: The new error types don't implement `Unwrap()` for `errors.Is()` chains. If callers need to wrap these errors while preserving type inspection, we may need to add wrapping support later.
- **Parallel logging interleave**: The ecosystem probe queries multiple registries in parallel. If each prober logs individually at DEBUG level, messages could interleave. This is acceptable for DEBUG but shouldn't affect INFO-level stage announcements, which are logged by the chain resolver before/after each stage completes.

## Decision Outcome

**Chosen: Individual error types with Suggestion() method, INFO-level stage announcements**

### Summary

Each error scenario gets its own error type that stores relevant context and implements both `Error()` and `Suggestion()` methods. The chain resolver logs stage transitions at INFO level using user-facing terminology. The CLI layer formats errors by calling `Error()` and `Suggestion()` on whatever error type is returned.

This approach builds on existing patterns (NotFoundError, RateLimitError) and infrastructure (log.Logger, --verbose flag). The error types are testable in isolation. The verbose output gives users visibility without exposing internal architecture.

### Error Type Inventory

| Type | Fields | When Used |
|------|--------|-----------|
| `NotFoundError` | Tool | All stages failed, no matches found |
| `ConfigurationError` | Tool, Reason | LLM stage skipped due to missing API key or --deterministic-only |
| `BuilderRequiresLLMError` | Tool, Builder, Source | Resolution succeeded but builder requires LLM and --deterministic-only is set |
| `RateLimitError` | ResetTime, Authenticated | GitHub API rate limit hit (already exists) |
| `ErrBudgetExceeded` | (sentinel) | Daily LLM budget exceeded (already exists) |

Note: `AmbiguousMatchError` is defined in #1321 (Disambiguation) and isn't duplicated here.

### Verbose Output Contract

With `--verbose`, users see:
```
Checking discovery registry for '<tool>'...
[Not found OR Found: <source>]
Probing package ecosystems...
[No match OR Found in <ecosystem> (<stats>)]
Using web search to find '<tool>'...
[Search results OR Timeout OR Rate limited]
```

Each line is an INFO log message. Failures within stages produce WARN messages. DEBUG includes additional detail (cache hits, API response times, etc.).

### Rationale

Individual error types match Go idioms and allow callers to use `errors.As()` for specific handling. The `Suggestion()` method separates "what went wrong" from "what to do about it," letting the CLI format these appropriately. INFO-level logging for stage progress uses existing infrastructure without new flags or configuration.

## Solution Architecture

### Component 1: Error Types

**Note**: The `errmsg` package already defines a `Suggester` interface. For consistency with existing error handling, the new error types should implement the same interface pattern. The `RateLimitError` in `llm_discovery.go` already follows this pattern.

Extend `internal/discover/resolver.go` with new error types:

```go
// Suggester is implemented by errors that provide actionable guidance.
type Suggester interface {
    Suggestion() string
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
```

Update `NotFoundError` to implement `Suggester`:

```go
func (e *NotFoundError) Suggestion() string {
    return fmt.Sprintf("Try tsuku install %s --from github:owner/repo if you know the source.", e.Tool)
}
```

### Component 2: Chain Resolver Logging

Add a logger field to `ChainResolver` and log stage transitions:

```go
type ChainResolver struct {
    stages    []Resolver
    telemetry *telemetry.Client
    logger    log.Logger
}

func (c *ChainResolver) WithLogger(l log.Logger) *ChainResolver {
    c.logger = l
    return c
}

func (c *ChainResolver) Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error) {
    // ... normalization ...

    c.logInfo("Checking discovery registry for '%s'...", normalized)

    for i, stage := range c.stages {
        result, err := stage.Resolve(ctx, normalized)
        if err != nil {
            // ... existing error handling ...
        }
        if result != nil {
            c.logStageHit(result)
            return result, nil
        }
        c.logStageMiss(i, len(c.stages), normalized)
    }

    c.logInfo("Could not find '%s' in any source", normalized)
    return nil, &NotFoundError{Tool: toolName}
}

func (c *ChainResolver) logInfo(format string, args ...any) {
    if c.logger != nil {
        c.logger.Info(fmt.Sprintf(format, args...))
    }
}

func (c *ChainResolver) logStageHit(result *DiscoveryResult) {
    if c.logger == nil {
        return
    }
    // Use confidence level to determine which stage produced the result
    switch result.Confidence {
    case ConfidenceRegistry:
        c.logger.Info(fmt.Sprintf("Found in registry: %s", result.Source))
    case ConfidenceEcosystem:
        c.logger.Info(fmt.Sprintf("Found in %s (%s)", result.Builder, formatMetadata(result.Metadata)))
    case ConfidenceLLM:
        c.logger.Info(fmt.Sprintf("Found via web search: %s/%s", result.Builder, result.Source))
    }
}

func (c *ChainResolver) logStageMiss(stageIndex int, stageCount int, tool string) {
    if c.logger == nil {
        return
    }
    // Use remaining stages to determine what to log
    remaining := stageCount - stageIndex - 1
    switch remaining {
    case 2: // Registry missed, ecosystem and LLM remain
        c.logger.Info("Not in registry, probing package ecosystems...")
    case 1: // Ecosystem missed, LLM remains
        c.logger.Info("No ecosystem match, trying web search...")
    // case 0: LLM missed, nothing remains - final "not found" logged separately
    }
}
```

### Component 3: CLI Error Formatting

In `cmd/tsuku/create.go` (and `install.go`), format errors with suggestions:

```go
func formatDiscoveryError(err error) string {
    var buf strings.Builder
    buf.WriteString("Error: ")
    buf.WriteString(err.Error())
    buf.WriteString("\n")

    if suggester, ok := err.(discover.Suggester); ok {
        if suggestion := suggester.Suggestion(); suggestion != "" {
            buf.WriteString("\n")
            buf.WriteString(suggestion)
            buf.WriteString("\n")
        }
    }

    return buf.String()
}
```

### Component 4: Configuration Error Detection

The chain resolver needs to know when LLM discovery is unavailable due to configuration. Currently, `create.go` determines LLM availability when constructing the chain:

```go
// From create.go - LLM availability is determined here
llmDiscovery, err := discover.NewLLMDiscovery(globalCtx, ...)
if err != nil {
    stages = append(stages, discover.NewLLMDiscoveryDisabled())
} else {
    stages = append(stages, llmDiscovery)
}
```

Add a configuration struct that the CLI passes to the chain resolver:

```go
// LLMAvailability represents whether LLM discovery can be attempted.
type LLMAvailability struct {
    DeterministicOnly bool  // --deterministic-only flag was set
    HasAPIKey        bool  // ANTHROPIC_API_KEY is configured
}

func (c *ChainResolver) WithLLMAvailability(avail LLMAvailability) *ChainResolver {
    c.llmAvailability = avail
    return c
}
```

When all stages fail:
- If `DeterministicOnly` is set, return `ConfigurationError{Reason: "deterministic_only"}`
- If `!HasAPIKey`, return `ConfigurationError{Reason: "no_api_key"}`
- Otherwise return `NotFoundError`

**Integration point**: The CLI constructs `LLMAvailability` from flags and environment, then passes it to the chain resolver via `WithLLMAvailability()` alongside the telemetry and logger configuration.

### Data Flow

```
User: tsuku install ripgrep -v
                |
                v
        ChainResolver.Resolve()
                |
                +-- INFO: Checking discovery registry for 'ripgrep'...
                |
        RegistryLookup.Resolve()
                |
                +-- (not found)
                |
                +-- INFO: Not in registry, probing package ecosystems...
                |
        EcosystemProbe.Resolve()
                |
                +-- INFO: Found in crates.io (45K downloads, 87 versions)
                |
                v
        Return DiscoveryResult
                |
                v
        CLI: Installing ripgrep from crates.io...
```

For failures:
```
User: tsuku install nonexistent --deterministic-only
                |
                v
        ChainResolver.Resolve()
                |
                +-- (registry miss)
                +-- (ecosystem miss)
                +-- (LLM skipped: deterministic-only)
                |
                v
        Return ConfigurationError{Reason: "deterministic_only"}
                |
                v
        CLI: Error: no deterministic source found for 'nonexistent'

             Remove --deterministic-only to enable LLM discovery,
             or use --from to specify the source.
```

## Implementation Approach

### Phase 1: Error Types

**Files to modify:**
- `internal/discover/resolver.go` - Add Suggester interface, ConfigurationError, BuilderRequiresLLMError
- `internal/discover/resolver_test.go` - Test error messages

**Deliverable:** New error types with correct Error() and Suggestion() output.

### Phase 2: Chain Resolver Logging

**Files to modify:**
- `internal/discover/chain.go` - Add logger field, WithLogger(), stage logging
- `internal/discover/chain_test.go` - Verify log output
- `internal/discover/ecosystem_probe.go` - Add logger field, WithLogger(), timeout WARN logging

**Note:** The ecosystem probe currently lacks logging infrastructure. The probe runs parallel queries with a 3-second timeout (per parent design), but timeout warnings aren't logged anywhere. This phase adds a logger to the ecosystem probe so it can emit WARN messages when individual ecosystem queries time out.

**Deliverable:** Stage transitions logged at INFO level, ecosystem probe timeouts logged at WARN level.

### Phase 3: CLI Formatting

**Files to modify:**
- `cmd/tsuku/create.go` - Format errors with suggestions
- `cmd/tsuku/install.go` - Same formatting

**Note:** The existing `errmsg.FormatError` already walks the error chain for `Suggester` implementations. Verify the new error types work with this infrastructure before adding custom formatting.

**Deliverable:** User sees error message + suggestion on separate lines.

### Phase 4: Configuration-Aware Errors

**Files to modify:**
- `internal/discover/chain.go` - Add LLMAvailability, return appropriate error type
- `cmd/tsuku/create.go` - Construct and pass LLMAvailability to chain resolver

**Depends on:** Phase 3 (CLI formatting needed to verify end-to-end behavior)

**Deliverable:** Different error types returned based on configuration state.

## Security Considerations

### Download Verification

Not applicable. This design adds error messages and logging; it doesn't change how downloads are verified.

### Execution Isolation

Not applicable. Error messages and logs don't execute code.

### Supply Chain Risks

**Information leakage**: Error messages shouldn't reveal internal architecture details that could help attackers. The design uses user-facing terms ("web search discovery" not "LLMDiscovery") and doesn't expose internal state.

**Verbose output in CI**: Users might pipe `--verbose` output to logs that get shared. The messages don't include sensitive data (no API keys, no file paths outside $TSUKU_HOME).

### User Data Exposure

Error messages include the tool name, which the user typed. Verbose output includes ecosystem names and public metadata (download counts). No private data is logged.

## Consequences

### Positive

- Users get actionable guidance for each failure scenario
- `--verbose` shows resolver progress without requiring `--debug`
- Error types are testable in isolation
- Existing callers continue to work (NotFoundError preserved)

### Negative

- More error types to maintain
- Chain resolver needs configuration state it didn't need before (LLMAvailability)
- Log output depends on logger being set; callers that don't set a logger see no verbose output

### Neutral

- Error message wording may need adjustment based on user feedback
- DEBUG level output unchanged; this design only addresses INFO level
