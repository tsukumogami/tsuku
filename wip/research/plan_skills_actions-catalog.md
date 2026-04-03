# Tsuku Action System Catalog

Complete catalog of all registered actions in `internal/actions/`.

## Legend

- **Type**: Primitive (deterministic core or ecosystem), Composite (decomposes to primitives), or System (delegates to OS)
- **Decomposable**: ✓ = implements Decomposable interface, ✗ = does not
- **Deterministic**: ✓ = produces identical results given identical inputs, ✗ = has residual non-determinism
- **Network**: ✓ = requires network access, ✗ = works offline

---

## Core Primitives (Deterministic)

| Action | Type | Decomposable | Key Parameters | Platform | Dependencies | Notes |
|--------|------|---|---|---|---|---|
| `download_file` | Primitive | ✗ | url, checksum, checksum_algo, dest | All | None | Requires checksum; use for static URLs. Deterministic with hash verification. |
| `extract` | Primitive | ✗ | archive, format, dest, strip_dirs, files | All | None | Supports tar.gz, tar.xz, tar.bz2, zip, tar, auto-detect. Path traversal protection. |
| `chmod` | Primitive | ✗ | files, mode | All | None | Makes files executable; default mode 0755. |
| `install_binaries` | Primitive | ✗ | outputs, executables, install_mode | All | None | Copies binaries to InstallDir. Modes: binaries (default), directory, directory_wrapped. |
| `set_env` | Primitive | ✗ | vars | All | None | Creates env.sh with environment variable exports. |
| `set_rpath` | Primitive | ✗ | binaries, rpath, create_wrapper | All | None | Linux only: modifies ELF RPATH for library resolution. Default: $ORIGIN/../lib |
| `install_libraries` | Primitive | ✗ | patterns | All | None | Copies libraries matching glob patterns; preserves symlinks. |
| `link_dependencies` | Primitive | ✗ | library, version | All | None | Creates symlinks from tool/lib to shared library location. |
| `apply_patch_file` | Primitive | ✗ | file OR data, strip, subdir | All | None | Applies patch; file or inline data (mutually exclusive). |
| `text_replace` | Primitive | ✗ | file, pattern, replacement, regex | All | None | Text replacement in files; supports literal or regex patterns. |
| `homebrew_relocate` | Primitive | ✗ | paths, prefix | macOS only | patchelf (Linux) | Relocates @@HOMEBREW_PREFIX@@ in Homebrew bottles. |

---

## Ecosystem Primitives (Non-Deterministic)

| Action | Type | Decomposable | Key Parameters | Platform | Dependencies | Notes |
|--------|------|---|---|---|---|---|
| `cargo_build` | Primitive | ✗ | source_dir, executables, lock_data, target, features, locked, offline | All | rust | Deterministic via Cargo.lock + offline mode. SOURCE_DATE_EPOCH=0. |
| `go_build` | Primitive | ✗ | module, version, executables, go_sum, go_version, install_module, cgo_enabled | All | go | Deterministic via go.sum capture + GOPROXY=off + GOSUMDB=off. |
| `cmake_build` | Primitive | ✗ | source_dir, cmake_args, executables, build_type | All | cmake, make, zig, pkg-config | Non-deterministic: depends on system compilers. |
| `configure_make` | Primitive | ✗ | source_dir, configure_args, make_targets, executables, prefix, skip_configure | All | make, zig, pkg-config | Non-deterministic: autotools depend on compiler + system libs. |
| `meson_build` | Primitive | ✗ | source_dir, meson_args, executables, buildtype, wrap_mode | All | meson, ninja, zig, patchelf (Linux only) | Non-deterministic: depends on system compilers. |
| `pip_exec` | Primitive | ✗ | package, version, executables, locked_requirements, python_version, has_native_addons | All | python-standalone | Deterministic: locked requirements + hash checking. |
| `npm_exec` | Primitive | ✗ | source_dir + command OR package + version + package_lock, node_version, npm_path, ignore_scripts | All | nodejs | Deterministic: uses npm ci with package-lock.json. SOURCE_DATE_EPOCH set. |
| `pip_install` | Primitive | ✗ | source_dir OR requirements, python_version, use_hashes, output_dir, python_path, constraints | All | python | Non-deterministic: residual variance from wheel selection + bytecode. |
| `cpan_install` | Primitive | ✗ | distribution, module, executables, perl_version, cpanfile, mirror, mirror_only, offline | All | perl | Deterministic: cpanfile.snapshot + SOURCE_DATE_EPOCH=0. |
| `gem_exec` | Primitive | ✗ | package, version, executables, lock_data, bundler_version | All | ruby | Deterministic: Gemfile.lock pinning. |
| `nix_realize` | Primitive | ✗ | flake_ref OR package, executables, locks (flake_lock, locked_ref, system), derivation_path | Linux only | nix-portable | Deterministic: flake.lock captured at eval time. |

---

## Composite Actions (Decomposable)

| Action | Type | Decomposable | Key Parameters | Platform | Dependencies | Notes |
|--------|------|---|---|---|---|---|
| `download` | Composite | ✓ | url, dest, checksum_url OR signature_url, checksum_algo, os_mapping, arch_mapping | All | None | Decomposes to download_file. URL with placeholders; dynamic checksum. |
| `download_archive` | Composite | ✓ | url, archive_format, binaries, strip_dirs, os_mapping, arch_mapping, install_mode | All | None | Download + Extract + InstallBinaries. Decomposes to download_file, extract. |
| `github_archive` | Composite | ✓ | repo, asset_pattern, binaries, strip_dirs, install_mode | All | None | Downloads GitHub release asset matching pattern. |
| `github_file` | Composite | ✓ | repo, file_path, binaries, install_mode | All | None | Downloads single file from GitHub release. |
| `fossil_archive` | Composite | ✓ | repo, project_name, tag_prefix, version_separator, binaries, strip_dirs | All | None | Downloads/extracts from Fossil SCM tarball endpoint. |
| `npm_install` | Composite | ✓ | package, executables, npm_path | All | nodejs (EvalTime too) | Decomposes to npm_exec. EvalTime for package-lock.json generation. |
| `pip_install` | Composite | ✓ | source_dir OR requirements, python_version, use_hashes, output_dir, python_path, constraints | All | python (EvalTime too) | Decomposes to pip_exec. EvalTime for locked requirements generation. |
| `cargo_install` | Composite | ✓ | crate, executables, cargo_path | All | rust (EvalTime too) | Decomposes to cargo_build. EvalTime for Cargo.lock generation. |
| `go_install` | Composite | ✓ | module, executables, go_version (optional) | All | go (EvalTime too) | Decomposes to go_build. EvalTime for go.sum generation. |
| `gem_install` | Composite | ✓ | gem, executables, gem_path | All | ruby (EvalTime + Runtime) | Decomposes to gem_exec. EvalTime for Gemfile.lock generation. |
| `cpan_install` | Composite | ✓ (via gem_exec) | distribution, module, executables, perl_version, cpanfile, mirror, mirror_only, offline | All | perl (EvalTime too) | Decomposes to cpan_install primitive. Snapshot-based. |
| `pipx_install` | Composite | ✓ | package, executables, pipx_path, python_path | All | python-standalone (EvalTime too) | Decomposes to pip_exec. Isolated venv per package. |
| `nix_install` | Composite | ✓ | package, executables | Linux only | nix-portable (EvalTime too) | Decomposes to nix_realize. Package manager isolation. |
| `homebrew` | Composite | ✓ | formula | macOS/Linux | patchelf (Linux only) | Downloads Homebrew bottle from GHCR; relocates @@HOMEBREW_PREFIX@@. |

---

## Command/Script Execution

| Action | Type | Decomposable | Key Parameters | Platform | Dependencies | Notes |
|--------|------|---|---|---|---|---|
| `run_command` | Primitive | ✗ | command, description, working_dir, requires_sudo | All | None | Executes shell command. Skips if requires_sudo=true. Requires network (conservative). |

---

## Shell Integration (Lifecycle)

| Action | Type | Decomposable | Key Parameters | Platform | Dependencies | Notes |
|--------|------|---|---|---|---|---|
| `install_shell_init` | Primitive | ✗ | source_file OR source_command, target, shells | All | None | Writes shell init scripts to $TSUKU_HOME/share/shell.d/{target}.{shell}. |
| `install_completions` | Primitive | ✗ | source_file OR source_command, target, shells | All | None | Writes completion scripts to $TSUKU_HOME/share/completions/{shell}/{target}. |

---

## System Package Managers (External)

| Action | Type | Decomposable | Key Parameters | Platform | Dependencies | Notes |
|--------|------|---|---|---|---|---|
| `apt_install` | System | ✗ | packages, fallback, unless_command | Linux (Debian) | None | Checks if packages installed; errors if missing. Network required. |
| `apt_repo` | System | ✗ | url, key_url, key_sha256 | Linux (Debian) | None | Adds APT repository with GPG key. Network required. |
| `apt_ppa` | System | ✗ | ppa | Linux (Ubuntu) | None | Adds Ubuntu PPA. Network required. |
| `brew_install` | System | ✗ | packages, tap, fallback, unless_command | macOS | None | Stub: checks packages; requires system Homebrew. Network required. |
| `brew_cask` | System | ✗ | packages, tap, fallback, unless_command | macOS | None | Installs GUI apps via Homebrew Casks. Network required. |
| `dnf_install` | System | ✗ | packages, fallback, unless_command | Linux (Fedora/RHEL) | None | Checks if packages installed; errors if missing. Network required. |
| `pacman_install` | System | ✗ | packages, fallback, unless_command | Linux (Arch) | None | Checks if packages installed; errors if missing. Network required. |
| `apk_install` | System | ✗ | packages, fallback, unless_command | Linux (Alpine) | None | Checks if packages installed; errors if missing. Network required. |
| `zypper_install` | System | ✗ | packages, fallback, unless_command | Linux (openSUSE) | None | Checks if packages installed; errors if missing. Network required. |

---

## System Configuration (Structured)

| Action | Type | Decomposable | Key Parameters | Platform | Dependencies | Notes |
|--------|------|---|---|---|---|---|
| `require_system` | Primitive | ✗ | command, version_flag, version_regex, min_version | All | None | Validates system dependency installed; checks version if regex provided. |
| `require_command` | System | ✗ | command | All | None | Checks if command exists in PATH. |
| `group_add` | System | ✗ | group | All | None | Stub: displays what would be done (requires sudo). |
| `service_enable` | System | ✗ | service | Linux (systemd) | None | Stub: displays what would be done (requires sudo). |
| `manual` | System | ✗ | instructions | All | None | Displays manual installation instructions. |

---

## macOS App Bundles

| Action | Type | Decomposable | Key Parameters | Platform | Dependencies | Notes |
|--------|------|---|---|---|---|---|
| `app_bundle` | Primitive | ✗ | url, checksum, app_name, binaries, symlink_applications | macOS only | None | Downloads ZIP/DMG; installs .app bundle; symlinks CLI tools. |

---

## Build Environment

| Action | Type | Decomposable | Key Parameters | Platform | Dependencies | Notes |
|--------|------|---|---|---|---|---|
| `setup_build_env` | Primitive | ✗ | (none - uses ctx.Dependencies) | All | None | Configures PATH, PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS from dependencies. Wrapper for buildAutotoolsEnv(). |

---

## Summary Statistics

- **Total Actions Registered**: 52
- **Primitives**: 21 (20 core + 1 ecosystem that decompose, 10 ecosystem final)
- **Composites**: 13 (decompose to primitives)
- **System Actions**: 14 (delegate to OS, external, or provide structured data)
- **Deterministic**: 22+ (download_file, extract, chmod, install_binaries, set_env, set_rpath, install_libraries, link_dependencies, apply_patch_file, text_replace, homebrew_relocate, cargo_build*, go_build*, cargo_install, go_install, gem_install, cpan_install*, pip_exec*, npm_exec*, nix_realize*, all composites, all shell integration, all system config stubs)
- **Non-Deterministic**: ~10 (cmake_build, configure_make, meson_build, pip_install)
- **Network Required**: ~20+ (all package managers, build ecosystem, downloads, system packages)

---

## Key Patterns

### Ecosystem Integration
- **npm**: npm_install (composite) → npm_exec (primitive with package-lock)
- **cargo**: cargo_install (composite) → cargo_build (primitive with Cargo.lock)
- **go**: go_install (composite) → go_build (primitive with go.sum)
- **gem**: gem_install (composite) → gem_exec (primitive with Gemfile.lock)
- **python**: pip_install/pipx_install (composites) → pip_exec (primitive with locked_requirements)
- **perl**: cpan_install (primitive with snapshot)
- **nix**: nix_install (composite) → nix_realize (primitive with flake.lock)

### Determinism Strategy
- **Primitives**: Checksums, lockfiles, environment isolation, deterministic flags (SOURCE_DATE_EPOCH, GOPROXY=off, etc.)
- **Ecosystem Primitives**: Locked dependency files capture transitive state; offline flags prevent network variance
- **Composites**: Decompose to primitives; eval-time computation (downloading, generating locks) produces inputs for plan-time execution

### Platform Constraints
- **Linux-only**: set_rpath, nix_install, nix_realize, apt/dnf/pacman/apk/zypper
- **macOS-only**: app_bundle, brew_install, brew_cask
- **Darwin (macOS) Constraint**: homebrew (implicit)
- **Debian Constraint**: apt_install, apt_repo, apt_ppa
- **All Platforms**: Core primitives, composites, run_command, require_system, shell integration

### Internal-Only Actions
All actions listed here are public (registered in the registry and available for recipe authors).

---

## Dependencies Pattern

### EvalTime Dependencies
Used during `tsuku eval` (plan generation, lockfile computation):
- npm_install, cargo_install, go_install, gem_install, pipx_install, nix_install: Need toolchain for decomposition

### InstallTime Dependencies
Used during `tsuku install` (execution):
- All ecosystem primitives + build actions: Need toolchain to build/install

### Runtime Dependencies
Needed when installed tool runs:
- Package managers (npm, cargo, go, gem, pip, cpan): Runtime environments

---

