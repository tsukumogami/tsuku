---
schema: plan/v1
status: Draft
execution_mode: single-pr
upstream: docs/designs/DESIGN-composite-action-checksum-support.md
milestone: "Composite Action Checksum Support"
issue_count: 9
---

# PLAN: composite-action-checksum-support

## Status

Draft

## Scope Summary

Add `checksum_url` and `checksum_asset` parameter forwarding to the
`github_archive` and `download_archive` composite actions, with
multi-line `SHA256SUMS` manifest auto-detection, validator
field-registry updates, cache-invalidation hardening, and a new
`codex` recipe as the validating consumer. The DESIGN selects Option A
(field forwarding at the composite layer), keeping the primitive
`download` action GitHub-agnostic and the `download_file` step shape
byte-stable so existing v5 golden files do not regenerate.

## Decomposition Strategy

**Horizontal, bottom-up.** The DESIGN's seven decisions (D-A1
through D-A8) split into nine atomic issues: one per discrete
code surface (manifest parser, signature extension, asset
resolution, Preflight rules, field registry, cache hardening,
recipe, skill docs) plus one explicit backward-compatibility
verification step.

Bottom-up ordering puts the parser change first (lowest risk,
purely additive, no callers), then the composite plumbing that
calls it, then the Preflight rules that validate the new fields,
then the validator registry that exposes them, then the codex
recipe that exercises the full path, then the docs and the
regression-verification gate. The validator-registry issue can
land in parallel since it has no dependency on the code changes.

Two issues are independent and may start in parallel (Issue 1 +
Issue 5). Everything else chains.

## Issue Outlines

### Issue 1: feat(checksum): multi-line manifest auto-detection in ReadChecksumFile

**Complexity**: testable

**Goal**: Extend `internal/actions/checksum.go`'s `ReadChecksumFile`
to detect single-line vs multi-line checksum file shapes and
return the SHA256 hex matching a target asset name. Single-line
behavior is preserved verbatim; multi-line parsing matches by
exact filename after stripping a leading `*` (binary-mode marker
from `sha256sum -b`) and reducing path-prefixed filenames to the
final path component. Malformed lines are skipped silently;
a manifest with zero matching lines returns an error naming the
target asset.

**Acceptance Criteria**:
- [ ] `ReadChecksumFile` signature gains a `targetAsset string`
      parameter and existing callers are updated to pass the
      `dest` value they already have
- [ ] One non-blank line in the file routes to the existing
      single-line parser unchanged (behavior byte-identical
      against existing fixtures)
- [ ] Two or more non-blank lines route to the multi-line
      manifest parser
- [ ] Manifest filename matching is exact after stripping a
      leading `*` and reducing to the basename
- [ ] Malformed lines (insufficient whitespace, hex column
      length not 64) are skipped without aborting; the function
      continues to the next line
- [ ] Manifest with zero matching lines returns an error
      `no matching line for asset <name> in manifest`
- [ ] Unit tests cover: single-line happy path, manifest happy
      path, manifest with `*` prefix, manifest with path-
      prefixed filename, manifest with no matching line,
      manifest with malformed lines, single-line / manifest
      boundary (1 vs 2 lines)

**Dependencies**: None

**Files**: `internal/actions/checksum.go`,
`internal/actions/checksum_test.go`

---

### Issue 2: feat(composites): decomposeDownload checksumURL parameter + Decompose forwarding

**Complexity**: testable

**Goal**: Extend `decomposeDownload` in
`internal/actions/composites.go` to accept a `checksumURL string`
parameter and forward it to the decomposed `download` step's
existing `checksum_url` field. Update both `github_archive.Decompose`
and `download_archive.Decompose` call sites to extract
`checksum_url` from their params, expand placeholders
(`{version}`, `{os}`, `{arch}` plus OS/arch mappings), and pass
the resolved value. Existing recipes pass no field; the empty
string is forwarded and the decomposed step behaves identically.

**Acceptance Criteria**:
- [ ] `decomposeDownload` signature gains `checksumURL string`
      as the last parameter
- [ ] Empty-string `checksumURL` produces the existing decomposed
      step shape byte-identical to pre-change behavior
- [ ] Non-empty `checksumURL` populates the decomposed
      `download` step's `checksum_url` field
- [ ] `github_archive.Decompose` extracts `checksum_url` from
      `params`, expands placeholders + mappings, passes through
- [ ] `download_archive.Decompose` does the same for its
      `checksum_url` field
- [ ] Unit tests cover: no-field control case (existing
      behavior), checksum_url with placeholder expansion,
      checksum_url with OS/arch mapping interaction
- [ ] No existing test fails

**Dependencies**: Issue 1

**Files**: `internal/actions/composites.go`,
`internal/actions/composites_decompose_test.go`

---

### Issue 3: feat(composites): checksum_asset sibling-URL resolution in github_archive

**Complexity**: testable

**Goal**: Add `checksum_asset` field handling to
`github_archive.Decompose`. The composite constructs the sibling
URL as
`https://github.com/<repo>/releases/download/<versionTag>/<asset>`,
expanding placeholders in the asset filename, and forwards the
constructed URL through `decomposeDownload`'s `checksumURL`
parameter. `download_archive` does not get this field — the
DESIGN limits `checksum_asset` to `github_archive` per
Decision D-A2.

**Acceptance Criteria**:
- [ ] `github_archive.Decompose` extracts `checksum_asset` from
      `params` when present
- [ ] Sibling URL is constructed correctly:
      `https://github.com/<repo>/releases/download/<tag>/<asset>`
- [ ] Asset filename placeholders are expanded (typically not
      needed for the bare asset name, but `{version}` etc.
      remain available)
- [ ] When `checksum_asset` is set, the resolved URL is passed
      via `decomposeDownload`'s `checksumURL` parameter
- [ ] Unit test: github_archive + checksum_asset produces a
      decomposed step with checksum_url pointing at the
      sibling URL
- [ ] Unit test: github_archive without checksum_asset (and
      without checksum_url) produces an identical step shape
      to pre-change behavior

**Dependencies**: Issue 2

**Files**: `internal/actions/composites.go`,
`internal/actions/composites_decompose_test.go`

---

### Issue 4: feat(composites): Preflight mutual-exclusion, scope guards, and static-checksum warning

**Complexity**: simple

**Goal**: Add four Preflight checks per DESIGN Decision D-A2:
(a) reject when both `checksum_url` and `checksum_asset` are
set on a single `github_archive` step; (b) reject
`checksum_asset` on `github_archive` when `asset_pattern`
contains wildcards (sibling URL cannot be constructed before
asset resolution); (c) reject `checksum_asset` on
`download_archive` entirely (the field is github_archive-only);
(d) WARN when `checksum_url` has no `{version}` placeholder
while `asset_pattern`/`url` is version-templated — the static-
checksum-with-versioned-asset combination would surface every
version bump as a hash mismatch.

**Acceptance Criteria**:
- [ ] `github_archive.Preflight` errors with
      `"checksum_url and checksum_asset are mutually exclusive on a single step; use one"`
      when both are set
- [ ] `github_archive.Preflight` errors with
      `"checksum_asset is not supported with wildcard asset_pattern; use checksum_url instead"`
      when both `checksum_asset` and wildcards in
      `asset_pattern` are present
- [ ] `download_archive.Preflight` errors with
      `"checksum_asset is github_archive-only; use checksum_url on download_archive"`
      when `checksum_asset` is set
- [ ] Both composites warn (not error) with
      `"checksum_url has no {version} placeholder but asset_pattern is version-templated; each install will fetch the same checksum file regardless of version — likely a recipe authoring mistake"`
      when `checksum_url` lacks `{version}` while `asset_pattern`
      (`github_archive`) or `url` (`download_archive`) contains
      `{version}`
- [ ] Unit tests cover all four checks and the happy paths
      they protect (only checksum_url with {version}; only
      checksum_asset; both absent; warning suppressed when both
      sides are versionless)

**Dependencies**: Issue 2, Issue 3

**Files**: `internal/actions/composites.go`,
`internal/actions/composites_test.go`

---

### Issue 5: feat(validate): field registry entries for checksum_url and checksum_asset

**Complexity**: simple

**Goal**: Register `checksum_url` and `checksum_asset` as
recognized fields on `github_archive` (both fields) and
`download_archive` (only `checksum_url`) in
`internal/recipe/types.go`'s field registry. Without
registration, the validator fires unknown-field notices when
recipes use the new fields.

**Acceptance Criteria**:
- [ ] `internal/recipe/types.go` field-registry map gains
      `checksum_url` for both composites
- [ ] Same map gains `checksum_asset` for `github_archive`
      only
- [ ] `internal/recipe/types_test.go` includes a test asserting
      no unknown-field warning fires when a recipe uses either
      new field on the appropriate composite
- [ ] Existing tests still pass (no registry-entry conflicts)

**Dependencies**: None

**Files**: `internal/recipe/types.go`,
`internal/recipe/types_test.go`

---

### Issue 6: feat(cache): cache-invalidation safety for upstream-pinned checksums

**Complexity**: testable

**Goal**: When a recipe uses `checksum_url` or `checksum_asset`,
the install-time download cache validates the cached blob's
checksum against the freshly-fetched upstream value before
serving the cache hit. Mismatch invalidates the entry and
triggers a fresh download. This closes the D7 surface
(rotated upstream asset, same URL, new checksum). When no
upstream checksum source is set, the cache continues to behave
as today.

**Acceptance Criteria**:
- [ ] `internal/actions/download_cache.go` cache lookup,
      when the requesting step carries an upstream-pinned
      checksum, re-fetches the upstream checksum and compares
      against the cached blob's hash
- [ ] Mismatch invalidates the cache entry and falls through
      to a fresh download
- [ ] No-checksum requests use the existing cache lookup
      unchanged (no extra network round-trip)
- [ ] Unit test: rotated upstream (same URL, new expected
      checksum) does NOT serve the stale cached blob
- [ ] Unit test: matching upstream checksum (cache hit valid)
      serves the cache as before
- [ ] Unit test: no-checksum request behavior unchanged

**Dependencies**: Issue 2

**Files**: `internal/actions/download_cache.go`,
`internal/actions/download_cache_test.go`

---

### Issue 7: feat(recipes): add codex recipe with checksum_asset (validating consumer)

**Complexity**: testable

**Goal**: Add `recipes/c/codex.toml` using `github_archive` with
`checksum_asset = "codex-package_SHA256SUMS"`,
`tag_prefix = "rust-v"`, `install_mode = "directory"`, and the
codex-specific vendor-target mapping
(`os_mapping = { darwin = "apple-darwin", linux = "unknown-linux-musl" }`,
`arch_mapping = { amd64 = "x86_64", arm64 = "aarch64" }`). The
recipe validates clean, installs end-to-end on Linux x86_64,
and the bundled `rg` and `bwrap` helpers co-locate with the
binary.

**Acceptance Criteria**:
- [ ] `recipes/c/codex.toml` exists and `shirabe validate
      recipes/c/codex.toml` exits clean (outcome `clean`,
      errors 0)
- [ ] `tsuku validate recipes/c/codex.toml` exits 0
- [ ] `tsuku install --recipe-file recipes/c/codex.toml`
      completes successfully on Linux x86_64 in a clean
      `$TSUKU_HOME`
- [ ] `codex --version` exits 0 after install
- [ ] On Linux installs: `$TSUKU_HOME/tools/codex-<version>/codex-resources/bwrap`
      is present and executable
- [ ] `$TSUKU_HOME/tools/codex-<version>/codex-path/rg` is
      present and executable on all platforms
- [ ] Recipe is added with no other test fixtures (the
      install itself is the test)

**Dependencies**: Issue 2, Issue 3, Issue 4, Issue 5

**Files**: `recipes/c/codex.toml`

---

### Issue 8: docs(skill): document checksum_url and checksum_asset in action-reference

**Complexity**: simple

**Goal**: Update
`plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md`
to document the two new fields. Include one example per
checksum file shape: a per-asset `.sha256` example (using a
representative recipe pattern like cargo-deny's) and a
multi-line `SHA256SUMS` manifest example (using the codex
recipe). Note the mutual-exclusion rule and the
`checksum_asset` GitHub-only constraint.

**Acceptance Criteria**:
- [ ] `action-reference.md` documents `checksum_url` on both
      composites with a per-asset `.sha256` example
- [ ] `action-reference.md` documents `checksum_asset` on
      `github_archive` only with a multi-line manifest example
      pointing at the codex recipe
- [ ] The mutual-exclusion rule and the `checksum_asset`
      github_archive-only constraint are stated explicitly
- [ ] Documentation links to the PRD and DESIGN for further
      reading

**Dependencies**: Issue 7

**Files**: `plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md`

---

### Issue 9: test(golden): backward-compatibility verification across representative recipes

**Complexity**: testable

**Goal**: Operationalize DESIGN Decision D1 (backward
compatibility, non-negotiable). Run
`scripts/regenerate-golden.sh` on a representative sample of 20
existing recipes (mix of `github_archive` and
`download_archive`, mix of platforms — include `bun`, `gh`,
`cargo-deny`, `delta`, `fd`, `bat`, `ripgrep`, `starship`,
`btop`, `fzf`, `helix`, `jj`, `k9s`, `gum`, `cmake`,
`golang`, `docker`, `gradle`, `maven`, `terraform`) and confirm
every regenerated golden file is byte-identical to the
committed version. Any diff signals a regression and must be
fixed before merging.

**Acceptance Criteria**:
- [ ] Run `scripts/regenerate-golden.sh` against the 20-recipe
      sample (script invocation captured in PR body)
- [ ] `git diff --stat testdata/golden/plans/` shows zero
      changes after the regen
- [ ] The list of 20 recipes is documented in the PR body
      under "Backward-compat verification" so reviewers can
      reproduce
- [ ] If any recipe's golden file changes: STOP and treat as
      an Issue 2 or Issue 3 regression; do not merge until the
      DESIGN's byte-stable shape is restored

**Dependencies**: Issue 2, Issue 3

**Files**: `testdata/golden/plans/` (verification, not
modification), PR body

---

## Dependency Graph

_Empty in single-pr mode per the PLAN format spec — per-issue
dependencies are named in the Issue Outlines above. The chain
visualization on the same data: I1 → I2 → {I3, I6, I9}; I3 →
I4; I5 unblocks I7 alongside the I2/I3/I4 chain; I7 → I8._

## Implementation Sequence

**Critical path**: I1 → I2 → I3 → I4 → I7 (5 issues)

**Recommended order on the single branch**:

1. **I1 — multi-line manifest parser.** Lowest-risk change;
   purely additive in `checksum.go`. Lands first so subsequent
   composite work can call it.
2. **I5 — validator field registry.** Independent of code in
   I2–I4; can land in parallel with I1. Doing it early means
   subsequent commits do not fire spurious unknown-field
   warnings.
3. **I2 — decomposeDownload + Decompose forwarding.** The
   composite plumbing on top of I1's parser.
4. **I3 — checksum_asset sibling-URL resolution.** Adds the
   ergonomic field to github_archive.
5. **I4 — Preflight mutual-exclusion + scope guards.** The
   validation surface on top of I2 + I3.
6. **I6 — cache-invalidation safety.** Touches a separate code
   surface; can interleave with I3/I4 if convenient.
7. **I7 — codex recipe.** The end-to-end validating consumer.
   Requires I2, I3, I4, I5 in place.
8. **I9 — backward-compat verification.** Run after I2 and I3
   land; gate the PR merge on a clean diff.
9. **I8 — action-reference docs.** Document the fields with
   the codex recipe as the multi-line example.

**Parallelization**: I1 and I5 can start the branch in
parallel. I6 is independent of I3/I4 and can interleave.
Everything else chains.
