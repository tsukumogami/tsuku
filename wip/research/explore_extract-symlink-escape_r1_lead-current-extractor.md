# Lead: How does archive extraction actually work today in tsuku, end to end?

Repo checkout: `/home/dgazineu/dev/niwaw/tsuku/tsuku+extract_symlink_escape-179b07a4/public/tsuku/.claude/worktrees/extract-symlink-escape`
Go toolchain: `go 1.25.8` (`go.mod:3`) — `os.Root` with `OpenRoot`/`Root.OpenFile`/`Root.Mkdir`/`Root.Symlink` is available and currently unused anywhere in the repo.

## Findings

### 1. `internal/actions/extract.go` — the entry loop

The whole action is one file, 463 lines. There is exactly **one** tar entry loop
(`extractTarReader`, `internal/actions/extract.go:282-363`) shared by every tar-based format,
and one zip loop (`extractZip`, `internal/actions/extract.go:390-462`).

`Execute` (`extract.go:85-167`) resolves params, then dispatches on format
(`extract.go:149-166`) to a thin per-compression wrapper — `extractTarGz` (193),
`extractTarXz` (210), `extractTarBz2` (226), `extractTarZst` (238), `extractTarLz` (255),
`extractTar` (271) — each of which opens the file, wraps a decompressor, and hands
`tar.NewReader(...)` to `extractTarReader`. So **all six tar variants share the same entry
loop and the same security properties**. `extractZip` (390) is separate.

Order of operations per tar entry (`extract.go:291-359`):

1. `header, err := tr.Next()` (292)
2. `cleanPath := strings.TrimPrefix(header.Name, "./")` (301) — only strips a single leading `./`
3. strip_dirs: `parts := strings.Split(cleanPath, "/")`; skip if `len(parts) <= stripDirs`; `parts = parts[stripDirs:]`; `relativePath := filepath.Join(parts...)` (304-309). `filepath.Join` implies a `Clean`, so `foo/../../bar` collapses to `../bar` here.
4. optional `files` allow-list filter on the *post-strip* relative path (312-314)
5. `target := filepath.Join(destPath, relativePath)` (316) — second `Clean`
6. **containment check**: `if !isPathWithinDirectory(target, destPath) { return error }` (319-321)
7. `switch header.Typeflag` (323):
   - `tar.TypeDir` (324): `os.MkdirAll(target, 0755)` — hardcoded 0755, header mode ignored
   - `tar.TypeReg` (329): `os.MkdirAll(filepath.Dir(target), 0755)`, then
     `os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))`, `io.Copy`, `Close`
   - `tar.TypeSymlink` (345): `validateSymlinkTarget(header.Linkname, target, destPath)` →
     `os.MkdirAll(filepath.Dir(target), 0755)` → `atomicSymlink(header.Linkname, target)`
   - **default: nothing.** No case for `tar.TypeLink` (hardlink), `TypeChar`, `TypeBlock`,
     `TypeFifo`, `TypeXGlobalHeader`, `TypeGNUSparse`, GNU long-name types, etc. They are
     silently dropped with no error and no log line. (`TypeRegA` is normalized to `TypeReg`/`TypeDir`
     by Go's `archive/tar` reader before we see it, so old-format tars still work.)

**What happens between validation and the write: nothing.** No `Lstat` of `target`, no
`O_NOFOLLOW`, no `O_EXCL`, no re-resolution, no directory-handle-relative I/O. The check at
line 319 is purely a string operation on a path that has never touched the filesystem; the
syscalls at 325/330/334/351/356 then resolve that same string through the *real* filesystem,
where intermediate components may be symlinks. That gap is the whole bug.

`isPathWithinDirectory` (`extract.go:21-35`) is `filepath.Abs` on both sides plus
`absTarget == absBase || strings.HasPrefix(absTarget, absBase+sep)`. `filepath.Abs` calls
`Clean` but **never** resolves symlinks (no `EvalSymlinks`), and never touches the filesystem.

`validateSymlinkTarget` (`extract.go:39-55`) rejects absolute link targets, then computes
`resolvedTarget := filepath.Join(filepath.Dir(linkLocation), linkTarget)` and runs the same
lexical containment check. "Resolved" is a misnomer — it is `Clean(dir + "/" + target)`, so
any `..` in the link target is cancelled against the *lexical* parent, not the real one.

`atomicSymlink` (`extract.go:368-387`) writes to `linkPath + ".tmp"` then `os.Rename`s over
`linkPath`. `rename(2)` does not follow a symlink at the final component, so this correctly
replaces an existing symlink at `target`. It does **not** protect intermediate components, and
the `.tmp` path is predictable (a known nit, not the issue at hand).

### 2. The escape, reproduced against this code

I wrote a throwaway test in `internal/actions/` (since the helpers are package-private), ran it,
and deleted it. The archive was three entries:

```
a          symlink -> "."
b          symlink -> "a/.."
b/pwned    regular file, content "OWNED"
```

Result: `extractTarGz` returned `nil` — **no error at all** — and `OWNED` was written to
`<destPath>/../pwned`.

Why each entry passes:

- `a -> "."`: `Join(dest, ".")` = `dest`, and `isPathWithinDirectory(dest, dest)` is true (the `==` arm at line 34).
- `b -> "a/.."`: `Join(dest, "a/..")` lexically cancels to `dest` → passes. On disk, `b` now resolves through `a` (which *is* `dest`) and then `..` → the parent of `dest`.
- `b/pwned`: `Join(dest, "b/pwned")` is lexically inside `dest` → passes line 319. `MkdirAll(Dir(target))` and `OpenFile(target, O_CREATE|...)` both follow `b` → write lands outside.

A second variant of the same primitive: once `b` exists as an escaping symlink, a plain
**regular-file entry named `b`** also escapes, because `os.OpenFile` at line 334 has no
`O_NOFOLLOW` and follows the final component. And a **directory entry** under `b` escapes via
`MkdirAll` at 325. So the sink is not just "file written through a staged symlink" — it is every
one of the three filesystem operations in the switch.

Also confirmed in the same run: a `tar.TypeLink` (hardlink) entry pointing at an absolute path
outside the dest, and a `tar.TypeFifo` entry, are both **silently skipped** — extraction returns
`nil` and the dest directory is empty. Safe, but silent: a legitimate archive that uses hardlinks
loses files with no diagnostic.

### 3. Every extraction / decompression path in the repo

Filesystem-**writing** extractors (these are the security surface):

| Path | Writes | Symlinks? | Notes |
|---|---|---|---|
| `internal/actions/extract.go:282` `extractTarReader` | yes | creates them | tar / tar.gz / tar.xz / tar.bz2 / tar.zst / tar.lz. **Vulnerable.** |
| `internal/actions/extract.go:390` `extractZip` | yes | **no symlink branch** | A zip symlink entry becomes a regular file whose content is the link text (`os.OpenFile` at 445 ignores `ModeSymlink` in the perm arg). So zip cannot *stage* the symlink — but it happily *traverses* one staged by an earlier tar extract into the same dir. |
| `internal/actions/app_bundle.go:262` `extractZIP` | yes | **creates them** (294-313) | Independent zip extractor for macOS `.app` bundles. Calls the *same* `isPathWithinDirectory` (277) and `validateSymlinkTarget` (306), uses plain `os.Symlink` (310) and `os.OpenFile(..., O_CREATE\|O_WRONLY\|O_TRUNC, file.Mode())` (322). **Same two-hop escape applies**, and unlike `extract.go` it can stage the symlink from a zip. Any fix must cover this file. |
| `internal/actions/app_bundle.go:342` `extractDMG` | yes | n/a | shells out to `hdiutil` + copy; macOS only. |

External `tar` binary invocations (bypass all of the above):

- `internal/actions/cargo_build.go:414` — `exec.CommandContext(ctx, "tar", "xzf", crateTarball, "-C", extractDir)`
- `internal/actions/cargo_install.go:318` — identical

These rely entirely on the host `tar`'s own protections. GNU tar and bsdtar both refuse to
extract *through* a symlink (they unlink it first) and reject `..` members, so these are not the
same bug, but they are a consistency gap: two different containment policies in one codebase.

Read-only archive inspection (no filesystem writes, no traversal risk):

- `internal/llm/archive.go` — `listTarGz` (150), `listTarXz` (167), `listTarReader` (183), `listZip` (222). Listing only; also caps download at 10 MB (`MaxArchiveSize`, line 20).
- `internal/builders/gem.go:312` `extractTarEntry` — reads one named entry into memory.
- `internal/builders/homebrew.go:1710` `extractBottleContents` — enumerates `bin/`/`lib/`/`include/` names.
- `internal/builders/go.go:277`, `internal/builders/pypi.go:348` — in-memory `zip.NewReader` scans.

No nested/recursive extraction anywhere: `extract` never feeds its own output back into itself.

### 4. All callers of the two validators, and same-shaped checks elsewhere

`isPathWithinDirectory` (defined `extract.go:21`) — non-test callers:
- `extract.go:319` (tar loop)
- `extract.go:425` (zip loop)
- `app_bundle.go:277` (app-bundle zip loop)

`validateSymlinkTarget` (package-level, `extract.go:39`) — non-test callers:
- `extract.go:347` (tar symlink)
- `app_bundle.go:306` (app-bundle zip symlink)

Two *unrelated* methods share the name (different receivers, different semantics — worth
renaming for clarity if this area is touched):
- `(*InstallLibrariesAction).validateSymlinkTarget` — `internal/actions/install_libraries.go:140`, called from `install_libraries.go:101`. Same lexical shape: `filepath.Clean(filepath.Join(symlinkDir, target))` then `strings.HasPrefix(resolvedTarget, cleanInstallDir+sep)` (line 159). Copies symlinks out of `WorkDir` into `InstallDir` — and `WorkDir` is exactly where a malicious archive just landed, so this is a *downstream* consumer of attacker-planted symlinks.
- `(*LinkDependenciesAction).validateSymlinkTarget` — `internal/actions/link_dependencies.go:219`, called from `link_dependencies.go:168`. Weaker still: rejects absolute targets and any target *containing* the substring `..`, with no containment computation.

Other lexical containment helpers with the same "string math instead of filesystem truth" shape:
- `internal/actions/set_rpath.go:379` `validatePathWithinDir` — `filepath.Rel` + `..` prefix check
- `internal/actions/install_program_files.go:206` `isWithin` — `filepath.Rel` + `..` check, **but** used correctly: line 138 calls `filepath.EvalSymlinks(src)` *first* and checks the resolved path, then opens with `O_RDONLY|O_NOFOLLOW` (147) and writes with `O_EXCL|O_NOFOLLOW` (177), deriving the mode from the source rather than trusting the archive header (161-165). **This file is the in-repo precedent for what "actually enforced" looks like** and its comments (145-146, 172-174) already articulate the TOCTOU argument.
- `internal/install/checksum.go:224` `isWithinDir`
- `internal/install/symlink.go:63`, `internal/actions/homebrew_relocate.go:1390`, `internal/actions/set_rpath.go:435` — `HasPrefix` containment on already-`EvalSymlinks`-resolved paths (these are fine).

### 5. Existing tests

- `internal/actions/extract_test.go` (873 lines)
  - `TestIsPathWithinDirectory` (14), `TestValidateSymlinkTarget` (71) — pure unit tables on the helpers
  - `TestExtractAction_ExtractTarGz` (223), `_StripDirs` (277), `TestExtractAction_ExtractZip` (325)
  - `TestAtomicSymlink` (372)
  - `TestExtractTar_PathTraversal_SecurityEdgeCases` (413) — 9 cases, all **single-entry** archives with hostile *names*
  - `TestExtractZip_PathTraversal_SecurityEdgeCases` (494) — same shape for zip
  - `TestExtractTar_SymlinkAttacks_SecurityEdgeCases` (568) — 9 cases, each archive has exactly **one** symlink plus one regular `target.txt`. Includes `{"cyclic a->b", "a", "b", false}` — i.e. the suite explicitly asserts that a symlink pointing at a sibling name is *allowed*, which is the exact building block of the escape.
  - `TestIsPathWithinDirectory_SameDir/_Parent/_PartialMatch` (660-684), `TestValidateSymlinkTarget_Absolute/_Escape/_Valid` (687-711)
  - **Helper for in-memory tar.gz: `buildTarGz(t, files map[string]string)` at line 849** — regular files only, no header type control, and `map` iteration means **entry order is nondeterministic**. It cannot express the attack (which needs ordered, typed entries), so a multi-entry test will need a new helper.
- `internal/actions/extract_formats_test.go` (730 lines) — per-format coverage. Helpers: `createTarArchive(t, dirName, fileName, content)` at line 20 (single entry, tar only, no gzip) and `testTarExtraction(...)` at line 42 (write archive → extract → assert one file). Also `TestExtractAction_Execute_WithDest` (343), `_WithOSArchMapping` (394), zip strip-dirs (446) and file-filter (488) tests.
- `internal/actions/app_bundle_test.go` exists but I did not find symlink-escape cases in it.

**The structural gap: every security test in the suite extracts an archive with one hostile
entry.** The vulnerability requires three ordered entries. No existing helper builds one.

### 6. `ExtractAction` as an action

- Type: `type ExtractAction struct{ BaseAction }` (`extract.go:58`). Registered at `internal/actions/action.go:206`.
- `IsDeterministic() bool { return true }` (61). `Dependencies()` and `RequiresNetwork()` are **inherited from `BaseAction`** (`action.go:175`, `action.go:179`) and return the zero `ActionDeps{}` / `false` — extract declares no dependencies, so no `tar`/`unzip` binary is required or checked for.
- `Preflight` (69-75) only checks that `archive` is present. Notably it does **not** check `format`, which `Execute` then requires at line 113.
- Params: `archive` (required), `format` (required, or `auto` → `detectFormat`, `extract.go:170-190`), `dest` (optional, default `.`), `strip_dirs` (optional, default 0), `files` (optional allow-list), plus `os_mapping`/`arch_mapping` for filename substitution (96-107).
- `destPath = filepath.Join(ctx.WorkDir, ExpandVars(dest, vars))` (`extract.go:129`). There is **no containment check on `destPath` itself** — a recipe with `dest = "../.."` would extract outside `WorkDir` entirely. That is blocked one layer up at recipe-validation time by `validatePathParam` (`internal/recipe/validator.go:661-676`, wired via `validatePathParams` at 563 with `pathParams = ["dest","archive","binary","src","path"]`), which errors on any `..` — but it *skips any value containing `{`* (line 663), so a template-bearing `dest` is unchecked. Recipe authors are a different trust tier from archive publishers, so this is a lower-priority gap, but worth noting.

**What `destPath` actually is at runtime.** The single production `ExecutionContext` constructor is
`internal/executor/executor.go:548` (and the dependency variant at 890), fed from
`Executor.workDir`/`installDir` set in `New` (`executor.go:71-84`):

```go
workDir, err := os.MkdirTemp("", "action-validator-*")   // executor.go:71
installDir := filepath.Join(workDir, ".install")          // executor.go:77
```

So `destPath` defaults to a fresh `/tmp/action-validator-XXXXXX` — a temp dir, **not** the tool
dir. Escaping it one level lands in `/tmp`; escaping N levels reaches anything the invoking user
can write: `~/.tsuku/bin`, `~/.bashrc`, `~/.ssh/authorized_keys`, `~/.config/`. `installDir` is
*inside* `workDir` (`workDir/.install`), so an escaping archive can also write directly into the
staging install dir. `internal/sandbox/executor.go:357` and `internal/validate/executor.go:240`
set `WorkDir: "/workspace"` inside a container — same code, contained blast radius.

**Does anything extract into a directory that already contains attacker-influenced content? Yes,
routinely.** `WorkDir` is shared across all steps of an install, and several composites run
download-then-extract into it repeatedly:

- `internal/actions/composites.go:272` (`download_archive`) — extracts into `WorkDir`, then **`CopyDirectory(ctx.WorkDir, ctx.InstallDir)`** at 280
- `internal/actions/composites.go:612` (`github_archive`) — extract → chmod → `install_binaries`
- `internal/actions/homebrew.go:115` — Homebrew bottle, `strip_dirs: 2`, followed by `homebrew_relocate`
- `internal/actions/fossil_archive.go:93` — source tarball, later built in place

A recipe with two `extract` steps into the same `WorkDir` gets the cross-archive version of the
same bug for free. And `CopyDirectory` (`internal/actions/utils.go:20-62`) preserves symlinks
verbatim via `CopySymlink` (65-89) with **no containment validation at all**, so an escaping
symlink planted in `WorkDir` is faithfully reproduced inside `~/.tsuku/tools/<tool>-<ver>/` and
persists after install.

### 7. strip_dirs / files / rename interactions

- `strip_dirs` is applied **before** the containment check (`extract.go:304-309`), on the raw
  split components. Because `filepath.Join(parts...)` cleans, `strip_dirs` can only ever make a
  path shallower or turn it into `..`-prefixed garbage that line 319 then rejects. I did not find
  a way to use it to bypass the check.
- `strip_dirs` counts `/`-split components of the *original* name, so a `./`-prefixed entry that
  survives `TrimPrefix` differently than expected can shift alignment — cosmetic, not a security issue.
- `files` is an allow-list matched against the post-strip relative path (312). It is **not** a
  mitigation: a filtered-out entry is skipped, but the attacker controls the archive and simply
  names the payload to match. (It does mean a `files`-constrained recipe is harder to attack, since
  every entry in the three-entry chain would need to be listed.)
- There is no per-entry rename/`dest` mapping — `install_binaries` does the src→dest renaming later,
  from `InstallDir`, not during extraction.
- `header.Mode` is passed straight to `os.OpenFile` (334). Go's `os.FileMode` is not a Unix mode,
  so raw setuid/setgid bits from the tar header land in undefined FileMode bits and get dropped by
  `syscallMode` — setuid is **not** propagated today. Compare `install_program_files.go:161-165`,
  which deliberately derives the mode instead of trusting the header.

## Implications

1. **The fix belongs at the syscall layer, not in a smarter string check.** No amount of lexical
   cleverness closes this, because the attacker controls the *filesystem state* between validation
   and write, not just the path string. There is no lexical predicate over `b/pwned` that
   distinguishes "`b` is a real directory" from "`b` is a symlink to `..`". The guarantee to aim
   for is: *every filesystem operation the extractor performs resolves within `destPath`, as
   enforced by the kernel* — which means directory-handle-relative I/O.
2. **`os.Root` (Go 1.24+, and the module already targets 1.25.8) is the natural mechanism.**
   `os.OpenRoot(destPath)` then `root.OpenFile`, `root.Mkdir`, `root.Symlink` (Go 1.25) refuse to
   traverse a symlink that escapes the root, enforced per path component by the runtime. That
   collapses `isPathWithinDirectory`, `validateSymlinkTarget`, and `atomicSymlink` into "let the
   root handle refuse it," and it fixes the regular-file and directory sinks at the same time —
   not just the symlink one. Keeping the lexical checks as a cheap pre-filter is fine (better error
   messages), but they must stop being the enforcement.
3. **Any fix must cover `app_bundle.go:262` too.** It is a second, independently-written extractor
   sharing the same two broken helpers, and it is *more* exposed than `extract.go`'s zip path
   because it actually creates symlinks from zip entries.
4. **`destPath` is a temp dir, and `installDir` lives inside it.** So the escape is not
   "corrupt this tool's install" — it is arbitrary write as the invoking user. Combined with
   `~/.tsuku/bin` being on `PATH`, that is straightforwardly RCE-on-next-shell. The severity
   argument does not need embellishment.
5. **Downstream consumers matter for the containment story.** Even with extraction fully hardened,
   `CopyDirectory` (`utils.go:20`) copies symlinks out of `WorkDir` into the permanent install dir
   with no validation, and `install_libraries.go:140` validates them only lexically. If the fix's
   stated guarantee is "nothing outside the destination is ever written or linked to," those two
   are in scope; if it is narrowly "extraction cannot escape," they are follow-ups. Worth deciding
   explicitly rather than by omission.
6. **Unhandled tar entry types are a decision, not an oversight to preserve.** Hardlinks are
   currently dropped silently. Under `os.Root` (which has `Link` in Go 1.25) they could be handled
   safely, or explicitly rejected with an error. Silent data loss is the worst of the three options.
7. **Test infrastructure needs a new multi-entry helper.** `buildTarGz` (`extract_test.go:849`)
   takes a `map`, so it is both untyped and order-nondeterministic. A regression test for this bug
   needs ordered `[]struct{name, typeflag, linkname, content}`. Ideally one shared helper serves
   `extract_test.go`, `extract_formats_test.go`, and `app_bundle_test.go`.

## Surprises

- **The escape returns `nil`.** I expected a partial write plus an error. Extraction reports
  complete success, so nothing downstream — not the executor, not telemetry, not the user — has any
  signal that something went wrong.
- **The existing symlink security test explicitly blesses the attack primitive.**
  `extract_test.go:589`: `{"cyclic a->b", "a", "b", false}` asserts that a symlink to a sibling
  name must *not* error. That is correct in isolation and is exactly step one of the exploit — a
  good illustration that per-entry validation cannot see a multi-entry attack.
- **`internal/actions/install_program_files.go` already solved this problem correctly** (lines
  138-180: `EvalSymlinks` + containment + `O_NOFOLLOW` + `O_EXCL` + derived mode), with comments
  that spell out the TOCTOU reasoning. The hardening pattern exists in-tree; it just never reached
  the extractor. That is a strong argument for one shared safe-write primitive rather than a
  point fix.
- **Two extractors, not one.** `app_bundle.go` has its own zip loop that creates symlinks. Reading
  only `extract.go` would leave half the bug live.
- **Two different containment policies coexist**: the Go extractors, and `exec.Command("tar", "xzf", ...)`
  in `cargo_build.go:414` / `cargo_install.go:318` which inherit whatever the host `tar` enforces.
- **`ExtractAction.extractZip` silently mangles zip symlinks** into regular files containing the
  target text. Accidentally safe, but it means zip archives with symlinks extract *incorrectly* —
  a latent correctness bug independent of security.
- **`validatePathParam` skips any value containing `{`** (`internal/recipe/validator.go:663`), so
  the `..` guard on `dest` does not apply to templated values.

## Open Questions

- **Scope**: does the fix cover only `extract.go`, or also `app_bundle.go`'s extractor,
  `CopyDirectory`/`CopySymlink`, and `install_libraries.go`? My read is that the containment
  *guarantee* should be stated once and enforced by one shared primitive, but that is a design call.
- **`os.Root` behavior on the paths tsuku uses**: `os.Root` refuses to traverse *any* symlink whose
  resolution leaves the root — including ones that stay inside. Do real recipes (Homebrew bottles
  especially, and Node.js-style directory installs) contain intra-archive symlinks that are
  legitimately traversed by *later* entries in the same archive? If yes, a naive `os.Root` swap
  could break real installs and needs a compatibility pass over the 18 recipes using `extract`
  plus the bottle path. This is the main thing I would test before committing to the approach.
- **Hardlinks**: reject loudly, or support via `Root.Link`? Currently silently dropped, so
  "support them" is a behavior change either way.
- **`destPath` containment**: should `Execute` verify that `filepath.Join(WorkDir, dest)` stays
  within `WorkDir`? Cheap to add; closes the templated-`dest` hole at `validator.go:663`.
- **Windows**: `strings.Split(cleanPath, "/")` (line 304) never splits on `\`, and `os.Root`
  semantics differ there. Is Windows in scope at all?
- Does `homebrew_relocate` (which runs immediately after the bottle extract) rewrite anything
  through symlinks in `WorkDir`? I did not audit it.

## Note on repo state

`git status` in the worktree shows `wip/` and an untracked
`internal/actions/zz_throwaway_symlink_probe_test.go`. The latter is **not mine** — presumably a
sibling research agent working in the same worktree. My own scratch test was created in
`internal/actions/` (the helpers are package-private so an external harness cannot reach them),
run once, and deleted.

## Summary

`extract`'s containment is enforced entirely by string math — `isPathWithinDirectory`
(`extract.go:21`) and `validateSymlinkTarget` (`extract.go:39`) never touch the filesystem, and
nothing sits between validation and the `MkdirAll`/`OpenFile`/`Symlink` calls that follow symlinks
through the real one; I reproduced the three-entry escape against the shared tar loop
(`extract.go:282`) and it wrote outside the destination while returning `nil`. The guarantee has to
move from lexical prediction to kernel-enforced, directory-handle-relative I/O — `os.Root`, which
the module's Go 1.25.8 already provides — applied to both extractors (`extract.go` *and*
`app_bundle.go:262`, which independently creates symlinks from zip entries and shares the same
broken helpers), with `internal/actions/install_program_files.go:138-180` as the in-tree precedent
for what enforcement looks like. The biggest open question is compatibility: `os.Root` refuses to
traverse symlinks during path resolution, so before committing to it we need to know whether real
archives — Homebrew bottles especially — rely on later entries resolving through symlinks staged by
earlier ones.
