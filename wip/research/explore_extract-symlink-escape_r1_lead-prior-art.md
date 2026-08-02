# Lead: What is the state of the art for safe archive extraction in Go, and which options are actually available to this repo?

All claims below were produced by running code on this machine unless marked
*(literature)*. Scratch programs live under
`/home/dgazineu/.claude/jobs/1013267d/tmp/` (`rootexp/`, `sj/`, `mini/`,
`bench/`, `tarexp/`). No tracked repo file was modified.

## Findings

### 1. Toolchain constraints -- minimum Go is 1.25.8, so `os.Root` is available

`go.mod` line 3 declares `go 1.25.8` with **no `toolchain` line**. Installed
toolchain is `go1.25.8 linux/amd64`, GOROOT
`/home/dgazineu/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.8.linux-amd64`.

Every CI job that compiles the CLI uses `go-version-file: go.mod` (61 call
sites across `.github/workflows/`), which pins setup-go to 1.25.8. Two jobs
look like exceptions but are not:

- `.github/workflows/integration-tests.yml:180` runs in `container: image: golang:1.23-alpine`, but line 182 sets `GOTOOLCHAIN: auto`. With `go 1.25.8` in `go.mod`, the 1.23 toolchain downloads and switches to 1.25.8 before building.
- `.github/workflows/weekly-coverage-report.yml:20` sets `go-version: '1.22'`. GOTOOLCHAIN defaults to `auto`, so `go build -o tsuku ./cmd/tsuku` at line 23 also switches to 1.25.8. A 1.22 toolchain cannot even parse `go 1.25.8` without switching.

No Dockerfile pins a Go version for the CLI build. Release targets, from
`.goreleaser.yaml:14-19`, are `linux` and `darwin` on `amd64`/`arm64` -- no
Windows, no js/wasm, no plan9.

**Minimum Go version the code must compile under: 1.25.8.** `os.Root` landed
in 1.24; the full method set below is present in 1.25.8. There is no
Go-version cost to using it.

### 2. `os.Root` -- what it actually guarantees (measured, not recalled)

**Method set in Go 1.25.8** (`$(go env GOROOT)/src/os/root.go`, all 23
methods): `Name`, `Close`, `Open`, `Create`, `OpenFile`, `OpenRoot`, `Chmod`,
`Mkdir`, `MkdirAll`, `Chown`, `Lchown`, `Chtimes`, `Remove`, `RemoveAll`,
`Stat`, `Lstat`, `Readlink`, `Rename`, `Link`, `Symlink`, `ReadFile`,
`WriteFile`, `FS`. Everything an extractor needs is there, including
`Symlink`, `MkdirAll`, `Chmod`, and `Chtimes`.

**It is not openat2.** `grep -rn "Openat2\|openat2" $GOROOT/src/os/*.go`
returns nothing. `strace` of the experiment shows Go walking the path
component by component:

```
symlinkat(".", 3, "a")     = 0
symlinkat("a/..", 3, "b")  = 0
readlinkat(3, "b", "a/..", 128) = 4
readlinkat(3, "a", ".", 128)    = 1
```

`root_openat.go` (build tag `//go:build unix || windows || wasip1`, so darwin
is covered) opens each component `O_NOFOLLOW` relative to a held dirfd,
`readlinkat`s any symlink it hits, and on a `..` component **restarts
resolution from the root fd** rather than doing `openat(dir, "..")` -- the
comment at `root_openat.go:287` says this is because "dir may have moved since
we opened it." Limits are `maxSteps = 255` and `maxRestarts = 8`; exceeding
both yields `syscall.ENAMETOOLONG`. Practical consequence: **no kernel-version
or platform dependency**, unlike an openat2-based approach that needs Linux
5.6+ and has no macOS equivalent.

**The guarantee is about the path being operated on, not about link content.**
Confirmed directly:

| Call | Result |
|---|---|
| `root.Symlink(".", "a")` | OK |
| `root.Symlink("a/..", "b")` | OK |
| `root.Symlink("/tmp/.../outside", "esc")` (absolute, escaping) | **OK** |
| `root.Symlink("../outside/lib.so", "relesc")` (relative, escaping) | **OK** |
| `root.Create("b/pwned")` -- the #2473 traversal | **blocked** |
| `root.Create("esc/pwned")` | **blocked** |
| `root.Create("../escape.txt")` | blocked |
| `root.Create("/etc/passwd-ish")` | blocked |
| `root.Link("esc/x", "hard2.txt")` | blocked |

So `Root.Symlink` **does** exist and **does** allow creating a link whose
target escapes -- exactly the property #2275 needs -- while traversal through
that link to write a later entry is refused.

**Error value: there is no exported sentinel.** The escape error is
`errPathEscapes`, defined unexported at `$GOROOT/src/os/file.go:421`:

```go
var errPathEscapes = errors.New("path escapes from parent")
```

Measured wrapping: `*os.PathError{Op:"openat", Path:"b/pwned", Err:errors.errorString{"path escapes from parent"}}`
(and `Op:"statat"` for stats, `*os.LinkError{Op:"linkat", ...}` for hardlinks).
Critically:

```
errors.Is(err, os.ErrInvalid)   = false
errors.Is(err, fs.ErrNotExist)  = false
errors.Is(err, os.ErrPermission)= false
errors.Is(err, syscall.ENOTDIR) = false
errors.Is(err, syscall.ELOOP)   = false
errors.Is(err, syscall.EXDEV)   = false
```

Nothing matches. Detecting "this specific archive was malicious" for a nicer
error message requires `strings.Contains(err.Error(), "path escapes from
parent")`, which is fragile. **Recommendation: do not branch on the error
value.** Wrap it with the archive entry name and surface it as a hard failure;
the security property holds regardless of whether we can classify the error.

**Benign internal symlinks still work.** `link2dir -> realdir` followed by
`root.Create("link2dir/inside.txt")` succeeds, and the file lands in
`dest/realdir/`. Likewise `dest/opt/lib/x -> ../../bin/x` reads back fine
through `Root.ReadFile`. The rule is per-step containment, not "no symlinks."

**It re-checks on every call -- no caching, no TOCTOU window.** Experiment 7:
`Create("swap/ok.txt")` succeeds while `swap` is a real directory; `swap` is
then replaced out-of-band with a symlink to `outside`; the very next
`Create("swap/after.txt")` on the *same* `*os.Root` fails with `path escapes
from parent` and nothing lands in `outside/`. This is the property
`EvalSymlinks` cannot offer.

**`Root.MkdirAll` reports EEXIST, not the escape error, for an existing
escaping component.** This is a real ergonomics wrinkle found while building
the end-to-end proof:

```
MkdirAll("link2dir")     err=<nil>              # benign internal symlink: fine
MkdirAll("link2dir/sub") err=<nil>
MkdirAll("b")            err=mkdirat b: file exists          # <-- escaping symlink
MkdirAll("b/sub")        err=mkdirat b/sub: path escapes from parent
```

Because tsuku's loop calls `os.MkdirAll(filepath.Dir(target))` before writing,
a naive port surfaces `mkdirat b: file exists` for the #2473 archive rather
than an escape message. It is **still safe** (nothing is written), just
confusing. Benign internal symlinks are unaffected -- `MkdirAll("link2dir")`
returns nil because `Stat` succeeds through it. Fix is cosmetic: `Root.Lstat`
the parent first, or wrap with the entry name.

**End-to-end proof** (`mini/main.go`, a ~40-line extractor that routes every
write through `*os.Root` and deliberately does *no* link-content validation):

```
#2473 evil archive -> mkdir parent of b/pwned: mkdirat b: file exists
  contained: no base/pwned
#2275 bottle archive -> <nil>
  created link content = "../../../../../opt/python@3.14/bin/python3.14"
two-archive stage   -> <nil>
two-archive payload -> mkdir parent of x/authorized_keys: mkdirat x: file exists
  victim/authorized_keys = "ORIGINAL\n"
```

One mechanism, zero link-content policy: #2473 blocked, #2275 bottle created
verbatim, and the two-archive (repeat-extraction-into-same-dest) variant
blocked too.

**The one caveat for #2275.** `os.Root` blocks *following* an escaping link as
well as writing through one:

```
Root.Stat("relesc")            -> statat relesc: path escapes from parent
Root.ReadFile("d1/d2/reenter") -> openat ...: path escapes from parent
os.ReadFile(same path)         -> "real"     (plain os follows it fine)
```

Note that "exit and re-enter" is **not** special-cased: `../dest/real.txt`
from inside `dest` is rejected even though it lands back inside, because
resolution transiently leaves the root. This does not affect *extraction*
(which only creates links), but it means any later tsuku action that reads
through a bottle symlink must keep using plain `os`, not a `*os.Root` scoped
to the same dest. Worth confirming with whoever is covering bottle behavior.

### 3. `filepath.EvalSymlinks` -- resolve-then-check is not sound here

Measured failure modes against the live #2473 tree:

```
EvalSymlinks(dest/nonexistent/child) = ""  err=lstat .../dest/nonexistent: no such file or directory
EvalSymlinks(dest/relesc)            = ""  err=lstat .../outside/lib.so: no such file or directory   # dangling
EvalSymlinks(dest/b/pwned)           = ""  err=lstat .../pwned: no such file or directory
EvalSymlinks(dest/b)                 = "/tmp/rootexp-2824133137"   err=<nil>                          # dest's PARENT
```

Four concrete problems:

1. **Nonexistent final component errors out.** Every regular-file entry in an archive names a path that does not exist yet, so `EvalSymlinks(target)` always fails. You are forced into resolve-*parent*-then-check.
2. **Dangling links error out.** #2275 bottles are dangling by construction at extraction time -- `../../../../../opt/python@3.14/...` does not exist in the temp dir. So `EvalSymlinks` cannot even be used to *validate* the bottle link, let alone allow it. Distinguishing "dangling" from "escaping" means parsing the error's path, which is exactly the kind of string surgery that produced [CPython #107845](https://github.com/python/cpython/issues/107845).
3. **TOCTOU.** Resolve returns a `string`. Between the check and the `os.OpenFile`, any component can be swapped. `os.Root` closes this by holding dirfds; `EvalSymlinks` structurally cannot.
4. **Cost.** It `lstat`s every component of every path. On a deep archive that is O(entries x depth) extra stats -- worse than `os.Root`, which at least reuses the root fd.

Resolve-parent-then-check *is* sound only under assumptions tsuku cannot fully
guarantee: single-threaded extraction, into a directory no other process can
write, with the parent chain fully materialized and non-dangling at check
time, plus correct handling of the "parent does not exist yet" case. Since
extraction runs into `ctx.WorkDir` (a tsuku-managed temp dir), the TOCTOU risk
is *lower* than for a world-writable target -- but item 2 alone disqualifies
it, because the mechanism cannot express #2275 at all.

### 4. `securejoin` -- available, but the wrong semantics for an extractor

`github.com/cyphar/filepath-securejoin` is **not** in `go.sum` today (grep
returns 0 hits). It fetches cleanly: `go get` resolved **v0.7.0**. The repo has
**20 direct requires**; neither `CLAUDE.md` nor `CONTRIBUTING.md` states any
policy on adding Go dependencies, so there is no written bar to clear -- but
also no precedent forcing one.

The decisive finding is behavioral. **`SecureJoin` clamps; it does not
reject.** Measured against the live #2473 tree:

```
SecureJoin(dest, "b/pwned")         = "/tmp/.../dest/pwned"                      err=<nil>
SecureJoin(dest, "esc/pwned")       = "/tmp/.../dest/tmp/.../outside/pwned"      err=<nil>
SecureJoin(dest, "link2dir/ok.txt") = "/tmp/.../dest/realdir/ok.txt"             err=<nil>
SecureJoin(dest, "../escape")       = "/tmp/.../dest/escape"                     err=<nil>
SecureJoin(dest, "/etc/shadow")     = "/tmp/.../dest/etc/shadow"                 err=<nil>
SecureJoin(dest, "relesc")          = "/tmp/.../dest/outside/lib.so"             err=<nil>
```

Every single call returns `err=<nil>`. It is a chroot-style *scoping*
primitive: it rewrites the path to something safe. For an extractor that is
the wrong contract -- a malicious archive would silently produce
`dest/pwned` instead of failing loudly, and the recipe author would never know
their archive was tampered with. It also returns a `string`, so it inherits
the same TOCTOU gap as `EvalSymlinks`.

v0.7.0 does ship an openat2-backed handle API in the `pathrs-lite`
subpackage (`OpenInRoot`, `OpenatInRoot`, `MkdirAll`, `MkdirAllHandle`,
`Reopen`), which is TOCTOU-safe. But it uses `RESOLVE_IN_ROOT` (clamping), not
`RESOLVE_BENEATH` (rejecting):

```
pathrs.OpenInRoot(dest, "b/pwned")  err=... openat2 .../dest/b/pwned: no such file or directory
```

ENOENT, not an escape error -- it resolved `b -> a/.. ` back to `dest` and
found no `pwned` there. Same clamping semantics, so the same mismatch. And
`pathrs-lite` has no `Symlink`, `Chmod`, or `Chtimes`, so an extractor would
need a hybrid anyway.

**Verdict: adding the dependency buys nothing over `os.Root` and costs a
semantic mismatch.**

### 5. Prior CVEs and what upstream converged on

*(literature, with one measurement)*

- **GNU tar, [CVE-2025-45582](https://github.com/advisories/GHSA-f93m-9mq4-2fjj)** -- this is #2473, upstream. GNU tar **through 1.35** allowed a two-step attack: archive 1 contains `x -> ../../../../../home/victim/.ssh`; archive 2 contains `x/authorized_keys`; extracting both into the same directory overwrites the victim's keys, bypassing the "Member name contains `..`" check. Fixed by **backporting `openat2` to jail the extraction directory**.
- **GNU tar delayed links** *(2006, [LWN 211489](https://lwn.net/Articles/211489/))* -- the older, portable half of the defense: symlinks are not created during the main pass. My strace shows it directly: tar writes a mode-`000` placeholder regular file (`openat(AT_FDCWD, "esc", O_WRONLY|O_CREAT|O_EXCL, 000)`) and only calls `symlinkat(...)` at the end of extraction. A later member therefore cannot traverse a symlink from the same archive -- it hits the placeholder and gets `ENOTDIR`. This does not help across two archives, which is precisely why CVE-2025-45582 existed.
- **Python `tarfile`, [PEP 706](https://peps.python.org/pep-0706/)** -- added `filter=`, default flipped to `data` in 3.14. The `data` filter rejects absolute link targets (`AbsoluteLinkError`) and links resolving outside the destination (`LinkOutsideDestinationError`). **This is the rule that is too strict for #2275**, and it demonstrably broke real archives: [CPython #107845](https://github.com/python/cpython/issues/107845) reports `data_filter` wrongly rejecting valid tarballs because it resolved symlink `linkname` relative to the archive root instead of relative to the link's containing directory (hardlinks *are* archive-root-relative; symlinks are not). See also [python-build-standalone #953](https://github.com/astral-sh/python-build-standalone/issues/953), where `AbsoluteLinkError` broke dependency extraction. A cautionary tale for any link-content-based rule.
- **npm `tar`, [CVE-2021-37701](https://github.com/advisories/GHSA-9r2w-394v-53qc) / CVE-2021-32804** -- symlink protection bypassed via backslash path separators on POSIX and via absolute paths. The fix added a **symlink cache**: node-tar remembers every directory it created and refuses to write through a path component it knows is a symlink. Still a userspace bookkeeping approach, not a kernel one.
- **npm `tar-fs`, [CVE-2024-12905](https://github.com/advisories/GHSA-pq67-2wwv-3xjx)** -- the original fix missed the `onsymlink` callback path, so the bypass came back. The lesson: **link-content validation scattered across code paths keeps regressing**; a single choke point at the write syscall does not.
- **Docker `chrootarchive` / containerd** -- both moved extraction into a `chroot`/`pivot_root` (or, in containerd's `fs` package, `securejoin`-style resolution) so that the kernel, not the application, enforces the boundary.
- **git `core.symlinks`** -- git refuses to check out a path whose leading directory is a symlink, and on platforms without symlink support writes plain files instead. Same rule again.

**The rule everyone converged on:** *the link's own target is not the thing to
police -- the traversal is.* Absolute or escaping link content is routinely
legitimate (Homebrew bottles, Nix store paths, `/usr/lib` shims); what must
never happen is a **write that resolves through** such a link. Every project
that tried to enforce safety by inspecting `linkname` (PEP 706, node-tar's
early fixes) either broke legitimate archives, shipped a bypass, or both.
Every project that moved enforcement to the write syscall stopped having the
class of bug.

This maps onto tsuku exactly: `validateSymlinkTarget`
(`internal/actions/extract.go:39`) is a link-content rule, and it is
simultaneously *too weak* for #2473 (`b -> a/..` resolves lexically to `dest`
itself and passes) and *too strong* for #2275 (it rejects legitimate bottle
shims). Those are not two bugs; they are one wrong choice of enforcement
point.

### 6. What GNU tar actually does with the #2473 archive (measured)

System tar: **GNU tar 1.35**, Ubuntu package `1.35+dfsg-3ubuntu0.4`. No
`bsdtar`/libarchive on this machine, so that comparison is untested.

The Debian changelog is the interesting part -- this build is *patched*:

```
tar (1.35+dfsg-3ubuntu0.2) noble-security; urgency=medium
  * SECURITY UPDATE: File overwrite via directory traversal
    - debian/patches/CVE-2025-45582-*.patch: Backport openat2 support in
      order to jailify the extraction directory.
    - CVE-2025-45582
```

So an **unpatched GNU tar 1.35 is vulnerable to this exact archive**; what I
measured is post-fix behavior.

**Single-archive #2473 chain** (`a -> .`, `b -> a/..`, `b/pwned`):

```
a
b
b/pwned
tar: b/pwned: Cannot open: Not a directory
tar: Exiting with failure status due to previous errors
EXIT=2
```

Nothing escaped; `dest/` contains only the two symlinks. strace shows *both*
defenses firing:

```
openat(AT_FDCWD, "esc", O_WRONLY|O_CREAT|O_EXCL, 000) = 4        # delayed-link placeholder
openat2(AT_FDCWD, "esc/", {flags=...O_NOFOLLOW|O_PATH|O_DIRECTORY,
        resolve=RESOLVE_BENEATH}, 24) = -1 ENOTDIR (Not a directory)
symlinkat("/tmp/.../outside2", AT_FDCWD, "esc") = 0              # real symlink, created LAST
```

Note the last line: **tar happily creates the escaping symlink** -- absolute
target and all. It only refuses the traversal. That is the #2275-compatible
rule, implemented.

**Two-archive variant** (the literal CVE-2025-45582 shape: extract `x ->
../victim` from one archive, then `x/authorized_keys` from a second, into the
same dest). This isolates `openat2` from delayed links, since the symlink is
already real:

```
--- extract archive 1 (symlink only) ---
x
EXIT1=0
--- extract archive 2 (file through pre-existing symlink) ---
x/authorized_keys
tar: x/authorized_keys: Cannot open: Invalid cross-device link
tar: Exiting with failure status due to previous errors
EXIT2=2
--- victim file now: ---
ORIGINAL
```

`Invalid cross-device link` is `EXDEV`, which is what `openat2` returns when
`RESOLVE_BENEATH` resolution escapes. Confirms the patch, and confirms delayed
links alone are insufficient.

`--absolute-names` / `-P` disables the leading-`/` and `..` stripping; it is
not what defends here and is irrelevant to tsuku (we control the flags, and
Go's `archive/tar` gives us raw headers anyway).

### 7. Blast radius in this codebase

`internal/actions/extract.go` (462 lines) is not the only site. Files with a
`tar.NewReader` / `zip.NewReader` / `zip.OpenReader` extraction loop:

| File | Writes symlinks? | Notes |
|---|---|---|
| `internal/actions/extract.go` | yes (`:345-357`) | the #2473 site; `isPathWithinDirectory` `:21`, `validateSymlinkTarget` `:39` |
| `internal/actions/app_bundle.go` (502 ln) | yes (`:310`, `:401`, `:483`) | reuses `isPathWithinDirectory` at `:277` -- **same lexical flaw** |
| `internal/builders/homebrew.go` | handles `tar.TypeSymlink` at `:1736` | bottle path |
| `internal/builders/go.go`, `gem.go`, `pypi.go` | no symlink case seen | |
| `internal/llm/archive.go` (251 ln) | no symlink handling found | |

`extract.go` has two extraction loops (tar at `:291`, a second at `:406`) that
both build `target := filepath.Join(destPath, relativePath)` and both call
`isPathWithinDirectory`. Any fix needs to cover both, plus `app_bundle.go`,
or the same bug persists one file over.

### 8. Performance

`os.Root` costs one extra `readlinkat`-ish step per path component. Measured
on 4000 file creations at three nesting depths:

```
depth=1  plain=194.8ms  os.Root=219.2ms  ratio=1.13x  (48.7us vs 54.8us per file)
depth=4  plain=170.6ms  os.Root=212.1ms  ratio=1.24x  (42.6us vs 53.0us per file)
depth=8  plain=179.9ms  os.Root=256.7ms  ratio=1.43x  (45.0us vs 64.2us per file)
```

Worst case ~19us extra per file at depth 8. A 10k-entry archive pays ~0.2s --
noise against download and decompression. **Not a decision factor.**

## Mechanism comparison

Scores: **++** strong / **+** adequate / **~** conditional / **-** weak /
**--** disqualifying.

| Mechanism | Correct vs #2473 chain | Compatible with #2275 bottles | Go-version + dependency cost | Blast radius in this codebase |
|---|---|---|---|---|
| **`os.Root` (`os.OpenRoot`)** | **++** Measured: blocks single-archive chain, two-archive variant, `../`, absolute paths, hardlinks, *and* a live directory-swap race. Enforced per syscall against held dirfds; no cache to poison. | **++** `Root.Symlink` performs **zero** validation of link content -- absolute and escaping relative targets both created verbatim. Proven end-to-end with the exact `../../../../../opt/python@3.14/...` shim. Benign internal symlinks (`link2dir -> realdir`) still traversable. | **++** Zero. Stdlib since 1.24; repo floor is 1.25.8. `unix||windows||wasip1` build tag covers linux+darwin. No openat2/kernel dependency. | **+** Rewrite two loops in `extract.go` + `app_bundle.go` to `root.OpenFile`/`MkdirAll`/`Symlink`. Mostly mechanical. Wrinkles: no error sentinel (`errPathEscapes` is unexported -- don't branch on it), and `MkdirAll` over an escaping component reports EEXIST not the escape error (cosmetic). |
| **`EvalSymlinks` resolve-then-check** | **~** Catches #2473 *if* you resolve the parent and re-check every entry, and *if* nothing races. Structurally TOCTOU: returns a string, then you open non-atomically. | **--** **Disqualifying.** Bottle links are dangling at extraction time; `EvalSymlinks` returns `lstat .../opt/python@3.14/...: no such file or directory`. Cannot distinguish dangling from escaping without parsing error paths -- the exact bug in CPython #107845. | **++** Zero. Pure stdlib, any Go version. | **~** Small diff, but adds subtle per-entry logic to 2+ loops, and O(entries x depth) extra `lstat`s. High chance of a future regression. |
| **`securejoin` (v0.7.0)** | **-** `SecureJoin` returns `err=<nil>` for *every* attack path -- it **clamps** rather than rejects (`b/pwned` -> `dest/pwned`, `/etc/shadow` -> `dest/etc/shadow`). Safe-by-rewriting, which silently mangles filenames instead of failing on a tampered archive. `pathrs-lite.OpenInRoot` is TOCTOU-safe but uses `RESOLVE_IN_ROOT` -- same clamping semantics. | **+** Would allow the bottle link (it does not police link content either), but only because it does not reject anything. | **-** New direct dependency (repo has 20; no stated policy either way). Fetches fine. `pathrs-lite` lacks `Symlink`/`Chmod`/`Chtimes`, so a hybrid with plain `os` is unavoidable. | **-** Comparable diff to `os.Root` plus a dependency plus a hybrid, for strictly worse semantics. |
| **Two-pass validation (scan headers, then extract)** | **-** Defeats the *known* shape but is a lexical simulation of the filesystem. Must model symlink resolution, `..`, `strip_dirs`, pre-existing dest contents, and hardlinks. Every project that tried this (node-tar's symlink cache, PEP 706's `data` filter) shipped a bypass or broke valid archives. Also cannot see state a *second* extraction into the same dest created -- the CVE-2025-45582 gap. | **~** Expressible in principle (allow-list bottle-shaped targets), but that is precisely the fragile link-content rule the whole industry backed away from. | **++** Zero deps. | **--** Largest and most delicate diff: a full symlink-resolution simulator to write, test, and keep correct in 2+ loops forever. |
| **Extract to staging, then verify** | **~** Catches escapes *after* the write already happened. A malicious archive has already overwritten `~/.ssh/authorized_keys` by the time you look; you can only detect, not prevent. Only sound if staging is on a filesystem the attack cannot reach out of, which symlinks defeat by definition. | **+** Neutral -- does not inspect link content. | **++** Zero deps. | **-** Extra copy of every extracted tree (I/O cost, disk), plus the verify pass, plus a cleanup path. And it does not actually prevent the vulnerability. |

**Recommendation: `os.Root`.** It is the only candidate that is strong in all
four columns, it is the mechanism GNU tar's own CVE-2025-45582 fix converged
on (jail the destination, let the kernel enforce), and it happens to resolve
#2473 and #2275 with the *same* change -- because it enforces on traversal and
is silent about link content, which is exactly the separation the design needs.

## Implications

1. **The two issues have one fix.** #2473 and #2275 are the too-weak and too-strong halves of a single wrong decision: policing `linkname` instead of policing the write. Switching to `os.Root` and **deleting `validateSymlinkTarget` entirely** closes #2473 and opens #2275 in one change. That is a stronger result than the design brief anticipated -- no rule is needed to "distinguish the link's own target from the traversal," because `os.Root` never conflates them in the first place.

2. **#2275's stated acceptance criterion "absolute symlink targets (e.g. `/etc/passwd`) are still rejected" is now a policy choice, not a security requirement.** GNU tar creates such links freely (measured: `symlinkat("/tmp/.../outside2", ...)` succeeded). With `os.Root`, an absolute link inside the extraction tree cannot be used to escape *during extraction*. Keeping the absolute-target rejection is defensible as defense-in-depth for what happens to that tree *later* (it gets copied into `~/.tsuku/tools/`), but it should be a deliberate, documented decision rather than load-bearing security.

3. **`app_bundle.go` has the same bug** (`isPathWithinDirectory` at `:277` guarding `os.Symlink` at `:310`/`:401`/`:483`). Scoping the fix to `extract.go` leaves an equivalent traversal reachable through the macOS app-bundle path.

4. **Don't branch on the error.** `errPathEscapes` is unexported and matches no `errors.Is` sentinel. Wrap with the offending archive entry name and fail. Also pre-`Lstat` the parent in the `MkdirAll` path, or the #2473 archive reports `mkdirat b: file exists`, which will confuse whoever triages the resulting bug report.

5. **Scope `os.Root` to writes only.** `Root.Stat`/`ReadFile` refuse to *follow* escaping links, including exit-and-re-enter targets. If a later action reads through a bottle shim using a `*os.Root` on the same dest, #2275 breaks at read time instead of extract time.

## Surprises

- **The system `tar` on this machine was patched for this exact CVE five weeks ago** (`1.35+dfsg-3ubuntu0.2`, 27 June 2026). Unpatched GNU tar 1.35 -- still the version in many images -- is vulnerable to the #2473 archive. tsuku is not an outlier; it is in the same cohort tar was in until very recently.

- **`os.Root` is not openat2.** I expected a `RESOLVE_BENEATH` wrapper. It is a portable userspace component walk with dirfd pinning and a restart-from-root strategy for `..` (255-step / 8-restart budget, then `ENAMETOOLONG`). This is *better* for tsuku than openat2 would be: it works identically on darwin, where openat2 does not exist.

- **`SecureJoin` returned `err=<nil>` for all seven attack paths I threw at it.** It is a sanitizer, not a validator. Anyone reaching for it as a drop-in "reject bad paths" function -- a natural assumption from the name -- would ship code that silently writes `b/pwned` to `dest/pwned`. Worth calling out explicitly in the design doc so it isn't proposed later.

- **Exit-and-re-enter is rejected, not allowed.** `Root.Stat("../dest/real.txt")` from inside `dest` fails even though the target is inside `dest`. Resolution is judged step by step, not on the endpoint. Irrelevant for extraction, but it invalidates the intuition that "re-entering links are fine."

- **`Root.MkdirAll` masks the escape error with EEXIST**, and it was the *only* error my end-to-end extractor produced for the #2473 archive -- the clean `path escapes from parent` message never appeared.

## Open Questions

1. Does anything downstream of extraction (`homebrew_relocate`, `install_binaries`, `chmod`, the copy into `~/.tsuku/tools/`) *follow* bottle symlinks? If a future refactor scopes those to a `*os.Root`, #2275 regresses at read time. Belongs with whoever owns bottle behavior.
2. Keep or drop the absolute-link-target rejection? #2275 asks to keep it, and it is cheap, but it is no longer security-critical and it is exactly the rule that broke `python-build-standalone`. Needs a decision.
3. Should `extract.go`'s second extraction loop (`:406`) and `app_bundle.go` land in the same PR, or does splitting risk shipping a half-fix?
4. `zip` archives: does tsuku's zip path create symlinks at all (zip stores them as entries with a symlink mode bit), and is it reachable from a recipe? Not covered here.
5. Hardlink entries (`tar.TypeLink`) are not handled by the current loop at all -- entries are silently dropped. `Root.Link` exists and is escape-safe; is silent-drop the intended behavior, or a separate gap?
6. No `bsdtar`/libarchive on this machine, so the libarchive comparison point is untested.

Note: `internal/actions/zz_throwaway_symlink_probe_test.go` is untracked in the
worktree and is not mine -- it appears to belong to another agent's probe. I
left it in place.

## Summary

tsuku's minimum Go is 1.25.8 (`go.mod`; the two workflows that name older Go still switch toolchains via `GOTOOLCHAIN=auto`), so stdlib `os.Root` is available at zero dependency cost, and measurement confirms it blocks the #2473 chain, the two-archive CVE-2025-45582 variant, and a live directory-swap race, while `Root.Symlink` creates escaping and absolute link targets verbatim -- meaning #2473 and #2275 are fixed by the same change, with `validateSymlinkTarget` deleted rather than rewritten. This is the rule GNU tar, Docker, containerd, and git all converged on -- police the traversal, not the link content -- and the projects that instead validated `linkname` (PEP 706's `data` filter, node-tar, tar-fs) each shipped either a bypass or broken legitimate archives; the alternatives here fail concretely, since `EvalSymlinks` cannot handle the dangling bottle links #2275 needs and `securejoin` returned `err=<nil>` for every attack path because it clamps rather than rejects. The biggest open question is blast radius rather than mechanism: `app_bundle.go` carries the identical lexical flaw at `internal/actions/app_bundle.go:277`, and it is unconfirmed whether any post-extraction action follows bottle symlinks in a way that would regress #2275 if it too were scoped to a root.
