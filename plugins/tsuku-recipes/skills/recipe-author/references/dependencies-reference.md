# Dependencies Reference

How to declare dependencies in tsuku recipes, how auto-provisioning works,
and how to set up build environments for source builds.

---

## Dependency Fields

Declare dependencies in `[metadata]`:

| Field | Purpose | Behavior |
|-------|---------|----------|
| `dependencies` | Install-time dependencies | **Replaces** auto-detected deps from actions |
| `runtime_dependencies` | Dependencies needed when the tool runs | Installed alongside the tool |
| `extra_dependencies` | Additional install-time deps | **Extends** auto-detected deps (additive) |
| `extra_runtime_dependencies` | Additional runtime deps | **Extends** auto-detected runtime deps |

### Auto-Detection

Most ecosystem actions auto-detect their toolchain as a dependency:

| Action | Auto-Detected Dependency |
|--------|--------------------------|
| `cargo_install` | rust |
| `npm_install` | nodejs |
| `pipx_install` | python-standalone |
| `go_install` | go |
| `gem_install` | ruby |
| `cpan_install` | perl |
| `nix_install` | nix-portable |
| `configure_make` | make, zig, pkg-config |
| `cmake_build` | cmake, make, zig, pkg-config |
| `meson_build` | meson, ninja, zig, patchelf (Linux) |

Use `extra_dependencies` when you need something beyond what the action implies.
Use `dependencies` only when you need to override the auto-detected set entirely.

### Example: Adding a library dependency

```toml
[metadata]
name = "curl"
extra_dependencies = ["openssl", "zlib"]
```

This keeps the auto-detected deps from the build action and adds openssl and zlib.

### Example: Runtime dependency override

```toml
[metadata]
name = "ruff"
runtime_dependencies = []
```

Ruff is distributed via PyPI (pipx_install) but ships a compiled binary --
it doesn't need Python at runtime. Setting `runtime_dependencies = []` prevents
tsuku from pulling python-standalone as a runtime dep.

---

## Dependency Timing

Dependencies are needed at different phases:

| Phase | Field | When |
|-------|-------|------|
| Eval time | (from action type) | `tsuku eval` -- generating lockfiles, resolving versions |
| Install time | `dependencies` / `extra_dependencies` | `tsuku install` -- building and installing |
| Runtime | `runtime_dependencies` / `extra_runtime_dependencies` | When the installed tool runs |

Ecosystem composites (cargo_install, npm_install, etc.) need their toolchain
at eval time to generate lockfiles, at install time to build, and sometimes
at runtime.

---

## Build Environment (setup_build_env)

Source builds that depend on libraries need `setup_build_env` to configure
compiler flags and paths.

```toml
[metadata]
name = "curl"
extra_dependencies = ["openssl", "zlib"]

[[steps]]
action = "download"
url = "https://curl.se/download/curl-{version}.tar.gz"

[[steps]]
action = "extract"
archive = "curl-{version}.tar.gz"

[[steps]]
action = "setup_build_env"

[[steps]]
action = "configure_make"
source_dir = "curl-{version}"
configure_args = ["--with-openssl", "--with-zlib"]
executables = ["curl"]
```

`setup_build_env` reads the recipe's dependencies and sets:
- `PATH` -- adds dependency bin directories
- `PKG_CONFIG_PATH` -- adds dependency lib/pkgconfig directories
- `CPPFLAGS` -- adds -I flags for dependency include directories
- `LDFLAGS` -- adds -L flags for dependency lib directories

Place it after extract and before the build action.

---

## Library Dependencies

### Declaring a library recipe

```toml
[metadata]
name = "openssl"
type = "library"
binaries = ["bin/openssl"]
```

Libraries use `type = "library"`. They can still have binaries (like the
openssl CLI) but their primary purpose is providing headers and .so/.dylib
files that other recipes link against.

### Library outputs

Use `install_binaries` with `install_mode = "directory"` and `outputs` to
register library files:

```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["lib/libcurl.so", "lib/libcurl.a", "lib/pkgconfig/libcurl.pc", "include/curl"]
```

### RPATH for shared libraries

When a tool links against bundled .so files, set the RPATH so the dynamic
linker finds them at runtime:

```toml
[[steps]]
action = "set_rpath"
binaries = ["bin/curl"]
rpath = "$ORIGIN/../lib"
```

This makes the binary look for .so files relative to its own location.
Linux only -- macOS uses different mechanisms (install_name_tool, handled
by homebrew_relocate).

### Link dependencies

For tools that share a library installed by another recipe:

```toml
[[steps]]
action = "link_dependencies"
library = "openssl"
version = "3.2.0"
```

Creates symlinks from the tool's lib/ directory to the shared library location.

---

## Dependency Resolution Order

When tsuku installs a recipe with dependencies:

1. Resolve all dependencies recursively (transitive deps included)
2. Install dependencies in topological order (leaves first)
3. Run `setup_build_env` to expose installed deps to the build
4. Build and install the target recipe
5. Install runtime dependencies if not already present

Circular dependencies are rejected at validation time.

---

## Step-Level Dependencies

Individual steps can declare their own dependencies, separate from the
recipe-level declaration:

```toml
[[steps]]
action = "configure_make"
source_dir = "my-tool-{version}"
dependencies = ["pkg-config"]
```

Step-level dependencies override the step's auto-detected deps. They don't
affect other steps in the recipe.

---

## Satisfies Map

Libraries can declare which ecosystem packages they satisfy, preventing
duplicate installs:

```toml
[metadata.satisfies]
homebrew = ["pcre2"]
```

This tells tsuku that if another recipe needs the `pcre2` Homebrew formula,
this recipe already provides it.
