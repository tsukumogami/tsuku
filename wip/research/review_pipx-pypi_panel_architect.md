# Architect review: pipx PyPI version pinning (#2331)

Branch: `docs/2331-pipx-pypi-version-pinning`
Commits: 19f2ca59, 576106fb, 25100a95
Design: `docs/designs/DESIGN-pipx-pypi-version-pinning.md`
Plan: `docs/plans/PLAN-pipx-pypi-version-pinning.md`

This review focuses on structure, not correctness or readability.

## 1. Layering and dependency direction

Dependencies flow correctly downward:

- `internal/version/pep440/` is a leaf package (stdlib-only). It is imported by `internal/version/provider_pypi.go` and nothing in `pep440/` reaches up. Layering OK.
- `internal/executor/executor.go` imports `internal/version` and `internal/actions` (it always did). The new `resolveBundledPythonMajorMinor` only reads `actions.ResolvePythonStandalone()` and `actions.GetPythonVersion(...)` — both already in the actions package. No new edge added.
- The factory's pipx-aware path is new: `Executor.resolveVersionWith` → `factory.ProviderFromRecipeForPipx(...)` → `PyPISourceStrategy.CreateWithPython` → `NewPyPIProviderForPipx`. Direction is clean (executor → factory → provider). No inversion.

The `PipxAwareStrategy` extension interface is declared in `provider_factory.go` next to `ProviderStrategy` (same file, same package) and is resolved via a type assertion in `ProviderFromRecipeForPipx`. Strategies that don't implement it fall through unchanged. This respects open/closed: existing strategies need no edits to keep working.

**Verdict: layering is sound.**

## 2. Interface contracts

### `NewPyPIProvider` vs `NewPyPIProviderForPipx`

The dual constructor is a defensible shape, but it's the second-best option for this codebase. The existing pattern across providers is the *single constructor with extra parameters* form (e.g., `NewGitHubProvider(resolver, repo, stableQualifiers)` accepts a list that can be `nil` for defaults; `NewGitHubProviderWithPrefix` exists, so dual-constructor precedent does exist). A future third axis (e.g., index URL override, mirror prefix) would add `NewPyPIProviderForPipxWithMirror`, which doesn't compose.

The cleanest fit would be a single `NewPyPIProvider(resolver, packageName, opts ...PyPIOption)` with `WithPythonMajorMinor("3.10")`. That's a refactor, not a blocker — call sites are concentrated in the two factory strategies. **Advisory.**

### `PipxAwareStrategy` as an extension interface

The current shape (a sibling interface checked via type assertion) is acceptable but parallel to `ProviderStrategy`. It works because exactly two strategies need pipx awareness today (`PyPISourceStrategy`, `InferredPyPIStrategy`); a third PyPI-aware strategy would just implement the interface.

The alternative — a method `Create(resolver, r, ctx ResolutionContext) (VersionProvider, error)` where `ResolutionContext` carries optional pythonMajorMinor — would unify both code paths into `ProviderFromRecipe`, eliminating the parallel `ProviderFromRecipeForPipx`. Today the factory has *two* entry points doing nearly identical work. This is the kind of duplication that compounds: the next provider-side context (e.g., libc family for native-extension builds) will either thread through a new `ProviderFromRecipeForLibc` or be retrofitted. **Advisory** — flag for follow-up before a third context type appears.

## 3. Cache-key invariant

**Blocking issue.** The design's central correctness argument is:

> "The probe must happen at version-resolution time (not at `Decompose` time) because the version produced by resolution is the install plan's cache key; if the filter ran later, the cache key and the installed version would diverge."

The code at `internal/executor/executor.go:95-146` partially honors this:

- The probe runs inside `resolveVersionWith`, before `factory.ProviderFromRecipeForPipx` is called. So when the filter fires, the filtered version is what flows into `e.version`, `vars["version"]`, and `evalCtx.Version`. That part is correct.
- **But** `resolveBundledPythonMajorMinor` is best-effort: if `python-standalone` is not installed, `actions.ResolvePythonStandalone()` returns `""` and the function returns `""`. The factory then constructs a non-filtering provider and `ResolveLatest` returns the absolute-latest version (e.g., 2.20.5 for ansible-core).

The design explicitly required:

> "the executor obtains that major.minor by ensuring `python-standalone` is installed (via the existing eval-deps mechanism, surfaced earlier in plan generation for `pipx_install` recipes) and probing the resolved binary via the existing `getPythonVersion` helper before constructing the provider"

> "Phase 6 architecture review caught that filtering in `Decompose` creates a cache-key divergence (the install plan's cache key is born from `ResolveLatest`, before `Decompose` runs). Resolution: filter moved back into `PyPIProvider.ResolveLatest`, with the bundled Python's major.minor supplied by the executor as a constructor field. The probe still uses the existing `getPythonVersion` helper; the executor calls it after surfacing `python-standalone`'s eval-dep check earlier in plan generation."

The implementation does **not** call `actions.GetEvalDeps("pipx_install")` and `actions.CheckEvalDeps(...)` before resolution. The `OnEvalDepsNeeded` callback is invoked only later, inside `resolveStep` (line 331-342 of `plan_generator.go`), which runs *after* `e.resolveVersionWith` (line 139).

Concrete divergence path:

1. Clean machine, no python-standalone installed.
2. `tsuku eval --recipe recipes/a/ansible.toml`.
3. `resolveBundledPythonMajorMinor` returns `""` (binary missing).
4. `ProviderFromRecipeForPipx` falls through to `Create` (no filter).
5. `ResolveLatest` returns 2.20.5 (absolute latest, declares `>=3.12`).
6. Plan generation continues; eventually `resolveStep` for the pipx_install action runs `OnEvalDepsNeeded`, installing python-standalone (3.13.x).
7. Decompose calls `pip download ansible-core==2.20.5` against Python 3.13 — succeeds (3.13 satisfies `>=3.12`).
8. **Cache key is 2.20.5; this is the wrong version (not what pip would pick on this Python)** — but since 2.20.5 happens to be 3.13-compatible the install proceeds, just to the wrong release.

If the bundled Python were 3.10 (or python-standalone is mid-update), step 7 would fail with the original `pip download` error — the design's explicit failure mode. The pre-flight typed `ErrTypeNoCompatibleRelease` never fires.

The docstring at `executor.go:117-121` *acknowledges* this:

```
// or when the binary is not yet installed (the existing CheckEvalDeps
// flow inside resolveStep will install it later, and resolution will
// fall back to absolute-latest for this run; subsequent runs probe
// successfully).
```

That's the cache-key divergence the design said this approach would prevent. The first run's plan records 2.20.5 even when filtering would pick 2.17.14 on the just-installed Python. Subsequent runs see the cached state, but a fresh-clone CI run sees the wrong version.

**Fix shape (what the design called for):** in `resolveVersionWith`, when the recipe has any `pipx_install` step, call `actions.GetEvalDeps("pipx_install")`, `actions.CheckEvalDeps(...)`, and `cfg.OnEvalDepsNeeded(...)` *before* the probe. This requires plumbing `OnEvalDepsNeeded` (and `AutoAcceptEvalDeps`) into `resolveVersionWith` — currently it's only available inside `resolveStep` via `cfg`. The design's "Phase 3 deliverables" list this exact change in `internal/executor/plan_generator.go`. The implementation skipped it.

## 4. The prerelease skip at filter time

`provider_pypi.go:74` and `:113` skip releases for which `isPyPIPrerelease(r.Version) == true`. The helper at `:192-200` does a textual scan: any letter in the version string marks it prerelease.

**Structural issues:**

- This is **not in the design or plan.** It was added during integration. The design's only correctness argument for what releases to skip is `requires_python` (Decision 2). Prerelease skipping is a separate semantic. Adding it without updating the design creates undocumented behavior.
- It **duplicates an existing mechanism.** `internal/version/version_utils.go` already has `splitPrerelease`, `comparePrereleases`, and stable/prerelease-aware ordering. `internal/version/provider_github.go` has `isStableVersion(version, stableQualifiers)` plus `nonSemverUnstableMarkers`. These are the project's canonical "is this stable?" predicates. Recipes can override the qualifier set via `[version] stable_qualifiers` — designed in `docs/designs/DESIGN-prerelease-detection.md`.
- The new `isPyPIPrerelease` ignores the recipe's `stable_qualifiers`. A recipe like `[version] stable_qualifiers = ["release", "final"]` would be respected by `provider_github.go` but silently overridden by `provider_pypi.go`'s any-letter-marks-prerelease scan.
- Behaviorally, `isPyPIPrerelease("1.0.0-rc1") == true` and `isPyPIPrerelease("1.0.0+local.1") == false`. A PEP 440 local-version segment (`+local.1`) is not a prerelease but contains no letters by definition — fine; conversely a calver-like `2024a` would be flagged unstable. PyPI metadata is unlikely to produce that, but the behavior diverges from `isStableVersion` for no documented reason.

**Severity: blocking.** This is a parallel pattern for stable-version detection. Either:

(a) Route through the existing `isStableVersion` helper (move it to a shared file and pass `DefaultStableQualifiers` plus the recipe's `Version.StableQualifiers`), or
(b) Document in the design that PyPI uses a different rule and why.

If (b), at minimum the design must be updated and `isPyPIPrerelease` must respect recipe-level `stable_qualifiers` overrides, otherwise the recipe contract drifts (one provider honors `stable_qualifiers`, another silently doesn't).

The conservative read: this is integration drift. Pip itself excludes prereleases by default unless `--pre` is passed, so the *intent* is reasonable. But the *mechanism* belongs in the existing helper, not as a parallel function.

## 5. Design-vs-implementation completeness

### Implemented matches design

- `internal/version/pep440/` package with `version.go`, `specifier.go`, `match.go`, `pep440_test.go` (and a `pep440_test.go` table). Operators, hardening checks, and `Canonical` all present.
- `pypiPackageInfo.Releases` migrated from `[]struct{}` to `[]pypiReleaseFile`. `requires_python` field is read.
- `ErrTypeNoCompatibleRelease` appended to the iota block in `errors.go`.
- `PyPIProvider.pythonMajorMinor` field, `NewPyPIProviderForPipx` constructor, filter in `ResolveLatest` and `ListVersions`, user-pin path bypassed.
- Typed `*ResolverError` with `pep440.Canonical(...)` rendering.
- `PyPISourceStrategy.CreateWithPython` and `InferredPyPIStrategy.CreateWithPython`.
- `recipes/a/ansible.toml` with `curated = true`, no version pin.
- `pipx_install.go` is unchanged (as the design required).

### Implemented but not in design

- `isPyPIPrerelease` skip in both `ResolveLatest` and `ListVersions` (see section 4).
- `PipxAwareStrategy` as an extension interface plus a parallel `ProviderFromRecipeForPipx` factory entry point. The design described "the strategy `Create` signature (or via a field on the strategy struct populated before `Create` is called)." The chosen shape (interface + parallel factory entry) is one of the option-space members but adds a parallel pattern for the factory caller.

### Specified but not implemented

- **Early `CheckEvalDeps` for `pipx_install` recipes.** Design Phase 3 deliverable: "`internal/executor/plan_generator.go` (early eval-deps check for `pipx_install` recipes; binary probe; major.minor truncation; pass to provider factory)." Implementation has the binary probe and major.minor truncation (in `executor.go`, not `plan_generator.go`) but **not** the early eval-deps install. This is the source of the cache-key divergence in section 3.
- **The `Executor.ResolveVersion` public method (`executor.go:166`)** still calls `factory.ProviderFromRecipe(...)`, not the pipx-aware variant. If any caller uses this for a pipx_install recipe to compute a cache key (e.g., the install orchestrator's pre-cache version resolve), the filter will be silently bypassed there too. The design only discusses `resolveVersionWith`/`GeneratePlan` flow; the public `ResolveVersion` is in the same file and was not updated. Worth a callsite audit before claiming the cache-key invariant holds across all entry points.

## Summary of severity calls

| # | Finding | Severity |
|---|---------|----------|
| 1 | Layering | OK |
| 2a | Dual constructor (`NewPyPIProvider` + `NewPyPIProviderForPipx`) | Advisory |
| 2b | Parallel `ProviderFromRecipeForPipx` entry point | Advisory |
| 3 | Cache-key divergence: `resolveBundledPythonMajorMinor` is best-effort, missing the design's required early `CheckEvalDeps` | **Blocking** |
| 4 | `isPyPIPrerelease` is a parallel pattern, not in design, and ignores recipe `stable_qualifiers` | **Blocking** |
| 5a | `Executor.ResolveVersion` public method not updated for pipx routing | Needs audit |
| 5b | Early `CheckEvalDeps` deliverable from Phase 3 not implemented | Blocking (same as #3) |
