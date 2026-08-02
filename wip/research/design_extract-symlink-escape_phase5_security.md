# Phase 5 Security Review

Adversarial review of `docs/designs/DESIGN-extract-symlink-escape.md`. The mandate was to
break the design, not confirm it. Every attack below was executed against a faithful
prototype of the proposed loop under Go 1.25.8; the output shown is real.

## Attacks attempted

### A1 -- Entry types the loop does not handle

The tar loop handles `TypeDir`, `TypeReg`, `TypeSymlink` and silently skips everything
else, including `TypeLink` (hardlink), device nodes, and FIFOs.

```
Root.Link("src", "dst")              -> <nil>
Root.Link("../etc/passwd", "dst2")   -> linkat ../etc/passwd dst2: path escapes from parent
```

`os.Root` does have `Link`, and it is contained. The silent skip of `TypeLink` is
pre-existing and is not made worse by this change: skipping writes nothing. Should the
loop ever grow hardlink support, `Root.Link` is the safe primitive and is already contained.

**Not a vulnerability. Informational.**

### A2 -- `destPath` is itself a symlink

```
OpenRoot on a symlinked destPath: <nil>
  write inside:    <nil>
  escape attempt:  openat ../escaped: path escapes from parent
```

`os.OpenRoot` follows symlinks to *establish* the root, then confines everything relative
to wherever it landed. A symlinked `destPath` is therefore not itself an escape -- writes
stay under the link's target. Benign in isolation.

But it is the mechanism for A3.

### A3 -- Sequential extraction: an archive plants the symlink a later `dest` resolves through

**This is a real escape and the design as written does not stop it.**

`extract.go:129` computes `destPath = filepath.Join(ctx.WorkDir, dest)` and the design then
calls `os.OpenRoot(destPath)`. Because `OpenRoot` resolves the path it is given, a symlink
planted at that location by an *earlier* archive redirects the root itself.

```
archive #1 plants sub -> ../OUTSIDE:   <nil>       (creating links is unrestricted, by design)
naive os.OpenRoot(work/sub):           <nil>
  write:                               <nil>
escaped to OUTSIDE?                    true
```

The attack needs no malicious recipe. A recipe that extracts archive #1 into the work
directory and then extracts archive #2 into a subdirectory of it -- an ordinary two-step
recipe -- is enough. The archive supplies the symlink; the recipe innocently supplies a
`dest` that happens to name it. This is squarely archive-controlled, so the design's
"recipe-controlled `dest` is out of scope" carve-out does not cover it.

It is the same class of bug as #2473 itself, one level up: containment was established
against a path that was predicted rather than resolved.

**Verified fix.** Open a root on `ctx.WorkDir` and derive the destination root *through*
it, so the destination is resolved under kernel enforcement too:

```
rw.MkdirAll("sub")    -> mkdirat sub: file exists
rw.OpenRoot("sub")    -> openat sub: path escapes from parent
```

Checked against every `dest` shape the action accepts:

| `dest` | `MkdirAll` | `OpenRoot` | write | escape attempt |
|---|---|---|---|---|
| `.` (the default) | nil | nil | nil | `path escapes from parent` |
| `sub` | nil | nil | nil | `path escapes from parent` |
| `a/b/c` | nil | nil | nil | `path escapes from parent` |
| `./x` | nil | nil | nil | `path escapes from parent` |
| `/tmp` (absolute) | -- | `path escapes from parent` | -- | -- |

The absolute-`dest` row is a bonus: a recipe can no longer point `dest` outside the work
directory at all, which closes the carve-out the design had explicitly accepted.

### A4 -- Can the retained lexical layer reject something the kernel allows?

The pre-check runs on `target` (the lexically joined absolute path) and rejects only what
`filepath.Clean` can already see escaping. Since `Clean` is purely textual and the entry
names it rejects would also fail under `os.Root`, it cannot reject a path the kernel would
have permitted. It is a strict subset. Control flow never *skips* a `Root` call on the
strength of the pre-check passing -- the `Root` call is unconditional -- so there is no
"trusted pre-check" bypass.

**Holds.**

### A5 -- Does `.Perm()` silently drop setuid/setgid/sticky?

The design mandates `.Perm()` because `Root.OpenFile` rejects modes outside `0o777`:

```
OpenFile perm=urwxr-xr-x (040000755): openat: unsupported file mode
OpenFile perm=trwxrwxrwx (04000777):  openat: unsupported file mode
OpenFile perm=-rw-r--r-- (0644):      <nil>
```

The question is whether `.Perm()` changes behavior. It does not:

```
tar header.Mode = 04755 (decimal 2541)
os.FileMode(header.Mode) = -rwxr-xr-x, .Perm() = -rwxr-xr-x, IsSetuid=false
current code os.OpenFile(...,m) -> mode on disk: -rwxr-xr-x (setuid preserved: false)
new code root.OpenFile(...,m.Perm()) -> mode on disk: -rwxr-xr-x
```

Tar stores the setuid bit in the unix layout (`04000`), while Go's `os.ModeSetuid` is
`1<<23`. `os.FileMode(header.Mode)` therefore never sets Go's setuid bit, so the *current*
code already drops setuid. `.Perm()` is behavior-preserving, not a regression.

**Holds. Worth stating in the design so a reviewer does not have to re-derive it.**

### A6 -- Escaping symlinks are still created; what consumes them afterward?

By design, a symlink whose target leaves the destination is still created (that is the
policy layer's business, and #2275 wants it). The containment win is at extraction; it does
not extend to actions that later walk the extracted tree.

This is a genuine limitation rather than a flaw in the mechanism, and the design already
states it under "What it does not guarantee". It bounds the claim correctly: extraction
cannot be made to write outside; it does not promise the same of every downstream consumer.

**Not a defect in this change. The scope statement is accurate.**

### A7 -- Symlink loops and pathological chains

```
write through symlink loop: openat l1/x: too many levels of symbolic links
```

`ELOOP` surfaces as an ordinary error and the extraction fails cleanly. No hang, no
unbounded recursion. Chain depth is bounded by the kernel rather than by the extractor.

**Holds.**

### A8 -- Platform coverage

`os.Root` carries a `unix||windows||wasip1` build tag, covering both platforms tsuku
supports. No `openat2`/kernel-version dependency, so no runtime fallback path is needed.

**Holds.**

## Findings

| Severity | Finding | Evidence | Recommended action | In scope |
|---|---|---|---|---|
| **High** | `os.OpenRoot(destPath)` resolves `destPath` itself, so an archive that plants a symlink where a later `extract` step's `dest` points redirects the whole root. Same bug class as #2473, one level up. | A3 -- file landed in `OUTSIDE/` with an innocent recipe | Derive the destination root through a root on `ctx.WorkDir`: `workRoot.OpenRoot(dest)`. Verified to reject. | **Yes -- required** |
| Informational | Absolute `dest` is refused for free by the A3 fix, closing a carve-out the design had accepted. | A3 fix table, `/tmp` row | Note it; delete the "recipe-controlled `dest` is out of scope" caveat. | Yes |
| Informational | `.Perm()` is behavior-preserving; tar's setuid bit was already being dropped before this change. | A5 | State it in the design so it does not read as a silent behavior change. | Yes |
| Informational | `TypeLink` and device/FIFO entries are silently skipped, pre-existing. `Root.Link` exists and is contained if support is ever added. | A1 | Leave as-is; note for a future issue. | No |
| Low | Escaping symlinks are still created and downstream actions may follow them. | A6 | Already bounded correctly in the design's scope statement. | No |

## Verdict

**DESIGN HOLDS WITH REQUIRED CHANGES.**

One required change: derive the destination root through a root anchored on `ctx.WorkDir`
rather than calling `os.OpenRoot` on the lexically joined `destPath`. Without it the fix
closes the single-archive escape while leaving a two-archive variant of the same bug open.

Two documentation changes: record that `.Perm()` is behavior-preserving, and drop the
recipe-controlled-`dest` carve-out, which the required change makes obsolete.

The core mechanism is sound. Every other attack -- entry types, symlinked destinations in
isolation, pre-check bypass, permission masking, symlink loops, platform coverage -- was
tried and held.

## Summary

The `os.Root` mechanism holds under attack, but the design's *anchoring* of the root did
not: calling `os.OpenRoot` on a lexically joined `destPath` reproduces the original bug one
level up, letting an archive plant the symlink that a later extraction step's destination
resolves through, with no malicious recipe required. Resolving the destination through a
root on `ctx.WorkDir` closes it, and as a bonus refuses absolute `dest` values, which
removes a carve-out the design had accepted. Everything else tried -- hardlink entries,
permission-bit masking, pre-check bypass, symlink loops, platform coverage -- held, and
`.Perm()` turns out to be behavior-preserving because tar's setuid bit was already being
dropped by the existing code.
