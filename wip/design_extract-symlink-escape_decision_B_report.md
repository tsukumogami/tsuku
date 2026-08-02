# Decision B: What is the exact integration surface of the os.Root conversion, and how are escape errors surfaced to users?

All line numbers are as of `HEAD` (`git show HEAD:internal/actions/extract.go`). Two
sibling agents transiently modified `extract.go` and `app_bundle.go` in this shared
worktree during the investigation; nothing here was measured against those edits, and
all scratch code lives under `/home/dgazineu/.claude/jobs/1013267d/tmp/`.

Toolchain: `go1.25.8`, `golangci-lint 2.12.2`.

## Options

**Scope of the conversion**

- **B1. `extract.go` tar loop only.** Minimal diff, leaves the zip loop and
  `app_bundle.go` on the broken helpers.
- **B2. Both `extract.go` loops + `app_bundle.go`'s `extractZIP`.** Every extractor that
  writes archive-controlled paths.
- **B3. B2 plus `copyDir` / `extractDMG` / `CopyDirectory`.** Full containment guarantee
  across the install pipeline.

**Error surface**

- **(a)** Wrap every `os.Root` call with the archive entry name.
- **(b)** Keep the existing lexical pre-check purely for its better message; let `os.Root`
  be the enforcement.
- **(c)** Branch on the error string.

## Evidence

### 1. Which code paths convert

| Path (HEAD) | Attacker-controlled content? | Reach | Conversion cost | Verdict |
|---|---|---|---|---|
| `extract.go:282` `extractTarReader` | Yes | All six tar formats; 18 recipes use `extract` directly, 111 use the `download_archive` / `github_archive` composites, plus the Homebrew bottle path (`internal/actions/homebrew.go:115`, `strip_dirs: 2`) | ~15 lines | **Convert** |
| `extract.go:390` `extractZip` | Yes | Same action, `format = "zip"` | ~12 lines | **Convert** |
| `app_bundle.go:262` `extractZIP` | Yes | macOS-only at runtime (`Execute` returns early when `ctx.OS != "darwin"`, `app_bundle.go:71`); exactly **one** recipe uses it (`recipes/i/iterm2.toml`). The function itself is portable Go and unit-testable on Linux. | ~12 lines **plus a mandatory `.Perm()` fix** (see §2e) | **Convert** |
| `app_bundle.go:349` `extractDMG` | No | Copies from an `hdiutil`-mounted read-only image, not from archive-controlled path strings | n/a | Out of scope |
| `app_bundle.go:459` `copyDir` | Partly | Called at `:163` on the extracted `.app` and at `:413` from `extractDMG` | n/a | **Out of scope, and it does not share the traversal defect** |

**`copyDir` does not share the #2473 defect.** It drives `filepath.Walk(src)`, and `Walk`
does not follow symlinks. For `a/b` to be enumerated, `a` must be a real directory in
`src`, so `src` can never contain both `a` as an escaping symlink and `a/b` as a walked
entry — the two-hop chain is unreachable. `copyDir` also `RemoveAll`s `dst` first
(`:461-466`), so it never writes into a pre-populated tree. What it *does* do is
faithfully recreate an escaping symlink (`:483 os.Symlink(linkTarget, dstPath)`) inside
`~/.tsuku/apps/<name>.app`. That is persistence of a bad link, not an arbitrary write —
the same class as `CopyDirectory` in `internal/actions/utils.go`. Note for whoever picks
it up later: `copyDir:474` does `os.MkdirAll(dstPath, info.Mode())` with `ModeDir` set,
so it would need the same `.Perm()` mask as `app_bundle.go:282`.

`atomicSymlink` (`extract.go:368`) is called from **three** sites, two of which are outside
any extraction root:

```
HEAD:internal/actions/app_bundle.go:195:  atomicSymlink(targetPath, symlinkPath)      // ctx.CurrentDir
HEAD:internal/actions/app_bundle.go:218:  atomicSymlink(destPath, applicationSymlink) // ~/Applications
HEAD:internal/actions/extract.go:356:     atomicSymlink(header.Linkname, target)
```

So `atomicSymlink` must stay as-is; the conversion adds a sibling `atomicSymlinkAt(root *os.Root, ...)`.

### 2. Mechanical shape of the conversion

Concrete before/after for the tar loop (`extract.go:282-363`):

```go
// BEFORE
func (a *ExtractAction) extractTarReader(tr *tar.Reader, destPath string, stripDirs int, files []string) error {
	fileFilter := ...
	for {
		header, err := tr.Next()
		...
		relativePath := filepath.Join(parts...)                      // :309
		if len(fileFilter) > 0 && !fileFilter[relativePath] { continue }
		target := filepath.Join(destPath, relativePath)              // :316
		if !isPathWithinDirectory(target, destPath) { ... }          // :319
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)                                // :325
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)                  // :330
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode)) // :334
		case tar.TypeSymlink:
			validateSymlinkTarget(header.Linkname, target, destPath) // :347
			os.MkdirAll(filepath.Dir(target), 0755)                  // :351
			atomicSymlink(header.Linkname, target)                   // :356
		}
	}
}

// AFTER
func (a *ExtractAction) extractTarReader(tr *tar.Reader, destPath string, stripDirs int, files []string) error {
	fileFilter := ...

	// os.OpenRoot requires the directory to exist; the loop used to create it lazily.
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	root, err := os.OpenRoot(destPath)
	if err != nil {
		return fmt.Errorf("failed to open destination directory: %w", err)
	}
	defer root.Close()

	for {
		header, err := tr.Next()
		...
		relativePath := filepath.Join(parts...)
		if len(fileFilter) > 0 && !fileFilter[relativePath] { continue }

		// filepath.Join returns "" when every part is empty -- entries naming the
		// destination itself ("./", or "pkg/1.0/" at strip_dirs=2). os.Root rejects
		// the empty path; "." is the same location and is accepted.
		// MUST come after the filter so `files` matching is unchanged.
		if relativePath == "" {
			relativePath = "."
		}

		// Cheap lexical pre-filter, kept only for its better message.
		if !isPathWithinDirectory(filepath.Join(destPath, relativePath), destPath) {
			return fmt.Errorf("archive entry escapes destination directory: %s", header.Name)
		}

		// os.Root rejects perm bits outside 0o777; the syscall layer already dropped
		// them, so masking is behavior-preserving.
		perm := os.FileMode(header.Mode).Perm()

		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(relativePath, 0755); err != nil {
				return wrapEntry(header.Name, "failed to create directory", err)
			}
		case tar.TypeReg:
			if err := root.MkdirAll(filepath.Dir(relativePath), 0755); err != nil {
				return wrapEntry(header.Name, "failed to create parent directory", err)
			}
			f, err := root.OpenFile(relativePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, perm)
			...
		case tar.TypeSymlink:
			if err := validateSymlinkTarget(header.Linkname, filepath.Join(destPath, relativePath), destPath); err != nil {
				return err
			}
			if err := root.MkdirAll(filepath.Dir(relativePath), 0755); err != nil {
				return wrapEntry(header.Name, "failed to create parent directory", err)
			}
			if err := atomicSymlinkAt(root, header.Linkname, relativePath); err != nil {
				return wrapEntry(header.Name, "failed to create symlink", err)
			}
		}
	}
	return nil
}

func atomicSymlinkAt(root *os.Root, target, linkPath string) error {
	tmpLink := linkPath + ".tmp"
	_ = root.Remove(tmpLink)
	if err := root.Symlink(target, tmpLink); err != nil {
		return err
	}
	if err := root.Rename(tmpLink, linkPath); err != nil {
		_ = root.Remove(tmpLink)
		return err
	}
	return nil
}

// wrapEntry attaches the archive entry name to an os.Root error. os.Root exports no
// sentinel for its escape error and its message names only the root-relative path.
func wrapEntry(entryName, what string, err error) error {
	return fmt.Errorf("archive entry %q: %s: %w", entryName, what, err)
}
```

#### a) Where does `os.MkdirAll(destPath)` go, and does anything rely on lazy creation?

`os.OpenRoot` requires the directory to exist:

```
os.OpenRoot(<nonexistent>)   -> open /tmp/probe-750599257/nope: no such file or directory
os.OpenRoot(<regular file>)  -> open /tmp/probe-750599257/afile: not a directory
os.OpenRoot(<symlink to dir>) -> nil
```

So the `MkdirAll` is hoisted above `OpenRoot`, before the loop. Today `destPath` is created
lazily by whichever entry lands first (`os.MkdirAll(target)` at `:325` or
`os.MkdirAll(filepath.Dir(target))` at `:330`). Measured consequence:

```
current, empty archive:  destPath exists? false
current, one file entry: destPath created by MkdirAll(Dir)? true
converted, empty archive: destPath exists? true   <- ONLY behavior delta
```

An empty archive now leaves an empty `destPath` behind. Benign. Nothing else relies on
lazy creation; the default `dest = "."` resolves to `ctx.WorkDir`, which always exists.
`OpenRoot` on a symlinked `dest` still works, so a symlinked destination behaves as today.

#### b) Is `relativePath` always safe to pass to `Root` methods?

**No — and this is the highest-value finding.** `filepath.Join` returns `""` when every
part is empty, and `os.Root` rejects the empty path:

```
root.MkdirAll("",  0755) -> mkdirat : empty path
root.OpenFile("", ...)   -> openat : empty path
root.MkdirAll(".", 0755) -> nil
```

The current path math produces `""` routinely:

```
  name="./"             strip=0 -> rel=""               dir(rel)="."
  name="pkg/"           strip=1 -> rel=""               dir(rel)="."
  name="node-v26.5.0-linux-x64/" strip=1 -> rel=""
  name="brotli/1.2.0/"  strip=2 -> rel=""
```

Scanned across the 19 real archives in the local cache (105,334 entries):

```
relativePath=="" @strip=0   : 0
relativePath=="" @strip=1   : 3    <-- would break Root (empty path)
relativePath=="" @strip=2   : 21   <-- would break Root (empty path)

empty relativePath examples:
  brotli.tar.gz: name="brotli/1.2.0/" strip=2 type=5
  curl.tar.gz: name="curl/8.18.0/" strip=2 type=5
  libcap.tar.gz: name="libcap/2.78/" strip=2 type=5
  node-v26.5.0-linux-x64.tar.gz: name="node-v26.5.0-linux-x64/" strip=1 type=5
  my-test-package.zip: name="my_test_package.egg-info/" strip=1 (zip)
```

That is *every* Homebrew bottle (`strip_dirs: 2`) and *every* GitHub-tarball-shaped
archive at `strip_dirs: 1` — 71 recipes declare `strip_dirs = 1`, 3 declare
`strip_dirs = 3`, plus the whole Homebrew builder path. Without the `"" -> "."`
normalization the conversion breaks essentially every stripped extraction with
`mkdirat : empty path`.

The normalization **must go after the `files` filter** so filter matching is unchanged
(verified: `files=[""]` and `files=["."]` produce identical empty trees before and after).

Other shapes are safe:

- **`..` inside the path**: `os.Root` rejects it (`mkdirat ..: path escapes from parent`),
  and `filepath.Join` has already collapsed the in-root case
  (`a/b/../c.txt` -> `a/c.txt`, identical in both implementations).
- **absolute entry names**: `strings.Split("/etc/passwd", "/")` -> `["", "etc", "passwd"]`,
  `Join` -> `"etc/passwd"`. Both implementations write `dest/etc/passwd`. Unchanged.
- **`relativePath == "."`**: `root.MkdirAll(".")` -> nil; `root.OpenFile(".")` ->
  `openat .: is a directory`, matching the current `open <dest>: is a directory`.

#### c) Does `Root.MkdirAll(".")` work? — **Tested, yes.**

```
root.MkdirAll(".", 0755)   -> nil
root.MkdirAll("./", 0755)  -> nil
root.Mkdir(".", 0755)      -> mkdirat .: file exists   (Mkdir, not MkdirAll -- we use MkdirAll)
root.MkdirAll("", 0755)    -> mkdirat : empty path
```

`filepath.Dir(".")` is `"."` and `filepath.Dir("foo")` is `"."`, so the parent-directory
call reduces to `root.MkdirAll(".")` for every top-level entry. That is the common case
and it works.

#### d) `Root.Symlink` / `Root.Rename` in Go 1.25.8 — both exist. Does the dance still buy anything?

Both are present (`go doc os.Root`), and both are root-confined:

```
r3.Symlink("old", "d/l")                                  -> nil
r3.Symlink("new", "d/l.tmp")                              -> nil
r3.Rename("d/l.tmp", "d/l")   [replace existing symlink]  -> nil
    d/l -> "new"
r3.Symlink("newer", "d/l")    [no rename dance]           -> symlinkat newer d/l: file exists
r3.Rename("tmpl", "sb/escaped")  [escaping rename target] -> renameat tmpl sb/escaped: path escapes from parent
r3.Symlink("x", "sb/escaped2")   [escaping symlink loc]   -> symlinkat x sb/escaped2: path escapes from parent
    nothing created outside root                          -> good
```

**Yes, keep it — but for a different reason than the comment claims.** `Root.Symlink`
returns `EEXIST` when the name is taken, so a bare `root.Symlink` would fail on duplicate
symlink entries and on re-extraction into a populated `WorkDir`. The temp-then-rename
dance is what makes symlink creation idempotent. The TOCTOU justification in the current
comment (`extract.go:366-367`) is now delivered by `os.Root` itself and the comment should
be rewritten to say "idempotent overwrite" rather than "prevents TOCTOU".

Also confirmed (and consistent with Decision A): `Root.Symlink` does *not* validate
`oldname`. `root.Symlink("../../etc/passwd", ...)` and `root.Symlink("/etc/passwd", ...)`
both return `nil`. So `validateSymlinkTarget` remains the only link-content policy layer.

#### e) Does `Root.OpenFile` honor `perm` the same way? — **No. This is the second regression.**

`Root.OpenFile` and `Root.MkdirAll` **return an error** if `perm` has any bit outside the
nine least-significant:

```
root.OpenFile("m1", ..., 0755)                          -> nil
root.OpenFile("m2", ..., FileMode(0o100644)) [st_mode]  -> openat m2: unsupported file mode
root.OpenFile("m3", ..., FileMode(0o4755))   [setuid]   -> openat m3: unsupported file mode
root.OpenFile("m4", ..., FileMode(0o2755))   [setgid]   -> openat m4: unsupported file mode
root.OpenFile("m5", ..., FileMode(0o1777))   [sticky]   -> openat m5: unsupported file mode
root.MkdirAll("d1", FileMode(0o40755))       [st_mode]  -> mkdirat d1: unsupported file mode

os.OpenFile(..., FileMode(0o100644)) [current]          -> nil
  resulting mode on disk                                -> -rw-r--r--
```

**For tar, `header.Mode` is still the right source and `.Perm()` is exactly
behavior-preserving.** `header.Mode` is the raw octal from the tar header, so it can carry
setuid/setgid/sticky (`0o7000`) or a full `st_mode` (`0o100755`). `os.FileMode` places its
named bits at `1<<19` and above, which a `< 1<<17` tar mode can never reach, so
`syscallMode` already drops everything but `Perm()`. Confirmed end-to-end in the
differential run:

```
--- full st_mode in header (0o100755) ---   current: F tool 0755 | os.Root: F tool 0755
--- setuid bit in header (0o4755)   ---     current: F tool 0755 | os.Root: F tool 0755
```

Corpus check: `header.Mode > 0o777` occurred **0 times** in 105,334 tar entries, so this is
a latent hazard rather than an observed one — but it costs one method call to close.

**For zip it is a hard break, and `app_bundle.go:282` is the worst case.** `f.Mode()` is a
real `os.FileMode` with type bits set. Measured against a constructed zip:

```
entry          f.Mode()      | os.OpenFile/MkdirAll (current) | Root.OpenFile/MkdirAll
"d/"           drwxr-xr-x    | <nil>                          | mkdirat root_d/: unsupported file mode
"d/f.txt"      -rw-r--r--    | <nil>                          | <nil>
"d/link"       Lrwxrwxrwx    | <nil>                          | openat root_d/link: unsupported file mode
"d/setuid"     urwxr-xr-x    | <nil>                          | openat root_d/setuid: unsupported file mode

Same but with .Perm() masking: all four -> <nil>
```

And confirmed against a real zip in the repo:

```
zip entries with mode bits > 0o777:
  my-test-package.zip: "my_test_package.egg-info/" mode=drwxrwxr-x isdir=true
```

`app_bundle.go:282` is `os.MkdirAll(targetPath, file.Mode())`. Every zip directory entry
carries `ModeDir`, so without `.Perm()` `extractZIP` would fail on essentially every zip.
`extract.go:445` is `os.OpenFile(target, ..., f.Mode())`, which sees `ModeSymlink` on zip
symlink entries (the flatten-to-text-file behavior noted in the exploration findings).

One genuine behavior change falls out: for zip entries the current code **actually sets**
setuid (`urwxr-xr-x` on disk); `.Perm()` drops it to `0755`. Since tsuku runs unprivileged
this is harmless in practice and strictly an improvement, but it should be stated rather
than discovered.

#### f) `defer root.Close()` across the loop

Fine. One `Root` per extraction, reused for the whole loop:

```
3 iterations on one Root -> ok; Close() -> nil
use after Close: r3.MkdirAll("late", 0755) -> mkdirat late: file already closed
double Close                               -> nil
```

**Lint caveat, measured against the repo's own `.golangci.yaml` with golangci-lint 2.12.2.**
`errcheck` excludes `(*os.File).Close` and `os.Remove` *by name*; `(*os.Root).*` matches
neither those entries nor the `std-error-handling` preset:

```
a.go:7:13:  Error return value of `root.Remove` is not checked (errcheck)
a.go:12:14: Error return value of `root.Remove` is not checked (errcheck)
a.go:23:18: Error return value of `root.Close` is not checked (errcheck)
   (os.Remove on the following line: not flagged)
```

Two fixes, both verified clean:

1. `_ = root.Remove(...)` at each site, `defer func() { _ = root.Close() }()`.
2. Add `(*os.Root).Close` and `(*os.Root).Remove` to `errcheck.exclude-functions` in
   `.golangci.yaml`, next to the existing `(*os.File).Close`.

Option 2 matches how the repo already handles `(*os.File).Close` and `os.Remove` and keeps
the extractor code clean. Two smaller notes from the same run: `misspell` (US locale)
rejects "behaviour" in comments, and **`dupl` did not fire** with both converted loops
(tar and zip) in the same package — the 250-token threshold flagged in the exploration
findings is not tripped by this change.

### 3. Error surface

**There is nothing to match on.** In Go 1.25.8:

```
GOROOT/src/os/file.go:421: var errPathEscapes = errors.New("path escapes from parent")
```

Unexported, wrapped in an `*fs.PathError` whose `.Err` is a bare `*errors.errorString`:

```
error type: *fs.PathError
PathError{Op:"openat", Path:"b/pwned", Err:path escapes from parent}  Err type *errors.errorString
errors.Is(err, os.ErrNotExist)=false ErrPermission=false ErrInvalid=false
```

`Path` is the *root-relative* path, which equals the archive entry name only when
`strip_dirs = 0` and there is no `./` prefix — so the entry name genuinely has to come
from us.

**The message the user sees depends on which call trips first, and for the canonical
#2473 chain it is not "path escapes from parent".** With `a -> "."` and `b -> "a/.."`
staged, every variant is blocked (`escaped=no` in all cases) but the wording varies:

```
entry "b/pwned" (regular file)      -> archive entry "b/pwned": failed to create parent directory: mkdirat b: file exists
entry "b/sub/" (directory)          -> archive entry "b/sub/": failed to create directory: mkdirat b/sub: path escapes from parent
entry "b/" (directory)              -> archive entry "b/": failed to create directory: mkdirat b: file exists
entry "b/l" (symlink)               -> archive entry "b/l": failed to create parent directory: mkdirat b: file exists
entry "b" (regular, overwrite link) -> archive entry "b": failed to create file: openat b: path escapes from parent
entry "c/pwned" (deeper chain)      -> archive entry "c/pwned": failed to create parent directory: mkdirat c: file exists
entry "s/pwned" (abs-target link)   -> archive entry "s/pwned": failed to create parent directory: mkdirat s: file exists
```

The `file exists` variants come from `Root.MkdirAll`: `mkdirat` returns `EEXIST`, MkdirAll
re-stats the path through the root, that stat itself escapes, and MkdirAll reports the
generic "already exists" outcome. Enforcement is correct in every case; only the wording
degrades. (For a legitimate in-root symlink-to-directory `MkdirAll` returns `nil` — see §4.)

The lexical pre-check still produces the crisper message for the lexical class:

```
current:  archive entry escapes destination directory: ../evil.txt
os.Root:  archive entry "../evil.txt": failed to create parent directory: mkdirat ..: path escapes from parent
```

**Option (c) is not viable**, and not only for the usual reason. The string is an
unexported implementation detail with no compatibility promise — but more concretely, it
*would not fire for the canonical #2473 archive at all*, because the message surfaced
there is `file exists`. String-matching would silently misclassify the exact attack the
fix exists to stop.

**Measured refinement, deliberately not recommended for this PR.** Making the parent
`MkdirAll` lazy — try `OpenFile` first, `MkdirAll(Dir)` only on `ENOENT`, then retry —
yields the good message consistently and skips a syscall on the common path:

```
lazy  "b/pwned"      -> openat b/pwned: path escapes from parent
lazy  "x/y/z.txt"    -> <nil>   (x/y/z.txt created correctly via the ENOENT retry)
lazy  "../evil.txt"  -> openat ../evil.txt: path escapes from parent
```

It is a real improvement, but it changes the error semantics of every non-escape failure
too. That does not belong in a security fix; file it as a follow-up.

### 4. Behavior changes for currently-working archives

Differential harness: verbatim copy of the HEAD loop versus the converted loop, run over
**19 real archives at `strip_dirs` 0, 1 and 2** (57 runs; the largest archive is 67,277
entries), comparing full trees — names, permissions, symlink targets, SHA-256 content
hashes — plus the parent directory to catch escapes.

```
brotli.tar.gz      strip=0 entries cur=44     new=44     IDENTICAL
brotli.tar.gz      strip=2 entries cur=42     new=42     IDENTICAL
curl.tar.gz        strip=0 entries cur=569    new=569    IDENTICAL
openssl@3.tar.gz   strip=0 entries cur=7650   new=7650   IDENTICAL
node-v26.5.0-linux-x64.tar.gz  strip=1 entries cur=5718  new=5718  IDENTICAL
cpython-3.10.20+20260728-...   strip=1 entries cur=5215  new=5215  IDENTICAL
rust-1.97.1-x86_64-unknown-linux-gnu.tar.gz strip=0 entries cur=67277 new=67277 IDENTICAL
rust-1.97.1-x86_64-unknown-linux-gnu.tar.gz strip=1 entries cur=67276 new=67276 IDENTICAL
   ... 57/57 IDENTICAL, zero parent-tree differences
```

The corpus carries 13,434 symlink entries, so in-root symlink traversal is exercised
heavily rather than assumed.

The full enumeration of cases where the converted code could return an error the current
code does not:

| # | Case | Outcome | Status |
|---|---|---|---|
| 1 | `relativePath == ""` (`./`, stripped top-level dir) | `mkdirat : empty path` | **Would break every `strip_dirs` recipe and every Homebrew bottle.** Fixed by the `"" -> "."` normalization. Verified identical after. |
| 2 | `perm` bits outside `0o777` | `unsupported file mode` | **Would break every zip with directory entries** (`app_bundle.go:282`) and any tar with setuid/full-`st_mode`. Fixed by `.Perm()`. |
| 3 | Empty archive | `destPath` now created | Intentional, benign. The only remaining delta. |
| 4 | Zip setuid entries | setuid dropped (`urwxr-xr-x` -> `-rwxr-xr-x`) | Intentional improvement; declare it. |
| 5 | Parent dir is an **in-root symlink to a directory** (bottle `bin -> libexec/bin`) | `root.MkdirAll("bin")` -> **nil**, file lands at `libexec/bin/tool` | **No regression.** Also verified for a relative climb (`sub/man -> ../share/man`). |
| 6 | Parent is a **dangling** in-root symlink | Root: `mkdirat dangle: file exists`; current: `mkdir .../dangle2: file exists` | Both error today. Same outcome. |
| 7 | Parent is a **regular file** | Root: `file exists`; current: `not a directory` | Both error today. Wording differs. |
| 8 | Duplicate entries (file, dir, symlink) | Identical trees | Files `O_TRUNC` over; dirs idempotent; symlinks replaced by the rename dance. No `O_EXCL`, per Decision A. |
| 9 | In-entry `..` that stays inside (`a/b/../c.txt`) | Identical (`a/c.txt`) | Already collapsed by `filepath.Join`. |
| 10 | In-entry `..` that escapes (`../evil.txt`) | Identical error, from the retained lexical pre-check | Unchanged. |
| 11 | Absolute entry name (`/etc/passwd`) | Identical (`dest/etc/passwd`) | `Split`/`Join` already relativizes. |
| 12 | Zero-length entry name | Both error `is a directory` | Wording differs. |
| 13 | `files` filter (`["bin/tool"]`, `[""]`, `["."]`) | Identical | Requires the `"" -> "."` normalization to sit **after** the filter. |
| 14 | Dangling relative symlink (bottle style, `../../../opt/py/bin/py`) | Identical; link created, target resolves once written | The #2275 polarity is preserved. |

Not covered: macOS bottles with symlinks that climb above `destPath` (unclosable without
macOS hardware, and neutralized by the design — link-content policy is untouched).
Windows is out of scope: there are no `*_windows.go` files in `internal/actions` and no
Windows job in `.github/workflows`.

## Recommendation

**Scope: B2.** Convert `extract.go:282` `extractTarReader`, `extract.go:390` `extractZip`,
and `app_bundle.go:262` `extractZIP`. These are the three loops that write
archive-controlled path strings. Leave `copyDir`, `extractDMG` and `CopyDirectory` out —
`copyDir` provably cannot traverse a symlink it created, and its actual defect (faithfully
reproducing an escaping link into the install tree) is a different, lesser problem that
deserves its own issue rather than a rider on a security fix.

**Error surface: (a) and (b) together, not either/or.** Keep the lexical pre-check for its
crisp message on the lexical class, and wrap every `os.Root` call with the archive entry
name. Reject (c).

Three things the conversion must not omit, in priority order:

1. **`if relativePath == "" { relativePath = "." }`, placed after the `files` filter.**
   Without it every `strip_dirs` recipe and every Homebrew bottle fails with
   `mkdirat : empty path`.
2. **`.Perm()` on every mode passed to a `Root` method** — `os.FileMode(header.Mode).Perm()`
   in the tar loop, `f.Mode().Perm()` in both zip loops. Without it `app_bundle.go`'s
   extractor fails on any zip containing directory entries.
3. **A new `atomicSymlinkAt(root, ...)` beside the existing `atomicSymlink`**, which two
   `app_bundle.go` call sites still need for absolute paths outside any root.

Exact text a user sees for the #2473 archive:

```
archive entry "b/pwned": failed to create parent directory: mkdirat b: file exists
```

## Consequences

- The lexical helpers stay, demoted from "the guarantee" to "a fast pre-filter with a
  better message". `validateSymlinkTarget` keeps its role as link-content policy, since
  `Root.Symlink` explicitly does not validate `oldname`.
- Three behavior deltas ship and should be named in the PR body: an empty archive now
  creates `destPath`; zip entries lose the setuid bit; and error text for a handful of
  already-failing edge cases changes wording.
- The comment on `atomicSymlink` needs rewriting — the dance now buys idempotent overwrite,
  not TOCTOU protection, which `os.Root` supplies.
- `.golangci.yaml` needs `(*os.Root).Close` and `(*os.Root).Remove` added to
  `errcheck.exclude-functions`, or `_ =` at each call site. `dupl` does not fire.
- The `"" -> "."` normalization is load-bearing and non-obvious. It needs a comment
  explaining *why* (`filepath.Join` returns `""` for all-empty parts) and a regression test
  over a bottle-shaped archive at `strip_dirs: 2`, or the next refactor will delete it.
- Error messages for the escape are correct but sometimes read `file exists` rather than
  `path escapes from parent`. Acceptable; the entry name is the actionable part. A
  follow-up can make the parent `MkdirAll` lazy for consistently better text.

## Summary

Convert three loops — `extract.go:282` `extractTarReader`, `extract.go:390` `extractZip`, and `app_bundle.go:262` `extractZIP`; leave `copyDir` out, since `filepath.Walk` never follows symlinks in the source, so it provably cannot write through a link it created.

The conversion has two non-obvious mandatory pieces, both measured: `filepath.Join(parts...)` returns `""` for stripped top-level directory entries (21 occurrences at `strip_dirs=2`, 3 at `strip_dirs=1` across the real 19-archive corpus — every Homebrew bottle and every node-style tarball), and `os.Root` rejects the empty path, so `"" -> "."` normalization after the `files` filter is required or every `strip_dirs` recipe breaks; and `Root.OpenFile`/`MkdirAll` **error** on perm bits outside `0o777`, so every mode needs `.Perm()` — without it `app_bundle.go:282` fails on any zip containing directory entries, since `f.Mode()` carries `ModeDir`. With both in place a differential run over 19 real archives at `strip_dirs` 0/1/2 produced byte-identical trees in all 57 runs, while the #2473 chain is blocked with nothing written outside.

For errors, keep the lexical pre-check for its crisper message and wrap every `Root` call with the entry name; `errPathEscapes` is unexported and string-matching would not even fire for the canonical chain, which surfaces as `archive entry "b/pwned": failed to create parent directory: mkdirat b: file exists`.
