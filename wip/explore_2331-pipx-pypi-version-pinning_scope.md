# Explore Scope: pipx PyPI version pinning (#2331)

## Visibility

Public

## Scope

Tactical — implementation-focused on the tsuku monorepo.

## Core Question

How should `pipx_install`-based recipes pick a PyPI version that is
compatible with the Python version tsuku ships, when the upstream's
absolute-latest release is not? The issue itself names two candidate
shapes (manual recipe-level constraint vs. automatic Python-compat
resolution) and the design phase must evaluate both — grounded in
how the existing version providers and pipx_install plumbing
actually work — before recommending one.

## Issue Summary

Issue #2331 wants `pipx_install` recipes to install successfully when the
upstream's latest PyPI release drops support for the Python that tsuku's
`python-standalone` recipe ships (currently CPython 3.10.20). Two concrete
recipes are blocked: `ansible-core` (latest 2.20.5 needs Python ≥ 3.12)
and `azure-cli` (latest 2.85.0 nominally accepts 3.10 but transitive deps
fail at runtime).

The issue body explicitly contemplates two solution shapes:

> a recipe-level version constraint (or, alternatively, a pypi-aware
> lower-bound that pipx_install resolves automatically against the
> bundled Python's `Requires-Python`) is the missing primitive

The previous design (PR #2351, closed) jumped to the first shape without
evaluating the second. This exploration must ground both alternatives in
the actual codebase before recommending one.

## Open Questions

1. What does "pin a PyPI version" mean today, mechanically? PyPI's
   `ResolveVersion` does fuzzy match (`2.17` → newest `2.17.x`). Is that
   already wired through `[version]` for recipe-level pinning, or only
   for user-level pin (`tsuku install foo@2.17`)?
2. Do other version sources (github, npm, etc.) have any recipe-level
   "default to a version range" mechanism today, or do all recipes
   uniformly default to "latest"?
3. What is the exact data flow from `pipx_install` step → `PyPIProvider`?
   Where does the bundled `python-standalone` version live, and is it
   reachable at `PyPIProvider` construction time?
4. PyPI's JSON API surfaces `requires_python` per release. Is there any
   precedent in tsuku for filtering a provider's version list by an
   external constraint (Python compat, minimum tsuku version, anything)?
5. How does pip itself resolve "latest version compatible with my Python"?
   Does pipx do this filtering automatically, or only after download?
6. What happens when the "latest" returned by `ResolveLatest` is
   incompatible with the bundled Python? Where does the failure surface
   (eval crash, install failure, runtime failure)? The issue mentions
   `pip download` rejecting newer versions and `tsuku eval` crashing
   before producing a plan — is that root cause confirmed or speculation?
7. azure-cli 2.85 declares `Requires-Python ≥ 3.10` and resolves at eval,
   but `az --version` fails post-install. What's the actual failure
   mode? Is it a transitive dep, a bundled-binary mismatch, something
   else? Without knowing, we can't say whether *any* version-pinning
   mechanism solves azure-cli or whether azure-cli is a separate
   problem.

## Leads (round 1)

These will be expanded by parallel research agents in Phase 2:

- **L1: Recipe-level pinning landscape** — survey every version provider
  (`provider_*.go`) to see whether any recipe-level fixed-version field
  exists today. Identify the actual gap: is "default to a fuzzy version"
  already expressible, or is it genuinely missing across the board?
- **L2: pipx_install ↔ PyPIProvider data flow** — trace from
  `recipe.toml` step through the executor to `PyPIProvider` to see how
  `package` is wired and whether anything Python-version-related is
  reachable at provider construction. Identify where the bundled
  `python-standalone` version is stored and how it's accessed.
- **L3: Failure modes today** — actually run `tsuku eval` against an
  ansible recipe and an azure-cli recipe to confirm the issue's
  failure descriptions, not just believe them. Identify the actual
  failing layer (eval, plan generation, pip download, post-install
  verify).
- **L4: pip / pipx Python-compat resolution semantics** — research
  what pip does when asked for "latest" of a package with mixed
  `Requires-Python` across versions. Determine the standard external
  behavior so we can match it (or document why we don't).
- **L5: PyPI JSON API surface** — confirm that per-release
  `requires_python` is reliably present in `https://pypi.org/pypi/<pkg>/json`
  for both `ansible-core` and `azure-cli` (and ideally a third sample),
  and check whether the `info.requires_python` differs from per-release
  values for older releases.

## Boundaries

In scope:
- Recipe-level mechanism (constraint, default, automatic, or none)
- `pipx_install` callers only — generic PyPI provider behavior change
  is fair game if it's the simpler path
- The two follow-up recipes (ansible, azure-cli) only as proof points

Out of scope:
- Adding new version providers
- Changing `python-standalone` to a newer Python (separate question)
- User-level pin syntax changes (`tsuku install foo@X` already works)
- Changes to non-pipx version sources, except as needed for symmetry
