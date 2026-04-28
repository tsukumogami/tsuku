---
status: Proposed
problem: |
  tsuku's `pipx_install` action installs Python CLI tools from PyPI but always
  resolves to the absolute latest released version. When that latest version
  drops support for the Python that tsuku's `python-standalone` recipe ships
  (currently CPython 3.10.20), the resulting plan fails to materialize: pip's
  download step rejects every newer release with `Requires-Python >=3.11`
  (or `>=3.12`), and tsuku's eval crashes before producing a deterministic
  plan. ansible-core and azure-cli, both deferred from #2296, hit this wall
  in PR #2329 and have remained uncuratable since. The recipe schema today
  has no syntax for "give me the latest version of X that satisfies version
  range Y," so the recipe author cannot work around the upstream's drop of
  support for the bundled Python.
decision: |
  Add an optional `version_constraint` parameter to the `pipx_install`
  action. The constraint is a comma-joined list of PEP 440 specifiers
  (e.g., `>=2.17,<2.18` or `>=1.0`). When present, the PyPI version
  provider filters its list of available releases to versions matching
  the constraint and returns the highest matching version. The default
  (no constraint) preserves the current behavior of always picking the
  latest. Constraint syntax is a small subset of PEP 440 — `>=`, `>`,
  `<`, `<=`, `==`, `!=`, comma-joined — covering every real-world need
  the immediate consumers express. Wildcard equality (`==1.4.*`) and
  compatible-release (`~=`) are deferred to v2 if a real consumer
  motivates them.
rationale: |
  A manual constraint is the smallest mechanism that solves the stated
  problem. It mirrors the version-pinning idiom that recipe authors
  already know from cargo, npm, pip, and similar package managers, so
  there is no new mental model. It ships in roughly 50 lines of
  focused Go (constraint parser, semver-style filter against the
  existing PyPI version list, plumbing through pipx_install). It does
  not regress: the default of "latest" is unchanged for recipes that
  do not opt in. Automatic filtering of releases by the bundled
  Python's `Requires-Python` was considered as a more "correct"
  alternative but rejected for v1: (a) its implementation cost is
  several times larger because tsuku does not currently have access
  to the resolved python-standalone version inside the PyPI provider
  at construction time, and (b) the manual mechanism is sufficient to
  unblock the deferred recipes today. Automatic filtering remains a
  reasonable v2 layered on top of the manual constraint.
---

# DESIGN: pipx_install Version Constraint

## Status

Proposed

## Context and Problem Statement

`pipx_install` recipes today look like:

```toml
[[steps]]
action = "pipx_install"
package = "black"
executables = ["black", "blackd"]
```

The version provider for these recipes is `PyPIProvider`, constructed via
`InferredPyPIStrategy` (or `PyPISourceStrategy` when the recipe declares
`[version] source = "pypi"` explicitly). Both code paths call
`(*Resolver).ResolvePyPI(ctx, package)`, which returns the value of
`info.version` in the PyPI JSON response — the absolute latest release.

This works as long as the latest release on PyPI supports the Python
that tsuku's `python-standalone` recipe installs alongside. tsuku
currently bundles **CPython 3.10.20** (resolved from
`indygreg/python-build-standalone`'s most recent 3.10 build).

It stops working when the upstream drops support for that Python
version. Two concrete cases, both deferred from #2296 and tracked in
this issue's parent (#2331):

- **`ansible-core`** — version 2.20.5 declares `Requires-Python >= 3.12`.
  2.18 and 2.19 declare `>= 3.11`. The last 3.10-compatible line is
  2.17.x, with 2.17.14 the latest patch. tsuku's PyPIProvider returns
  `2.20.5`. The pipx_install Decompose then runs `pip download
  ansible-core==2.20.5 --python <python-standalone path>`, and pip
  refuses because the bundled Python doesn't satisfy `>= 3.12`. Eval
  fails before producing a plan; no install is possible.
- **`azure-cli`** — version 2.85.0 declares `Requires-Python >= 3.10.0`.
  Eval succeeds (the bundled 3.10.20 nominally satisfies the
  constraint), but at install time some transitive dependency closure
  fails on the bundled Python. The fix here is also "let the recipe
  pin a known-good version," because the upstream's claimed Python
  range and the actual runtime requirement diverge.

The recipe schema today has no equivalent of `version = ">=2.17,<2.18"`
or `version_constraint`. Recipe authors who understand the upstream's
Python compat windows have no way to express "give me the latest
ansible-core that runs on Python 3.10."

This design adds that primitive.

## Decision Drivers

1. **Unblock ansible and azure-cli.** The immediate goal: a recipe-only
   change to those two recipes resolves and installs cleanly on tsuku's
   bundled Python.
2. **Familiar to recipe authors.** Use a syntax that matches what
   tooling around PyPI already does (`pip install pkg>=1.0,<2.0`),
   what cargo does (`crate = "1.2.3"` with semver-range syntax), what
   npm does. No new mental model.
3. **Preserve existing behavior by default.** Recipes that don't opt
   in continue to resolve to the absolute latest release, exactly as
   today. The mechanism is purely additive.
4. **Keep implementation footprint reasonable.** A small parser and
   a filter on the existing version list, plus plumbing through
   `pipx_install`'s decompose path. No new HTTP calls, no new schema
   beyond one optional field.
5. **Don't preclude richer mechanisms later.** A future automatic
   filter-by-Python-compat layer should be able to cooperate with the
   manual constraint (manual wins, automatic provides the default).

Out of scope for v1:

- Automatic filtering of PyPI releases by the bundled Python's
  `Requires-Python`. Rejected as more general than the v1 problem
  needs and substantially harder to implement (requires threading the
  resolved python-standalone version into the PyPI provider at
  construction time, which the current provider-factory plumbing does
  not do). Captured as a v2 alternative in "Considered Options."
- Wildcard equality (`==1.4.*`) and compatible-release (`~=`)
  operators. Neither immediate consumer needs them; we add them when a
  real recipe wants them.
- Constraint support for non-PyPI sources (npm, crates.io, etc.).
  Different problem, different design conversation.
- Version pinning beyond what tsuku users already get via
  `tsuku install <pkg>@<version>` on the command line. The constraint
  is for the *recipe's resolved-latest selection*, not user-facing
  pinning.

## Considered Options

### Option A: Per-step `version_constraint` parameter on `pipx_install` (chosen)

Recipe declares an optional constraint string on the action:

```toml
[[steps]]
action = "pipx_install"
package = "ansible-core"
version_constraint = ">=2.17,<2.18"
executables = ["ansible", "ansible-playbook", ...]
```

`PipxInstallAction.Decompose` reads the constraint, builds a constrained
PyPIProvider, and uses its resolved version. The PyPI provider gains a
new `NewPyPIProviderWithConstraint(resolver, package, constraint)`
constructor that filters `ListVersions` output by the constraint and
returns the highest matching version from `ResolveLatest`.

- Pro: Smallest mechanism that solves the stated problem (~50 lines).
- Pro: Recipe authors already know constraint syntax.
- Pro: Default unchanged. Purely additive.
- Pro: Localizes the new schema to where it's used (the
  `pipx_install` action) instead of polluting the top-level
  `[version]` block.
- Con: Manual. Recipe authors must research upstream's Python compat
  window and choose a constraint. Stale when tsuku upgrades
  python-standalone (a 3.13 upgrade doesn't auto-relax a `<2.18` pin).
- Con: PyPI uses PEP 440, not semver. Most PyPI versions look like
  semver, but a few (`1.0.0a1`, `1.0.0.post1`) do not. The v1 parser
  handles only the operators in scope; PEP 440 prerelease ordering is
  out of scope.

### Option B: Automatic filtering by bundled-Python `Requires-Python`

PyPIProvider fetches each release's per-file `requires_python` field
from the PyPI JSON (already in the response we already fetch), filters
the version list to releases whose at-least-one-file matches the
bundled Python version, and returns the highest matching version. No
recipe change.

- Pro: Recipes "just work" through Python upgrades. When tsuku ships
  Python 3.13, ansible's recipe automatically picks the latest 3.13-
  compatible release without a recipe edit.
- Pro: Recipe authors don't need to learn about constraints or Python
  compat windows.
- Con: Significantly more implementation surface (~200 lines):
  a small PEP 440 specifier evaluator, per-release file inspection,
  and — most awkwardly — wiring the bundled Python version into the
  PyPI provider at construction time. The provider is built by the
  factory before the python-standalone recipe is resolved; getting
  the version through requires plumbing work or a new "context"
  parameter.
- Con: Edge cases: packages with mis-declared `requires_python` (rare
  but real); releases that ship multiple files with different
  specifiers; releases with no `requires_python` declared at all
  (treat as universally compatible? probably yes, but a real
  decision).

### Option C: Both — automatic by default, `version_constraint` as escape hatch

Default behavior is Option B; the recipe-level `version_constraint`
exists as an explicit override when automatic filtering goes wrong
(upstream bugs, intentional pinning).

- Pro: Best of both — common case is automatic, escape hatch covers
  the edge cases.
- Con: Carries Option B's full implementation cost. The escape-hatch
  add is small once Option B is built, so it's only worth doing once
  Option B is in.

### Option D: Top-level `[version] constraint` field

Recipe declares the constraint at the top level:

```toml
[version]
source = "pypi"
constraint = ">=2.17,<2.18"
```

- Pro: Generalizes to other version sources (npm, crates.io) by
  reusing the same field.
- Con: Couples the constraint to the `[version]` block even when the
  consumer is a per-step action like `pipx_install`. For pipx
  recipes, the package name is already on the step, so the constraint
  is naturally a step parameter too.
- Con: Premature generalization. We have one consumer in the
  immediate set; a top-level field invites copy-paste into npm
  recipes that may have different semantics for what "version
  constraint" means.

## Decision Outcome

Chose **Option A: per-step `version_constraint` parameter on `pipx_install`**.

Option A satisfies all five decision drivers. It unblocks ansible and
azure-cli with a recipe-only change to those two recipes (driver 1).
The `version_constraint` field uses the same comma-joined PEP 440
syntax that pip itself accepts on the command line, which recipe
authors already know (driver 2). Recipes without the field continue
to resolve to the absolute latest, exactly as today (driver 3). The
change ships in roughly 50 lines plus tests (driver 4). The mechanism
is additive and does not preclude a future v2 that layers automatic
Python-compat filtering on top — that v2 would set the *default*
behavior; the explicit constraint would still win (driver 5).

Option B is the more "correct" long-term behavior but its
implementation cost is several times larger, and the immediate
consumers do not need it: a one-line constraint on each recipe today
expresses the same intent and lands on the same resolved version.

Option C is what we end up with if we ever do Option B; it's not
worth pre-planning for, since the refactor at that point is small
(add the auto-filter, keep the constraint as override).

Option D's generalization across sources is real but not yet
motivated — only `pipx_install` recipes hit this problem today, and
the per-step location keeps the constraint near the package name it
constrains.

## Solution Architecture

The implementation lives in three places: the PyPI provider (filter
logic), the pipx_install action (parameter plumbing), and the recipe
schema (no change beyond the new action parameter).

### Recipe shape

Recipes that need the constraint declare it on the `pipx_install` step:

```toml
[[steps]]
action = "pipx_install"
package = "ansible-core"
version_constraint = ">=2.17,<2.18"
executables = ["ansible", "ansible-playbook", "ansible-galaxy",
               "ansible-vault", "ansible-config", "ansible-doc",
               "ansible-inventory", "ansible-pull", "ansible-console"]
```

The recipe author chose the constraint by looking at PyPI's history
for the package (`pypi.org/pypi/ansible-core/json`'s `releases` map)
and identifying the latest line whose `requires_python` includes the
bundled Python. Future v2 work (Option B layered on top) would make
this lookup automatic.

### Constraint syntax

A `version_constraint` string is a comma-joined list of clauses. Each
clause has the shape `<operator><version>`:

| Operator | Meaning |
|----------|---------|
| `==` | Exact equality |
| `!=` | Not equal |
| `>=` | Greater than or equal |
| `>` | Greater than |
| `<=` | Less than or equal |
| `<` | Less than |

Clauses are AND-joined: `>=2.17,<2.18` matches versions that satisfy
both. Whitespace around commas and operators is permitted but not
required.

Versions are compared using the existing `version_utils.go`
comparison logic, which handles SemVer-style version strings
correctly. Most PyPI versions are SemVer-compatible; the rare
prerelease forms (`1.0.0a1`, `1.0.0.dev1`) sort according to the
existing `splitPrerelease` semantics established in
`docs/designs/current/DESIGN-prerelease-detection.md`.

Out of scope for v1:

- `~=` (compatible release, e.g., `~=2.2` matches `>=2.2,<3.0`).
- `==1.4.*` (wildcard equality).
- `===` (arbitrary equality).

These are added when a real recipe needs them.

### PyPI provider extensions

`internal/version/provider_pypi.go` gains a constraint-aware
constructor:

```go
type PyPIProvider struct {
    resolver    *Resolver
    packageName string
    constraint  []constraintClause // nil when no constraint
}

func NewPyPIProvider(resolver *Resolver, packageName string) *PyPIProvider {
    return &PyPIProvider{resolver: resolver, packageName: packageName}
}

// NewPyPIProviderWithConstraint creates a provider that filters versions
// against a PEP 440-style comma-joined constraint string.
func NewPyPIProviderWithConstraint(resolver *Resolver, packageName, constraint string) (*PyPIProvider, error) {
    clauses, err := parseConstraint(constraint)
    if err != nil {
        return nil, fmt.Errorf("invalid version_constraint %q: %w", constraint, err)
    }
    return &PyPIProvider{
        resolver:    resolver,
        packageName: packageName,
        constraint:  clauses,
    }, nil
}
```

`ResolveLatest` is updated:

```go
func (p *PyPIProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
    if p.constraint == nil {
        return p.resolver.ResolvePyPI(ctx, p.packageName)
    }
    versions, err := p.ListVersions(ctx)
    if err != nil {
        return nil, err
    }
    for _, v := range versions {
        // ListVersions returns versions newest-first; pick the first
        // version that satisfies every clause.
        if matchesConstraint(v, p.constraint) {
            return &VersionInfo{Tag: v, Version: v}, nil
        }
    }
    return nil, fmt.Errorf("no PyPI version of %s matches constraint %q",
        p.packageName, formatConstraint(p.constraint))
}
```

`ResolveVersion` (used for explicit user pins) keeps its current
behavior; the constraint is applied to "give me the latest" requests
only.

### Constraint parser and matcher

A new file, `internal/version/pypi_constraint.go`:

```go
type constraintOp int

const (
    opEQ constraintOp = iota
    opNE
    opGE
    opGT
    opLE
    opLT
)

type constraintClause struct {
    op      constraintOp
    version string
}

// parseConstraint parses a comma-joined list of clauses.
// Returns an error for malformed input.
func parseConstraint(s string) ([]constraintClause, error) { ... }

// matchesConstraint returns true if the version satisfies every clause.
func matchesConstraint(version string, clauses []constraintClause) bool { ... }
```

The matcher reuses `CompareVersions` from `version_utils.go` for
ordering.

### pipx_install plumbing

`internal/actions/pipx_install.go` reads the new parameter from the
action's `params` map and threads it through to provider construction.
The most natural seam is the `Inferred*Strategy` (or
`PyPISourceStrategy`) inside `provider_factory.go`:

```go
func (s *InferredPyPIStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
    for _, step := range r.Steps {
        if step.Action == "pipx_install" {
            pkg, _ := step.Params["package"].(string)
            if constraint, ok := step.Params["version_constraint"].(string); ok && constraint != "" {
                return NewPyPIProviderWithConstraint(resolver, pkg, constraint)
            }
            return NewPyPIProvider(resolver, pkg), nil
        }
    }
    return nil, fmt.Errorf("no PyPI package found in pipx_install steps")
}
```

The matching `PyPISourceStrategy` for `[version] source = "pypi"` gets
the same treatment.

### Validator

`internal/recipe/validator.go` validates the parameter at strict-validate
time:

- If `version_constraint` is present on a `pipx_install` step, it must
  parse successfully via `parseConstraint`.
- The action preflight (`PipxInstallAction.Preflight`) issues a warning
  if `version_constraint` is set but the parameter is empty after
  trimming.

### What is explicitly out of scope

- **Automatic Python-compat filtering.** The provider does not inspect
  per-release `requires_python` fields. A future v2 design covers this.
- **Constraint reuse for npm, crates.io, etc.** Each registry's
  semantics for "version constraint" differs; a single shared field
  would couple them in confusing ways. We add per-registry support
  when a real consumer needs it.

## Implementation Approach

The implementation lands in three contained slices:

1. **Constraint parser and matcher.**
   - Add `internal/version/pypi_constraint.go` with `parseConstraint`,
     `matchesConstraint`, and `formatConstraint` (for error messages).
   - Tests in `internal/version/pypi_constraint_test.go` covering:
     each operator individually, multi-clause AND, whitespace
     tolerance, parse errors (unknown operator, missing version,
     trailing comma), match cases (between bounds, edge of bound,
     mismatch).

2. **PyPI provider integration.**
   - Add `NewPyPIProviderWithConstraint` and the `constraint` field
     to `PyPIProvider`.
   - Update `ResolveLatest` to walk `ListVersions` output when a
     constraint is set, picking the first match.
   - Tests using a mock PyPI server (existing pattern in
     `pypi_test.go`) covering: latest with no constraint, constraint
     selecting an older version, constraint that matches no version,
     constraint that matches the absolute latest.

3. **Recipe and action plumbing.**
   - Update `InferredPyPIStrategy` and `PyPISourceStrategy` in
     `provider_factory.go` to read `version_constraint` from the
     `pipx_install` step and use the constrained constructor.
   - Update `PipxInstallAction.Preflight` to surface a warning for
     empty/invalid constraints.
   - Add validator support in `internal/recipe/validator.go`.
   - Author the ansible and azure-cli recipes as the first consumers
     in the same PR. Each adds one line: `version_constraint =
     ">=2.17,<2.18"` for ansible-core, an analogous pin for azure-cli
     based on its 3.10-compatible line.

The three slices land in a single PR so the mechanism is exercised
end-to-end by at least one real recipe at merge time.

## Security Considerations

- **Constraint string is data, not code.** The parser produces a
  fixed-shape `[]constraintClause` value; matching is structural
  comparison. No template evaluation, regex compilation from user
  input, or shell-out behavior.
- **Recipe-controlled but reviewed at merge.** The constraint is a
  recipe field. Recipe changes are reviewed; the constraint cannot
  affect any code path other than version selection within
  PyPIProvider.
- **No new external attack surface.** The provider already fetches
  from `pypi.org`; the constraint changes which version it picks from
  the response, not what it fetches. The 10 MB response size cap
  remains in effect.
- **Plan-time integrity preserved.** The selected version flows into
  the locked-requirements generation in
  `PipxInstallAction.Decompose`, which captures hashes of every wheel
  in the resolved dependency closure. A later compromise of PyPI
  cannot substitute different bytes without the plan's checksums
  mismatching.
- **Bounded compute.** The matcher runs in O(versions × clauses).
  PyPI typically lists hundreds of versions per package; clauses are
  capped implicitly by the constraint string length the validator
  accepts.

## Consequences

### Positive

- Unblocks ansible (#2345) and azure-cli (#2345) recipes in a
  recipe-only follow-up PR.
- Provides a clean syntax recipe authors already know from pip and
  cargo, with no new mental model.
- Localizes the new schema to the action that uses it, keeping the
  top-level `[version]` block unchanged.
- Leaves room for a future v2 that layers automatic filtering on top
  — the manual constraint becomes the explicit override.

### Negative

- **Stale constraints.** When tsuku upgrades python-standalone (e.g.,
  3.10 → 3.13), recipes that pin `<2.18` continue to install
  ansible-core 2.17.x even though 2.20.x would now work. Mitigation:
  audit the small set of pipx recipes when the bundled Python
  upgrades; constraints are a few lines to update. Future v2 makes
  this audit unnecessary by inferring the upper bound automatically.
- **Recipe authors must research upstream's Python compat window.**
  This is one extra step in the recipe-authoring flow. Mitigation:
  the recipe-author skill documents the lookup (visit
  `pypi.org/pypi/<package>/json`, scan for `requires_python`).
- **Subset of PEP 440.** The v1 parser does not accept `~=`, `==X.*`,
  or `===`. Recipes that need those operators will be rare; we
  extend when a real consumer requests them.

### Affected Components

- `internal/version/pypi_constraint.go` (new file, parser + matcher)
- `internal/version/pypi_constraint_test.go` (new test)
- `internal/version/provider_pypi.go` (constrained constructor +
  ResolveLatest update)
- `internal/version/provider_factory.go` (`InferredPyPIStrategy` and
  `PyPISourceStrategy` read the new parameter)
- `internal/actions/pipx_install.go` (preflight warning for
  malformed constraint)
- `internal/recipe/validator.go` (strict validator parses the
  constraint)
- `recipes/a/ansible.toml` (new recipe, first consumer)
- `recipes/a/azure-cli.toml` (new recipe, second consumer)
