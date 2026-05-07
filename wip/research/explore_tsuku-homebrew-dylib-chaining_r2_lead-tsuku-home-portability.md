# Stress test 6: `$TSUKU_HOME` portability after move/symlink

## Lead

Validate empirically whether `$ORIGIN`-relative RPATHs survive moving `$TSUKU_HOME` to a different path (or being accessed via a symlink). Compare against the existing `set_rpath` action's absolute-path template substitution.

## Hypothesis

Relative RPATHs of the form `$ORIGIN/../../../libs/<dep>-<version>/lib` survive both physical moves of `$TSUKU_HOME` and symlink redirection. Absolute RPATHs of the form `/path/to/tsuku-home/libs/<dep>-<v>/lib` break under move and only work under symlink redirection if the original path still resolves.

## Result

**Validated.** Relative RPATHs survive both move and symlink. Absolute RPATHs break on move, work via symlink only.

## Test environment

- Image: `debian:bookworm-slim`
- Tools: `patchelf 0.14.3`, `gcc 12.2.0`, `glibc 2.36`
- `$TSUKU_HOME` layout (matches the design):
  ```
  $TSUKU_HOME/
    tools/<name>-<version>/bin/<binary>
    libs/<dep>-<version>/lib/<libname>.so.X
  ```
- Relative path from `tools/<name>-<v>/bin/` to `libs/<dep>-<v>/lib/` is `../../../libs/<dep>-<v>/lib` (3 levels up: `bin` → `<name>-<v>` → `tools` → `$TSUKU_HOME`, then descend into `libs/<dep>-<v>/lib`).

The git+pcre2 path was attempted but the `pcre2` recipe builds from source via `configure_make`, which depends on `zig`, whose download mirror (ziglang.org / 65.109.105.178) was unreachable in the container's network environment for the duration of the test (5 retries, all timed out or reset). Pivoted to a synthetic faithful reproduction since the hypothesis is fundamentally about ELF dynamic-linker semantics, not git/pcre2 specifically.

## Synthetic test artifacts

```c
// libdep.c — provides dep_greet()
const char *dep_greet(void) { return "hello-from-dep-v1.2.3"; }

// tool.c — calls dep_greet()
extern const char *dep_greet(void);
int main(void) { printf("tool says: %s\n", dep_greet()); return 0; }
```

Built with:
```
gcc -fPIC -shared -Wl,-soname,libdep.so.1 -o libdep.so.1.2.3 libdep.c
gcc -o tool tool.c -L. -ldep
```

Installed into `$TSUKU_HOME/tools/faketool-9.9/bin/faketool` and `$TSUKU_HOME/libs/fakedep-1.2.3/lib/libdep.so.{,1,1.2.3}` (with normal SONAME symlink chain).

## Scenarios

### Baseline 0: no RPATH

```
patchelf --print-rpath /tmp/tsuku-A2/tools/faketool-9.9/bin/faketool
(empty)
$ /tmp/tsuku-A2/tools/faketool-9.9/bin/faketool
error while loading shared libraries: libdep.so.1: cannot open shared object file
exit=127
```

Confirms the gap the design addresses: a tool installed under `$TSUKU_HOME` cannot find its sibling library via the loader's default search path.

### Scenario 1: `$ORIGIN`-relative RPATH at original prefix

```
patchelf --set-rpath '$ORIGIN/../../../libs/fakedep-1.2.3/lib' \
  /tmp/tsuku-A2/tools/faketool-9.9/bin/faketool

readelf -d ...
0x000000000000001d (RUNPATH)  Library runpath: [$ORIGIN/../../../libs/fakedep-1.2.3/lib]

$ ldd .../faketool
libdep.so.1 => /tmp/tsuku-A2/tools/faketool-9.9/bin/../../../libs/fakedep-1.2.3/lib/libdep.so.1

$ /tmp/tsuku-A2/tools/faketool-9.9/bin/faketool
tool says: hello-from-dep-v1.2.3
exit=0
```

Note: patchelf 0.14 emits **DT_RUNPATH** by default (not DT_RPATH). For our use case (ELF binary loading its own NEEDED libraries), DT_RUNPATH is fine. DT_RUNPATH does *not* propagate to transitive dependencies — but the chained-dylib test below shows we don't need it to, as long as each `.so` carries its own `$ORIGIN`-relative RPATH.

### Scenario 2: move `$TSUKU_HOME` to a different path

```
mv /tmp/tsuku-A2 /tmp/tsuku-B2

readelf -d /tmp/tsuku-B2/.../faketool | grep RUNPATH
RUNPATH: [$ORIGIN/../../../libs/fakedep-1.2.3/lib]   (unchanged)

ldd /tmp/tsuku-B2/.../faketool
libdep.so.1 => /tmp/tsuku-B2/tools/faketool-9.9/bin/../../../libs/fakedep-1.2.3/lib/libdep.so.1

$ /tmp/tsuku-B2/.../faketool
tool says: hello-from-dep-v1.2.3
exit=0
```

`$ORIGIN` resolves at runtime to the directory of the loaded executable, so the same RUNPATH string transparently retargets to the new prefix. **Move-portability confirmed.**

### Scenario 3: symlink old path to new

```
ln -sf /tmp/tsuku-B2 /tmp/tsuku-A2
$ /tmp/tsuku-A2/tools/faketool-9.9/bin/faketool   # via symlink
tool says: hello-from-dep-v1.2.3
exit=0
$ /tmp/tsuku-B2/tools/faketool-9.9/bin/faketool   # real path
tool says: hello-from-dep-v1.2.3
exit=0
```

`$ORIGIN` for a symlinked invocation resolves to the *symlink-traversed* path (`/tmp/tsuku-A2/.../bin`), and the lib resolution then climbs back up that path. Both invocation paths work. **Symlink-portability confirmed.**

Edge case: even when there is no real directory at the original location and `$TSUKU_HOME` is *only* reachable via a symlink, the binary works via either the symlink or the real path.

### Scenario 4: absolute-path RPATH baseline

```
patchelf --set-rpath '/tmp/tsuku-A3/libs/fakedep-1.2.3/lib' \
  /tmp/tsuku-A3/tools/faketool-9.9/bin/faketool

$ /tmp/tsuku-A3/.../faketool        # works, prefix exists
tool says: hello-from-dep-v1.2.3

mv /tmp/tsuku-A3 /tmp/tsuku-B3
$ /tmp/tsuku-B3/.../faketool
error while loading shared libraries: libdep.so.1: cannot open shared object file
exit=127
```

ldd confirms `libdep.so.1 => not found`. The RPATH is now a dangling absolute reference to a directory that no longer exists. **This is the failure mode the relative RPATH fixes.**

The absolute-path version *does* recover if the old path is restored as a symlink (`ln -sf /tmp/tsuku-B3 /tmp/tsuku-A3`), but that requires the user to know to do this and only works for the single original prefix.

### Bonus: chained dylibs (libtrans → libdep → tool)

This is the design's "dylib chaining" case. Built `libtrans.so.0.5.0` providing `trans_msg()`, then `libdep.so.1.2.3` linking to libtrans, then `tool` linking to libdep.

```
$TSUKU_HOME/tools/faketool-9.9/bin/faketool
$TSUKU_HOME/libs/fakedep-1.2.3/lib/libdep.so.1.2.3   (NEEDED libtrans.so.1)
$TSUKU_HOME/libs/transdep-0.5.0/lib/libtrans.so.0.5.0
```

RPATHs:
- `tool`: `$ORIGIN/../../../libs/fakedep-1.2.3/lib`  (3 up to `$TSUKU_HOME`)
- `libdep.so.1.2.3`: `$ORIGIN/../../transdep-0.5.0/lib`  (2 up: `lib` → `fakedep-1.2.3` → `libs`, then descend `transdep-0.5.0/lib`)

Result:
```
$ /tmp/tsuku-AC/tools/faketool-9.9/bin/faketool
tool says: chained: from-transitive-v0.5
exit=0

ldd:
libdep.so.1 => .../libs/fakedep-1.2.3/lib/libdep.so.1
libtrans.so.1 => .../libs/fakedep-1.2.3/lib/../../transdep-0.5.0/lib/libtrans.so.1
```

After move and after symlink: still works. **Chaining is portable** as long as each link in the chain carries its own `$ORIGIN`-relative RPATH. Crucially, this means we can use DT_RUNPATH (patchelf default) and don't need DT_RPATH — the transitive lookup uses libdep's own RUNPATH, not the executable's RUNPATH inheritance.

### Bonus: combined `$ORIGIN`+absolute multi-RPATH

```
RPATH: $ORIGIN/../../../libs/fakedep-1.2.3/lib:/tmp/tsuku-A4/libs/fakedep-1.2.3/lib
```

Works at original prefix and after move. The `$ORIGIN` entry resolves first; the dangling absolute entry is silently skipped after move. This means the design can ship belt-and-suspenders RPATHs without breaking anything.

### Bonus: tarball-style relocation across paths

Copying `$TSUKU_HOME` to `/opt/elsewhere/tsuku-tarball` and to `/var/lib/foo/bar/baz/tsuku` (a deeper path): the binary works from both new locations without re-patching. This is the "user untars our distribution into wherever they want" scenario.

## Captured commands

Full session is reproducible from the synthetic sources at `/tmp/synth/src` inside the test container. Layout per scenario:

| Scenario | RPATH | Original prefix | Moved prefix | Symlinked prefix |
|---|---|---|---|---|
| 0 (none) | empty | fail | fail | fail |
| 1 ($ORIGIN) | `$ORIGIN/../../../libs/...` | pass | pass | pass |
| 4 (absolute) | `/tmp/tsuku-A3/libs/...` | pass | **fail** | pass (symlink restores old path) |
| Chained | `$ORIGIN/...` on tool and libdep | pass | pass | pass |
| Combined | `$ORIGIN/...:$abs/...` | pass | pass | pass |

## Implementation notes for the design

1. **Use DT_RUNPATH (patchelf default) not DT_RPATH.** DT_RPATH is deprecated. DT_RUNPATH is sufficient: each ELF object resolves its own NEEDED libraries against its own RUNPATH, so chains work as long as every link is patched.

2. **Compute the relative path from `<install-dir>/bin` (or wherever the binary lives) to each dep's `lib`.** For the standard layout `tools/<name>-<v>/bin` → `libs/<dep>-<v>/lib`, this is always `../../../libs/<dep>-<v>/lib`. For nested-binary cases (e.g. `tools/<name>-<v>/libexec/foo/bar`), use Go's `filepath.Rel` from the binary's directory to the dep's `lib`.

3. **Patch shared libraries too, not just executables.** When a recipe's install puts a library under `tools/<name>-<v>/lib/`, that library may itself depend on a sibling `libs/<dep>-<v>/lib`. Each library needs its own `$ORIGIN`-relative RPATH for chained resolution to work. (In the synthetic chained test, `libdep` had its own RPATH pointing to `libtrans`.)

4. **Multiple RPATH entries are colon-separated and probed in order.** Safe to combine `$ORIGIN`-relative entries for portability + an absolute entry as a last-resort fallback (for, e.g., `LD_LIBRARY_PATH=` style overrides where users explicitly point the loader).

5. **`$ORIGIN` is honored even when the binary is loaded via a symlinked path.** It resolves to the path-as-loaded, which means symlinking `$TSUKU_HOME` is a first-class supported usage pattern (e.g. `ln -s /opt/tsuku ~/.tsuku`).

## Verdict on the design's portability claim

> "Relative-path RPATHs survive moving `$TSUKU_HOME`. This is a portability win over the existing `set_rpath` action's absolute-path template substitution."

**Confirmed.** All three flavors of relocation (move, symlink-redirect, copy-to-new-prefix) work for `$ORIGIN`-relative RPATHs. The same operations break the absolute-path baseline (move, copy-to-new-prefix) or require the user to manually create a back-symlink (move + symlink). The design's portability claim is empirically validated, including the harder chained-dylib subcase that "dylib chaining" implies.
