# Decision Report: Where the Python-Compat Filter Runs

**Question:** Where does the Python-compat filter run, and how does that
location obtain the bundled Python major.minor?

**Status:** COMPLETE
**Chosen:** Option C (filter inside `pipx_install.Decompose`, after
`ResolvePythonStandalone()` returns a path) -- with a refinement: read the
major.minor by probing the resolved Python binary (reusing `getPythonVersion`).
**Confidence:** High
**Tier:** 3 (standard, fast path)

---

## TL;DR

Filter inside `pipx_install.Decompose`, immediately before the existing
`ResolveLatest`-derived `version` is consumed. Discover the bundled Python
major.minor by running `python3 --version` against the binary returned by
`ResolvePythonStandalone()` -- the same probe already used a few lines later
to populate `python_version` for `pip_exec`. No constants package, no factory
plumbing, no extra recipe metadata.

The filter belongs at the only point in the codebase where (a) we've already
guaranteed python-standalone is installed (eval-deps were just checked) and
(b) we have a concrete binary path whose major.minor we can read at runtime.
Putting it earlier (in the provider factory or in `PyPIProvider`) requires
either plumbing or a hardcoded constant, and the constant would be a lie:
python-standalone is not pinned to a single CPython line.

---

## Verification of premises (before recommending)

### Are there non-pipx PyPI consumers in the codebase today?

**No.** `NewPyPIProvider` has exactly two callers, both in
`internal/version/provider_factory.go`:

- `PyPISourceStrategy.Create` (line 173) -- requires a `pipx_install` step.
- `InferredPyPIStrategy.Create` (line 270) -- requires a `pipx_install` step.

Both strategies' `CanHandle` checks gate on `step.Action == "pipx_install"`.
There is no path by which a non-pipx step constructs a `PyPIProvider` today.
A future non-pipx PyPI consumer would have to thread Python compatibility
context through anyway (it might not even depend on the bundled Python),
so designing the filter for "all PyPI consumers" today is speculative
generality.

### Is python-standalone locked to a single major.minor?

**No.** The recipe at `internal/recipe/recipes/python-standalone.toml`
declares:

```toml
version_format = "custom"

[[steps]]
asset_pattern = "cpython-*+{version}-{arch}-{os}-install_only.tar.gz"
```

The asset pattern uses `cpython-*+{version}-...` where `*` is a glob over
the CPython version (3.10.x, 3.11.x, 3.12.x, 3.13.x...) and `{version}` is
the python-build-standalone release date (e.g., `20251120`). Asset matching
picks the lexicographically-newest matching name (verified in
`internal/version/assets_test.go`), which today resolves to CPython 3.13.x.
When python-build-standalone publishes a 3.14 wheel set, the asset matcher
will silently move to it.

This means a constant `BundledPythonMajorMinor = "3.10"` in code would be
**wrong today** -- the real bundled Python is whichever release the asset
pattern most-recently selected on the user's machine.

### Does the constants approach impose a maintenance contract?

**Yes -- and the contract is unworkable.** A constants approach
(`internal/python/bundled.go`) would require updating tsuku's source
every time python-build-standalone publishes a release that bumps CPython,
which is **out-of-band of any tsuku change**. A user who runs
`tsuku install python-standalone` today gets 3.13.x; the constant in
source might still say 3.10 from when it was first written. Filter results
would diverge from what pip actually sees inside the same install.

The only way to make a constant honest would be to also pin
python-standalone's asset pattern to a single major.minor line (e.g.,
`cpython-3.10.*+...`). That's a separate, larger change with its own
trade-offs (security updates, ecosystem support windows). Not in scope
for this design.

---

## Options Evaluated

### Option A: Filter inside `PyPIProvider`, read bundled Python from a constants package

Construct `PyPIProvider` with a Python major.minor read at construction from
`internal/python/bundled.go` (e.g., `BundledPythonMajorMinor = "3.10"`).
Filter releases by `requires_python` inside `ResolveLatest`/`ListVersions`.

**Strengths:**
- Reaches all PyPI consumers (current and future) automatically.
- `PyPIProvider`'s contract becomes "the latest version compatible with
  the bundled Python," which is intuitive.
- No surgery in plan generation or `Decompose`.

**Weaknesses (decisive):**
- The constant is not honest. python-standalone's recipe does not pin a
  major.minor; it picks the newest CPython release in the
  python-build-standalone catalog. The constant would silently drift from
  reality the moment python-build-standalone publishes a new line.
- Maintenance contract is implicit and external. A python-build-standalone
  release outside tsuku's CI cycle changes user-visible behavior in ways
  the constant does not reflect.
- "Reaches all PyPI consumers" is speculative -- there are zero non-pipx
  consumers today. The breadth is unused.

**Verdict:** Reject. The premise (a stable bundled Python major.minor) does
not hold.

### Option B: Filter inside `PyPIProvider`, plumb bundled Python through factory strategies

Extend `PyPISourceStrategy.Create` and `InferredPyPIStrategy.Create` to look
up the python-standalone recipe and extract its bundled Python somehow,
passing it to `NewPyPIProvider(resolver, pkg, pythonMajorMinor)`.

**Strengths:**
- Same as Option A, but the bundled Python is "looked up" rather than
  hardcoded.

**Weaknesses (decisive):**
- The python-standalone recipe doesn't carry a major.minor either. The
  asset pattern is `cpython-*+...`. The lookup would be reading nothing.
- Even if we added a `python_major_minor` field to the python-standalone
  recipe, that introduces a recipe-side coupling (recipe must declare what
  CPython line it ships) and still doesn't reflect the **installed** Python
  if the user's installation predates a recipe update.
- `PyPIProvider` is constructed at version-resolution time (line 139 of
  `plan_generator.go`), which is **before** eval-deps are checked (line
  331). At that point, there is no guarantee python-standalone is even
  installed; querying its installed binary is unreliable.
- More surgery than Option C with no compensating advantage.

**Verdict:** Reject. The plumbing target (bundled Python at provider
construction) is not knowable at that point in the lifecycle.

### Option C (chosen): Filter inside `pipx_install.Decompose`, after `ResolvePythonStandalone()`

In `internal/actions/pipx_install.go:Decompose`, after the existing
`pythonPath := ResolvePythonStandalone()` call, derive the bundled Python
major.minor by calling the existing `getPythonVersion(pythonPath)` helper
(which runs `python3 --version` against the binary). Then, instead of using
`ctx.Version` directly (which is whatever `PyPIProvider.ResolveLatest`
returned), call a new filter routine that walks the PyPI release list
newest-first, evaluates each release's `requires_python` against the probed
Python major.minor, and returns the first compatible version.

The filter routine becomes the source of truth for the version that
`Decompose` then pins via `pip download <pkg>==<version>` and feeds into the
locked requirements step.

**Strengths:**
- The bundled Python major.minor read here is **the actual installed
  binary's version** -- ground truth, not a constant or recipe-declared
  hint. Self-correcting against python-build-standalone churn.
- Eval-deps have just been checked (line 331-341 of
  `plan_generator.go`); python-standalone is guaranteed installed.
  `ResolvePythonStandalone()` already runs successfully a few lines later
  in the existing code path.
- `getPythonVersion()` already exists and is already invoked in this same
  function (line 337) to populate `pip_exec`'s `python_version` param.
  Reusing it for the filter is one extra read of an already-computed value.
- Smallest blast radius: changes one function. No new package, no factory
  signature change, no recipe schema change.
- Matches pip's behavior exactly: pip's `_check_link_requires_python` uses
  the running Python's version; we use the same Python's version that
  `pip download` will run inside.
- Reproducible: `tsuku eval` produces a concrete version because the filter
  resolves to a single concrete version before `Decompose` returns its
  primitive steps. The plan's `pip_exec` step still embeds an exact pin.
- Honest scoping: the filter applies to pipx_install, which is the only
  PyPI consumer. No pretense that this fixes hypothetical other consumers.

**Weaknesses (acknowledged):**
- If a future non-pipx PyPI consumer is added, it does not automatically
  inherit Python-compat filtering. **Acceptable.** Such a consumer would
  need its own decision about what Python to filter against (it might not
  even use the bundled Python). Adding the filter at `Decompose` time
  doesn't preclude moving it down to the provider later if a real second
  consumer materializes.
- The filter runs once per `tsuku eval`, but `PyPIProvider.ResolveLatest`
  already ran in the version-resolution phase and its result is now
  partially discarded (we query the release list again, or we re-use the
  cached release list). Mild redundancy. Mitigation: have
  `Decompose` call into a `PyPIProvider` method like
  `ResolveLatestCompatibleWith(pythonMajorMinor)`, or have it use the
  resolver directly. Either way, the work is one PyPI JSON fetch (cached).
- Slight asymmetry: `tsuku install foo@2.20` (user-pinned) bypasses the
  filter and may produce an unsatisfiable pin -- same behavior as today,
  just narrower than the auto path. This is **correct** behavior: a user
  pin is an explicit override, and tsuku should not silently substitute
  something else. It surfaces as the existing pip-download error.

**Verdict:** Accept.

### Option D: Read bundled Python dynamically from the python-standalone recipe at filter time

Treat the python-standalone recipe as the source of truth: at filter time,
load it, inspect a (newly-added) `python_major_minor` metadata field, and
use that.

**Strengths:**
- Decouples the filter from the actual installed binary (reproducible
  filter results across machines that have not yet installed
  python-standalone).

**Weaknesses (decisive):**
- The python-standalone recipe currently does not carry a major.minor
  field -- adding one introduces a maintenance burden (must update recipe
  every time python-build-standalone bumps CPython lines) AND can disagree
  with the installed binary on a user's machine that hasn't run
  `tsuku update python-standalone`. Worst of both worlds.
- For the actual `pip download` invocation that runs a few lines later,
  ground truth is the installed binary, not the recipe metadata. Filtering
  against recipe metadata could pick a version that pip then refuses to
  install when run against the actually-installed older Python.
- The "reproducibility across machines" benefit is illusory: the
  decomposed plan is meant to install the tool against the user's
  python-standalone, so disagreement with the installed binary is exactly
  the failure mode we're trying to fix.

**Verdict:** Reject. Inverts the source of truth.

---

## Decision

**Chosen: Option C.**

Implementation outline:

1. In `internal/actions/pipx_install.go:Decompose`, after the existing
   `pythonPath := ResolvePythonStandalone()` (line 318), call
   `pythonVersion, _ := getPythonVersion(pythonPath)` and extract the
   `major.minor` (e.g., `"3.13"` from `"3.13.1"`). The current code already
   calls `getPythonVersion` slightly later (line 337); move that call up
   or perform a second read -- the helper is cheap.
2. Replace direct use of `ctx.Version` (the latest PyPI version from
   `ResolveLatest`) with a filtered version. The filter:
   - Lists PyPI releases for `packageName` (newest-first).
   - For each, evaluates the release's `requires_python` PEP 440 specifier
     against the bundled Python major.minor.
   - Returns the first compatible version.
   - If no version is compatible, returns the original `ctx.Version`
     (preserving today's behavior so the failure surface is identical and
     the user gets pip's familiar error message).
3. The filter routine lives in a new file under `internal/version/`
   (e.g., `pypi_compat.go`) so the PEP 440 specifier evaluator is
   reusable. The PEP 440 evaluator itself is ~50 LOC (per L5 research).
4. Retention of `requires_python` in the PyPI JSON struct (`pypi.go`,
   ~5 LOC) is required so the release list carries the metadata.
5. User-pinned versions (`tsuku install foo@x.y`) flow through
   `ResolveVersion`, not `ResolveLatest`, and are NOT filtered. This
   preserves the principle that an explicit user pin is authoritative.
   `tsuku eval --recipe ansible.toml` (no user pin) gets the auto-filtered
   path.

Where the filter formally "obtains the bundled Python major.minor":
**by probing the binary at `ResolvePythonStandalone()` via the existing
`getPythonVersion` helper.** This is ground truth, runs once per
decomposition, and is already adjacent to where it's needed.

---

## Rationale

The decisive factors, in order:

1. **The premise of any earlier-binding option is false.** python-standalone
   is not locked to a single major.minor line; the asset pattern matches
   any CPython release. Both the constants approach (Option A) and the
   recipe-lookup approach (Option D) require either pinning the recipe or
   accepting silent drift. Neither is in scope.
2. **The runtime probe already exists.** `getPythonVersion` is called
   in this exact function today. The filter needs nothing new beyond a
   second read of an already-computed value (or moving the existing call
   four lines earlier).
3. **Eval-deps guarantee the binary is present.** At `Decompose` time,
   python-standalone has just been verified installed by `CheckEvalDeps`.
   At provider construction time, it has not been. The filter can only run
   reliably in the latter half of plan generation.
4. **Smallest surgery wins.** The constraint "Minimize surgery" rules out
   Option B's factory plumbing. Option C touches one function; everything
   else (PEP 440 evaluator, JSON retention) is additive code in the
   `version` package that supports the filter.
5. **No non-pipx PyPI consumers exist.** The breadth advantage of putting
   the filter in `PyPIProvider` is hypothetical. If a real second consumer
   materializes, the filter can be lifted into the provider with awareness
   of how that consumer obtains its Python context (which may differ).

---

## Assumptions

- python-standalone is installed before `Decompose` runs. Verified: this
  is already required by `pipx_install.Dependencies().EvalTime`, and
  `CheckEvalDeps` runs immediately before `Decompose` in
  `plan_generator.go:331-341`.
- `getPythonVersion(pythonPath)` succeeds for any python-standalone build
  tsuku ships. Verified: it's called in the same function today and is
  not gated by a feature flag; failures fall back to empty string and the
  filter degrades to "use latest" (today's behavior).
- A correct PEP 440 specifier evaluator can be implemented in ~50 LOC
  inside this PR. Per L5 research, this is feasible; `Masterminds/semver`
  does not suffice but a focused implementation does.
- PyPI's `requires_python` metadata is well-populated for the relevant
  packages (ansible, etc.). Per L5: yes, modern releases of long-lived
  projects have it; some pre-2018 packages have nulls. The filter must
  treat null `requires_python` as "compatible" (matching pip's behavior).
- User-pinned versions (`tsuku install foo@x.y`) intentionally bypass
  the filter. This preserves explicit-pin-as-authoritative semantics and
  keeps the change scoped to auto-resolution.

---

## Rejected Alternatives

| Option | Reason Rejected |
|--------|-----------------|
| A: Filter in `PyPIProvider`, constant for bundled Python | Constant would be wrong today; python-standalone is not locked to a single major.minor line; introduces an out-of-band maintenance contract that depends on python-build-standalone releases. |
| B: Filter in `PyPIProvider`, plumb bundled Python through factory | python-standalone is not yet installed at provider construction time; recipe doesn't carry the major.minor anyway; more surgery than the chosen option with no compensating advantage. |
| D: Read bundled Python from python-standalone recipe at filter time | Inverts the source of truth -- the actual installed binary is what `pip download` runs against; recipe metadata can disagree with the installed binary; requires adding a recipe field that needs ongoing curation. |

---

## Consumer Sections

### Design Doc — Considered Options block

> We considered three families of locations for the Python-compat filter:
> inside the version provider with the bundled Python major.minor read
> from a constant or factory-plumbed source; inside the version provider
> with the major.minor looked up from the python-standalone recipe; and
> inside `pipx_install.Decompose` with the major.minor probed from the
> installed `python3` binary.
>
> The provider-level options (A, B, D) all founder on the same point:
> python-standalone is not pinned to a single CPython line. Its asset
> pattern (`cpython-*+{version}-...`) selects whichever CPython release
> python-build-standalone most recently published, currently 3.13.x.
> A constant in source code would silently drift from reality whenever
> python-build-standalone bumps CPython lines. A recipe-declared field
> would face the same drift, and would also disagree with the actual
> installed binary on machines that have not yet updated.
>
> We chose to filter inside `pipx_install.Decompose`, immediately after
> the existing `ResolvePythonStandalone()` call, using the existing
> `getPythonVersion()` helper to read the major.minor from the actually-
> installed binary. This is the only point in the lifecycle where the
> bundled Python's major.minor is both knowable (the binary is guaranteed
> present, since eval-deps were just checked) and authoritative (it's the
> same Python `pip download` will run against a few lines later). The
> change touches one function plus a small reusable PEP 440 specifier
> evaluator and a one-field PyPI JSON struct addition.

### Decision Record (ADR) — Decision and Consequences

> **Decision:** The Python-compat filter for PyPI release selection runs
> inside `internal/actions/pipx_install.go:Decompose`, after the existing
> `ResolvePythonStandalone()` call. The bundled Python's major.minor is
> obtained by calling the existing `getPythonVersion()` helper against
> the resolved python-standalone binary.
>
> **Consequences (positive):** The filter sees the same Python that
> `pip download` will run against, eliminating the class of "tsuku
> resolves a version pip then refuses" failures. No constants package,
> no recipe schema change, no provider factory signature change. The
> filter degrades gracefully to today's behavior when no compatible
> release exists (so error messages remain pip's familiar output).
>
> **Consequences (negative):** A future non-pipx PyPI consumer, if added,
> would not automatically benefit from Python-compat filtering. This is
> acceptable because (a) no such consumer exists today, and (b) a future
> consumer may have its own Python context that differs from
> python-standalone's. If such a consumer is added, the filter can be
> lifted into `PyPIProvider` then with appropriate knowledge of how
> that consumer obtains its Python.

---

## Result Summary (YAML)

```yaml
status: COMPLETE
chosen: "Option C: Filter inside pipx_install.Decompose, after ResolvePythonStandalone(); read bundled Python major.minor by probing the binary via the existing getPythonVersion helper."
confidence: high
rationale: >
  python-standalone is not locked to a single CPython major.minor line, so
  any earlier-binding option (constants package, factory plumbing, recipe
  metadata lookup) would be either wrong-on-arrival or silently drift from
  reality. The Decompose call site is the only point in the lifecycle where
  the bundled Python's major.minor is both knowable (eval-deps guarantee
  the binary is installed) and authoritative (it is the same Python
  pip download will run against a few lines later). The runtime probe
  already exists in the same function. Smallest surgery; honest scoping;
  matches pip's behavior exactly.
assumptions:
  - "python-standalone is installed before Decompose runs (enforced by CheckEvalDeps)."
  - "getPythonVersion(pythonPath) succeeds against any python-standalone build tsuku ships."
  - "A focused PEP 440 specifier evaluator can be implemented in roughly 50 lines (Masterminds/semver does not suffice)."
  - "PyPI requires_python metadata is well-populated for in-scope packages; null values are treated as compatible (matching pip)."
  - "User-pinned versions (tsuku install foo@x.y) intentionally bypass the auto filter and preserve their pin."
rejected:
  - name: "A: Filter in PyPIProvider with a bundled-Python constant"
    reason: "Constant would be wrong today and drift over time; python-standalone is not pinned to a single major.minor; introduces an out-of-band maintenance contract tied to python-build-standalone releases."
  - name: "B: Filter in PyPIProvider with bundled Python plumbed through factory"
    reason: "python-standalone is not yet installed at provider construction; recipe carries no major.minor field to plumb; more surgery for no advantage over Option C."
  - name: "D: Read bundled Python dynamically from the python-standalone recipe at filter time"
    reason: "Inverts the source of truth; the installed binary is what pip download runs against, and recipe metadata can disagree with it; would require adding a curated recipe field."
report_file: "wip/design_pipx-pypi-version-pinning_decision_1_report.md"
```
