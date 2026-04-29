---
status: Planned
problem: |
  tsuku's `pipx_install` action installs Python CLI tools from PyPI, but
  `PyPIProvider.ResolveLatest` always returns the absolute-latest release
  regardless of whether that version is compatible with the Python the
  bundled `python-standalone` recipe ships. When the upstream drops
  support for the bundled Python, tsuku asks `pip download` for an exact
  pin pip cannot satisfy and the eval fails. Pip on its own would have
  walked back to the newest compatible release; tsuku is forcing pip to
  act on an incompatible version. Recipes should not paper over this with
  hardcoded version pins; the design needs a mechanism that consumes
  PyPI's per-release `requires_python` metadata to pick the newest
  Python-compatible release automatically.
decision: |
  Add automatic Python-compatibility filtering inside `PyPIProvider`.
  The provider gains a `pythonMajorMinor` field set at construction
  time; when set, `ResolveLatest` walks PyPI's release list newest-first
  and returns the first release whose `requires_python` is satisfied.
  The executor obtains that major.minor by ensuring `python-standalone`
  is installed (via the existing eval-deps mechanism, surfaced earlier
  in plan generation for `pipx_install` recipes) and probing the
  resolved binary via the existing `getPythonVersion` helper before
  constructing the provider. A new in-tree `internal/version/pep440/`
  package provides a focused PEP 440 specifier evaluator (~250 LOC)
  covering the operators that appear in real PyPI metadata. When no
  release is compatible, the provider returns a typed `*ResolverError`
  (new `ErrTypeNoCompatibleRelease` value appended to the existing
  iota-based enum) with a one-line message naming the package, bundled
  Python, latest release, and its (canonicalized) requirement.
rationale: |
  Probing the installed binary is the only honest source for the bundled
  Python's major.minor — `python-standalone`'s asset pattern matches
  whatever CPython release python-build-standalone most recently
  published, so a constants file or recipe metadata field would be wrong
  on arrival or drift silently. The probe must happen at version-
  resolution time (not at `Decompose` time) because the version
  produced by resolution is the install plan's cache key; if the filter
  ran later, the cache key and the installed version would diverge.
  Surfacing eval-deps for `pipx_install` recipes earlier in plan
  generation lets the executor probe the binary before constructing
  the provider. An in-tree PEP 440 evaluator is preferred over the
  existing `aquasecurity/go-pep440-version` because the surface we
  actually need is small (~250 LOC vs. three new transitive deps) and
  matches tsuku's stated dependency hygiene. Recipes carry no version
  pins; PyPI's upstream metadata is the source of truth, matching
  pip's own selection behavior on the same Python.
---

# DESIGN: pipx PyPI Version Pinning by Python Compatibility

## Status

Planned

## Context and Problem Statement

`pipx_install` recipes today look like:

```toml
[[steps]]
action = "pipx_install"
package = "ansible-core"
executables = ["ansible", "ansible-playbook"]
```

The recipe declares no version. Resolution flows through
`PyPIProvider`, constructed by either `PyPISourceStrategy`
(`internal/version/provider_factory.go:173`) or `InferredPyPIStrategy`
(`internal/version/provider_factory.go:270`), and `ResolveLatest`
returns whatever PyPI's JSON `info.version` says — the absolute
latest release. tsuku then writes that version into the install
plan, and `pipx_install.Decompose`
(`internal/actions/pipx_install.go:283`) calls
`generateLockedRequirements` → `pip download <package>==<version>`
with that exact pin (`internal/actions/pipx_install.go:402, :460`).

Pip refuses the exact pin when the running Python is not in the
release's `Requires-Python` range and exits 1. tsuku's eval crashes
with that exit code before producing a deterministic plan.

The pip behavior is deliberate: `pip install ansible-core` (no
version) on Python 3.10 picks `ansible-core 2.17.14`, walking
backward through releases via `_check_link_requires_python` in
`src/pip/_internal/index/package_finder.py`. Pip already does the
right thing. tsuku breaks that by pre-pinning to the absolute
latest before pip ever sees the request.

The acceptance criteria (#2331) say `tsuku eval` must resolve a
pipx_install recipe to a Python-compatible version on every
supported platform, and `recipes/a/ansible.toml` must validate
under `tsuku validate --strict`. Those criteria do not name a
mechanism; the mechanism is what this design needs to settle.

Five research leads explored the design space and reached
convergence (see `wip/explore_2331-pipx-pypi-version-pinning_findings.md`):

1. **No version provider in tsuku has a recipe-level constraint
   mechanism today.** `VersionSection` carries source identity and
   stable-qualifier metadata but no version range.
   (`internal/recipe/types.go:178-208`)
2. **PyPI's per-release `requires_python` metadata is reliable for
   modern tools.** ansible-core: 313/314 populated. azure-cli
   post-2020: clean. Format variance is real (whitespace, operator
   ordering, four-segment versions, `!=X.*` exclusions); a real
   PEP 440 specifier evaluator is required — `Masterminds/semver`
   does not suffice.
3. **At PyPIProvider construction time, the bundled python-standalone
   version is not in scope.** Construction happens during version
   resolution (executor's pre-plan phase), and python-standalone
   resolution happens during dependency discovery (later, in plan
   generation). However, the bundled Python's major.minor is
   knowable from a constants source: tsuku ships exactly one
   python-standalone line at any given time (currently CPython
   3.10).
4. **azure-cli's reported failure does not reproduce the same way
   as ansible's.** Eval succeeds at 2.85.0; azure-cli's PyPI
   metadata declares `requires_python >= 3.10.0` and the resolution
   path is happy. Its post-install `az --version` failure noted in
   #2331 likely stems from transitive C-extension ABI mismatches,
   not Python compat. azure-cli is therefore deferred to a separate
   follow-up issue; this design does not commit to fixing it.

Open architectural and technical questions for this design:

- Where does the Python-compat filter run? Inside `PyPIProvider`
  (so any caller of `ResolveLatest`/`ListVersions` benefits)? Inside
  `pipx_install.Decompose` (where `ResolvePythonStandalone()` is
  already called)? Both?
- How does `PyPIProvider` learn the bundled Python's major.minor?
  Constants package read at construction? Plumbed through the
  factory strategies? Read directly from the python-standalone
  recipe?
- What PEP 440 subset must the specifier evaluator support?
  Strict subset (`>=`, `>`, `<=`, `<`, `==`, `!=`)? Full PEP 440
  including `~=` and wildcard equality (`==3.0.*`)?
- What should the failure message say when no PyPI release is
  compatible with the bundled Python? Should it name the bundled
  Python version? Should it suggest upgrading the python-standalone
  recipe?
- Do `pipx_install`-only callers want this filtering, or every
  PyPI consumer? (Are there any non-pipx PyPI consumers today?)

## Decision Drivers

- **Recipes must not carry hardcoded versions.** User direction:
  the upstream metadata is the source of truth; tsuku consumes it
  rather than asking authors to mirror it.
- **Match pip's behavior.** Pip on Python 3.10 picks
  `ansible-core 2.17.14` for an unpinned install. tsuku should
  resolve to the same version. Diverging from pip's choice would
  surprise authors and users.
- **Preserve reproducibility.** `tsuku eval` must produce a
  deterministic plan with a concrete version. A "let pip pick at
  install time" approach loses that and breaks the cache-key
  contract documented at `internal/executor/executor.go:104-110`.
- **Minimize surgery.** Provider construction order and the
  decompose-time call into `ResolvePythonStandalone()` already
  exist. The design should slot into existing seams rather than
  re-architecting plan generation.
- **Keep the failure message useful.** When no compatible release
  exists, the error must point to the actual cause (bundled Python
  version too old / package upstream dropped support) rather than
  surfacing a generic pip exit code.
- **Defer azure-cli cleanly.** This design does not need to fix
  azure-cli; it should not be coupled to azure-cli's resolution.

## Decisions Already Made

From the exploration's convergence (see
`wip/explore_2331-pipx-pypi-version-pinning_decisions.md`):

- **Approach: auto Python-compat filter using PyPI's `requires_python`
  metadata.** Manual recipe-level constraints (a `version_constraint`
  TOML field) and hybrid auto+manual approaches are rejected. Recipes
  do not carry version pins.
- **Scope: PyPI provider only.** No symmetry change for github, npm,
  or other providers.
- **azure-cli deferred.** A follow-up issue tracks azure-cli's
  separate failure mode after this design lands; the ansible-core
  recipe is the proof point for this design's acceptance.

## Considered Options

This design decomposes into three independent technical decisions.
Each was evaluated separately; cross-validation reconciled three
inter-decision assumptions (see "Cross-validation reconciliation"
at the end of this section).

### Decision 1: Where the Python-compat filter runs

The filter has to know the bundled Python's major.minor at the
moment it makes the decision. Three families of locations were
considered: inside `PyPIProvider` with the major.minor read from a
constants package; inside `PyPIProvider` with the major.minor
plumbed through factory strategies (potentially read from the
python-standalone recipe); and inside `pipx_install.Decompose`
where `ResolvePythonStandalone()` already runs.

The decisive empirical finding: **python-standalone is not pinned
to a single CPython major.minor line.** Its recipe at
`internal/recipe/recipes/python-standalone.toml` uses
`asset_pattern = "cpython-*+{version}-..."`, where `*` matches any
CPython release in the python-build-standalone catalog. Today's
asset selection picks 3.13.x (verified in
`internal/version/assets_test.go`). Any earlier-binding option
(constants package, recipe-side metadata) would either be wrong
on arrival or drift silently as python-build-standalone publishes
newer CPython lines.

Verified in code: `NewPyPIProvider` has exactly two callers, both
in `internal/version/provider_factory.go` (`PyPISourceStrategy`
and `InferredPyPIStrategy`), and both gate on
`step.Action == "pipx_install"`. There are no non-pipx PyPI
consumers today; the breadth advantage of putting the filter in
the provider is hypothetical.

#### Chosen: Filter inside `PyPIProvider`, bundled Python obtained by runtime probe at construction

The chosen approach evolved during Phase 6 review. The original
proposal placed the filter inside `pipx_install.Decompose`; that
created a cache-key divergence (the executor's
`Executor.ResolveVersion` produced the cache key from
`PyPIProvider.ResolveLatest`, then `Decompose` would have picked a
*different* version, leaving `state.json` and the install
directory disagreeing with the installed binary). The fix is to
move the filter earlier — into `PyPIProvider.ResolveLatest`
itself — and ensure the bundled Python's major.minor is in hand
before that call.

The provider gains a `pythonMajorMinor string` field set at
construction. When non-empty, `ResolveLatest` walks PyPI's release
list newest-first using the new in-tree PEP 440 evaluator (see
Decision 2) and returns the first release whose `requires_python`
is satisfied. The bundled Python's major.minor is supplied by the
executor: for `pipx_install` recipes, plan generation surfaces
the action's eval-deps (which include `python-standalone`) up
front, runs the existing `CheckEvalDeps` flow to install
`python-standalone` if needed, then probes the binary via the
existing `getPythonVersion(pythonPath)` helper (truncating its
full version string to major.minor, e.g., `"3.13.0" → "3.13"`).
The provider factory's `PyPISourceStrategy` and
`InferredPyPIStrategy` accept the major.minor through their
`Create` signatures and pass it into `NewPyPIProvider`.

This keeps the filter at the seam where the cache key is born,
preserves `Decompose` exactly as it is today, and produces the
correct Python at construction time **without** a constants
package, factory metadata lookup, or recipe-side field. The
runtime probe is the only honest source — `python-standalone`'s
asset pattern selects whichever CPython release
python-build-standalone most recently published.

User-pinned versions (`tsuku install foo@x.y`) flow through
`ResolveVersion`, not `ResolveLatest`, and bypass the filter
intentionally. An explicit pin is authoritative even if it
produces an incompatible install (the existing `pip download`
error surfaces).

#### Alternatives Considered

- **A — Filter in `PyPIProvider`, constant for bundled Python**
  (`internal/python/bundled.go: BundledPythonMajorMinor = "3.10"`).
  Rejected: the constant would be wrong on arrival (today's
  python-standalone resolves to 3.13.x, not 3.10) and would drift
  out-of-band of any tsuku change every time
  python-build-standalone publishes a new CPython line. Making the
  constant honest would require also pinning python-standalone's
  asset pattern to a single line — a separate, larger change. The
  chosen option keeps the filter location (inside `PyPIProvider`)
  but supplies the major.minor by runtime probe instead.
- **C — Filter inside `pipx_install.Decompose`** (the original
  recommendation, superseded by Phase 6 review). Rejected: creates
  a cache-key vs. installed-version divergence.
  `Executor.ResolveVersion` runs first and uses the result as the
  cache key (driving `state.json` and the
  `~/.tsuku/tools/<name>-<version>/` directory name). If
  `Decompose` then picks a different version, the directory name
  records one version while pip installs another. The chosen
  option moves the filter back into the version-resolution path
  (where the cache key is born) and supplies the Python via the
  same runtime probe instead of via Decompose.
- **D — Read bundled Python dynamically from the python-standalone
  recipe at filter time.** Rejected: inverts the source of truth.
  The actually-installed binary is what `pip download` runs
  against; recipe metadata can disagree with the installed binary
  on machines that have not yet run `tsuku update python-standalone`.
  Filtering against recipe metadata could pick a version pip then
  refuses to install.

### Decision 2: PEP 440 specifier subset

The filter consumes PyPI's per-release `requires_python` strings
(e.g., `>=3.11`, `>=2.7,!=3.0.*,!=3.1.*,<4`) and answers "does
Python X.Y satisfy this specifier?" A correct evaluator is
required because format variance is real: an empirical survey
across 14 popular pipx-style tools (poetry, black, mypy, ruff,
flake8, pylint, isort, tox, pipx, httpx, requests, django, numpy,
pandas) found 96 unique `requires_python` strings, 182 clauses,
with whitespace variance, operator-order variance, four-segment
versions (`3.6.0.0`), and `!=X.Y.*` wildcard exclusions all
present. `Masterminds/semver` (already in tsuku's dependencies)
does not suffice — it rejects four-segment versions, rejects
`!=3.0.*` exclusions, and misorders RC strings.

#### Chosen: In-tree minimal evaluator

A new package `internal/version/pep440/` (~250 LOC implementation,
~150 LOC tests) supports the operators and forms that actually
appear in modern PyPI metadata:

- Operators: `>=`, `<=`, `>`, `<`, `==`, `!=`
- Combination: comma-joined AND
- Wildcards: `==X.Y.*` and `!=X.Y.*`
- Versions: 1- to 4-segment integer (`3`, `3.6`, `3.6.2`, `3.6.0.0`),
  missing trailing components treated as 0

Empirically this covers 181 of 182 clauses (99.4%) across the
sampled tools. Operators not seen in the survey (`===`, `~=`,
local versions) are rejected with a clear, wrapped error naming
the offending clause and package.

When the evaluator encounters an unsupported operator on a
specific release, that release is treated as incompatible (skipped
in the candidate walk) — see "Cross-validation reconciliation"
below. If every release of a package is unevaluable, the failure
falls through to Decision 3's typed error.

#### Alternatives Considered

- **A — Hand-rolled minimal subset, no wildcards** (`>=`, `<=`,
  `>`, `<`, `==`, `!=`, AND only). Rejected: 73 of 182 observed
  clauses (40%) use `!=X.Y.*` exclusions. Refusing them would
  break newest-compatible selection on requests, numpy, pandas,
  poetry, flake8, tox, pylint, isort, and cookiecutter — the
  long-tail of widely-installed tools.
- **B — Full PEP 440** (adds `~=`, `===`, local versions, prefix
  matching). Rejected: `===` and local versions never appear in
  observed `requires_python` data; `~=` appears once across the
  full sample (a single pylint clause). Implementing them is
  dead code with a real test-matrix tax.
- **C — `github.com/aquasecurity/go-pep440-version`.** Rejected:
  maintained (last commit 2026-02-24), Apache-2.0, complete — a
  respectable choice. But it adds three direct dependencies
  (`aquasecurity/go-version`, `golang.org/x/xerrors`,
  `stretchr/testify`) plus transitives for ~250 LOC of focused
  work. Tsuku's stated convention is "Go stdlib + minimal deps
  with justification"; the marginal correctness gain over a
  focused implementation does not clear that bar. The other
  available Go option (`quay/claircore/pkg/pep440`) is strictly
  worse — no wildcards, no `===`.

### Decision 3: Failure-message contract when no release is compatible

The branch fires only when **every** PyPI release of the package
requires a Python newer than the bundled major.minor (with `null`
`requires_python` treated as compatible per pip's behavior). This
is rare but real for packages whose oldest release predates the
bundled Python line.

The bundled Python is fixed by tsuku's CLI distribution; the user
cannot change it via env var, recipe field, or CLI flag.
Constraint 4 (out of scope: a force-install flag) excludes
`--ignore-requires-python`-style escape hatches; analysis surfaced
no strong case to add one (a force flag produces broken installs
that fail at first import, with no benefit over a manual
`pip install` outside tsuku).

#### Chosen: Typed pre-flight error with concise wording

`PyPIProvider.ResolveLatestCompatibleWith(pythonMajorMinor)`
returns a typed `*ResolverError` with a new
`ErrTypeNoCompatibleRelease` classification when the candidate
walk yields no compatible release. The error message follows the
codebase's existing terse tone:

```
pypi resolver: no release of <package> is compatible with bundled Python <X.Y> (latest is <V>, requires Python <Z>)
```

Concrete example for ansible-core on bundled Python 3.10:

```
pypi resolver: no release of ansible-core is compatible with bundled Python 3.10 (latest is 2.20.5, requires Python >=3.12)
```

The `pypi resolver:` source prefix is automatic from
`ResolverError.Error()`. The message names the package, the
bundled Python (so the user understands they cannot reconfigure
it), the latest release, and its `Requires-Python`. The error
propagates through the existing
`Executor.ResolveVersion` → CLI surface chain unchanged;
`tsuku eval` and `tsuku install` exit non-zero with the message
on stderr.

This pre-flight error replaces today's failure mode, where
`pip download` runs and emits its own verbose enumeration. Pip's
detailed output is still available to anyone running pip directly;
tsuku's contribution is a tight pre-flight signal.

#### Alternatives Considered

- **B — A plus actionable suggestion** ("may need a follow-up
  issue if the package supports older Python via a backport
  branch"). Rejected: the suggestion is incorrect when the branch
  fires. With the auto-filter active, reaching this error means
  no compatible release exists at all — a backport would have to
  be a *new* PyPI release the package author publishes.
  Encouraging issue-filing belongs in higher-level documentation,
  not in an edge-case error message.
- **C — A plus the top-3 compatible versions.** Rejected: the
  compatible set is empty by definition for this branch, so the
  list would always be empty.
- **D — Pip-style enumeration of every release with its
  Requires-Python.** Rejected: verbose, breaks tsuku's terse-error
  tone, duplicates content pip's own stderr produces in the rare
  cases users want it (they can run `pip install` directly), and
  adds non-trivial implementation cost for marginal benefit on a
  rare branch.

### Cross-validation reconciliation

Three inter-decision assumptions surfaced during cross-validation,
plus one design correction during Phase 6 architecture review:

1. **D2 assumed a "recipe-author override path established in
   Decision 1."** No such override exists. Resolution: when the
   PEP 440 evaluator encounters an unsupported operator or
   malformed clause for a specific release, that release is
   treated as incompatible (skipped). If every release is
   unevaluable, the failure surfaces as Decision 3's typed error.
   No new recipe-side mechanism is added.
2. **D1 originally proposed falling back to "use the original
   `ctx.Version`" (let pip emit its own error) when no compatible
   release exists.** D3's pre-flight typed error supersedes this.
   The provider returns the typed error before `pip download` is
   invoked. This gives users a tight, tsuku-tone error in the
   rare case it fires, without depending on pip's verbose stderr.
3. **D3 assumed the filter would live in `PyPIProvider`; D1
   originally chose `pipx_install.Decompose`.** D3's assumption
   was correct. Phase 6 architecture review caught that filtering
   in `Decompose` creates a cache-key divergence (the install
   plan's cache key is born from `ResolveLatest`, before
   `Decompose` runs). Resolution: filter moved back into
   `PyPIProvider.ResolveLatest`, with the bundled Python's
   major.minor supplied by the executor as a constructor field.
   The probe still uses the existing `getPythonVersion` helper;
   the executor calls it after surfacing `python-standalone`'s
   eval-dep check earlier in plan generation.
4. **Phase 6 architecture review found that `pypiPackageInfo.Releases`
   is currently `map[string][]struct{}` (an empty struct slice).**
   Capturing per-release `requires_python` requires replacing the
   empty struct with a typed file struct. The implementation
   surface is ~15 LOC across `pypi.go` (struct change, field
   wiring) plus updates to existing `pkgInfo.Releases` references
   in the same file (loops at `pypi.go:194-198`).

## Decision Outcome

The three decisions compose into one coherent change at the
version-resolution seam. For `pipx_install` recipes, the executor
surfaces the action's `python-standalone` eval-dep up front in
plan generation: it runs the existing `CheckEvalDeps` flow before
constructing the version provider, then probes the installed
binary via `getPythonVersion(pythonPath)` to obtain
`pythonMajorMinor` (truncating the helper's full version string,
e.g., `"3.13.0" → "3.13"`). The provider factory's
`PyPISourceStrategy` and `InferredPyPIStrategy` accept the
major.minor and pass it into `NewPyPIProvider`, which stores it
on the provider struct. `ResolveLatest` then walks PyPI's release
list newest-first, evaluates each release's `requires_python`
with the new in-tree PEP 440 evaluator, and returns the first
compatible release. That version becomes the install plan's cache
key, the directory name in `~/.tsuku/tools/`, and the version
that `pipx_install.Decompose` pins into the existing
`pip download <package>==<version>` call. **`Decompose` itself
does not change** — the existing `getPythonVersion` call stays
where it is for `pip_exec`'s `python_version` parameter, and the
filter has already happened upstream.

When no compatible release exists — a rare branch reachable only
when every release of a package requires a newer Python than
tsuku ships — the provider returns a typed `*ResolverError`
(value `ErrTypeNoCompatibleRelease` appended to the existing
iota-based `ErrorType` enum at
`internal/version/errors.go:15`) naming the package, the bundled
Python, the latest release, and a canonicalized form of its
`Requires-Python` string (never the raw bytes from PyPI; see
Security Considerations). `tsuku eval` and `tsuku install` exit
non-zero with that message on stderr; no `pip download`
invocation runs.

The change touches three call sites in version resolution
(`provider_factory.go` strategies, `provider_pypi.go` constructor
and `ResolveLatest`, `plan_generator.go` for the early eval-dep
surface), modifies one struct (`pypi.go`'s `pypiPackageInfo` to
retain `requires_python` per release), adds one focused package
(`internal/version/pep440/`), and adds one new value to the
`ErrorType` enum. No recipe schema change, no constants package,
no new dependencies. Recipes carry no version pins; PyPI's
upstream metadata is the source of truth. The golden-file branch
in `pipx_install.Decompose` (`ctx.Constraints != nil &&
ctx.Constraints.PipRequirements != ""`, line ~313) is unchanged
— constrained evaluations bypass live PyPI resolution entirely.

## Solution Architecture

### Overview

A new in-tree PEP 440 specifier evaluator filters PyPI's release
list by `requires_python` against the installed `python-standalone`
binary's major.minor. Filtering happens inside
`PyPIProvider.ResolveLatest` so that the resolved version becomes
the install plan's cache key. The major.minor reaches the
provider via a new constructor parameter, supplied by the
executor after surfacing the `pipx_install` action's eval-deps
earlier in plan generation.

### Components

```
internal/version/pep440/        (new package, ~250 LOC + ~200 LOC tests)
  version.go     -- Version parsing: 1- to 4-segment integer; missing
                    components default to 0; "rc"/"a"/"b" suffixes parsed
                    but ignored for specifier matching.
  specifier.go   -- Parse(s string) → Specifier{ clauses []clause }.
                    Entry checks reject input failing any of:
                      - total length cap (1024 bytes)
                      - clause count cap (32)
                      - per-clause length cap (256 bytes)
                      - ASCII-only validation (any byte > 0x7F → reject)
                      - segment-magnitude cap (>6 digits or > MaxInt32)
                    Tokenize on commas, TrimSpace each part, match leading
                    operator (longest-prefix). Supported operators:
                    >=, <=, >, <, ==, != (plus ==X.Y.* / !=X.Y.* wildcards).
                    Reject ~= and === with ErrUnsupportedOperator wrapping
                    the offending clause and source package.
                    Canonical(s string) string returns a sanitized,
                    parseable form (or "<malformed>" on rejection) for
                    use in error messages — never the raw bytes.
  match.go       -- (Specifier).Satisfies(target Version) bool.
                    Per-clause integer compare; AND semantics across clauses;
                    wildcard prefix-compare.
  pep440_test.go -- Table-driven tests seeded with the 96 requires_python
                    strings from the L5 survey (poetry, ansible, black,
                    mypy, ruff, flake8, pylint, isort, tox, pipx, httpx,
                    requests, django, numpy, pandas), plus negative tests
                    for each input-hardening check.

internal/version/pypi.go        (modified, ~15 LOC)
  pypiPackageInfo.Releases changes from
    map[string][]struct{}            (current, file array intentionally empty)
  to
    map[string][]pypiReleaseFile      (new)
  with
    type pypiReleaseFile struct { RequiresPython string `json:"requires_python"` }
  Existing loops at pypi.go:194-198 are updated to compile against the
  new shape (they read keys, not file contents, so the change is
  mechanical). The existing 10 MB response cap (maxPyPIResponseSize) is
  unchanged.

internal/version/provider_pypi.go        (modified, ~40 LOC)
  PyPIProvider gains a `pythonMajorMinor string` field and a
  constructor variant:
    NewPyPIProviderForPipx(resolver *Resolver, pkg, pythonMajorMinor string) *PyPIProvider
  When pythonMajorMinor is set, ResolveLatest:
    1. Loads the parsed PyPI response (uses the same cache as today —
       no duplicate HTTP fetch).
    2. Walks releases newest-first using the existing version-sort
       comparator.
    3. For each release: read requires_python (any one file's value;
       all files share); empty/null treated as compatible. Parse via
       pep440.ParseSpecifier; on parse error, treat as incompatible
       (skip).
    4. Return first compatible.
    5. If none compatible: return *ResolverError with
       Type = ErrTypeNoCompatibleRelease and message naming
       package, bundled Python, latest version, and the
       canonicalized requires_python.

  ListVersions output is filtered the same way when pythonMajorMinor is set.
  When pythonMajorMinor is empty, the provider behaves exactly as today.

internal/version/errors.go        (modified, ~3 LOC)
  Append ErrTypeNoCompatibleRelease as a new value to the existing
  iota-based ErrorType enum at internal/version/errors.go:15.

internal/version/provider_factory.go        (modified, ~25 LOC)
  PyPISourceStrategy and InferredPyPIStrategy gain an optional
  pythonMajorMinor input via the strategy Create signature (or via a
  field on the strategy struct populated before Create is called by
  the factory's caller). When set, both strategies call
  NewPyPIProviderForPipx instead of NewPyPIProvider.

internal/executor/plan_generator.go        (modified, ~30 LOC)
  Before calling e.resolveVersionWith (line 139), scan the recipe's
  steps. If any step has Action == "pipx_install":
    1. Call actions.GetEvalDeps("pipx_install") to obtain the
       eval-dep list (includes "python-standalone").
    2. Call actions.CheckEvalDeps(...) for those deps. If missing,
       invoke cfg.OnEvalDepsNeeded (existing path) to install them.
    3. Call ResolvePythonStandalone() to obtain the binary path.
    4. Call getPythonVersion(pythonPath) and truncate to major.minor:
         parts := strings.SplitN(full, ".", 3)
         pythonMajorMinor := parts[0] + "." + parts[1]
    5. Pass pythonMajorMinor through resolveVersionWith into the
       provider factory (new signature or context field).

internal/actions/pipx_install.go        (UNCHANGED)
  No edits required. Decompose continues to use ctx.Version, which is
  now the python-compatible version produced upstream.

recipes/a/ansible.toml        (new, ~30 lines)
  pipx_install recipe for ansible-core. Curated proof point.
  Will resolve to ansible-core 2.17.x under bundled Python 3.10.
```

### Key Interfaces

**PEP 440 evaluator (`internal/version/pep440`):**

```go
// Version represents a PEP 440-style version as integer components.
// 1-4 segments accepted; missing components default to 0.
type Version []int

func ParseVersion(s string) (Version, error)
func (v Version) Compare(other Version) int  // -1, 0, 1
func (v Version) String() string

// Specifier represents one or more PEP 440 specifier clauses.
type Specifier struct { /* ... */ }

func ParseSpecifier(s string) (Specifier, error)
// Returns ErrInputTooLong (>1024 bytes total or >32 clauses or any
// clause >256 bytes), ErrNonASCII (any byte > 0x7F), ErrSegmentTooLarge
// (any segment >6 digits or > math.MaxInt32), ErrUnsupportedOperator
// (~=, ===), or ErrMalformed (other). All wrap the offending token +
// source context.

func (s Specifier) Satisfies(target Version) bool

// Canonical returns a sanitized, ASCII-only canonical form of a
// requires_python string suitable for inclusion in error messages.
// Returns "<malformed>" if the input fails any input-hardening check.
// Never returns raw upstream bytes.
func Canonical(s string) string
```

**PyPI provider (`internal/version/provider_pypi.go`):**

```go
type PyPIProvider struct {
    resolver         *Resolver
    packageName      string
    pythonMajorMinor string  // empty when not constructed for pipx
}

// NewPyPIProvider remains unchanged for backward compatibility. The
// pipx-specific path uses the variant below.
func NewPyPIProvider(resolver *Resolver, pkg string) *PyPIProvider

// NewPyPIProviderForPipx constructs a provider that filters releases
// by requires_python against pythonMajorMinor (e.g., "3.10").
//
// ResolveLatest then walks releases newest-first. For each release:
//   - Empty/null requires_python → treated as compatible (matches pip).
//   - Parseable specifier and Satisfies(python) → return.
//   - Unparseable specifier or unsupported operator → release is
//     skipped (treated as incompatible).
//
// On no compatible release found: returns a *ResolverError with
// Type = ErrTypeNoCompatibleRelease and a message of the shape:
//   "no release of <package> is compatible with bundled Python <X.Y>
//    (latest is <V>, requires Python <pep440.Canonical(Z)>)"
//
// User-pinned versions go through ResolveVersion, not ResolveLatest,
// and bypass filtering intentionally.
func NewPyPIProviderForPipx(resolver *Resolver, pkg, pythonMajorMinor string) *PyPIProvider
```

**Error contract (`internal/version/errors.go`):**

```go
// Append to the existing iota block at internal/version/errors.go:15.
// The const block already declares ErrTypeNetwork, ErrTypeNotFound,
// ErrTypeParsing, etc. — same convention.
const ErrTypeNoCompatibleRelease // appended after the existing values

// *ResolverError formats as "<source> resolver: <message>", so the
// user sees:
//   "pypi resolver: no release of ansible-core is compatible with
//    bundled Python 3.10 (latest is 2.20.5, requires Python >=3.12)"
```

### Data Flow

```
tsuku eval --recipe recipes/a/ansible.toml
  │
  └─► PlanGenerator.GeneratePlan (modified)
        │   1. Recipe scanned for pipx_install steps. Found.
        │   2. CheckEvalDeps("pipx_install") → install python-standalone
        │      if missing (existing OnEvalDepsNeeded path).
        │   3. pythonPath := ResolvePythonStandalone()
        │   4. full := getPythonVersion(pythonPath)  // e.g., "3.13.0"
        │      pythonMajorMinor := "3.13"
        │   5. Provider factory constructs PyPIProvider with
        │      pythonMajorMinor set.
        │
        ▼
      Executor.ResolveVersion (existing flow, modified provider)
        │   provider.ResolveLatest():
        │     2.20.5 → requires_python ">=3.12" → not 3.10 → skip
        │     2.20.4 → ">=3.12" → skip
        │     ...
        │     2.17.14 → ">=3.10" → satisfies → return 2.17.14
        │   versionInfo.Version = "2.17.14"  ← becomes the cache key
        │
        ▼
      PlanGenerator continues (unchanged)
        │   e.version = "2.17.14"; vars["version"] = "2.17.14";
        │   evalCtx.Version = "2.17.14"
        │
        ▼
      pipx_install.Decompose (UNCHANGED)
        │   ctx.Version is "2.17.14"; pip download ansible-core==2.17.14
        │
        ▼
      Install proceeds with cache key, install dir name, and installed
      version all = "2.17.14". No divergence.
```

User-pinned versions (`tsuku install ansible@2.20`) flow through
`Executor.ResolveVersion` with the user's constraint and call the
provider's `ResolveVersion` (not `ResolveLatest`). The pin bypass
is intentional: an explicit pin is authoritative even if it produces
an incompatible install.

For the `ctx.Constraints` (golden-file) branch in
`pipx_install.Decompose`: this branch already has the version
baked into `PipRequirements` and bypasses live PyPI resolution
entirely, so it is unaffected by this change.

## Implementation Approach

Build order is constrained by dependencies (later phases depend on
earlier ones).

### Phase 1: PEP 440 specifier evaluator

Standalone, no dependencies on other phases. Lands as its own
package with table-driven tests. The 96 survey strings from the
L5 research become the golden test table. Five input-hardening
checks are enforced at `ParseSpecifier` entry:

- Total specifier length cap of 1024 bytes
- Clause count cap of 32
- Per-clause length cap of 256 bytes
- ASCII-only byte validation (any byte > 0x7F rejected)
- Segment-magnitude cap (>6 digits or `> math.MaxInt32` rejected)

Each check has a dedicated negative test case. The package also
exports a `Canonical(s string) string` helper that returns a
sanitized, ASCII-only parseable form of a `requires_python`
string for safe inclusion in error messages — and `"<malformed>"`
when the input fails any hardening check.

Deliverables:
- `internal/version/pep440/version.go`
- `internal/version/pep440/specifier.go` (includes the five
  input-hardening checks and `Canonical`)
- `internal/version/pep440/match.go`
- `internal/version/pep440/pep440_test.go` (positive cases from
  the L5 survey table; negative cases for each hardening check;
  `Canonical` round-trip cases for the survey strings; `Canonical`
  malformed-input cases for each rejection path)

### Phase 2: PyPI provider integration

Replaces `pypiPackageInfo.Releases` from `map[string][]struct{}`
to a typed file struct that retains `requires_python`. Updates
existing loops in `pypi.go` to compile against the new shape (the
loops at `pypi.go:194-198` only read keys today, so the change is
mechanical). Adds the new `ErrTypeNoCompatibleRelease` value to
the iota-based enum. Adds `pythonMajorMinor` to the
`PyPIProvider` struct and the `NewPyPIProviderForPipx`
constructor variant. Modifies `ResolveLatest` (and `ListVersions`)
to filter when `pythonMajorMinor` is set. Tests reuse the
`httptest`-based fixture pattern already present in
`provider_pypi_test.go`.

Deliverables:
- `internal/version/pypi.go` (struct change for `requires_python`
  retention)
- `internal/version/errors.go` (append `ErrTypeNoCompatibleRelease`
  to the existing iota block)
- `internal/version/provider_pypi.go` (`pythonMajorMinor` field,
  new constructor, modified `ResolveLatest` and `ListVersions`)
- `internal/version/provider_pypi_test.go` (cases: latest compatible
  in middle of list; no compatible release at all; null
  `requires_python` treated as compatible; unparseable specifier
  on a release skipped without aborting the walk; user-pin path
  unaffected; error message renders `Canonical(Z)`, never raw bytes)

### Phase 3: Provider factory + executor wiring + ansible recipe

Threads `pythonMajorMinor` from the executor through the factory
into the provider. Adds the early eval-deps check for
`pipx_install` recipes in `plan_generator.go`. Lands the proof-
point recipe. `pipx_install.go` itself is unchanged. End-to-end
behavior is verified via `tsuku eval --recipe
recipes/a/ansible.toml --os linux --arch amd64` resolving to
`ansible-core 2.17.x` under the bundled Python (whichever
major.minor python-build-standalone currently ships).

Deliverables:
- `internal/version/provider_factory.go` (`PyPISourceStrategy` and
  `InferredPyPIStrategy` accept `pythonMajorMinor` and route to
  `NewPyPIProviderForPipx` when set)
- `internal/executor/plan_generator.go` (early eval-deps check
  for `pipx_install` recipes; binary probe; major.minor truncation;
  pass to provider factory)
- `recipes/a/ansible.toml` (curated, with `curated = true`)
- Tests: factory test for the new strategy parameter; plan-generator
  test for the early eval-deps path; integration test running
  `tsuku eval --recipe recipes/a/ansible.toml`
- Validation passes via `tsuku validate --strict --check-libc-coverage`

### Phase 4 (optional, deferred): azure-cli investigation

Not in this design's PR. After Phase 3 lands, a separate follow-up
investigates whether azure-cli's failure is Python-compat (solved
by this design) or a transitive-dep ABI issue (separate fix
required).

## Security Considerations

The change consumes additional fields from PyPI's existing JSON
response and runs new in-tree string parsing. Concrete review:

- **Untrusted input source.** PyPI metadata is fetched from an
  external service over HTTPS. The existing provider already
  treats this as untrusted (parses JSON, validates fields,
  returns typed errors on malformed responses). Adding
  `requires_python` retention does not introduce a new input
  source — it consumes one more field from the same response.
  The response is capped at 10 MB
  (`maxPyPIResponseSize`, `internal/version/pypi.go:18`); the
  parser inherits that bound on its untrusted-input budget.

- **PEP 440 parser as attack surface.** The parser uses byte-level
  scanning (no regex, no recursion). Mitigations beyond the
  bounded grammar, all enforced at `ParseSpecifier` entry:
  - **Total specifier length cap of 1024 bytes.** Closes the
    "many small clauses" gap — a per-clause cap alone could be
    bypassed by packing thousands of short clauses into a single
    specifier.
  - **Clause count cap of 32.** Real-world specifiers have 1–4
    clauses; 32 is a generous ceiling that fast-fails on packing
    attacks before per-clause work begins.
  - **Per-clause length cap of 256 bytes.** Real-world clauses
    are <64 bytes; longer inputs are rejected as malformed.
  - **ASCII-only validation.** Any byte > 0x7F is rejected. PyPI's
    `requires_python` is ASCII in practice; this prevents Unicode
    confusable inputs from reaching the operator matcher.
  - **Segment-magnitude cap.** Version segments above 6 digits (or
    `math.MaxInt32`) are rejected as malformed, preventing
    integer overflow in comparison.
  - Output is `[]int` plus a small struct — no execution path,
    no eval. Worst-case parser cost is linear in input length.

- **Error-message rendering of `requires_python`.** The
  `ErrTypeNoCompatibleRelease` message includes the latest
  release's `Requires-Python` (the `<Z>` slot). To prevent log
  injection or terminal-escape smuggling via attacker-controlled
  PyPI metadata, the message **always** renders `<Z>` through
  `pep440.Canonical(...)`, which produces a sanitized ASCII-only
  parseable form or the literal string `"<malformed>"` when the
  input fails any input-hardening check. The raw upstream bytes
  never reach the error stream.

- **No new secrets, no new credentials, no new file writes.** The
  new code path is read-only against PyPI and the installed
  Python binary.

- **Subprocess call to `python --version`.** Already done today
  by `getPythonVersion(pythonPath)`. The path comes from
  `ResolvePythonStandalone()`, which constructs the path inside
  `$TSUKU_HOME/tools/python-standalone-*` via `filepath.Join`
  over directories tsuku itself creates. `exec.Command` invokes
  the binary directly with a literal `--version` arg — no shell,
  no argv injection. Not a new subprocess invocation surface.

- **Error messages.** The typed error includes the package name,
  bundled Python version, latest release, and `Requires-Python`.
  None of these fields contain secrets or paths; all are public
  PyPI metadata. The wording template is plain ASCII.

- **Recipe-side trust model unchanged.** Recipes do not declare
  Python version, do not declare specifier strings, and gain no
  new privilege. The auto-filter consumes only upstream PyPI
  metadata. User pins (`tsuku install foo@x.y`) bypass the filter
  intentionally; pins are a CLI-side mechanism, so a recipe
  cannot inject one.

- **DoS via crafted `requires_python`.** A malicious PyPI
  response declaring "no release is compatible" causes the
  typed pre-flight error to fire and tsuku to exit non-zero
  per-package. The "attack" is per-package and per-mirror; an
  attacker who controls PyPI for a package already has stronger
  compromise primitives (e.g., publishing a malicious version).
  No cross-package poisoning, no quadratic walks, no resource
  amplification.

No new privileged operations, no new credential surface, no new
input source beyond the existing PyPI JSON response. The PEP 440
parser is a small in-tree component with a bounded grammar,
bounded input length, and clear failure modes.

## Consequences

### Positive

- **`pipx_install` recipes resolve correctly under the bundled
  Python.** ansible-core picks 2.17.14 instead of 2.20.5; the
  same logic applies automatically to any future tool whose
  upstream drops support for the bundled Python.
- **Recipes carry no version pins.** Recipe authors don't have
  to mirror PyPI's `requires_python` metadata in TOML, and
  recipes never go stale on patch releases.
- **Cache key, install directory, and installed version agree.**
  Filtering at the version-resolution seam (not in
  `Decompose`) means the version produced by `ResolveLatest` is
  the version pip installs. `state.json`, the
  `~/.tsuku/tools/<name>-<version>/` directory, and the binary
  inside all match.
- **Matches pip's own behavior.** A user running
  `pip install ansible-core` outside tsuku on the same Python
  picks the same version tsuku does. No surprise divergence.
- **Self-correcting against `python-standalone` updates.** When
  tsuku eventually ships a newer python-standalone (CPython 3.11,
  3.12, etc.), every `pipx_install` recipe automatically picks
  newer releases without anyone touching recipe files or constants.
- **No new dependencies, no recipe schema change, no constants
  package.** Surgery is contained to the version-resolution path
  and the new evaluator package.
- **Pre-flight error replaces opaque pip failure** in the rare
  no-compatible-release case.

### Negative

- **`pipx_install` eval-deps surface earlier in plan generation.**
  Today, `CheckEvalDeps` runs inside the per-step loop in
  `resolveStep`. The new flow runs it for `pipx_install` before
  version resolution so python-standalone is installed before
  the provider is constructed. This is a small re-order rather
  than a new install path, but it changes the `tsuku eval` UX
  for recipes that previously could resolve a version before
  python-standalone was present (they can no longer).
- **A future non-pipx PyPI consumer would not automatically
  get Python-compat filtering.** The factory only sets
  `pythonMajorMinor` for pipx-action contexts. A hypothetical
  second consumer would need its own decision about what Python
  to filter against and would route through `NewPyPIProvider`
  (the existing constructor) by default.
- **PEP 440 evaluator is a new piece of code to maintain.**
  Format variance in PyPI's metadata could surface unsupported
  clauses over time (e.g., `~=` adoption). Mitigation:
  clause-rejection is loud (clear error in logs), and the test
  surface is table-driven so adding a new operator is a
  localized change.
- **Filter results depend on the actually-installed binary's
  major.minor.** A user who never updates `python-standalone`
  while tsuku ships a recipe expecting a newer line will see
  resolution mismatches. This is the correct behavior (the
  install must match what's installed) but means recipes can't
  unilaterally bump the python-standalone target.
- **`tsuku eval` becomes slightly slower** for `pipx_install`
  recipes because the filter walks the release list.
  Quantified: one already-cached PyPI JSON read plus N PEP 440
  evaluations (typically <10 to find the compatible one).
  Negligible in practice.

### Mitigations

- The new package boundary (`internal/version/pep440/`) makes
  future operator additions localized.
- Table-driven tests against the L5 survey strings detect
  regression on real-world `requires_python` formats.
- Logging for unsupported-operator events surfaces format drift
  early (visible in `tsuku eval --debug` output if needed).
- The pre-flight error guides users to the actual cause rather
  than pip's verbose stderr; future doctor commands or telemetry
  can `errors.As` on `ErrTypeNoCompatibleRelease` if needed.
