# Library Dependency Auto-Provisioning Guide

This guide explains how tsuku automatically provisions library dependencies, eliminating the need to install system packages like zlib, readline, or openssl before using tsuku.

## What is Library Dependency Auto-Provisioning?

When you install a tool with tsuku, any library dependencies it needs are automatically downloaded and installed to `$TSUKU_HOME`. You don't need to install them using apt-get, brew, or any system package manager.

**Key benefits:**
- No sudo required
- No system package manager needed
- Consistent across Linux and macOS
- Everything isolated to `$TSUKU_HOME`
- Multiple versions can coexist

## How It Works

When you install a tool, tsuku checks the recipe's `dependencies` field and automatically installs any missing dependencies before building or installing the tool.

**Example:** Installing sqlite

```bash
tsuku install sqlite
```

Behind the scenes:
1. tsuku reads sqlite recipe: `dependencies = ["readline"]`
2. tsuku checks if readline is installed
3. If missing, tsuku installs readline (which depends on ncurses)
4. ncurses is installed first, then readline, then sqlite
5. Everything goes to `$TSUKU_HOME/tools/`

**Result:** You get sqlite with readline support without installing any system packages.

## Migration Guide

If you're used to apt-get, brew, or older tsuku versions, here's how your workflow changes.

### Before Auto-Provisioning (Old Workflow)

```bash
# Install system dependencies
sudo apt-get install libreadline-dev libncurses-dev libssl-dev zlib1g-dev

# Then install your tool
tsuku install my-tool
```

### After Auto-Provisioning (New Workflow)

```bash
# Just install the tool
tsuku install my-tool
```

That's it. Tsuku handles all dependencies automatically.

### Real-World Examples

**Installing curl (needs openssl and zlib):**

```bash
# Old way
sudo apt-get install libssl-dev zlib1g-dev
tsuku install curl

# New way
tsuku install curl  # openssl and zlib auto-installed
```

**Installing sqlite (needs readline and ncurses):**

```bash
# Old way
sudo apt-get install libreadline-dev libncurses-dev
tsuku install sqlite

# New way
tsuku install sqlite  # readline and ncurses auto-installed
```

**Installing git (needs curl, openssl, zlib):**

```bash
# Old way
sudo apt-get install libcurl4-openssl-dev libssl-dev zlib1g-dev
tsuku install git

# New way
tsuku install git  # All dependencies auto-installed
```

## Build System Integration

For tools built from source, tsuku automatically configures the build environment to use tsuku-provided dependencies.

### Automatic Configuration

When a recipe uses build actions like `configure_make` or `cmake_build`, tsuku sets:

- **PKG_CONFIG_PATH**: Points to dependency pkg-config files
- **CPPFLAGS**: Includes `-I` flags for dependency headers
- **LDFLAGS**: Includes `-L` flags for dependency libraries
- **CMAKE_PREFIX_PATH**: Points to dependency installation paths
- **CC/CXX**: Compiler paths (uses zig if no system compiler)

### Example: curl Recipe

```toml
[metadata]
name = "curl"
dependencies = ["openssl", "zlib"]

[[steps]]
action = "setup_build_env"

[[steps]]
action = "configure_make"
source_dir = "curl-{version}"
configure_args = ["--with-openssl", "--with-zlib"]
```

When you run `tsuku install curl`:
1. openssl and zlib are installed first
2. `setup_build_env` configures paths to find them
3. `configure_make` uses those paths to link against tsuku's openssl and zlib
4. No system libraries needed

### Example: sqlite Recipe

```toml
[metadata]
name = "sqlite"
dependencies = ["readline"]

[[steps]]
action = "setup_build_env"

[[steps]]
action = "configure_make"
source_dir = "sqlite-autoconf-3510100"
configure_args = ["--enable-readline"]
```

When you run `tsuku install sqlite`:
1. readline is installed (which auto-installs ncurses)
2. Build environment is configured with readline paths
3. sqlite compiles with readline support from `$TSUKU_HOME`

## Checking Dependencies

You can see what dependencies a tool needs before installing it:

```bash
# View tool information including dependencies
tsuku info sqlite
```

Output shows:
```
Dependencies: readline
```

You can also list all installed tools and their dependencies:

```bash
tsuku list
```

## Troubleshooting

### What if a dependency is missing from the recipe?

If a tool builds successfully but fails at runtime with missing library errors, the recipe may be missing a dependency declaration. Report this as an issue.

### How do I verify dependencies were installed?

Check `$TSUKU_HOME/tools/` for dependency directories:

```bash
ls $TSUKU_HOME/tools/
```

You should see directories like:
- `readline-8.3.3/`
- `ncurses-6.5/`
- `openssl-3.6.0/`
- `zlib-1.3.1/`

### What if installation fails with "library not found"?

This typically means:
1. The dependency recipe is missing or incorrect
2. The build environment configuration failed
3. The library's pkg-config file is missing

Check the installation logs for details. If the issue persists, report it as a bug.

### Can I see the dependency graph?

Currently, tsuku doesn't have a visual dependency graph command, but you can trace dependencies by checking `tsuku info` for each tool:

```bash
tsuku info sqlite    # Shows: readline
tsuku info readline  # Shows: ncurses
tsuku info ncurses   # Shows: (none)
```

### Can I use system libraries instead?

No. Tsuku installs dependencies to `$TSUKU_HOME` to ensure consistency and avoid conflicts with system packages. This also means you don't need sudo privileges.

### How much disk space do dependencies use?

Typical library dependencies are small:
- zlib: ~1 MB
- ncurses: ~3 MB
- readline: ~2 MB
- openssl: ~10 MB

Compilers and build tools (zig, make, cmake) are larger but shared across all tools you build from source.

### Can I manually install a dependency?

Yes, you can explicitly install any dependency:

```bash
tsuku install readline
tsuku install openssl
tsuku install zlib
```

This is useful for testing or ensuring a specific version is available.

### What about build tools (make, gcc, cmake)?

Build tools are also auto-provisioned when needed. If a recipe uses `configure_make`, tsuku automatically installs:
- make
- zig (C/C++ compiler if no system compiler exists)
- pkg-config

See [BUILD-ESSENTIALS.md](BUILD-ESSENTIALS.md) for details on build tool auto-provisioning.

## How Dependency Resolution Works

Tsuku uses a dependency graph to determine installation order:

1. **Load recipe**: Read the tool's recipe file
2. **Collect dependencies**: Find all `dependencies` entries
3. **Recursive resolution**: Load recipes for each dependency
4. **Topological sort**: Order dependencies so each is installed before its dependents
5. **Install in order**: Install dependencies first, then the requested tool

**Example:** Installing sqlite

```
sqlite → readline → ncurses
```

Installation order:
1. ncurses (no dependencies)
2. readline (depends on ncurses)
3. sqlite (depends on readline)

## Reference

For more details, see:

- [Actions and Primitives Guide](GUIDE-actions-and-primitives.md) - Build environment configuration and actions
- [Build Essentials](BUILD-ESSENTIALS.md) - Compiler and build tool auto-provisioning
- [Dependency Provisioning Design](DESIGN-dependency-provisioning.md) - Technical architecture and implementation details

## Summary

Library dependency auto-provisioning means:
- No system package manager needed
- No sudo required
- Dependencies installed automatically to `$TSUKU_HOME`
- Build environment configured automatically
- Consistent behavior across platforms

Just run `tsuku install <tool>` and everything works.
