# Decision 3: Failure-message contract when no PyPI release is compatible

**Status:** COMPLETE
**Confidence:** high
**Tier:** 3 (standard)
**Mode:** --auto

## Question

What does tsuku surface to the user when no PyPI release is compatible with
the bundled Python?

## Decision

**Adopt Option A's wording, plumbed via Option E's two-layer pattern.**

Concrete contract:

- **Wording (Option A):** A single-line, source-prefixed error of the shape
  `pypi resolver: no release of <package> is compatible with bundled Python
  <X.Y> (latest is <V>, requires Python <Z>)`.
- **Plumbing (Option E):** `PyPIProvider.ResolveLatest` (and `ListVersions`)
  returns a typed `*ResolverError` with a new `ErrTypeNoCompatibleRelease`
  classification. `pipx_install.Decompose` does not need to re-wrap with
  package context, because the package name is already attached to the
  provider and is included in the error message.
- **Exit behaviour:** the error propagates through the existing
  `Executor.ResolveVersion` -> CLI surface chain. `tsuku eval` and `tsuku
  install` exit non-zero with the message printed to stderr. No new flag,
  no `--ignore-requires-python` escape hatch (out of scope per constraint
  4; named separately below).

## Confidence

**High.** The chosen option does the minimum that satisfies the constraints,
matches the codebase's existing tone, and avoids two failure modes in the
alternatives (B's misleading suggestion, C's empty list).

## Rationale

### 1. Reachability says: keep it short

The branch is only reached when *every* release of a package requires a
Python newer than the bundled 3.10. Pip's own filter and Option A
(auto Python-compat filter, already chosen in Decision 1) walk the full
release list and pick the newest survivor. So this error fires only when
the survivor set is empty.

In practice, that requires a package whose oldest release sets
`Requires-Python >= 3.11` (or higher) on every entry. Modern PyPI packages
created during the Python 3.11+ era can produce this; long-lived packages
with releases predating 2016 typically have `requires_python = null` on
old releases, which pip treats as compatible, making the branch
unreachable for them.

This is a real but rare corner. The verbose options (D) and the
suggestion-laden ones (B, C) over-engineer for that frequency.

### 2. Tone match: tsuku's version errors are terse

Sampled patterns:

- `"version %s not found for crate %s"` (provider_crates_io.go:241)
- `"version %s not found for Homebrew formula %s"` (provider_homebrew.go:60)
- `"package %s not found on PyPI"` (pypi.go ResolverError)
- `"python-standalone not found: install it first (tsuku install
  python-standalone)"` (pipx_install.go:320)

The codebase consistently uses one-line, factual messages with the
identifying name and the operation that failed. Option A fits this mould.
Option D (pip-style enumeration of every release with its `Requires-Python`)
would clash visibly with the rest of the surface.

### 3. Today's baseline already gives the verbose form -- for free

`tsuku eval --recipe ansible.toml` today fails with:

```
failed to generate locked requirements: pip download failed: exit status 1
Output: ERROR: Ignored the following versions that require a different
python version: ... 2.18.0 Requires-Python >=3.11 ... 2.20.5 Requires-Python
>=3.12 ...
ERROR: Could not find a version that satisfies the requirement
ansible-core==2.20.5 (from versions: 0.0.1a1, 2.11.0b1, ..., 2.17.14)
```

Pip's own stderr already contains the full enumeration. Reproducing it in
tsuku's wrapper would duplicate output for the rare case where this
message even fires, and would push tsuku's prose toward pip's verbose
style.

The new error message is a *pre-flight* signal: tsuku notices the
incompatibility before invoking `pip download`, fails fast with a tight
message, and never reaches the verbose pip path.

### 4. Why not B (suggest "may need a follow-up issue")?

The suggestion is incorrect when the branch is reached. With Option A's
auto-filter active, finding an older compatible release is the *success*
path, not the failure path. Reaching this error already means no
compatible release exists. Telling the user "this may need a follow-up
issue if the package supports older Python via a backport branch" is
inaccurate -- by definition, none of the existing PyPI releases are
compatible. A real backport would have to be a *new* PyPI release.

If we want to encourage issue-filing, that belongs in higher-level
documentation, not in an error message that fires on an edge case where
the suggestion may not match the user's situation.

### 5. Why not C (top-3 compatible versions)?

By definition, the compatible set is empty when this error fires. The list
would always be empty. The option's own description acknowledges the value
"is when *some* exist but the latest doesn't (which is the success case,
not the failure case)." That branch is already handled by Option A's
auto-filter in Decision 1. C does not apply to this decision's question.

### 6. Why not D (pip-style enumerate-all)?

- Verbose, breaks the codebase's tone.
- Pip's stderr already contains this content in the rare cases users want
  it -- they can run `pip install <pkg>` directly under bundled python to
  see it.
- Adds non-trivial implementation cost (per-release `Requires-Python`
  retention, formatting) for marginal benefit on a rare branch.
- Reproduces what pip is the canonical source of truth for.

### 7. Why E (two-layer plumbing) on top of A?

Existing PyPIProvider errors are already typed `*ResolverError` (see
`pypi.go`: `ErrTypeNotFound`, `ErrTypeValidation`, `ErrTypeParsing`,
`ErrTypeNetwork`). Adding `ErrTypeNoCompatibleRelease` (or a new typed
error struct alongside `ResolverError`) fits the established pattern.
Benefits:

- Callers can `errors.As` on the typed error if they ever need to react
  programmatically (e.g., a future doctor command, telemetry counter).
- The `Source: "pypi"` prefix from `ResolverError.Error()` already
  produces the `pypi resolver: ...` format the existing chain uses.
- `pipx_install.Decompose` does not need a separate wrapping layer: the
  provider already knows its package name and includes it in the message.

This is implementation-style guidance, not a competing wording choice.
The decision text remains Option A.

## Final Wording

The error string the user sees:

```
pypi resolver: no release of <package> is compatible with bundled Python
<X.Y> (latest is <V>, requires Python <Z>)
```

Concrete example for ansible-core on bundled Python 3.10:

```
pypi resolver: no release of ansible-core is compatible with bundled
Python 3.10 (latest is 2.20.5, requires Python >=3.12)
```

Notes on wording choices:

- "bundled Python" makes it explicit that the user can't change it via
  env var or recipe pin -- it's tsuku's CLI distribution.
- "latest is <V>, requires Python <Z>" gives the user enough to understand
  the cause without reproducing pip's enumeration.
- The `pypi resolver:` prefix is automatic from `ResolverError.Error()`;
  no manual prefixing in the message body.
- One sentence, no trailing newline, no multi-line structure.

## Constraint Verification

| Constraint | Met? | How |
|------------|------|-----|
| 1. Failure must be actionable | Yes | Names the package, the bundled Python version, the latest release, and its Python requirement. The user knows nothing they can do via CLI flags will install this package on this tsuku build. |
| 2. Reaches `tsuku eval` and `tsuku install` with non-zero exit | Yes | Returned from `PyPIProvider.ResolveLatest`; propagates through `Executor.ResolveVersion` into both commands' existing error paths. |
| 3. Bundled Python is fixed; no user knob | Yes | The wording calls it "bundled Python" rather than "your Python", reflecting that the user cannot reconfigure it. |
| 4. Out of scope: force-install flag | Honored | No `--ignore-requires-python` equivalent is added. See "Separate Proposal" below. |

## Separate Proposal (not part of this decision)

The constraint allows surfacing an escape hatch as a separate proposal if
the analysis surfaces a strong case. Analysis: a force-install flag
(`pip install --ignore-requires-python` equivalent) is a clear footgun --
it produces broken installs that fail at first import or runtime, with
no benefit over a manual `pip install` outside tsuku. There is *no*
strong case for adding this surface. **Recommendation: do not file a
follow-up.** If a future user case justifies it, that is the time to
revisit.

## Assumptions

1. Decision 1 will place the auto Python-compat filter inside
   `PyPIProvider` (or with the provider as the surface that returns the
   error). If Decision 1 instead places the filter inside
   `pipx_install.Decompose`, the wording is identical but the source
   prefix may shift from `pypi resolver:` to a Decompose-layer prefix.
   The wording template still applies; only the prefix changes.
2. Decision 2's PEP 440 evaluator can produce a single
   `Requires-Python` string for the latest incompatible release (the
   "<Z>" slot in the template). This is trivially true since the
   evaluator must already parse these strings to filter releases.
3. The bundled Python version exposed via the constants package
   (Decision 1) is a `<major>.<minor>` string. The error template uses
   `<X.Y>`; if the package surfaces a full `<major>.<minor>.<patch>`
   string, the template should still print only `<major>.<minor>` for
   readability and to match Python's own `Requires-Python` granularity.

## Rejected Options

| Option | Reason |
|--------|--------|
| B (A + actionable suggestion) | The suggestion is misleading when the error fires. After Option A's filter is in place, this branch implies *every* release is incompatible, contradicting the suggestion's "supports older Python via a backport branch" framing. |
| C (A + top-3 compatible versions) | The compatible set is empty by definition for this branch, so the list would always be empty. The option's value applies to the success path, not the failure path being decided here. |
| D (pip-style enumerate-all) | Verbose, breaks tsuku's terse-error tone, duplicates content pip's own stderr produces, adds implementation cost for marginal benefit on a rare branch. |
| E alone (without A wording) | E is a plumbing pattern, not a wording. It must be combined with one of the wording options. Combined with A as the chosen recommendation. |

## Consumer Rendering (for design doc Considered Options)

### Option A (chosen)

Plain error: `pypi resolver: no release of <package> is compatible with
bundled Python <X.Y> (latest is <V>, requires Python <Z>)`. Concise; names
package, bundled Python, latest version, and its requirement. Matches
existing tsuku version-error tone. Implemented as a typed
`*ResolverError` with a new error-type constant (Option E plumbing) so
callers can react programmatically if needed in future.

### Option B (rejected)

A plus a hint to file a follow-up if the package has a backport branch.
Rejected: the suggestion is inaccurate when the branch fires. With the
auto-filter active (Decision 1), reaching this error means no compatible
release exists at all -- not that one might exist on a backport branch.

### Option C (rejected)

A plus the top-3 compatible versions. Rejected: the compatible set is
empty by definition for this branch. The list would always be empty. The
value of the option applies to a different branch (the success path).

### Option D (rejected)

Pip-style enumeration of all releases with their `Requires-Python` strings.
Rejected: verbose, off-tone for the codebase, duplicates content pip's
own error output already provides, and adds implementation cost for
marginal user benefit on a rare branch.

### Option E (folded into A as plumbing pattern)

Two-layer surfacing: typed error from PyPIProvider, wrapped by
`pipx_install.Decompose`. Adopted as the implementation style for Option
A. Note: separate wrapping in Decompose is unnecessary because the
provider already attaches the package name; the typed error is enough.

## Report Metadata

- **Decision question:** What does tsuku surface to the user when no PyPI
  release is compatible with the bundled Python?
- **Prefix:** design_pipx-pypi-version-pinning_decision_3
- **Complexity:** standard (Tier 3 fast path -- Phases 0, 1, 2, 6)
- **Background grounding:**
  `wip/explore_2331-pipx-pypi-version-pinning_findings.md`,
  `wip/research/.../lead-pip-pipx-semantics.md`,
  `internal/version/errors.go`,
  `internal/version/provider_pypi.go`,
  `internal/version/pypi.go`,
  `internal/actions/pipx_install.go`.
