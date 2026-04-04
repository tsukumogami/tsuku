# Verification Reference

How to write `[verify]` sections that confirm a tool installed correctly.

---

## Mode Selection

### Version Mode (default)

Runs the command, extracts a version string, and compares it to the expected
version. Use this whenever the tool prints a parseable version.

```toml
[verify]
command = "trivy --version"
pattern = "{version}"
```

The `pattern` field is a regex where `{version}` expands to a version-matching
group. tsuku compares the captured group against the installed version.

Common pattern variations:

```toml
# Tool prints: "trivy 0.50.0"
pattern = "{version}"

# Tool prints: "Poetry (version 1.8.0)"
pattern = "version {version}"

# Tool prints: "pcre2grep version 10.42"
pattern = "pcre2grep version {version}"

# Tool prints a multi-line banner, version on its own line
pattern = "v{version}"
```

### Output Mode

Runs the command and checks that it succeeds (or produces output). No version
comparison. Use this when the tool doesn't print a parseable version string.

```toml
[verify]
command = "mytool --help"
mode = "output"
reason = "Binary has no --version flag"
```

The `reason` field documents why version mode isn't feasible. Required when
using output mode.

---

## Decision Flowchart

```
Does the tool have a --version or version flag?
  YES --> Does the output contain the version number?
            YES --> Use version mode.
                    Write a pattern that matches the output format.
            NO  --> Does a different flag print the version?
                      YES --> Use that flag with version mode.
                      NO  --> Use output mode.
  NO  --> Does --help or any flag confirm the tool is functional?
            YES --> Use output mode with that flag.
            NO  --> Use output mode with the bare command.
                    Set exit_code if it exits non-zero.
```

---

## Format Transforms

The `version_format` field normalizes the version string before comparison.
Set it in `[verify]` (per-verify) or `[metadata]` (recipe-wide).

| Transform | Input Example | Output | When to Use |
|-----------|---------------|--------|-------------|
| `raw` | `v1.2.3-beta` | `v1.2.3-beta` | Default. Use when versions match exactly. |
| `semver` | `1.2.3-beta+build` | `1.2.3` | Strip pre-release and build metadata. |
| `semver_full` | `v1.2.3-beta+build` | `1.2.3-beta+build` | Keep pre-release, strip leading v. |
| `strip_v` | `v1.2.3` | `1.2.3` | Version provider includes v prefix but tool output doesn't. |
| `calver` | `2024.01.15` | `2024.01.15` | Calendar versioning. |

The transform applies to the version extracted from tool output before
comparing it to the version resolved by the provider.

---

## Exit Code Handling

Some tools exit with a non-zero code when printing version info. By default
tsuku expects exit code 0. Override with:

```toml
[verify]
command = "mytool --version"
pattern = "{version}"
exit_code = 1
```

---

## Additional Verification Commands

When a tool installs multiple binaries, verify each one:

```toml
[verify]
command = "black --version"
pattern = "{version}"

[[verify.additional]]
command = "blackd --version"
pattern = "{version}"
```

---

## Common Patterns by Tool Type

### GitHub binary downloads

```toml
[verify]
command = "toolname --version"
pattern = "{version}"
```

Most Go and Rust binaries follow this pattern.

### Homebrew-installed tools

```toml
[verify]
command = "toolname --version"
pattern = "{version}"
```

Same as binary downloads. The homebrew action doesn't affect verification.

### Python tools (pipx_install)

```toml
[verify]
command = "ruff --version"
pattern = "ruff {version}"
```

Many Python CLIs prefix the tool name in version output.

### Ruby tools (gem_install)

Gems install to a non-standard location. Use `{install_dir}` to reference
the actual install path:

```toml
[verify]
command = "{install_dir}/bin/jekyll --version"
pattern = "jekyll {version}"
```

This ensures you verify the tsuku-installed copy, not a system-installed one.

### Node tools (npm_install)

```toml
[verify]
command = "zx --version"
pattern = "{version}"
```

### Tools with non-standard version output

```toml
# Poetry: "Poetry (version 1.8.0)"
[verify]
command = "poetry --version"
pattern = "version {version}"

# Skaffold: custom subcommand
[verify]
command = "skaffold version"
pattern = "{version}"
```

---

## Validation

Run `tsuku validate --strict` to catch verification issues:

- Missing `[verify]` section (required for tools, not libraries)
- Missing `pattern` in version mode
- Pattern without `{version}` placeholder
- Unrecognized `version_format` values

For troubleshooting installed tools, see `docs/guides/GUIDE-troubleshooting-verification.md`
(available to central registry contributors).
