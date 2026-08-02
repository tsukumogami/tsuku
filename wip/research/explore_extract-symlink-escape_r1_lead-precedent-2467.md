# Lead: What does the in-tree containment precedent from PR #2467 do, and how much of it generalizes into a reusable primitive?

## Findings

### 1. What `install_program_files.go` actually does

The whole action is 234 lines; the containment logic is `Execute` (lines 97-131) plus
`copyProgramFile` (lines 134-203) plus `isWithin` (lines 206-212), all in
`internal/actions/install_program_files.go`.

**Layer 1 — lexical rejection at Preflight (lines 57-68, 74-81).** Before anything
touches the filesystem, each `files` entry must be relative and free of `..`:

```go
if filepath.IsAbs(f) {
    result.AddErrorf("install_program_files: %q must be relative to the tool install directory", f)
}
if hasParentTraversal(f) {
    result.AddErrorf("install_program_files: %q may not contain %q", f, "..")
}
```

`hasParentTraversal` splits on **both** `/` and `\` (line 75) rather than using
`filepath.Split`, so a Windows-style separator cannot smuggle a `..` past a Unix build.
Note this is *not* the security boundary — it is a cheap early reject. The real boundary
is layer 2.

**Layer 2 — resolve the root once, before the loop (lines 117-122).** This is the
load-bearing comment in the whole file:

```go
// The archive extractor's containment checks are lexical, so a symlink in a release
// tarball can point outside the tool directory. Resolve before trusting.
resolvedRoot, err := filepath.EvalSymlinks(ctx.ToolInstallDir)
if err != nil {
    return fmt.Errorf("install_program_files: failed to resolve tool install directory: %w", err)
}
```

The root is resolved **once**, outside the per-file loop, and every subsequent comparison
is against `resolvedRoot`, never against `ctx.ToolInstallDir`. That matters: comparing a
resolved child against an unresolved root produces false rejections whenever the root
itself sits behind a symlink (macOS `/var` → `/private/var` is the standard case, and
`internal/install/checksum.go:47` calls out exactly that).

**Layer 3 — resolve each source and re-check containment (lines 135-143).**

```go
src := filepath.Join(resolvedRoot, rel)

resolvedSrc, err := filepath.EvalSymlinks(src)
if err != nil {
    return fmt.Errorf("%q: %w", rel, err)
}
if !isWithin(resolvedSrc, resolvedRoot) {
    return fmt.Errorf("%q resolves to %q, outside the tool install directory", rel, resolvedSrc)
}
```

`EvalSymlinks` resolves *every* component, not just the leaf, so a symlinked intermediate
directory (`bin` → `/etc`, then `bin/passwd`) is caught too. Both sides of the comparison
are in the same resolved namespace.

**Layer 4 — open with `O_NOFOLLOW` and type-check the handle, not the path (lines 145-159).**

```go
// O_NOFOLLOW plus fstat on the handle: the type check and the read then see the same
// object, which a stat-then-open pair cannot guarantee.
in, err := os.OpenFile(resolvedSrc, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
...
info, err := in.Stat()   // fstat(2) on the fd, not stat(2) on the path
...
if !info.Mode().IsRegular() {
    return fmt.Errorf("%q is not a regular file", rel)
}
```

Two distinct guarantees here. `O_NOFOLLOW` closes the TOCTOU window between
`EvalSymlinks` returning and the open happening — if someone swapped `resolvedSrc` for a
symlink in between, the open fails with `ELOOP` instead of following it. And `in.Stat()`
is `fstat` on the descriptor, so the regular-file check and the subsequent `io.Copy`
provably see the same inode. A `os.Stat(path)` / `os.Open(path)` pair cannot promise that.

**Layer 5 — mode derived, not carried (lines 161-166).**

```go
// Derived from the execute bit rather than carried over from the tar header, so a
// crafted archive cannot choose the mode of a file tsuku writes.
mode := os.FileMode(0644)
if info.Mode().Perm()&0111 != 0 {
    mode = 0755
}
```

The mode collapses to exactly two values. Contrast `extract.go:334`, which passes
`os.FileMode(header.Mode)` straight from the tar header into `OpenFile` — setuid, setgid
and sticky bits included. (`install_libraries.go:110` does mask those three; `extract`
does not.)

**Layer 6 — the write side: `O_EXCL|O_NOFOLLOW` temp file, then rename (lines 168-199).**

```go
base := filepath.Base(rel)       // flattens: dest is always one level deep
dest := filepath.Join(destDir, base)
tmp := dest + ".tsuku-tmp"

// O_EXCL|O_NOFOLLOW so a pre-planted symlink at the temp path cannot redirect the
// write. Removing first keeps a crashed previous run from wedging this one.
if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) { ... }
out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, mode)
...
// O_CREATE honors the umask, so set the mode explicitly.
if err := os.Chmod(tmp, mode); err != nil { ... }
// Rename last: a concurrent `nvm exec` never observes a half-written file.
if err := os.Rename(tmp, dest); err != nil { ... }
```

`O_EXCL` and `O_NOFOLLOW` are belt-and-braces on the same hazard: `O_EXCL` refuses to open
anything that already exists (including a symlink), `O_NOFOLLOW` refuses to traverse one.
The explicit `Chmod` after the open exists because `O_CREATE`'s `perm` argument is masked
by umask — a detail any reusable write helper has to carry.

**Layer 7 — the destination is not a parameter at all.** The doc comment on `Execute`
(lines 85-96) is explicit that this is a security property: a recipe-supplied destination
could reach `share/shell.d`, which is concatenated into the user's shell init script.
`destDir` comes from `dataDir(ctx)` (`internal/actions/data_dir.go:23`), which builds
from `ctx.ToolsDir` and a validated recipe-name segment. Plus `filepath.Base(rel)` at line
168 means even the *filename* can't carry a directory component into the destination.

### 2. What generalizes, and what does not

Layers 2, 3, 4 and 6 are pure containment logic with nothing nvm- or program-file-specific
about them. Layers 1, 5 and 7 are policy that each caller has to choose for itself
(`extract` cannot flatten with `filepath.Base`, and it must create directories and
symlinks that `install_program_files` never creates).

The reusable core is two operations:

```go
// ResolveWithin resolves name (relative to resolvedRoot) through every symlink and
// returns the resolved absolute path, erroring if it lands outside resolvedRoot.
// resolvedRoot must already be EvalSymlinks-resolved.
func ResolveWithin(resolvedRoot, name string) (string, error)

// OpenFileNoFollow opens path with O_NOFOLLOW and fstats the handle, returning an
// error unless the handle is a regular file.
func OpenRegularNoFollow(path string) (*os.File, os.FileInfo, error)

// WriteFileAtomic writes via a temp sibling opened O_CREATE|O_EXCL|O_NOFOLLOW,
// chmods explicitly (umask), then renames.
func WriteFileAtomic(dest string, mode os.FileMode, w func(io.Writer) error) error
```

**But the honest answer to "how much generalizes" is: less than you would hope, because
`EvalSymlinks`-then-check does not compose for a writer.** `install_program_files` is a
*reader* — every path it resolves already exists, so `EvalSymlinks` always succeeds.
`extract` is a *writer*: the paths it is about to create do not exist yet, and
`EvalSymlinks` returns `ENOENT` for any path with a non-existent component. So a shared
`ResolveWithin` would have to be called on `filepath.Dir(target)` rather than `target`,
and only after that parent has been created — and it would have to be re-called after
every entry, because a later entry can change what an earlier component resolves to. That
is a per-entry `EvalSymlinks` on a path that grows with archive depth, and it is *still*
TOCTOU-racy against a concurrent process.

### 3. The better primitive is already in the standard library: `os.Root`

`go.mod` declares `go 1.25.8`, and `os.Root` (Go 1.24, extended in 1.25) is a
directory-handle-scoped filesystem API implemented with `openat2`/`openat` +
per-component `O_NOFOLLOW` on Linux. The repo uses it in **zero** places — the grep for
`os.Root` and `openat` across every `.go` file returns nothing.

I ran the exact three-entry attack from #2473 against `os.Root` on this machine
(scratch program, since deleted):

```
Symlink a -> '.':                  <nil>
Symlink b -> 'a/..':               <nil>
OpenFile b/pwned:                  openat b/pwned: path escapes from parent
contained: no file at /tmp/rootprobe.../pwned
Symlink abs -> /etc:               <nil>
Open abs/passwd:                   openat abs/passwd: path escapes from parent
MkdirAll b/sub:                    mkdirat b/sub: path escapes from parent
2275 Symlink create (exits dest):  <nil>          <- #2275's bottle case still works
2275 Lstat:                        true <nil>
OpenFile lib/ok.txt (legit in-root symlinked dir): <nil>
Open 'a' (symlink as final component):             ok (followed)
```

Three things worth pulling out:

- `os.Root` blocks the #2473 chain **mechanically**, with no lexical reasoning at all, and
  the kernel enforces it per component so there is no TOCTOU window.
- `os.Root.Symlink` *creates* links without validating the target ("Symlink does not
  validate oldname" — `go doc os.Root.Symlink`), so #2275's
  `libexec/bin/python3.14 -> ../../../../../opt/python@3.14/bin/python3.14` still
  extracts. #2473 and #2275 stop being in tension: containment moves to *traversal*,
  while *what a link says* becomes a separate policy decision the extractor can keep
  making lexically.
- `os.Root` still **follows a symlink as the final component** if it stays in-root. So a
  two-entry archive (`link -> real.txt`, then a regular-file entry named `link`) would
  clobber `real.txt` rather than replacing the link. Contained, but wrong. The final open
  still wants `O_EXCL` or `O_NOFOLLOW` — which is exactly the layer-6 pattern from #2467,
  now expressed as `root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)`.

Windows is not a concern: `syscall.O_NOFOLLOW` is Unix-only, but `GOOS=windows go build`
already fails in `internal/autoinstall` (`undefined: syscall.Stat_t`), and neither the
Makefile nor any workflow names windows. The project is Unix-only today.

### 4. Where a helper would live

`internal/actions` has 64 non-test `.go` files and is already the dumping ground. PR #2467
itself set the useful precedent: it put `dataDir`/`recipePathSegment` in their own small
file, `internal/actions/data_dir.go`, rather than growing `utils.go`. A containment helper
should follow that — `internal/actions/pathsafe.go` — **not** a new `internal/pathsafe`
package, unless callers outside `internal/actions` need it.

They do, though, if the goal is to deduplicate. There are currently **four** independent
containment predicates:

| Location | Form | Notes |
|---|---|---|
| `internal/actions/extract.go:21` `isPathWithinDirectory` | `Abs` + `strings.HasPrefix(base+sep)` | lexical only |
| `internal/actions/install_program_files.go:206` `isWithin` | `filepath.Rel` + `HasPrefix("..")` | the #2467 one |
| `internal/actions/set_rpath.go:379` `validatePathWithinDir` | `Abs` + `Rel` + `HasPrefix("..")` | returns error |
| `internal/install/checksum.go:224` `isWithinDir` | `Clean` + `Rel` + `len(rel)>=2 && rel[:2] != ".."` | **has a bug, see below** |

Three of them (`isWithin`, `validatePathWithinDir`, `isPathWithinDirectory` are all in
`internal/actions`) plus one in `internal/install`. If the helper is only ever going to
serve `internal/actions`, put it in `internal/actions/pathsafe.go`. If you want
`internal/install/checksum.go` on it too — and you should, see below — it needs to be
`internal/pathsafe`. My recommendation: `internal/pathsafe`, because the containment
predicate is not action-specific and the current duplication is already producing
divergent bugs.

### 5. Other in-tree resolve-then-check sites, and whether each is correct

**Correct, and the closest sibling to #2467:**

- `internal/actions/shell_init.go:355-374` — `EvalSymlinks` on both the binary and
  `toolInstallDir`, then `Rel` + `HasPrefix("..")`. Same shape, done independently. No
  `O_NOFOLLOW` because it only validates a path it then passes to a subprocess, so there
  is a TOCTOU window, but the window is inherent to exec-by-path.
- `internal/install/checksum.go:47-67` `ComputeBinaryChecksums` — resolves `toolDir` once
  outside the loop, resolves each binary, checks containment. Same structure as #2467.
- `internal/verify/dltest.go:334-350` — `EvalSymlinks` on `libsDir` and each path before
  the prefix check; the doc comment at :330 says so explicitly.
- `internal/project/config.go:80` — resolves `startDir` before upward traversal, with a
  comment saying why.

**Incorrect or incomplete:**

- **`internal/install/checksum.go:80-110` `VerifyBinaryChecksums` resolves but never
  checks.** It calls `filepath.EvalSymlinks(absPath)` at line 95 and then goes straight to
  `ComputeFileChecksum(realPath)` — the `isWithinDir` guard that its sibling
  `ComputeBinaryChecksums` applies at line 65 is simply absent. A binary path that resolves
  outside the tool directory gets checksummed anyway. Consequence is bounded (it reports a
  mismatch rather than writing anything) but the asymmetry is clearly unintentional.
- **`internal/install/checksum.go:224` `isWithinDir` rejects single-character children.**
  `return !filepath.IsAbs(rel) && (rel == "." || (len(rel) >= 2 && rel[:2] != ".."))` — for
  `rel == "a"` the length guard is false, so the whole expression is false and a file
  literally named `a` directly inside `dir` is reported as outside it. Fail-closed, so not
  a security hole, but it is a latent false-rejection.
- **`isWithin` / `validatePathWithinDir` use `strings.HasPrefix(rel, "..")`,** which also
  rejects a legitimate child named `..foo`. `filepath.Rel` only ever emits `..` as a whole
  component, so the correct test is
  `rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))`. Fail-closed
  again, but a shared helper should get it right once.
- `internal/actions/download_cache.go:357-388` `containsSymlink` walks each parent
  component with `Lstat` looking for any symlink. It is a *reject-if-any-symlink* policy,
  not a containment check, and it is racy by construction (check-then-use on a path
  string). For cache directories that is arguably fine; it is not a model to copy.
- `internal/actions/set_rpath.go:331-338` — `Lstat` + `ModeSymlink` check on the binary,
  then `os.Rename` + `os.WriteFile` on the same path. Classic TOCTOU: the check and the
  write are separate syscalls on a path. `#2467`'s fstat-the-handle pattern is the fix.
- `internal/actions/utils.go:90-117` `CopyFile` uses plain `os.Create(dst)`, which follows
  a symlink at the destination. The #2467 design doc says so in as many words
  (`docs/designs/current/DESIGN-nvm-data-root.md:518`): *"The shared `copyFile` helper
  cannot be reused: it uses plain `os.Create`, which follows a symlink at the
  destination."* `install_libraries.go:112` and `homebrew` paths call it.

### 6. What #2467's review and design doc say about generalizing

There are **zero** review comments and **zero** reviews on PR #2467 (`gh api
repos/tsukumogami/tsuku/pulls/2467/comments` and `.../reviews` both return empty) — it
was authored and merged by the same person. So there is no reviewer commentary to mine.
What exists instead is the PR body and the design doc, both of which are unusually
explicit.

The PR body's follow-ups section states the intent directly:

> **#2473 — the archive extractor's containment is purely lexical**, so a symlink chain
> in a release tarball escapes the destination and writes an arbitrary file. Re-verified
> against `extractTarGz` before filing: every entry validates, the call returns `nil`, and
> the file lands outside. The most serious finding of the review, and the reason
> `install_program_files` resolves with `EvalSymlinks` and opens with `O_NOFOLLOW` rather
> than trusting extraction.

`docs/designs/current/DESIGN-nvm-data-root.md:405-407` and `:513-518` restate the
mechanism. But **neither states an intent to extract a shared helper.** The framing
throughout is "this action does not trust extraction", not "this is the first user of a
primitive". The generalization is an inference the reader is invited to make, not a
commitment the PR made.

Issue #2473's own "Suggested direction" already names both options and prefers the second:

> Either run `filepath.EvalSymlinks` on each entry's parent before the containment check,
> or extract with `openat`-style relative descriptors so resolution happens against a
> directory handle rather than a string. `O_NOFOLLOW` on the final open closes the last hop.

### 7. Existing symlink-safety tests worth mirroring

- **`internal/actions/install_program_files_test.go:81-97`** is the closest model. It
  seeds a real symlink pointing at a real file in a *different* `t.TempDir()`, runs the
  real `Execute`, and asserts on the error substring. Table-driven with a `seed` closure
  per case. The comment above it (lines 82-84) states *why* the case exists, which is the
  house style.
- **`internal/actions/extract_test.go:567-655`
  `TestExtractTar_SymlinkAttacks_SecurityEdgeCases`** is the one to extend. It builds a
  real `tar.gz` in memory and drives `action.extractTarGz`. Critically it is
  **single-entry**: each subtest writes one `target.txt` plus exactly one symlink header,
  so the multi-entry stage-then-traverse attack is structurally outside what this test can
  express. Extending it means changing the fixture from `linkName/linkTarget` to a slice of
  headers.
- `internal/actions/extract_test.go:412-560` — `TestExtractTar_PathTraversal_SecurityEdgeCases`
  and its zip twin, same single-entry shape.
- `internal/actions/extract_test.go:685-710` — direct unit tests of `validateSymlinkTarget`
  against string inputs. These will need rewriting or deleting if the function's contract
  changes, and they are the tests most likely to give false confidence: they assert the
  lexical function does its lexical job, which it does.
- `internal/install/symlink_test.go`, `internal/actions/util_resolve_test.go`,
  `internal/shellenv/cache_test.go` and `internal/shellenv/doctor_test.go` all have
  symlink-rejection cases worth reading for style.

The mutation-testing table in the #2467 PR body is also a house convention worth
mirroring — it lists which deliberately-injected defect each guard catches, and
`Drop the resolved-source containment check | TestInstallProgramFilesExecute` is exactly
the row an extract fix should be able to produce.

## Implications

1. **The precedent's *reasoning* generalizes; its *mechanism* does not, cleanly.**
   Resolve-before-trusting, check both sides in the same resolved namespace, `O_NOFOLLOW`
   on the open, fstat the handle, temp-file-plus-rename — all correct, all worth carrying.
   But `EvalSymlinks` requires the path to exist, and `extract` creates paths. Lifting
   `copyProgramFile` verbatim into the extractor does not work.

2. **`os.Root` is the primitive the repo is missing, and it is free.** Go 1.25.8 is already
   the declared version, `os.Root` has every method extraction needs (`MkdirAll`,
   `OpenFile`, `Symlink`, `Lstat`, `Remove`, `Rename`), it blocks the #2473 chain
   mechanically, it is race-free where a resolve-then-check helper cannot be, and it
   *simultaneously* unblocks #2275 because it validates traversal rather than link text.
   A hand-rolled `EvalSymlinks`-per-entry helper would be more code, slower, and strictly
   weaker.

3. **The shape of the fix, concretely.** In `extractTarReader`/`extractZip`, replace the
   `filepath.Join(destPath, relativePath)` + `isPathWithinDirectory` pair with
   `os.OpenRoot(destPath)` and route every `MkdirAll`, `OpenFile` and `Symlink` through
   the root using the *relative* entry path. Keep `validateSymlinkTarget`'s absolute-target
   rejection as an explicit policy layer (os.Root will happily create an absolute symlink
   it then refuses to traverse, and downstream consumers — `install_binaries`,
   `homebrew_relocate`, the user's shell — do not go through the Root). Add `O_EXCL` to
   the regular-file open so a staged in-root symlink cannot be written through. Rewrite the
   `SECURITY:` comments to describe traversal containment rather than "prevents path
   traversal attacks", per #2473's fourth acceptance criterion.

4. **A shared containment predicate is still worth extracting, for the non-extract
   callers.** Four divergent implementations, two with latent false-rejection bugs and one
   (`VerifyBinaryChecksums`) missing the check entirely. `internal/pathsafe` with a single
   correct `Within(path, root string) bool` plus `ResolveWithin(root, name string) (string,
   error)` would fix all four. That is a separate, smaller change from the extract fix and
   should not be bundled with it.

5. **The write-side pattern is independently reusable.** `install_program_files`'s
   `O_EXCL|O_NOFOLLOW` temp file + explicit `Chmod` + `Rename` should replace
   `utils.go:CopyFile`'s `os.Create`, which currently follows a destination symlink and is
   called from `install_libraries` and the homebrew paths.

## Surprises

- **PR #2467 has no reviewers, no review comments, and no reviews.** The "security,
  architecture, and pragmatism reviews" the PR body describes were agent reviews conducted
  during authoring, not GitHub review threads. There is no external commentary on the
  approach and no stated intent to generalize it — the durable record is the PR body and
  `DESIGN-nvm-data-root.md`.

- **`os.Root` resolves the apparent #2473-vs-#2275 conflict outright.** #2473 asks for
  symlink escapes to be blocked; #2275 asks for bottles whose symlinks exit the dest to
  extract. Both are satisfiable at once because `os.Root.Symlink` does not validate its
  target while every traversal *through* the root is checked. Neither issue notices this.

- **The repo uses `os.Root` in exactly zero places** despite declaring `go 1.25.8`, and has
  four hand-rolled containment predicates instead.

- **`VerifyBinaryChecksums` (`internal/install/checksum.go:80-110`) calls `EvalSymlinks`
  and then never checks the result**, while the function 30 lines above it does. Looks like
  a copy that lost a guard.

- **`isWithinDir` (`internal/install/checksum.go:224`) reports single-character children as
  outside the directory** — `len(rel) >= 2` makes `rel == "a"` fail. Fail-closed, but a
  real latent bug.

- **`extract.go:334` passes `os.FileMode(header.Mode)` straight from the tar header** into
  `OpenFile`, setuid/setgid/sticky included. `install_libraries.go:110` masks exactly those
  three bits; `install_program_files.go:163-166` derives the mode from scratch. The
  extractor is the only one of the three that trusts the archive.

- **`extractTarReader` silently ignores `tar.TypeLink` (hardlinks)** — the switch at
  `extract.go:323` handles only `TypeDir`, `TypeReg` and `TypeSymlink`. Also, zip archives
  have no symlink branch at all: a zip entry with the symlink mode bit is written as a
  regular file containing the target string. Neither is a hole, but both are gaps a fix
  should decide about deliberately rather than inherit.

## Open Questions

1. Does `os.Root`'s per-entry cost matter? Each entry becomes a fresh `openat` walk from
   the root descriptor. For a large bottle (tens of thousands of entries) this is more
   syscalls than `filepath.Join` + one `OpenFile`. Probably irrelevant next to
   decompression, but unmeasured. A `Root.OpenRoot` cache keyed on directory would fix it
   if it does.

2. `extract`'s `destPath` is `filepath.Join(ctx.WorkDir, dest)` with a recipe-controlled
   `dest` and no containment check on `dest` itself (`extract.go:124-129`). Is that
   reachable — can a recipe set `dest = "../../.."`? If so, `os.OpenRoot(destPath)` fixes
   the archive-entry problem while leaving the destination problem open, and the fix
   should also validate `dest` against `ctx.WorkDir`. Not investigated.

3. Should `internal/pathsafe` exist as part of this fix, or as a follow-up? Extracting it
   touches `internal/install/checksum.go`, `internal/actions/set_rpath.go` and
   `internal/actions/extract.go` and fixes two latent bugs — but it is orthogonal to
   #2473's acceptance criteria and would inflate the diff. My read is: follow-up issue.

4. Does anything downstream of extraction rely on the current behaviour that an
   escaping-but-lexically-valid symlink chain gets created? The homebrew relocate path
   (`internal/actions/homebrew_relocate.go`) does a lot of `EvalSymlinks`-based rewriting
   over the extracted tree. Unknown whether switching extraction to `os.Root` changes what
   it sees.

5. `os.Root` does not prohibit crossing filesystem boundaries, bind mounts, `/proc`, or
   device files (per `go doc os.Root`). Is any of that in scope for a package manager
   extracting into a temp work dir? Probably not, but it should be stated rather than
   assumed.

## Summary

PR #2467's `install_program_files` establishes a five-part containment recipe — resolve the
root once with `EvalSymlinks`, resolve each entry and compare in the same resolved
namespace, open `O_RDONLY|O_NOFOLLOW` and `fstat` the *handle* rather than the path, derive
the file mode instead of carrying it from the archive, and write via an
`O_CREATE|O_EXCL|O_NOFOLLOW` temp file plus rename — and its reasoning generalizes even
though its mechanism does not, because `EvalSymlinks` needs paths to exist and `extract`
is a writer, not a reader. The stronger primitive is already available and unused: `go.mod`
declares `go 1.25.8`, `os.Root` appears nowhere in the repo, and I confirmed empirically
that it blocks #2473's exact `a -> "."` / `b -> "a/.."` / `b/pwned` chain at the kernel
per-component level while still permitting #2275's exit-and-re-enter bottle symlinks to be
created — so routing extraction through `os.OpenRoot(destPath)` with `O_EXCL` on the final
open resolves both issues at once, with #2467's lexical rules demoted from security
boundary to policy layer. The biggest open question is whether `extract`'s own
recipe-controlled `dest` parameter (`extract.go:124-129`, joined onto `ctx.WorkDir` with no
containment check) is separately reachable, since `os.Root` would contain the archive
entries while leaving that path untouched.
