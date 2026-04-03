# Exemplar Recipes

Curated recipes demonstrating distinct authoring patterns. Read these as starting
points when writing similar recipes.

---

## Binary Download

**Recipe:** `recipes/t/trivy.toml`
**Pattern:** github_archive with os/arch mapping, simple verification

Trivy shows the core binary distribution pattern: a single `github_archive` step
with `os_mapping` and `arch_mapping` to handle platform-specific asset names.
No dependencies, no conditionals -- just download, extract, and verify.

Start here if your tool publishes pre-built binaries as GitHub release assets.

---

## Homebrew-Backed

**Recipe:** `recipes/z/zoxide.toml`
**Pattern:** homebrew formula + install_binaries, single binary

Zoxide demonstrates the simplest Homebrew pattern: one step to fetch the bottle,
one step to register the binary. Works on macOS and glibc Linux (Homebrew doesn't
build musl bottles).

Start here if your tool has a Homebrew formula and you don't need musl support.

---

## Source Build with Dependencies

**Recipe:** `recipes/c/curl.toml`
**Pattern:** download + configure_make with library deps, set_rpath, signature verification

Curl shows a full source build workflow: download with GPG signature verification,
`setup_build_env` to expose dependency headers/libs, `configure_make` with
`--with-openssl` and `--with-zlib`, then `set_rpath` so the binary finds its
shared libraries at runtime.

Start here for C/C++ tools that need to link against other tsuku-managed libraries.

---

## Platform-Conditional (libc Splits)

**Recipe:** `recipes/p/pcre2.toml`
**Pattern:** three-way split -- homebrew on glibc, source build on musl, homebrew on macOS

PCRE2 demonstrates the most common conditional structure:
- `when = { os = ["linux"], libc = ["glibc"] }` -- Homebrew bottle (fast path)
- `when = { os = ["linux"], libc = ["musl"] }` -- full source build (no bottles for musl)
- `when = { os = ["darwin"] }` -- Homebrew

Each branch has its own install steps but shares the same `[verify]` section.
This is the go-to template for any library or tool that needs to work on both
glibc and musl Linux.

---

## Ecosystem-Delegated

Multiple exemplars, one per ecosystem:

### Cargo (Rust)

**Recipe:** `recipes/t/try-rs.toml`
**Pattern:** cargo_install with crate name and executables

### npm (Node)

**Recipe:** `recipes/z/zx.toml`
**Pattern:** npm_install with package name and single executable

### pipx (Python)

**Recipe:** `recipes/r/ruff.toml`
**Pattern:** pipx_install with `runtime_dependencies = []` (compiled binary, no Python at runtime)

### Go

**Recipe:** `recipes/s/staticcheck.toml`
**Pattern:** go_install with full module path

### Gem (Ruby)

**Recipe:** `recipes/j/jekyll.toml`
**Pattern:** gem_install with `{install_dir}` in verify command, zig runtime dependency for native extensions

---

## Library with Outputs and rpath

**Recipe:** `recipes/l/libcurl-source.toml`
**Pattern:** library type, directory install mode, multiple outputs (.so, .a, .pc, headers)

Libcurl-source is a dedicated library recipe showing how to register shared
libraries, static archives, pkg-config files, and headers as outputs. Uses
`install_mode = "directory"` to preserve the full directory tree. Includes
GPG signature verification.

Start here if you're packaging a library that other tsuku recipes will depend on.

Also see `recipes/c/curl.toml` for `set_rpath` usage in a binary that links
against bundled shared libraries.

---

## Custom Verification

**Recipe:** `recipes/j/jekyll.toml`
**Pattern:** verify command using `{install_dir}` to target the tsuku-installed copy

Jekyll demonstrates why custom verification matters: Ruby gems install binaries
to a non-standard location, so the verify command needs an absolute path via
`{install_dir}` to avoid testing a system-installed copy.

**Secondary exemplars:**
- `recipes/p/poetry.toml` -- custom pattern for parenthetical version output: `"Poetry (version {version})"`
- `recipes/s/skaffold.toml` -- uses `skaffold version` subcommand instead of `--version` flag
