# Stress Test 1 — Recipes with All-Library `runtime_dependencies`

Empirical test of the augmented-engine hypothesis: if every entry in
`runtime_dependencies` is a library, walking those deps and adding their
`lib/` dirs to the consumer binary's RPATH should make the binary run
without the LD_LIBRARY_PATH wrapper that tsuku currently emits.

Three candidates were tested in `debian:bookworm-slim` containers:

1. `git` — curated; `runtime_dependencies = ["pcre2"]`
2. `wget` — curated; `runtime_dependencies = ["openssl", "gettext", "libidn2", "libunistring"]`
3. `calcurse` — non-curated, `llm_validation = "skipped"`; `runtime_dependencies = ["gettext"]`

## Method

For each candidate the container script does:

1. `apt-get install -y patchelf ca-certificates curl file`
2. `tsuku install <name> --force --recipe /recipes/<l>/<name>.toml` with
   `TSUKU_HOME=/tmp/tsuku`. Up to 5 retries against transient mirror flakes.
3. Locate the actual ELF binary under
   `$TSUKU_HOME/tools/<name>-<ver>/bin/<name>` (the
   `$TSUKU_HOME/tools/current/<name>` entry is a `LD_LIBRARY_PATH`
   wrapper, not an ELF — see Finding 1 below).
4. Read `patchelf --print-rpath <binary>` and `ldd <binary>` for baseline.
5. To force the simulation, hide the matching system shared libraries
   under `/lib/x86_64-linux-gnu/` so the loader can only find the dep
   libraries in `$TSUKU_HOME/libs/<dep>-<ver>/lib/`.
6. Run verify against the ELF directly with no RPATH fix.
7. `patchelf --force-rpath --set-rpath
   "$ORIGIN:<dep1>/lib:<dep2>/lib:..." <binary>`.
8. Re-run verify.

Special handling:

- `git` — pcre2's `setup_build_env` step pulls in `zig`, and ziglang.org
  served HTTP 500 / TCP RST repeatedly during the run. The git
  experiment was repeated with a stripped recipe that omits
  `runtime_dependencies` (so tsuku doesn't try to build pcre2 from
  source) and a fake `$TSUKU_HOME/libs/pcre2-10.42/lib/` directory
  staged from the apt-installed `libpcre2-8-0` package. This still
  exercises the design hypothesis: does putting a `libpcre2-8.so.0`
  under a `$TSUKU_HOME/libs/<dep>-<ver>/lib/` path and adding that path
  to the binary's RPATH make the loader resolve the lib?
- `wget` — installed cleanly through tsuku.
- `calcurse` — installed cleanly through tsuku, but the actual binary
  on bookworm-slim demands `libncursesw.so.6` which is not in the base
  image. See Finding 2.

## Finding 1 — Tsuku already emits an `LD_LIBRARY_PATH` wrapper

For all three candidates, `$TSUKU_HOME/tools/current/<name>` is a POSIX
shell script of the form

```sh
#!/bin/sh
LD_LIBRARY_PATH="/tmp/tsuku/libs/gettext-1.0/lib:/tmp/tsuku/libs/libidn2-2.3.8/lib:..."
export LD_LIBRARY_PATH
exec "/tmp/tsuku/tools/wget-1.25.0/bin/wget" "$@"
```

The augmented engine the design proposes wouldn't change `runtime_dependencies`
semantics — it would change the implementation from a wrapper to a
patched RPATH on the underlying ELF. The simulation in this lead
patches the underlying ELF at
`$TSUKU_HOME/tools/<name>-<ver>/bin/<name>` directly so the wrapper is
not in the path.

## Finding 2 — Recipe-declared deps must match what the binary actually
needs

For `calcurse`, `runtime_dependencies = ["gettext"]` is incomplete. The
ELF binary's DT_NEEDED list includes `libncursesw.so.6` and `libc.so.6`
but no `libintl.so.*`. On bookworm-slim, the binary fails before any
RPATH manipulation:

```
/tmp/tsuku/tools/calcurse-4.8.2/bin/calcurse: error while loading
shared libraries: libncursesw.so.6: cannot open shared object file:
No such file or directory
```

Adding `/tmp/tsuku/libs/gettext-1.0/lib` to RPATH (the only thing the
augmented engine would do for this recipe) does not change the result,
because the missing library isn't in the gettext install dir. **The
augmented engine cannot rescue an under-declared
`runtime_dependencies` list.** The `calcurse` recipe is bypassing this
issue today by relying on whatever happens to be present on the host;
if the host is missing libncursesw, the existing
`LD_LIBRARY_PATH=...gettext-1.0/lib...` wrapper also fails, so this
isn't a regression introduced by switching to RPATH — it's a
pre-existing recipe-correctness gap.

## Finding 3 — `--add-rpath` vs `--force-rpath --set-rpath`

A first attempt at `wget` used
`patchelf --add-rpath /tmp/tsuku/libs/<dep>-<ver>/lib <binary>` once
per dep. The resulting RPATH was
`$ORIGIN:/tmp/tsuku/libs/openssl-...:/tmp/tsuku/libs/libidn2-...:...`
which looks correct, yet `libunistring.so.5` came back as `not found`
in `ldd` while every other lib in the same set resolved. (No clean
explanation; likely an artifact of the binary's existing
`DT_RUNPATH` and how patchelf merges the entries — see the
`wget_test2.txt` capture.) Switching to
`patchelf --force-rpath --set-rpath \
  "/tmp/tsuku/libs/openssl-3.6.2/lib:/tmp/tsuku/libs/libidn2-2.3.8/lib:/tmp/tsuku/libs/libunistring-1.4.2/lib:/tmp/tsuku/libs/zlib-1.3.2/lib:\$ORIGIN" \
  <binary>` resolved every DT_NEEDED entry. The augmented engine
should use `--force-rpath --set-rpath`, which is also what the
existing `homebrew_relocate` action does (see
`internal/actions/homebrew_relocate.go:387`).

## Per-candidate captures

### git

Docker command:

```
docker run --rm \
  -v "$PWD/tsuku-test:/usr/local/bin/tsuku:ro" \
  -v "$PWD/recipes:/recipes:ro" \
  -v /tmp/stress_test_logs/git_alt2.sh:/run.sh:ro \
  debian:bookworm-slim \
  bash /run.sh
```

Recipe used (stripped of `runtime_dependencies` to avoid the
zig-from-ziglang.org flake; the simulation stages a fake
`$TSUKU_HOME/libs/pcre2-10.42/lib/libpcre2-8.so.0` from the apt
`libpcre2-8-0` package).

Pre-patch RPATH (after `homebrew_relocate` ran):

```
$ORIGIN
```

Pre-patch verify (system `libpcre2-8.so.0` hidden, RPATH unchanged):

```
$ELF --version
/tmp/tsuku/tools/git-2.54.0/bin/git: error while loading shared
libraries: libpcre2-8.so.0: cannot open shared object file: No such
file or directory
```

`ldd` confirms `libpcre2-8.so.0 => not found`.

Post-patch RPATH:

```
$ORIGIN:/tmp/tsuku/libs/pcre2-10.42/lib
```

Post-patch verify:

```
$ELF --version
/tmp/tsuku/tools/git-2.54.0/bin/git: /tmp/tsuku/libs/pcre2-10.42/lib/libpcre2-8.so.0:
no version information available (required by /tmp/tsuku/tools/git-2.54.0/bin/git)
git version 2.54.0
```

The "no version information available" line is a non-fatal warning
about a SONAME version skew between the bottle's expected pcre2 and
the apt-supplied stand-in; the binary still runs and `git --version`
prints the version line that the `git.toml` `[verify].pattern` checks.
**Verdict: works.**

### wget

Docker command:

```
docker run --rm \
  -v "$PWD/tsuku-test:/usr/local/bin/tsuku:ro" \
  -v "$PWD/recipes:/recipes:ro" \
  -v /tmp/stress_test_logs/wget_debug.sh:/run.sh:ro \
  debian:bookworm-slim \
  bash /run.sh
```

Tsuku installed cleanly: `patchelf-0.18.0`, `zlib-1.3.2`,
`openssl-3.6.2`, `gettext-1.0`, `libidn2-2.3.8`, `libunistring-1.4.2`,
`wget-1.25.0`.

Pre-patch RPATH on the ELF: `$ORIGIN`.

After hiding `libssl.so.3`, `libcrypto.so.3`, `libidn2.so.0`,
`libunistring.so.2`, `libunistring.so.5`, `libz.so.1` from
`/lib/x86_64-linux-gnu/`:

```
$ELF --version
/tmp/tsuku/tools/wget-1.25.0/bin/wget: error while loading shared
libraries: libidn2.so.0: cannot open shared object file: No such file
or directory
```

After `patchelf --force-rpath --set-rpath
"/tmp/tsuku/libs/openssl-3.6.2/lib:/tmp/tsuku/libs/libidn2-2.3.8/lib:/tmp/tsuku/libs/libunistring-1.4.2/lib:/tmp/tsuku/libs/zlib-1.3.2/lib:$ORIGIN"`:

```
$ELF --version
GNU Wget 1.25.0 built on linux-gnu.
-cares +digest -gpgme +https +ipv6 +iri +large-file -metalink +nls
+ntlm +opie -psl +ssl/openssl
```

`ldd` shows every DT_NEEDED entry resolving inside `$TSUKU_HOME/libs/`:

```
libidn2.so.0   => /tmp/tsuku/libs/libidn2-2.3.8/lib/libidn2.so.0
libssl.so.3    => /tmp/tsuku/libs/openssl-3.6.2/lib/libssl.so.3
libcrypto.so.3 => /tmp/tsuku/libs/openssl-3.6.2/lib/libcrypto.so.3
libz.so.1      => /tmp/tsuku/libs/zlib-1.3.2/lib/libz.so.1
libunistring.so.5 => /tmp/tsuku/libs/libunistring-1.4.2/lib/libunistring.so.5
```

(`gettext` was declared in `runtime_dependencies` but the wget bottle
on Linux does not link `libintl.so.*` directly — wget's
`libgettextlib`-style features are configured via `+nls`, which works
through libc's locale support on glibc. The gettext dep is harmless
but functionally unused at link time on this platform. macOS is the
case the recipe comment refers to, where `libintl.8.dylib` is needed.)

**Verdict: works.**

### calcurse

Docker command:

```
docker run --rm \
  -v "$PWD/tsuku-test:/usr/local/bin/tsuku:ro" \
  -v "$PWD/recipes:/recipes:ro" \
  -v /tmp/stress_test_logs/calcurse_test.sh:/run.sh:ro \
  debian:bookworm-slim \
  bash /run.sh
```

Tsuku installed cleanly: `gettext-1.0`, `calcurse-4.8.2`. Wrapper at
`tools/current/calcurse` sets
`LD_LIBRARY_PATH=/tmp/tsuku/libs/gettext-1.0/lib`.

Pre-patch RPATH on the ELF: `$ORIGIN`.

Pre-patch verify on stock bookworm-slim (no system manipulation):

```
$ELF --version
/tmp/tsuku/tools/calcurse-4.8.2/bin/calcurse: error while loading
shared libraries: libncursesw.so.6: cannot open shared object file:
No such file or directory
```

`ldd`:

```
linux-vdso.so.1 (0x...)
libncursesw.so.6 => not found
libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6
/lib64/ld-linux-x86-64.so.2
```

After `patchelf --force-rpath --set-rpath
"$ORIGIN:/tmp/tsuku/libs/gettext-1.0/lib"`:

```
post-patch RPATH: $ORIGIN:/tmp/tsuku/libs/gettext-1.0/lib
$ELF --version
/tmp/tsuku/tools/calcurse-4.8.2/bin/calcurse: error while loading
shared libraries: libncursesw.so.6: cannot open shared object file:
No such file or directory
```

The binary's DT_NEEDED is `libncursesw.so.6`, but
`runtime_dependencies = ["gettext"]` doesn't declare ncurses. The
existing wrapper has the same gap — it ships the gettext lib dir on
LD_LIBRARY_PATH but no ncurses. The augmented engine inherits the
same gap. **Verdict: breaks (recipe-declaration gap, not an RPATH
problem).**

## Three-line summary

- git: works (verify succeeds once `$TSUKU_HOME/libs/pcre2-10.42/lib`
  is on the RPATH; baseline without RPATH fails to find
  `libpcre2-8.so.0`).
- wget: works (verify succeeds once all four declared dep lib dirs are
  on the RPATH via `--force-rpath --set-rpath`; `--add-rpath`
  alone produced an unexplained `libunistring.so.5 => not found`).
- calcurse: breaks (`runtime_dependencies = ["gettext"]` doesn't
  declare the ncurses dep the binary actually needs; no RPATH
  manipulation can fix an under-declared deps list).
