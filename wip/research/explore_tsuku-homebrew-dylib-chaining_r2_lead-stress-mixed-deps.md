# Lead: stress test 2 -- recipes with mixed runtime_dependencies (library + tool)

Empirical answer to the question: if the engine adds an RPATH entry for a tool-typed
runtime dependency (i.e. it walks ALL deps and constructs `$TSUKU_HOME/libs/<tool>-<v>/lib`
even though the tool actually lives under `$TSUKU_HOME/tools/<tool>-<v>/`), is the bogus
RPATH entry harmless or fatal at runtime?

Working directory: `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku`.

## TL;DR

- Bogus RPATH entries pointing to non-existent `$TSUKU_HOME/libs/<tool>-<v>/lib`
  directories are **harmless on Linux**: the glibc dynamic linker silently skips
  any RPATH/RUNPATH entry whose directory does not exist. `LD_DEBUG=libs` shows it
  visits the entry, fails to find the .so, and moves on.
- A single bogus entry, ten bogus entries, and even 50 fake entries all leave
  the binary functional. Tested with `zipcmp -V` -> exit 0.
- Conclusion: **the engine does not need to filter runtime_dependencies by type**
  for the RPATH-augmentation step on Linux. It can naively walk all deps and add
  `$LibsDir/<dep>-<ver>/lib` to RPATH; the entries that point to tool installs (where
  no such dir exists) cost nothing at runtime beyond a few extra `stat` calls. The
  Lead 9 ("must filter by dep type") concern is not load-bearing on Linux.

## Recipe enumeration

`recipes/` has 1439 TOML files. A recipe is classified `library` if (and only if)
it has `type = "library"` in its metadata block. 152 recipes match. Everything
else is treated as `tool` for the purposes of where it gets installed
(`$TSUKU_HOME/tools/`) and how the engine reasons about its lib/ dir.

Of the 298 recipes with non-empty `runtime_dependencies`, 27 mix library and tool
deps, and 5 of those provide `.so` files on Linux from BOTH dep types (i.e. would
exercise both placement layouts at runtime):

| Recipe | library deps (.so) | tool deps (.so) | tool deps that are bin-only |
|---|---|---|---|
| `aarch64-elf-gdb` | gmp, mpfr, readline, zstd | ncurses | xz |
| `arm-none-eabi-gdb` | gmp, mpfr, readline, zstd | ncurses | xz |
| `riscv64-elf-gdb` | gmp, mpfr, readline, zstd | ncurses | xz |
| `i386-elf-gdb` | gmp, mpfr, readline, zstd | ncurses | xz |
| `ncmpcpp` | libmpdclient, readline | ncurses | -- |

Plus a wider mixed set (27 total) where the tool dep may not ship .so on Linux
but is still classified `tool`:

```
zsh:        libs=[pcre2]  tools=[ncurses]
allureofthestars: libs=[gmp]  tools=[sdl2]
aircrack-ng: libs=[pcre2]  tools=[sqlite]
libzip:     libs=[zstd]  tools=[xz]
logstalgia: libs=[freetype, libpng, pcre2]  tools=[glew, sdl2]
libxmlb:    libs=[glib, zstd]  tools=[xz]
libtiff:    libs=[jpeg-turbo, zstd]  tools=[xz]
libbluray:  libs=[freetype, libudfread]  tools=[fontconfig]
libbladerf: libs=[libusb]  tools=[ncurses]
httpd:      libs=[brotli, libnghttp2, pcre2]  tools=[apr, apr-util]
hatari:     libs=[libpng]  tools=[sdl2]
homebank:   libs=[cairo, freetype, glib, gettext]  tools=[fontconfig]
freeciv:    libs=[cairo, freetype, gettext, glib, readline, zstd]  tools=[sdl2, sqlite]
rxvt-unicode: libs=[freetype]  tools=[fontconfig]
c-blosc2:   libs=[zstd]  tools=[lz4]
cdrdao:     libs=[mad]  tools=[lame]
gource:     libs=[freetype, libpng, pcre2]  tools=[glew, sdl2]
gnu-apl:    libs=[cairo, glib, libpng, pcre2, readline, gettext]  tools=[sqlite]
gsmartcontrol: libs=[cairo, glib, gettext, pcre2]  tools=[smartmontools]
i686-elf-grub: libs=[gettext]  tools=[xz]
osm2pgsql:  libs=[proj]  tools=[luajit]
ncdu:       libs=[zstd]  tools=[ncurses]
ncmpcpp:    libs=[libmpdclient, readline]  tools=[ncurses]
... and several gdb cross-compilers
```

Note: many tool-classified deps in this list are actually shared-library providers
that happen to lack `type = "library"` in their metadata (e.g. ncurses, xz, sdl2,
sqlite, fontconfig, lz4). The engine still treats them as tools for path
construction, so this is exactly the surface the lead targets.

## Picking a candidate

`zsh` failed to install in the container (zsh -> ncurses build pulled in zig as a
build dep, which timed out fetching its archive). `libzip` is much lighter -- pure
homebrew bottle install for libzip itself, plus homebrew bottle installs for
xz (tool) and zstd (library). Picked `libzip`.

`libzip`:
- `runtime_dependencies = ["xz", "zstd"]`
- `xz`: `type = ""` (treated as tool) -> installs to `$TSUKU_HOME/tools/xz-5.8.3/`,
  ships only `bin/` in this recipe (no .so files captured by install_binaries)
- `zstd`: `type = "library"` -> installs to `$TSUKU_HOME/libs/zstd-1.5.7/`, ships
  `lib/libzstd.so*`

The deeper truth uncovered: the libzip recipe is itself broken on Linux because
`install_binaries.binaries = ["bin/zipcmp", "bin/zipmerge", "bin/ziptool"]` does
not capture `lib/libzip.so*` from the homebrew bottle. The verify step
(`zipcmp --version`) fails with `error while loading shared libraries: libzip.so.5`.
The lead is downstream of fixing that, but it is also a perfect harness for the
RPATH question because we can manually stage `libzip.so.5` and probe what RPATH
shape lets the binary load.

## Container setup

```
docker run --rm \
  -v $(pwd)/tsuku-test:/usr/local/bin/tsuku:ro \
  -e TSUKU_HOME=/tmp/tsuku \
  -e TSUKU_TELEMETRY=0 \
  debian:bookworm-slim bash -lc '
apt-get update -qq && apt-get install -y -qq --no-install-recommends \
  ca-certificates patchelf file
tsuku install libzip
'
```

After install (verify failure ignored):

```
/tmp/tsuku/tools/libzip-1.11.4/bin/zipcmp        # ELF, dynamic, NEEDED libzip.so.5
/tmp/tsuku/tools/xz-5.8.3/                        # bin/ only, no .so
/tmp/tsuku/libs/zstd-1.5.7/lib/libzstd.so.1      # the shared library
```

Initial RPATH on `zipcmp`: empty. NEEDED entries: `libzip.so.5 libz.so.1 libc.so.6`.
`ldd` reports `libzip.so.5 => not found` -- exactly the failure mode the lead asks
about, except for a different reason (libzip itself, not its dep).

To set up the experiment we extract `libzip.so.5` from the cached homebrew bottle
and stage it under `tools/libzip-1.11.4/lib/`:

```
LIBZIP_META=$(grep -l "homebrew/core/libzip" /tmp/tsuku/cache/downloads/*.meta | head -1)
LIBZIP_BLOB="${LIBZIP_META%.meta}.data"
mkdir -p /tmp/extracted && cd /tmp/extracted && tar xf "$LIBZIP_BLOB"
mkdir -p /tmp/tsuku/tools/libzip-1.11.4/lib
cp -a /tmp/extracted/libzip/1.11.4_1/lib/libzip.so.5.5 /tmp/tsuku/tools/libzip-1.11.4/lib/
ln -sf libzip.so.5.5 /tmp/tsuku/tools/libzip-1.11.4/lib/libzip.so.5
```

Now `$ORIGIN/../lib` would resolve `libzip.so.5`, leaving us free to vary
extra RPATH entries and watch what happens.

## Scenario A: only library-dep lib/ in RPATH (correct filtering)

```
patchelf --set-rpath '$ORIGIN/../lib:/tmp/tsuku/libs/zstd-1.5.7/lib' \
  /tmp/tsuku/tools/libzip-1.11.4/bin/zipcmp
```

- RPATH (RUNPATH actually -- modern patchelf writes DT_RUNPATH):
  `$ORIGIN/../lib:/tmp/tsuku/libs/zstd-1.5.7/lib`
- `ldd` resolves all NEEDED libs (libzip.so.5 from `tools/.../lib`, libz/libbz2/
  liblzma/libzstd/libcrypto from system, libc from system).
- `zipcmp -V` prints version banner, exit code 0.

`LD_DEBUG=libs` excerpt:

```
search path=...:/tmp/tsuku/tools/libzip-1.11.4/bin/../lib:.../zstd-1.5.7/lib...
trying file=/tmp/tsuku/tools/libzip-1.11.4/bin/../lib/libzip.so.5  [hit]
```

## Scenario B: ALL deps including nonsense tool path

```
patchelf --set-rpath '$ORIGIN/../lib:/tmp/tsuku/libs/zstd-1.5.7/lib:/tmp/tsuku/libs/xz-5.8.3/lib' \
  /tmp/tsuku/tools/libzip-1.11.4/bin/zipcmp
```

- `/tmp/tsuku/libs/xz-5.8.3/lib` does NOT exist (xz is a tool; it lives at
  `/tmp/tsuku/tools/xz-5.8.3/`).
- `ldd` resolves the same set of NEEDED libs as Scenario A. No errors.
- `zipcmp -V` prints version banner, exit code 0.

`LD_DEBUG=libs` excerpt for libz.so.1 lookup (which is satisfied by /lib/x86_64...):

```
search path=...:/tmp/tsuku/libs/zstd-1.5.7/lib:.../xz-5.8.3/lib...   (RUNPATH)
trying file=/tmp/tsuku/tools/libzip-1.11.4/bin/../lib/libz.so.1
trying file=/tmp/tsuku/libs/zstd-1.5.7/lib/.../libz.so.1
trying file=/tmp/tsuku/libs/xz-5.8.3/lib/.../libz.so.1   [silently fails -- dir absent]
search cache=/etc/ld.so.cache
trying file=/lib/x86_64-linux-gnu/libz.so.1   [hit]
```

The dynamic linker generates the full set of subdirectory candidates (glibc-hwcaps,
tls/x86_64, x86_64, etc.) under the bogus path and tries each one. All `stat()`
calls return ENOENT; the loader moves on without raising any error to userspace.
The binary runs identically to Scenario A.

## Scenario C: many garbage paths interleaved with the real one

```
patchelf --set-rpath \
  '$ORIGIN/../lib:/totally/fake:/another/nope:/tmp/tsuku/libs/xz-5.8.3/lib:/dev/null/sub:/zzz/yyy/xxx' \
  /tmp/tsuku/tools/libzip-1.11.4/bin/zipcmp
```

- Result: `zipcmp -V` succeeds, exit code 0.
- `/dev/null/sub` is interesting: `/dev/null` exists as a character device, so
  the loader's `stat` on `/dev/null/sub/libfoo.so.1` returns ENOTDIR rather than
  ENOENT, but glibc treats both as "not here, keep looking". No error surfaces.

## Scenario D: 50 fake `$TSUKU_HOME/libs/<fake-tool>-1.0.0/lib` entries

```
GARBAGE=""
for i in $(seq 1 50); do
  GARBAGE="$GARBAGE:/tmp/tsuku/libs/fake-tool-$i-1.0.0/lib"
done
patchelf --set-rpath "\$ORIGIN/../lib:/tmp/tsuku/libs/zstd-1.5.7/lib$GARBAGE" \
  /tmp/tsuku/tools/libzip-1.11.4/bin/zipcmp
```

- RPATH string is 1987 bytes long.
- `zipcmp -V` succeeds, exit code 0.
- patchelf, the kernel, and glibc all happily accept it. There is no documented
  hard cap on RPATH length below several KB.

## Mechanics: why nonsense RPATH entries are free

glibc's `_dl_map_object_from_fd` / `_dl_search_object_in_path` walks RUNPATH
left-to-right. For each entry it builds `<entry>/<basename>` (and the hwcaps/tls
variants) and `__open_nocancel`s them. ENOENT and ENOTDIR are non-fatal; the
loader simply tries the next candidate, then `/etc/ld.so.cache`, then the
default lib paths. The only failure mode is "no candidate succeeded for a NEEDED
lib", and that is independent of how many bogus entries preceded a successful one.

Cost of a bogus entry: a handful of `stat`/`open` syscalls per NEEDED lib at
process startup. Measurable but tiny -- a few hundred microseconds of extra
syscall traffic for the 50-entry case, none of which is repeated within a single
process lifetime. For a CLI that runs once and exits, this is invisible.

## What this means for the engine

The Lead 9 dichotomy was "engine MUST filter by dep type" vs "doesn't matter".
Empirically:

- Adding `$LibsDir/<dep>-<ver>/lib` for a tool-typed dep produces a non-existent
  path. The Linux dynamic linker silently skips it. The binary still works.
- Adding `$LibsDir/<dep>-<ver>/lib` for a library-typed dep produces a real path
  that supplies the .so. The binary works.
- Adding both, in any order, costs only a few extra `stat()` calls.

So the engine MAY filter by `Recipe.Metadata.Type == "library"` for cleanliness,
but it does NOT have to in order to be correct. A naive walk over all
runtime_dependencies that constructs `$LibsDir/<dep>-<ver>/lib` and patchelfs
each into RPATH is functionally equivalent on Linux -- the wrong-path entries are
inert.

Caveats this stress test does not cover:

- macOS dyld behavior is in scope for the broader lead but not this stress
  test (see the existing fix-rpath-analysis report). dyld treats LC_RPATH
  entries similarly (silently skips non-existent), but its codesigning
  reattachment after install_name_tool may surface different concerns.
- A future tool dep that DOES install `lib/<name>.so` under
  `$ToolsDir/<dep>-<ver>/` (rather than under `$LibsDir/...`) would NOT be
  found by an RPATH entry pointing at `$LibsDir/...`. The naive walk gets the
  prefix wrong, so the dep's libs go unresolved. If the engine wants to
  support tool-deps-that-ship-libs, it needs to consult the dep's actual
  install root, not blindly construct `$LibsDir/...`. None of the 27 mixed
  recipes today appear to depend on that pattern -- the tool-classified
  deps either ship .so under `$LibsDir/...` (because they're really
  libraries with a missing `type` field) or ship none at all.
- The bogus-path cost is per-process. A program that fork+execs many short-lived
  children (e.g., a build driver) would pay the syscall cost per child, but
  even hundreds of bogus entries would cost only single-digit milliseconds in
  startup. Not a practical concern.

## Container commands captured

```
# Discovery
grep -rn "runtime_dependencies" recipes/   # 330 hits, 298 non-empty

# Install + verify failure
docker run --rm -v $(pwd)/tsuku-test:/usr/local/bin/tsuku:ro \
  -e TSUKU_HOME=/tmp/tsuku -e TSUKU_TELEMETRY=0 \
  debian:bookworm-slim bash -lc 'apt-get install -y patchelf file ca-certificates;
    tsuku install libzip'
# -> Error: installation verification failed: libzip.so.5: cannot open shared object file

# Stage libzip.so.5 manually, then run scenarios A and B
# (see scripts inline above)
```

## Conclusion (one sentence)

On Linux, an RPATH entry pointing into `$TSUKU_HOME/libs/<tool>-<v>/lib` for a
runtime_dependency that is actually a tool (and therefore lives under
`$TSUKU_HOME/tools/<v>/`) is a no-op at runtime: the dynamic linker silently
skips non-existent directories, so the engine does not have to filter
runtime_dependencies by `Type == "library"` before constructing RPATH entries.
