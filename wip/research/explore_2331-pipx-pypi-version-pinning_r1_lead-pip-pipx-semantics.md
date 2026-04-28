# Pip / Pipx semantics for Requires-Python compat

Research lead L4 — issue #2331 / explore round 1.

Question: when no version is pinned, when an incompatible version is pinned, and
when older releases differ in `Requires-Python`, what does pip do? Does pipx add
anything? This determines whether tsuku must pre-resolve a Python-compatible
version or can rely on pip.

## TL;DR

- **Pip 9.0+ already does the right thing** for the unpinned case. It walks the
  full release list, filters out candidates whose `Requires-Python` excludes the
  running interpreter, then picks the newest version that survives the filter.
  Empirically verified: `pip install ansible-core` on Python 3.10 selects
  `ansible-core 2.17.14` (latest 2.17.x), not 2.20.5 (latest absolute, requires
  Python 3.12+).
- **Pinning an incompatible version fails hard.** `pip install ansible-core==2.20.5`
  on Python 3.10 returns `ERROR: Could not find a version that satisfies the
  requirement` — pip does not fall back when the user supplied an exact version.
- **Pipx inherits pip's behavior** for version selection. Pipx's only role here
  is choosing *which Python* to create the venv with; once the venv is built,
  pip inside it does the resolution. Pipx does not pre-filter by
  `Requires-Python`.
- **Implication for tsuku:** if the recipe lets pip resolve (no version pinned,
  or a range), pip will already pick the newest Python-compatible release. tsuku
  only needs its own resolver if recipes pin exact versions and want a
  fallback-on-incompatibility behavior — which would be a *new policy*, not a
  fix for a pip deficiency.

## (a) Unpinned: pip picks newest compatible, skips newer incompatible

### Empirical evidence — `python:3.10-slim` Docker image

Image: `python:3.10-slim` (Python 3.10.x, pip 23.0.1).

Command: `pip install --dry-run ansible-core`

Output (truncated):
```
Collecting ansible-core
  Downloading ansible_core-2.17.14-py3-none-any.whl (2.2 MB)
  ...
Would install ... ansible-core-2.17.14 ...
```

Latest absolute is 2.20.5 (Requires-Python `>=3.12`). 2.18.x and 2.19.x require
`>=3.11`. Pip walked the list, rejected every 2.18+, and selected the highest
2.17 release (2.17.14). This matches the documented "fall back to last
compatible distribution" behavior.

Same image, `pip install --dry-run azure-cli` → selects `azure-cli-2.85.0`
(latest, since azure-cli's current Python floor still includes 3.10; this
confirms pip picks the absolute latest when nothing prevents it).

### Documentation evidence

- Python Packaging User Guide, "Dropping support for older Python versions":
  > Pip 9.0+ ... if [the current Python and Requires-Python] do not match, it
  > will attempt to install the last package distribution that supported that
  > Python runtime.
  Source:
  https://packaging.python.org/guides/dropping-older-python-versions/
- Pip dependency resolution docs: "starts by picking the most recent version"
  and backtracks on conflicts.
  Source: https://pip.pypa.io/en/stable/topics/dependency-resolution/
- Pip source: `_check_link_requires_python()` in
  `src/pip/_internal/index/package_finder.py` (LinkEvaluator) is what filters
  links whose `Requires-Python` excludes the running interpreter; surviving
  candidates are then ranked by `CandidateEvaluator._sort_key`, whose tuple
  contains `candidate.version` — so within the compatible set, newest wins.
  Source:
  https://github.com/pypa/pip/blob/main/src/pip/_internal/index/package_finder.py

### Specs underpinning the behavior

- PEP 345 defines `Requires-Python` (free-form PEP-440 specifier):
  https://peps.python.org/pep-0345/
- PEP 503 added `data-requires-python` to the simple index so installers can
  filter without downloading metadata; pip 9.0 honors it (cited in the
  packaging-user-guide page above).
- PEP 440 defines specifier syntax (used by `Requires-Python`, but PEP 440
  itself does not discuss the field): https://peps.python.org/pep-0440/

## (b) Pinned exact-version that's incompatible: hard error, no fallback

Command: `pip install --dry-run 'ansible-core==2.20.5'` on Python 3.10.

Output (truncated):
```
ERROR: Ignored the following versions that require a different python version:
  ... 2.20.5 Requires-Python >=3.12 ...
ERROR: Could not find a version that satisfies the requirement
       ansible-core==2.20.5 (from versions: 0.0.1a1, 2.11.0b1, ..., 2.17.14)
ERROR: No matching distribution found for ansible-core==2.20.5
```

Pip lists the rejected versions and the surviving compatible set, then aborts
because the exact pin can't be honored. **There is no automatic relaxation of
the pin** — the user explicitly asked for 2.20.5, and pip won't substitute
2.17.14.

`--ignore-requires-python` exists (documented on the `pip install` page) and
forces pip to disregard the metadata, but that just makes it install something
broken; it doesn't perform a smart fallback.

## (c) Multiple older releases with differing Requires-Python

Pip evaluates each release independently. The `_check_link_requires_python`
filter runs per-link, so if release N requires `>=3.11`, N-1 requires `>=3.10`,
and N-2 requires `>=3.9`, on Python 3.10 pip rejects N, accepts N-1 and N-2,
and picks N-1 (newest of the surviving set).

Empirical confirmation comes from the ansible-core run above — pip's error
message even when you pin an incompatible version enumerates the per-release
`Requires-Python` values, showing it really does inspect every release:

```
2.18.0 Requires-Python >=3.11
2.18.10 Requires-Python >=3.11
...
2.19.x Requires-Python >=3.11
2.20.0 Requires-Python >=3.12
2.20.5 Requires-Python >=3.12
```

(2.17.x and earlier had no such line, meaning their `Requires-Python` was
satisfied by Python 3.10. The compatible set ended at 2.17.14, which is what
pip selected when no version was pinned.)

The selection order is "newest first, within compatible set" — confirmed by
the `_sort_key` tuple in pip's `CandidateEvaluator`, whose `candidate.version`
component dominates absent hash/yank/binary preferences.

## Pipx: thin wrapper, no extra logic for Requires-Python

Pipx creates a venv (using the Python it was told to use, defaulting to its own
interpreter or `PIPX_DEFAULT_PYTHON`), then runs `pip install <pkg>` inside
that venv. The version selection is pip's, not pipx's.

- "pipx creates virtual environments using your default Python interpreter,
  and pip resolves the latest package version compatible with that
  interpreter." — pipx documentation/community articles.
  Source: https://pipx.pypa.io/stable/
  Source (community): https://til.codeinthehole.com/posts/how-pipx-choose-which-python-to-install-a-package-with/
- Pipx exposes `--python` and `--fetch-missing-python` to control the
  interpreter, and `PIPX_DEFAULT_PYTHON` as an env override. None of these
  change the version-resolution algorithm; they only change which Python is in
  the venv.
- Notable consequence (also called out in the search results): if the running
  Python's compatible release is older than the absolute latest, **pipx
  silently installs the older release** because pip silently selects it.
  There's no warning that the user is getting a back-version.

## Pip-version caveats

- Behavior described requires **pip 9.0 or newer** (released Nov 2016). Anything
  older is irrelevant in practice — every supported Python ships with a much
  newer pip. Pipx itself requires recent pip.
- The 2020 pip resolver rewrite (the "new resolver", default since pip 20.3)
  did not change the `Requires-Python` filtering — that's a candidate-filtering
  step, applied before resolution. The new resolver changed how *dependency
  conflicts* are handled, not how Python compatibility is filtered.
- A handful of bugs have surfaced over the years where `Requires-Python`
  filtering interacted badly with caching or error messages (e.g. pip
  changelog mentions of "fixed a regression that caused dependencies to be
  checked before Requires-Python project metadata is checked"), but the
  documented and observed top-level behavior has been stable for years.
  Source: https://pip.pypa.io/en/stable/news/

## What this means for the design (issue #2331)

The original concern motivating the issue — "if a user is on Python 3.10 and
the recipe runs `pipx install ansible-core`, will pipx try to install 2.20.5
and fail?" — is **not how pip/pipx behave**. Pip will pick 2.17.14
automatically.

The remaining design questions are:

1. **Does tsuku want the same behavior, or does it want exact-version pinning
   in recipes?** If recipes set `version = "2.20.5"` explicitly, pip will
   refuse on incompatible Python and tsuku must decide whether to:
   (a) propagate the failure,
   (b) relax the pin to the newest Python-compatible release, or
   (c) refuse to even attempt and produce a clearer error.
2. **If tsuku wants to surface what version was actually installed** (since pip
   does this silently when it falls back), tsuku would need to parse pip's
   output or query the venv after install — pipx itself doesn't surface this.
3. **If recipes pin a range** (e.g. `>=2.17,<2.18`), pip will already pick the
   newest compatible release within that range. No tsuku-side resolution needed.

## Sources cited

- PEP 345 (Requires-Python field): https://peps.python.org/pep-0345/
- PEP 440 (version specifier syntax): https://peps.python.org/pep-0440/
- PEP 503 (data-requires-python on simple index): https://peps.python.org/pep-0503/
- Pip dependency resolution: https://pip.pypa.io/en/stable/topics/dependency-resolution/
- Pip install reference (incl. `--ignore-requires-python`): https://pip.pypa.io/en/stable/cli/pip_install/
- Pip package-finding architecture: https://pip.pypa.io/en/stable/development/architecture/package-finding/
- Pip source (`package_finder.py`, `_check_link_requires_python`, `_sort_key`):
  https://github.com/pypa/pip/blob/main/src/pip/_internal/index/package_finder.py
- Python Packaging User Guide, dropping older Python versions:
  https://packaging.python.org/guides/dropping-older-python-versions/
- Pipx documentation: https://pipx.pypa.io/stable/
- Pipx Python-selection article (David Winterbottom):
  https://til.codeinthehole.com/posts/how-pipx-choose-which-python-to-install-a-package-with/
- Empirical: `docker run --rm python:3.10-slim pip install --dry-run ansible-core`
  → installs `ansible-core 2.17.14` (latest 2.17 line; 2.18+ require
  Python >=3.11, 2.20 requires >=3.12).
- Empirical: `docker run --rm python:3.10-slim pip install --dry-run
  'ansible-core==2.20.5'` → `ERROR: No matching distribution found`.
- Empirical: `docker run --rm python:3.10-slim pip install --dry-run azure-cli`
  → installs `azure-cli 2.85.0` (no Python-compat skip needed).
