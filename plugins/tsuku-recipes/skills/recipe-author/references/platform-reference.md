# Platform Reference

When clause syntax, libc decision tree, and migration templates for
platform-conditional recipes.

---

## When Clause Syntax

Every `[[steps]]` entry can include a `when` table that restricts the step
to matching platforms. Omit `when` to run on all platforms.

```toml
[[steps]]
action = "homebrew"
formula = "pcre2"
when = { os = ["linux"], libc = ["glibc"] }
```

### Available Fields

| Field | Type | Values | Scope |
|-------|------|--------|-------|
| `os` | []string | `"linux"`, `"darwin"` | Operating system |
| `arch` | string | `"amd64"`, `"arm64"` | CPU architecture |
| `libc` | []string | `"glibc"`, `"musl"` | C library (Linux only) |
| `linux_family` | string | `"debian"`, `"rhel"`, `"alpine"`, `"arch"`, `"suse"` | Distro family |
| `gpu` | []string | `"nvidia"`, `"amd"`, `"intel"`, `"none"` | GPU vendor |
| `platform` | []string | `"linux/amd64"`, `"darwin/arm64"` | Exact os/arch tuple |
| `package_manager` | string | `"brew"`, `"apt"`, `"dnf"`, `"pacman"`, `"apk"`, `"zypper"` | Runtime detection |

### Matching Logic

- Multiple fields within one `when` clause AND together
- Array values within a field OR together
- Empty `when` (or omitted) matches everything
- `platform` and `os` are mutually exclusive at the top level

Examples:

```toml
# Matches: Linux with glibc (any arch)
when = { os = ["linux"], libc = ["glibc"] }

# Matches: Linux on amd64 with an NVIDIA GPU
when = { os = ["linux"], arch = "amd64", gpu = ["nvidia"] }

# Matches: exactly darwin/arm64
when = { platform = ["darwin/arm64"] }

# Matches: Debian-family Linux
when = { linux_family = "debian" }
```

---

## Metadata Platform Fields

Restrict which platforms a recipe supports at the recipe level (not per-step):

```toml
[metadata]
supported_os = ["darwin", "linux"]
supported_arch = ["amd64", "arm64"]
supported_libc = ["glibc"]                    # omit to allow both
unsupported_platforms = ["linux/arm64"]        # exceptions
unsupported_reason = "No arm64 Linux binaries available upstream"
```

These fields are advisory. tsuku uses them to skip recipes during search/install
on unsupported platforms and to power `--check-libc-coverage` validation.

---

## Libc Decision Tree

Use this when deciding whether your recipe needs glibc/musl splits.

```
Is this a library (type = "library")?
  YES --> You almost certainly need libc splits.
          Libraries link against libc; a glibc .so won't work on musl.
          --> Go to "Three-Way Split" template below.
  NO  --> Does the tool use a pre-built binary?
            YES --> Does the upstream project provide musl builds?
                      YES --> Use different asset_pattern per libc.
                      NO  --> Set supported_libc = ["glibc"] OR
                              add a source-build fallback for musl.
            NO  --> Does the tool use an ecosystem installer?
                      YES (cargo/go/npm/pip) --> Usually fine.
                            Go and Rust static-link by default.
                            Python/Node use python-standalone (musl-aware).
                      NO  --> Source build: use homebrew for glibc,
                              configure_make/cmake for musl.
```

---

## Migration Templates

### Template A: Homebrew on glibc + source build on musl

The most common pattern for C/C++ tools and libraries. Homebrew bottles are
glibc-only; musl systems need a source build.

```toml
# glibc Linux: Homebrew bottle
[[steps]]
action = "homebrew"
formula = "my-tool"
when = { os = ["linux"], libc = ["glibc"] }

[[steps]]
action = "install_binaries"
binaries = ["bin/my-tool"]
when = { os = ["linux"], libc = ["glibc"] }

# musl Linux: build from source
[[steps]]
action = "download"
url = "https://example.com/my-tool-{version}.tar.gz"
checksum_url = "https://example.com/my-tool-{version}.tar.gz.sha256"
when = { os = ["linux"], libc = ["musl"] }

[[steps]]
action = "extract"
archive = "my-tool-{version}.tar.gz"
format = "tar.gz"
when = { os = ["linux"], libc = ["musl"] }

[[steps]]
action = "setup_build_env"
when = { os = ["linux"], libc = ["musl"] }

[[steps]]
action = "configure_make"
source_dir = "my-tool-{version}"
executables = ["my-tool"]
when = { os = ["linux"], libc = ["musl"] }

[[steps]]
action = "install_binaries"
binaries = ["bin/my-tool"]
when = { os = ["linux"], libc = ["musl"] }

# macOS: Homebrew
[[steps]]
action = "homebrew"
formula = "my-tool"
when = { os = ["darwin"] }

[[steps]]
action = "install_binaries"
binaries = ["bin/my-tool"]
when = { os = ["darwin"] }
```

### Template B: System packages on musl (Alpine)

When the tool is available in Alpine's package repository, use apk_install
as the musl path instead of building from source.

```toml
# glibc Linux: Homebrew
[[steps]]
action = "homebrew"
formula = "my-tool"
when = { os = ["linux"], libc = ["glibc"] }

# musl Linux: Alpine package
[[steps]]
action = "apk_install"
packages = ["my-tool"]
when = { os = ["linux"], libc = ["musl"] }

# macOS: Homebrew
[[steps]]
action = "homebrew"
formula = "my-tool"
when = { os = ["darwin"] }
```

### Template C: Binary download with libc-specific assets

When the upstream project publishes separate glibc and musl binaries.

```toml
# glibc Linux
[[steps]]
action = "github_archive"
repo = "owner/repo"
asset_pattern = "tool-{version}-linux-glibc-{arch}.tar.gz"
binaries = ["tool"]
when = { os = ["linux"], libc = ["glibc"] }

# musl Linux
[[steps]]
action = "github_archive"
repo = "owner/repo"
asset_pattern = "tool-{version}-linux-musl-{arch}.tar.gz"
binaries = ["tool"]
when = { os = ["linux"], libc = ["musl"] }

# macOS (no libc split needed)
[[steps]]
action = "github_archive"
repo = "owner/repo"
asset_pattern = "tool-{version}-darwin-{arch}.tar.gz"
binaries = ["tool"]
when = { os = ["darwin"] }
```

---

## GPU Conditionals

For tools with hardware-specific builds (CUDA, Vulkan, etc.):

```toml
[[steps]]
action = "github_file"
repo = "owner/repo"
file_path = "tool-{version}-linux-cuda-{arch}"
binaries = ["tool"]
when = { os = ["linux"], arch = "amd64", gpu = ["nvidia"] }

[[steps]]
action = "github_file"
repo = "owner/repo"
file_path = "tool-{version}-linux-vulkan-{arch}"
binaries = ["tool"]
when = { os = ["linux"], arch = "amd64", gpu = ["amd", "intel"] }

[[steps]]
action = "github_file"
repo = "owner/repo"
file_path = "tool-{version}-linux-cpu-{arch}"
binaries = ["tool"]
when = { os = ["linux"], arch = "amd64", gpu = ["none"] }
```

See `recipes/t/tsuku-llm.toml` for a real-world GPU-conditional recipe.
