# tsuku-expert review: pipx PyPI version pinning (issue #2331)

Branch: `docs/2331-pipx-pypi-version-pinning`
Commits inspected:
- `19f2ca59` — `internal/version/pep440/` PEP 440 evaluator
- `576106fb` — `PyPIProvider` requires_python filter
- `25100a95` — executor wiring + `recipes/a/ansible.toml`

Files inspected (selected):
- `recipes/a/ansible.toml`
- `internal/version/pep440/pep440.go` (`Version`, `ParseVersion`, `Compare`)
- `internal/version/pep440/specifier.go` (`Specifier`, `ParseSpecifier`, `Canonical`)
- `internal/version/pep440/match.go` (`Satisfies`, `IsEmpty`, `clause.matches`)
- `internal/version/pep440/pep440_test.go`
- `internal/version/provider_pypi.go` (`PyPIProvider`, `NewPyPIProviderForPipx`, filter helpers)
- `internal/version/pypi.go` (`pypiPackageInfo`, `listPyPIReleasesWithMetadata`)
- `internal/version/factory.go` (`PyPISourceStrategy`, `InferredPyPIStrategy`, `PipxAwareStrategy`, `ProviderFromRecipeForPipx`)
- `internal/version/resolve.go` (`ResolveWithinBoundary`)
- `internal/version/errors.go` (`ErrTypeNoCompatibleRelease`)
- `internal/version/version_utils.go` (existing semver/calver utilities)
- `internal/executor/executor.go` (`resolveVersionWith`, `ResolveVersion`, `resolveBundledPythonMajorMinor`)
- `internal/executor/plan_generator.go` (`GeneratePlan`, `resolveStep`, `PlanConfig`)
- `internal/actions/eval_deps.go` (`GetEvalDeps`, `CheckEvalDeps`)
- `internal/actions/pipx_install.go` (`Dependencies`, `Decompose`, `decomposeWithConstraints`)
- `internal/actions/pip_exec.go` (decomposed primitive)
- `plugins/tsuku-recipes/skills/recipe-author/SKILL.md`
- `plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md`

---

## 1. Recipe schema fit (`recipes/a/ansible.toml`)

The new recipe is minimal and follows tsuku's `pipx_install` conventions:

```toml
[metadata]
name = "ansible"
description = "Radically simple IT automation"
homepage = "https://www.ansible.com"
version_format = "semver"
curated = true
supported_libc = ["glibc"]

[[steps]]
action = "pipx_install"
package = "ansible-core"
executables = [...]   # 9 ansible-* binaries

[verify]
command = "ansible --version"
pattern = "{version}"
```

Comparison points against existing curated `pipx_install` recipes:

| Field | ansible.toml | poetry.toml | black.toml | httpie.toml |
|-------|--------------|-------------|------------|-------------|
| `curated = true` | yes | (no) | yes | (no) |
| `supported_libc` | `["glibc"]` | `["glibc"]` | step-level `when` | `["glibc"]` |
| `supported_os` | (omitted) | (omitted) | step-level `when` | `["linux"]` |
| `[version]` block | (omitted) | (omitted) | (omitted) | (omitted) |

Findings:

- **Inferred PyPI source is correct.** Other curated `pipx_install` recipes (poetry, httpie, black) omit `[version]` and rely on `InferredPyPIStrategy` (`internal/version/factory.go:296-334`). Adding `[version] source = "pypi"` is unnecessary; the inferred strategy now also implements `PipxAwareStrategy.CreateWithPython`, so it gets the same filtering treatment as the explicit form. Good.
- **`supported_os` is not declared.** httpie restricts to Linux glibc explicitly (`supported_os = ["linux"]`); ansible only restricts libc. This means the recipe will be attempted on macOS too. That's likely intentional (ansible-core ships pure-Python wheels), but worth confirming against the issue requirements. If macOS isn't a tested path, add `supported_os = ["linux"]` to be consistent with httpie.
- **No mention of the python-standalone dependency.** Per `pipx_install`'s `Dependencies()` (`internal/actions/pipx_install.go:22-28`), python-standalone is pulled in as `EvalTime + InstallTime + Runtime` automatically, so the recipe doesn't need to declare it explicitly. That matches conventions.
- **Verify section.** `pattern = "{version}"` is fine for `ansible-core`, since `ansible --version` prints `ansible [core <version>]`. The literal `{version}` placeholder is expanded against the resolved version, which matches httpie's pattern.
- **Schema-wise the recipe is clean.** No extra/missing required fields.

Minor nit: it's stylistically inconsistent with httpie that ansible doesn't pin `supported_os = ["linux"]`. Either is defensible.

---

## 2. Action and version provider plumbing

### 2a. `Decompose` and the `ctx.Constraints` golden-file branch

Looking at `PipxInstallAction.Decompose` (`internal/actions/pipx_install.go:283-359`), the constraints branch is at line 313:

```go
if ctx.Constraints != nil && ctx.Constraints.PipRequirements != "" {
    return a.decomposeWithConstraints(ctx, packageName, version, executables)
}
```

This branch is **independent of the new PyPI filter**. The new filter operates earlier — at version resolution time, before `Decompose` is called. By the time `Decompose` runs, `ctx.Version` is already the chosen (compatible) version. This is the right design: the filter narrows what gets resolved as "latest"; `Decompose` then locks the deps for whatever version was chosen.

Nothing in the constrained-eval (golden file) path needs changes. The constrained branch consumes `ctx.Constraints.PipRequirements` — it never looks at PyPI again, so requires_python filtering is irrelevant there. **The bypass is correct: golden-file replay is unaffected by the new filter.** This is what the design intended.

### 2b. Eval-deps registration and the executor's early probe

`PipxInstallAction.Dependencies()` (`internal/actions/pipx_install.go:22-28`):

```go
return ActionDeps{
    InstallTime: []string{"python-standalone"},
    Runtime:     []string{"python-standalone"},
    EvalTime:    []string{"python-standalone"},
}
```

`EvalTime` correctly contains `python-standalone`. This is unchanged by the PR.

The executor's pre-resolution probe is in `executor.resolveBundledPythonMajorMinor` (`internal/executor/executor.go:111-146`). It returns `""` when:

1. The recipe has no `pipx_install` step, OR
2. `actions.ResolvePythonStandalone()` returns `""` (not installed yet), OR
3. `actions.GetPythonVersion(...)` errors.

Returning `""` causes `ProviderFromRecipeForPipx` to fall through to `Create` (no filtering). The downstream `CheckEvalDeps` in `resolveStep` (`internal/executor/plan_generator.go:331-342`) then triggers the install of python-standalone via `cfg.OnEvalDepsNeeded` and decomposition runs.

**Consistency observation:** the early probe and `CheckEvalDeps` use **the same dependency name** (`python-standalone`) — `executor.resolveBundledPythonMajorMinor` calls `actions.ResolvePythonStandalone` directly while `CheckEvalDeps` reads from `pipx_install`'s `Dependencies().EvalTime`. Both arrive at `python-standalone`, so they don't diverge. That's good.

**However, there is a subtle first-run regression.** When `python-standalone` isn't yet installed:

1. The probe returns `""`.
2. `ProviderFromRecipeForPipx` falls through to unfiltered `Create`.
3. `ResolveLatest` returns absolute-latest from PyPI (including releases that require Python newer than what tsuku will actually install).
4. `resolveStep` then triggers `CheckEvalDeps` → installs python-standalone → calls `Decompose`.
5. `Decompose` runs `pip download` against the absolute-latest version chosen in step 3 — which can fail with a `requires_python` error from pip itself.

The comment at `executor.go:114-117` acknowledges this fallback: *"resolution will fall back to absolute-latest for this run; subsequent runs probe successfully."* This is the documented behavior, but it means the **failure mode the PR is supposed to fix can still surface on first install** of an ansible-core-like tool when python-standalone isn't already on the system. The user-visible error in that case will be a `pip download` failure inside `generateLockedRequirements`, not the typed `ErrTypeNoCompatibleRelease` the new code defines.

A fix would be to install eval-deps **before** version resolution when the recipe has a pipx step. The current ordering (resolve → check deps → decompose) is fine for non-pipx recipes; pipx is the one case where eval-deps inform resolution. Worth flagging as follow-up.

### 2c. User-pin authority

`Executor.ResolveVersion` (`internal/executor/executor.go:166-183`) — the path used by the install flow when the user passes `tsuku install foo@1.2` — uses `factory.ProviderFromRecipe(...)`, **not** `ProviderFromRecipeForPipx`. So user pins go through an unfiltered provider. Inside that path, `ResolveWithinBoundary` ends up calling `provider.ResolveVersion(ctx, requested)` (`internal/version/resolve.go:52`), and `PyPIProvider.ResolveVersion` (`internal/version/provider_pypi.go:138-158`) explicitly uses `ListPyPIVersions` (unfiltered). So an explicit pin like `ansible@2.18` resolves regardless of bundled-Python compatibility. Good — the pin authority is preserved as the design intends.

But there's a **subtle inconsistency on partial pins** when the install flow uses `GeneratePlan` with `cfg.PinnedVersion == ""` and the user passed a partial constraint via `e.reqVersion`:

- `resolveVersionWith` (`internal/executor/executor.go:95-109`) calls `ProviderFromRecipeForPipx` → returns a *filtered* provider.
- `ResolveWithinBoundary` sees `requested = "2.17"` (a partial pin), takes the `VersionLister` branch (`resolve.go:33-49`), and calls `lister.ListVersions(ctx)` — **which is the filtered list**.
- The for-loop matches the partial pin against the filtered list, then calls `provider.ResolveVersion(ctx, v)` for the matched version.

Result: a partial pin like `ansible@2.18` will silently resolve to `2.17.x` if 2.18 requires Python newer than the bundled standalone. The user explicitly asked for `2.18`; the filter quietly downgraded the result. The current `PyPIProvider.ResolveVersion` path is "user-pin authoritative," but the partial-pin path through `ListVersions` isn't, because partial pins use the lister path.

This may be the intended trade-off — a partial pin is "the highest matching version," and "highest *compatible*" is a reasonable interpretation. But it's a deviation from the design's stated principle that "user-pin behavior flows through `ResolveVersion`, not `ResolveLatest`," and worth either documenting in `PyPIProvider`'s comments or correcting by giving `ListVersions` an `unfiltered=true` mode for the resolve.go boundary path.

---

## 3. `internal/version/pep440/` package boundaries

The pep440 package is a **clean, scoped subpackage**. It declares upfront (`pep440.go` doc) that it's "scoped to PyPI Python-compatibility filtering" and "not a complete PEP 440 implementation." It has its own `Version` (integer-segment, 1-4 parts) and `Compare`, separate from `version.CompareVersions` in `version_utils.go`.

Is this duplication?

- `version_utils.go::CompareVersions` is a string-based, prerelease-aware semver/calver/Go-tag comparator. It uses `fmt.Sscanf` for parsing and supports `1.0.0-rc.1`, `2024.01.15`, `Release_1_15_0`, `kustomize/v5.7.1`, etc.
- `pep440.Version.Compare` is integer-typed, ASCII-only, hardened (segment caps, segment count caps, length caps), and only compares numeric integer segments.

They serve different purposes and the duplication is intentional. The pep440 package's hardening (`maxVersionLen`, `maxSegmentDigits`, `maxClauseLen`, ASCII-only) is appropriate for parsing untrusted upstream PyPI metadata; folding it into `version_utils.go` would either weaken `version_utils.go`'s type system or pollute pep440 with the loose `interface{}`/`Sscanf` style.

**Subpackage placement is correct.** `internal/version/pep440/` is the right home — it's a piece of version-resolution machinery used only by the PyPI provider, and keeping it as a subpackage prevents the top-level `version` package from inheriting another `Version` type that conflicts with the ad-hoc string-based one.

One small structural note: the package has a sentinel `ErrUnsupportedOperator` for `~=` and `===`, but the corresponding `isPyPIReleaseCompatible` (`provider_pypi.go:175-184`) treats *any* parse failure (including unsupported operators) as "incompatible." That means a release pinning Python with `~=3.10` will be **silently excluded** from the candidate list rather than treated as a soft compatibility hint. The conservative-fallthrough comment in `provider_pypi.go` calls this out — but it's worth noting that real-world PyPI metadata occasionally uses `~=` (compatible-release) for `requires_python`. A check of how often this appears in the survey table (`pep440_test.go::surveyRequirements`) would inform whether this is a meaningful gap. The survey covers 30+ real-world strings and none use `~=`, so the risk is low.

---

## 4. Skill drift

Per `tsuku/CLAUDE.md`, changes to `internal/actions/`, `internal/version/`, `internal/recipe/`, and `cmd/tsuku/` require checking the corresponding skills. Reviewed:

### tsuku-recipe-author

- `SKILL.md` lists `pypi` as a version source auto-detected from `pipx_install`. Still accurate.
- `references/action-reference.md::pipx_install` lists the action as "Platform: All" with parameters `package`, `executables`, `pipx_path`, `python_path`. **No mention** of:
  - The new behavior that `latest` now means "newest compatible with bundled Python."
  - The `ErrTypeNoCompatibleRelease` error users may see.
  - That this filter does not apply to user-pinned versions.

This is **a contract change visible to recipe authors and end users**: a recipe that worked yesterday (returning whatever PyPI's `info.version` says) may now return an older version, and authors need to know why. Recommend updating `action-reference.md::pipx_install` with a brief paragraph and pointing at the design doc. This is the kind of behavior change the "skill drift" rule in CLAUDE.md was written for.

### tsuku-recipe-test

- `SKILL.md` lists exit code 5 for "network error" and 8 for "dependency failed." The new typed error `ErrTypeNoCompatibleRelease` doesn't have a dedicated CLI exit code — it surfaces as a generic install failure (exit 6 / `ExitInstallFailed`). If the team wants this case to be diagnosable via exit code, that's a follow-up change and a corresponding skill update. As-is, no contract change to recipe-test.

### tsuku-user

- Doesn't cover pipx specifics. No update needed for this PR.

**Verdict:** the recipe-author skill (specifically `references/action-reference.md`) is missing an update for the new `pipx_install` behavior. Including this update in the same PR would satisfy the CLAUDE.md "skill drift" rule.

---

## 5. Subtle correctness concerns

### 5a. `requires_python` is not the only compatibility axis

The design framed pipx incompatibility as "package's `requires_python` excludes our bundled Python." That's the dominant cause for ansible-core, but PyPI compatibility is a multi-dimensional question:

1. **`requires_python`** (top-level metadata) — what the new filter handles.
2. **Wheel platform tags** (manylinux1, manylinux2014, manylinux_2_28, musllinux_1_2, etc.) — a release with a perfectly fine `requires_python` may have **no wheel** for the current `manylinux` baseline. This is the dominant `pipx_install` failure on Alpine and old-glibc systems.
3. **Wheel ABI tags** (cp310, cp311, abi3) — a release may have only `cp311` wheels even though `requires_python = ">=3.10"`. Choosing python-standalone 3.10 against a cp311-only release will fail at `pip install` time.
4. **Transitive dependency compatibility** — the `ansible-core` release is compatible, but a transitive dep's `requires_python` excludes our Python. The new filter doesn't see transitives.
5. **Prereleases** — handled (`isPyPIPrerelease` in `provider_pypi.go:192-200`), but the heuristic ("any letter character") will exclude `1.0.0-postN`-style tags that aren't actually prereleases. This is conservative-correct for the ansible case but could over-exclude.

Each of these is a follow-up rather than a bug in this PR — the design explicitly scoped to `requires_python`. But it's worth being clear: **this PR fixes one cause of `pipx_install` failures on tsuku's bundled Python; it does not turn pipx_install into a hermetic ecosystem.** The `pep440` package alone won't fix musllinux wheel gaps, ABI mismatches, or transitive `requires_python` failures.

### 5b. azure-cli is misclassified by the design

The design deferred azure-cli. From inspection, **azure-cli's failure mode is fundamentally different** from ansible's:

- Ansible: `requires_python = ">=3.10"` excludes Python 3.9; latest version is unusable on older Python. **Resolved by this PR.**
- Azure-cli: depends on `azure-mgmt-*` packages that historically had ABI issues, plus its own complex dependency graph (cryptography, paramiko, msal). Its failures are usually wheel-availability or dep-resolution failures, not top-level `requires_python` mismatches.

This PR's filter would **not** correctly classify azure-cli's failure. Running `tsuku install azure-cli` against this code would still fail at the `pip download` stage in `Decompose`, surfacing as a generic install failure rather than `ErrTypeNoCompatibleRelease`. The "no compatible release" message would be **misleading** if shown for azure-cli, because the latest azure-cli release may have a satisfiable `requires_python` and still fail.

The deferral is therefore correct: addressing azure-cli requires either wheel-tag inspection at version-resolution time (cost: another PyPI fetch per release to read file lists, plus a manylinux/glibc compatibility model) or a different strategy (recipe-level version pinning override).

### 5c. Empty release handling

`listPyPIReleasesWithMetadata` (`pypi.go:322-336`) takes the **first non-empty** `requires_python` from a release's file list. For releases with no files (yanked-only), it sets `RequiresPython = ""`, which `isPyPIReleaseCompatible` treats as compatible. This means **yanked-only releases can still be returned as "latest."** The comment acknowledges this matches pip's behavior, but pip also has its own yanked-release filter that this code doesn't replicate. Worth a follow-up: PyPI's release files have a `yanked` field that this struct doesn't model.

### 5d. Specifier parser hardening is good; one minor gap

The hardening checks (`ErrInputTooLong`, `maxClauses`, `ErrSegmentTooLarge`, ASCII-only) are appropriate for parsing untrusted upstream metadata. The survey-string test (`TestParseSpecifier_SurveyStrings`) covers a representative sample. No issues.

One small gap: `Canonical` returns `<malformed>` on any parse failure. The implementation re-runs `ParseSpecifier` instead of holding parsed state, which means an already-parsed specifier still pays the parse cost when canonicalizing. Minor performance question, not correctness.

---

## Summary of actionable items

1. **Skill update missing** — `references/action-reference.md::pipx_install` should describe the new `latest` semantics and point at the design. (Same PR.)
2. **First-install regression on partial pins / fresh systems** — when python-standalone isn't yet installed AND user passes a partial pin, the for-pipx provider can downgrade silently. Either move eval-deps install to *before* version resolution for pipx recipes, or document this behavior in `PyPIProvider`. (Follow-up.)
3. **`ansible-core` recipe could optionally add `supported_os = ["linux"]`** for consistency with httpie. Not required.
4. **`ErrTypeNoCompatibleRelease` has no dedicated exit code.** Currently surfaces as generic install failure. (Follow-up.)
5. **Misleading "no compatible release" framing for non-`requires_python` failures.** Document scope in user-facing error message: "no release of X declares compatibility with Python Y" rather than implying the diagnosis is exhaustive.
6. **Yanked-only releases can still be returned** as "latest." `pypiReleaseFile` doesn't model `yanked`. (Follow-up.)

The PR is in good shape overall. The pep440 subpackage is well-bounded, the factory/strategy seam is clean, the constrained-eval (golden file) branch is correctly bypassed, and user-pin authority is preserved in the obvious paths. The main concerns are documentation drift (skill) and one subtle partial-pin downgrade edge case.
