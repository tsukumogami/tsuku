# Review: pipx PyPI version pinning (#2331)

Scope: PLAN-pipx-pypi-version-pinning.md, three commits 19f2ca59, 576106fb, 25100a95.

## 1. Plan Coverage

### Issue 1 — pep440 evaluator (19f2ca59)

All ACs satisfied. Files `version.go`, `specifier.go`, `match.go`, `pep440_test.go`
present. All four exported types/functions match the interface signatures
(`ParseVersion`, `ParseSpecifier`, `Satisfies`, `Canonical`, `Compare`, `String`).
Hardening checks all enforced at `ParseSpecifier` entry. Operator semantics correct
(longest-prefix match: `>=` before `>`). Wildcards restricted to `==`/`!=`. `~=`,
`===` rejected with `ErrUnsupportedOperator`. `Canonical` returns `"<malformed>"`
for invalid inputs.

Minor deviation: plan called for "96 requires_python strings from the L5 survey" but
test seeds with 30 representative strings (commit message acknowledges this).
Coverage of operator/wildcard variants and key bundled-Python scenarios is sound.
Not a blocker.

No regex, no recursion, stdlib-only — all met.

### Issue 2 — PyPI provider filter (576106fb)

All ACs satisfied. `pypiPackageInfo.Releases` shape changed correctly. 10 MB
response cap preserved. `ErrTypeNoCompatibleRelease` appended to the existing
iota at the documented position. `NewPyPIProvider` signature unchanged.
`NewPyPIProviderForPipx` constructor present. `ResolveLatest`/`ListVersions`
filtering implemented. `ResolveVersion` (user-pin) uses unfiltered
`ListPyPIVersions` — never filtered. Error message rendering goes through
`pep440.Canonical(...)`.

All seven test cases the plan called out are present:
- `TestPyPIProvider_ResolveLatest_FiltersByPython` (latest compatible in middle)
- `TestPyPIProvider_ResolveLatest_NoCompatibleRelease`
- `TestPyPIProvider_ResolveLatest_NullRequiresPythonIsCompatible`
- `TestPyPIProvider_ResolveLatest_UnparseableSpecifierSkipsRelease`
- `TestPyPIProvider_ResolveVersion_UserPinUnaffected`
- `TestPyPIProvider_ErrorMessage_RendersCanonicalNotRawBytes`
- `TestPyPIProvider_ResolveLatest_EmptyPythonMajorMinorPreservesBehavior`

### Issue 3 — executor wiring + ansible recipe (25100a95)

Most ACs satisfied. `PipxAwareStrategy` interface and `ProviderFromRecipeForPipx`
factory entry present. `PyPISourceStrategy.CreateWithPython` and
`InferredPyPIStrategy.CreateWithPython` implemented. `resolveBundledPythonMajorMinor`
guards on `Action == "pipx_install"` so non-pipx recipes incur no probe.
`getPythonVersion` exported as `GetPythonVersion`. `recipes/a/ansible.toml` added
without a version pin.

**One AC NOT satisfied — see Section 4 / Blocker 1 below.** The plan required:
> The cache key (from `Executor.ResolveVersion`), the install directory name,
> and the version `pipx_install.Decompose` writes... are the same string —
> no divergence.

`Executor.ResolveVersion` (called from `cmd/tsuku/install_deps.go:81` to compute
the cache key) still uses `factory.ProviderFromRecipe`, NOT the pipx-aware
variant. `resolveVersionWith` (called from `GeneratePlan`) was updated.
The two paths now produce different versions for pipx_install recipes.

Plan AC also said the eval-deps check should run **before** `resolveVersionWith`
in `plan_generator.go`. Implementation instead silently returns "" from
`resolveBundledPythonMajorMinor` when `python-standalone` is missing and
relies on the existing decomposition-time `CheckEvalDeps` to install it. This
works for second-and-later runs, but the **first** run resolves to absolute-latest
(unfiltered), then the decompose path installs python-standalone, and the
plan ends up with the wrong version cached. See Blocker 1.

## 2. Test Coverage

Issue 2 test names map 1:1 to the plan's enumerated cases. Assertions check
the right things (e.g., the "Canonical not raw bytes" test asserts both that
the zero-width space is absent AND that `<malformed>` appears).

Issue 3 tests cover factory routing (pipx vs. non-pipx, set vs. unset
`pythonMajorMinor`), `truncatePythonMajorMinor` table-driven cases, and the
no-probe path for non-pipx recipes. Plan AC also called for "plan-generator
early eval-deps path with python-standalone absent (triggers
`cfg.OnEvalDepsNeeded`)" — there is no test for this path. The implementation
doesn't actually take an early path; it uses the existing decompose-time path,
which has its own tests, but the wiring of `pythonMajorMinor=""` → fall-back-to-
absolute-latest is not exercised by an integration test. **Should-fix**.

End-to-end resolution against `recipes/a/ansible.toml` is not in a unit test;
it relies on offline-skipped `tsuku eval` validation via the commit message. The
plan AC included this as a manual acceptance step rather than an automated test,
so this is acceptable, but a sandbox-tagged integration test would harden it.

## 3. Standards (CLAUDE.md)

- gofmt: clean across all changed files.
- go vet: clean across `internal/version/...`, `internal/executor/...`,
  `internal/actions/...`.
- AI attribution in commits: none. Commit messages follow Conventional Commits
  (`feat(version):`, `feat(executor):`).
- `$TSUKU_HOME` vs `~/.tsuku`: new doc comment in `pip_exec.go` correctly uses
  `$TSUKU_HOME`. Pre-existing `~/.tsuku` mentions in `executor.go:34-37` are
  not introduced by this PR.
- Emojis in code: pre-existing `❌`/`✅` in `executor.go` log lines are not
  introduced by this PR. New code is emoji-free.
- Comments referencing issue numbers (CLAUDE.md says "should be in commit
  messages, not code"):
  - `internal/version/pep440/pep440_test.go:222` — `// ansible-core lines
    vs Python 3.10 (the failing case from #2331)`
  - `internal/version/provider_pypi.go:30` — `// Behavior matches the
    pre-#2331 contract.`
  Both should be removed or rephrased without the issue number. **Nit**.

## 4. Bugs

### Blocker 1 — cache-key / plan-version divergence

`cmd/tsuku/install_deps.go:81` calls `resolver.ResolveVersion(ctx, ...)` where
`resolver` is the `*Executor` (line 58). `Executor.ResolveVersion`
(`internal/executor/executor.go:166`) calls `factory.ProviderFromRecipe`, NOT
`ProviderFromRecipeForPipx`. The downstream `GeneratePlan` calls
`resolveVersionWith` which uses the pipx-aware factory.

For ansible (the proof-point recipe):
- `Executor.ResolveVersion` returns absolute-latest, e.g. `2.20.5` (Python ≥ 3.12).
- Cache key: `ansible-2.20.5-linux-amd64`.
- `GeneratePlan` returns plan with version `2.17.14`.
- The actual install proceeds with `plan.Version = 2.17.14` (line 405 of
  install_deps.go uses `plan.Version`, not `resolvedVersion`).
- Cache lookup misses every run because the cache is keyed by the unfiltered
  version while the cached plan is stored under the filtered one.
- More serious: a future invariant relying on `cacheKey == plan.Version` would
  break silently.

The plan called this out as an explicit AC. The fix is to update
`Executor.ResolveVersion` to use `ProviderFromRecipeForPipx` (or share the
same code path as `resolveVersionWith`).

### Should-fix 1 — `resolveBundledPythonMajorMinor` silent fall-through

When `ResolvePythonStandalone()` returns "" (python-standalone not installed)
or `GetPythonVersion()` errors, `resolveBundledPythonMajorMinor` returns "".
The provider falls back to absolute-latest for that run. The function comment
acknowledges this and asserts "subsequent runs probe successfully", but:

1. The first run will resolve and cache an incompatible version (the bug
   #2331 was meant to fix). On a fresh machine, `tsuku install ansible`
   without `python-standalone` already present will pick `ansible-core 2.20.5`
   (incompatible with the to-be-installed 3.10 python-standalone).
2. The plan called for `actions.GetEvalDeps`/`CheckEvalDeps` BEFORE
   `resolveVersionWith` runs, so python-standalone is installed first and
   the probe always succeeds. The implementation skips this step.

Concrete failure mode: pipx_install fails on first run for any recipe whose
unfiltered latest is incompatible. The actual `pipx_install.Decompose` step
will fail with a Python-incompatible PyPI download.

### Should-fix 2 — `isPyPIPrerelease` ASCII letter scan

`provider_pypi.go:192-200` scans for any ASCII letter to identify prereleases.
Edge cases:

- PEP 440 `epoch!` syntax (e.g., `1!2.0.0`) contains no letters, not flagged
  as prerelease — correct.
- Local version segments (`+local.foo`) contain letters — falsely flagged
  as prerelease. PyPI doesn't publish local version identifiers, so unlikely
  to matter, but the comment doesn't acknowledge the limitation.
- Calver versions like `2025.04.28` — no letters, correctly accepted.
- `1.0.0.dev1` — flagged as prerelease (matches pip's default behavior).
- `1.0.0.post1` — flagged as prerelease. Pip actually treats post-releases
  as installable by default. The comment claims "matches pip's default" but
  this specific case doesn't match. In practice, very few packages ship
  `.postN` releases on PyPI, but the comment is misleading.

### Should-fix 3 — `listPyPIReleasesWithMetadata` first-non-empty `requires_python`

`pypi.go:328-334` takes the first file's non-empty `requires_python` as
authoritative. JSON file order within a release is preserved by
`encoding/json`. Releases with mixed `requires_python` across files
(e.g., wheel `>=3.10`, sdist `>=3.6`) would silently use whichever appears
first in the JSON. In practice all files share the same value; the comment
acknowledges this. A defensive choice would be to take the loosest-bound
value or warn on mismatch. **Nit-level concern.**

### Should-fix 4 — `ProviderFromRecipeForPipx` priority ordering

Strategies are evaluated in priority order. `PyPISourceStrategy` (priority 100,
`source = "pypi"`) is evaluated before `InferredPyPIStrategy` (priority 10).
For ansible, `source = "pypi"` is not set in the recipe, so the inferred path
fires. Both strategies implement `PipxAwareStrategy`. The factory loop returns
on the first `CanHandle = true`. This is correct. No bug, but worth noting that
the priority chain works: a recipe with `source = "pypi"` AND `pipx_install`
goes through the explicit strategy; recipes with only `pipx_install` go
through the inferred strategy. Both routes apply the filter.

## 5. Style

- `version.go:68`: `for j := range len(p)` — Go 1.22+ idiom. Surrounding
  codebase mostly uses `for i := 0; i < len(...); i++`. Both work; minor
  inconsistency.
- `provider_pypi.go:193`: `for i := 0; i < len(v); i++` — uses the older
  idiom. Same package mixes styles.
- `pep440/specifier.go` / `version.go` — clean, well-documented. Doc
  comments are unusually thorough relative to the rest of the codebase
  (positive observation).
- `recipes/a/ansible.toml` doesn't include a `[version]` section. Inferred
  strategy is the documented path for pipx_install when no source is
  specified, so this is fine.

## Test Run Results

```
go test ./internal/version/pep440/...    PASS
go test ./internal/version/...           PASS
go test ./internal/executor/...          PASS
go vet ./internal/version/...            clean
go vet ./internal/executor/...           clean
go vet ./internal/actions/...            clean
gofmt -l (changed files)                 clean
```

## File References

- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/pep440/version.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/pep440/specifier.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/pep440/match.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/pep440/pep440_test.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/pypi.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/provider_pypi.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/provider_pypi_test.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/provider_factory.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/provider_factory_test.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/errors.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/executor/executor.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/executor/executor_test.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/actions/pip_exec.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/actions/pipx_install.go`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/a/ansible.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/cmd/tsuku/install_deps.go` (call site that exposes the divergence)
