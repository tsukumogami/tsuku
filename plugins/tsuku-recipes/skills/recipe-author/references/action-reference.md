# Action Reference

Lookup table of all tsuku recipe actions grouped by category. For each action:
key parameters, platform constraints, and a usage note.

---

## Download and Archive Composites

### github_archive

Downloads and extracts a GitHub release asset matching a pattern.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | Yes | GitHub `owner/repo` |
| `asset_pattern` | string | Yes | Pattern with `{version}`, `{os}`, `{arch}` placeholders |
| `binaries` | []string | Yes | Binary names to install |
| `strip_dirs` | int | No | Directory levels to strip from archive (default: 0) |
| `install_mode` | string | No | `"binaries"` (default), `"directory"`, `"directory_wrapped"` |
| `os_mapping` | map | No | Remap OS names (e.g., `{ darwin = "macOS" }`) |
| `arch_mapping` | map | No | Remap arch names (e.g., `{ amd64 = "x86_64" }`) |

Platform: All. Decomposes to: download_file + extract + install_binaries.

### github_file

Downloads a single file from a GitHub release.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | Yes | GitHub `owner/repo` |
| `file_path` | string | Yes | Asset filename pattern |
| `binaries` | []string | Yes | Binary names to install |
| `install_mode` | string | No | Install mode |

Platform: All. Decomposes to: download_file + install_binaries.

### download

Downloads a file from any URL. Supports placeholder expansion.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | Yes | URL with `{version}`, `{os}`, `{arch}` placeholders |
| `dest` | string | No | Destination filename |
| `checksum_url` | string | No | URL to checksum file |
| `signature_url` | string | No | URL to signature file |
| `checksum_algo` | string | No | Hash algorithm (default: sha256) |
| `os_mapping` | map | No | Remap OS names |
| `arch_mapping` | map | No | Remap arch names |

Platform: All. Decomposes to: download_file.

### download_archive

Downloads, extracts, and installs binaries in a single composite action.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | Yes | Archive URL with placeholders |
| `archive_format` | string | No | `tar.gz`, `tar.xz`, `zip`, etc. (auto-detected) |
| `binaries` | []string | Yes | Binaries to install |
| `strip_dirs` | int | No | Directory levels to strip |
| `install_mode` | string | No | Install mode |
| `os_mapping` | map | No | Remap OS names |
| `arch_mapping` | map | No | Remap arch names |

Platform: All. Decomposes to: download_file + extract + install_binaries.

### fossil_archive

Downloads and extracts a tarball from a Fossil SCM repository.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | Yes | Fossil repo URL |
| `project_name` | string | Yes | Project name for URL construction |
| `binaries` | []string | Yes | Binaries to install |
| `tag_prefix` | string | No | Version tag prefix |
| `version_separator` | string | No | Separator in version numbers |
| `strip_dirs` | int | No | Directory levels to strip |

Platform: All.

---

## Ecosystem Composites

These actions handle the full install lifecycle for their ecosystem: resolve
dependencies at eval time, generate lockfiles, then build/install deterministically.

### cargo_install

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `crate` | string | Yes | Crate name |
| `executables` | []string | Yes | Binary names produced |
| `cargo_path` | string | No | Custom cargo binary path |

Platform: All. Dependencies: rust (eval + install time).

### npm_install

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `package` | string | Yes | npm package name |
| `executables` | []string | Yes | CLI commands produced |
| `npm_path` | string | No | Custom npm binary path |

Platform: All. Dependencies: nodejs (eval + install + runtime).

### pipx_install

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `package` | string | Yes | PyPI package name |
| `executables` | []string | Yes | CLI commands produced |
| `pipx_path` | string | No | Custom pipx binary path |
| `python_path` | string | No | Custom Python binary path |

Platform: All. Dependencies: python-standalone (eval + install time).
If the package ships compiled binaries (like ruff), set
`runtime_dependencies = []` to skip runtime Python.

**Version selection.** Recipes do not declare a version pin. tsuku
resolves `latest` to the newest PyPI release whose `requires_python`
metadata is satisfied by the bundled `python-standalone` binary's
major.minor. PEP 440 prereleases (e.g., `2.17.9rc1`) and yanked
releases are skipped automatically; `.post` releases are accepted.
When no PyPI release is compatible, resolution fails with a typed
`*ResolverError` (`ErrTypeNoCompatibleRelease`) of the shape:

```
pypi resolver: no release of <package> is compatible with bundled
Python <X.Y> (latest is <V>, requires Python <Z>)
```

User pins (`tsuku install foo@x.y`) bypass this filter — explicit
pins are authoritative. To recover when a recipe's auto-resolution
hits the no-compatible-release branch, either pin via the CLI, or
file an issue against the recipe so a follow-up can pin to a known-
good range. See `docs/designs/DESIGN-pipx-pypi-version-pinning.md`
for the full design.

### go_install

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `module` | string | Yes | Go module path (e.g., `honnef.co/go/tools/cmd/staticcheck`) |
| `executables` | []string | Yes | Binary names produced |
| `go_version` | string | No | Required Go version |

Platform: All. Dependencies: go (eval + install time).

### gem_install

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `gem` | string | Yes | Gem name |
| `executables` | []string | Yes | CLI commands produced |
| `gem_path` | string | No | Custom gem binary path |

Platform: All. Dependencies: ruby (eval + install + runtime).

### cpan_install

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `distribution` | string | Yes | CPAN distribution name |
| `module` | string | No | Module name (if different) |
| `executables` | []string | Yes | CLI commands produced |
| `perl_version` | string | No | Required Perl version |
| `mirror` | string | No | CPAN mirror URL |

Platform: All. Dependencies: perl (eval + install + runtime).

### nix_install

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `package` | string | Yes | Nix package attribute |
| `executables` | []string | Yes | Binary names produced |

Platform: Linux only. Dependencies: nix-portable (eval + install time).

---

## Build System Primitives

### configure_make

Autotools-style build: ./configure && make && make install.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `source_dir` | string | Yes | Source directory (supports `{version}`) |
| `configure_args` | []string | No | Arguments to ./configure |
| `make_targets` | []string | No | Make targets (default: all, install) |
| `executables` | []string | No | Expected output binaries |
| `prefix` | string | No | Install prefix |
| `skip_configure` | bool | No | Skip the configure step |

Platform: All. Dependencies: make, zig, pkg-config (auto-detected).
Pair with `setup_build_env` when the recipe has library dependencies.

### cmake_build

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `source_dir` | string | Yes | Source directory |
| `cmake_args` | []string | No | CMake configuration arguments |
| `executables` | []string | No | Expected output binaries |
| `build_type` | string | No | CMake build type (Release, Debug, etc.) |

Platform: All. Dependencies: cmake, make, zig, pkg-config.

### meson_build

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `source_dir` | string | Yes | Source directory |
| `meson_args` | []string | No | Meson configuration arguments |
| `executables` | []string | No | Expected output binaries |
| `buildtype` | string | No | Meson build type |
| `wrap_mode` | string | No | Meson wrap mode |

Platform: All. Dependencies: meson, ninja, zig, patchelf (Linux).

### setup_build_env

Configures PATH, PKG_CONFIG_PATH, CPPFLAGS, and LDFLAGS from the recipe's
declared dependencies. No parameters -- reads from the action context.

Platform: All. Place this step before any build action when the recipe declares
library dependencies.

---

## Homebrew

### homebrew

Downloads a Homebrew bottle from GHCR and relocates prefix paths.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `formula` | string | Yes | Homebrew formula name |

Platform: macOS and glibc Linux (Homebrew bottles aren't built for musl).
Follow with `install_binaries` to register specific binaries.

### homebrew_relocate

Fixes @@HOMEBREW_PREFIX@@ placeholders in bottle files.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `paths` | []string | No | Paths to process |
| `prefix` | string | No | Replacement prefix |

Platform: macOS (primarily). Dependencies: patchelf on Linux.
Rarely needed directly -- the `homebrew` composite handles this.

---

## System Package Managers

These actions check whether system packages are installed. They don't install
packages automatically (that would need sudo).

| Action | Platform | Key Parameters |
|--------|----------|----------------|
| `apt_install` | Debian/Ubuntu | `packages`, `fallback`, `unless_command` |
| `apk_install` | Alpine | `packages`, `fallback`, `unless_command` |
| `dnf_install` | Fedora/RHEL | `packages`, `fallback`, `unless_command` |
| `pacman_install` | Arch | `packages`, `fallback`, `unless_command` |
| `zypper_install` | openSUSE | `packages`, `fallback`, `unless_command` |
| `brew_install` | macOS | `packages`, `tap`, `fallback`, `unless_command` |
| `brew_cask` | macOS | `packages`, `tap`, `fallback`, `unless_command` |

Common parameters:
- `packages` ([]string): Package names to check
- `fallback` (string): Message shown if packages are missing
- `unless_command` (string): Skip check if this command exists

APT also has `apt_repo` (add repository) and `apt_ppa` (add Ubuntu PPA).

---

## File Operations

### install_binaries

Copies binaries (or entire directories) into the tool's install location and
creates PATH symlinks in `$TSUKU_HOME/bin/`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `binaries` | []string | No | Binary paths relative to work dir |
| `outputs` | []string | No | Additional output paths (libraries, etc.) |
| `executables` | []string | No | Alias for binaries |
| `install_mode` | string | No | `"binaries"` (default), `"directory"`, `"directory_wrapped"` |

Use `install_mode = "directory"` for tools that need their full directory tree
(libraries, data files). Use `"directory_wrapped"` to generate wrapper scripts.

### chmod

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `files` | []string | Yes | Files to modify |
| `mode` | int | No | Permission mode (default: 0755) |

### set_rpath

Modifies ELF RPATH so binaries find their bundled libraries at runtime.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `binaries` | []string | Yes | ELF binaries to patch |
| `rpath` | string | No | RPATH value (default: `$ORIGIN/../lib`) |
| `create_wrapper` | bool | No | Create wrapper script instead of patching |

Platform: Linux only.

### set_env

Creates an env.sh file that tsuku sources when activating the tool.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `vars` | map | Yes | Environment variable name-value pairs |

### text_replace

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `file` | string | Yes | File to modify |
| `pattern` | string | Yes | Search pattern |
| `replacement` | string | Yes | Replacement string |
| `regex` | bool | No | Treat pattern as regex (default: false) |

---

## Shell Integration

### install_shell_init

Writes shell initialization scripts (sourced on shell startup).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `source_file` | string | Mutual | Path to init script |
| `source_command` | string | Mutual | Command that outputs init script |
| `target` | string | Yes | Target name for the init file |
| `shells` | []string | No | Shell types: `bash`, `zsh`, `fish` |

### install_completions

Writes shell completion scripts.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `source_file` | string | Mutual | Path to completion script |
| `source_command` | string | Mutual | Command that outputs completions |
| `target` | string | Yes | Target name for the completion file |
| `shells` | []string | No | Shell types: `bash`, `zsh`, `fish` |

---

## Special Actions

### app_bundle

Installs a macOS .app bundle and optionally symlinks CLI tools.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | Yes | Download URL for ZIP or DMG |
| `checksum` | string | Yes | SHA256 of the download |
| `app_name` | string | Yes | Name of the .app bundle |
| `binaries` | []string | No | CLI tools to symlink from the bundle |
| `symlink_applications` | bool | No | Symlink to /Applications |

Platform: macOS only.

### run_command

Executes an arbitrary shell command. Use sparingly -- prefer structured actions.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | Yes | Shell command to execute |
| `description` | string | No | Human-readable description of what this does |
| `working_dir` | string | No | Working directory |
| `requires_sudo` | bool | No | If true, step is skipped (tsuku doesn't use sudo) |

Platform: All. Validation warns on dangerous patterns (rm, eval, exec, piped curl).

### require_system

Validates that a system dependency is present and optionally checks its version.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | Yes | Command to check |
| `version_flag` | string | No | Flag to get version (e.g., `--version`) |
| `version_regex` | string | No | Regex to extract version from output |
| `min_version` | string | No | Minimum acceptable version |

### require_command

Checks that a command exists in PATH. Simpler than require_system when you
don't need version checking.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | Yes | Command to check |
