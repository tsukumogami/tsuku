# Lead: Classify recipes' `metadata.runtime_dependencies` by dep `Type`

**Date:** 2026-05-02
**Working dir:** `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku`
**Method:** Empirical scan of `recipes/` plus embedded recipes in `internal/recipe/recipes/`. Script: `/tmp/classify_runtime_deps.py`. JSON dump: `/tmp/classify_runtime_deps_result.json`.

## Summary (3 lines)

- **298** consumer recipes declare a non-empty `metadata.runtime_dependencies` list.
- **all-library: 219; all-tool: 41; mixed: 38; has-missing: 0.**
- No missing deps — every name in every list resolves either in `recipes/<a>/<name>.toml` or `internal/recipe/recipes/<name>.toml`.

## Question

The reviewer of `docs/designs/DESIGN-tsuku-homebrew-dylib-chaining.md` proposes dispatching on the dep recipe's `Type` instead of introducing a `chained_lib_dependencies` field. This survey quantifies how the existing `runtime_dependencies` universe partitions under that dispatch:

- Library deps would feed RPATH chaining for the consumer.
- Tool deps would continue through the existing wrapper-PATH mechanism.

## Method

1. Walked `recipes/**/*.toml` for files whose `[metadata]` block contains a `runtime_dependencies = [...]` list with at least one entry. The TOML is indented (two-space) inside the table, so the regex must allow leading whitespace; my first pass missed this and only found 5 files. After the fix: 298.
2. For each dep name, looked it up at:
   - `recipes/<first-letter>/<name>.toml` (most cases), or
   - `internal/recipe/recipes/<name>.toml` (embedded core recipes such as `bash`, `nodejs`, `gcc-libs`, `perl`, `make`, `meson`).
3. Read the dep's `[metadata].type`:
   - `"library"` -> classified `library`.
   - field absent or `""` -> classified `tool` (the default).
   - file not found -> `missing`.
4. Categorised the consumer:
   - **all-library** -- every dep is a library recipe.
   - **all-tool** -- every dep is a tool recipe.
   - **mixed** -- both kinds present.
   - **has-missing** -- at least one dep doesn't resolve.

The 25-recipe gap between the 323 grep hits and the 298 parsed consumers is fully accounted for: 23 are recipes with `runtime_dependencies = []` (the field exists but the list is empty -- e.g. `kiota`, `pdftk-java`, `lc0`, `osm2pgrouting`, `gopass`), and 2 are pure comment mentions (`pcre2.toml`, `curl.toml` mentions the field in a TODO).

## Counts

| Category    | Count |
|-------------|-------|
| all-library | 219   |
| all-tool    | 41    |
| mixed       | 38    |
| has-missing | 0     |
| **total**   | **298** |

Global dep-occurrence counts across all 298 consumers (a dep appearing in N consumers' lists counts N times):

- `library`: 401 occurrences
- `tool`: 91 occurrences

Library deps dominate by ~4.4x.

## Samples

### all-library (10 of 219)

- `dfu-programmer`: [libusb]
- `darcs`: [gmp]
- `desktop-file-utils`: [glib, gettext]
- `dhall-yaml`: [gmp]
- `dissent`: [cairo, gettext, glib, graphene]
- `dhall-lsp-server`: [gmp]
- `dfu-util`: [libusb]
- `diff-pdf`: [cairo, glib, gettext]
- `unbound`: [libevent, libnghttp2, openssl]
- `util-linux`: [gettext]

### all-tool (10 of 41)

- `testkube`: [helm, kubernetes-cli]
- `jjui`: [jj]
- `wails`: [go]
- `watch`: [ncurses]
- `whisper-cpp`: [sdl2]
- `zls`: [zig]
- `bashdb`: [bash]
- `bochs`: [sdl2]
- `bedtools`: [xz]
- `vet`: [go]

Note: several "tool" deps in this column ship libraries that consumers actually link against (`ncurses`, `sdl2`, `xz`). They are tool-classified only because their recipe's `type` field is unset or empty -- see "Caveat" below.

### mixed (full 38)

| Consumer | Deps (name:type) |
|---|---|
| fluent-bit | libyaml:library, luajit:tool, openssl:library |
| freeciv | cairo:library, freetype:library, gettext:library, glib:library, readline:library, sdl2:tool, sqlite:tool, zstd:library |
| zsh | ncurses:tool, pcre2:library |
| allureofthestars | gmp:library, sdl2:tool |
| aarch64-elf-gdb | gmp:library, mpfr:library, ncurses:tool, readline:library, xz:tool, zstd:library |
| apr-util | apr:tool, openssl:library |
| arm-none-eabi-gdb | gmp:library, mpfr:library, ncurses:tool, readline:library, xz:tool, zstd:library |
| aircrack-ng | openssl:library, pcre2:library, sqlite:tool |
| biber | openssl:library, perl:tool |
| libzip | xz:tool, zstd:library |
| logstalgia | freetype:library, glew:tool, libpng:library, pcre2:library, sdl2:tool |
| libxmlb | glib:library, xz:tool, zstd:library |
| libtiff | jpeg-turbo:library, xz:tool, zstd:library |
| libbluray | fontconfig:tool, freetype:library, libudfread:library |
| libbladerf | libusb:library, ncurses:tool |
| httpd | apr:tool, apr-util:tool, brotli:library, libnghttp2:library, openssl:library, pcre2:library |
| help2man | gettext:library, perl:tool |
| hatari | libpng:library, sdl2:tool |
| homebank | cairo:library, fontconfig:tool, freetype:library, glib:library, gettext:library |
| rattler-build | openssl:library, xz:tool |
| riscv64-elf-gdb | gmp:library, mpfr:library, ncurses:tool, readline:library, xz:tool, zstd:library |
| rxvt-unicode | fontconfig:tool, freetype:library |
| pdnsrec | lua:tool, openssl:library |
| po4a | gettext:library, perl:tool |
| c-blosc2 | lz4:tool, zstd:library |
| cdrdao | lame:tool, mad:library |
| gource | freetype:library, glew:tool, libpng:library, pcre2:library, sdl2:tool |
| gnu-apl | cairo:library, glib:library, libpng:library, pcre2:library, readline:library, sqlite:tool, gettext:library |
| gsmartcontrol | cairo:library, glib:library, smartmontools:tool, gettext:library, pcre2:library |
| git-xet | git-lfs:tool, openssl:library |
| sngrep | ncurses:tool, openssl:library |
| irssi | gettext:library, glib:library, openssl:library, perl:tool |
| i386-elf-gdb | gmp:library, mpfr:library, ncurses:tool, readline:library, xz:tool, zstd:library |
| i686-elf-grub | xz:tool, gettext:library |
| mender-cli | openssl:library, xz:tool |
| osm2pgsql | luajit:tool, proj:library |
| ncmpcpp | libmpdclient:library, ncurses:tool, readline:library |
| ncdu | ncurses:tool, zstd:library |

## Caveat: type-misclassified library deps

The mixed list and the "all-tool" list both surface deps that are **classified as `tool`** (their recipe lacks an explicit `type = "library"`) but **consumers link against their libraries**. Top offenders, ordered by occurrence in mixed recipes:

| Tool-classified dep | Mixed-recipe occurrences | Reality |
|---|---|---|
| `xz` | 10 | Ships `liblzma` -- consumers link it |
| `ncurses` | 9 | Ships `libncurses` -- consumers link it |
| `sdl2` | 5 | Ships `libSDL2` -- consumers link it |
| `perl` | 4 | Tool, but consumers may need its embedded interp libs |
| `sqlite` | 3 | Ships `libsqlite3` -- consumers link it |
| `fontconfig` | 3 | Ships `libfontconfig` -- consumers link it |
| `glew` | 2 | Ships `libGLEW` -- consumers link it |
| `luajit` | 2 | Ships `libluajit` -- consumers link it |
| `apr` | 2 | Ships `libapr` -- consumers link it |

Spot check: `recipes/x/xz.toml` has `type = ""`. `ncurses.toml`, `sqlite.toml`, `fontconfig.toml` have no `type` field at all.

This means the dispatch-on-type proposal would **not** automatically chain library paths for these consumers -- their dep would resolve as "tool" and fall through to wrapper-PATH, which doesn't help dylib resolution. Either:

1. The 9 mostly-library tool recipes above need their `type` reclassified (some are dual-purpose: `xz` ships both `xz` binary and `liblzma`), or
2. `chained_lib_dependencies` stays a separate field so authors mark per-consumer that "I link `liblzma` from xz", or
3. Type becomes a richer enum (e.g. `tool+library` for dual-purpose recipes).

## What "type-on-dep dispatch" would change

Under the reviewer's proposal, every consumer in **all-library (219)** would get its deps' `lib/` directories merged into the consumer's RPATH automatically. Every consumer in **mixed (38)** would get its library deps chained and its tool deps still resolved via wrapper-PATH. No consumer in **all-tool (41)** would see new behavior beyond status quo.

So the proposal directly affects **257 of 298** recipes (86%). The 38 mixed consumers are the ones that prove the dispatch *is* useful (they need both behaviors, and a single field can't express that). But they are also the cases where mis-typed "tool" deps that actually ship libraries (`xz`, `ncurses`, `sdl2`, etc.) would silently fail to chain unless types are reviewed.

## Files

- Survey script: `/tmp/classify_runtime_deps.py`
- Raw JSON output: `/tmp/classify_runtime_deps_result.json`
- This report: `wip/research/explore_tsuku-homebrew-dylib-chaining_r2_lead-classify-runtime-deps.md`
