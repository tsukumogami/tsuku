# Stress test 4 -- declaration completeness

## Lead

If we ship "auto-augment runtime_dependencies" as the chaining fix, is it
sufficient? Or do recipe-author-declared `runtime_dependencies` already
miss SONAMES that the bottle's binaries actually NEED? If the dep list
is incomplete, augmenting it changes nothing -- the missing lib still
isn't installed because the recipe never said it needed one.

## Setup

- Working dir: `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku`
- Tool: `./tsuku-test` mounted into a `debian:bookworm-slim` container
- TSUKU_HOME = `/tmp/tsuku` (host writes confined to `/tmp/explore_r2_decl`)
- All install/inspection runs inside containers; no host pollution.

## Recipes under test

Three recipes that ship a homebrew bottle on Linux glibc and declare
`runtime_dependencies`:

| Recipe | Declared `runtime_dependencies` | Source |
|---|---|---|
| `git` | `["pcre2"]` | `recipes/g/git.toml` |
| `wget` | `["openssl", "gettext", "libidn2", "libunistring"]` | `recipes/w/wget.toml` |
| `coreutils` | `["gmp"]` | `recipes/c/coreutils.toml` |

`git` and `wget` are the two curated tools with deps that round 1
identified. `coreutils` is the third sample -- it's not curated but
declares `runtime_dependencies` and uses the homebrew action; it ships
many binaries with diverse NEEDED lists so it surfaces more SONAMES per
install.

## Per-recipe data

### git

`git` install on debian:bookworm-slim failed five attempts because the
runtime dep `pcre2` does a Linux source build via `configure_make`,
which pulls `zig` from `65.109.105.178:443` (ziglang.org Hetzner CDN).
That host returned `connection reset by peer` and `500 Internal Server
Error` repeatedly during the test window -- a genuine upstream outage,
not a tsuku bug. To unblock, I downloaded the `git` Homebrew bottle
directly from `ghcr.io/v2/homebrew/core/git/blobs/sha256:81874ce3...`
(the URL `formulae.brew.sh` advertises for `x86_64_linux`) and ran
`readelf -d` on the un-relocated binary -- the same bytes tsuku would
extract before `homebrew_relocate` rewrites the placeholders.

**`git/2.54.0/bin/git` (pre-relocate, x86_64_linux Homebrew bottle):**

```
NEEDED  libpcre2-8.so.0
NEEDED  libz.so.1
NEEDED  libc.so.6
RPATH   @@HOMEBREW_PREFIX@@/opt/pcre2/lib :
        @@HOMEBREW_PREFIX@@/Cellar/git/2.54.0/lib :
        @@HOMEBREW_PREFIX@@/opt/gcc/lib/gcc/current :
        @@HOMEBREW_PREFIX@@/opt/bzip2/lib :
        @@HOMEBREW_PREFIX@@/opt/zlib-ng-compat/lib :
        @@HOMEBREW_PREFIX@@/opt/brotli/lib :
        @@HOMEBREW_PREFIX@@/opt/libnghttp2/lib :
        @@HOMEBREW_PREFIX@@/opt/libnghttp3/lib :
        @@HOMEBREW_PREFIX@@/opt/openssl@3/lib :
        @@HOMEBREW_PREFIX@@/opt/libngtcp2/lib :
        @@HOMEBREW_PREFIX@@/opt/libssh2/lib :
        @@HOMEBREW_PREFIX@@/opt/lz4/lib :
        @@HOMEBREW_PREFIX@@/opt/xz/lib :
        @@HOMEBREW_PREFIX@@/opt/zstd/lib :
        @@HOMEBREW_PREFIX@@/opt/ncurses/lib :
        @@HOMEBREW_PREFIX@@/opt/libedit/lib :
        @@HOMEBREW_PREFIX@@/opt/keyutils/lib :
        @@HOMEBREW_PREFIX@@/opt/krb5/lib :
        @@HOMEBREW_PREFIX@@/opt/libxcrypt/lib :
        @@HOMEBREW_PREFIX@@/opt/cyrus-sasl/lib :
        @@HOMEBREW_PREFIX@@/opt/readline/lib :
        @@HOMEBREW_PREFIX@@/opt/sqlite/lib :
        @@HOMEBREW_PREFIX@@/opt/util-linux/lib :
        @@HOMEBREW_PREFIX@@/opt/openldap/lib :
        @@HOMEBREW_PREFIX@@/opt/libunistring/lib :
        @@HOMEBREW_PREFIX@@/opt/libidn2/lib :
        @@HOMEBREW_PREFIX@@/opt/curl/lib :
        @@HOMEBREW_PREFIX@@/opt/expat/lib :
        @@HOMEBREW_PREFIX@@/lib
```

#### Per-SONAME classification (git)

| SONAME | Source library | Recipe declares it? | Classification |
|---|---|---|---|
| `libpcre2-8.so.0` | pcre2 | yes (`runtime_dependencies = ["pcre2"]`) | **covered** |
| `libz.so.1` | zlib | **no** | **declaration gap** -- there is no curated `recipes/z/zlib.toml`; only `recipes/z/zlib-ng-compat.toml`. tsuku does have an auto-resolved `zlib` library entry surfacing through homebrew satisfies (we see it in `wget`'s install -- `/tmp/tsuku/libs/zlib-1.3.2/`), so the recipe-coverage exists; the `git` recipe just doesn't declare it. |
| `libc.so.6` | glibc | n/a | **system library** |

The `git` binary's NEEDED list is short because git's curl/openssl/krb5/
etc. are loaded as plugins or used by helper binaries (git-http-fetch,
git-credential-osxkeychain, etc.). The RPATH is the inherited bottle
build configuration from Homebrew -- it lists the *transitive build-time
view*, not what the `git` ELF actually loads.

For the *runtime* view of just the `git` binary on `debian:bookworm-
slim`, the recipe's `runtime_dependencies = ["pcre2"]` covers
`libpcre2-8.so.0` (the only non-system, non-zlib NEEDED). zlib is
masked by the system in this container -- `/lib/x86_64-linux-gnu/
libz.so.1` resolves at load time.

If git's git-http-fetch / git-imap-send / etc. were also tested, the
NEEDED list expands to libcurl/libssh2/libssl/libcrypto/libidn2/
libnghttp2/libnghttp3/libssh2/libsasl2/libldap/etc. -- every entry in
the RPATH list above is a candidate; only system libs would hide them.
None of those are in `runtime_dependencies`. **The git recipe declares
the floor (just `pcre2`); the actual runtime dependency surface for
all of git's binaries is the entire RPATH list.**

### wget

`wget` install succeeded. `runtime_dependencies = ["openssl", "gettext",
"libidn2", "libunistring"]`.

**`/tmp/tsuku/tools/wget-1.25.0/bin/wget` (post-relocate, on Linux):**

```
RPATH   [$ORIGIN]
NEEDED  libuuid.so.1
NEEDED  libidn2.so.0
NEEDED  libssl.so.3
NEEDED  libcrypto.so.3
NEEDED  libz.so.1
NEEDED  libc.so.6
```

Only six NEEDED entries, but the wget recipe ships five lib deps under
`/tmp/tsuku/libs/`:

| Tsuku dep dir | Files shipped (with SONAMEs from `readelf -d`) |
|---|---|
| `libs/openssl-3.6.2/lib/` | `libssl.so.3` (SONAME `libssl.so.3`), `libcrypto.so.3` (SONAME `libcrypto.so.3`) |
| `libs/libidn2-2.3.8/lib/` | `libidn2.so.0` (SONAME `libidn2.so.0`) |
| `libs/zlib-1.3.2/lib/` | `libz.so.1` (SONAME `libz.so.1`) |
| `libs/libunistring-1.4.2/lib/` | `libunistring.so.5` (SONAME `libunistring.so.5`) |
| `libs/gettext-1.0/lib/` | `preloadable_libintl.so` (SONAME `libgnuintl.so.8`), `libgettextlib-1.0.so`, `libgettextsrc-1.0.so`, `libtextstyle.so.0`, `libgettextpo.so.0`, `libasprintf.so.0` |

#### Per-SONAME classification (wget)

| SONAME | Source library | Recipe declares it? | Classification |
|---|---|---|---|
| `libidn2.so.0` | libidn2 | yes | covered |
| `libssl.so.3` | openssl | yes | covered |
| `libcrypto.so.3` | openssl | yes | covered |
| `libz.so.1` | zlib | **no** | **declaration gap** -- zlib was auto-installed (visible in `libs/zlib-1.3.2/`), most likely as a transitive of openssl. The wget recipe doesn't list zlib, and it's pure luck that it gets pulled in. |
| `libuuid.so.1` | util-linux | **no** | **declaration gap** -- util-linux isn't in `runtime_dependencies`; it's also not visible under `libs/` after install, so on a system without the OS-provided `libuuid.so.1` the wget binary would fail to load. There is a `recipes/u/util-linux.toml`, so this is a "recipe exists, dep not declared" gap. |
| `libc.so.6` | glibc | n/a | system library |

**Three declared deps were NOT actually NEEDED by the wget binary**:
`gettext`, `libunistring`, plus most of the libs gettext ships
(`libgettextlib-1.0.so`, `libtextstyle.so.0`, `libgettextpo.so.0`,
`libasprintf.so.0`, `libgnuintl.so.8`). These declarations exist
because the macOS bottle of wget does NEED them (per the recipe
comment: `libintl.8.dylib` etc. via gettext, `libunistring.5.dylib`,
etc.), but they're carried over to the Linux glibc target where the
bottle was built without `--enable-nls` or with internationalisation
linked statically.

In other words, on Linux the recipe **over-declares** (gettext,
libunistring) AND **under-declares** (zlib, util-linux). The
over-declaration is harmless (extra installs); the under-declaration
is invisible because debian-bookworm-slim ships both `libz.so.1` and
`libuuid.so.1` in `/lib/x86_64-linux-gnu/`. **`LD_DEBUG=libs` confirms
every NEEDED resolved against the system path, not against
`/tmp/tsuku/libs/*/lib/`** -- the loader doesn't know about
`$TSUKU_HOME/libs/`, the wget binary's RPATH is just `$ORIGIN` which
points at `/tmp/tsuku/tools/wget-1.25.0/bin/`.

There is also a **major-version mismatch** that would bite a tighter
container: wget's bottle NEEDS `libunistring.so.2`, but tsuku's
`libunistring` recipe ships `libunistring.so.5`. So even if the
declared dep made it into `RPATH`, the SONAMEs don't match.

### coreutils

`coreutils` install succeeded. `runtime_dependencies = ["gmp"]`. 105
binaries shipped (b2sum, base32, basenc, dir, dircolors, factor, g[,
gb2sum, ..., gtsort, ..., uniq, wc, etc.).

**Aggregate NEEDED across ALL coreutils binaries** (`for b in
$BINDIR/*; do readelf -d "$b" | grep NEEDED; done | sort -u`):

```
libacl.so.1
libattr.so.1
libc.so.6
libgmp.so.10
```

Only four distinct SONAMEs across 105 binaries.

| Binary | NEEDED entries |
|---|---|
| `gls` | `libc.so.6` |
| `factor` | `libgmp.so.10`, `libc.so.6` |
| `gcp`, `ginstall`, `gmv` | `libacl.so.1`, ... |
| (most) | `libc.so.6` only |

#### Per-SONAME classification (coreutils)

| SONAME | Source library | Recipe declares it? | Classification |
|---|---|---|---|
| `libgmp.so.10` | gmp | yes | covered. `libs/gmp-6.3.0/lib/libgmp.so.10` is shipped; SONAME inside the file is `libgmp.so.10`, matches. |
| `libacl.so.1` | acl | **no** | **recipe-coverage gap** -- there is **no recipe for `acl`** anywhere in `recipes/` (curated *or* discovery). `gcp`, `ginstall`, `gmv` need `libacl.so.1`. On debian:bookworm-slim it loads from the OS; in a stripped container it would fail. |
| `libattr.so.1` | attr | **no** | **recipe-coverage gap** -- there is **no recipe for `attr`** anywhere in `recipes/`. Same set of binaries (`gcp` et al.) need this. Same OS-masking story. |
| `libc.so.6` | glibc | n/a | system library |

The `gmp` recipe declares Linux outputs as `lib/libgmp.so` and
`lib/libgmp.a` (no versioned SONAMEs), but the homebrew gmp install
*also* deposits `libgmp.so.10` and `libgmp.so.10.5.0` under
`libs/gmp-6.3.0/lib/` (because `install_mode = "directory"` copies the
whole bottle). So tsuku does ship the right versioned file -- the
issue is that the binary loads it via the system path
(`/lib/x86_64-linux-gnu/libgmp.so.10`) rather than tsuku's, because
of the `RPATH = $ORIGIN` problem. On a container without
`libgmp10`, factor wouldn't work even though tsuku has the file.

## Cross-recipe summary

| Recipe | NEEDED non-system SONAMEs | Covered by declared deps | Declaration gap | Recipe-coverage gap |
|---|---|---|---|---|
| `git` | `libpcre2-8.so.0`, `libz.so.1` | `libpcre2-8.so.0` | `libz.so.1` (zlib not declared; recipe exists in homebrew satisfies) | none |
| `wget` | `libidn2.so.0`, `libssl.so.3`, `libcrypto.so.3`, `libz.so.1`, `libuuid.so.1`, plus `libunistring.so.2` (note: not what tsuku ships) | `libidn2.so.0`, `libssl.so.3`, `libcrypto.so.3` | `libz.so.1`, `libuuid.so.1`. Plus over-declares `gettext`, `libunistring`, and a SONAME-mismatch on libunistring (system: `.so.2`, recipe ships: `.so.5`). | none for declared SONAMEs; `libuuid` provider (util-linux) exists |
| `coreutils` | `libgmp.so.10`, `libacl.so.1`, `libattr.so.1` | `libgmp.so.10` | `libacl.so.1`, `libattr.so.1` | **acl + attr have no tsuku recipe at all** |

## Answer to the lead

**Existing recipes' `runtime_dependencies` declarations are NOT
sufficient to cover all NEEDED non-system SONAMEs.** Of the three
recipes tested, all three have at least one undeclared NEEDED
SONAME, and `coreutils` has two undeclared SONAMEs whose source
libraries don't have any tsuku recipe (curated or discovered).

This means an "auto-augment `runtime_dependencies` from `homebrew_satisfies`
metadata" fix would not solve the problem on its own. The recipe author
omits libraries that a) the macOS bottle doesn't reference and b) Linux
system loaders mask on the test matrix. A complete fix needs at least
one of:

1. **Bottle-aware NEEDED extraction at install time.** After extracting
   the homebrew bottle, run `readelf -d` over every binary the recipe
   exports, collect the union of NEEDED entries, subtract OS-provided
   SONAMEs (`libc.so.6`, `libpthread.so.0`, the `linux-vdso.so.1`
   class), then verify each remaining SONAME against tsuku-shipped
   library files (with SONAME match, not filename match). Anything
   that doesn't resolve becomes either an auto-resolved dep (if a
   recipe ships that SONAME) or a hard error (no recipe ships it).

2. **Validator-side warning.** Run the same NEEDED extraction during
   `tsuku validate`, surface unmatched SONAMEs to the recipe author as
   "your bottle's binaries reference SONAME X but no declared dep
   ships it; either declare a dep that does, or document the system
   library expectation". Doesn't fix runtime, but stops new recipes
   from shipping broken.

3. **RPATH rewrite or link map.** Independent of the declaration
   problem -- even with all the right deps shipped, the binary's
   RPATH = `$ORIGIN` doesn't reach `$TSUKU_HOME/libs/openssl-3.6.2/lib`.
   The recipes "work" today only because the OS provides the SONAME on
   the loader's default path. The declaration-completeness fix and the
   RPATH-chain fix have to ship together; either one alone leaves the
   gap.

The "homebrew bottle has a 30-deep RPATH list of libs we'd never declare"
example from `git` (29 distinct `@@HOMEBREW_PREFIX@@/opt/<lib>/lib`
entries) is a strong argument that hand-curation cannot keep up. **An
augment-runtime-deps design that relies only on what the recipe author
typed will keep missing libs unless it pairs with binary inspection.**

## Notes on test conditions

- The `pcre2` source build path needed `zig` from `ziglang.org`'s
  Hetzner CDN; that host returned `connection reset by peer` and `500
  Internal Server Error` for git's pcre2 dep across five attempts.
  Direct bottle inspection via `formulae.brew.sh` and `ghcr.io`
  produced the data without that build path.
- All inspection was performed in fresh `debian:bookworm-slim`
  containers; host `$TSUKU_HOME` was untouched.
- `LD_DEBUG=libs` was used to confirm the loader's actual search path
  and which file each NEEDED entry resolved against. In every
  resolved-from-tsuku case it was `linux-vdso` (kernel) or
  `/lib/x86_64-linux-gnu/<lib>` (system), never `/tmp/tsuku/libs/`.

## Files referenced

- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/g/git.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/w/wget.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/c/coreutils.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/g/gmp.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/p/pcre2.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/g/gettext.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/l/libidn2.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/l/libunistring.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/recipes/u/util-linux.toml`
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/actions/homebrew_relocate.go`
- `/tmp/explore_r2_decl/wget_libs.txt` -- per-lib SONAME data + ldd resolutions for wget (host scratch; not on the tsuku repo).
- `/tmp/explore_r2_decl/coreutils_libs.txt` -- coreutils per-binary NEEDED + libgmp resolutions.
- `/tmp/explore_r2_decl/git_direct.txt` -- direct fetch of `git/2.54.0` Linux bottle, raw NEEDED + RPATH.
