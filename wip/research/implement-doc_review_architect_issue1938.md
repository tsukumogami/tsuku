# Architectural Review: #1938 BinaryNameProvider and Orchestrator Validation

**Reviewer**: architect-reviewer
**Commit**: 0fd9d7f7b3b41c5ce18920f892f8546c0e6f8f0b
**Branch**: docs/deterministic-recipe-repair

---

## Summary

This change adds a `BinaryNameProvider` optional interface that ecosystem builders can implement to expose registry-authoritative binary names. The orchestrator type-asserts to this interface after generation and before sandbox validation, correcting recipe executables when they diverge from registry metadata. Cargo and npm builders implement the interface; Go, PyPI, and LLM builders do not (by design).

The change touches the builders package (`internal/builders/`) and the telemetry package (`internal/telemetry/`). No CLI surface, state contract, action dispatch, or provider registry changes.

**Blocking findings**: 1
**Advisory findings**: 2

---

## Findings

### 1. ADVISORY: `BinaryNameRepairMetadata` is returned but never stored or consumed

**Location**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/orchestrator.go:172`

```go
if binaryNameProvider != nil {
    o.validateBinaryNames(binaryNameProvider, result, builder.Name())
}
```

`validateBinaryNames()` returns `*BinaryNameRepairMetadata`, but the return value is discarded. Compare with the existing `VerifyRepair` field pattern: `VerifyRepairMetadata` is stored in `BuildResult.VerifyRepair` (line 233 of orchestrator.go: `result.VerifyRepair = verifyMeta`) and `BuildResult` has a dedicated field for it.

`BinaryNameRepairMetadata` has no equivalent field on `BuildResult`. The struct is defined, tested, and returned -- but nothing upstream can read it. This is currently a dead return value.

This isn't blocking because the metadata isn't needed by any existing consumer today, and the correction side effect (recipe mutation + warning + telemetry) works correctly. But it breaks the parallel with `VerifyRepairMetadata` and will need a `BuildResult.BinaryNameRepair *BinaryNameRepairMetadata` field when the create command or batch-generate pipeline wants to report what was corrected.

**Suggestion**: Either add the field to `BuildResult` now (consistent with `VerifyRepair`), or document the return value as "reserved for future use" so the next PR doesn't re-discover this gap.

---

### 2. ADVISORY: Telemetry `SendBinaryNameRepair` continues the per-type method proliferation

**Location**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/telemetry/client.go:158-171`

The telemetry `Client` now has 5 typed Send methods: `Send(Event)`, `SendLLM(LLMEvent)`, `SendDiscovery(DiscoveryEvent)`, `SendBinaryNameRepair(BinaryNameRepairEvent)`, `SendVerifySelfRepair(VerifySelfRepairEvent)`. Each method is a copy-paste with only the event type changed.

This is an existing pattern in the codebase -- `SendVerifySelfRepair` (from the previous PR) established it. Adding one more isn't a new divergence. But note that a single `SendAny(event interface{})` method already exists as the private `sendJSON`. The typed methods add no compile-time safety because each struct already has correct JSON tags, and the methods don't validate anything type-specific.

Not blocking because: (a) the pattern was already established before this PR, and (b) collapsing the Send methods is a separate refactor that touches all callers.

---

### 3. BLOCKING: Cargo `AuthoritativeBinaryNames()` duplicates `discoverExecutables()` logic without fallback parity

**Location**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go:306-327`

`AuthoritativeBinaryNames()` reimplements the "find first non-yanked version, filter bin_names" logic from `discoverExecutables()` (lines 220-259), but diverges in an important way: when `discoverExecutables()` finds an empty `binNames` slice for a non-yanked version, it falls back to the crate name (line 256: `return []string{crateInfo.Crate.Name}, warnings`). `AuthoritativeBinaryNames()` in the same scenario returns an empty `[]string{}` (it does `return names` where `names` is nil from no appends).

This behavioral divergence means: when a library crate has no bin_names, `discoverExecutables()` produces `["crate-name"]` as the recipe executable, but `AuthoritativeBinaryNames()` returns `nil`/empty. The `validateBinaryNames` method then skips validation entirely (line 55-57: `if len(authoritative) == 0 { return nil }`), which is the correct behavior for the "no data" case.

However, the divergence is accidental and fragile. If `discoverExecutables` changes its fallback logic, `AuthoritativeBinaryNames` won't follow. The two methods share a data source (the cached `cratesIOCrateResponse`) and a filtering algorithm but are separate implementations. The same issue exists in the npm builder: `AuthoritativeBinaryNames()` (lines 359-376) re-derives the bin field from cached data with different edge-case handling than `discoverExecutables()`.

**Why blocking**: This is a parallel pattern for the same concern (extracting bin names from registry metadata). When #1939 adds PyPI support, the implementer will see two templates: `discoverExecutables` (used during Build) and `AuthoritativeBinaryNames` (used post-Build for validation). They'll need to decide which to follow and risk duplicating the divergence. The pattern should be: extract bin names once, store them, return them from both paths.

**Suggestion**: Extract a `registryBinNames(crateInfo) []string` helper that both `discoverExecutables` and `AuthoritativeBinaryNames` call for the "raw registry data" portion. `discoverExecutables` adds the fallback-to-crate-name logic on top. `AuthoritativeBinaryNames` returns the raw result (empty means no data). This eliminates the second implementation and makes the contract clear: authoritative names are the raw registry data; discovery adds fallbacks.

---

## Structural Assessment

### What fits well

1. **Optional interface via type assertion**: Using `BinaryNameProvider` as an optional interface that builders opt into is the right Go pattern. It doesn't pollute `SessionBuilder` with methods that only some builders can implement. The type assertion in `Create()` is clean.

2. **Orchestrator ownership**: The validation logic lives in the orchestrator, not in individual builders. This keeps the "generate -> validate -> repair" responsibility centralized. Builders just expose data; the orchestrator decides what to do with it.

3. **Placement in the build pipeline**: Running binary name validation after `Generate()` but before sandbox validation is architecturally correct. It avoids wasting a container cycle on a recipe with known-wrong executables.

4. **No dependency direction violations**: `binary_names.go` imports `recipe` and `telemetry`, both of which are lower-level packages. The `builders` package already depends on both. No new edges in the dependency graph.

5. **Telemetry integration**: Follows the established pattern exactly (create event, call typed Send method, fire-and-forget).

### PyPI extensibility (#1939)

The current PyPI builder (`pypi.go`) discovers executables from `pyproject.toml` via GitHub raw content, not from the PyPI JSON API directly. PyPI's JSON API doesn't expose a `bin` or `scripts` field. So for #1939, implementing `BinaryNameProvider` on `PyPIBuilder` would mean:

- Caching the `pypiPackageResponse` (like Cargo/npm do)
- Fetching and parsing `pyproject.toml` again in `AuthoritativeBinaryNames()`, or caching the executables discovered during `Build()`

The simplest path is to cache the discovered executables during `Build()` and return them from `AuthoritativeBinaryNames()`. This avoids the duplication problem flagged in Finding 3. If the pattern is fixed for Cargo/npm first, PyPI can follow the clean pattern.

### Gem/CPAN/Go extensibility

These builders don't have registry-authoritative binary name data:
- **Go**: The Go module proxy doesn't expose binary names
- **Gem**: RubyGems API has an `executables` field -- this could implement `BinaryNameProvider` in a future PR
- **CPAN**: MetaCPAN might expose script names; unclear

The optional interface pattern makes it trivial to add support for these without touching existing code.
