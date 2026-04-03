---
name: recipe-author
description: |
  Guides authoring tsuku recipe TOML files for both the central registry
  and distributed (.tsuku-recipes/) repositories. Covers actions, version
  providers, platform targeting, verification, and dependency declaration.

  Use this skill when writing or modifying recipe TOML files. For scaffolding
  a new recipe interactively, run `tsuku create` first -- it generates a
  starter TOML from a GitHub repo. This skill covers everything `tsuku create`
  doesn't handle: platform conditionals, custom verification, library outputs,
  dependency chains, and distributed recipe publishing.
---

# Recipe Authoring

## Recipe Structure

Every recipe is a TOML file with four sections:

```toml
[metadata]        # Name, description, platform support
[version]         # How to resolve the latest version (often inferred)
[[steps]]         # Ordered installation actions
[verify]          # Post-install verification command
```

Optional sections: `[[resources]]` for extra downloads, `[[patches]]` for source patches.

## Actions

Actions are the building blocks of `[[steps]]`. Each step has an `action` field
and action-specific parameters.

### Download and Archive

| Action | Description |
|--------|-------------|
| `github_archive` | Download and extract a GitHub release asset |
| `github_file` | Download a single file from a GitHub release |
| `download` | Download from any URL with `{version}`, `{os}`, `{arch}` placeholders |
| `download_archive` | Download + extract + install binaries in one step |
| `fossil_archive` | Download from a Fossil SCM tarball endpoint |

### Ecosystem

| Action | Description |
|--------|-------------|
| `cargo_install` | Install a Rust crate via Cargo |
| `npm_install` | Install an npm package globally |
| `pipx_install` | Install a Python package in an isolated venv |
| `go_install` | Install a Go module |
| `gem_install` | Install a Ruby gem |
| `cpan_install` | Install a Perl distribution |
| `nix_install` | Install via Nix (Linux only) |

### Package Managers

| Action | Description |
|--------|-------------|
| `homebrew` | Download a Homebrew bottle (macOS and glibc Linux) |
| `apt_install` | Check for Debian/Ubuntu packages |
| `apk_install` | Check for Alpine packages |
| `dnf_install` | Check for Fedora/RHEL packages |
| `pacman_install` | Check for Arch packages |
| `zypper_install` | Check for openSUSE packages |
| `brew_install` | Check for system Homebrew packages (macOS) |
| `brew_cask` | Install macOS GUI apps via Homebrew Cask |

### Build Systems

| Action | Description |
|--------|-------------|
| `configure_make` | Autotools-style ./configure && make |
| `cmake_build` | CMake build |
| `meson_build` | Meson + Ninja build |
| `setup_build_env` | Configure PATH/CFLAGS/LDFLAGS from dependencies |

### File Operations and Shell

| Action | Description |
|--------|-------------|
| `install_binaries` | Copy binaries to install dir; register for PATH symlinking |
| `chmod` | Set file permissions |
| `set_rpath` | Modify ELF RPATH for library resolution (Linux) |
| `set_env` | Export environment variables via env.sh |
| `text_replace` | Find/replace in files |
| `install_shell_init` | Write shell initialization scripts |
| `install_completions` | Write shell completion scripts |
| `homebrew_relocate` | Fix @@HOMEBREW_PREFIX@@ paths in bottles |

### Special

| Action | Description |
|--------|-------------|
| `app_bundle` | Install a macOS .app bundle from ZIP/DMG |
| `run_command` | Execute a shell command |
| `require_system` | Validate a system dependency exists |
| `require_command` | Check that a command is in PATH |

See [references/action-reference.md](references/action-reference.md) for full
parameter tables and usage notes.

## Version Providers

The `[version]` section controls how tsuku resolves the latest version.
Most providers auto-detect from actions -- you can often omit `[version]` entirely.

| Source | Resolves From | Auto-Detects From |
|--------|---------------|-------------------|
| `github` | GitHub releases/tags | `github_archive`, `github_file` |
| `npm` | npm registry | `npm_install` |
| `pypi` | PyPI | `pipx_install` |
| `crates_io` | crates.io | `cargo_install` |
| `rubygems` | RubyGems.org | `gem_install` |
| `goproxy` | proxy.golang.org | `go_install` |
| `metacpan` | MetaCPAN | `cpan_install` |
| `homebrew` | Homebrew API | `homebrew` |
| `cask` | Homebrew Cask API | -- (set `cask` field) |
| `tap` | Third-party Homebrew tap | -- (set `tap` + `formula`) |
| `fossil` | Fossil VCS timeline | `fossil_archive` |
| `nixpkgs` | NixOS channels | -- |
| `go_toolchain` | go.dev/dl | -- |

When auto-detection works, skip the `[version]` section. Override with explicit
fields when the inferred source is wrong or you need `tag_prefix`, `github_repo`,
or other provider-specific config.

## Platform Conditionals

Steps can target specific platforms with a `when` clause.

```toml
[[steps]]
action = "homebrew"
formula = "pcre2"
when = { os = ["linux"], libc = ["glibc"] }

[[steps]]
action = "configure_make"
source_dir = "pcre2-{version}"
when = { os = ["linux"], libc = ["musl"] }

[[steps]]
action = "homebrew"
formula = "pcre2"
when = { os = ["darwin"] }
```

Available `when` fields:

| Field | Values | Example |
|-------|--------|---------|
| `os` | `["linux"]`, `["darwin"]` | `when = { os = ["linux"] }` |
| `arch` | `"amd64"`, `"arm64"` | `when = { arch = "amd64" }` |
| `libc` | `["glibc"]`, `["musl"]` | `when = { libc = ["musl"] }` |
| `linux_family` | `"debian"`, `"rhel"`, `"alpine"`, `"arch"`, `"suse"` | `when = { linux_family = "debian" }` |
| `gpu` | `["nvidia"]`, `["amd"]`, `["intel"]`, `["none"]` | `when = { gpu = ["nvidia"] }` |
| `platform` | `["linux/amd64"]` | `when = { platform = ["linux/arm64"] }` |
| `package_manager` | `"brew"`, `"apt"`, `"dnf"` | `when = { package_manager = "apt" }` |

Multiple fields AND together. Empty `when` (or omitted) matches all platforms.

See [references/platform-reference.md](references/platform-reference.md) for the
libc decision tree and migration templates.

## Verification

Every tool recipe needs a `[verify]` section. Libraries are exempt.

**Version mode** (default) -- extracts and compares version:
```toml
[verify]
command = "trivy --version"
pattern = "{version}"
```

**Output mode** -- checks the command runs, no version comparison:
```toml
[verify]
command = "mytool --help"
mode = "output"
reason = "Tool prints no parseable version string"
```

**Format transforms** -- normalize extracted versions:

| Transform | Input | Output |
|-----------|-------|--------|
| `semver` | `1.2.3-beta` | `1.2.3` |
| `semver_full` | `v1.2.3-beta+build` | `1.2.3-beta+build` |
| `strip_v` | `v1.2.3` | `1.2.3` |
| `raw` | (as-is) | (as-is) |
| `calver` | `2024.01.15` | `2024.01.15` |

Set via `version_format` in `[verify]` or `[metadata]`.

Tools that exit non-zero on `--version`: set `exit_code` in `[verify]`.

See [references/verification-reference.md](references/verification-reference.md)
for the full decision flowchart and common patterns.

## Dependencies

Declare dependencies in `[metadata]`:

```toml
[metadata]
dependencies = ["openssl", "zlib"]          # needed at install time
runtime_dependencies = ["python-standalone"] # needed when tool runs
extra_dependencies = ["pkg-config"]          # extends auto-detected deps
```

Most ecosystem actions (cargo_install, npm_install, etc.) auto-detect their
toolchain dependency. Use `extra_dependencies` to add more without replacing
the implicit set.

See [references/dependencies-reference.md](references/dependencies-reference.md)
for library dependencies, build environment setup, and resolution order.

## Distributed Recipes (.tsuku-recipes/)

Distribute recipes from any GitHub repo without contributing to the central registry.

**Setup:**
1. Create `.tsuku-recipes/` at your repo root
2. Add `recipe-name.toml` files (filename must match `metadata.name`)
3. Push to main or master branch

**Install syntax:**
```bash
tsuku install owner/repo              # single-recipe repo
tsuku install owner/repo:recipe       # named recipe from multi-recipe repo
tsuku install owner/repo@v1.0.0       # pinned version
tsuku install owner/repo:recipe@v1.0  # named recipe + version
```

First install from an unregistered source prompts for confirmation. Use
`tsuku registry add owner/repo` to pre-approve, or set `strict_registries = true`
in `$TSUKU_HOME/config.toml` to block unregistered sources entirely.

See [references/distributed-reference.md](references/distributed-reference.md)
for manifest.json, cache behavior, and the trust model.

## Bundled References

| File | When to Read |
|------|-------------|
| [action-reference.md](references/action-reference.md) | Looking up action parameters or platform constraints |
| [platform-reference.md](references/platform-reference.md) | Writing when clauses, handling glibc/musl splits |
| [verification-reference.md](references/verification-reference.md) | Choosing verification mode or format transforms |
| [dependencies-reference.md](references/dependencies-reference.md) | Declaring dependencies or building libraries |
| [distributed-reference.md](references/distributed-reference.md) | Publishing recipes from your own repo |
| [exemplar-recipes.md](references/exemplar-recipes.md) | Finding a starting-point recipe for your pattern |

## Additional Guides

Central registry contributors who have the full tsuku repo can find extended
guides in `docs/guides/`. External consumers who installed this plugin via
sparsePaths won't have access to that directory -- the bundled references
above cover the same material in condensed form.

| Guide | Topic |
|-------|-------|
| `docs/guides/GUIDE-actions-and-primitives.md` | Deep dive on action types and determinism |
| `docs/guides/GUIDE-hybrid-libc-recipes.md` | Full libc migration templates |
| `docs/guides/GUIDE-recipe-verification.md` | Verification troubleshooting |
| `docs/guides/GUIDE-library-dependencies.md` | Library auto-provisioning internals |
| `docs/guides/GUIDE-distributed-recipe-authoring.md` | Extended distributed authoring guide |

## Validation

Before submitting a recipe:

```bash
tsuku validate path/to/recipe.toml
tsuku validate --strict path/to/recipe.toml        # treats warnings as errors
tsuku validate --check-libc-coverage path/to/recipe.toml  # library libc coverage
```

For distributed recipes, test with sandbox mode:

```bash
tsuku install --recipe .tsuku-recipes/my-tool.toml --sandbox
```
