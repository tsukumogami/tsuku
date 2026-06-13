---
schema: brief/v1
status: Accepted
problem: |
  Recipes that download tarballs from GitHub releases via the
  github_archive and download_archive composite actions cannot
  pin against upstream-published checksum files, because the
  composites do not forward the checksum_url field to the
  primitive download action they decompose into.
outcome: |
  A recipe author writes one composite-action step that names an
  upstream checksum (sibling release asset or full URL) and tsuku
  verifies the downloaded archive against that checksum before
  extracting — the same UX as the primitive download action.
motivating_context: |
  The codex CLI ships a multi-line codex-package_SHA256SUMS
  manifest with every release; adding a tsuku recipe for it
  forced the gap into the open. Either ship the codex recipe
  without upstream integrity (every sibling tarball recipe does
  this today) or close the gap at the composite-action layer.
---

# BRIEF: composite-action-checksum-support

## Status

Accepted

The downstream PRD owns turning these journeys into requirements,
including the field-shape decision (one field vs two) and the
checksum-file format policy (per-asset vs multi-line manifest vs
both). The DESIGN owns the seven traps the testing-infra audit
flagged.

## Problem Statement

Two of tsuku's composite actions — `github_archive` and
`download_archive` — internally decompose into a `download` step
that already accepts a `checksum_url` parameter for upstream
integrity verification. The composite layer does not forward any
checksum parameter to the decomposed step; the call sites pass
`nil` for the checksum slot. The result: recipes that use either
composite (97 `github_archive` recipes and 11 `download_archive`
recipes today) ship without upstream-pinned checksums regardless
of whether the upstream release publishes one.

Recipe authors who need upstream verification today have one
escape hatch: drop the composite and re-author the recipe as a
manual `download` + `extract` + `chmod` + `install_binaries`
sequence (the cargo-nextest pattern, used by ten recipes in the
registry, eight of which are HashiCorp tools with `SHA256SUMS`
manifests). The escape works but costs the composite's
ergonomics; the author hand-writes platform-cased steps that the
composite would otherwise handle.

The gap was not previously load-bearing because most recipes did
not need it; the codex CLI's release shape — a Rust binary with
a multi-line `codex-package_SHA256SUMS` manifest covering every
platform asset — both demands upstream verification (the binary
ships a sandbox helper) and forces the choice between adding
codex via the existing manual-decomposition workaround or
closing the gap at the composite layer where every github_archive
recipe benefits.

## User Outcome

Once this lands, a recipe author writes one composite-action step
that names an upstream checksum source — either a sibling release
asset filename (the ergonomic default for github releases) or a
full URL (the escape hatch for off-release checksums) — and tsuku
verifies the downloaded archive's SHA256 against the upstream
value before extraction. The verification path matches the
primitive `download` action's existing behavior; the
composite-author experience matches the existing composite-author
experience minus the manual decomposition.

Authors of the 13 recipes that today work around the gap (the
HashiCorp manifest-using set plus cargo-nextest, kubectl, helm,
pcre2 with per-asset `.sha256`) can collapse their manual
decomposition back into the composite shape in follow-up PRs.
Existing recipes that pass no checksum field continue installing
identically — backward compatibility is non-negotiable and the
new fields are purely additive.

End users installing tools shipped through these composites get
upstream-pinned integrity verification whenever the recipe author
opts in. Installs fail fast on mismatch rather than silently
extracting a tampered or corrupted archive.

## User Journeys

### Recipe author adopting checksum on a new tool

A recipe author writes the new `codex.toml` recipe and adds
`checksum_asset = "codex-package_SHA256SUMS"` to the
`github_archive` step. They run `tsuku validate path/to/codex.toml`
locally; the validator accepts the new field. They run
`tsuku install --recipe-file codex.toml`; the install downloads
the codex tarball plus the SHA256SUMS file, picks the matching
line for the resolved asset, verifies the SHA256, then extracts.
A bad upstream checksum aborts before extraction with a clear
error naming the expected and actual hashes.

### Recipe author hardening an existing recipe

A recipe author opens `recipes/b/bun.toml` and adds
`checksum_url = "https://github.com/oven-sh/bun/releases/download/bun-v{version}/SHASUMS256.txt"`
to the existing `github_archive` step. They run `tsuku validate`;
the file passes. They open a PR; the CI recipe-test job confirms
the install still works on every supported platform. The recipe
ships with upstream verification with no other change required.

### Recipe author simplifying a manually-decomposed recipe

A recipe author opens `recipes/c/cargo-nextest.toml`, which today
decomposes manually because its only entry point needs upstream
checksum support. They replace the four-step manual decomposition
with a single `github_archive` step using `checksum_url`. The
recipe shrinks from ~40 lines to ~10; the install behavior is
identical to the pre-collapse version.

### End user installing a verified tool

An end user runs `tsuku install codex`. The CLI resolves the
recipe, fetches the latest version, downloads the
`codex-package-x86_64-unknown-linux-musl.tar.gz` archive plus
`codex-package_SHA256SUMS`, parses the manifest, finds the line
matching the asset name, verifies the SHA256, and extracts the
tree into `$TSUKU_HOME/tools/codex-<version>/`. On mismatch, the
install aborts before extraction and the user sees an error
naming the asset, the expected hash, and the actual hash.

## Scope Boundary

**In:**

- Composite-level `checksum_url` (full URL with placeholders)
  and `checksum_asset` (sibling release filename) field
  forwarding on `github_archive` and `download_archive`
  composite actions in `internal/actions/composites.go`.
- Multi-line `SHA256SUMS` manifest format parsing in
  `internal/actions/checksum.go`, if the existing
  `ReadChecksumFile` function does not already handle it (the
  DESIGN doc owns the discovery and the implementation choice).
- New `codex` recipe at `recipes/c/codex.toml` exercising the
  new field (`checksum_asset`), the harder of the two checksum-
  file shapes (multi-line manifest), and `install_mode = "directory"`
  so the bundled `rg` and `bwrap` co-locate with the binary.
- Updated `tsuku-recipe-author` skill's `action-reference.md` to
  document the new fields.
- Validator field-registry updates in `internal/recipe/types.go`
  so unknown-field warnings do not fire on the new field.
- Tests: composite forwarding (unit), multi-line manifest parsing
  (unit), and a real install of the codex recipe (manual or CI
  fixture).

**Out:**

- Backporting existing recipes to use the new field — deferred
  to follow-up PRs. The 10 manually-decomposed recipes and the
  top-10 backport candidates from the recipe-inventory research
  ship in their current form; the inventory becomes a
  follow-up's input, not this PR's scope.
- Changes to the primitive `download` action's checksum API — it
  already supports `checksum_url` and the implementation
  forwards into the existing surface, not a new one.
- Signature-based verification (GPG, sigstore) — a separate
  hardening pass tracked under its own design.
- R2 golden-file storage migration — the testing-infra audit
  surfaced this as planned-but-not-implemented; the checksum
  work uses git-stored golden files at `testdata/golden/plans/`
  per current convention.
- Auto-deriving checksum URLs from `asset_pattern` (e.g.,
  appending `.sha256` automatically) — the explicit-field shape
  is what the PRD will commit to; auto-derivation can become a
  follow-on ergonomic if demand surfaces.

## References

- `wip/research/scope_phase0_codex-install-script.md` — codex
  release shape, manifest format, vendor-target mapping.
- `wip/research/scope_phase0_testing-infra-audit.md` —
  plan-time hash flow, golden-file format, S3-trap false-alarm
  finding, seven named DESIGN traps.
- `wip/research/scope_phase0_archive-recipe-inventory.md` — 97
  github_archive + 11 download_archive recipes, 13 already-
  checksummed via manual decomposition, top-10 backport
  candidates.
- `internal/actions/composites.go` — the file the field
  forwarding lands in.
- `internal/actions/download.go` — the existing checksum
  surface the forwarding plugs into.
- `internal/actions/checksum.go` — the parsing surface that may
  need multi-line manifest support.
