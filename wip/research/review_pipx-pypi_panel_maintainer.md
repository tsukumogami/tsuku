# Maintainer Review — pipx PyPI version pinning (issue #2331)

Branch: `docs/2331-pipx-pypi-version-pinning`
Commits: `19f2ca59` (PEP 440 evaluator), `576106fb` (PyPI provider filter), `25100a95` (executor wiring + ansible recipe)

Reviewer lens: a contributor who didn't write this. Can they read it, build a correct mental model, and change it confidently?

## 1. Naming

| Symbol | Verdict | Notes |
|---|---|---|
| `PipxAwareStrategy` | OK | Reads as "strategy that knows about pipx." Accurate. The name surfaces the asymmetry honestly: only PyPI strategies implement it today. |
| `ProviderFromRecipeForPipx` | OK with caveat | The name says "build a provider for pipx," but the function actually accepts an empty `pythonMajorMinor` and silently degrades to non-pipx behavior. A reader who sees `ProviderFromRecipeForPipx(resolver, r, "")` may assume it errors or asserts. It does neither. See finding (3). |
| `NewPyPIProviderForPipx` | OK with same caveat | Same: `NewPyPIProviderForPipx(resolver, "pkg", "")` is permitted and behaves like `NewPyPIProvider`. The doc comment names this contract; the symbol name does not hint at the empty-string fallback. |
| `pythonMajorMinor` | Good | Better than `pythonVersion` — the truncation is encoded in the name. Consistent across `provider_pypi.go`, `provider_factory.go`, and `executor.go`. |
| `resolveBundledPythonMajorMinor` | OK | "Resolve" usually implies "fail if not found"; this function returns `""` on every probe-failure path. The leading doc comment ("Returns the empty string for recipes that have no pipx_install steps... or when the binary is not yet installed") clears this up, but the verb is mildly misleading. `probeBundledPythonMajorMinor` would track its actual semantics better. **Advisory.** |
| `truncatePythonMajorMinor` | OK | Accurate. Falls back to the input string if it has fewer than two segments — a maintainer reading this might wonder why it doesn't error; the comment explains it. |
| `ErrTypeNoCompatibleRelease` | Good | Reads cleanly alongside the existing iota members. Positive. |

**No blocking naming issues.** The advisory above is worth a follow-up; everything else is accurate.

## 2. Comments

Where the WHY is well-captured:
- `provider_pypi.go:165-174` — `isPyPIReleaseCompatible`: the "treat unparseable as incompatible — conservative" decision is documented with a clear reason ("we can't verify satisfaction"). Future contributors will understand why they can't loosen this without thinking.
- `provider_pypi.go:23-27` — struct doc: pipx vs. non-pipx behavior is explicit, including the user-pin bypass.
- `provider_factory.go:111-117` — `PipxAwareStrategy` interface: explicit "called only after the executor has verified python-standalone is installed." This is a real implicit-contract documented well.
- `executor.go:89-94` — `resolveVersionWith` doc: links to the design and explains the cache-key rationale.
- `executor.go:111-122` — `resolveBundledPythonMajorMinor`: explains why probe failures *don't* abort, and documents the "fall back to absolute-latest for this run" behavior.

Comments that are missing or weak:

**Blocking — `provider_pypi.go:73-76` (and `:113-115`): the prerelease skip has no rationale on the call site.** The function `isPyPIPrerelease` (lines 186-200) explains *what* it detects ("any alphabetic character after the leading numeric segments triggers the skip") but **does not explain why this filter exists in commit 576106fb**. The design doc never mentions a prerelease filter — it talks about Python-compatibility filtering only. A reader who walks up cold sees:

```go
for _, r := range releases {
    if isPyPIPrerelease(r.Version) {
        continue
    }
    if isPyPIReleaseCompatible(r.RequiresPython, target) {
        ...
```

...and has no way to tell whether (a) the Python compat filter implicitly required a prerelease skip, (b) this was an unrelated drive-by, or (c) it papers over a real test that was failing. The function's own comment ("Used to mirror pip's default behavior of preferring stable releases unless `--pre` is requested") is plausible but unverified — pip's `--pre` behavior depends on whether the user pinned a prerelease, not on whether the resolver skips them outright. **Action:** add a comment at the call site explaining the choice and its scope (e.g., "pip's default install picks stable releases; tsuku has no `--pre` flag, so we mirror that here. User pins via `ResolveVersion` are unaffected — see ResolveVersion."), and a corresponding sentence in the design doc. Without this, the next person who needs to add prerelease support will spend 30+ minutes reverse-engineering whether removing the skip is safe.

**Advisory — `executor.go:123-146`, `resolveBundledPythonMajorMinor`:** the "downstream `CheckEvalDeps` will surface a missing python-standalone... as today" is the load-bearing claim. It would be stronger if it pointed at the file/function (`plan_generator.go:resolveStep:331-342`) so the next contributor doesn't have to find it. Without that pointer they may believe the probe failure silently breaks installs.

**Advisory — `provider_pypi.go:55-57` (`ListVersions`):** when `pythonMajorMinor == ""`, the function falls through to `ListPyPIVersions`. The doc comment says "When the provider was constructed with a Python major.minor, the list is filtered." It doesn't restate the empty-string semantics here as explicitly as `ResolveLatest` does. Minor.

**Stale risk — none observed.** The design doc's claim about `pypiPackageInfo.Releases` (`map[string][]struct{}` → typed) lines up with what `listPyPIReleasesWithMetadata` returns; the comments and code agree.

## 3. Implicit contracts

**Blocking — empty-string overload of `pythonMajorMinor` is the central implicit contract and is under-defended.**

The contract is: "empty means today's behavior; non-empty means filter." This is fine *if* the next contributor reads the constructor doc. But:

1. `NewPyPIProviderForPipx(resolver, "pkg", "")` is silently accepted and produces a non-filtering provider. There is no `panic`, no error, no log. A future contributor who wires up a new caller and forgets to plumb the major.minor will get a non-filtering provider at runtime with no signal — and the whole point of this issue is that the absence of filtering is the bug.
2. `ProviderFromRecipeForPipx(resolver, r, "")` exhibits the same.

**Recommendation:** either (a) make `NewPyPIProviderForPipx` reject the empty string with a clear error ("use NewPyPIProvider for non-pipx contexts") and have callers route the empty case to `ProviderFromRecipe`, *or* (b) leave today's behavior but add one line to the function doc making the failure mode explicit ("Passing `pythonMajorMinor == \"\"` is permitted and produces a non-filtering provider equivalent to `NewPyPIProvider`. Most callers should use `NewPyPIProvider` directly for that case."). Option (a) is the safer maintenance posture; option (b) is the minimum that tells the next contributor "the empty case is intentional, not a bug."

Currently the executor uses pattern (b) implicitly — `resolveVersionWith` always calls `ProviderFromRecipeForPipx`, even for non-pipx recipes, relying on the empty-string fallthrough. That pattern is fine *given* the current set of strategies; but it means the dual-path is exercised on every plan generation and a regression in the fallthrough would silently affect every recipe.

**Advisory — `PipxAwareStrategy` is not enforced or documented as exhaustive.** `ProviderFromRecipeForPipx` only routes through `CreateWithPython` when *both* the strategy implements `PipxAwareStrategy` AND `pythonMajorMinor != ""`. If a future contributor adds a new pipx-style ecosystem (say, a Conda/anaconda strategy) and forgets to implement `PipxAwareStrategy`, the filter silently won't apply. There's no compile-time check that "if your strategy fires on `pipx_install` steps, it must implement `PipxAwareStrategy`." The PyPI-only scope is documented in the design's "Negative consequences" but not in the code. A short comment on the `PipxAwareStrategy` interface listing today's implementers would be enough to alert the next contributor.

## 4. Test legibility

`internal/version/pep440/pep440_test.go` and `internal/version/provider_pypi_test.go` are both well-organized for a new contributor.

Strengths:
- The `surveyRequirements` table at the top of `pep440_test.go:18-59` is a fixture-with-provenance — comments explain each cluster (ansible-core, poetry-style, requests/numpy long-tail, etc.) and tie back to the L5 research. Adding a new survey string is one line.
- `pypiTestRelease` and `newMockPyPIServer` (lines 15-49 of `provider_pypi_test.go`) are clean fixture helpers. The naming makes their role obvious. Adding a new release scenario takes 4 lines.
- Each test name describes the scenario in the Go convention (`TestPyPIProvider_ResolveLatest_NoCompatibleRelease`). The name doesn't lie about what it tests.
- `ansibleStyleReleases()` (`provider_pypi_test.go:54-63`) is a named fixture builder with a comment explaining the real-world progression. Good.

Possible reductions in clarity:
- **Advisory — adversarial fixture relies on a literal zero-width space character.** `provider_pypi_test.go:222` uses a raw zero-width-space embedded in a Go string literal: `">=3.12​"`. This is invisible in most editors. If a future contributor copy-pastes the line to make a similar test, the zero-width space may or may not survive. The test name `TestPyPIProvider_ErrorMessage_RendersCanonicalNotRawBytes` is honest, but a comment near the literal — or a `​` escape — would make the intent legible. The same pattern appears in `pep440_test.go:132` and `:200`. **Action:** swap the raw character for a constructed string (`">=3.12" + "​"`) or document the byte at the call site.
- The `TestSatisfies_BundledPythonScenarios` table (lines 216-267) uses `tc.spec+"|"+tc.target` as the `t.Run` name. The pipe character renders cleanly in test output. Fine, but a future test using a spec like `"!=3.0.*|special"` might collide. Minor.

The tests do not lie; the assertions match the names. No invisible side effects in fixtures (all use `httptest.Server`).

## 5. Documentation drift risk

**Blocking — design says one thing, code does another for plan generation.**

The design doc (`docs/designs/DESIGN-pipx-pypi-version-pinning.md`, "Solution Architecture > Components" and Phase 3) commits to:

> `internal/executor/plan_generator.go` (modified, ~30 LOC)
> Before calling e.resolveVersionWith (line 139), scan the recipe's steps. If any step has Action == "pipx_install":
> 1. Call actions.GetEvalDeps("pipx_install") to obtain the eval-dep list (includes "python-standalone").
> 2. Call actions.CheckEvalDeps(...) for those deps. If missing, invoke cfg.OnEvalDepsNeeded (existing path) to install them.
> 3. Call ResolvePythonStandalone() to obtain the binary path.
> 4. Call getPythonVersion(pythonPath) and truncate to major.minor [...]
> 5. Pass pythonMajorMinor through resolveVersionWith into the provider factory

The actual implementation (`internal/executor/executor.go:95-146`) does **none** of (1) and (2). `resolveBundledPythonMajorMinor` is a probe-only function: if `python-standalone` is not installed, it returns `""` and the provider falls back to absolute-latest semantics. The eval-deps install happens later, inside `resolveStep` (`plan_generator.go:331-342`), which fires during step decomposition — after version resolution.

Operational consequence: on a fresh install where `python-standalone` is not yet present, the *first* `tsuku eval` of an ansible recipe will resolve to the absolute-latest version (the bug #2331 was meant to fix), then install python-standalone, then decompose. The user sees the wrong cache key on first run; subsequent runs are correct.

The executor comment at lines 116-122 is honest about this ("the existing CheckEvalDeps flow inside resolveStep will install it later, and resolution will fall back to absolute-latest for this run; subsequent runs probe successfully"). So the *code* is internally consistent and well-documented. The drift is between the **design doc** and the **code**.

**Action:** either update the design doc's Phase 3 deliverables to match the implemented behavior (single probe inside `resolveVersionWith`, no early eval-deps install, first-run fallback acceptable), or land the deferred plan_generator.go work that would make the design accurate. Until one of those happens, the next contributor reading the design doc will form a wrong mental model of where the eval-deps install runs.

**Blocking — prerelease skip is not in the design.** Already flagged in §2. Restating here under documentation drift: the design doc is silent on prerelease behavior, but the code skips them. Either add a paragraph to the design (preferred — the choice has reasoning) or remove the skip if it was unintended.

## Summary table

| # | Finding | Severity |
|---|---|---|
| 2.A | Prerelease skip in `provider_pypi.go:74,113` lacks WHY at the call site and is absent from the design | Blocking |
| 3.A | `NewPyPIProviderForPipx` / `ProviderFromRecipeForPipx` silently accept empty `pythonMajorMinor` with no error or warning; the central implicit contract is under-defended | Blocking |
| 5.A | Design doc's Phase 3 promises early eval-deps install in `plan_generator.go`; code does a no-install probe in `executor.go`. Next contributor will read the design and form a wrong model | Blocking |
| 1.A | `resolveBundledPythonMajorMinor` is a probe, not a resolve; verb is mildly misleading | Advisory |
| 2.B | `resolveBundledPythonMajorMinor` doc claims "downstream CheckEvalDeps will surface a missing python-standalone" without pointing at the file/function | Advisory |
| 2.C | `ListVersions` empty-string semantics under-documented (vs. `ResolveLatest` which states them clearly) | Advisory |
| 3.B | `PipxAwareStrategy` doesn't list its implementers; a future pipx-style strategy may forget to implement it and silently lose filtering | Advisory |
| 4.A | Tests use raw zero-width-space characters in source-file string literals; invisible in most editors | Advisory |

## What's clear and good

- The PEP 440 package boundary is excellent: well-bounded grammar, exhaustive hardening checks documented in the package comment and matched by tests.
- `Canonical()` and the malformed-path tests close the log-injection seam cleanly. Future security review will not need to re-derive the contract.
- Test fixture naming (`ansibleStyleReleases`, `surveyRequirements`) is the kind of provenance-rich naming that helps a new contributor extend coverage without re-doing the L5 research.
- The user-pin bypass is consistent across `ResolveVersion` (provider) and the design doc's data flow section. No drift there.
- The error message format and its security rationale (always render `Canonical()`, never raw bytes) is documented in the security section of the design AND enforced by `TestPyPIProvider_ErrorMessage_RendersCanonicalNotRawBytes`. This is the model a maintainer wants to see.
