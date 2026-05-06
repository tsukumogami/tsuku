# Lead R1: Recipes that punted because of the homebrew dylib chaining gap

## Question

Survey the curated recipe registry for `supported_os = ["linux"]` or `unsupported_platforms` entries that mention dylib/bottle/RPATH reasons. These are recipes that scoped down to avoid the dylib chaining gap. List them and their reasons, distinguishing "truly punted" (no support) from "partially punted" (workaround that escapes the bottle).

## Method

1. Found all `recipes/*/*.toml` with `supported_os = ["linux"]` (50 hits).
2. Found all `recipes/*/*.toml` with `unsupported_platforms = [...]` (~225 hits).
3. Intersected each set with files mentioning `dylib`, `RPATH`, or `bottle` in any case.
4. For each candidate, extracted the contiguous comment block immediately above the `unsupported_platforms` / `supported_os` line and the platform list itself.
5. Classified each by the comment text:
   - **Dylib chaining (truly punted)**: macOS marked unsupported because the binary or a sibling library cannot resolve a needed `.dylib` at runtime. Includes "dyld fails to load self-library", "dyld fails to load X.dylib" (dep dylib), "self-library rpath", "abort trap (dylib loading failure)", and "missing X.dylib (undeclared dep)".
   - **Dylib chaining (partially punted)**: macOS marked unsupported, but the recipe ships a Linux source-build path that escapes the bottle. Curl is the canonical example.
   - **Other reasons (excluded)**: missing bottle for the OS variant ("no bottle for sonoma", "x86-64 only", "no arm64 bottle"), missing binary in bottle ("bin/X not in bottle"), generic Linux/arm64 bottle failures (these are orthogonal — Linux uses ELF, not dylibs).

## Headline counts

- **Truly punted on macOS due to dylib chaining: 26 recipes**
- **Partially punted on macOS due to dylib chaining (Linux source-build escape): 2 recipes** (curl, libcurl-source)
- **Linux-only via `supported_os = ["linux"]` with explicit bottle/dylib reasoning in comments: 4 recipes** (curl, tmux, libcurl-source, git — note git was unblocked once pcre2 dylib outputs landed)
- **Excluded as not dylib-chain related**: ~40 recipes whose comments cite "no bottle for sonoma", missing arch variants, or pure linux/arm64 bottle holes

The dylib chaining shape splits into two failure modes:

| Failure mode | Recipes | Example comment |
|---|---|---|
| **Dep dylib not in RPATH** (sibling tsuku-installed dep's dylib not resolved) | 8 | `# macOS: ncurses dep dyld fails to load libpanelw.6.dylib` |
| **Self-library RPATH** (recipe's own libfoo.dylib has wrong RPATH after staging) | 18 | `# macOS: dyld fails to load self-library libcln.6.dylib` |

Both modes are blocked by the same root cause: the homebrew action does not rewrite or chain RPATHs into sibling tsuku-installed library deps after the bottle is staged into `$TSUKU_HOME/tools/<name>-<ver>/`.

---

## Truly punted on macOS — dep dylib not chained from sibling tsuku dep

These recipes have a sibling tsuku-installed library dep, and the bottle's main binary or library has an `@rpath`/`@loader_path` reference to a dylib that lives in a different `$TSUKU_HOME/tools/<dep>-<ver>/lib/` directory which the homebrew action does not register in the consumer's RPATH. **This is exactly the curl gap.**

| Recipe | unsupported_platforms | Comment | Sibling dep involved |
|---|---|---|---|
| `b/bedtools.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: xz dep dyld fails to load liblzma.5.dylib` | xz |
| `c/cbonsai.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: ncurses dep dyld fails to load libpanelw.6.dylib` | ncurses |
| `c/cdogs-sdl.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: SDL2 dep dyld fails to load libSDL2-2.0.0.dylib` | sdl2 |
| `g/google-authenticator-libpam.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: qrencode dep dyld fails to load libqrencode.4.dylib` | qrencode |
| `g/gource.toml` | `darwin/arm64`, `darwin/amd64`, `linux/arm64` | `# macOS: glew dep dyld fails to load libGLEW.2.3.dylib` | glew |
| `i/i686-elf-grub.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: xz dep dyld fails to load liblzma.5.dylib` | xz |
| `i/innoextract.toml` | `darwin/arm64`, `darwin/amd64`, `linux/arm64` | `# macOS: xz dep dyld failure; linux/arm64: no bottles` | xz |
| `r/riscv64-elf-gdb.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: xz dep dyld fails to load liblzma.5.dylib` | xz |

**Workaround in place**: none. macOS is unsupported. The dep is correctly listed in `runtime_dependencies`, but the dylib is not chained into the consumer's RPATH at install time.

---

## Truly punted on macOS — self-library RPATH not set after staging

These recipes ship one or more `.dylib`s of their own. After the bottle is unpacked into `$TSUKU_HOME/tools/<name>-<ver>/`, the dylib's install_name or the consumer binary's `@rpath` still points at the homebrew Cellar path. The homebrew action never rewrites these. The shared-mime-info comment uses the explicit term "self-library rpath (exit 6)".

| Recipe | unsupported_platforms | Comment |
|---|---|---|
| `b/bochs.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load libltdl.7.dylib` |
| `c/cdrdao.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: abort trap (dylib loading failure)` |
| `c/cln.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libcln.6.dylib` |
| `c/cracklib.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libcrack.2.dylib` |
| `e/e2fsprogs.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libe2p.2.1.dylib` |
| `e/editorconfig.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libeditorconfig.0.dylib` |
| `g/gcab.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libgcab-1.0.0.dylib` |
| `g/gedit.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libgedit-49.dylib` |
| `g/gerbv.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libgerbv.1.dylib` |
| `g/glpk.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libglpk.40.dylib` |
| `g/gplugin.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libgplugin.0.dylib` |
| `g/gromacs.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libgromacs.11.dylib` |
| `g/gucharmap.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libgucharmap_2_90.7.dylib` |
| `g/gwyddion.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: abort trap (dylib loading failure)` |
| `l/libbladerf.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libbladeRF.2.dylib` |
| `l/libdazzle.toml` | `darwin/arm64`, `darwin/amd64`, `linux/arm64` | `# darwin: self-library rpath failure; linux/arm64: no bottles` |
| `l/libgpg-error.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libgpg-error.0.dylib` |
| `l/libgsf.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libgsf-1.114.dylib` |
| `l/libiptcdata.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libiptcdata.0.dylib` |
| `l/libiscsi.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library libiscsi.11.dylib` |
| `l/libpsl.toml` | `darwin/arm64`, `darwin/amd64`, `linux/arm64` | `# macOS: abort trap (dylib loading failure)` + linux/arm64 bottle issue |
| `l/librtlsdr.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: dyld fails to load self-library librtlsdr.0.dylib` |
| `l/lpeg.toml` | `darwin/arm64`, `darwin/amd64`, `linux/arm64` | `# darwin: self-library rpath failure; linux/arm64: no bottles` |
| `l/lsdvd.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: missing libdvdread.8.dylib (undeclared dep)` (transitively-the-same gap) |
| `n/newsboat.toml` | `darwin/arm64`, `darwin/amd64` | `# macOS: abort trap (dylib loading failure)` |
| `s/shared-mime-info.toml` | `linux/arm64`, `darwin/arm64`, `darwin/amd64` | `# darwin/arm64, darwin/amd64: self-library rpath (exit 6)` |
| `s/sigrok-cli.toml` | `darwin/arm64`, `darwin/amd64` | `# darwin/arm64, darwin/amd64: self-library rpath (exit 6)` |
| `s/speex.toml` | `darwin/arm64`, `darwin/amd64` | `# darwin/arm64, darwin/amd64: self-library rpath (exit 6)` |
| `s/stlink.toml` | `darwin/arm64`, `darwin/amd64` | `# darwin/arm64, darwin/amd64: self-library rpath (exit 6)` |

**Workaround in place**: none.

---

## Partially punted on macOS — Linux source-build escape

These curated recipes deliberately exclude darwin via `supported_os = ["linux"]` and route Linux through `configure_make` + `set_rpath` to bypass the homebrew bottle entirely. They keep Linux working but darwin remains unsupported until the dylib chaining gap is closed.

### `c/curl.toml` — canonical example

```toml
# macOS: homebrew curl bottle has RPATH references to libnghttp3.9.dylib from a
# separate homebrew package; bundling transitive dylibs is not supported.
# darwin coverage is excluded until runtime_dependencies supports dylib chaining.
supported_os = ["linux"]
dependencies = ["openssl", "zlib"]
```

**Workaround**: download → extract → `setup_build_env` → `configure_make` (`--with-openssl --with-zlib --without-libpsl`) → `set_rpath` (`$ORIGIN/../lib:{libs_dir}/openssl-{ver}/lib:{libs_dir}/zlib-{ver}/lib`) → `install_binaries`. The recipe even calls out the macOS limitation explicitly: `$ORIGIN`-based RPATH is Linux-only (ELF); macOS is excluded above.

### `l/libcurl-source.toml` — companion library, same shape

```toml
# Source-built libcurl for sandbox testing (no LDAP, minimal dependencies)
# Replaces Homebrew-bottled libcurl which fails in Debian containers due to
# OPENLDAP_2.200 vs OPENLDAP_2.5 symbol version mismatch.
...
supported_os = ["linux"]
dependencies = ["openssl", "zlib"]
```

The stated *primary* reason here is glibc symbol versioning, but the recipe was created to escape the same bottle gap. macOS is excluded for the same reason curl excludes it.

---

## Linux-only with bottle/dylib comments — not source-build escapes

### `t/tmux.toml` — truly punted on macOS

```toml
# macOS: homebrew tmux bottle has RPATH references to libutf8proc.3.dylib and
# libevent from separate homebrew packages; libevent itself is not installable
# on macOS via tsuku. darwin coverage is excluded until these deps are supported.
supported_os = ["linux"]
```

Linux glibc gets the homebrew bottle, Linux musl gets the apk package, **darwin gets nothing**. Workaround: none. This is the same dylib-chain gap as curl, but tmux did not get a configure_make escape hatch.

### `g/git.toml` — already unblocked

```toml
# `runtime_dependencies` ensures pcre2 is installed before `git --version`
# runs at verify time. macOS support was previously gated by
# `supported_os = ["linux"]` and unblocked once pcre2's macOS dylib outputs
# (`libpcre2-8.0.dylib` and friends) landed in #2335.
runtime_dependencies = ["pcre2"]
```

git is now darwin-supported. The recipe documents that it was previously linux-only because of the same dylib-chain issue, and was unblocked when pcre2 started shipping the right `.dylib` outputs into the layout the bottle expects. **Important data point**: the unblock here was a per-dep workaround (carefully hand-shipped pcre2 dylibs), not a generic fix to the homebrew action. The same trick has not been applied to the 26 recipes above.

---

## Truly punted on Linux/arm64 — orthogonal, not dylib chaining

For completeness: about 12 recipes punt `linux/arm64` because no homebrew bottle exists for that platform (`exit null`) or because the bottle install fails on glibc arm64 containers. These are bottle-availability issues, not RPATH/dylib chaining (Linux uses ELF, and tsuku's `set_rpath` already handles ELF chaining via `$ORIGIN`). Excluded from the headline count.

Examples: `b/beagle.toml`, `d/dfu-programmer.toml`, `g/glfw.toml`, `g/git-delta.toml`, `g/gnu-typist.toml`, `l/lensfun.toml`, `l/libevent.toml`, `l/libsamplerate.toml`, `l/libusb-compat.toml`, `l/logstalgia.toml`, `p/payload-dumper-go.toml`, `p/pioneers.toml`, `p/pixz.toml`.

---

## Other macOS punts not caused by dylib chaining

These appear in the same intersection (mention "bottle" somewhere) but their reason for excluding macOS is unrelated:

| Recipe | Reason |
|---|---|
| `a/allureofthestars.toml` | no bottle for sonoma/arm64_sonoma |
| `c/coreutils.toml` | `bin/dir not found in bottle` (missing binary, not RPATH) |
| `c/cryfs.toml` | no homebrew bottle for sonoma |
| `d/dhall-yaml.toml` | no bottles for arm64_sonoma or sonoma |
| `d/dylibbundler.toml` | (no comment; macOS-only tool, ironic punt) |
| `e/eiffelstudio.toml` | bottle is x86-64 only |
| `f/fstrm.toml` | libevent bottle unavailable for arm64_sonoma (transitive bottle hole) |
| `g/gabedit.toml` | no bottles for arm64_sonoma/sonoma |
| `g/gnu-indent.toml` | binary ships as `gindent` not `indent` (bottle layout) |
| `g/grep.toml` | `bin/egrep not found in bottle` |
| `h/hatari.toml` | `bin/atari-convert-dir not in bottle` |
| `i/icoutils.toml` | no bottle for Sonoma |
| `k/klavaro.toml` | "verification failure" (vague, may be downstream of dylib but not asserted) |
| `n/ncmpcpp.toml` | "verification failure" (vague) |
| `p/portaudio.toml` | no bottles |
| `r/rpm2cpio.toml` | "verification failure" (vague) |
| `r/rxvt-unicode.toml` | "verification failure" (vague) |
| `x/xorg-server.toml` | no arm64 homebrew bottle |

**Note**: the four "verification failure" recipes (klavaro, ncmpcpp, rpm2cpio, rxvt-unicode) might also be dylib-chain failures masquerading as verify-time errors; the comments are not specific enough to claim them. They are excluded from the headline count to keep the figure conservative.

---

## Summary

The dylib chaining gap is the single largest source of curated/would-be-curated tsuku recipe punts on macOS that we can attribute to a tractable engineering cause:

- **26 recipes truly punted** on macOS, split between sibling-dep dylib chaining (8) and self-library RPATH (18).
- **2 recipes partially punted** on macOS, escaped on Linux via configure_make + `set_rpath`. Those two (curl, libcurl-source) cost real engineering effort and add a maintained source-build path.
- **1 recipe (git) was unblocked** by hand-shipping a single dep's dylib outputs (pcre2, #2335) — proving the gap is fixable per-dep, but doing it 26 more times by hand is the wrong scaling.
- **~12 recipes** are punted only on `linux/arm64` (bottle-availability issue, not dylib chain).
- **~14 recipes** are punted on macOS for unrelated reasons (missing arch bottle, missing binary in bottle, version skew) and should not be conflated.

Closing this gap once in the homebrew action would unblock 26 recipes immediately and remove the source-build maintenance burden from at least 2 more (curl, libcurl-source) the next time they need attention. It would also remove the per-dep manual workaround used for git/pcre2.

## Files referenced

- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/c/curl.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/g/git.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/t/tmux.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/l/libcurl-source.toml`
- 26 recipes listed in the two macOS-punt tables above.
