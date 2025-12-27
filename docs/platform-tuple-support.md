# Platform Tuple Support in install_guide

Platform tuple support allows recipes to provide platform-specific installation guides at the OS/architecture level, not just the OS level. This enables more precise guidance for system dependencies.

## Overview

When using `require_system` actions, you can now specify installation guides using:
- **Platform tuples** (e.g., `darwin/arm64`, `linux/amd64`)
- **OS-only keys** (e.g., `darwin`, `linux`)
- **Fallback key** (`fallback`)

The system uses hierarchical fallback to find the most specific guide available.

## Hierarchical Fallback

When looking up installation guidance for a platform, tsuku checks in this order:

1. **Exact platform tuple** - e.g., `darwin/arm64`
2. **OS-only key** - e.g., `darwin`
3. **Generic fallback** - `fallback`

This allows you to provide specific guidance where needed while falling back to broader instructions.

## Examples

### OS-only keys (backwards compatible)

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.params.install_guide]
darwin = "brew install docker"
linux = "sudo apt install docker.io"
```

**Result:** All darwin platforms (darwin/amd64, darwin/arm64) use the darwin guide.

### Platform tuples for architecture-specific guidance

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.params.install_guide]
"darwin/arm64" = "brew install docker (optimized for Apple Silicon)"
"darwin/amd64" = "brew install docker (Intel Mac)"
"linux/amd64" = "sudo apt install docker.io"
"linux/arm64" = "sudo apt install docker.io"
```

**Result:** Each platform gets architecture-specific guidance.

### Mixed keys (tuple + OS)

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.params.install_guide]
"darwin/arm64" = "brew install docker (Apple Silicon optimizations available)"
darwin = "brew install docker"
linux = "sudo apt install docker.io"
```

**Result:**
- `darwin/arm64` uses the tuple-specific guide
- `darwin/amd64` falls back to the OS-only guide
- All linux platforms use the OS-only guide

### Generic fallback

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.params.install_guide]
fallback = "See https://docker.com/install for installation instructions"
```

**Result:** All platforms use the fallback guide.

### Partial coverage with fallback

```toml
[[steps]]
action = "require_system"
command = "cuda"

[steps.params.install_guide]
"linux/amd64" = "sudo apt install nvidia-cuda-toolkit"
fallback = "CUDA is not supported on this platform"
```

**Result:**
- `linux/amd64` gets specific installation instructions
- All other platforms (darwin/arm64, darwin/amd64, linux/arm64) use the fallback

## Validation

Recipes are validated at load time to ensure install_guide coverage:

1. **Tuple keys must exist in supported platforms**
   - If you specify `darwin/arm64`, the recipe must support that platform

2. **OS-only keys must exist in supported OS set**
   - If you specify `darwin`, the recipe must support at least one darwin platform

3. **All platforms must have coverage**
   - Each supported platform must be covered by an exact tuple, OS key, or fallback

### Validation Examples

**Valid - complete OS coverage:**
```toml
supported_os = ["linux", "darwin"]
# supported_arch defaults to ["amd64", "arm64"]

[steps.params.install_guide]
darwin = "brew install tool"
linux = "apt install tool"
```

**Valid - tuple + fallback:**
```toml
supported_os = ["linux", "darwin"]

[steps.params.install_guide]
"darwin/arm64" = "brew install tool (optimized)"
fallback = "brew install tool"
```

**Invalid - missing coverage:**
```toml
supported_os = ["linux", "darwin"]

[steps.params.install_guide]
"darwin/arm64" = "brew install tool"
"linux/amd64" = "apt install tool"
# Error: darwin/amd64 and linux/arm64 not covered
```

**Invalid - unsupported tuple:**
```toml
supported_os = ["linux"]

[steps.params.install_guide]
"linux/amd64" = "apt install tool"
"darwin/arm64" = "brew install tool"  # Error: darwin not supported
```

## TOML Syntax Notes

Platform tuples contain `/` which requires quoting in TOML:

```toml
# Correct
"darwin/arm64" = "guide text"

# Incorrect (will be parsed as nested table)
darwin/arm64 = "guide text"
```

The BurntSushi/toml library correctly handles quoted keys with slashes.

## Migration Guide

Existing recipes using OS-only keys continue to work without changes. Platform tuple support is fully backwards compatible.

To add architecture-specific guidance:

1. **Identify platforms needing different guidance**
   - Check if ARM vs Intel requires different instructions

2. **Add tuple keys for specific platforms**
   - Keep OS-only keys for common guidance

3. **Test validation**
   - Run `tsuku validate <recipe>` to ensure all platforms are covered

## Related

- Platform constraints: `supported_os`, `supported_arch`, `unsupported_platforms`
- Step-level when clause support: Tracked in issue #690
