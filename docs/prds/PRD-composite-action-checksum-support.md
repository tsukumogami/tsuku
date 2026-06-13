---
status: Accepted
problem: |
  github_archive and download_archive composite actions cannot
  forward upstream checksum verification to their decomposed
  download steps. Recipe authors who need integrity pinning have
  to drop the composite and re-author the recipe as a manual
  download+extract+chmod+install_binaries sequence, losing the
  composite's ergonomics. 97 github_archive recipes and 11
  download_archive recipes ship today without upstream
  checksums regardless of whether the upstream publishes one.
goals: |
  Add checksum_url and checksum_asset field forwarding on the
  two composite actions so a recipe author writes one composite
  step with one extra field and gets upstream-pinned SHA256
  verification. Support both per-asset .sha256 (existing) and
  multi-line SHA256SUMS manifest (new) checksum file formats.
  Preserve backward compatibility — existing recipes without the
  new fields install identically. Validate the implementation
  with a new codex recipe that exercises the harder manifest
  format.
upstream: docs/briefs/BRIEF-composite-action-checksum-support.md
motivating_context: |
  The codex CLI ships a multi-line codex-package_SHA256SUMS
  manifest. Adding it to the registry surfaced the gap; the
  composite-action layer is where every github_archive recipe
  benefits.
---

# PRD: composite-action-checksum-support

## Status

Accepted

## Problem Statement

tsuku's `github_archive` and `download_archive` composite actions
internally decompose into a `download` step that already accepts a
`checksum_url` parameter for upstream integrity verification. The
composite layer does not forward any checksum parameter to the
decomposed step — both Decompose call sites in
`internal/actions/composites.go` pass `nil` for the checksum slot.
Recipes using either composite (97 + 11 in the current registry)
ship without upstream-pinned checksums, regardless of whether the
upstream release publishes one.

Recipe authors who need verification today escape the composite
and re-author the recipe as a manual `download` + `extract` +
`chmod` + `install_binaries` sequence (the cargo-nextest pattern,
used by 10 recipes — 8 of which are HashiCorp tools with
SHA256SUMS manifests). The escape works but discards the
composite's platform-cased ergonomics; the author hand-writes
steps the composite would otherwise handle.

The codex CLI release surfaced the cost: codex ships a
`codex-package_SHA256SUMS` manifest listing every platform asset.
Adding codex to the registry forced the choice between (a)
shipping codex via the manual-decomposition workaround that
every other tool with a manifest already uses, or (b) closing
the gap at the composite layer where every github_archive
recipe benefits.

## Goals

- **Field-level parity with the primitive.** Recipe authors can
  set `checksum_url` (the same field the primitive `download`
  action accepts) on `github_archive` and `download_archive`
  steps and get the same verification behavior.
- **Ergonomic default for GitHub releases.** Recipe authors can
  set `checksum_asset` (the bare filename of a sibling release
  asset) on `github_archive`, avoiding URL boilerplate.
- **Multi-line manifest support.** The verification path
  recognizes both per-asset single-line checksum files (existing
  shape) and multi-line `SHA256SUMS` manifests (new), matching
  the per-line filename against the resolved asset name.
- **Backward compatibility — non-negotiable.** Existing
  github_archive and download_archive recipes that pass no
  checksum field install identically (same plan output, same
  golden files, no behavioral regression).
- **Validated end-to-end.** A new codex recipe — using
  `checksum_asset` with the multi-line manifest — installs
  successfully and fails fast on tampered upstream content.

## User Stories

- **As a recipe author**, I want to add upstream checksum
  verification to a new tool by setting one extra field on the
  composite step, so I do not have to re-author the recipe as a
  manual decomposition just to get integrity pinning.
- **As a recipe author**, I want to harden an existing
  github_archive recipe with one-line edits, so backporting
  checksum support to popular recipes (bun, gh, ripgrep,
  starship, cargo-deny) is a low-friction follow-up.
- **As a recipe author of a manually-decomposed recipe** (e.g.,
  cargo-nextest, kubectl, helm, the HashiCorp set), I want a
  path to simplify my recipe back into the composite shape once
  upstream-checksum support is in.
- **As an end user**, I want installs to fail fast with a clear
  error if the downloaded archive does not match the upstream
  checksum, so I learn about a corrupted or tampered download
  before extraction.
- **As a downstream maintainer of the registry**, I want the
  validator to accept the new fields without unknown-field
  warnings, so authoring tools and CI stay green.

## Requirements

### Functional

- **R1.** `github_archive` accepts an optional `checksum_url`
  string parameter. When present, the composite's Decompose
  forwards it (with placeholders expanded) to the decomposed
  `download` step's existing `checksum_url` field.
- **R2.** `github_archive` accepts an optional `checksum_asset`
  string parameter — the bare filename of a sibling asset on
  the same GitHub release. The composite's Decompose resolves
  it to a sibling URL on the release-download URL and forwards
  it to the decomposed `download` step's `checksum_url` field.
  `checksum_asset` and `checksum_url` are mutually exclusive on
  a single step.
- **R3.** `download_archive` accepts an optional `checksum_url`
  string parameter. When present, the composite's Decompose
  forwards it (with placeholders expanded) to the decomposed
  `download` step. (`download_archive` does not get
  `checksum_asset` — its base URL is fully recipe-supplied, so
  the field would have no anchor to resolve a sibling against.)
- **R4.** `checksum_asset` and `checksum_url` placeholder
  expansion uses the same template variables as `asset_pattern`
  / `url`: `{version}`, `{os}`, `{arch}`. OS and arch mappings
  apply identically. The recipe stays versionless; placeholder
  expansion at install time produces a per-version URL that
  resolves against the same release tag as the asset itself.
  For `checksum_asset`, the sibling URL is constructed against
  the resolved version tag (not a static one), so a versionless
  recipe fetches a different checksum file for each version
  install — the recipe is never "pinned" against any specific
  version's hash file.
- **R5.** The checksum-verification path fetches the resolved
  per-version checksum file at install time and recognizes two
  file shapes:
  - **Per-asset single-line**, format `<hex>[  <filename>]` —
    one line, optional filename. Existing behavior.
  - **Multi-line manifest**, format `<hex>  <filename>\n...` —
    multiple lines, each line a `<hex>  <filename>` pair. The
    verifier picks the line whose filename matches the resolved
    asset name. New behavior.
- **R6.** When a checksum source is provided, the install
  download path fetches the checksum file, parses it, verifies
  the downloaded archive's SHA256 against the resolved
  expected hash, and aborts before extraction on mismatch with
  an error naming the asset, expected hash, and actual hash.
- **R7.** The `shirabe validate` field registry recognizes
  `checksum_url` and `checksum_asset` on both composites; no
  unknown-field warning fires when either is present.
- **R8.** The codex recipe at `recipes/c/codex.toml` is added
  in the same PR. It uses `github_archive` with
  `checksum_asset = "codex-package_SHA256SUMS"`,
  `install_mode = "directory"`, and the codex-specific
  `tag_prefix = "rust-v"`. It validates clean and installs
  end-to-end.

### Non-functional

- **R9.** **Backward compatibility.** Every existing recipe that
  does NOT specify `checksum_url` or `checksum_asset` produces
  the same plan output (identical golden file content) as
  before this change. The new fields are additive; their
  absence is the default behavior.
- **R10.** **Plan-generation parity for nil-Downloader callers.**
  Callers that generate plans without a `Downloader` (e.g.,
  `--dry-run`, validation contexts) continue to work; the
  checksum forwarding gracefully no-ops when a Downloader is
  not available, just as the primitive `download` action does
  today.
- **R11.** **Error clarity.** The mismatch error includes the
  asset filename, the URL of the checksum source, the expected
  hash, and the actual hash. The manifest-parse error (when no
  line matches the asset) names the asset filename and the
  manifest URL.
- **R12.** **Documentation.** The `tsuku-recipe-author` skill's
  `action-reference.md` documents the two new fields with one
  example per shape (per-asset .sha256 and multi-line
  SHA256SUMS).

## Acceptance Criteria

- [ ] `github_archive` Preflight accepts `checksum_url` and
      `checksum_asset` independently; Preflight rejects both
      being set on the same step with a clear error.
- [ ] `github_archive` Decompose, when `checksum_url` is set,
      forwards the placeholder-expanded URL to the decomposed
      `download` step's `checksum_url` field.
- [ ] `github_archive` Decompose, when `checksum_asset` is set,
      constructs the sibling URL
      (`https://github.com/<repo>/releases/download/<tag>/<asset>`)
      and forwards it to the decomposed `download` step.
- [ ] `download_archive` Preflight accepts `checksum_url`;
      Decompose forwards it to the decomposed `download` step.
- [ ] The verification path parses both single-line and
      multi-line checksum file shapes correctly. Filename
      matching is exact (`<hex>  <filename>` lines where
      `<filename>` equals the resolved asset name).
- [ ] On checksum mismatch, install aborts before extraction
      with an error naming the asset, expected hash, and actual
      hash.
- [ ] Every existing tarball recipe in `recipes/` produces an
      identical golden file before and after the change
      (verified by running `scripts/regenerate-golden.sh` on a
      representative sample and diffing).
- [ ] The `shirabe validate` field registry accepts
      `checksum_url` and `checksum_asset` on both composites
      without unknown-field warnings.
- [ ] `recipes/c/codex.toml` exists, validates clean, and
      `tsuku install --recipe-file recipes/c/codex.toml`
      completes successfully on at least one platform (Linux
      x86_64 in CI; macOS optional).
- [ ] After install, `codex --version` succeeds and the bundled
      `rg` and `bwrap` (Linux) helpers are present in the
      install tree.
- [ ] The `tsuku-recipe-author` skill's `action-reference.md`
      includes both new fields with examples.
- [ ] Unit tests cover composite forwarding (both fields, both
      composites, mutual-exclusion rejection) and multi-line
      manifest parsing (matching line found, no matching line,
      malformed line).

## Out of Scope

- **Backporting existing recipes** to use the new fields in this
  PR. The 10 manually-decomposed recipes and the top-10 backport
  candidates from the recipe-inventory research ship in their
  current form. Each backport is its own follow-up PR with its
  own recipe-test job; bundling them here would balloon the
  diff and complicate the regression-verification sample.
- **Signature-based verification** (GPG, sigstore, in-toto).
  This work bounds checksum (SHA256) integrity only. Signature
  hardening is a separate design.
- **Auto-deriving checksum sources** from `asset_pattern` (e.g.,
  appending `.sha256` automatically when no field is set).
  Explicit fields keep behavior predictable; auto-derivation
  can become a follow-on ergonomic if demand surfaces.
- **R2 golden-file storage migration**
  (DESIGN-r2-golden-storage.md, Milestone 45). The work proceeds
  against the existing git-committed `testdata/golden/plans/`
  fixtures.
- **Algorithm choice beyond SHA256.** `checksum_algo` remains
  whatever the primitive `download` action supports today (the
  field already exists). Extending to SHA512 or stronger digests
  is out of scope for this PR.
- **GitHub-API-derived asset existence preflight checks.** The
  validator does NOT do network calls; missing checksum files
  surface at install time, not at validation time.

## Decisions and Trade-offs

The BRIEF's downstream-PRD questions land here.

### D1 — Field shape: two fields, not one

The composite gets `checksum_url` and `checksum_asset` rather
than only `checksum_url`. Rationale: `checksum_asset` is the
ergonomic default for GitHub releases (sibling file on the
same release; the URL is fully implied by `asset_pattern` and
the release tag), and `checksum_url` is the escape hatch for
off-release checksums (third-party hosting, organization-wide
mirrors). Forcing every author to type the full URL for the
common case would be hostile; supporting only the bare asset
name would close the escape hatch.

Alternatives considered:

- **One field, `checksum_url` only.** Simpler implementation;
  worse author UX for the common case. Rejected — the registry
  inventory shows GitHub-release-sibling is the dominant
  shape (covers all 13 currently-checksummed recipes), so
  saving the URL boilerplate has real volume.
- **One field, `checksum_asset` only.** No escape hatch for
  off-release checksums. Rejected — at least one recipe
  category (HashiCorp tools host their manifests on
  releases.hashicorp.com, not on the GitHub release) needs the
  full-URL shape.

### D2 — Mutual exclusion on a single step

Setting both `checksum_url` and `checksum_asset` on the same
`github_archive` step is a Preflight error. Rationale: it's
ambiguous which the author intended, and the precedence
question has no obviously-correct answer. Make the author
choose one explicitly.

### D3 — `download_archive` gets `checksum_url` only

The `checksum_asset` field is `github_archive`-only;
`download_archive` does not get it. Rationale: `checksum_asset`
resolves to a sibling URL on the GitHub release-download URL,
which `github_archive` constructs from `repo + tag`. The
`download_archive` action has no equivalent anchor — its
`url` is fully recipe-supplied with no known siblings — so
`checksum_asset` would have nothing to resolve against.

### D4 — Multi-line manifest format is in-scope; auto-detection deferred to DESIGN

The PRD commits to supporting multi-line `SHA256SUMS` manifests
(R5). The mechanism — auto-detect single-vs-multi-line from
file content vs require an explicit `checksum_format` field —
is a DESIGN decision because the trade-off is
implementation-level (parser complexity, error mode on
malformed files). The PRD's requirement is "both shapes work";
the DESIGN's choice is "how".

### D5 — Backport scope: zero in this PR

This PR ships the CLI change + codex recipe + skill docs.
Zero existing recipes are modified. Rationale:

- The validator inventory confirms purely-additive change has
  no regression risk on existing recipes.
- Backports change install behavior for popular tools (bun,
  gh, ripgrep). Each backport deserves its own recipe-test
  job and PR-level review.
- Bundling 10+ backports here would dilute the CLI-change
  reviewer attention and slow time-to-merge.

Follow-up backport PRs become normal recipe-maintenance work;
the inventory artifact at
`wip/research/scope_phase0_archive-recipe-inventory.md` feeds
the prioritization (rank by criticality + checksum
availability).

## Known Limitations

- **No upstream signature verification.** SHA256 protects
  against accidental corruption and a class of MITM attacks
  but not against an attacker who controls the upstream GitHub
  release itself. Signature-based verification is the next
  hardening layer (out of scope for this PR).
- **Manifest format detection.** The DESIGN must pick between
  auto-detect (one fewer recipe field; ambiguous on malformed
  files) and explicit `checksum_format` (one more recipe
  field; deterministic parsing). The PRD bounds the choice to
  these two; either is acceptable from the requirements side.
- **First-version conservatism on filename matching.** The
  PRD's R5 requires exact filename match in the manifest. Some
  upstream manifests prefix the filename with `*` (binary-mode
  marker from `sha256sum -b`) or path components. The DESIGN
  may choose to strip these prefixes for robustness; the PRD
  permits but does not require this normalization.
