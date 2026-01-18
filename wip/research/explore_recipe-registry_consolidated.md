# Recipe Registry Separation - Consolidated Research

## Executive Summary

This research informs the strategic decision to separate recipes into "critical" (embedded in CLI binary) and "non-critical" (external registry) categories. The goal is to reduce binary size while ensuring the CLI can always install its own dependencies.

## Current State

### Recipe Embedding Mechanism

**Location:** `internal/recipe/embedded.go:13`
```go
//go:embed recipes/*/*.toml
var embeddedRecipes embed.FS
```

**Current structure:**
- 171 recipes embedded from `internal/recipe/recipes/{a-z}/*.toml`
- All recipes compiled into the binary at build time
- `recipes/` directory at repo root is empty (placeholder)

**Loading priority chain (loader.go:73-115):**
1. In-memory cache (fastest)
2. Local recipes (`~/.tsuku/recipes/*.toml` user overrides)
3. Embedded recipes (binary-embedded)
4. Registry (remote fetch via GitHub raw, cached to `~/.tsuku/registry/`)

### Recipe Statistics

- **Total recipes:** 171
- **By type:** ~154 tools, 17 libraries
- **With dependencies:** 20 recipes (12%)
- **Nix-based (Tier 3):** 2 recipes

### Action Dependencies Analysis

**Actions that require tsuku-managed tools:**

| Action | Required Tool | Dependency Type |
|--------|--------------|-----------------|
| `go_install`, `go_build` | go | EvalTime + InstallTime |
| `cargo_install`, `cargo_build` | rust | EvalTime + InstallTime |
| `npm_install`, `npm_exec` | nodejs | InstallTime + Runtime + EvalTime |
| `pip_install`, `pip_exec`, `pipx_install` | python | InstallTime + Runtime |
| `gem_install`, `gem_exec` | ruby | InstallTime + Runtime + EvalTime |
| `cpan_install` | perl | InstallTime + Runtime |
| `nix_install`, `nix_realize` | nix-portable | InstallTime (auto-bootstrapped) |
| `homebrew`, `homebrew_relocate` | patchelf (Linux) | InstallTime |
| `configure_make`, `cmake_build` | make, zig, pkg-config | InstallTime |
| `meson_build` | meson, ninja, zig, patchelf (Linux) | InstallTime |

**Critical dependency chains:**
- `rust` recipe enables all Cargo-based installations
- `go` recipe enables all Go-based installations
- `python-standalone` enables pipx ecosystem
- `patchelf` enables Homebrew bottles on Linux
- `zig` enables C/C++ compilation without system compilers

### CI/CD Testing Architecture

**Three-layer golden file validation:**
1. `validate-golden-recipes.yml` - Recipe changes trigger golden file validation
2. `validate-golden-code.yml` - Plan generation code changes trigger ALL golden file validation
3. `validate-golden-execution.yml` - Golden file changes trigger execution on platform matrix

**Current exclusion mechanisms:**
- `testdata/golden/exclusions.json` - Platform-specific (265 entries)
- `testdata/golden/execution-exclusions.json` - Recipe-level (10 entries)
- `testdata/golden/code-validation-exclusions.json` - Code bypass (7 entries)

**Test matrix tiers:**
- CI tests (PR/push): 11 Linux + 5 macOS tests
- Scheduled tests (nightly): +7 slow tests

## Critical Recipe Candidates

Based on dependency analysis, these recipes MUST be embedded:

### Tier 1: Language Toolchains (Enable Ecosystems)
- `go` - Enables go_install, go_build
- `rust` - Enables cargo_install, cargo_build
- `nodejs` - Enables npm_install, npm_exec
- `python-standalone` - Enables pip_install, pip_exec, pipx_install
- `ruby` - Enables gem_install, gem_exec
- `perl` - Enables cpan_install

### Tier 2: Build System Dependencies
- `make` - Required by configure_make, cmake_build
- `zig` - C compiler (configure_make, cmake_build, meson_build)
- `cmake` - Required by cmake_build
- `ninja` - Required by meson_build
- `meson` - Required by meson_build
- `pkg-config` - Required by configure_make, cmake_build
- `patchelf` - Linux RPATH modification (homebrew_relocate, meson_build)

### Tier 3: Their Dependencies
- `zlib` - Dependency of curl, others
- `openssl` - Dependency of curl, others
- `libyaml` - Dependency of ruby

### Special Cases
- `nix-portable` - Auto-bootstrapped (not a recipe, hardcoded download)

**Estimated critical recipe count:** ~15-20 recipes

## Key Constraints Identified

1. **Embedded recipes must be self-sufficient**: Critical recipes and their transitive dependencies must all be embedded
2. **Golden file testing assumes all recipes embedded**: Current CI validates all recipes as if embedded
3. **Registry fallback exists**: Non-embedded recipes can be fetched from GitHub raw URLs
4. **Local overrides have priority**: Users can always override any recipe via `~/.tsuku/recipes/`
5. **Version resolution still works**: Version providers (GitHub, Homebrew, PyPI) work regardless of embedding

## Questions for Design Phase

1. How should we mark a recipe as "critical" in TOML metadata?
2. Should the build system automatically compute transitive closure of critical recipes?
3. How should golden file testing differ between critical and non-critical recipes?
4. What happens if a non-critical recipe is unavailable (network failure)?
