# Build Essentials

Build essentials are tools with specialized tsuku actions or configuration that enable source compilation. Unlike generic libraries (which are simply installed as dependencies), build essentials have dedicated integration into tsuku's build system.

## What Makes a Tool "Essential"?

Build essentials fall into two categories:

1. **Tools with dedicated actions** - cmake_build, configure_make, meson_build
2. **Tools with specialized handling** - Compiler fallback (zig), library discovery (pkg-config), binary relocation (patchelf)

Generic libraries like zlib, openssl, readline, ncurses, etc. are **not** build essentials. They auto-provision as dependencies without special handling.

## Build System Actions

tsuku provides three specialized actions for building tools from source:

### configure_make

Autotools-based builds using the classic `./configure && make && make install` pattern.

**Action:** `configure_make`

**Automatic environment configuration:**
- CC/CXX: Set to `zig cc`/`zig c++` when system compiler unavailable
- PKG_CONFIG_PATH: Includes all dependency `lib/pkgconfig` directories
- CPPFLAGS: Includes `-I` flags for dependency headers
- LDFLAGS: Includes `-L` flags for dependency libraries

**Example recipe:**
```toml
[[steps]]
action = "configure_make"
url = "https://ftp.gnu.org/gnu/gdbm/gdbm-1.23.tar.gz"
configure_flags = ["--disable-shared"]
```

**Implicit dependencies:** make, zig (if no system compiler)

### cmake_build

CMake-based builds for projects using CMakeLists.txt.

**Action:** `cmake_build`

**Automatic environment configuration:**
- CMAKE_PREFIX_PATH: Semicolon-separated list of dependency installation paths
- CC/CXX: Set to `zig cc`/`zig c++` when system compiler unavailable

**Example recipe:**
```toml
[[steps]]
action = "cmake_build"
source_dir = "ninja-{version}"
executables = ["ninja"]
cmake_args = ["-DBUILD_TESTING=OFF"]
```

**Implicit dependencies:** cmake, make, zig (if no system compiler)

### setup_build_env

Validates and displays build environment configuration. This action doesn't modify files - it ensures the environment can be configured and shows what will be available to build actions.

**Action:** `setup_build_env`

**What it does:**
- Validates dependency paths are accessible
- Displays PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS, CC, CXX configuration
- No-op execution (actual environment setup happens in build actions)

### fossil_archive

Downloads and extracts source archives from Fossil SCM repositories.

**Action:** `fossil_archive`

**Parameters:**
- `repo` (required): Fossil repository URL (must be HTTPS)
- `project_name` (required): Name used in tarball filename
- `tag_prefix` (optional): Prefix before version in tags (default: "version-")
- `version_separator` (optional): Separator in version numbers (default: ".")
- `strip_dirs` (optional): Directories to strip from archive (default: 1)

**Example recipe:**
```toml
[[steps]]
action = "fossil_archive"
repo = "https://sqlite.org/src"
project_name = "sqlite"
strip_dirs = 1
```

**URL construction:**
The action builds tarball URLs using the pattern: `{repo}/tarball/{tag}/{project_name}.tar.gz`

For example, with version `3.46.0` and `tag_prefix = "version-"`:
`https://sqlite.org/src/tarball/version-3.46.0/sqlite.tar.gz`

**See also:** [DESIGN-fossil-archive.md](DESIGN-fossil-archive.md) for detailed implementation

## Core Build Tools

### zig

**Purpose:** C/C++ compiler fallback

**Special handling:**
- Used as `zig cc` and `zig c++` when no system compiler is found
- Automatically invoked by configure_make and cmake_build actions
- Creates wrapper scripts at `$TSUKU_HOME/tools/zig-cc-wrapper/` for build system compatibility

**When it's used:**
- No gcc or cc found in system PATH
- Build actions (configure_make, cmake_build) automatically use zig as compiler

**Cross-platform support:** Linux x86_64, macOS Intel, macOS ARM

**Recipe:** `internal/recipe/recipes/z/zig.toml`

### cmake

**Purpose:** Build system generator with dedicated cmake_build action

**Special handling:**
- Has dedicated `cmake_build` action
- Automatically sets CMAKE_PREFIX_PATH for dependency discovery
- Configures CC/CXX compiler environment

**Recipe:** `internal/recipe/recipes/c/cmake.toml`

### make

**Purpose:** Build automation tool

**Special handling:**
- Required by configure_make and cmake_build actions
- Automatically installed as implicit dependency when needed

**Recipe:** `internal/recipe/recipes/m/make.toml`

### pkg-config

**Purpose:** Library discovery for compilation

**Special handling:**
- PKG_CONFIG_PATH automatically set by build actions
- Includes all dependency `lib/pkgconfig` directories
- Enables autotools/cmake to find tsuku-installed libraries

**How it works:**
Build actions automatically configure PKG_CONFIG_PATH to include all dependency library metadata, allowing tools like autoconf and cmake to discover library locations and required compiler flags.

**Recipe:** `internal/recipe/recipes/p/pkg-config.toml`

### ninja

**Purpose:** Fast build tool (optional make alternative)

**Special handling:**
- Alternative to make for CMake projects
- Built using cmake_build action (example of action recursion)

**Recipe:** `internal/recipe/recipes/n/ninja.toml`

### patchelf

**Purpose:** ELF binary RPATH modification on Linux

**Special handling:**
- Used by `set_rpath` action to modify dynamic linker paths
- Enables relocatable binaries by setting `$ORIGIN`-relative library paths
- Only required on Linux (macOS uses install_name_tool from system)

**When it's used:**
- set_rpath action on Linux systems
- Automatic dependency relocation for Homebrew bottles
- Custom RPATH configuration in recipes

**Platform:** Linux only

**Recipe:** `internal/recipe/recipes/p/patchelf.toml`

## Platform Support

Build essentials are validated on:
- Linux x86_64 (ubuntu-latest)
- macOS Intel (macos-13)
- macOS Apple Silicon (macos-14)

Note: arm64 Linux is not currently supported for Homebrew bottles due to upstream availability limitations.

## Usage Examples

### Building from Source with Autotools

When you install a tool that uses configure_make, tsuku:
1. Auto-installs make and zig (if no system compiler)
2. Configures PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS
3. Runs `./configure` with specified flags
4. Runs `make` and `make install`

Example installation:
```bash
tsuku install gdbm
```

Behind the scenes (from recipe):
```toml
[[steps]]
action = "configure_make"
url = "https://ftp.gnu.org/gnu/gdbm/gdbm-1.23.tar.gz"
configure_flags = ["--disable-shared"]

# tsuku automatically:
# - Installs make, zig as implicit dependencies
# - Sets CC=zig cc (if no system compiler)
# - Configures PKG_CONFIG_PATH for dependency discovery
```

### Building with CMake

When you install ninja (which builds itself using CMake):
```bash
tsuku install ninja
```

From recipe:
```toml
[[steps]]
action = "cmake_build"
source_dir = "ninja-{version}"
executables = ["ninja"]
cmake_args = ["-DBUILD_TESTING=OFF"]

# tsuku automatically:
# - Installs cmake, make, zig as implicit dependencies
# - Sets CMAKE_PREFIX_PATH for dependency discovery
# - Runs cmake and make to build ninja
```

### Using zig as Compiler Fallback

If you have no system compiler, tsuku automatically uses zig:
```bash
# Install zig explicitly (optional - auto-installed when needed)
tsuku install zig

# Install any tool that requires compilation
tsuku install some-tool-from-source

# tsuku automatically:
# - Detects no system compiler (gcc/cc)
# - Creates zig wrapper scripts at ~/.tsuku/tools/zig-cc-wrapper/
# - Sets CC=~/.tsuku/tools/zig-cc-wrapper/cc
# - Sets CXX=~/.tsuku/tools/zig-cc-wrapper/c++
```

## Installation

Build essentials are installed automatically when needed. You can also install them explicitly:

```bash
# Install compiler fallback
tsuku install zig

# Install build tools
tsuku install cmake
tsuku install make
tsuku install ninja

# Install library discovery
tsuku install pkg-config

# Install RPATH modification (Linux only)
tsuku install patchelf
```

All build essentials are installed to `$TSUKU_HOME/tools/` and managed using the same dependency tracking as regular tools.

## Validation

Build essentials undergo additional validation beyond standard tests:

- **Functional tests**: `test/scripts/verify-tool.sh`
- **Relocation tests**: `test/scripts/verify-relocation.sh`
- **Dependency isolation**: `test/scripts/verify-no-system-deps.sh`

See the [Build Essentials CI workflow](../.github/workflows/build-essentials.yml) for the complete validation matrix.

## See Also

- [Actions and Primitives Guide](GUIDE-actions-and-primitives.md#build-environment-configuration) - Detailed action documentation
- [Design: Dependency Provisioning](DESIGN-dependency-provisioning.md) - How tsuku manages build dependencies
- [Contributing Guide](../CONTRIBUTING.md#testing-build-essentials) - How to test build essential recipes
