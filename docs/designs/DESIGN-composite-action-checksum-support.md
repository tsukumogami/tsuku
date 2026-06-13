---
schema: design/v1
status: Accepted
problem: |
  github_archive and download_archive composites pass nil for the
  checksum slot when delegating to download, so recipes that use
  the composites cannot opt into upstream-pinned SHA256
  verification without re-authoring as a manual download chain.
decision: |
  Add checksum_url and checksum_asset parameter forwarding on
  both composites. The composite Decompose resolves
  checksum_asset to a sibling-asset URL at plan time (composite
  layer has the repo+tag context), populates Step.Checksum from
  the fetched manifest line, and emits the same download_file
  step shape as before so existing golden files remain stable.
  Multi-line SHA256SUMS manifests are auto-detected from file
  content alongside the existing per-asset single-line format.
rationale: |
  Adding checksum_asset URL resolution at the composite layer
  preserves the primitive download action's GitHub-agnostic
  contract. Populating Step.Checksum without expanding the
  download step's params keeps the golden file content
  byte-identical for the 108 existing composite recipes that
  pass no checksum field, satisfying R9 backward compatibility
  without a format_version bump. Auto-detection of manifest
  format follows the upstream convention (sha256sum/shasum
  output is unambiguously parseable line-by-line) and avoids a
  new mandatory field on every checksummed recipe.
upstream: docs/prds/PRD-composite-action-checksum-support.md
---

# DESIGN: composite-action-checksum-support

## Status

Accepted

This DESIGN settles the PRD's Decision D4 (manifest format
auto-detection vs explicit field) and the seven implementation
traps the pre-/scope testing-infra audit surfaced. The chosen
shape is field-forwarding plumbing at the composite layer; no
new abstractions, no expansion of the primitive download
action's contract.

## Context and Problem Statement

The composite actions `github_archive` and `download_archive`
both decompose into a `download` step that already supports
upstream checksum verification via the `checksum_url`
parameter. The composites do not forward any checksum
parameter — both Decompose call sites in
`internal/actions/composites.go` (line 594 for `github_archive`,
line 288 for `download_archive`) invoke
`decomposeDownload(ctx, url, name, nil, nil)` with `nil` in
the checksum and signature slots.

Recipe authors who need upstream verification today escape the
composites and re-author the recipe as a manual `download` +
`extract` + `chmod` + `install_binaries` sequence (10 recipes
do this in the current registry — 8 HashiCorp tools with
`SHA256SUMS` manifests, plus cargo-nextest and kubectl/helm-
adjacent recipes). The escape works but discards the
composite's platform-cased ergonomics.

The PRD's R1–R6 require checksum forwarding through the
composite layer with support for both per-asset single-line
checksum files and multi-line `SHA256SUMS` manifests. R9
mandates that existing recipes produce identical plan output
(byte-identical golden files) — the change must be purely
additive at every layer the validator inspects.

## Decision Drivers

- **D1. Backward compatibility — non-negotiable.** Every
  existing recipe that passes no checksum field produces the
  same plan output (identical golden file content) as before.
  The validator inventory found 97 github_archive + 11
  download_archive recipes with no checksum field today. None
  may regress.
- **D2. Minimal API surface change.** The PRD asks for one new
  field (`checksum_url`) plus one ergonomic alias
  (`checksum_asset`). The DESIGN should not introduce new
  abstractions — no checksum-source registry, no pluggable
  checksum parser, no new interface types. Plumbing, not
  architecture.
- **D3. Golden-file format stability.** Bumping
  `format_version` from 5 to 6 forces regeneration of every
  golden file (155 embedded recipes × 3 platforms ≈ 418
  files). The DESIGN should arrive at a shape that keeps v5
  golden files valid; if v6 turns out to be unavoidable, the
  DESIGN must make that explicit so the PLAN can scope the
  regen.
- **D4. Multi-line manifest support.** Codex ships a multi-line
  `codex-package_SHA256SUMS` manifest; HashiCorp tools all
  ship manifests. The parser must handle this shape alongside
  the existing per-asset `.sha256` shape (per-asset is the
  existing-test-covered path; manifest is the new one).
- **D5. Mutual exclusion of `checksum_url` and `checksum_asset`.**
  Setting both on a single step is ambiguous. Preflight catches
  the conflict; runtime is too late.
- **D6. nil-`Downloader` callers must continue working.**
  `--dry-run` and validate flows generate plans without a
  `Downloader`. The existing pattern at
  `internal/executor/plan_generator.go:325-340` gracefully
  no-ops when `Downloader == nil`; the new code preserves the
  same pattern when fetching checksum files.
- **D7. Cache-invalidation safety.** The download cache keys on
  URL + `checksum_algo`, not on checksum value
  (`internal/actions/download_cache.go`). If a recipe's upstream
  asset rotates (same URL, new checksum), the new code must
  not serve stale cached content; cache lookups must validate
  against the freshly-fetched expected checksum, not the
  cached one.
- **D8. Validator field-registry update.** Without registering
  the new fields in `internal/recipe/types.go`, the validator
  fires unknown-field warnings. The fields must be registered
  on both `github_archive` and `download_archive`.

## Considered Options

### Option A: Field forwarding at the composite layer (selected)

Both composites accept `checksum_url` and (github_archive only)
`checksum_asset` as optional string parameters. The composite's
`Decompose` extracts the field, resolves placeholders, and:

- For `checksum_url`: passes the resolved URL into the
  decomposed `download` step via the existing `decomposeDownload`
  helper, which is extended to accept a checksum URL argument
  (currently passed `nil`).
- For `checksum_asset`: constructs the sibling URL
  (`https://github.com/<repo>/releases/download/<tag>/<asset>`)
  at the composite layer, then forwards that resolved URL the
  same way.

The primitive `download` action's contract is unchanged. The
composite-layer change is the `decomposeDownload` signature
gain (one extra string parameter) plus param extraction in each
composite's `Decompose`.

**Why selected:** Smallest blast radius. The
`decomposeDownload` helper is the natural seam (already shared
by both composites). `checksum_asset` resolution lives at the
composite layer because the composite has the repo+tag context
the primitive deliberately does not have. The primitive stays
GitHub-agnostic.

### Option B: Push `checksum_asset` resolution into the primitive

The primitive `download` action grows GitHub awareness — a new
field that takes a repo+tag+asset triple and constructs the
sibling URL itself. The composites forward all three.

**Why rejected:** Couples the primitive to GitHub. The primitive
download action's contract today is "given a URL, fetch and
verify it." Growing GitHub-specific construction logic in the
primitive violates the layering — the composites exist to
encapsulate GitHub-specific knowledge (`github_archive` already
constructs the asset download URL). Pushing more such logic
down breaks the composite/primitive separation.

### Option C: Explicit `checksum_format` field on each recipe

Every recipe that uses `checksum_url` or `checksum_asset` also
declares `checksum_format = "single" | "manifest"`. The
verifier branches on the field; no auto-detection.

**Why rejected:** Adds a mandatory field for the common case
(per-asset .sha256 is the dominant shape; manifest is rarer)
when the file content is unambiguously self-describing.
sha256sum/shasum format is line-delimited with a fixed
`<hex>  <filename>` shape; presence of multiple non-empty
lines unambiguously signals "manifest" and the absence
signals "single-line." Forcing the recipe author to declare
the format adds boilerplate to every checksummed recipe.

### Option D: Embed the manifest line in the recipe directly

The recipe author copies the expected SHA256 hex into a
`checksum` field at recipe-write time. No fetch, no manifest
parse — the recipe is self-contained.

**Why rejected:** The primitive `download` action already
rejected this shape (line 42-44 of `internal/actions/download.go`
explicitly warns "use checksum_url or download_file" for static
checksums). Recipes that pin static checksums lock to a
specific upstream version; updating to a new version requires
both an `asset_pattern` edit AND a `checksum` edit. The
composite layer should preserve the dynamic-resolution model
the primitive already established.

## Decision Outcome

Option A: field forwarding at the composite layer. The
specifics:

### Decision D-A1: `decomposeDownload` signature extension

The helper at `internal/actions/composites.go:27-60` gains one
parameter:

```go
func decomposeDownload(
    ctx *EvalContext,
    url string,
    dest string,
    osMapping, archMapping map[string]string,
    checksumURL string,  // NEW — empty string means "no upstream checksum"
) (Step, error)
```

Existing call sites (both composites) currently pass no checksum
URL; they will pass the resolved value from the composite's
`Decompose` (empty string when the recipe author did not set
either checksum field).

### Decision D-A2: Composite Preflight grows three field validations

`GitHubArchiveAction.Preflight` (composites.go:343-381) and
`DownloadArchiveAction.Preflight` (composites.go:72-101) each
gain three checks (two errors + one warning):

- Reject if both `checksum_url` AND `checksum_asset` are set
  (D5 mutual exclusion). Error: `"checksum_url and
  checksum_asset are mutually exclusive on a single step; use
  one"`.
- (`github_archive` only) Reject if `checksum_asset` is set on
  a recipe with `asset_pattern` using wildcards — the sibling
  URL cannot be constructed without the resolved asset name.
  Error: `"checksum_asset is not supported with wildcard
  asset_pattern; use checksum_url instead"`.
- Warn if `checksum_url` is set WITHOUT `{version}` placeholder
  while `asset_pattern` (`github_archive`) or `url`
  (`download_archive`) contains `{version}`. The static-
  checksum-with-versioned-asset combination would fetch the
  same checksum file for every version install, surfacing every
  version bump as a hash mismatch rather than a successful
  upgrade. Warning, not error, because the rare recipe where
  upstream publishes one signing artifact across versions is
  legitimate and should not be blocked. Warning text:
  `"checksum_url has no {version} placeholder but
  asset_pattern is version-templated; each install will fetch
  the same checksum file regardless of version — likely a
  recipe authoring mistake"`.

`download_archive` does NOT accept `checksum_asset` (per PRD
R3 / D3); Preflight rejects the field with `"checksum_asset
is github_archive-only; use checksum_url on download_archive"`.

### Decision D-A3: Composite Decompose resolves `checksum_asset` to a URL

In `GitHubArchiveAction.Decompose`, after the existing asset
resolution:

```go
checksumURL := ""
if asset, ok := GetString(params, "checksum_asset"); ok && asset != "" {
    checksumURL = fmt.Sprintf(
        "https://github.com/%s/releases/download/%s/%s",
        repo, versionTag, ExpandVars(asset, vars),
    )
} else if url, ok := GetString(params, "checksum_url"); ok && url != "" {
    checksumURL = ExpandVars(url, vars)
}
```

`download_archive.Decompose` is the same minus the
`checksum_asset` branch.

### Decision D-A4: Multi-line manifest auto-detection in checksum.go

`internal/actions/checksum.go`'s `ReadChecksumFile` is the
parsing seam. The function gains a target-filename parameter
and an auto-detect step:

```go
func ReadChecksumFile(path string, targetAsset string) (string, error) {
    content, err := os.ReadFile(path)
    if err != nil { return "", err }
    lines := strings.Split(strings.TrimSpace(string(content)), "\n")
    nonBlank := filterBlank(lines)
    if len(nonBlank) == 1 {
        return parseSingleLine(nonBlank[0])  // existing behavior
    }
    return findManifestLine(nonBlank, targetAsset)  // new behavior
}
```

The detection rule: one non-blank line ⇒ single-line shape;
two or more non-blank lines ⇒ manifest shape. Filename matching
in manifest mode is exact AFTER stripping a leading `*`
(binary-mode marker from `sha256sum -b`) and after taking the
last path component (some manifests prefix `./` or longer
paths). Lines that do not parse (insufficient whitespace
separation, hex column not 64 chars) are skipped silently;
only the line whose normalized filename matches the target is
returned.

Callers (the existing checksum-verify path in download.go) pass
the resolved asset name as `targetAsset`. The download step
already knows the asset filename (it's the `dest` parameter).

### Decision D-A5: Step.Checksum populated; download step params unchanged

The composite Decompose computes the expected checksum from
the manifest at plan time (via the existing `Downloader` →
`PreDownloader` path, extended to fetch the checksum file when
`checksumURL != ""`) and stores it in `Step.Checksum`. The
`download_file` step's `params` map is NOT extended with
`checksum_url` or `checksum_asset` — only `Step.Checksum`
carries the value.

**This is the key D3 (golden-file stability) decision.** The
emitted `download_file` step shape is byte-identical to the
pre-change shape for every existing recipe (no new params
means no diff in the params map). Only recipes that opt into
checksum forwarding see new content in `Step.Checksum`, and
even there the change is additive (the field already exists in
v5).

### Decision D-A6: Validator field registry update

`internal/recipe/types.go:894` and surrounding registry map
gain entries for `checksum_url` and `checksum_asset` on both
composites' field lists. The validator's unknown-field warning
no longer fires when these are present.

### Decision D-A7: Cache-invalidation hardening

The download cache (`internal/actions/download_cache.go`) keys
on `URL + checksum_algo`. When `checksumURL != ""`, the cache
lookup additionally re-fetches the checksum file at install
time and compares against the cached blob's checksum;
mismatch invalidates the cache entry and triggers a fresh
download. The existing cache-miss path is unchanged.

This closes the D7 surface: a rotated upstream asset (same
URL, new checksum) is detected before the cached (stale) blob
is reused.

### Decision D-A8: nil-Downloader graceful no-op

When `EvalContext.Downloader == nil` (the existing
`--dry-run`/validate code path), the composite Decompose does
NOT fetch the checksum file. The decomposed `download` step
is emitted with `Step.Checksum == ""`, and the existing
nil-Downloader fallback path at
`internal/executor/plan_generator.go:325-340` handles it.
This matches the existing primitive `download` action's
behavior under the same condition.

## Solution Architecture

### Components touched

| File | Change |
|------|--------|
| `internal/actions/composites.go` | `decomposeDownload` signature extension (one parameter); Preflight + Decompose for both composites; sibling-URL construction in github_archive |
| `internal/actions/checksum.go` | `ReadChecksumFile` signature extension (target filename); auto-detection between single-line and manifest formats; manifest filename normalization |
| `internal/recipe/types.go` | Field-registry entries for `checksum_url` and `checksum_asset` on both composites |
| `internal/validate/predownload.go` | (Possibly) extension to fetch the checksum file at plan time so `Step.Checksum` can be populated. Verify the existing `PreDownloader.Download` is sufficient; if not, add a `DownloadText` shape for the checksum file fetch. |
| `recipes/c/codex.toml` | New recipe — the validating consumer |
| `plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md` | Document new fields with examples |

### Data flow

```
Recipe author writes step
  ↓
Phase 1: Plan generation
  ↓
github_archive.Decompose
  ├─ extract checksum_asset OR checksum_url from params
  ├─ resolve placeholders + (for asset) construct sibling URL
  ├─ call decomposeDownload(..., checksumURL)
  │   ├─ if Downloader != nil AND checksumURL != "":
  │   │   ├─ fetch <checksumURL> via PreDownloader
  │   │   ├─ pass content to ReadChecksumFile(content, dest)
  │   │   └─ assign returned hash to Step.Checksum
  │   └─ if Downloader == nil OR checksumURL == "":
  │       └─ existing behavior (Step.Checksum stays empty or
  │           is computed from archive download at plan time)
  ↓
Phase 2: Plan execution
  ↓
download_file step runs
  ├─ download archive
  ├─ if Step.Checksum != "": verify SHA256 matches
  │   ├─ match: extract
  │   └─ mismatch: abort with error naming asset + hashes
  └─ if Step.Checksum == "": skip verification (existing
      behavior for unchecksummed recipes)
```

### Test surface

- **Unit: composite forwarding** —
  `internal/actions/composites_decompose_test.go` gains four
  tests: github_archive with checksum_url, github_archive with
  checksum_asset, download_archive with checksum_url, and the
  mutual-exclusion Preflight rejection. Verify the emitted
  download step's `Step.Checksum` matches the expected value.
- **Unit: manifest parsing** —
  `internal/actions/checksum_test.go` gains six tests: single-
  line happy path, manifest happy path with binary-mode marker
  (`*` prefix), manifest with path-prefixed filename,
  manifest with no matching line, manifest with malformed
  lines (skipped silently), and the auto-detection boundary
  (one line vs two lines).
- **Integration: backward-compat golden files** — a script
  (`scripts/validate-golden.sh`) is run before and after the
  change on a representative sample of 20 existing
  github_archive + download_archive recipes; diff must be empty.
- **End-to-end: codex install** — manual run of
  `tsuku install --recipe-file recipes/c/codex.toml` in a
  clean `$TSUKU_HOME`. Verify `codex --version` succeeds and
  the bundled `rg` and `bwrap` files are present.
- **Recipe-test CI** — the existing `.github/workflows/test-recipe.yml`
  job picks up the new `codex.toml` automatically (detects
  changed recipes); the CI job validates the codex install on
  Linux containers.

## Implementation Approach

Single-pr execution. The diff bundles all CLI changes + codex
recipe + skill docs as one unit. No staged rollout.

The work order within the PR:

1. **Implementation order: bottom-up.**
   - First: `ReadChecksumFile` extension in `checksum.go`
     (D-A4) with its unit tests. Lowest-risk change; the
     existing single-line path is untouched, the new
     manifest path is purely additive.
   - Second: `decomposeDownload` signature extension and the
     composite Preflight/Decompose changes (D-A1, D-A2, D-A3,
     D-A5) with unit tests covering forwarding and mutual
     exclusion.
   - Third: validator field-registry update (D-A6) with
     a test that confirms no unknown-field warning fires.
   - Fourth: cache-invalidation hardening (D-A7) if the
     existing cache logic does not already handle it
     (DESIGN-time inspection required).
   - Fifth: codex recipe (`recipes/c/codex.toml`) — exercises
     the full path end-to-end.
   - Sixth: skill docs update (`action-reference.md`).

2. **Backward-compat verification as a gate.** Before each
   commit that touches `composites.go` or `checksum.go`,
   regenerate golden files for a representative sample of 20
   pre-existing recipes (mix of github_archive and
   download_archive, mix of platforms) and diff. Diff must be
   empty. This is the operational manifestation of D1 and D3.

3. **The S3-test false alarm.** The pre-/scope testing-infra
   audit found that the S3-backed hash tests do not exist —
   the codebase uses git-stored golden files at
   `testdata/golden/plans/` (format_version 5). The R2 plan
   in `docs/designs/current/DESIGN-r2-golden-storage.md` is
   not implemented. The PLAN does NOT need a step for "verify
   S3 tests still pass"; they don't exist to test.

## Security Considerations

- **The change adds an integrity-verification capability where
  none existed before for composite users.** Net security
  posture improves: recipe authors who opt in get
  SHA256-verified downloads, defending against in-flight
  tampering and a class of mirror-corruption attacks.
- **SHA256 is not a signature.** An attacker who controls the
  upstream release (e.g., compromises the GitHub release page)
  can rotate both the archive and the checksum file in lockstep.
  This work does NOT defend against such an attacker; signature
  verification (GPG, sigstore) is the next hardening layer
  (PRD's Out of Scope).
- **The checksum file fetch happens over HTTPS via the
  existing `PreDownloader`,** which already enforces
  `https://` and IP-validation (private/loopback/multicast
  blocked). No new network primitive is introduced.
- **Manifest parsing — defensive.** The `findManifestLine`
  helper silently skips malformed lines rather than aborting,
  preventing a denial-of-service via a single malformed line.
  A manifest with zero parseable lines returns "no match" and
  the install aborts with a clear error naming the manifest
  URL and the target asset.
- **No new auth surface.** The checksum file is a public asset
  on a public release; no credentials needed.

## Consequences

### Positive

- Recipe authors gain a one-line opt-in to upstream-pinned
  integrity verification on 108 existing composite recipes
  and every future github_archive / download_archive recipe.
- The codex recipe ships with upstream verification, not
  without it.
- The 10 manually-decomposed recipes (HashiCorp set,
  cargo-nextest, kubectl, helm, pcre2) become eligible to
  simplify back to the composite shape in follow-up PRs,
  reducing registry maintenance burden.
- Plan-time hash computation infrastructure (`PreDownloader`,
  `Step.Checksum`) sees one new caller, exercising paths that
  today only the primitive `download` triggers.

### Negative

- Adds two fields to the composite schemas — small but real
  cognitive load for recipe authors.
- The auto-detection heuristic (one line vs two+) is a
  judgment call. An upstream that ships a single-asset
  manifest (one `<hex>  <filename>` line, indistinguishable
  from per-asset .sha256 with filename) is correctly handled
  by either path. An upstream that ships a one-asset manifest
  with the asset name not matching the recipe's asset name
  would fail the manifest path while succeeding the single-
  line path — but this case is contrived.
- The `decomposeDownload` signature extension touches every
  composite's `Decompose` (currently two; future composites
  too). Mitigation: keep the parameter name unambiguous
  (`checksumURL`) and document the empty-string-means-none
  convention in the helper's godoc.

### Mitigations

- The auto-detection edge case is documented in the recipe-
  author skill docs (a one-line caveat: "if your manifest is
  one-line, the parser will treat it as per-asset shape;
  filename matching still applies").
- The compatibility-verification step in Implementation
  Approach is the operational guarantee that no existing
  recipe regresses. The PLAN issues will name this step
  explicitly.

### References

- `docs/prds/PRD-composite-action-checksum-support.md` —
  upstream requirements (R1–R12, acceptance criteria, D1–D5)
- `internal/actions/composites.go` — the file the change
  primarily lands in
- `internal/actions/download.go:169-181` — the existing
  plan-time hash computation path the new code parallels
- `internal/actions/checksum.go` — the parsing seam
  (`ReadChecksumFile`)
- `internal/recipe/types.go:894` — the field registry
- `internal/executor/plan_generator.go:321-340` —
  Step.Checksum precedence (read-time, not write-time)
- `internal/validate/predownload.go:18-189` — the
  `PreDownloader` already used for archive checksumming
- `testdata/golden/plans/embedded/cmake/v4.2.3-darwin-amd64.json`
  — sample of v5 golden-file shape
- `recipes/c/codex.toml` — the validating consumer (added in
  the same PR)
- `https://chatgpt.com/codex/install.sh` — codex install
  script, source of release shape + manifest format facts the
  DESIGN encodes inline (vendor targets, archive layout,
  `rust-v{version}` tag prefix, `codex-package_SHA256SUMS`
  manifest shape)
