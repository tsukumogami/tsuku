---
status: Proposed
problem: |
  tsuku's `pipx_install` action installs Python CLI tools from PyPI, but
  `PyPIProvider.ResolveLatest` always returns the absolute-latest release
  (`info.version`) regardless of whether that version is compatible with
  the Python that tsuku's `python-standalone` recipe ships (currently
  CPython 3.10). When the upstream drops support for the bundled Python,
  tsuku then asks `pip download` for an exact pin pip cannot satisfy and
  the eval fails. Pip on its own would have walked back to the newest
  compatible release; tsuku is forcing pip to act on an incompatible
  version. The design needs to decide where in tsuku's resolution path
  to consume PyPI's per-release `requires_python` metadata so that
  `pipx_install` recipes resolve to the newest Python-compatible version
  automatically, without requiring recipe authors to mirror that data
  by hand in TOML. Concrete trigger: `recipes/a/ansible.toml` cannot
  resolve under bundled Python 3.10 because `ansible-core` 2.18+ requires
  Python ≥ 3.11; the highest compatible release is 2.17.14.
---

# DESIGN: pipx PyPI Version Pinning by Python Compatibility

## Status

Proposed

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
