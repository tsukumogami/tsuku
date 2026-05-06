# Architect Review: tsuku-homebrew-dylib-chaining (revised design)

Reviewer focus: structural fit, layering, interface contracts, dependency
direction. Run after the round-2 design reversal and the Phase 5 security
re-run, before the design transitions toward `/plan`.

## Verdict

**Concerns to address before plan.** Five concrete structural gaps; none
require a redesign loop. All are addressable with localized edits to the
design document.

## Findings

### 1. SONAME index layering is inverted

The design table places `soname_index.go` in `internal/install/` and the
data flow says the index is built "at plan-generation time." But
`internal/executor/plan_generator.go` does not import `internal/install/`
today — the executor is the higher-level orchestrator, and `install`
sits above the plan generator in the dependency graph. Putting the
index in `internal/install/` would invert the existing direction.

**Fix.** Place the index in a leaf package (`internal/sonameindex/`)
that both the executor (for plan-time index construction) and the
homebrew action (for install-time lookup) can import without inverting
direction. The design doc's Solution Architecture row updated to reflect
this.

### 2. `runtime_dependencies` dual-consumer contract should be pinned

The wrapper-PATH consumer reads the field as "deps to add to PATH"; the
new chain consumer reads it as "deps whose lib/ should be in RPATH."
Both consumers fit the author-facing semantic ("needed at runtime"), but
the engine code now has two paths reading the same field. A future
maintainer reading either consumer in isolation needs to know about the
other.

**Fix.** Phase 1 acceptance now requires a docstring on the
`RuntimeDependencies` field that enumerates its consumers. The contract
pin is in code, not just in this design doc.

### 3. `fixDylibRpathChain` is overloaded

The pre-rename function (`fixLibraryDylibRpaths`) walks
`ctx.Dependencies.InstallTime` using **absolute** paths and adds a bare
`@loader_path`. The post-rename function will walk `RuntimeDeps` using
**relative** paths. These are different semantics under the same name.

**Fix.** The renamed function reads `RuntimeDeps` (and the local
auto-included slice) only — it does not walk `InstallTime`. The
library-recipe install-time chain (which used `InstallTime`) migrates to
relative paths in a separate Phase 3 sub-step with golden-fixture
coverage so the absolute → relative shift is reviewable in isolation.

### 4. Auto-include data path was unspecified

The SONAME scanner runs in Execute and produces auto-included entries.
The chain walk reads `ctx.Dependencies.RuntimeDeps`, populated at plan
time. Mutating `ctx` from inside Execute would be a layering smell.

**Fix.** The design now states explicitly that the SONAME scanner
produces a local `[]chainEntry` slice in Execute scope. The chain walk
iterates `RuntimeDeps ∪ autoIncluded` without touching `ctx`. `ctx` is
read-only inside Execute.

### 5. Order conflict between `fixMachoRpath`/`fixElfRpath` and the new chain

The existing per-binary pass (`fixMachoRpath`, `fixElfRpath`) calls
`--remove-rpath` to wipe stale entries before setting the `@rpath` /
`$ORIGIN` anchor. If the new chain walk runs **before** that pass, the
chain entries get wiped.

**Fix.** Phase 3 and Phase 4 now state explicitly that the chain step
runs **after** the per-binary `fixMachoRpath`/`fixElfRpath` pass.
Acceptance criteria include "chain entries survive the existing
per-binary relocate pass."

### 6. Phase 7 noise control

Phase 5's "no candidate" log line will fire at every install of `git`,
`wget`, `coreutils`, etc., until the missing library recipes
(`libuuid`, `libacl`, `libattr`) land. Without a noise-control
mechanism, install logs grow noisy.

**Fix.** Phase 2 now includes a small static known-gap allowlist that
downgrades the log line to debug-level for entries on the list.
Entries are removed when the corresponding library recipe is authored,
so the mechanism self-clears.

## Out of scope, correctly deferred

- Authoring missing library recipes (was Phase 7 in the design; now
  framed as follow-up work). One small PR per recipe.
- `tsuku doctor` RPATH walk. Already noted in Consequences as a
  follow-up.
- Phase 5 caching by bottle sha256. Mentioned in Negative Consequences;
  not blocking.

## YAML summary

```yaml
outcome: concerns_addressed_before_plan
findings_count: 6
files_referenced:
  - docs/designs/DESIGN-tsuku-homebrew-dylib-chaining.md
  - internal/actions/homebrew_relocate.go
  - internal/executor/plan_generator.go
```
