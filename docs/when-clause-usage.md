# When Clause Usage

Step-level `when` clauses allow conditional execution of recipe steps based on platform (OS/architecture) and runtime conditions.

## Syntax

When clauses use inline TOML syntax:

```toml
[[steps]]
action = "some_action"
when = { platform = ["darwin/arm64", "linux/amd64"] }
```

## Supported Conditions

### Platform Tuples

Execute on specific OS/architecture combinations:

```toml
[[steps]]
action = "apply_patch"
file = "fix-arm.patch"
when = { platform = ["darwin/arm64"] }
# Executes only on Apple Silicon Macs
```

```toml
[[steps]]
action = "run_command"
command = "./configure --enable-optimizations"
when = { platform = ["linux/amd64", "linux/arm64"] }
# Executes on Linux (both amd64 and arm64)
```

### OS Arrays

Execute on any architecture of specified operating systems:

```toml
[[steps]]
action = "homebrew"
formula = "gcc"
when = { os = ["linux"] }
# Executes on Linux (any architecture)
```

```toml
[[steps]]
action = "run_command"
command = "brew install deps"
when = { os = ["darwin"] }
# Executes on macOS (Intel or ARM)
```

### Package Manager

Execute only when a specific package manager is available:

```toml
[[steps]]
action = "run_command"
command = "apt-get update"
when = { package_manager = "apt" }
# Executes only if apt is available
```

## Matching Semantics

### OR Logic Within Arrays

Multiple values in `platform` or `os` arrays are ORed together:

```toml
when = { platform = ["darwin/arm64", "linux/amd64"] }
# Matches: darwin/arm64 OR linux/amd64
```

```toml
when = { os = ["darwin", "linux"] }
# Matches: darwin (any arch) OR linux (any arch)
```

### Additive Matching Between Steps

All steps whose when clauses match the current platform will execute:

```toml
[[steps]]
action = "step1"
when = { os = ["linux"] }

[[steps]]
action = "step2"
when = { platform = ["linux/amd64"] }
```

On `linux/amd64`, **both steps execute** (additive semantics).

This follows precedents from Cargo and Homebrew.

### Empty Clause Matches All

Steps without a when clause (or with an empty clause) execute on all platforms:

```toml
[[steps]]
action = "download_archive"
# Executes on all platforms
```

## Mutual Exclusivity

The `platform` and `os` fields cannot coexist in the same when clause:

```toml
# INVALID - will cause validation error
when = { platform = ["darwin/arm64"], os = ["linux"] }
```

Choose one:
- Use `platform` for precise control
- Use `os` for any-architecture matching

## Validation

When clauses are validated at recipe load time:

1. **Platform tuple format**: Must use `os/arch` format with a slash
   ```toml
   when = { platform = ["darwin"] }  # ERROR: missing arch
   when = { platform = ["darwin/arm64"] }  # OK
   ```

2. **Supported platforms**: All platform tuples must exist in recipe's `GetSupportedPlatforms()`
   ```toml
   [metadata]
   supported_os = ["linux"]

   [[steps]]
   when = { platform = ["darwin/arm64"] }  # ERROR: darwin not supported
   ```

3. **Supported OS**: All OS values must exist in recipe's supported OS set
   ```toml
   [metadata]
   supported_os = ["linux"]

   [[steps]]
   when = { os = ["darwin"] }  # ERROR: darwin not supported
   ```

## Examples

### Linux-Only Step

```toml
[[steps]]
action = "link_dependencies"
library = "gcc-libs"
when = { os = ["linux"] }
```

### Apple Silicon Optimization

```toml
[[steps]]
action = "apply_patch"
file = "m1-optimizations.patch"
when = { platform = ["darwin/arm64"] }
```

### Multiple Platform-Specific Builds

```toml
[[steps]]
action = "run_command"
command = "./configure --enable-optimizations --with-universal-archs=universal2"
when = { platform = ["darwin/arm64", "darwin/amd64"] }

[[steps]]
action = "run_command"
command = "./configure --enable-optimizations"
when = { platform = ["linux/amd64", "linux/arm64"] }
```

### Migration from Old Syntax

**Old (table syntax):**
```toml
[[steps]]
action = "homebrew"
[steps.when]
os = "linux"
```

**New (inline syntax):**
```toml
[[steps]]
action = "homebrew"
when = { os = ["linux"] }
```

Note: OS values are now arrays to support multiple values.

## Platform Tuple Format

Platform tuples use the `os/arch` format:

- **Supported OS**: `linux`, `darwin`
- **Supported arch**: `amd64`, `arm64`

Valid tuples:
- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`

## Consistency with install_guide

The `when` clause uses the same platform tuple format as `install_guide` in `require_system` actions. Both support:

- Platform tuples: `darwin/arm64`, `linux/amd64`
- OS-only keys: `darwin`, `linux`

Difference: `install_guide` uses hierarchical fallback (lookup table), while `when` clauses use exact matching (boolean filter).

## See Also

- [Platform Tuple Support in install_guide](platform-tuple-support.md)
- [Design: When Clause Platform Tuples](DESIGN-when-clause-platform-tuples.md)
