# Design Summary: pipx PyPI version pinning (#2331)

## Input Context (Phase 0)

**Source:** /shirabe:explore handoff

**Problem:** `pipx_install` recipes fail to resolve when the
absolute-latest PyPI release of a tool drops support for the Python
that tsuku's `python-standalone` ships. Root cause: tsuku pre-pins
to absolute latest, then asks pip for an unsatisfiable exact pin.
Design needs to settle where and how to consume PyPI's per-release
`requires_python` metadata so resolution picks the newest compatible
release automatically.

**Constraints (from /explore decisions):**

- Recipes must not carry hardcoded versions; PyPI metadata is the
  source of truth.
- Approach is auto Python-compat filtering (Option A in findings).
  Manual `version_constraint` and hybrid options rejected.
- Scope is PyPI provider only — no symmetry change to other providers.
- azure-cli deferred to a follow-up; ansible-core is the proof point.

**Key research artifacts:**

- `wip/explore_2331-pipx-pypi-version-pinning_findings.md` — synthesized
  decision space, why the issue's framing was reframed
- `wip/explore_2331-pipx-pypi-version-pinning_decisions.md` — round-1
  directional decisions
- `wip/research/.../lead-pinning-landscape.md` — recipe-level
  constraint gap is real; user-pin path uses fuzzy match
- `wip/research/.../lead-data-flow.md` — PyPIProvider construction is
  upstream of python-standalone resolution; Decompose has both
- `wip/research/.../lead-failure-reproduction.md` — empirical eval
  failures; ansible verified, azure-cli eval succeeds (separate problem)
- `wip/research/.../lead-pip-pipx-semantics.md` — pip already filters
  Requires-Python; tsuku is fighting it
- `wip/research/.../lead-pypi-api-surface.md` — `requires_python` is
  reliable for modern tools; PEP 440 evaluator needed (semver insufficient)

## Open Design Questions

1. Filter location: inside `PyPIProvider` (broadest) vs. inside
   `pipx_install.Decompose` (narrowest, where python path is already
   in hand)?
2. How does PyPIProvider learn the bundled Python's major.minor?
   Constants file? Plumbed through factory strategies? Read from
   python-standalone recipe metadata?
3. PEP 440 subset: minimal (`>=`, `>`, `<=`, `<`, `==`, `!=`) or full
   (including `~=` and `==X.*` wildcards)?
4. Failure message contract when no release is compatible.
5. Should non-pipx PyPI consumers (if any exist) opt in or get the
   filter automatically?

## Current Status

**Phase:** 6 — Final Review (complete)
**Last Updated:** 2026-04-28

## Security Review (Phase 5)

**Outcome:** Option 2 — Document considerations
**Summary:** Three parser hardening checks added at Phase 5
(per-clause length cap, ASCII-only validation, segment-magnitude cap).
Folded into Phase 1 deliverables and Security Considerations.

## Phase 6 Architecture Review

**Outcome:** Three blocking issues raised, all addressed by revising
Decision 1's chosen approach. Filter relocated from
`pipx_install.Decompose` to `PyPIProvider.ResolveLatest`, with the
bundled Python supplied via runtime probe at provider construction.
Cache-key vs. installed-version divergence eliminated. `pypiPackageInfo`
struct shape correctly described (`Releases map[string][]struct{}`
replaced by typed file struct).

## Phase 6 Security Re-review

**Outcome:** Two new low-severity findings, both folded in. Total-length
and clause-count caps added to the parser hardening list (now 5
checks). Error message template explicitly renders `<Z>` through
`pep440.Canonical(...)` to prevent log injection / terminal-escape via
attacker-controlled PyPI metadata.
