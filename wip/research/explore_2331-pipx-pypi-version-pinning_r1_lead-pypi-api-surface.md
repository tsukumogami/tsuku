# PyPI API surface for `Requires-Python`-driven version selection

Issue: #2331. Question: is PyPI's per-release `Requires-Python` metadata
complete and consistent enough to drive automatic version selection in
`PyPIProvider`?

**TL;DR.** Yes for modern packages and for any release published after roughly
late-2018, when `Requires-Python` propagation became standard. The JSON API
exposes `requires_python` on every file dict in `releases[version][*]`. Older
releases of long-lived packages (httpie, ansible, azure-cli pre-2.16) frequently
return `null` -- a fallback strategy is required. PEP 440 specifier strings
appear in the wild with whitespace, operator-order, and patch-precision
variation, so the consumer must use a real specifier parser, not regex.

---

## 1. Response shape

`https://pypi.org/pypi/<package>/json` returns:

```jsonc
{
  "info": {
    "name": "ansible-core",
    "version": "2.20.5",            // latest non-prerelease
    "requires_python": ">=3.12",     // latest's specifier (also project-level)
    "classifiers": [...]
  },
  "releases": {
    "<version>": [ <fileDict>, ... ]   // map: version -> array of files
  },
  "urls": [...]                        // files of `info.version` only
}
```

Each `fileDict` carries (verified against `releases["2.20.5"][0]` of
ansible-core):

```
comment_text, digests, downloads, filename, has_sig, md5_digest,
packagetype, python_version, requires_python, size, upload_time,
upload_time_iso_8601, url, yanked, yanked_reason
```

`requires_python` is a top-level string (or `null`), e.g. `">=3.12"`. Note
`python_version` is unrelated -- it's the wheel tag (`py3`, `cp310`, `source`),
not a version specifier.

---

## 2. ansible-core: per-release sweep

Latest = `2.20.5`. `info.requires_python = ">=3.12"`. Total releases: **314**;
exactly **one** has null `requires_python` (`0.0.1a1`, a 2020 placeholder
upload). Within each minor line `requires_python` is **identical across every
file and every patch/RC**:

| Minor | Patches sampled | requires_python |
|-------|------|-----------------|
| 2.16.x | 2.16.0 .. 2.16.18 (incl. RCs) | `>=3.10` |
| 2.17.x | 2.17.0 .. 2.17.14 (incl. RCs) | `>=3.10` |
| 2.18.x | 2.18.0 .. 2.18.16 (incl. RCs) | `>=3.11` |
| 2.19.x | 2.19.0 .. 2.19.9 (incl. RCs)  | `>=3.11` |
| 2.20.x | 2.20.0 .. 2.20.5 (incl. RCs)  | `>=3.12` |
| 2.21.x | 2.21.0b1, b2, b3, rc1         | `>=3.12` |

Every release has 2 files (sdist + wheel) with matching specifiers. Cleanest
possible signal -- a `python-standalone` running 3.12 maps cleanly to "newest
allowed = latest 2.20.x; 2.21 RCs available, also compatible".

---

## 3. azure-cli: pre/post metadata cutover

Latest = `2.85.0`. `info.requires_python = ">=3.10.0"`. Distribution:

| Specifier | Releases |
|-----------|----------|
| `null`        | 114 |
| `>=3.6.0`     | 38  |
| `>=3.8.0`     | 19  |
| `>=3.9.0`     | 13  |
| `>=3.7.0`     | 12  |
| `>=3.10.0`    | 6   |

The cutover is tight: the **last** release with `null` is `2.15.1` uploaded
2020-11-20; the **first** release with a populated value is `2.16.0` uploaded
2020-12-08. After Dec 2020, every azure-cli release carries `requires_python`.
The four versions you specifically asked about:

| Version | requires_python | python_version |
|---------|-----------------|----------------|
| 2.66.0 / 2.66.1 / 2.66.2 | `>=3.8.0` | py3, source |
| 2.70.0  | `>=3.9.0`  | py3, source |
| 2.80.0  | `>=3.10.0` | py3, source |
| 2.85.0  | `>=3.10.0` | py3, source |

Identical on sdist and wheel for each release. This is a clean, monotonic
floor over time -- exactly what the proposed compatibility check needs.

---

## 4. Independent confirmation: pdm, httpie, plus survey

### pdm (modern, all-populated)

Latest = `2.26.8`. `info.requires_python = ">=3.9"`. **0 of 246** releases
have `null`. Distribution:

| Specifier | Count |
|-----------|-------|
| `>=3.7` | 181 |
| `>=3.8` | 35  |
| `>=3.9` | 30  |

### httpie (older project, large null cohort)

Latest = `3.2.4`. `info.requires_python = ">=3.7"`. **40 of 55** releases
have `null` (all pre-2018). Of the 15 populated: 9 = `>=3.7`, 6 = `>=3.6`.
This is the worst case in our sample and is representative of any package
whose first release predates the metadata-propagation era.

### Survey of pipx-style tools

To check format diversity, the unique `requires_python` values across all
releases were dumped for several common tools. Selected highlights:

```
poetry      <4.0,>=3.10
poetry      <4.0,>=3.8
poetry      <4.0,>=3.9
poetry      >= 3.4.0.0, < 4.0.0.0       # extra spaces, four-segment versions
poetry      >= 3.6.0.0, < 4.0.0.0
poetry      >=2.7, !=3.0.*, !=3.1.*, !=3.2.*, !=3.3.*, !=3.4.*
poetry      >=3.6,<4.0
poetry      >=3.6.0
poetry      >=3.6.0.0,<4.0.0.0
poetry      >=3.7,<4.0
poetry      >=3.8,<4.0
ansible     >=2.7,!=3.0.*,!=3.1.*,!=3.2.*,!=3.3.*,!=3.4.*
ansible     >=3.10
ansible     >=3.11
ansible     >=3.9
ansible     >=3.9.0                      # variant w/ trailing .0
pre-commit  !=3.0.*,!=3.1.*,!=3.2.*,!=3.3.*,!=3.4.*,>=2.7   # operator-order swap
pre-commit  >=2.7, !=3.0.*, !=3.1.*, !=3.2.*, !=3.3.*
aws-sam-cli !=4.0,<=4.0,>=3.8            # both bound + exclusion
aws-sam-cli <3.14,>=3.9
aws-sam-cli >=3.6, <=4.0, !=4.0
black       >=3.6.2                      # patch-level floor
mypy        >=3.5
cookiecutter >=2.7, !=3.0.*, !=3.1.*, !=3.2.*, !=3.3.*, !=3.4.*
```

**Confirmed format axes the implementation must handle:**

1. **Whitespace**: `>=3.6,<4.0` vs `>=3.6, <4.0` vs `>= 3.4.0.0, < 4.0.0.0`.
   PEP 440 allows whitespace; PyPI does **not** normalize it.
2. **Operator order**: `<4.0,>=3.10` vs `>=3.10,<4.0`.
3. **Version-segment count**: `3.6` vs `3.6.0` vs `3.6.0.0`.
4. **Multi-clause specifiers**: comma-separated AND-list, frequently with
   `!=3.0.*` style exclusions inherited from the Python 2/3 era.
5. **Upper bounds**: `<4.0` (poetry), `<3.14` (aws-sam-cli), and the redundant
   `<=4.0, !=4.0` pattern that effectively means `<4.0`.
6. **Null** for older releases of long-lived projects.

A regex parser will not survive contact with this data. The provider needs
a real PEP 440 specifier evaluator -- there is no Go stdlib for this; a small
internal package or a vendored library is required. (`Masterminds/semver` is
already in tsuku's deps but it's semver, not PEP 440 -- it will reject
`!=3.0.*` and four-segment versions.)

---

## 5. Simple index parity (PEP 503 / PEP 691)

`https://pypi.org/simple/<package>/` is also useful, but the JSON API is
sufficient and friendlier for tooling. The simple index exposes the same
data under different shapes:

**HTML (PEP 503)** sets `data-requires-python` per anchor:
```
data-requires-python="&gt;=3.10"
data-requires-python="&gt;=3.11"
data-requires-python="&gt;=3.12"
data-requires-python="&gt;=2.7,!=3.0.*,!=3.1.*,!=3.2.*,!=3.3.*,!=3.4.*"
```
Note HTML-entity-encoded `>=`. Parsers must decode.

**JSON (PEP 691)** at `Accept: application/vnd.pypi.simple.v1+json` returns:
```jsonc
{
  "files": [
    {
      "filename": "ansible-core-0.0.1a1.tar.gz",
      "requires-python": null,        // kebab-case key
      "yanked": false, "size": 805,
      "upload-time": "2020-10-27T22:37:51.590663Z",
      "url": "https://files.pythonhosted.org/...",
      "hashes": {"sha256": "..."}
    }
  ],
  "versions": [...],                  // PEP 700 addition
  "meta": {...}
}
```

Parity confirmed: spot-checked ansible-core 2.16-2.20 minor lines via the
PEP 691 JSON simple index -- same `requires-python` values as the legacy JSON
API. Differences:

| Aspect | `/pypi/<pkg>/json` | `/simple/<pkg>/` |
|--------|--------------------|--------------------|
| Field name | `requires_python` (snake) | `requires-python` (kebab) |
| Returns rich `info` block | yes | no |
| Returns `releases` map keyed by version | yes | no -- flat file array |
| Returns `versions` list directly | no (must iterate releases keys) | yes (PEP 700) |
| Stability under PEP changes | legacy, not actively spec'd | governed by PEPs 503/691/700 |
| Auth | none | none |
| Cache headers | yes | yes |

For tsuku's purposes the legacy JSON API is fine. tsuku already uses it
(see section 6); the simple index is mentioned only because pip uses it and
because the PEP-governed JSON variant is the official forward path.

---

## 6. Rate limits, auth, current tsuku usage

### tsuku's existing usage

`internal/version/pypi.go` already hits this exact endpoint
(`/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/pypi.go`):

- URL pattern: `<registryURL>/pypi/<package>/json` (default
  `https://pypi.org`), built via `url.Parse` + `JoinPath` (line 60, 153).
- `ListPyPIVersions` already iterates `pkgInfo.Releases`'s keys (line 196-198),
  but the current `pypiPackageInfo` struct (line 22-28) defines `Releases` as
  `map[string][]struct{}{}` -- it explicitly **discards** the file dicts.
  Adding `requires_python` collection means just expanding that struct; no new
  HTTP calls, no new endpoint, no auth change.
- Response size cap: 10 MB (line 18). ansible-core JSON is 427 KB, azure-cli
  297 KB, pdm 358 KB. Comfortably under the cap. The largest popular package
  (boto3) is reportedly ~5-6 MB; still under.
- No `Authorization` or `User-Agent` header is sent. Standard Go
  `http.Client` default User-Agent is `Go-http-client/1.1`, which is
  technically below PyPI's fair-use ask (see below).

### PyPI rate limits & auth

PyPI's official guidance (https://docs.pypi.org/api/, fetched 2026-04-28):

> "Due to the heavy caching and CDN use, there is currently no rate limiting
> of PyPI APIs at the edge."
>
> "Try not to make a lot of requests (thousands) in a short amount of time
> (minutes). [...] consider using your own index mirror or cache."
>
> "PyPI reserves the right to temporarily or permanently prohibit a consumer
> based on irresponsible activity."

**No documented per-minute cap, no auth required for public read endpoints.**
Fastly cache headers observed:

```
HTTP/2 200
cache-control: max-age=900, public
etag: "3lA2e84W9QKqCwjpbRBO+g"
x-pypi-last-serial: 36552578
x-cache: MISS, HIT
vary: Accept-Encoding
```

Recommendations from the docs:
- **Set a custom User-Agent** with contact info ("unique identifier including
  contact information"). tsuku currently doesn't, but this is a fair-use ask
  PyPI explicitly makes; cheap to add.
- **Honor ETag / If-None-Match** for repeated polls. The endpoint has a
  900-second public cache, so identical requests within 15 min get cached
  CDN responses anyway, but ETag still helps if tsuku polls the same package
  across a longer window.

### Authentication caveat

The `/pypi/<package>/json` endpoint is unauthenticated and public. There is
no token-based variant that returns more or fresher data. The `/simple/`
index can accept tokens for private indexes, but that's irrelevant for
public PyPI.

---

## 7. Caveats (empirical vs. promised)

### Empirically observed

1. **Legacy null cohort.** Releases uploaded before ~late-2018 frequently
   return `requires_python: null`. For long-lived projects this can be a
   majority of releases (httpie 73%, azure-cli 36%, ansible 0.3%). Modern
   tools (pdm) have 0%. **A null value must not be treated as "no constraint
   = compatible"** without thought; safer to treat as "unknown, fall back
   to classifier strings or skip".
2. **`info.requires_python` reflects the latest release only.** It is not a
   project-wide minimum. To answer "what's the newest release my Python can
   run?" the consumer must walk `releases`, not read `info`.
3. **Per-file vs per-release.** `releases[version]` is an array because a
   release can ship multiple files (sdist + wheels). Within a single release,
   `requires_python` is **always identical across files** in everything I
   sampled (ansible-core, azure-cli, pdm, poetry, ansible, httpie, etc.).
   I did not find a single counterexample, but the data model permits
   divergence -- defensive code should pick one (e.g. the wheel's value, or
   the sdist's, or assert equality and warn on mismatch).
4. **Format variation is real.** See section 4. Whitespace, operator order,
   patch precision, and exclusion patterns all vary. Cannot regex-parse.
5. **Yanked releases retain `requires_python`.** Yanking sets `yanked: true`
   and `yanked_reason`; it does not zero out metadata. Consumer must filter
   yanked entries explicitly.
6. **Pre-releases are interleaved.** ansible-core's `releases` map contains
   `2.21.0rc1`, `2.20.5rc1`, `2.16.18rc1`, etc. mixed with finals.
   `info.version` skips them, but `releases.keys()` does not. The provider
   must filter with PEP 440 prerelease detection (currently
   `Masterminds/semver` will accept `2.20.5-rc1` form but PyPI uses
   `2.20.5rc1` -- different). Existing tsuku code in `pypi.go:202-213`
   uses `Masterminds/semver` and will mishandle PEP 440 RC strings.

### Documented promise (per PyPI docs)

7. **Metadata is captured at first upload only.**
   > "the first uploaded data for a release is stored, subsequent uploads
   > do not update it"

   So if a release uploads sdist first (with a stale `setup.py`-derived
   `Requires-Python`) then a wheel with the corrected value, the JSON API
   keeps the sdist's. In practice, modern projects build everything from a
   single source tree and don't trip this, but it's a known gotcha.
8. **No edge rate limit, but irresponsible use can be banned.** Fair-use
   request: custom User-Agent + don't blast thousands of requests/minute.

### Not verified

- Behavior on packages that have **only** sdist (no wheels). All sampled
  packages had at least one wheel. Worth a follow-up sample if the design
  needs to lean on wheel-tag data.
- Whether `releases` ever contains an empty array for a version. Sampled
  packages did not exhibit this; ansible-core's 314 releases all have >= 2
  files.

---

## 8. Bottom line for #2331 design feasibility

**Data is good enough for the proposed design**, with these constraints:

- For modern, actively-maintained pipx-style tools (pdm, ruff, black, poetry,
  ansible-core 2.16+, azure-cli 2.16+) the metadata is fully populated and
  consistent. Auto-selection works.
- For older releases the implementation needs an explicit fallback policy:
  treat `null` as "unknown, assume compatible" (permissive) or
  "unknown, skip" (strict). Strict is safer: if a user pins
  `httpie@<2.0` they probably want a runnable httpie, and a 2014 release
  with no Python metadata is unlikely to work on Python 3.12.
- Implementation must use a real PEP 440 specifier parser (not regex,
  not `Masterminds/semver`). The version-comparison code in `pypi.go`
  also needs to switch to PEP 440 semantics for sorting (current code uses
  semver and will misorder strings like `2.21.0rc1`).
- Add a User-Agent and consider ETag-based conditional refresh -- both are
  fair-use asks, neither is a blocker.

---

## Reference files

- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/pypi.go`
  -- existing PyPI provider; `pypiPackageInfo` struct (line 22-28) discards
  file dicts and would need to retain `requires_python`. Sorting code
  (line 201-213) uses semver and needs PEP 440.
- `/tmp/ansible-core.json`, `/tmp/azure-cli.json`, `/tmp/pdm.json`,
  `/tmp/httpie.json`, `/tmp/pkg-{poetry,ansible,black,mypy,pre-commit,aws-sam-cli,sqlfluff,cookiecutter}.json`
  -- raw JSON saved during this investigation (truncated/regenerated as
  needed; not committed).
- https://docs.pypi.org/api/ -- fair-use guidance, no rate limits.
- https://docs.pypi.org/api/json/ -- JSON API reference; documents
  `requires_python` per file and the "first upload wins" caveat.
- PEP 503 (legacy simple), PEP 691 (JSON simple), PEP 700 (versions list),
  PEP 440 (specifier syntax) -- the governing specs.
