# Exploration Findings: extract-symlink-escape

Round 1 complete. Five leads dispatched, five returned. Sources:
`wip/research/explore_extract-symlink-escape_r1_lead-*.md`.

## The bug, restated precisely

`internal/actions/extract.go` decides containment with string math and then performs
filesystem writes that follow symlinks. `isPathWithinDirectory` (`:21`) and
`validateSymlinkTarget` (`:39`) never touch the filesystem; `filepath.Clean` collapses
`..` textually. Between validation and the `os.MkdirAll` / `os.OpenFile` / `os.Symlink`
calls that follow, nothing re-checks anything. A lexically-innocent path can therefore
resolve, at the syscall level, to somewhere else entirely.

Reproduced against the shared tar loop (`extract.go:282`): the three-entry archive writes
outside the destination and `extractTarGz` returns `nil`.

## What round 1 established

### 1. The escape is broader than the issue reports

The chain plants **directories and symlinks** outside the destination too, not just
regular files, and lengthening it reaches an arbitrary absolute path. The fix must cover
every entry type, not just `tar.TypeReg`.

### 2. There are three vulnerable code paths, not one

| Path | Location | Notes |
|---|---|---|
| tar (all six compressions) | `extract.go:282` `extractTarReader` | Shared loop; one fix covers `.tar.gz/.xz/.bz2/.zst/.lz/.tar` |
| zip | `extract.go:390` `extractZip` | Never creates symlinks, but writes *through* pre-existing ones |
| app bundle zip | `app_bundle.go:277-310` `extractZipWithSymlinks` | **Does** create symlinks, uses both broken helpers, handles macOS `.app` bundles which are symlink-heavy |

`app_bundle.go` is the same defect via the same two helpers. Fixing `extract.go` alone
would leave an identical hole one file over.

### 3. #2275's bottles do NOT extract today

Verified empirically, not from documentation. The exact symlink #2275 quotes --
`libexec/bin/python3.14 -> ../../../../../opt/python@3.14/bin/python3.14` -- fails with
`symlink target escapes destination directory`. `recipes/a/awscli.toml` already records
that macOS awscli was dropped for this reason.

**Therefore #2473's third acceptance criterion ("#2275's relative-symlink bottles still
extract") cannot mean what it says.** It means: do not make #2275 harder to fix. Shallower
bottle symlinks that climb up and back down *without* passing above `destPath` do extract
today, and that already-working population is what must not regress. This gets stated
plainly in the PR rather than silently reinterpreted.

### 4. The line sits at traversal, not at link content

This is the design question the brief flagged, and it has a clean answer. Two separate
rules were being conflated:

- **Link content rule** (what `validateSymlinkTarget` enforces): where may a symlink
  *point*? This is #2275's territory. Policy, not security.
- **Traversal rule** (what nothing enforces): may a write *walk through* a symlink that
  leaves the destination? This is #2473's territory. Security.

Every project that tried to make link content the security boundary shipped either a
bypass or broke legitimate archives -- node-tar's symlink cache, tar-fs, PEP 706's `data`
filter. GNU tar, Docker, containerd and git all converged on policing traversal instead.
GNU tar's own CVE-2025-45582 fix is exactly this.

### 5. `os.Root` is the mechanism, at zero cost

`go.mod` declares `go 1.25.8`, so `os.Root` is stdlib-available with no new dependency.
Confirmed by direct measurement (three independent probes, including two of my own):

```
Symlink a -> "."               -> nil          (creating links is unrestricted)
Symlink b -> "a/.."            -> nil
OpenFile b/pwned               -> openat b/pwned: path escapes from parent
MkdirAll b/x                   -> mkdirat b/x: path escapes from parent
Symlink esc -> ../../outside   -> nil          (#2275 polarity preserved)
OpenFile through escaping final-component symlink -> blocked; target file unmodified
OpenFile through in-root symlink -> nil        (legitimate archives unaffected)
Symlink abs -> /etc, then open abs/passwd -> blocked
```

`os.Root` also blocked the two-archive CVE-2025-45582 variant and a live directory-swap
race, because enforcement is per-syscall against held directory descriptors -- there is no
resolved-path cache to poison.

Alternatives failed concretely rather than theoretically:

- **`EvalSymlinks` resolve-then-check**: disqualifying. Bottle links are dangling at
  extraction time, so it returns `no such file or directory` and cannot distinguish
  dangling from escaping without parsing error strings -- the exact CPython #107845 bug.
  Also structurally TOCTOU.
- **`securejoin`**: returned `err=<nil>` for *every* attack path. It clamps rather than
  rejects (`b/pwned` -> `dest/pwned`), silently mangling filenames instead of failing on a
  tampered archive.
- **Two-pass header validation**: a lexical simulation of the filesystem; must model
  symlink resolution, `..`, `strip_dirs`, and pre-existing destination contents forever.
- **Extract-to-staging-then-verify**: detects after the write already happened.

### 6. Compatibility cost is measured at zero

Across 146 real archives from the local download cache -- 89 containing symlinks, 49,590
symlink entries total -- there was **not one archive that writes an entry through a
symlink**, no absolute link target, and no link escaping at the `strip_dirs` its recipe
actually uses. The traversal rule costs nothing on the real corpus.

### 7. CI will not catch a bottle regression

`extract.go` is on `validate-golden-code.yml`'s explicit exclusion list, and no Homebrew
recipe appears in `test-matrix.json`'s `ci.linux` or `ci.macos` lists. nodejs is the only
matrix tool whose archive carries symlinks. Synthetic table-driven unit tests over
bottle-shaped archives are the only affordable coverage.

Test constraints confirmed: `errcheck` covers test files and excludes neither
`os.WriteFile`, `os.MkdirAll` nor `os.Symlink`; `dupl` fires at 250 tokens across the
near-identical tar and zip paths; the unit-test job fails on non-empty
`git status --porcelain`, so the escape *target* must itself live inside a `t.TempDir()`.

## Decisions taken

| Decision | Rationale |
|---|---|
| Enforce with `os.Root`, not `EvalSymlinks` | Strong in all four evaluation columns; the two `EvalSymlinks` variants are disqualified by dangling bottle links and TOCTOU. Zero dependency cost at Go 1.25.8. |
| Keep `validateSymlinkTarget` as an explicit policy layer | Deleting it would fix #2275, which the brief puts out of scope. Retaining it makes this PR strictly additive on security with no behavior change, and reduces #2275 to a later one-function deletion. |
| Keep the lexical checks as cheap defense in depth | They cost nothing, give better error messages than `os.Root`'s `path escapes from parent`, and demote cleanly from "the guarantee" to "a fast pre-filter". |
| Include `app_bundle.go` | Same defect, same two helpers, and it is the path that handles symlink-heavy macOS bundles. Excluding it would ship a known-identical hole. |
| Exclude the recipe-controlled `dest` parameter | `dest` is joined onto `WorkDir` with no containment check, but recipes can already run arbitrary build commands. The trust boundary is recipe-trusted / archive-untrusted, so `dest` is not a privilege escalation. Note it, do not fix it here. |
| Do not add `O_EXCL` to the file open | Measured unnecessary: `os.Root` blocks writes through an escaping final-component symlink already. `O_EXCL` would break archives with duplicate entries for no security gain. |

## Open questions carried forward

1. **#2275's exact bottle shape is unobserved first-hand** -- it is a macOS Python bottle
   and the survey cache is Linux-only. Mitigation: the fix does not touch link-content
   policy at all, so it cannot change #2275's outcome either way. Synthetic bottle-shaped
   fixtures cover the shallower population that works today.
2. **Error message quality.** `os.Root` exports no sentinel (`errPathEscapes` is
   unexported), so errors must be wrapped with archive context rather than branched on.
3. **Zip silently flattens symlinks into text files** in `extract.go`'s zip path -- a
   latent inconsistency worth documenting, not fixing here.

## Decision: Crystallize

Coverage is sufficient. The mechanism is settled by measurement across three independent
investigations plus direct probes; the compatibility question is answered empirically on a
146-archive corpus; the code paths are enumerated. No gap remains that another research
round would close -- open question 1 is unclosable by research (needs macOS hardware) and
is neutralized by the design rather than by evidence.
