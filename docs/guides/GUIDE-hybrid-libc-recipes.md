# Hybrid Libc Recipes Guide

This guide explains how to write recipes that work on both glibc-based Linux distributions (Debian, Fedora, Arch, SUSE) and musl-based systems (Alpine Linux).

## Platform Support Matrix

Tsuku uses different library provisioning strategies depending on the target platform:

| Platform | Libc | Library Source | Version Control | User Action |
|----------|------|----------------|-----------------|-------------|
| Linux (Debian, Fedora, Arch, SUSE) | glibc | Homebrew bottles | Hermetic (tsuku controls) | None |
| Linux (Alpine) | musl | System packages | Non-hermetic (apk controls) | `apk add` before tsuku |
| macOS | libSystem | Homebrew | Homebrew controls | None |

### Why the Difference?

**glibc systems** use Homebrew bottles because:
- Hermetic version control (same library versions everywhere)
- CI reproducibility and audit trails
- No system package manager conflicts

**musl systems** use system packages because:
- Homebrew bottles are built for glibc and won't work on musl
- Alpine doesn't retain old package versions, so hermetic versions wouldn't provide reproducibility anyway
- The `apk` package manager handles all transitive dependencies automatically

## The `libc` Filter

Use the `libc` filter in `when` clauses to create platform-specific steps:

```toml
# Only runs on glibc Linux (Debian, Fedora, Arch, SUSE)
[[steps]]
action = "homebrew"
formula = "zlib"
when = { os = ["linux"], libc = ["glibc"] }

# Only runs on musl Linux (Alpine)
[[steps]]
action = "apk_install"
packages = ["zlib-dev"]
when = { os = ["linux"], libc = ["musl"] }

# Only runs on macOS
[[steps]]
action = "homebrew"
formula = "zlib"
when = { os = ["darwin"] }
```

### Valid `libc` Values

- `glibc` - GNU C Library (Debian, Ubuntu, Fedora, RHEL, Arch, openSUSE)
- `musl` - musl libc (Alpine Linux)

The `libc` filter only applies on Linux. On macOS, libc is always libSystem.

## Step-Level Dependencies

Dependencies can be declared at the step level, so they're only resolved when that step runs:

```toml
# Dependencies only resolved on glibc
[[steps]]
action = "homebrew"
formula = "openssl@3"
when = { os = ["linux"], libc = ["glibc"] }
dependencies = ["zlib"]  # Only needed for Homebrew path

# No dependencies needed - apk handles transitive deps
[[steps]]
action = "apk_install"
packages = ["openssl-dev"]
when = { os = ["linux"], libc = ["musl"] }
```

Step-level dependencies prevent "phantom dependencies" where the plan shows resolved dependencies that aren't used.

## Migration Templates

### Template A: Library with No Dependencies

Use this pattern for libraries that have no upstream dependencies (e.g., zlib, brotli).

```toml
[metadata]
name = "zlib"
description = "General-purpose lossless data-compression library"
type = "library"

# glibc Linux: Homebrew bottles
[[steps]]
action = "homebrew"
formula = "zlib"
when = { os = ["linux"], libc = ["glibc"] }

[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["lib/libz.a", "lib/libz.so", "lib/libz.so.1", "include/zlib.h", "include/zconf.h"]
when = { os = ["linux"], libc = ["glibc"] }

# musl Linux: System packages (apk handles transitive deps)
[[steps]]
action = "apk_install"
packages = ["zlib-dev"]
when = { os = ["linux"], libc = ["musl"] }

# macOS: Homebrew
[[steps]]
action = "homebrew"
formula = "zlib"
when = { os = ["darwin"] }

[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["lib/libz.a", "lib/libz.dylib", "lib/libz.1.dylib", "include/zlib.h", "include/zconf.h"]
when = { os = ["darwin"] }
```

### Template B: Library with Dependencies

Use this pattern for libraries that depend on other libraries (e.g., openssl depends on zlib).

```toml
[metadata]
name = "openssl"
description = "Cryptography and SSL/TLS Toolkit"
type = "library"
binaries = ["bin/openssl"]

# glibc Linux: Homebrew bottles with step-level dependencies
[[steps]]
action = "homebrew"
formula = "openssl@3"
when = { os = ["linux"], libc = ["glibc"] }
dependencies = ["zlib"]  # Only resolved on glibc

[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = [
    "bin/openssl",
    "lib/libssl.so",
    "lib/libssl.so.3",
    "lib/libcrypto.so",
    "lib/libcrypto.so.3",
]
when = { os = ["linux"], libc = ["glibc"] }

# musl Linux: System packages (apk handles transitive deps)
[[steps]]
action = "apk_install"
packages = ["openssl-dev"]
when = { os = ["linux"], libc = ["musl"] }

# macOS: Homebrew
[[steps]]
action = "homebrew"
formula = "openssl@3"
when = { os = ["darwin"] }

[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = [
    "bin/openssl",
    "lib/libssl.dylib",
    "lib/libssl.3.dylib",
    "lib/libcrypto.dylib",
    "lib/libcrypto.3.dylib",
]
when = { os = ["darwin"] }

[verify]
command = "openssl version"
pattern = "OpenSSL {version}"
```

### Template C: Tool That Depends on Libraries

Use this pattern for tools that need library dependencies (e.g., cmake needs openssl).

```toml
[metadata]
name = "cmake"
description = "Cross-platform build system"
# Note: No recipe-level dependencies for hybrid recipes

# glibc Linux: Build with hermetic deps
[[steps]]
action = "download_file"
url = "https://github.com/Kitware/CMake/releases/download/v{version}/cmake-{version}-linux-x86_64.tar.gz"
when = { os = ["linux"], libc = ["glibc"], arch = "amd64" }
dependencies = ["openssl", "zlib"]  # Only resolved on glibc

# musl Linux: Require system libraries first
[[steps]]
action = "apk_install"
packages = ["openssl-dev"]
when = { os = ["linux"], libc = ["musl"] }

[[steps]]
action = "download_file"
url = "https://github.com/Kitware/CMake/releases/download/v{version}/cmake-{version}-linux-x86_64.tar.gz"
when = { os = ["linux"], libc = ["musl"], arch = "amd64" }
# No dependencies - apk already installed them

# macOS: Download binary
[[steps]]
action = "download_file"
url = "https://github.com/Kitware/CMake/releases/download/v{version}/cmake-{version}-macos-universal.tar.gz"
when = { os = ["darwin"] }

[verify]
command = "cmake --version"
```

## Local Testing

### Testing on Alpine (musl)

Test your recipe on Alpine before submitting:

```bash
# Quick test - verify library dlopen works
docker run --rm -v $(pwd):/work -w /work alpine:3.19 sh -c '
  apk add --no-cache curl
  ./tsuku-linux-amd64 verify --dlopen zlib
'

# Full test with library deps
docker run --rm -v $(pwd):/work -w /work alpine:3.19 sh -c '
  apk add --no-cache curl zlib-dev libyaml-dev openssl-dev
  ./tsuku-linux-amd64 verify --dlopen zlib
  ./tsuku-linux-amd64 verify --dlopen libyaml
  ./tsuku-linux-amd64 install jq
  jq --version
'

# Interactive testing session
docker run --rm -it -v $(pwd):/work -w /work alpine:3.19 sh
# Then run tsuku commands interactively
```

### Testing on glibc

```bash
# Test on Debian
docker run --rm -v $(pwd):/work -w /work debian:bookworm-slim sh -c '
  apt-get update && apt-get install -y curl ca-certificates
  ./tsuku-linux-amd64 install zlib
  ./tsuku-linux-amd64 verify --dlopen zlib
'

# Test on Fedora
docker run --rm -v $(pwd):/work -w /work fedora:41 sh -c '
  dnf install -y curl
  ./tsuku-linux-amd64 install zlib
  ./tsuku-linux-amd64 verify --dlopen zlib
'
```

### Verifying dlopen Works

The `verify --dlopen` command checks that a library can be dynamically loaded:

```bash
# Should succeed on the correct platform
tsuku verify --dlopen zlib

# Output on success:
# zlib: OK (dlopen verified)

# Output on failure:
# zlib: FAILED (cannot open shared object file)
```

## Troubleshooting

### Why do I need to install system packages on Alpine?

On Alpine (musl), tsuku uses system packages instead of Homebrew bottles because:

1. **Homebrew bottles are glibc-specific** - They link against glibc symbols that don't exist on musl
2. **Alpine removes old packages** - There's no package version archive like Debian's snapshot.debian.org
3. **apk handles dependencies** - The Alpine package manager resolves transitive deps automatically

### How do I find the right Alpine package name?

Alpine package names follow these conventions:

| Library | Development Package |
|---------|---------------------|
| zlib | `zlib-dev` |
| openssl | `openssl-dev` |
| libyaml | `yaml-dev` |
| curl | `curl-dev` |
| ncurses | `ncurses-dev` |

Search for packages:
```bash
# In Alpine container
apk search <library-name>

# Or online
# https://pkgs.alpinelinux.org/packages
```

### Can I use hermetic versions on Alpine?

Not with the hybrid approach. If you need exact version control on Alpine, use the `nix-portable` action which provides hermetic Nix packages on any Linux:

```toml
[[steps]]
action = "nix-portable"
packages = ["zlib"]
when = { os = ["linux"], libc = ["musl"] }
```

This is slower but provides the same hermetic guarantees as glibc.

### My library recipe passes validation but fails at runtime on Alpine

Check these common issues:

1. **Missing `apk_install` step** - Ensure you have a musl path with the right packages
2. **Wrong package name** - Alpine packages have `-dev` suffix for development headers
3. **Missing transitive deps** - On glibc you declare deps explicitly; on musl, apk handles them

### Tool works on Debian but not Alpine

If a tool recipe works on glibc but fails on musl:

1. **Check for library dependencies** - Add an `apk_install` step for required system packages
2. **Check binary compatibility** - Some upstream binaries are glibc-only
3. **Consider static builds** - Some tools provide musl-static binaries for Alpine

## Recipe Reference: New Fields

### `libc` Filter in `when` Clause

Filter steps by C library implementation:

```toml
[[steps]]
action = "homebrew"
when = { os = ["linux"], libc = ["glibc"] }  # glibc only

[[steps]]
action = "apk_install"
when = { os = ["linux"], libc = ["musl"] }   # musl only

[[steps]]
action = "homebrew"
when = { libc = ["glibc", "musl"] }          # Both (unusual)
```

The `libc` filter is only valid when `os` includes `"linux"` or is omitted.

### `dependencies` at Step Level

Declare dependencies that are only resolved when the step matches:

```toml
[[steps]]
action = "homebrew"
formula = "curl"
when = { os = ["linux"], libc = ["glibc"] }
dependencies = ["openssl", "zlib"]  # Only resolved if this step runs
```

Step-level dependencies are additive with recipe-level dependencies.

### `supported_libc` in Metadata

Explicitly constrain which libc implementations a recipe supports:

```toml
[metadata]
name = "glibc-only-tool"
supported_libc = ["glibc"]
unsupported_reason = "Upstream only provides glibc binaries (tracked: github.com/foo/bar/issues/123)"
```

This prevents CI errors for recipes that genuinely cannot support musl.

### `unsupported_reason` in Metadata

Explain why platform constraints exist (applies to all constraints, not just libc):

```toml
[metadata]
supported_os = ["linux"]
unsupported_reason = "Requires Linux-specific syscalls"

# Or for libc
supported_libc = ["glibc"]
unsupported_reason = "Depends on glibc-specific features not in musl"
```

## CI Validation

CI automatically validates recipes for libc coverage:

- **Library recipes** (`type = "library"`) must have musl support OR explicit `supported_libc` constraint
- **Tool recipes** with library dependencies get warnings (not errors) if missing musl support

Run validation locally:

```bash
# Build tsuku
go build -o tsuku ./cmd/tsuku

# Validate a single recipe
./tsuku validate --check-libc-coverage internal/recipe/recipes/zlib.toml

# Validate all embedded recipes
./tsuku validate-recipes --check-libc-coverage
```

## Summary

Writing hybrid libc recipes:

1. **Use the `libc` filter** to separate glibc and musl paths
2. **Use step-level dependencies** so glibc deps aren't resolved on musl
3. **Test on Alpine** before submitting
4. **Use explicit constraints** for recipes that can't support musl

The hybrid approach preserves hermetic library versions on glibc while enabling Alpine support via system packages.

## See Also

- [Library Dependencies Guide](GUIDE-library-dependencies.md) - How library auto-provisioning works
- [System Dependencies Guide](GUIDE-system-dependencies.md) - How system package actions work
- [When Clause Usage](when-clause-usage.md) - Complete when clause reference
- [Platform Compatibility Design](designs/DESIGN-platform-compatibility-verification.md) - Technical design document
