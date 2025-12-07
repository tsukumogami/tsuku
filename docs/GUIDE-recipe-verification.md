# Recipe Verification Guide

This guide explains how to configure the `[verification]` section of tsuku recipes to ensure installed tools work correctly.

## Overview

Verification confirms that a tool was installed successfully and is functional. After installation, tsuku runs the tool with a version-checking command and validates the output matches the expected version.

## Quick Start

Most tools support `--version` and output their version in a standard format. For these, verification is straightforward:

```toml
[verification]
command = "mytool --version"
pattern = "mytool {version}"
```

The `{version}` placeholder is replaced with the installed version and matched against the command output.

## Verification Modes

### Version Mode (Default)

Version mode is the default and most common. It runs a version command and checks that the output contains the expected version string.

```toml
[verification]
command = "ripgrep --version"
pattern = "ripgrep {version}"
```

When you install `ripgrep` version `14.1.0`, tsuku:
1. Runs `ripgrep --version`
2. Looks for "ripgrep 14.1.0" in the output
3. Marks verification as passed if found

### Output Mode

Use output mode when a tool doesn't have a traditional `--version` flag, or when matching the version directly isn't practical.

```toml
[verification]
mode = "output"
command = "mytool --help"
pattern = "Usage: mytool"
reason = "mytool does not support --version"
```

Output mode checks for a static pattern in the output without version matching. The `reason` field documents why output mode is necessary.

## Version Format Transforms

Tools report versions in different formats. Version format transforms normalize these differences.

### semver (Recommended)

Extracts the core `X.Y.Z` version, stripping prefixes, suffixes, and metadata:

```toml
[verification]
command = "biome --version"
pattern = "biome {version}"
version_format = "semver"
```

| Tool Output | Extracted Version |
|-------------|-------------------|
| `biome 2.3.8` | `2.3.8` |
| `v1.29.0` | `1.29.0` |
| `go1.21.0` | `1.21.0` |
| `2.4.0-0` | `2.4.0` |
| `tool-name-v2.3.8-linux` | `2.3.8` |

### semver_full

Preserves prerelease and build metadata:

```toml
[verification]
command = "mytool --version"
pattern = "{version}"
version_format = "semver_full"
```

| Tool Output | Extracted Version |
|-------------|-------------------|
| `v1.2.3-rc.1` | `1.2.3-rc.1` |
| `v1.2.3+build.123` | `1.2.3+build.123` |
| `v1.2.3-beta.1+build` | `1.2.3-beta.1+build` |

### strip_v

Removes only the leading `v` or `V` prefix from the installed version before matching:

```toml
[verification]
command = "deno --version"
pattern = "deno {version}"
version_format = "strip_v"
```

Use this when the tool outputs version without `v` prefix but GitHub tags include it (e.g., tag `v1.29.0` but tool outputs `deno 1.29.0`).

| Installed Version | Transformed For Matching |
|-------------------|--------------------------|
| `v1.29.0` | `1.29.0` |
| `V1.0.0` | `1.0.0` |
| `1.2.3` | `1.2.3` (unchanged) |

### raw (Default)

Uses the version string exactly as-is, with no transformation:

```toml
[verification]
command = "go version"
pattern = "go version go{version}"
version_format = "raw"
```

This is the default when `version_format` is not specified.

## Handling Non-Zero Exit Codes

Some tools return non-zero exit codes even on success. Use `exit_code` to specify the expected code:

```toml
[verification]
mode = "output"
command = "goimports -h"
pattern = "usage: goimports"
exit_code = 2
reason = "goimports returns exit code 2 for help output"
```

By default, tsuku expects exit code `0`.

## Examples

### Standard Tool with v-prefixed Version

```toml
# ripgrep outputs: ripgrep 14.1.0
[verification]
command = "rg --version"
pattern = "ripgrep {version}"
```

### Tool with Non-Standard Version Format

```toml
# biome outputs: biome 2.3.8-nightly.abc123
[verification]
command = "biome --version"
pattern = "biome {version}"
version_format = "semver"
```

### Go-Style Version Prefix

```toml
# go outputs: go version go1.21.0 linux/amd64
[verification]
command = "go version"
pattern = "go version go{version}"
```

### Tool Without --version Support

```toml
# goimports has no --version, use help output
[verification]
mode = "output"
command = "goimports -h"
pattern = "usage: goimports"
exit_code = 2
reason = "goimports does not have a --version flag"
```

### Multiple Binaries

When a recipe installs multiple binaries, verify the primary one:

```toml
[verification]
command = "terraform version"
pattern = "Terraform v{version}"
version_format = "strip_v"
```

## Decision Flowchart

Use this flowchart to choose the right verification configuration:

```
Does the tool have --version or similar?
├─ Yes → Use version mode (default)
│   │
│   └─ Does the version output match the installed version exactly?
│       ├─ Yes → Use version_format = "raw" (or omit)
│       └─ No → Choose appropriate version_format:
│           ├─ Has v prefix only → strip_v
│           ├─ Has prefix/suffix/prerelease to remove → semver
│           └─ Need to preserve prerelease → semver_full
│
└─ No → Use mode = "output"
    │
    ├─ Find a command that produces consistent output (--help, etc.)
    ├─ Set pattern to match stable text in output
    ├─ Set exit_code if the command returns non-zero
    └─ Document the reason with the "reason" field
```

## Troubleshooting

### Verification Fails with "version mismatch"

The extracted version doesn't match the installed version. Common causes:

1. **Wrong version_format**: Try `semver` to extract just X.Y.Z
2. **Tool reports different version**: Some tools report build numbers or dates instead of release versions
3. **Pattern mismatch**: Ensure the pattern matches the actual output format

Debug by running the command manually:
```bash
mytool --version
```

### Verification Fails with "pattern not found"

The pattern doesn't match the command output. Check:

1. **Case sensitivity**: Patterns are case-sensitive
2. **Exact spacing**: Whitespace must match exactly
3. **Tool output format**: Run the command manually to see actual output

### Tool Returns Non-Zero Exit Code

If the tool succeeds but returns a non-zero exit code:

```toml
[verification]
exit_code = 2  # Expected exit code
```

## Validation

Before submitting a recipe, validate it locally:

```bash
# Validate recipe syntax and rules
./tsuku validate path/to/recipe.toml

# Strict mode checks verification best practices
./tsuku validate --strict path/to/recipe.toml
```

Strict validation warns when:
- Version mode patterns don't include `{version}`
- Output mode is used without a `reason` field
