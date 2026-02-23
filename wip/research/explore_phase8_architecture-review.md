# Architecture Review: DESIGN-binary-name-discovery.md

**Reviewer**: architect-reviewer
**Date**: 2026-02-22
**Design**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/designs/DESIGN-binary-name-discovery.md`

## 1. Is the Architecture Clear Enough to Implement?

**Verdict: Mostly yes, with two ambiguities that need resolution before implementation.**

### What is clear

The design correctly identifies the current flow: each builder's `discoverExecutables()` fetches source manifests from GitHub, which breaks for workspace monorepos. The proposed replacement -- reading `bin_names` from the crates.io version API response and fixing `parseBinField()` in npm -- maps directly to specific functions in the codebase:

- `CargoBuilder.discoverExecutables()` at `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go:222`
- `parseBinField()` at `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/npm.go:257`
- The `cratesIOCrateResponse` struct at `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go:28`

The data flow diagram accurately reflects how `Orchestrator.Create()` sequences `session.Generate()` then `o.validate()`, which is exactly how the code works at `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/orchestrator.go:144-260`.

### Ambiguity 1: `bin_names` lives on version objects, but versions are currently opaque

The design says "read `bin_names` from the latest non-yanked version." The current `cratesIOCrateResponse` struct declares `Versions []struct{}` -- an empty struct slice that discards all version-level fields. The design says "a `BinNames` field added" to the response struct, but `bin_names` is a per-version field, not a per-crate field.

The implementer needs to:
1. Change `Versions []struct{}` to a struct that includes `Num string`, `Yanked bool`, and `BinNames []string`.
2. Implement logic to find the latest non-yanked version and read its `bin_names`.
3. This means iterating through the versions array (which can be large for popular crates), or using the version ordering from the API.

The design should specify whether to use the crate's `newest_version` field (if available) or iterate the versions list. This isn't just an implementation detail -- it affects response parsing memory and correctness.

### Ambiguity 2: BinaryNameProvider + DeterministicSession interaction

The design proposes an optional `BinaryNameProvider` interface that builders implement. But deterministic builders use `DeterministicSession` (at `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/builder.go:255`), which wraps a `BuildFunc` closure. The session is what the orchestrator interacts with, not the builder directly.

The orchestrator's `Create()` method receives a `SessionBuilder` and creates a `BuildSession`. It doesn't retain a reference to the `SessionBuilder` after session creation. For the orchestrator to call `AuthoritativeBinaryNames()`, it needs to either:

(a) Type-assert the `SessionBuilder` to `BinaryNameProvider` before creating the session.
(b) Add `BinaryNameProvider` as an optional interface on `BuildSession` instead.
(c) Have the `DeterministicSession` proxy the call through to the builder.

Option (a) is simplest and fits the existing pattern where the orchestrator already receives the builder. The design should specify this.

## 2. Missing Components or Interfaces

### Finding 1 (Advisory): `BinaryNameProvider` data lifecycle

The design says "the builder must cache the API response from `fetchCrateInfo()` so that `AuthoritativeBinaryNames()` can return the data later." Currently, `fetchCrateInfo()` is called in both `CanBuild()` and `Build()` independently -- there's no cross-call caching. The `CargoBuilder` struct holds only `httpClient` and `cratesIOBaseURL`, no state.

Adding cached state to the builder means the builder is no longer stateless between calls. This isn't a structural violation, but it means `CanBuild()` and `Build()` must be called for the same package in sequence. The current flow guarantees this (the orchestrator calls `CanBuild` then `NewSession`/`Generate`), so it works in practice. But the design should call out this statefulness as a constraint, since the builder was previously stateless.

An alternative: have `AuthoritativeBinaryNames()` accept the `BuildResult` and extract the data from the recipe's step params (which already contain the executables list). This avoids adding state to the builder. The orchestrator already has the `BuildResult` from `session.Generate()`.

However, this doesn't serve the cross-checking purpose -- the whole point is to compare the recipe's executables against the authoritative source. The data must come from the registry, not from the recipe.

### Finding 2 (Advisory): npm `parseBinField` fix needs package name passed in

The design correctly identifies that `parseBinField()` needs the package name to handle string-type `bin`. Currently, `parseBinField()` takes only `bin any` -- no package name parameter. The caller `discoverExecutables()` at `npm.go:226` has access to `pkgInfo.Name`, so the fix is to pass it through. This is a minor signature change but it means existing tests (`TestParseBinField` at `npm_test.go:319`) need updating. Clear enough to implement.

### Finding 3: No missing interfaces

The design correctly identifies that `BinaryNameProvider` follows the existing pattern of optional interfaces in the builders package (e.g., `EcosystemProber` at `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/probe.go:14`, `EcosystemDiscoverer` at `probe.go:33`). These are exactly the same pattern: optional capability interfaces that builders can implement, checked via type assertion. No new dispatch or registration mechanism needed.

## 3. Are the Implementation Phases Correctly Sequenced?

**Verdict: Yes, the phasing is sound.**

### Phase 1: crates.io + npm fixes

These are isolated builder changes with no dependencies on other phases. The Cargo builder change modifies `discoverExecutables()` to read from the API response instead of fetching Cargo.toml. The npm fix is a one-line change in `parseBinField()` plus passing the package name. Both can ship independently and immediately fix the known failures.

Dead code cleanup (`buildCargoTomlURL`, `fetchCargoTomlExecutables`, `cargoToml`, `cargoTomlBinSection`) should happen in this phase, not deferred. The design says "remove once the new approach is validated" -- that's fine for a single PR.

### Phase 2: Orchestrator validation

Depends on phase 1 for having at least one `BinaryNameProvider` implementation. The design correctly identifies this. The validation step goes between `session.Generate()` and `o.validate()` in the orchestrator's `Create()` method. This is a clean insertion point.

One concern: the design says "correction is silent." The orchestrator should still emit telemetry (similar to how `attemptVerifySelfRepair` at `orchestrator.go:224` emits telemetry events). Without telemetry, there's no data on how often the safety net catches real mismatches vs. false positives. This doesn't block implementation but should be added to the design.

### Phase 3: PyPI + RubyGems artifact discovery

Correctly deferred. These require downloading `.whl` and `.gem` files -- fundamentally different complexity from reading existing API responses. The `BinaryNameProvider` interface established in phase 2 makes adding these straightforward.

The design mentions reusing "existing `downloadFile()` infrastructure." There is no `downloadFile()` in the builders package; downloads are handled by the `actions` package (`internal/actions/`). The builders would need to implement their own download-and-extract logic, or the design should reference the correct download utility. This is phase 3 scope, so it doesn't block phases 1-2.

## 4. Simpler Alternatives Overlooked?

### Alternative A: Skip the orchestrator validation (phase 2) entirely

If the Cargo builder reads `bin_names` from the API response and the npm builder handles string `bin`, the primary failure cases are fixed. The orchestrator validation adds a cross-cutting safety net, but if no builder produces wrong names after the phase 1 fixes, the safety net catches nothing.

**Assessment**: The orchestrator validation is warranted as defense-in-depth. The pattern matches the existing `attemptVerifySelfRepair` -- the orchestrator already performs post-generation corrections. Adding another correction step is architecturally consistent. The cost is a new interface and a few lines in `Create()`. The benefit is catching future builder bugs before wasted sandbox runs.

### Alternative B: Make `fetchCrateInfo` return a richer response directly

Instead of adding `BinaryNameProvider` to the builder, have `fetchCrateInfo` return a struct that includes `bin_names`, and have `discoverExecutables` use it. No new interface, no cached state, no orchestrator changes. The builder just produces the right executables in phase 1.

**Assessment**: This is what phase 1 already does. Phase 2 (the orchestrator validation) is the additive part. Alternative B amounts to "do phase 1 only" which is viable for the known cases but doesn't establish the safety net pattern.

### Alternative C: Use the crates.io sparse index instead of the full API

The crates.io sparse index (`index.crates.io`) provides crate metadata without the full API overhead. However, the sparse index does not include `bin_names` -- it only has dependency and feature data. The full API at `/api/v1/crates/{name}` is required.

**Assessment**: Not a viable alternative. The design's choice of the version API endpoint is correct.

## 5. Structural Fit Assessment

### Positive structural alignment

1. **`BinaryNameProvider` follows the `EcosystemProber`/`EcosystemDiscoverer` pattern.** Optional interface, checked via type assertion, implemented by builders that have the relevant data. This is the established extensibility pattern in the builders package.

2. **Orchestrator validation sits in the same control flow as existing repair logic.** The `attemptVerifySelfRepair` at `orchestrator.go:338` already modifies recipes between generation and sandbox validation. Adding `validateBinaryNames()` in the same position is structurally consistent.

3. **No new packages or dependencies introduced.** All changes are within `internal/builders/`. No dependency direction issues.

4. **The design removes code (Cargo.toml fetching) rather than adding a parallel path.** The old repository-based approach is explicitly replaced, not retained alongside the new one.

### One structural concern (Advisory)

The design introduces cached state in the `CargoBuilder` (to store the API response between `Build()` and `AuthoritativeBinaryNames()`). Currently all ecosystem builders are stateless between method calls. This isn't a blocking issue because:
- The builder is not shared across concurrent requests.
- The orchestrator's `Create()` calls `CanBuild()`, `NewSession()`, `Generate()` in sequence for a single package.
- No other caller assumes builders are stateless.

But it's worth noting as a precedent: future builders shouldn't assume they can cache arbitrarily across calls.

## Summary of Findings

| # | Finding | Severity | Action |
|---|---------|----------|--------|
| 1 | `bin_names` is per-version, not per-crate; design needs to specify version iteration/selection strategy since `Versions` is currently `[]struct{}` | Advisory | Clarify in design: use the version matching the resolved version (first non-yanked) |
| 2 | `BinaryNameProvider` interaction with `DeterministicSession` unspecified; orchestrator needs access to the builder (not just session) to call `AuthoritativeBinaryNames()` | Advisory | Specify: type-assert `SessionBuilder` to `BinaryNameProvider` in `Create()` before session creation |
| 3 | Design should recommend telemetry emission for binary name corrections, matching the `attemptVerifySelfRepair` pattern | Advisory | Add telemetry event to design |
| 4 | Reference to "`downloadFile()` infrastructure" in phase 3 is incorrect; no such function exists in builders | Advisory | Correct reference for phase 3 (not blocking since phase 3 is deferred) |
| 5 | Phase sequencing is correct | -- | No action |
| 6 | `BinaryNameProvider` matches `EcosystemProber` extensibility pattern | -- | No action (positive alignment) |

No blocking issues found. The design fits the existing architecture and can proceed to implementation with the advisory clarifications above.
