# Scrutiny Review: Intent - Issue #1938

## Focus: Design Intent Alignment + Cross-Issue Enablement

## Sub-check 1: Design Intent Alignment

### BinaryNameProvider Interface

**Design doc (Solution Architecture, section 4):**
> Add an optional interface that builders can implement to provide authoritative binary name data:
> ```go
> type BinaryNameProvider interface {
>     AuthoritativeBinaryNames() []string
> }
> ```

**Implementation** (`internal/builders/binary_names.go:19-25`):
```go
type BinaryNameProvider interface {
    AuthoritativeBinaryNames() []string
}
```

Assessment: Matches the design exactly. The interface is defined in its own file (`binary_names.go`) within the `builders` package, which is appropriate since it's used across builders and the orchestrator. The design didn't specify file placement, and keeping it alongside the `validateBinaryNames()` function that consumes it is clean.

### Orchestrator validateBinaryNames() Step

**Design doc (Solution Architecture, section 3):**
> Add a `validateBinaryNames()` method that runs after `buildRecipe()` and before `validateInSandbox()`. [...] When a mismatch is detected, log a warning, emit a telemetry event (following the pattern in `attemptVerifySelfRepair`), and correct the recipe's executables.

**Implementation** (`internal/builders/binary_names.go:49-130`, `internal/builders/orchestrator.go:168-173`):

The method runs between `session.Generate()` and the sandbox validation loop, matching the design's described sequencing. On mismatch:
- Warning appended to `result.Warnings` (line 113)
- Telemetry event emitted via `telemetry.NewBinaryNameRepairEvent` (lines 116-123)
- Recipe executables corrected in-place (line 98)
- Verify command updated if it references the old first executable (lines 101-107)

The telemetry event follows the same fire-and-forget pattern as `SendVerifySelfRepair` in `internal/telemetry/client.go:158`, matching the design's instruction to follow the pattern in `attemptVerifySelfRepair`.

Assessment: Matches the design intent. The verify command update is an implementation detail not mentioned in the design, but it's a sensible addition -- without it, a corrected binary name would leave the verify command pointing at the old (wrong) name, which would fail sandbox validation.

### Type-Assert Before NewSession

**Design doc (Solution Architecture, section 3):**
> The orchestrator should type-assert the `SessionBuilder` to `BinaryNameProvider` in `Create()` before creating the session, since the builder reference isn't retained after session creation.

**Implementation** (`internal/builders/orchestrator.go:151-153`):
```go
binaryNameProvider, _ := builder.(BinaryNameProvider)
```

This happens at the top of `Create()`, before `builder.NewSession()` on line 156. Matches design intent exactly.

### Cargo and npm BinaryNameProvider Implementations

**Design doc (Implementation Approach, Phase 3):**
> Implement `BinaryNameProvider` on Cargo and npm builders.

**Implementation:**
- `CargoBuilder.AuthoritativeBinaryNames()` at `internal/builders/cargo.go:306`
- `NpmBuilder.AuthoritativeBinaryNames()` at `internal/builders/npm.go:359`

Both use cached API responses (populated during `Build()`) to return names without re-fetching. Both filter through `isValidExecutableName()`. The Cargo implementation reuses the same yanked-version-skipping logic as `discoverExecutables()`. The npm implementation reuses `parseBinField()` with the package name, consistent with the fix from #1937.

Assessment: Matches design intent. The caching pattern (field set during `Build()`, read by the orchestrator later) is exactly what the design describes.

### isValidExecutableName Validation

**Design doc (Security Considerations):**
> The existing `isValidExecutableName()` regex [...] must be applied to every binary name from `bin_names` before it enters a recipe.

**Implementation:** `validateBinaryNames()` filters authoritative names through `isValidExecutableName()` at line 62 of `binary_names.go`. Additionally, `AuthoritativeBinaryNames()` on `CargoBuilder` also filters (cargo.go:319). This is defense-in-depth -- the orchestrator validates regardless of whether the builder already filtered.

Assessment: Matches design intent with an extra safety layer.

### Empty Slice Skips Validation

**Design doc (Solution Architecture, section 4):**
> Builders without metadata (Go, LLM builders) skip this step.

**Implementation:** `validateBinaryNames()` returns nil when `len(authoritative) == 0` (line 55-57). The orchestrator also checks `binaryNameProvider != nil` before calling (orchestrator.go:171). Both paths are tested.

Assessment: Matches design intent.

## Sub-check 2: Cross-Issue Enablement

### Downstream: #1939 (PyPI wheel-based executable discovery)

**#1939 depends on:**
1. **BinaryNameProvider interface** -- Defined at `binary_names.go:19-25`. The interface has a single method `AuthoritativeBinaryNames() []string`. This is what #1939 will implement on `PyPIBuilder`.
2. **validateBinaryNames() orchestrator step** -- Implemented and integrated into `Create()`. When #1939 adds `AuthoritativeBinaryNames()` to `PyPIBuilder`, the orchestrator will automatically pick it up via the type assertion at orchestrator.go:153. No changes to the orchestrator needed.
3. **Telemetry event type** -- `BinaryNameRepairEvent` and `SendBinaryNameRepair` are ready at `internal/telemetry/event.go` and `internal/telemetry/client.go:158`. #1939 doesn't need to add any telemetry code.

Assessment: The foundation is sufficient for #1939. The PyPI builder just needs to:
- Add a `cachedPackageInfo` field (following the Cargo/npm pattern)
- Implement `AuthoritativeBinaryNames()` using wheel data
- Everything else (validation, telemetry, correction) is handled by the existing orchestrator code

No missing interface methods, no missing data structures, no missing orchestrator hooks.

## Sub-check 3: Backward Coherence

### Previous issues: #1936, #1937

**#1936** added `cachedCrateInfo` to `CargoBuilder` and rewrote `discoverExecutables()` to use `bin_names` from the crates.io API. **#1937** added `parseBinField()` string-type handling with `packageName` and `unscopedPackageName`.

#1938 builds on both:
- The `cachedCrateInfo` field from #1936 is read by `AuthoritativeBinaryNames()` on `CargoBuilder`
- The `parseBinField()` function from #1937 is called by `AuthoritativeBinaryNames()` on `NpmBuilder`
- The `cachedPackageInfo` field for `NpmBuilder` follows the same caching pattern established by #1936 for `CargoBuilder`

No contradictions, no renamed patterns, no restructured code.

## Advisory Finding: BinaryNameRepairMetadata Return Value Discarded

The `validateBinaryNames()` method returns `*BinaryNameRepairMetadata`, and it's defined and populated (binary_names.go:125-129). However, in `Create()` (orchestrator.go:172), the return value is discarded:

```go
o.validateBinaryNames(binaryNameProvider, result, builder.Name())
```

By contrast, `VerifyRepairMetadata` has a corresponding field on `BuildResult.VerifyRepair` and is stored at orchestrator.go:232. The `BinaryNameRepairMetadata` has no such field on `BuildResult`.

This doesn't affect correctness -- the recipe is corrected in-place regardless, and the telemetry event fires. But the metadata (old names, new names, builder) is lost to callers of `Create()`. If a caller wanted to log or display what was corrected (beyond the warning string), it couldn't.

This is advisory because no current caller needs this data, and the warning string in `result.Warnings` provides a human-readable summary. Adding the field to `BuildResult` later is a contained change.
