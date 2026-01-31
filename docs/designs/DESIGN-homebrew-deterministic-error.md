---
status: Proposed
problem: The batch pipeline crashes when Homebrew deterministic generation fails because the CLI can't suppress LLM fallback, producing "no LLM providers available" instead of a structured failure record.
decision: Add a --deterministic-only CLI flag to tsuku create that sets SessionOptions.DeterministicOnly, and add a dedicated exit code (9) for deterministic failures so the batch orchestrator can categorize them without output parsing.
rationale: The Go API already supports deterministic-only mode with structured errors. Exposing it through the CLI closes the gap between the builder API and the subprocess-based orchestrator. A dedicated exit code lets the orchestrator classify failures reliably.
---

# DESIGN: Homebrew Deterministic Error Path

## Status

Proposed

## Upstream Design Reference

This design implements part of [DESIGN-homebrew-deterministic-mode.md](current/DESIGN-homebrew-deterministic-mode.md) and [DESIGN-registry-scale-strategy.md](DESIGN-registry-scale-strategy.md).

**Relevant sections:**
- DESIGN-homebrew-deterministic-mode.md: DeterministicOnly session option, DeterministicFailedError type
- DESIGN-registry-scale-strategy.md: M-HomebrewBuilder milestone, deterministic-only pipeline decision

## Context and Problem Statement

The Homebrew builder has a `DeterministicOnly` session option and a `DeterministicFailedError` type (delivered by #1188). These work correctly when the builder is called through Go code. But the batch orchestrator doesn't call builders through Go code. It invokes `tsuku create` as a subprocess.

The CLI's `create` command has no flag to set `DeterministicOnly`. When deterministic generation fails, the command falls through to LLM fallback. Without API keys (the batch pipeline's design constraint), this produces:

```
Error building recipe: generation failed: failed to create LLM factory: no LLM providers available
```

The batch pipeline gets exit code 1 (general error) and an unhelpful error message. The structured failure data from `DeterministicFailedError` (formula name, failure category, human-readable reason) never reaches the orchestrator.

Some Homebrew packages can't be handled deterministically (e.g., formulas with no bottles, complex archive layouts). These need to produce structured failure records for gap analysis, not crashes.

### Scope

**In scope:**
- CLI flag to enable deterministic-only mode for `tsuku create`
- Dedicated exit code for deterministic failures
- Batch orchestrator updates to use the flag and exit code
- Structured error output that preserves `DeterministicFailedError` data

**Out of scope:**
- Improving deterministic success rate (separate work)
- Changes to `DeterministicFailedError` categories or classification logic
- Structured JSON output for other commands (#1273)

## Decision Drivers

- The Go API already works correctly; only the CLI-to-API bridge is missing
- Batch orchestrator uses exit codes to classify failures (`categoryFromExitCode` in orchestrator.go)
- Exit codes are the most reliable signal for subprocess-based callers (output parsing is fragile)
- The existing `ExitInstallFailed` (6) is too broad to distinguish "deterministic insufficient" from other failures
- Interactive `tsuku create` users shouldn't see any behavior change

## Considered Options

### Option A: Add `--deterministic-only` CLI Flag

Add a `--deterministic-only` flag to `tsuku create`. When set, pass `DeterministicOnly: true` in `SessionOptions`. Add a new exit code (9) for deterministic failures. Print the failure category and message to stderr before exiting.

**Pros:**
- Direct mapping from CLI flag to existing Go API
- Exit code gives the orchestrator a clean classification signal
- No output parsing needed for basic failure categorization
- Flag is self-documenting (`--help` shows it)

**Cons:**
- Adds a new exit code (needs documentation)
- Adds a flag that's only useful for automation, not interactive users

### Option B: Auto-detect No-LLM Environment

When no LLM API keys are configured, automatically treat all builders as deterministic-only. The CLI detects missing keys and sets `DeterministicOnly` internally.

**Pros:**
- No new flag needed
- Batch pipeline doesn't need changes (it already runs without keys)

**Cons:**
- Changes behavior for interactive users who forgot to set API keys (they'd get a structured error instead of "no LLM providers")
- Implicit behavior is harder to debug than explicit flags
- Couples LLM key presence to deterministic-only mode, which are conceptually separate
- Doesn't help if someone runs with API keys but wants deterministic-only output

### Option C: Orchestrator Parses CLI Output

Keep the CLI unchanged. Have the batch orchestrator parse the error output to detect "no LLM providers available" and reclassify it as a deterministic failure.

**Pros:**
- No CLI changes needed
- Quick to implement

**Cons:**
- Fragile: any change to the error message breaks classification
- Loses structured data (category, formula name) that `DeterministicFailedError` provides
- The TODO comment at orchestrator.go:260 already notes output parsing is brittle
- Doesn't fix the root cause (CLI can't suppress LLM fallback)

## Decision Outcome

**Chosen: Option A (CLI flag + exit code)**

### Rationale

The builder API already does the right thing. The missing piece is a CLI flag to activate it and an exit code to signal the result. Option A adds both with minimal changes.

Option B was rejected because implicit behavior from environment detection is confusing. A user running `tsuku create --from homebrew:imagemagick` without API keys currently gets told "no LLM providers available," which at least explains what's wrong. Silently switching to deterministic-only mode and returning a different error would be surprising.

Option C was rejected because it builds on the exact pattern the codebase is already trying to move away from (output parsing). The TODO at orchestrator.go:260 calls for structured output, not more regex.

### Trade-offs Accepted

- `--deterministic-only` is a flag only useful for automation. Interactive users won't need it. This is acceptable because CLI flags don't impose cost on users who ignore them, and `tsuku create --help` makes the flag discoverable for pipeline authors.
- A new exit code (9) means `ExitDeterministicFailed` must be documented. The exit code table in exitcodes.go already has 8 codes, so one more is a small addition.

## Solution Architecture

### Overview

Three changes close the gap between the builder API and the subprocess-based orchestrator:

1. **CLI flag**: `--deterministic-only` on `tsuku create` sets `SessionOptions.DeterministicOnly`
2. **Exit code**: New `ExitDeterministicFailed = 9` signals that deterministic generation produced a structured failure
3. **Orchestrator update**: Pass `--deterministic-only` for Homebrew packages, map exit code 9 to the right failure category

### CLI Changes (create.go)

```go
// New flag
var deterministicOnly bool
createCmd.Flags().BoolVar(&deterministicOnly, "deterministic-only", false,
    "Skip LLM fallback; exit with structured error if deterministic generation fails")

// In session options construction
sessionOpts := &builders.SessionOptions{
    ProgressReporter:  progressReporter,
    LLMConfig:         userCfg,
    LLMStateTracker:   stateManager,
    DeterministicOnly: deterministicOnly,
}
```

When `Generate()` returns a `DeterministicFailedError`, the CLI prints a structured message and exits with code 9:

```go
var detErr *builders.DeterministicFailedError
if errors.As(err, &detErr) {
    fmt.Fprintf(os.Stderr, "deterministic generation failed: [%s] %s\n",
        detErr.Category, detErr.Message)
    exitWithCode(ExitDeterministicFailed)
}
```

The stderr format `[category] message` is simple for humans and parseable by the orchestrator if needed, though the exit code alone is sufficient for basic classification.

### New Exit Code (exitcodes.go)

```go
// ExitDeterministicFailed indicates deterministic generation failed
// and LLM fallback was suppressed (--deterministic-only flag).
ExitDeterministicFailed = 9
```

### Orchestrator Changes (batch/orchestrator.go)

The `generate` method adds `--deterministic-only` to the command arguments:

```go
func (o *Orchestrator) generate(bin string, pkg seed.Package, recipePath string) generateResult {
    args := []string{
        "create", pkg.Name,
        "--from", pkg.ID,
        "--output", recipePath,
        "--yes",
        "--skip-sandbox",
        "--deterministic-only",
    }
    // ... rest unchanged
}
```

The `categoryFromExitCode` function adds a case for exit code 9:

```go
func categoryFromExitCode(code int) string {
    switch code {
    case 5:
        return "api_error"
    case 6:
        return "validation_failed"
    case 7:
        return "validation_failed"
    case 8:
        return "missing_dep"
    case 9:
        return "deterministic_insufficient"
    default:
        return "validation_failed"
    }
}
```

### Data Flow

```
Batch Orchestrator                    CLI (tsuku create)                  Homebrew Builder
     |                                     |                                    |
     |-- tsuku create foo                  |                                    |
     |   --from homebrew:foo               |                                    |
     |   --deterministic-only ------------>|                                    |
     |                                     |-- SessionOptions{                  |
     |                                     |     DeterministicOnly: true} ----->|
     |                                     |                                    |
     |                                     |                            generateDeterministic()
     |                                     |                                    |
     |                                     |                            (fails)
     |                                     |                                    |
     |                                     |<-- DeterministicFailedError{       |
     |                                     |      Category: "no_bottles",       |
     |                                     |      Message: "..."} -------------|
     |                                     |                                    |
     |<-- exit code 9 --------------------|                                    |
     |    stderr: [no_bottles] ...         |                                    |
     |                                     |                                    |
     |-- categoryFromExitCode(9)           |                                    |
     |   = "deterministic_insufficient"    |                                    |
     |                                     |                                    |
     |-- FailureRecord{                    |                                    |
     |     Category: "deterministic_insufficient"                               |
     |   }                                 |                                    |
```

### Error Propagation

The CLI calls `builders.Orchestrator.Create()`, which wraps errors with `fmt.Errorf("generation failed: %w", err)`. The `%w` verb preserves the error chain, so `errors.As(err, &detErr)` in the CLI's error handler correctly unwraps through to the `DeterministicFailedError` returned by the Homebrew builder. No special passthrough logic is needed.

### Flag Scope

The `--deterministic-only` flag applies to all builders, not just Homebrew. For ecosystem builders (Cargo, NPM, etc.) that are already fully deterministic, the flag is a no-op since they never attempt LLM fallback. The batch orchestrator passes the flag unconditionally rather than conditionally per ecosystem.

### Key Design Points

**Why not parse the `[category]` from stderr?** The exit code is sufficient for the orchestrator's current needs. It maps to `"deterministic_insufficient"` which tells the failure analysis system this package needs LLM or improved heuristics. If finer-grained categorization is needed later (distinguishing `no_bottles` from `complex_archive` at the orchestrator level), the stderr format supports it, but that's a future enhancement.

**Why `"deterministic_insufficient"` instead of passing through the builder's category?** The builder's categories (`no_bottles`, `complex_archive`, etc.) describe *why* deterministic failed. The orchestrator's category describes *what happened* from the pipeline's perspective: the deterministic path wasn't sufficient. The builder's detailed categories still get logged in the CLI's stderr output and can be parsed later if needed (#1273 will add structured JSON output).

## Implementation Approach

### Step 1: Exit Code

Add `ExitDeterministicFailed = 9` to `exitcodes.go`. No behavioral change.

**Files:** `cmd/tsuku/exitcodes.go`

### Step 2: CLI Flag and Error Handling

Add `--deterministic-only` flag to `create` command. Handle `DeterministicFailedError` in the error path, printing structured output and exiting with code 9.

**Files:** `cmd/tsuku/create.go`

### Step 3: Orchestrator Update

Add `--deterministic-only` to the generate command arguments. Map exit code 9 in `categoryFromExitCode`.

**Files:** `internal/batch/orchestrator.go`

### Step 4: Tests

- Unit test: `create` command with `--deterministic-only` exits 9 on deterministic failure
- Unit test: `categoryFromExitCode(9)` returns `"deterministic_insufficient"`
- Unit test: orchestrator generates with `--deterministic-only` flag

**Files:** `cmd/tsuku/create_test.go`, `internal/batch/orchestrator_test.go`

## Security Considerations

### Download Verification

Not applicable. This change affects error handling and control flow, not how artifacts are downloaded or verified. The deterministic generation path's existing checksum verification (SHA256 against GHCR manifest digests) is unchanged.

### Execution Isolation

Not applicable. No new code execution paths are introduced. The `--deterministic-only` flag prevents LLM fallback, which actually *reduces* the execution surface (no LLM API calls, no generated code from LLM responses).

### Supply Chain Risks

Not applicable. This change doesn't alter where artifacts come from or how they're verified. It changes how failures are reported, not how successes are produced.

### User Data Exposure

Not applicable. The `--deterministic-only` flag prevents LLM API calls, which means no formula data is sent to external LLM providers. Error messages written to stderr contain formula names (public Homebrew data) and failure categories (enum values), neither of which constitutes user data.

## Consequences

### Positive

- Batch pipeline produces structured failure records instead of crashes when deterministic generation fails
- Exit code 9 gives the orchestrator a clean signal without output parsing
- `DeterministicFailedError` data reaches the failure analysis system
- Interactive `tsuku create` is completely unchanged (flag defaults to false)

### Negative

- One more exit code to document and maintain
- `--deterministic-only` is a flag that only automation uses, adding slight surface area to `tsuku create --help`

### Mitigations

- Exit code 9 follows the existing pattern (sequential numbering, documented in exitcodes.go)
- The flag can be hidden from `--help` output via cobra's `MarkHidden` if the help text gets too long, though this isn't necessary yet
