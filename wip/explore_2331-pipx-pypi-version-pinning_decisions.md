# Exploration Decisions: pipx PyPI version pinning (#2331)

## Round 1

- **Direction: Option A (auto Python-compat filter), reject B and C-with-manual-override.**
  Rationale: User stated principle that recipes should not carry hardcoded
  versions. PyPI exposes per-release `requires_python` metadata; tsuku
  should consume that metadata rather than asking authors to mirror it
  manually in TOML.

- **azure-cli scope: deferred.** Research shows azure-cli's eval already
  succeeds at 2.85.0 with valid `requires_python >= 3.10.0` metadata.
  Its claimed post-install failure (the `az --version` exit-non-zero
  observation in #2331) is unreproducible without sandbox infra and
  may be a transitive-dep ABI problem unrelated to Python compat.
  Option A solves ansible cleanly; if azure-cli's failure persists
  after A lands, it's tracked as a separate issue. The original #2331
  acceptance criterion that bundles azure-cli into this design will
  be split: ansible recipe lands with A, azure-cli investigation
  becomes a follow-up.

- **Scope: PyPI provider only, not all providers.** No other provider
  has an analogous "what Python this expects" upstream signal that
  matches our use case. Symmetry across providers is not a goal.
