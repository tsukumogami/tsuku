# Phase 2 Research: Codebase Analyst

## Lead 1: Recipe Skill Content Needs

### Findings

**Action Registry (60+ actions across 8 categories):**
1. Download/Extract Composites: download, download_file, download_archive, extract
2. GitHub Composites: github_archive, github_file
3. Package Manager Primitives: apt_install, apt_repo, apt_ppa, apk_install, dnf_install, dnf_repo, pacman_install, zypper_install, brew_install, brew_cask
4. Ecosystem Composites: cargo_install, cargo_build, npm_install, npm_exec, pip_install, pip_exec, pipx_install, gem_install, gem_exec, go_install, go_build, cpan_install
5. Build System Primitives: configure_make, cmake_build, meson_build, setup_build_env
6. System Configuration: group_add, service_enable, service_start, require_command, require_system
7. File Operation Primitives: install_binaries, install_libraries, link_dependencies, set_rpath, set_env, chmod, text_replace, apply_patch, apply_patch_file
8. Specialized Composites: app_bundle, nix_install, nix_realize, nix_portable, fossil_archive, homebrew, homebrew_relocate, shell_init, completions

**Version Providers (15 types):** GitHub, Homebrew, Tap, Custom, PyPI, npm, RubyGems, MetaCPAN, Cask, NixPkgs, GoProxy, GoToolchain, CratesIO, FossilTimeline

**Existing Guide Documentation (11 guides):**
- GUIDE-actions-and-primitives.md: Full action taxonomy, decomposition model, determinism
- GUIDE-recipe-verification.md: Version modes, format transforms, output modes
- GUIDE-library-dependencies.md: Auto-provisioning, build env setup
- GUIDE-hybrid-libc-recipes.md: glibc/musl platform splitting
- GUIDE-distributed-recipe-authoring.md: .tsuku-recipes/ setup, naming, caching, testing
- GUIDE-distributed-recipes.md: Installation syntax, registry management, strict mode
- GUIDE-system-dependencies.md: Package manager actions, implicit platform constraints
- GUIDE-troubleshooting-verification.md: Tier-based library verification
- GUIDE-plan-based-installation.md: Plan format, execution
- GUIDE-local-llm.md: LLM integration
- GUIDE-command-not-found.md: Command-not-found integration

**Exemplar Recipes Found:**
1. age.toml: Binary download (github_archive) with os/arch mapping
2. curl.toml: Source build with deps, set_rpath, library dependencies
3. dav1d.toml: Platform-conditional (libc-specific, Homebrew vs apk)
4. double-conversion.toml: Library with outputs field, platform splits
5. dotslash.toml: Ecosystem (cargo_install)
6. dprint.toml: Ecosystem (npm_install)

**Platform Conditional Syntax:**
- Inline: `when = { os = ["linux"], libc = ["glibc"] }`
- Table: `[steps.when]` with os, libc, linux_family arrays
- Filters: os (darwin, linux), libc (glibc, musl), linux_family (debian, rhel, alpine, arch, suse), gpu

### Implications for Requirements
- Skill must teach composite-to-primitive decomposition model
- Version provider integration with templating syntax ({version}, {version.url})
- Platform targeting via when clauses is critical for hybrid recipes
- Dependency propagation for library recipes (outputs field, setup_build_env)
- Must leverage existing 11 guide files rather than duplicating content

## Lead 2: Distributed Recipe Authoring

### Findings

**Discovery Mechanism:**
- Directory: .tsuku-recipes/ at repo root (no config required)
- Naming: kebab-case TOML files matching recipe name
- Optional manifest: .tsuku-recipes/manifest.json for advanced layouts
- Branch probing: tries main/master, falls back to flat layout

**Installation Syntax:**
- owner/repo -- default/only recipe
- owner/repo:recipe -- named recipe
- owner/repo@version -- specific version
- owner/repo:recipe@version -- full specification

**GitHub API Infrastructure (internal/distributed/):**
- Two-tier client: authenticated API + unauthenticated raw downloads
- TTL-based caching with configurable TSUKU_RECIPE_CACHE_TTL
- GITHUB_TOKEN optional for rate limits (60 -> 5000 req/hr)

**Trust Model:**
- First install from new source prompts confirmation (-y to suppress)
- Registries in $TSUKU_HOME/config.toml
- Strict mode: strict_registries = true blocks untrusted sources
- tsuku registry add/remove commands

**Testing Workflow:**
- tsuku validate .tsuku-recipes/recipe.toml
- tsuku validate --strict .tsuku-recipes/recipe.toml
- tsuku install --recipe .tsuku-recipes/recipe.toml --sandbox

### Implications for Requirements
- Skill should provide templates for single-recipe and multi-recipe repos
- Must clarify that .tsuku-recipes/ requires no setup -- just create dir and add TOML
- Caching behavior matters for authoring: "when do users see my changes?"
- Test-before-publish workflow: validate -> sandbox -> verify

## Lead 3: Plugin Infrastructure

### Findings
- No .claude-plugin/ or marketplace.json exists in tsuku yet
- .claude/ has settings.local.json only
- koto pattern: marketplace.json at .claude-plugin/, plugins/ directory with plugin.json per plugin
- koto plugin.json lists skills in a skills array

### Implications for Requirements
- Need to create full plugin infrastructure from scratch
- Follow koto pattern: .claude-plugin/marketplace.json + plugins/tsuku-recipes/
- Committed settings.json enables local marketplace + shirabe remote

## Summary

60+ actions, 15 version providers, 11 guide files, and a full distributed recipe system. The recipe-author skill should use a hybrid approach: quick-reference tables for common lookups, pointers to the 11 existing guides for deep dives, and 5-8 curated exemplar recipes. Distributed recipe authoring (.tsuku-recipes/) is well-supported with discovery, caching, trust model, and testing -- the skill needs to cover repo setup patterns, install syntax, and the test-before-publish workflow.
