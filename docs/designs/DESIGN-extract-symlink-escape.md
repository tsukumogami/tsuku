---
status: Planned
problem: |
  The extract action decides containment with string math and then performs writes
  that follow symlinks. isPathWithinDirectory and validateSymlinkTarget never touch
  the filesystem, so an archive can stage a symlink in one entry and traverse it in a
  later entry to write outside the destination directory. A three-entry archive is
  enough, extraction reports success, and lengthening the chain reaches an arbitrary
  absolute path. Archives come from upstream release pages and are not trusted, so
  this is an arbitrary file write into $TSUKU_HOME or anywhere else the process can
  reach.
decision: |
  Route every archive-controlled write through a directory handle opened on the
  destination with os.OpenRoot, using each entry's relative path. The kernel then
  enforces containment per path component, so a write that would traverse out of the
  destination fails at the syscall rather than passing a prediction that turns out to
  be wrong. The destination handle is itself derived through a root anchored on the
  work directory, so the destination path is resolved under the same enforcement.
  Three loops convert: extractTarReader and extractZip in extract.go, and extractZIP
  in app_bundle.go. The existing lexical checks stay, demoted from security boundary
  to policy layer and diagnostic pre-filter.
rationale: |
  Two rules were conflated: where a symlink may point, and whether a write may walk
  through one. Only the second is a security boundary, and it is the one nothing
  enforced. Policing traversal rather than link content is where GNU tar, Docker,
  containerd and git all converged, and the projects that instead validated link
  targets each shipped a bypass or broke valid archives. os.Root costs no new
  dependency at the Go version already required, and measurement over 146 real
  archives found none that writes through a symlink, so the rule costs nothing.
---

# DESIGN: Kernel-enforced containment for archive extraction

## Status

Planned

## Context and Problem Statement

`internal/actions/extract.go` guards extraction with two helpers. `isPathWithinDirectory`
(`:21`) compares absolute path strings; `validateSymlinkTarget` (`:39`) rejects absolute
link targets and lexically joins relative ones to check they land inside the destination.
Both call into `filepath.Abs` and `filepath.Join`, which call `filepath.Clean`, which
collapses `..` textually without consulting the filesystem. Neither helper ever asks where
a path component actually points.

Between the check and the write, nothing re-validates. `os.MkdirAll`, `os.OpenFile` and
`os.Symlink` all follow symlinks during path resolution. So a path that is lexically inside
the destination can resolve, at the syscall level, to somewhere else entirely.

Three entries are enough to exploit it:

| Entry | Type | Link target | Lexical verdict | Actual resolution |
|---|---|---|---|---|
| `a` | symlink | `.` | `Join(dest, ".")` = `dest` -- inside | `dest/a` -> `dest` |
| `b` | symlink | `a/..` | `Join(dest, "a/..")` = `dest` -- inside | `dest/b` -> `dest/a/..` -> `dest/..` |
| `b/pwned` | regular | -- | `dest/b/pwned` -- inside | parent of `dest` |

Every entry passes validation. `extractTarGz` returns `nil`. The file lands outside.

The attack turns on ordering: the symlink an entry traverses is created by an *earlier*
entry in the same archive, so no amount of inspecting one entry in isolation can see it.
Lengthening the chain walks up as many levels as desired and reaches an arbitrary absolute
path. Confirmed during exploration, the same trick also plants directories and symlinks
outside the destination, not only regular files.

Recipes are fetched from a registry and archives are downloaded from upstream release
pages. The archive is the untrusted input. A malicious or compromised release tarball can
write into `$TSUKU_HOME/bin`, into `$TSUKU_HOME/share/shell.d` (which is concatenated into
the script the user's shell sources), or anywhere else the process can write.

The comments on both helpers say `SECURITY: Prevents path traversal attacks where
malicious archives could write outside destPath` and `SECURITY: Prevents symlink attacks`.
They do not.

### The distinction the current code is missing

Two separate rules were being enforced by one set of helpers:

- **Link-content rule** -- where may a symlink *point*? This is what
  `validateSymlinkTarget` enforces, and it is a packaging-policy question.
- **Traversal rule** -- may a write *walk through* a symlink that leaves the destination?
  This is the security question, and nothing enforces it.

A symlink pointing outside the tree is inert until something is written through it. A
symlink pointing *inside* the tree -- `a -> "."` -- is the one that carries the attack.
Any fix that stays in link-content space keeps missing this. That is not a hypothesis:
it is the recorded history of node-tar, tar-fs, and CPython's PEP 706 `data` filter.

## Decision Drivers

- **The archive is untrusted; the recipe is not.** A recipe can already run build commands,
  so recipe-controlled inputs are not a privilege boundary. Archive content is.
- **Do not break archives that work today.** 89 of 146 surveyed archives contain symlinks,
  49,590 entries in total. A rule that rejects symlinks broadly is not shippable.
- **Do not make tsukumogami/tsuku#2275 harder.** #2275 asks for relative bottle symlinks
  that exit and re-enter the destination to be *allowed*. It is out of scope here and has
  the opposite polarity, so the fix must not entrench against it.
- **The guarantee must survive shapes nobody enumerated.** The bug exists because the
  original guard reasoned about the archive shapes its author imagined.
- **CI cannot catch a regression here.** No Homebrew recipe is in the CI matrix, and
  `extract.go` is on the golden-plan workflow's exclusion list. Correctness has to come
  from the design and from synthetic tests, not from the pipeline.
- **Time matters.** The reproduction is public in the issue.

## Considered Options

Five mechanisms were evaluated. The full evidence is in the decision reports; the
condensed comparison:

| Mechanism | vs the #2473 chain | vs #2275 bottles | Cost | Blast radius |
|---|---|---|---|---|
| **`os.Root`** | Blocks the chain, the two-archive variant, and a live directory-swap race. Per-syscall, against held descriptors. | Creating escaping targets is unrestricted; only traversal is policed. | Stdlib since Go 1.24; module floor is 1.25.8. | Three loops, mechanical. |
| `EvalSymlinks` resolve-then-check | Catches it only if every entry's parent is re-resolved and nothing races. Structurally TOCTOU. | **Disqualifying** -- bottle links dangle at extraction time, so it returns `no such file or directory` and cannot tell dangling from escaping. | Zero. | Subtle per-entry logic in every loop, `O(entries x depth)` extra `lstat`s. |
| `securejoin` | **Returns `err=<nil>` for every attack path** -- it clamps rather than rejects (`b/pwned` -> `dest/pwned`). | Permissive, but only because it rejects nothing. | New dependency; `pathrs-lite` lacks `Symlink`, forcing a hybrid. | Same diff plus a dependency, for worse semantics. |
| Two-pass header validation | A lexical simulation of the filesystem; must model symlink resolution, `..`, `strip_dirs` and pre-existing contents forever. Blind to state a second extraction created. | Expressible only as a fragile link-target allow-list. | Zero. | Largest and most delicate. |
| Staging + verify | Detects *after* the write has landed on the attacker's chosen path. | Neutral. | Zero. | Extra copy of every tree, and it does not prevent the vulnerability. |

### On rejecting `securejoin` specifically

It is the option that looks most like "use the library everyone uses", so its rejection
deserves to be explicit. Measured, `SecureJoin` mapped `b/pwned` to `dest/pwned` and
`/etc/shadow` to `dest/etc/shadow`, reporting success in both cases. Safe-by-rewriting is
a reasonable default for a server serving user paths. For a package manager it means a
tampered archive installs quietly under mangled filenames instead of failing, which is a
worse outcome than an error.

## Decision Outcome

Route every archive-controlled write through `os.OpenRoot(destPath)` and perform
`MkdirAll`, `OpenFile` and `Symlink` against that handle using the entry's *relative*
path. Containment stops being a property the code predicts and becomes one the kernel
enforces, per component, on every syscall.

Measured behavior against the reproduction:

```
Symlink a -> "."                        -> nil     creating links is unrestricted
Symlink b -> "a/.."                     -> nil
OpenFile b/pwned                        -> openat b/pwned: path escapes from parent
MkdirAll  b/x                           -> mkdirat b/x: path escapes from parent
Symlink esc -> "../../outside"          -> nil     escaping targets still creatable
OpenFile through escaping final symlink -> blocked; target byte-identical after
OpenFile through in-root symlink        -> nil     legitimate archives unaffected
```

The last two lines are the design in miniature. Writing *through* a link that leaves the
destination is refused even when it is the final path component; writing through a link
that stays inside is allowed, so archives that rely on internal symlinks keep working.

This is the rule GNU tar, Docker's `chrootarchive`, containerd and git converged on, and
it is the shape of GNU tar's own CVE-2025-45582 fix.

### The lexical validators stay

`isPathWithinDirectory` and `validateSymlinkTarget` are retained and demoted, for two
distinct reasons.

`validateSymlinkTarget` is retained as **policy**. Deleting it would permit symlinks whose
targets leave the destination -- which is exactly what #2275 requests and exactly what this
change is not chartered to grant. Keeping it makes the change strictly additive: no archive
that extracts today stops extracting, and no archive that fails today starts succeeding.
The behavior change is confined to archives that were escaping.

`isPathWithinDirectory` is retained as a **diagnostic pre-filter**. `os.Root` exports no
sentinel error, and its message names neither the archive nor the entry. The lexical check
catches plain `../` entry names with a message that identifies the offending entry;
`os.Root` catches everything the string comparison cannot see. Neither is load-bearing for
security on its own -- only `os.Root` is.

Both `SECURITY:` comments are rewritten to say what each layer actually guarantees, and
what the lexical helpers explicitly do not. The original comments are the reason this bug
survived review; leaving them approximately true would invite the next reader to repeat it.

### Relationship to tsukumogami/tsuku#2275

#2275's bottles **do not extract today**. Verified against current `main`: the exact
symlink it quotes, `libexec/bin/python3.14 -> ../../../../../opt/python@3.14/bin/python3.14`,
fails with `symlink target escapes destination directory`. `recipes/a/awscli.toml` already
records macOS awscli being dropped for this reason.

So #2473's acceptance criterion "#2275's relative-symlink bottles still extract" cannot
mean what it literally says. It means: do not make #2275 harder to fix. The population that
must not regress is the shallower bottle symlinks that climb up and back down without
passing above the destination -- those do extract today, and the test suite pins them.

This change leaves #2275 strictly easier: once traversal is enforced by the kernel,
granting it is deleting one function and its call sites, with the security guarantee
already underneath. Today it cannot be granted at all without opening the hole this change
closes. It is deliberately not granted here.

## Solution Architecture

### Two layers, clearly separated

```
entry name from archive
        |
        v
  strip_dirs, files filter          -> relativePath   (existing, unchanged)
        |
        v
  lexical pre-filter                -> diagnostic only, names the entry
  (isPathWithinDirectory,
   validateSymlinkTarget)
        |
        v
  os.Root handle on destPath        -> SECURITY BOUNDARY
  root.MkdirAll / OpenFile / Symlink   kernel-enforced, per component
        |
        v
      filesystem
```

### Converted loops

| Loop | Location | Writes archive-controlled paths | Converted |
|---|---|---|---|
| `extractTarReader` | `extract.go:282` | yes -- all six tar compressions share it | yes |
| `extractZip` | `extract.go:390` | yes | yes |
| `extractZIP` | `app_bundle.go:262` | yes -- and it creates symlinks from zip entries | yes |
| `copyDir` | `app_bundle.go` | no -- `filepath.Walk` does not follow symlinks in the source, so it cannot write through a link it created | no |

`copyDir` has a different and lesser defect (it faithfully reproduces an escaping link into
the install tree). That is its own issue, not a rider on a security fix.

### Anchoring the root

`os.OpenRoot` follows symlinks to *establish* the root and only then confines everything
relative to where it landed. So `os.OpenRoot(filepath.Join(ctx.WorkDir, dest))` -- the
obvious spelling -- reproduces the original bug one level up: an archive extracted earlier
into the work directory can plant a symlink exactly where a later step's `dest` points, and
the later extraction gets a root anchored outside.

The security review demonstrated this end to end. No malicious recipe is needed; an
ordinary two-step recipe that extracts one archive into the work directory and a second
into a subdirectory of it is enough. The archive supplies the symlink, the recipe
innocently supplies the `dest` that names it.

The destination handle is therefore derived *through* a root on the work directory:

```go
workRoot, err := os.OpenRoot(ctx.WorkDir)   // resolved once, before any archive content
defer workRoot.Close()
if err := workRoot.MkdirAll(dest, 0o755); err != nil { ... }
destRoot, err := workRoot.OpenRoot(dest)    // dest resolved under enforcement
defer destRoot.Close()
```

Verified against every `dest` shape the action accepts -- `.` (the default), `sub`,
`a/b/c`, `./x` -- each of which writes normally and refuses `../` escapes. A `dest`
planted as an escaping symlink now fails with `openat sub: path escapes from parent`.

This also refuses an absolute `dest` (`openat /tmp: path escapes from parent`), which
closes a carve-out an earlier draft had accepted on the grounds that recipes are trusted.
Recipes remain trusted; the destination is simply no longer a way to leave the work
directory, which is one less thing to reason about.

### Two mandatory details that measurement surfaced

Both were found by running real archives, not by reading code, and either would have
broken production recipes.

1. **`filepath.Join(parts...)` returns `""` for a stripped top-level directory entry.**
   `os.Root` rejects the empty path with `mkdirat : empty path`. Across a 19-archive real
   corpus this occurs 21 times at `strip_dirs=2` and 3 times at `strip_dirs=1` -- that is
   every Homebrew bottle and every node-style tarball. The conversion normalizes `""` to
   `"."` after the `files` filter.

2. **`Root.OpenFile` and `Root.MkdirAll` error on permission bits outside `0o777`**
   (`openat: unsupported file mode`). `f.Mode()` from a zip directory entry carries
   `ModeDir`, so `app_bundle.go` would fail on any zip containing directory entries. Every
   mode passed to a `Root` method takes `.Perm()`.

   `.Perm()` is behavior-preserving, not a silent tightening. Tar stores setuid in the unix
   layout (`04000`) while Go's `os.ModeSetuid` is `1<<23`, so `os.FileMode(header.Mode)`
   never sets Go's setuid bit and the *existing* code already drops it. Verified: a `04755`
   header produces `-rwxr-xr-x` on disk both before and after the change.

A differential run over 19 real archives at `strip_dirs` 0, 1 and 2 -- 57 runs -- produced
byte-identical output trees before and after the conversion.

### Symlink creation

`atomicSymlink` (`extract.go:368`) creates a temp link and renames it. A root-relative
sibling, `atomicSymlinkAt(root, ...)`, is added for the converted loops. The existing
function stays: two `app_bundle.go` call sites create symlinks at absolute paths outside
any root, and those are not archive-controlled.

### Error surface

`os.Root`'s escape error is unexported, so no code branches on it. Every `Root` call is
wrapped with the entry name. For the canonical archive a user sees:

```
archive entry "b/pwned": failed to create parent directory: mkdirat b: file exists
```

The underlying message is the kernel's, not a chosen one, and it names the symlink rather
than the escape. That is a readability cost accepted in exchange for not string-matching on
an unexported error; the entry name in the wrapper is what makes it actionable.

## Implementation Approach

1. **Convert `extractTarReader`.** Anchor a root on `ctx.WorkDir` and derive the
   destination root through it, normalize the empty relative path, route `MkdirAll` /
   `OpenFile` / `Symlink` through the destination root, apply `.Perm()`, wrap errors with
   the entry name.
2. **Convert `extractZip`** in the same file, sharing the normalization and wrapping.
3. **Convert `extractZIP`** in `app_bundle.go`.
4. **Rewrite both `SECURITY:` comments** to state the real guarantee and the real
   non-guarantee.
5. **Add the regression suite** -- 31 table-driven cases across the three extractors.
6. **Run the mutation exercise** and record the result in the PR body.
7. **Update the recipe-authoring skill** with the symlink contract, which no skill
   documents today.

## Security Considerations

**Threat model.** The archive is attacker-controlled: recipes name upstream release URLs,
and a compromised or malicious release replaces the tarball. The recipe itself is not a
privilege boundary -- a recipe can already run build commands. But recipe-controlled inputs
are still in scope where an *archive* can influence how they resolve, which is exactly the
two-archive escape the security review found and the anchoring change closes.

**What the change guarantees.** No archive entry can cause a write outside the destination,
for any entry type, regardless of entry ordering, symlink chain length, symlinks already
present in the destination, or symlinks planted by a previously-extracted archive along the
destination path itself. Enforcement is per-syscall against held directory descriptors, so
it holds against a second archive extracted into the same destination and against an
attacker racing a directory swap -- both verified.

**Attacks tried and held.** Hardlink and device entries (silently skipped today; `Root.Link`
is contained if they are ever supported), a symlinked destination in isolation, permission-bit
masking, tricking the retained lexical pre-check into skipping a `Root` call, symlink loops
(`ELOOP` surfaces as an ordinary error, no hang), and platform coverage (`os.Root`'s build
tag covers both supported platforms with no kernel-version fallback).

**What it does not guarantee.** Content written *inside* the destination is unconstrained:
an archive can still overwrite its own files, and a symlink inside the tree that points
outside is still created (that is the policy layer's business, and it is what
`install_binaries` and `homebrew_relocate` consume afterward). Extraction does not
constrain what later actions do with an escaping link they find.

**Residual risks.** `copyDir` still reproduces escaping links into the install tree; filed
separately. Zip extraction in `extract.go` flattens symlink entries into text files with no
warning, which is a pre-existing inconsistency, not a containment hole.

**Why the guards are believed to work.** Twelve defects were injected one at a time and ten
died to a specifically named test. The remaining two -- root-scoping the symlink *write* --
cannot produce an escape given `symlink(2)` and `rename(2)` semantics, verified by probe,
and are recorded as expected survivors rather than papered over. The first draft of the
suite, built only from the issue's reproducer, let ten of twelve defects survive because
the guards mask each other; deep-nesting and final-component-escape cases were added
specifically to break that masking.

## Consequences

**Positive.** Containment becomes a kernel-enforced property rather than a predicted one,
so it holds for shapes nobody enumerated. The `SECURITY:` comments become true. #2275
becomes tractable instead of blocked. Two latent breakages (`strip_dirs` empty paths, zip
directory modes) were found and are covered by tests.

**Negative.** Extraction holds an open directory handle for the loop's duration. Escape
errors carry the kernel's wording rather than a chosen one. The two lexical helpers now
have a subtler role than their names suggest.

**Mitigations.** The handle is closed with `defer` in each loop. The entry name is wrapped
into every error so the message is actionable despite the kernel's phrasing. The rewritten
comments state each layer's role explicitly, which is what stops the next reader from
repeating the original mistake.

**Out of scope, deliberately.** tsukumogami/tsuku#2275; `copyDir`'s link reproduction;
zip symlink flattening; hardlink and device entries, which are silently skipped today; and
consolidating the four divergent containment predicates elsewhere in the tree into a
shared helper.
