---
status: Current
problem: The Homebrew builder declares RequiresLLM()=true and fails when deterministic generation can't fall back to LLM, blocking the batch pipeline which must operate without API keys at $0 cost.
decision: Add a DeterministicOnly session option that prevents LLM fallback and returns structured DeterministicFailedError on failure, with failure categories matching the failure-record schema.
rationale: The builder already tries deterministic first. The change is small (new option, new error type, conditional in Generate) and doesn't alter the existing interactive flow. Structured errors feed directly into the batch pipeline's failure analysis system.
---

# DESIGN: Homebrew Deterministic Mode

## Status

Current

## Upstream Design Reference

This design implements part of [DESIGN-registry-scale-strategy.md](DESIGN-registry-scale-strategy.md).

**Relevant sections:**
- Decision 1: Fully deterministic batch generation
- Builder Gaps: "Homebrew Deterministic Success Rate"
- M-HomebrewBuilder milestone: Issue #1188

**This design must deliver:**
- Homebrew builder API for deterministic-only mode (required by #1189)

## Context and Problem Statement

The Homebrew builder generates recipes from Homebrew formulas. It works in two stages: first, it inspects bottle tarballs from GHCR to extract binary names deterministically (no LLM, $0 cost). When that fails, it falls back to an LLM conversation that analyzes the formula.

The batch pipeline (#1189) needs to run Homebrew generation at scale without any LLM API key. The pipeline produces failure records when generation doesn't work, feeding a gap analysis system that identifies which capabilities to build next.

Three problems block this:

1. **`RequiresLLM()` returns `true` unconditionally.** Callers that filter builders by this flag skip Homebrew entirely in no-LLM environments, even though most formulas succeed deterministically.

2. **No structured failure type.** When deterministic generation fails, the error is a generic `fmt.Errorf`. The batch pipeline needs failure categories (no_bottles, missing_dep, complex_archive, etc.) matching `failure-record.schema.json` to populate failure records.

3. **No way to prevent LLM fallback.** In interactive mode, falling back to LLM is useful. In batch mode, it must not happen. There's no session option to control this.

### Scope

**In scope:**
- `DeterministicOnly` session option to suppress LLM fallback
- `DeterministicFailedError` type with category field matching failure schema
- Adjusting `RequiresLLM()` to reflect actual LLM dependency
- Fixing the nil dereference at homebrew.go:430 (`s.provider.Name()` before provider init)

**Out of scope:**
- Improving deterministic success rate (separate issue)
- Batch pipeline integration (downstream #1189)
- Changes to other builders

## Decision Drivers

- Batch pipeline must run without LLM API keys at $0 cost
- Failure categories must match `failure-record.schema.json` enum values
- Existing interactive flow (deterministic + LLM fallback + repair) must keep working unchanged
- SessionBuilder interface contract shouldn't change
- The change should be small since the deterministic path already exists

## Considered Options

### Decision 1: How to Signal Deterministic-Only Mode

How does the caller tell the builder to skip LLM fallback?

#### Option 1A: Session Option Flag

Add `DeterministicOnly bool` to `SessionOptions`. The builder checks this during `Generate()` and returns an error instead of falling back.

**Pros:**
- Per-session control: same builder instance serves both interactive and batch callers
- No new interfaces or builder types
- Matches existing pattern (SessionOptions already has LLMConfig, ForceInit, etc.)

**Cons:**
- Adds a field to a shared options struct
- Builder must check the flag in multiple places (Generate and Repair)

#### Option 1B: Separate Builder Type

Create `HomebrewDeterministicBuilder` that wraps `HomebrewBuilder` and never initializes LLM.

**Pros:**
- Clean separation between deterministic and LLM paths
- `RequiresLLM()` returns false naturally

**Cons:**
- Duplicates builder registration and configuration
- Harder to maintain as two builders diverge
- Over-engineers a one-flag change

#### Option 1C: Constructor Option

Add `WithDeterministicOnly()` to `HomebrewBuilder` constructor, making it a builder-level setting.

**Pros:**
- Set once at construction, can't change per-session

**Cons:**
- Can't share builder instance between interactive and batch callers
- Forces separate builder instances for different modes

### Decision 2: How to Report Deterministic Failures

What error type should be returned when deterministic generation fails?

#### Option 2A: New DeterministicFailedError with Category

Create a new error type carrying a failure category from a predefined enum matching the failure schema.

**Pros:**
- Callers can type-assert to get structured failure data
- Category field maps directly to failure-record.schema.json
- Clear contract: batch pipeline knows exactly what to expect

**Cons:**
- New error type to maintain
- Categories must stay in sync with failure schema

#### Option 2B: Wrap Existing Errors with Category Metadata

Use Go error wrapping to add category context to existing errors (HomebrewNoBottlesError, etc.).

**Pros:**
- No new error types
- Leverages existing error hierarchy

**Cons:**
- Category extraction requires unwrapping chains, fragile
- Existing errors don't all map cleanly to failure schema categories
- Harder for callers to get structured data

### Decision 3: How to Handle RequiresLLM()

How should the builder report its LLM dependency?

#### Option 3A: Return false (Builder Never Requires LLM)

Change `RequiresLLM()` to return `false`. The builder can work without LLM. It just produces different results (success or structured failure vs. success with LLM fallback).

**Pros:**
- Accurate: the builder doesn't _require_ LLM; it optionally uses it
- Batch pipeline sees Homebrew as eligible without special-casing

**Cons:**
- Callers that gate on `RequiresLLM()` to warn about LLM cost no longer warn for Homebrew
- Changes the semantics of a method used by multiple callers

#### Option 3B: Keep true, Add CanRunDeterministic() Method

Keep `RequiresLLM() = true` but add a new method `CanRunDeterministic() bool` to the interface.

**Pros:**
- Backward compatible
- Callers that check RequiresLLM still get accurate "may use LLM" info

**Cons:**
- Adds to the SessionBuilder interface
- All builders need to implement it

#### Option 3C: Keep true, Batch Pipeline Handles Differently

Don't change the method. The batch pipeline ignores `RequiresLLM()` for Homebrew since it knows to set `DeterministicOnly`.

**Pros:**
- Zero changes to interface or existing behavior
- Simplest implementation

**Cons:**
- Batch pipeline needs Homebrew-specific knowledge
- `RequiresLLM()` is technically inaccurate

### Uncertainties

- The 85-90% deterministic success rate is estimated, not measured. Actual success rate will be known after the batch pipeline runs real data.
- Some failure categories (complex_archive) may not occur for Homebrew specifically, but we include them for schema consistency.

## Decision Outcome

**Chosen: 1A + 2A + 3A**

### Summary

Add a `DeterministicOnly` field to `SessionOptions`, a new `DeterministicFailedError` type with a category enum, and change `RequiresLLM()` to return `false`. When `DeterministicOnly` is set, `Generate()` skips LLM fallback and returns `DeterministicFailedError` on failure. `Repair()` returns `RepairNotSupportedError`. The interactive flow is unchanged when the option isn't set.

### Rationale

Option 1A (session flag) keeps a single builder serving both modes, matching how `SessionOptions` already gates LLM behavior (via `LLMConfig`). Option 2A (new error type) gives the batch pipeline a clean type assertion with the category field it needs. Option 3A (RequiresLLM=false) is the most accurate answer: the builder works without LLM. The interactive `create` command already checks for LLM availability separately through `CheckLLMPrerequisites`.

Option 1B was rejected because maintaining two builder types for one flag is unnecessary. Option 3B was rejected because it adds interface surface for a problem better solved by fixing the existing method's return value.

### Trade-offs Accepted

- Callers that used `RequiresLLM()` to decide whether to warn about LLM costs will no longer warn for Homebrew. This is acceptable because the warning should be based on whether LLM will actually be used, not whether the builder can use it. The `create` command already has separate LLM configuration checks.
- `DeterministicFailedError` categories must stay in sync with `failure-record.schema.json`. This coupling is intentional and desirable since the error feeds directly into failure records.

## Solution Architecture

### Overview

The change touches three areas of the Homebrew builder: session options, the Generate/Repair flow, and the error type system.

### New Error Type

```go
// DeterministicFailedError indicates deterministic generation failed
// and LLM fallback was either unavailable or suppressed.
type DeterministicFailedError struct {
    Formula  string              // Homebrew formula name
    Category DeterministicFailureCategory // Failure classification
    Message  string              // Human-readable template-based description (no internal paths)
    Err      error               // Underlying error (for logging only, not for failure records)
}

func (e *DeterministicFailedError) Error() string { ... }
func (e *DeterministicFailedError) Unwrap() error { return e.Err }

// DeterministicFailureCategory classifies why deterministic generation failed.
// Values match the category enum in failure-record.schema.json.
type DeterministicFailureCategory string

const (
    FailureCategoryNoBottles     DeterministicFailureCategory = "no_bottles"
    FailureCategoryMissingDep    DeterministicFailureCategory = "missing_dep"
    FailureCategoryBuildFromSrc  DeterministicFailureCategory = "build_from_source"
    FailureCategoryComplexArchive DeterministicFailureCategory = "complex_archive"
    FailureCategoryAPIError      DeterministicFailureCategory = "api_error"
    FailureCategoryValidation    DeterministicFailureCategory = "validation_failed"
)
```

The `Message` field uses fixed templates (e.g., "formula %s has no bottles available") rather than passing through raw internal error strings. Internal errors go in `Err` for logging; `Message` goes into failure records. `Unwrap()` supports `errors.Is`/`errors.As` chains, matching the convention used by other error types in `errors.go`.

### Modified Session Options

```go
// In SessionOptions (builder.go):
type SessionOptions struct {
    // ... existing fields ...
    DeterministicOnly bool // When true, skip LLM fallback; return DeterministicFailedError on failure
}
```

### Modified Generate Flow

```go
func (s *HomebrewSession) Generate(ctx context.Context) (*BuildResult, error) {
    result, err := s.generateDeterministic(ctx)
    if err == nil {
        return result, nil
    }

    // In deterministic-only mode, don't fall back to LLM
    if s.deterministicOnly {
        return nil, s.classifyDeterministicFailure(err)
    }

    // Existing LLM fallback path (unchanged)
    return s.generateBottle(ctx)
}
```

### Modified Repair Flow

`RepairNotSupportedError` already exists in `builder.go` (used by `DeterministicSession` for ecosystem builders). We reuse it here:

```go
func (s *HomebrewSession) Repair(ctx context.Context, failure *sandbox.SandboxResult) (*BuildResult, error) {
    if s.deterministicOnly {
        return nil, &RepairNotSupportedError{BuilderType: "homebrew-deterministic"}
    }
    // ... existing repair logic unchanged ...
}
```

### Nil Dereference Fix

Line 430 in `Generate()` calls `s.provider.Name()` before `ensureLLMProvider()` runs. The provider is nil until LLM fallback initializes it. Fix: move the progress message into `generateBottle()` after `ensureLLMProvider()` succeeds, or use a static string like "LLM" as placeholder.

### Failure Classification

The `classifyDeterministicFailure` method maps internal errors to schema categories:

| Internal Error | Category | When |
|---------------|----------|------|
| No bottles available | `no_bottles` | Formula has `bottles: false` or no bottle for the target platform |
| No binaries in bottle | `complex_archive` | Bottle exists but no files in `bin/` |
| Missing tsuku deps | `missing_dep` | Formula needs deps without recipes |
| GHCR/API request failed | `api_error` | Network or auth error fetching bottle |
| Sandbox validation failed | `validation_failed` | Recipe generated but didn't pass sandbox |

### Key Interfaces (Unchanged)

`SessionBuilder` and `BuildSession` interfaces stay the same. `RequiresLLM()` changes its return value but not its signature. The orchestrator calls `Generate()` and `Repair()` as before.

### Data Flow

```
Batch Pipeline                    Homebrew Builder
     |                                 |
     |-- NewSession(req, opts{         |
     |     DeterministicOnly: true}) ->|
     |                                 |-- fetchFormulaInfo()
     |                                 |-- return session
     |                                 |
     |-- session.Generate(ctx) ------->|
     |                                 |-- generateDeterministic()
     |                                 |   |-- success? return result
     |                                 |   |-- fail? classifyDeterministicFailure()
     |                                 |      return DeterministicFailedError{
     |                                 |        Category: "no_bottles"
     |<--- error --------------------- |      }
     |                                 |
     |-- (records failure to JSONL) ---|
```

## Implementation Approach

### Phase 1: Error Type and Session Option

Add `DeterministicFailedError`, `DeterministicFailureCategory` constants, and `DeterministicOnly` field to `SessionOptions`. This is additive with no behavioral change.

**Files:** `internal/builders/errors.go`, `internal/builders/builder.go`

### Phase 2: Generate/Repair Guard and Classification

Add `deterministicOnly` field to `HomebrewSession`, set from `SessionOptions` in `NewSession`. Add the classification method. Guard `Generate()` and `Repair()` to check the flag. Fix the nil dereference at line 430.

**Files:** `internal/builders/homebrew.go`

### Phase 3: RequiresLLM Change

Change `HomebrewBuilder.RequiresLLM()` to return `false`. Update tests that assert `RequiresLLM() == true`.

**Files:** `internal/builders/homebrew.go`, `internal/builders/homebrew_test.go`

### Phase 4: Tests

Add test cases for:
- `Generate()` with `DeterministicOnly=true` returns `DeterministicFailedError` when deterministic fails
- `Repair()` with `DeterministicOnly=true` returns `RepairNotSupportedError`
- `Generate()` without `DeterministicOnly` still falls back to LLM (existing behavior)
- `DeterministicFailedError` fields populated correctly
- `RequiresLLM()` returns `false`

**Files:** `internal/builders/homebrew_test.go`

## Security Considerations

### Download Verification

The deterministic path downloads bottle tarballs from GHCR (`ghcr.io/v2/homebrew/core/`) using anonymous tokens. SHA256 checksums are computed during download and compared against manifest digests. This verification already exists in `listBottleBinaries` and is unchanged by this design.

### Execution Isolation

No change. The deterministic path inspects tarball contents without executing anything. Generated recipes are validated by the sandbox (in interactive mode) or skipped (in deterministic-only mode where validation is the batch pipeline's responsibility).

### Supply Chain Risks

Bottles come from Homebrew's official GHCR registry, verified by content-addressable SHA256 digests. The deterministic path doesn't introduce new supply chain surface beyond what `generateDeterministic` already uses. The batch pipeline adds its own validation gates downstream.

### User Data Exposure

No change. The deterministic path accesses the Homebrew JSON API (public, anonymous) and GHCR (public, anonymous token). No user data is sent. In deterministic-only mode, no LLM API calls are made, so no formula data is sent to LLM providers.

### Mitigations

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Compromised bottle on GHCR | SHA256 verification against manifest digest | GHCR itself compromised (out of scope) |
| GHCR token scope creep | Anonymous token only, no write permissions | None |
| Failure category leaking internal details | Category is enum value, message is human-readable | Message could reveal internal paths; keep messages generic |

## Consequences

### Positive

- Batch pipeline can run Homebrew generation without LLM API keys
- Failures produce structured data for gap analysis
- `RequiresLLM()` accurately reflects the builder's actual requirements
- Fixes existing nil dereference bug at homebrew.go:430

### Negative

- `DeterministicFailureCategory` constants are coupled to `failure-record.schema.json` categories. If the schema adds categories, the Go constants must be updated.
- Callers checking `RequiresLLM()` to show LLM cost warnings won't warn for Homebrew.

### Mitigations

- Schema validation scripts (from #1201) can verify Go constants match JSON schema categories as a CI check.
- The `create` command uses `CheckLLMPrerequisites` independently, so LLM cost warnings still work for interactive usage.
