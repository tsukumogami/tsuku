# Convergence: pipx PyPI version pinning (#2331)

## What changed in our understanding

The original framing of #2331 — "tsuku tries to install latest, fails on
incompatible Python, needs a constraint primitive" — is **partially wrong**.
Five parallel research leads converge on a different diagnosis.

### The big finding (L4)

`pip` already filters by `Requires-Python` and picks the newest
compatible release. Empirically verified on `python:3.10-slim`:
`pip install --dry-run ansible-core` selects `ansible-core 2.17.14`,
not 2.20.5. (See `wip/research/.../lead-pip-pipx-semantics.md`.)

So when does the failure happen? Per L3 reproduction: tsuku's
`pipx_install.Decompose` calls `pip download <package>==<version>`
with the version **already resolved by tsuku's `PyPIProvider`** —
which returns `info.version` (absolute latest, ignoring Python compat).
Pip is then asked to download an exact pinned version that doesn't
satisfy the running Python; pip's smart-fallback can't kick in
because the pin is exact.

**Root cause:** tsuku is fighting pip. tsuku pre-resolves to absolute
latest, then hands pip an unsatisfiable pin. Pip is doing the right
thing; tsuku is overriding pip with an incompatible request.

## Findings by lead

### L1 — recipe-level pinning landscape

- **Confirmed gap.** Across all 12+ providers, `VersionSection` has zero
  fields that constrain the resolved version. Every recipe defaults to
  "latest." (`internal/recipe/types.go:178-208`)
- **No deliberate decision.** Provider constructors take only source
  identity; constraint plumbing would slot in cleanly via existing
  factory strategies. The gap is architectural, not by design.
- **User pinning works.** `tsuku install foo@2.17` flows through
  `Executor.ResolveVersion` → `ResolveWithinBoundary` → provider's
  `ResolveVersion` (fuzzy match). Recipes have no equivalent path.

### L2 — pipx_install ↔ PyPIProvider data flow

- **PyPIProvider construction is upstream of python-standalone resolution.**
  At `provider_factory.go:173` (PyPISourceStrategy) and `:270`
  (InferredPyPIStrategy), the only data available is the resolver and
  the package name. python-standalone hasn't been resolved yet.
- **Inside Decompose, both are available.** `pipx_install.go:318`
  calls `ResolvePythonStandalone()` to find the bundled `python3`
  binary. So a Python-aware filter could plug in at Decompose time
  (where pythonPath is already known) or at provider-construction
  time (with a constant lookup, since python-standalone always ships
  CPython 3.10.x today).
- **`pip download` failure is exact-pin.** `generateLockedRequirements`
  (line 402) → `runPipDownloadWithHashes` (line 460) is what fails for
  ansible. The version it pins is whatever `PyPIProvider.ResolveLatest`
  returned during plan generation.

### L3 — failure reproduction

- **ansible-core: confirmed.** `tsuku eval --recipe ansible.toml` fails
  with exit 1 at decompose. pip's stderr enumerates every release with
  its `Requires-Python` and confirms 2.17.14 is the highest 3.10-compatible
  version. Issue's description verified.
- **azure-cli: eval succeeds.** Resolves to 2.85.0; pip happily
  produces a hash-locked requirements.txt (~120 transitive deps,
  including recent C-extension packages). The issue's claim that
  `az --version` fails post-install is **not reproducible** without
  sandbox infrastructure and remains unverified.
- **Implication:** azure-cli may be a separate problem (transitive
  dep ABI/runtime, not Python compat). A version-pinning fix designed
  around ansible's Python-compat failure may not solve azure-cli at all.

### L4 — pip / pipx semantics (the reframer)

- **Unpinned: pip already does the right thing.** `_check_link_requires_python`
  in pip's `package_finder.py` filters every release; `CandidateEvaluator._sort_key`
  picks the newest survivor.
- **Exact-pin + incompatible: hard error, no fallback.** `pip install
  ansible-core==2.20.5` on Python 3.10 → "Could not find a version that
  satisfies the requirement." Pip refuses to silently relax exact pins.
- **Range-pin + compatible release exists: fine.** `pip install
  "ansible-core>=2.17,<2.18"` on Python 3.10 picks 2.17.14.
- **pipx adds nothing.** Delegates to pip for all version selection.

### L5 — PyPI API surface

- **`requires_python` is per-release, well-populated for modern tools.**
  ansible-core: 313/314 populated. azure-cli post-2020: clean. pdm: 246/246.
  Long-lived projects have nulls on legacy releases (httpie pre-2018:
  40/55 null).
- **Format variance is real.** Whitespace, operator-order, patch-precision
  all vary. Cannot regex-parse. Needs a real PEP 440 specifier evaluator.
  `Masterminds/semver` (already in tsuku deps) does NOT suffice — rejects
  four-segment versions and `!=3.0.*`-style exclusions; misorders RC
  strings. A small PEP 440 specifier matcher would need to be added.
- **API consumption is fine.** Existing `pypi.go` already hits the right
  endpoint; the struct just discards file dicts. Adding `requires_python`
  retention is ~5 lines.

## Decision space (revealed by research)

The original two-option framing in the issue (manual constraint vs.
auto Python-compat) misses two important realities:

1. The fundamental issue is that `PyPIProvider.ResolveLatest` ignores
   `Requires-Python`, then tsuku hands pip an incompatible exact pin.
2. azure-cli's failure may not be a Python-compat problem at all.

Real options now look like:

### Option A — Auto Python-compat filter in PyPIProvider (NEW preferred candidate)

Make `PyPIProvider.ResolveLatest` filter releases by `requires_python`
against the bundled python-standalone's major.minor. Walk newest-first;
return the first compatible. No recipe-author input needed; works for
every pipx_install recipe automatically.

- **Mechanism:** A constants package (e.g., `internal/python/bundled.go`)
  exports `BundledPythonMajorMinor = "3.10"`. PyPIProvider reads it at
  construction, filters during `ResolveLatest`. `ListVersions` filters too.
- **Code surface:** ~80-120 LOC: PEP 440 specifier evaluator (~50 LOC),
  PyPIProvider integration (~30 LOC), constants package (~10 LOC),
  retention of `requires_python` in the JSON struct (~5 LOC).
- **Solves ansible.** Resolves to 2.17.14 automatically. No recipe edits.
- **Does NOT solve azure-cli** if the failure is transitive-dep ABI.
  But azure-cli's eval already succeeds, so this option doesn't make
  things worse — it just doesn't fix that specific case.
- **Future-proof.** When tsuku upgrades to python-standalone 3.11 or
  3.12, recipes pick up newer versions automatically.

### Option B — Manual recipe-level constraint

Add a `version_constraint` field (in `[version]` or per-step) that
accepts PEP 440 syntax. Recipe author writes `>=2.17,<2.18` to pin
ansible. PyPIProvider filters its release list against the constraint.

- **Code surface:** Similar PEP 440 evaluator, plus schema additions
  and per-recipe maintenance. Estimate ~100 LOC + per-recipe author burden.
- **Solves both ansible and azure-cli** (if author pins to a known-good
  azure-cli range).
- **Recipe maintenance burden:** Author must know which range works,
  must update on EOL.
- **Doesn't auto-track the bundled Python.** When tsuku upgrades, every
  pipx recipe still has its old constraint.

### Option C — Both (auto + manual override)

Auto filter by Python compat as the default; manual `version_constraint`
as an override for cases where Python compat isn't the issue (azure-cli
transitive deps, security pinning, etc.).

- **Code surface:** ~150-200 LOC.
- **Maximum flexibility.** Auto handles 95% of cases; manual override
  handles the rest.
- **More schema, more docs.**

### Option D — "Don't pre-resolve, let pip choose"

Skip `PyPIProvider.ResolveLatest` entirely for pipx_install; pass the
unpinned package name to `pip download` and let pip pick.

- **Tiny code change.**
- **Loses reproducibility.** `tsuku eval` can't show a concrete version;
  every run could pick a different one as PyPI publishes patches.
- **Breaks the install plan model.** tsuku's two-phase install needs
  the version to determine the cache key (per `Executor.ResolveVersion`
  comment).
- **Probably a non-starter** but worth naming for completeness.

## Open questions for the user

1. **Is azure-cli within scope?** The issue lumps it with ansible, but
   research suggests they're different problems. If azure-cli is a
   transitive-dep issue, version pinning may not fix it — a separate
   investigation is needed. Should this design only solve ansible
   and defer azure-cli?
2. **Auto vs. manual vs. both?** Option A is leanest and most automatic;
   Option C is most general; Option B is what the issue's body
   originally suggested. The research shifted my own preference toward
   Option A — but does the user have a preference informed by other
   recipes I haven't seen?
3. **Should recipe-level constraints exist for non-PyPI providers?**
   None today have them (L1). Adding it for PyPI only is fine, but
   if the design wants symmetry across providers (github, npm, etc.)
   that's a much bigger scope. I'd narrow to PyPI-only unless told
   otherwise.

## Status

Five leads complete. No major contradictions. Two real candidate
designs (A and C). The choice depends mostly on the azure-cli scope
question above.

## Decision: Crystallize

Direction set in round 1 (see `_decisions.md`):
- Option A (auto Python-compat filter from PyPI's `requires_python`).
- No recipe-author version pins.
- azure-cli deferred to a follow-up issue.
- Scope: PyPI provider only.

Proceed to Phase 4.
