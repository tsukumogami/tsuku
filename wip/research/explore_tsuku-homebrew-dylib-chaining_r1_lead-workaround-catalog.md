# Workaround catalog: recipe-side responses to dylib chaining

## Lead

Catalog the recipe-side workarounds that already exist in the registry for
the dylib chaining problem. Question: are recipes converging on the same
shape (signal to lift to a primitive) or each one bespoke (signal that
recipe-side fixes scale)?

## Method

Searched `recipes/*/*.toml` for four candidate workaround patterns:

1. `set_rpath` chains using `{libs_dir}/<dep>-{deps.<dep>.version}/lib`
2. Source-build escapes (configure_make on one platform + homebrew on another)
3. Library outputs shipped explicitly via `install_mode = "directory"` with
   `outputs = ["lib/lib*.dylib", ...]`
4. `mode = "output"` verify with non-version pattern (tolerates bottle vs
   distro version skew)

## Pattern 1: set_rpath chains (curl pattern)

**Count: 1 recipe.**

| Recipe | Notes |
|--------|-------|
| `recipes/c/curl.toml` | Linux-only; `rpath = "$ORIGIN/../lib:{libs_dir}/openssl-{deps.openssl.version}/lib:{libs_dir}/zlib-{deps.zlib.version}/lib"` |

**Convergence: N/A — sample size 1.**

`{libs_dir}` interpolation is used in exactly one recipe in the entire
registry. `deps.<name>.version` interpolation is also used only in
`curl.toml`. The interpolation primitives themselves exist (so the executor
supports them) but no other recipe author has reached for the pattern yet.

The curl comment explicitly notes the workaround is Linux-only and that
darwin coverage is excluded "until runtime_dependencies supports dylib
chaining" — recognising that the equivalent macOS pattern (`@loader_path`
or `LC_RPATH` rewriting via `install_name_tool`) is missing entirely.

## Pattern 2: Source-build escape from the bottle

**Count: 3 recipes.**

| Recipe | Linux strategy | macOS strategy | Reason |
|--------|---------------|---------------|--------|
| `recipes/c/curl.toml` | configure_make from upstream tarball | not supported (`supported_os = ["linux"]`) | Linux bottle pulls libnghttp3.9.dylib via RPATH; bundling transitive dylibs unsupported |
| `recipes/p/pcre2.toml` | configure_make with `--disable-bzip2 --disable-readline --disable-shared --enable-static` | homebrew bottle | Linuxbrew bottle hard-codes `libbz2.so.1.0` (RHEL ships `.so.1`); musl loader cannot resolve sibling sonames; Fedora segfault in dynamic build |
| `recipes/a/apr.toml` | configure_make on musl only; homebrew on glibc | homebrew bottle | Linux bottle works on glibc but not musl |

`recipes/l/libcurl-source.toml` is a fourth source-built libcurl variant,
but the header explicitly marks it as a sandbox testing fixture rather
than a real recipe ("Source-built libcurl for sandbox testing... Replaces
Homebrew-bottled libcurl which fails in Debian containers due to
OPENLDAP_2.200 vs OPENLDAP_2.5 symbol version mismatch"). Same root cause
(transitive dep mismatch from the bottle), different scope.

**Convergence: low.** Each escape disables a different set of optional
features (curl drops nghttp2/nghttp3/libpsl; pcre2 drops bzip2/readline/
shared linking; apr is a clean configure with no flags). The shared shape
is "configure_make replaces homebrew on the platform where the bottle is
broken," but the configure flags are bespoke per recipe and per failure
mode (symbol version skew, missing transitive .so, musl loader behaviour,
Fedora segfault).

## Pattern 3: Explicit lib outputs from a homebrew bottle (libnghttp3 / pcre2 darwin pattern)

**Count: ~138 recipes (essentially every `type = "library"` recipe).**

Filter: recipes that combine `action = "homebrew"` with
`install_mode = "directory"` and explicit `lib/lib*.{so,dylib}` entries
in `outputs`. 138 recipes match; 100% of them carry `type = "library"`.

Subset that ships **versioned soname dylibs** (the `lib*.<N>.dylib`
pattern that libnghttp3 uses to satisfy the curl bottle's `@rpath`
lookup): 109 recipes.

**Convergence: very high — this is the registry's standard library shape,
not a workaround.** What was special about libnghttp3 in PR #2336 is that
the bottle was being installed for the *consumer* (curl) rather than for
its own sake. The recipe pattern itself is identical to every other
library bottle in the registry: ship `lib/libfoo.dylib`,
`lib/libfoo.<N>.dylib`, `lib/libfoo.a`, `lib/pkgconfig/libfoo.pc`, plus
headers.

This means tsuku already has a stable convention for "publish a homebrew
library bottle's payload." What's missing is a way to *consume* that
payload from a sibling tools dir at runtime — which is the dylib chaining
problem itself, not a workaround for it.

## Pattern 4: mode = "output" verify to tolerate version skew

**Count: 28 recipes total use `mode = "output"`. Of those, 7 explicitly
cite homebrew-vs-distro version skew as the reason.**

Genuine version-skew workarounds (the openjdk pattern):

| Recipe | Skew between |
|--------|-------------|
| `recipes/a/awscli.toml` | Homebrew bottle (no versioned macOS zips) vs Linux PGP-verified install |
| `recipes/b/black.toml` | apk_install on Alpine vs PyPI release |
| `recipes/g/git.toml` | Homebrew bottle (currently 2.54) vs Alpine apk (currently 2.47) |
| `recipes/o/openjdk.toml` | Homebrew formula vs Alpine apk |
| `recipes/p/pre-commit.toml` | Alpine package vs upstream PyPI |
| `recipes/p/python.toml` | calver release tag vs python.exe version |
| `recipes/s/shellcheck.toml` | apk_install on Alpine vs GitHub release |

The remaining 21 `mode = "output"` recipes use it for unrelated reasons
(tool prints usage to stderr, exits non-zero, has no `--version` flag,
embeds no version info, .app bundle verification, etc.).

**Convergence: high within the subset.** All 7 version-skew cases use
the exact same TOML shape:

```toml
[verify]
command = "<tool> --version"
mode = "output"
pattern = "<tool-name-prefix>"  # or omitted entirely
reason = "<homebrew vs other source> version skew"
```

This is a recipe-side convention that has crystallised cleanly. The
pattern doesn't need to become a primitive — it already *is* a primitive
(`mode = "output"`); recipes just need to know to reach for it. The skill
docs may be the right home for a worked example.

## Summary

| Pattern | Count | Convergence | Read |
|---------|-------|-------------|------|
| set_rpath chain with `{libs_dir}/<dep>-{deps.dep.version}/lib` | 1 | N/A | Primitive exists in executor; no recipe author has copied curl's pattern. Either the use case is rare or the pattern is too unergonomic to spread. |
| Source-build escape from broken bottle | 3 (+1 test fixture) | Low — same shape, bespoke flags per failure mode | The "swap configure_make in for homebrew on the broken platform" shape is recognisable, but every escape reflects a different bottle pathology. Hard to lift to a primitive without losing the per-recipe judgement about which optional features to drop. |
| Ship versioned soname dylibs from a bottle | 109 (versioned) / 138 (any lib output) | Very high — this is the standard library recipe shape | Already a primitive in everything but name. Doesn't address the dylib chaining problem; it's the building block the chaining problem needs to consume. |
| `mode = "output"` verify for version skew | 7 (within 28 total mode=output uses) | High — identical TOML shape across all 7 | Already a working primitive; the convention has settled. No tsuku-core change needed; only docs/examples. |

## Implication for the exploration

The convergence/divergence picture is mixed and the per-pattern reads point in different directions:

- **Pattern 1 (set_rpath chains)**: A primitive exists but adoption is
  zero outside curl. Either this is the right primitive and recipes
  haven't found it yet (docs problem), or the primitive is too unergonomic
  for recipe authors to reach for (UX problem). The curl recipe's own
  comment says darwin can't use it because no equivalent exists for
  Mach-O — so the primitive is also incomplete on macOS.

- **Pattern 2 (source-build escape)**: This is the "give up on the bottle"
  fallback. Every instance is a one-off response to a specific bottle
  pathology. No shape to lift.

- **Pattern 3 (publish lib outputs)**: Already converged; this is
  tsuku's standard library shape. The chaining problem needs this
  pattern's *consumer side* (a bottle that already publishes
  `libnghttp3.9.dylib` cannot be picked up by a sibling
  `curl-X.Y.Z/bin/curl` at runtime without RPATH/install_name_tool
  rewriting).

- **Pattern 4 (mode = output for version skew)**: Already a stable
  recipe-side convention. Not a tsuku-core problem.

**Net signal**: the dylib chaining problem is *not* solved by a converged
recipe pattern that can be promoted. Pattern 1 is the closest fit but
exists only for ELF on Linux and only in one recipe. The macOS half of
the problem has no recipe-side workaround at all — `recipes/c/curl.toml`
explicitly drops darwin support and points at "until runtime_dependencies
supports dylib chaining" as the missing piece. That's a tsuku-core
deliverable, not a recipe-author guideline.

The high convergence of Pattern 3 (every library recipe ships its libs
the same way) means the upstream half of the chain is ready. The missing
piece is the downstream half: a primitive that lets a consumer recipe
declare "at runtime, my binary needs to find these sibling tools' lib/
directories on its loader path" and have tsuku rewrite Mach-O LC_RPATH /
ELF DT_RPATH accordingly.
