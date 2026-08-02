# Explore Scope: extract-symlink-escape

## Visibility

Public

## Scope Type

Tactical

## Core Question

`extract`'s containment checks are lexical, so an archive can stage a symlink in one
entry and traverse it in a later entry to write outside the destination directory
(tsukumogami/tsuku#2473, verified reproduction). The question is what the containment
guarantee should actually be and how to enforce it: resolve-then-check on each entry,
extract through directory handles, or something narrower. The hard part is drawing the
line between a symlink that legitimately exits and re-enters the destination (which
tsukumogami/tsuku#2275 wants allowed) and one that escapes.

## Context

- The bug is pre-existing and independent of current work; found while reviewing
  PR #2467.
- PR #2467 landed `internal/actions/install_program_files.go`, which deliberately does
  not trust extraction for containment: it resolves with `filepath.EvalSymlinks`, checks
  against the resolved tool directory, opens `O_RDONLY|O_NOFOLLOW` and type-checks the
  handle, and writes via `O_CREATE|O_EXCL|O_NOFOLLOW` + rename. That is in-tree
  precedent for the technique.
- #2275 is open and has the opposite polarity. Whether its bottles extract today is
  unknown and must be established empirically, because #2473's acceptance criterion
  ("#2275's relative-symlink bottles still extract") is written as if they do.
- Input is untrusted: recipes come from a registry, archives from upstream release
  pages. Impact is arbitrary file write into `$TSUKU_HOME/bin` or
  `$TSUKU_HOME/share/shell.d`.
- Time-sensitive: the reproduction is public in the issue.

## In Scope

- `internal/actions/extract.go` -- the entry loop and both containment helpers.
- Any other archive extraction path in the repo with the same shape (zip, other tar
  variants, nested extraction).
- A regression test that drives a real archive through the real extractor.
- The `SECURITY:` comments, which currently claim a guarantee the code does not provide.
- `plugins/tsuku-recipes/` skill updates if the set of accepted archives changes.

## Out of Scope

- Fixing tsukumogami/tsuku#2275 (relative bottles). Do not make it harder to fix; do
  not fix it here.
- The other open siblings from the same review (#2460, #2461, #2463, #2468-#2472,
  #2474-#2477).
- Reworking shell.d lifecycle, dependency-recording, or nvm data-root work.

## Research Leads

1. **How does extraction actually work today, end to end?** (lead-current-extractor)
   Entry loop, where each helper is called, how symlinks/hardlinks/dirs/regular files
   are created, what other extraction paths exist in the repo, and every caller of the
   two validation helpers. Without this the fix lands in the wrong place or misses a
   sibling code path.

2. **What does the #2467 precedent do, and how much of it generalizes?**
   (lead-precedent-2467)
   Read `install_program_files.go` closely. Identify the reusable containment primitive
   and whether other in-tree code already does resolve-then-check that a shared helper
   should absorb.

3. **Do #2275's relative-symlink bottles extract on current main?**
   (lead-bottle-behavior-2275)
   Empirical, not documentary: build bottle-shaped archives whose symlinks exit and
   re-enter the destination, drive them through `extractTarGz` on current main, and
   record exactly which shapes pass and which fail. This settles whether #2473's third
   acceptance criterion is "don't break it" or "don't make it harder to fix".

4. **What is the state of the art for safe archive extraction in Go?**
   (lead-prior-art)
   `os.Root` (Go 1.24+), `securejoin`, `openat2` / `RESOLVE_BENEATH`, known tar
   extraction CVEs and how upstreams fixed them, and what GNU/BSD tar do with this
   exact archive. Includes the repo's Go version and toolchain constraints, which
   decide whether `os.Root` is even available.

5. **What is the blast radius of a stricter rule?** (lead-blast-radius)
   Which recipes produce archives containing symlinks, what shapes those symlinks
   take, whether any legitimately point outside the destination, and what CI surfaces
   (golden-plan allowlist, sandbox tests, plugin skills) a change here touches.
