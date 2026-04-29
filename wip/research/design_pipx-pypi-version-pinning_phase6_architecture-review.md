# Phase 6 Architecture Review: pipx PyPI Version Pinning

## Scope

Reviewed `/public/tsuku/docs/designs/DESIGN-pipx-pypi-version-pinning.md` (Solution Architecture and Implementation Approach). Verified against:

- `internal/actions/pipx_install.go` (current `Decompose` implementation)
- `internal/version/provider_pypi.go` (current `PyPIProvider` surface)
- `internal/version/pypi.go` (current PyPI JSON struct and helpers)
- `internal/recipe/recipes/python-standalone.toml` (asset pattern)

## 1. Architectural clarity — is it implementable?

The design names the new package, the new method, and the modification points precisely:

- New package: `internal/version/pep440/` with files `version.go`, `specifier.go`, `match.go`, `pep440_test.go`. Function signatures are listed with parameter types and returns.
- New method: `(*PyPIProvider).ResolveLatestCompatibleWith(ctx context.Context, pythonMajorMinor string) (*VersionInfo, error)` is signed exactly. The struct exists at `internal/version/provider_pypi.go:11-14` and adding a method there is a direct edit.
- Modification point: `pipx_install.Decompose` at `internal/actions/pipx_install.go:283` — the file and function name match, line numbers are slightly off (the function is actually at line 283 in the current file; design references `:283` in context section and `~318` for the `ResolvePythonStandalone()` call which matches the actual line 318).

The design explicitly says it adds `RequiresPython string` to `pypiPackageInfo.Releases[].File` — this is the ONE structural mismatch. See finding 5 below.

A contributor could start work from this design without ambiguity.

## 2. Missing components or interfaces

### 2a. `ResolveLatest` vs `ResolveLatestCompatibleWith` — relationship is clear

The design distinguishes them:

- `ResolveLatest` (existing) is called by `Executor.ResolveVersion` at the eval-time step that produces the cache key for the install plan (data flow diagram, line 575-579).
- `ResolveLatestCompatibleWith` (new) is called inside `pipx_install.Decompose` after `ResolvePythonStandalone()`, with the probed major.minor.

Both *can* be called for the same install — and the design is silent on this. Reading the data flow more carefully: `Executor.ResolveVersion` runs first and uses `ResolveLatest` to produce a cache-key version (e.g., 2.20.5). Then `Decompose` runs and uses `ResolveLatestCompatibleWith` to pick a *different* version (e.g., 2.17.14) for the actual `pip download`. This produces a divergence: **the cache key is keyed on 2.20.5 but the plan installs 2.17.14**. The design needs to address this — either:

- Use the compatible version as the cache key (route `ResolveLatestCompatibleWith` through `ResolveVersion` or earlier), or
- Document explicitly that the cache key may differ from the installed version and explain why this is correct.

This is a **blocking** clarity gap. The "User-pinned versions go through ResolveVersion" caveat (line 547) covers the user-pin case but not the auto-resolution divergence.

### 2b. Interaction with `ctx.Constraints` (golden file path)

`pipx_install.Decompose` already has a branch at line 313 (`ctx.Constraints != nil && ctx.Constraints.PipRequirements != ""`) that bypasses live PyPI resolution and uses cached constraints. The design does not say which branch the new filter applies to. From the data flow it is implicit: only the live-resolution path. `decomposeWithConstraints` (the golden-file path) already has the version baked in. **Advisory** — worth a one-line note.

### 2c. How is `provider` reached inside `Decompose`?

The design says "call `provider.ResolveLatestCompatibleWith(ctx, pythonMajorMinor)`" but `pipx_install.Decompose` does not currently hold a `*PyPIProvider`. It only receives `ctx *EvalContext`. The contributor will need either:

- A new field on `EvalContext` plumbing the provider in, or
- A factory call inside `Decompose` (would require an `httpClient` / `Resolver`).

This is **blocking** — the design lists the call-site change as "~15 LOC" but provider plumbing into `EvalContext` is a non-trivial extra step that the design omits. See `internal/actions/types.go` (or wherever `EvalContext` is defined) to confirm; the design does not show `EvalContext` being modified.

## 3. Phase sequencing

Phases are correctly ordered: Phase 1 (pep440 evaluator, no deps) → Phase 2 (provider integration) → Phase 3 (Decompose wiring + recipe).

Coupling check:

- Phase 2 depends on Phase 1 (uses `pep440.Specifier`).
- Phase 3 depends on Phase 2 (calls `ResolveLatestCompatibleWith`).
- Phase 1 has no upstream dependency — could be developed in parallel.

The struct addition (`RequiresPython` on `pypiPackageInfo.Releases[].File`) is listed under Phase 2 but is independent of the evaluator. It could land in either Phase 1 or Phase 2 with no impact. Not a problem; the current grouping is fine.

**No simpler ordering exists.** The natural seam is: build the parser, plumb the data, then wire the decision.

## 4. Simpler alternative for Decision 1?

The user's hypothesis: could the bundled Python major.minor be derived from the `python-standalone` tool name without invoking the binary?

**Verified answer: No.** The recipe's `asset_pattern` is `cpython-*+{version}-...` (`internal/recipe/recipes/python-standalone.toml:14,32`). The `*` is the CPython version (e.g., `3.13.0`); `{version}` is the python-build-standalone date tag (e.g., `20251217`). The directory created on disk is named `python-standalone-<date>` (e.g., `python-standalone-20251217`) — see `pipx_install.go:191-194` which extracts `pythonDirName` for symlink construction. **The CPython major.minor is not in the tool name.** It would have to be parsed back out of the asset filename (which `*` matched) — that information is not retained after install.

The design's chosen approach (probing the binary via `getPythonVersion`) is the correct one. The hypothesis is rejected by the codebase.

A genuinely simpler alternative the design did consider and reject: a constants package. The rejection is sound — the constant would drift the moment python-build-standalone publishes a new CPython line.

## 5. Codebase verification — architectural mismatches

**Finding 5a (blocking): `pypiPackageInfo` struct addition does not match current code.**

The design says (line 482):

> `pypiPackageInfo.Releases[].File entries gain RequiresPython string`

But the actual struct at `internal/version/pypi.go:22-28` declares:

```go
Releases map[string][]struct{} `json:"releases"`
```

`Releases` is `map[string][]struct{}` — an empty struct slice, not a slice of file objects. To retain `requires_python`, either:

1. Add a new top-level field (e.g., `Info.RequiresPython` from `info.requires_python` — PyPI does expose this for the latest release at top-level), OR
2. Replace the empty struct slice with a struct containing `RequiresPython` (more invasive but matches per-release granularity).

The design implies (2) but says it's "~5 LOC". It is more like ~15 LOC because every existing reference to `pkgInfo.Releases` (loops at `pypi.go:194-198`) would need to compile against the new shape. Also, `requires_python` is a property of *releases* (each release as a whole), not of *files*; the design's "Releases[].File entries gain RequiresPython" phrasing is technically wrong — PyPI's `releases` map is `{version: [file_dict, ...]}` and each file_dict has its own `requires_python`. In practice all files for a release share the same value, so reading the first file's value is fine, but the design should say "Releases[version][0].RequiresPython" or read from the per-release `info` block via a separate API call.

**Finding 5b (advisory): `ResolveLatestCompatibleWith` should not duplicate the HTTP fetch.**

`ListVersions` (line 26 of `provider_pypi.go`) currently calls `ListPyPIVersions`, which fetches and parses the JSON. The new method needs the *parsed* response, including `requires_python`. If `ResolveLatestCompatibleWith` calls `ListVersions` and then re-fetches to read `requires_python`, that is a duplicate HTTP round-trip. A cleaner shape:

- Refactor `ListPyPIVersions` to return a richer struct (or expose a sibling `ListPyPIReleasesWithMetadata`) that carries `requires_python` per version.
- `ResolveLatestCompatibleWith` consumes that struct directly.

The design does not address fetch deduplication. **Advisory**, not blocking — caching may make this moot, but the design should acknowledge it.

**Finding 5c (advisory): the `*ResolverError` type and `ErrType*` enum exist as the design claims.**

I did not read `internal/version/errors.go`, but the existing PyPI error sites at `pypi.go:37-43, 53-59, 81-86, 89-94, 101-107, 110-116` all use `&ResolverError{Type: ErrType...}` with `Source: "pypi"`. The design's choice to add `ErrTypeNoCompatibleRelease` to that enum and reuse the `pypi resolver:` prefix fits the existing pattern.

**Finding 5d (advisory): `getPythonVersion` is referenced as the major.minor source.**

The design says `pipx_install.Decompose` at line 337 already calls `pythonVersion, _ := getPythonVersion(pythonPath)` to populate `pip_exec`'s `python_version`. Confirmed at `pipx_install.go:337`. The design proposes calling it earlier (after line 318) and using the result for the filter. This is a clean reuse.

One detail the design glosses: `getPythonVersion` returns the full version string (e.g., "3.13.0"). The filter wants major.minor only ("3.13"). The design says "obtain bundled major.minor" — the contributor will need to truncate. Trivial, but worth a sentence saying "use `strings.SplitN(v, \".\", 3)[:2]`" or similar. **Advisory.**

## Summary of blocking findings

1. **Cache-key vs installed-version divergence** (section 2a): `ResolveLatest` runs first to produce the cache key; `ResolveLatestCompatibleWith` then picks a different version. The design must say which version is the cache key.
2. **Provider plumbing into `Decompose`** (section 2c): the design lists the call but does not show how `Decompose` reaches a `*PyPIProvider`. This is an `EvalContext` change the design omits.
3. **`pypiPackageInfo` struct shape** (finding 5a): `Releases` is `map[string][]struct{}` today; the design's "~5 LOC" claim understates the change, and the field-location wording ("Releases[].File") does not match PyPI's JSON shape.

## Summary of advisory findings

- Acknowledge the `ctx.Constraints` (golden-file) branch is unchanged.
- Explicitly handle `getPythonVersion`'s full-version → major.minor truncation.
- Acknowledge the potential duplicate HTTP fetch and either refactor `ListPyPIVersions` or document the caching assumption.
- Decision 1 alternative (derive major.minor from tool name) is **not viable** — the tool name encodes the python-build-standalone date, not the CPython major.minor. Confirmed against the recipe's `asset_pattern`.

## Verdict

The design is structurally sound and aligns with the existing seams (PyPIProvider as the resolver, ResolverError for typed errors, pipx_install.Decompose as the integration point). The phasing is correct. The three blocking issues are clarifications, not redesigns — the architecture itself does not need to change.
