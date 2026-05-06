# Stress Test 3: Recipes with `runtime_dependencies` That Don't Publish Dylibs

## Lead

Augmenting RPATH with `<dep>/lib` entries assumes the dep's lib/ is non-empty
and contains shared objects. But some library recipes are header-only,
publish only static archives (`*.a`), or have an empty lib/ on the target
platform. If the augmented engine blindly adds an RPATH entry for a missing
or empty `lib/`, what happens at runtime?

**Hypothesis:** the runtime linker silently skips non-existent paths and
empty dirs (harmless). Failure mode would be loud errors or warnings that
break otherwise-working installs.

## Result

**Empty/missing/bogus RPATH entries are harmless in every variant tested.**
The glibc `ld-linux` runtime linker treats a non-existent path identically to
an existing-but-empty one: it `stat()`s, fails, and silently moves on. No
stderr output, no warning, no errno propagation, no exit failure. Behavior
is the same in `LD_DEBUG=libs` mode (it logs `trying file=...`, then logs
the next attempt — no error line is emitted).

The only observable cost is **wall-clock latency**: each bogus RPATH entry
adds ~10 stat()s per missing library lookup (because glibc expands each
RPATH entry through `glibc-hwcaps/*`, `tls/*`, `x86_64/*` subdirs). In our
worst-case test this added ~5.5 ms per process invocation for 100 bogus
paths.

## Detailed Findings

### Test environment

- Image: `debian:bookworm-slim` (glibc 2.36)
- Tool under test: `bat 0.26.1` installed via `tsuku-test install bat`
  (zero existing `runtime_dependencies`, native ELF, RUNPATH initially empty)
- Method: `patchelf --set-rpath` to inject pathological RPATH entries, then
  run `bat --version` and capture exit code, stderr, `LD_DEBUG=libs` output.
- Dynamic libraries that bat actually needs: `libgcc_s.so.1`, `librt.so.1`,
  `libpthread.so.0`, `libm.so.6`, `libdl.so.2`, `libc.so.6`,
  `ld-linux-x86-64.so.2`. None are present in any RPATH entry tested; all
  were resolved via `/etc/ld.so.cache` → `/lib/x86_64-linux-gnu/`.

### Test 1 — empty `lib/` directory

```
mkdir -p /tmp/empty-libs/dep-1.0.0/lib
patchelf --set-rpath "/tmp/empty-libs/dep-1.0.0/lib:" /tmp/bat-test1
/tmp/bat-test1 --version
```

- `patchelf --print-rpath` → `/tmp/empty-libs/dep-1.0.0/lib:`
- exit 0, stdout `bat 0.26.1 (979ba22)`, stderr empty
- `LD_DEBUG=libs` shows the linker walking glibc-hwcaps/tls/x86_64 subdirs
  under the empty lib/, all `trying file=...` attempts fail silently, then
  `search cache=/etc/ld.so.cache` succeeds.

### Test 2 — non-existent directory

```
patchelf --set-rpath "/tmp/does-not-exist-anywhere/dep-9.9.9/lib:" /tmp/bat-test2
/tmp/bat-test2 --version
```

- exit 0, stderr empty
- `LD_DEBUG=libs` output identical pattern to Test 1: every `trying file=`
  fails, no error message about a missing path. The linker doesn't
  distinguish "ENOENT on the directory" from "ENOENT on the library".

### Test 3 — five bogus paths

```
patchelf --set-rpath "/tmp/bogus1/lib:/tmp/bogus2/lib:/tmp/bogus3/lib:/tmp/bogus4/lib:/tmp/bogus5/lib:" /tmp/bat-test3
```

- exit 0, stderr empty
- `LD_DEBUG=libs` shows linker walking each of the 5 paths × 10 hwcaps
  subdirs = 50 file probes per missing-but-eventually-found library. Still
  silent.

### Test 4 — combined empty + missing + 5 bogus

- exit 0, stderr 0 lines

### Test 5 — 100 bogus paths

- RPATH 2,193 chars, exit 0
- See timing below.

### Test 6 — RPATH entry that exists but is a regular file

```
touch /tmp/i-am-a-file
patchelf --set-rpath "/tmp/i-am-a-file:" /tmp/bat-test6
```

- exit 0, stderr 0 lines. Linker ENOTDIR is silent.

### Test 7 — chmod 000 directory (root)

- root bypasses chmod 000, so this didn't actually exercise EACCES.

### Test 8 — unreadable directory, non-root user

```
mkdir -p /tmp/no-read-perm/lib
chmod 700 /tmp/no-read-perm
chown root:root /tmp/no-read-perm
# Confirmed tester user gets EACCES on `ls /tmp/no-read-perm/`
patchelf --set-rpath "/tmp/no-read-perm/lib:" /tmp/bat-perm
su tester -c "/tmp/bat-perm --version"
```

- exit 0, stderr 0 lines.
- **Notable:** even when the kernel returns EACCES on the RPATH directory
  (not ENOENT), the runtime linker is still silent.

### Test 10 — setuid binary (glibc secure-execution mode)

```
chmod 4755 /tmp/bat-suid     # owned by root, setuid bit set
su tester -c "/tmp/bat-suid --version"
```

- exit 0, stderr empty
- glibc's "secure execution" mode (AT_SECURE) usually strips RPATH/RUNPATH
  entries containing `..` or empty, but here the entry is just an
  unwritable-by-non-root absolute path; no stripping observed.

### Test 11 — leading colon (literal empty entry)

```
patchelf --set-rpath ":<orig>" → final RPATH ":"
```

Note: patchelf collapses RPATHs aggressively when the original was empty;
the final RPATH was just `:`. Linker runs fine, exit 0, no stderr.

### Test 12 — weird RPATH (double colons, trailing slashes)

```
patchelf --set-rpath "/tmp/foo//::/tmp/bar/:"
```

- exit 0, stderr 0 lines. Linker tolerates `//`, `::`, trailing `/`.

### Test 13 — 1000 bogus entries (extreme size)

- RPATH 23,894 characters. patchelf accepted it without complaint.
- exit 0, stderr 0 lines. No truncation observed.

## Timing Observations

200 invocations of `bat --version` in a tight loop:

| Variant         | Real time | Per-invocation overhead vs baseline |
| --------------- | --------- | ----------------------------------- |
| baseline (`''` RPATH) | 0.559 s   | —                                  |
| 100 bogus paths       | 1.695 s   | +5.7 ms                            |

A single missing-library lookup costs roughly 10 stat() syscalls per RPATH
entry (one per glibc-hwcaps/tls/cpu subdir variant). With 100 bogus entries
that's ~1,000 extra `stat`/`open` syscalls per `dlopen`/library resolution.
For typical short-running CLI tools (`bat`, `eza`, `gh`) this is invisible
to users (~5 ms). For shells, hot-loops, or tools spawned at high frequency
(e.g. autocompletion helpers), an unbounded RPATH could become a tax —
worth keeping the dep list tight, but not a correctness issue.

## Implications for tsuku RPATH-augmentation work

1. **No correctness risk** from blindly adding RPATH entries for deps whose
   lib/ is empty, header-only, or missing. The runtime linker is silent.
2. **No need to filter** RPATH entries during installation based on whether
   the dep's lib/ has dynamic objects. The simpler "always add" rule is
   safe.
3. **Stat overhead is real but small.** ~5 ms per 100 bogus entries per
   process spawn. Recommend not enforcing a hard cap, but documenting that
   long dep chains have a measurable startup cost.
4. **EACCES is also silent**, which is good — recipes installed in shared
   filesystems with weird permissions won't break.
5. **patchelf and the linker tolerate every malformed RPATH variant** we
   threw at them: `::`, `:`, trailing `/`, `//`, regular files in place of
   directories, 1000-element entries, 23 KB total length.

## Caveats

- All tests on **glibc Linux x86_64**. macOS `dyld` and musl have not been
  tested but are believed to behave similarly (silent skip on missing
  paths). Static-archive-only deps are a Linux-relevant case anyway, since
  on macOS `.a` archives don't enter dynamic resolution either way.
- A setuid corner case (Test 10) didn't actually trigger AT_SECURE
  stripping because no `..` or empty entry was present. tsuku does not ship
  setuid binaries, so this is academic.

## Exact commands used

Scripts: `/tmp/tsuku-stress3/run.sh` and `/tmp/tsuku-stress3/run3.sh`.
Output logs: `/tmp/tsuku-stress3/output.log` and `output3.log`.

Container: `docker run --rm -v /tmp/tsuku-stress3:/work:ro debian:bookworm-slim bash /work/run.sh`.
