---
status: Planned
problem: The install_binaries action's binaries parameter conflates two separate concerns (files to export vs files to make executable) and uses misleading semantics, creating confusion for recipe authors and blocking static analysis.
decision: Rename binaries parameter to outputs and infer executability from path prefix (bin/ = executable) with an optional executables parameter for edge cases.
rationale: This approach provides semantic clarity while maintaining backward compatibility through convention-based inference. Path prefix inference works for all 35 existing recipes and aligns with Unix conventions that developers already understand. The optional executables override handles future edge cases without burdening typical recipes.
---

# DESIGN: install_binaries Parameter Semantics

## Status

Planned

## Upstream Design Reference

This design addresses issue [#648](https://github.com/tsukumogami/tsuku/issues/648).

## Context and Problem Statement

The `install_binaries` action uses a parameter named `binaries` that actually contains a mix of executables, libraries, and other files. This creates semantic confusion and conflates two separate concerns: what to export vs what to make executable.

**Current behavior:**

```toml
# ncurses.toml - "binaries" contains libraries
[[steps]]
action = "install_binaries"
install_mode = "directory"
binaries = [
    "bin/ncursesw6-config",      # Actual binary
    "lib/libncurses.so",         # Library - NOT a binary!
    "lib/libncurses.dylib",      # Library - NOT a binary!
]
```

The parameter behavior differs by `install_mode`:
- `install_mode = "binaries"`: Copies files and applies `chmod 0755` to ALL items
- `install_mode = "directory"`: Copies entire tree, uses list only for symlink creation

**Why this matters:**

1. **Misleading semantics**: The name "binaries" implies executable files, but the list includes shared libraries (`.so`, `.dylib`), headers, and other artifacts
2. **Conflated concerns**: The parameter serves two purposes that should be separate:
   - What files to export/track for the installation
   - What files to make executable
3. **Mode-dependent interpretation**: The same parameter means different things depending on `install_mode`
4. **Recipe author confusion**: 17 recipes currently mix libraries with executables in the `binaries` list, following a confusing precedent set by early recipes

**Impact:**

- 35 recipes use `install_mode = "directory"`
- 17 of those mix library paths (`lib/`) with executable paths (`bin/`)
- Recipe authors must understand undocumented semantics
- Static analysis tools cannot reliably determine what files are executables

### Scope

**In scope:**
- Renaming the `binaries` parameter to accurately reflect its purpose
- Separating the concepts of "files to export" from "files to make executable"
- Updating affected recipes to use the new parameter names
- Migration path for existing recipes

**Out of scope:**
- Changes to `install_mode` behavior
- New installation strategies
- The `install_binaries` action name itself (that's a separate concern)

## Decision Drivers

1. **Semantic clarity**: Parameter names should accurately describe their contents
2. **Separation of concerns**: Export listing and executability should be independent
3. **Recipe author experience**: The API should be intuitive without documentation
4. **Backward compatibility**: Existing recipes should continue to work during transition
5. **Static analyzability**: Tools should be able to determine which files are executables
6. **Simplicity**: Avoid adding complexity where simple conventions suffice
7. **Codebase consistency**: Align with existing patterns in other actions

## Research Findings

### Industry Terminology

Package managers use distinct terminology for different artifact types:

| Package Manager | Executables | Libraries | All Outputs |
|-----------------|-------------|-----------|-------------|
| [Cargo (Rust)](https://doc.rust-lang.org/cargo/reference/cargo-targets.html) | `[[bin]]` targets | `[lib]` target | targets |
| [npm](https://docs.npmjs.com/cli/v7/configuring-npm/package-json/) | `bin` field | - | `files` |
| [Homebrew](https://docs.brew.sh/Formula-Cookbook) | linked to `bin/` | linked to `lib/` | - |
| [Nix](https://nixos.org/) | - | - | `outputs` |

**Key insight**: Industry practice consistently distinguishes between:
1. **Executables/binaries**: Files that should be runnable (in PATH)
2. **Libraries**: Files linked by other programs (not in PATH)
3. **Outputs/artifacts**: All files produced by a build

### Existing Codebase Patterns

The tsuku codebase already establishes a pattern with `executables`:

**`npm_install` action:**
```go
// executables (required): List of executable names to install
executables, ok := GetStringSlice(params, "executables")
```

**`configure_make` action:**
```go
// executables (required): List of executable names to verify
executables, ok := GetStringSlice(params, "executables")
```

Both actions use `executables` to mean "files that should be runnable/verified in `bin/`". This is the correct semantic term for executable files.

**Recipe example (readline-source.toml):**
```toml
[[steps]]
action = "configure_make"
source_dir = "."
executables = []  # Empty because it's a library-only build
```

This pattern demonstrates that the codebase already treats `executables` as explicitly separate from library outputs.

### Research Summary

**Industry standards:**
- Use `bin` or `executables` for runnable files
- Use `lib` or libraries for shared/static libraries
- Use `outputs`, `exports`, or `files` for all artifacts

**Codebase patterns:**
- `executables` is already the established term for runnable files
- Library-type recipes (`type = "library"`) are already a distinct concept

**Implementation approach:**
- Adopt `executables` for runnable files (aligns with existing actions)
- Use `outputs` or `files` for the complete list of exported artifacts
- Infer executability from path prefix (`bin/` = executable) as a safe default

## Considered Options

This design addresses two related but independent decisions:
1. What to rename the `binaries` parameter to
2. How to determine which files should be made executable

### Decision 0: Status Quo

#### Option 0: Improve Documentation Only

Keep `binaries` parameter name, add documentation and linting.

```toml
# No code changes - add CONTRIBUTING.md documentation
# The binaries parameter lists files to symlink, not just executables
```

**Pros:**
- Zero code changes
- Zero recipe migration needed
- Low risk and effort

**Cons:**
- Semantic confusion persists in API
- Documentation doesn't fix the naming problem
- Static analysis still cannot distinguish executables from libraries
- New recipe authors will repeat the confusion

### Decision 1: Parameter Naming

#### Option 1A: Rename to `files`

Replace `binaries` with `files` - a generic term for all output artifacts.

```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
files = ["bin/ncursesw6-config", "lib/libncurses.so"]
```

**Pros:**
- Semantically accurate - these ARE files
- Simple and familiar terminology
- Matches npm's `files` field concept

**Cons:**
- Very generic - doesn't convey installation intent
- Could be confused with "all files in directory"
- Doesn't align with existing tsuku terminology

#### Option 1B: Rename to `outputs`

Replace `binaries` with `outputs` - files produced by the build that should be tracked.

```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["bin/ncursesw6-config", "lib/libncurses.so"]
```

**Pros:**
- Semantically accurate - these are build outputs
- Matches Nix terminology (`outputs`)
- Conveys "things the build produces"

**Cons:**
- Could be confused with stdout/stderr outputs
- Slightly technical terminology

#### Option 1C: Rename to `exports`

Replace `binaries` with `exports` - files that should be "exported" for external use.

```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
exports = ["bin/ncursesw6-config", "lib/libncurses.so"]
```

**Pros:**
- Semantically accurate for the use case
- Matches npm's `exports` field concept
- Conveys "things others can use"

**Cons:**
- Could be confused with JavaScript module exports
- May not be intuitive for compiled software

### Decision 2: Executability Logic

#### Option 2A: Infer from Path Prefix

Determine executability from the file path: files in `bin/` are executable, files in `lib/` are not.

```go
// No explicit executables parameter needed
for _, path := range outputs {
    if strings.HasPrefix(path, "bin/") {
        chmod(path, 0755)
    }
}
```

**Pros:**
- Zero config - follows Unix convention
- No migration burden - existing recipes "just work"
- Handles most real-world cases correctly
- Simple mental model for recipe authors

**Cons:**
- Some executables might not be in `bin/` (edge case)
- Implicit behavior may surprise users
- Relies on recipe authors using standard paths

#### Option 2B: Explicit `executables` Parameter

Add a separate `executables` parameter listing files that need executable permission.

```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["bin/ncursesw6-config", "lib/libncurses.so"]
executables = ["bin/ncursesw6-config"]
```

**Pros:**
- Explicit is better than implicit
- Handles edge cases (executables outside `bin/`)
- Aligns with existing `configure_make` and `npm_install` patterns
- Full control for recipe authors

**Cons:**
- Redundant specification for typical cases
- More verbose recipes
- Migration burden for existing recipes

#### Option 2C: File Type Detection

Detect whether files are executables by inspecting ELF/Mach-O headers or existing permissions.

```go
for _, path := range outputs {
    if isELFExecutable(path) || isMachOExecutable(path) {
        chmod(path, 0755)
    }
}
```

**Pros:**
- Handles edge cases automatically
- Works for any directory structure
- No configuration needed

**Cons:**
- Adds complexity (ELF/Mach-O parsing)
- May not work for scripts without shebangs
- Cross-platform detection is tricky
- Shared libraries are also ELF/Mach-O (can detect with ET_EXEC vs ET_DYN but adds more complexity)

#### Option 2D: Hybrid (Infer with Override)

Default to path-based inference (Option 2A), but allow explicit `executables` parameter to override.

```toml
# Default: inferred from path (bin/* = executable)
[[steps]]
action = "install_binaries"
outputs = ["bin/tool", "lib/lib.so"]

# Override when needed
[[steps]]
action = "install_binaries"
outputs = ["libexec/helper", "lib/lib.so"]
executables = ["libexec/helper"]  # Not in bin/, needs explicit marking
```

**Pros:**
- Zero migration burden for typical cases
- Handles edge cases when needed
- Explicit when implicit won't work
- Best of both worlds

**Cons:**
- Two ways to specify executables (potential confusion)
- Slightly more complex implementation
- Need to document when to use override

### Evaluation Against Decision Drivers

| Option | Semantic Clarity | Separation of Concerns | Author Experience | Backward Compat | Static Analysis | Simplicity | Codebase Consistency |
|--------|------------------|------------------------|-------------------|-----------------|-----------------|------------|---------------------|
| 0 (status quo) | Poor | Poor | Fair | Good | Poor | Good | Fair |
| 1A (files) | Fair | N/A | Good | Good | Good | Good | Poor |
| 1B (outputs) | Good | N/A | Good | Good | Good | Good | Fair |
| 1C (exports) | Good | N/A | Fair | Good | Good | Good | Poor |
| 2A (path infer) | Good | Good | Good | Good | Fair | Good | Fair |
| 2B (explicit) | Good | Good | Fair | Poor | Good | Fair | Good |
| 2C (detection) | Fair | Good | Good | Good | Poor | Fair | Poor |
| 2D (hybrid) | Good | Good | Good | Good | Good | Fair | Good |

### Validation Results

**Audit of 35 directory-mode recipes** (completed during review):
- All executable paths use `bin/` prefix
- All library paths use `lib/` prefix
- No executables found outside standard paths
- Option 2A (path inference) would correctly handle all existing recipes

### Uncertainties

- **Future recipes**: Path inference assumes future recipe authors will follow Unix conventions, which is not enforced.
- **Script executables**: Some tools install shell scripts that need executable bit but may not be in `bin/` (not currently seen in recipes).
- **Recipe author expectations**: We haven't validated whether recipe authors find explicit specification (2B) valuable or burdensome.

## Decision Outcome

**Chosen: 1B (outputs) + 2D (hybrid)**

### Summary

Rename the `binaries` parameter to `outputs` and infer executability from path prefix (`bin/` = executable), with an optional `executables` parameter for edge cases. This combination provides semantic clarity while maintaining backward compatibility through convention-based inference.

### Rationale

**Why 1B (outputs) for naming:**
- **Semantic clarity**: "outputs" accurately describes the parameter's purpose - these are build outputs to track, not just executables
- **Codebase consistency**: Aligns with Nix's `outputs` terminology and the existing build-centric vocabulary in tsuku
- **Author experience**: Clearly conveys "things the build produces" without misleading implications

**Why 2D (hybrid) for executability:**
- **Backward compatibility**: All 35 existing directory-mode recipes follow Unix conventions and will work without modification
- **Static analyzability**: Tools can reliably determine executables by checking `bin/` prefix OR explicit `executables` list
- **Flexibility**: Handles edge cases (executables outside `bin/`) when they arise, without burdening typical recipes

**Alternatives rejected:**
- **Option 0 (status quo)**: Doesn't fix the semantic confusion or enable static analysis
- **Option 1A (files)**: Too generic, doesn't convey build/installation intent
- **Option 1C (exports)**: JavaScript connotation may confuse recipe authors
- **Option 2A (pure inference)**: No escape hatch for edge cases
- **Option 2B (pure explicit)**: Migration burden and verbosity for no benefit in typical cases
- **Option 2C (detection)**: Complexity not justified when path inference works for all current recipes

### Trade-offs Accepted

By choosing this option, we accept:
- **Implicit behavior**: Path-based inference is implicit, which may surprise users unfamiliar with Unix conventions
- **Two ways to specify executables**: The optional `executables` parameter creates a second code path, adding minor complexity
- **Recipe migration**: All recipes need `binaries` → `outputs` rename (can be automated)

These are acceptable because:
- Unix path conventions are well-established and intuitive to most developers
- The `executables` override is rarely needed (audit found 0 current use cases)
- Recipe migration is mechanical and can be done in a single PR

## Solution Architecture

### Overview

The solution replaces the `binaries` parameter with `outputs` across the `install_binaries` action, and adds path-based executability inference with an optional `executables` override.

### Parameter Changes

**Before:**
```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
binaries = ["bin/tool", "lib/libfoo.so"]
```

**After:**
```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["bin/tool", "lib/libfoo.so"]
# executables = ["bin/tool"]  # Optional: only needed if not in bin/
```

### Executability Logic

```go
// DetermineExecutables returns the list of files that should be made executable.
// If an explicit executables list is provided, use it.
// Otherwise, infer from path: files in bin/ are executable.
func DetermineExecutables(outputs []string, explicitExecutables []string) []string {
    if len(explicitExecutables) > 0 {
        return explicitExecutables
    }

    var result []string
    for _, path := range outputs {
        if strings.HasPrefix(path, "bin/") {
            result = append(result, path)
        }
    }
    return result
}
```

### Affected Code Paths

| File | Change |
|------|--------|
| `internal/actions/install_binaries.go` | Update parameter name, add `DetermineExecutables()` |
| `internal/recipe/recipe.go` | Update `BinaryMapping` struct field names if needed |
| `internal/recipe/types.go` | Update `ExtractBinaries()` to search for `outputs` |
| `internal/recipe/recipes/*/*.toml` | Rename `binaries` → `outputs` in `install_binaries` steps only |
| `testdata/recipes/*.toml` | Rename `binaries` → `outputs` in `install_binaries` steps |
| `docs/*.md` | Update documentation references |

**Note**: The `binaries` field appears in multiple contexts:
- `[metadata]` section: Lists exported executables - no change needed
- `set_rpath` action: Targets executables for RPATH patching - no change needed
- `install_binaries` action: This is the field being renamed

### Backward Compatibility

During a transition period, both `binaries` and `outputs` will be accepted:

```go
func (a *InstallBinariesAction) Preflight(params map[string]interface{}) *PreflightResult {
    result := &PreflightResult{}

    _, hasOutputs := params["outputs"]
    _, hasBinaries := params["binaries"]

    if !hasOutputs && !hasBinaries {
        result.AddError("install_binaries action requires 'outputs' parameter")
    }
    if hasOutputs && hasBinaries {
        result.AddError("cannot specify both 'outputs' and 'binaries'; use 'outputs' only")
    }
    // Deprecation warning added in Phase 3 (after migration)
    // ...
}

func (a *InstallBinariesAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    // Prefer outputs, fall back to binaries during transition
    outputs, hasOutputs := GetStringSlice(params, "outputs")
    if !hasOutputs {
        outputs, _ = GetStringSlice(params, "binaries")
    }
    // ...
}
```

After migration is complete (all recipes updated), the deprecation warning can be added and later changed to an error.

## Implementation Approach

### Phase 1: Core Changes
- Add `outputs` parameter support alongside `binaries` (both accepted during transition)
- Implement `DetermineExecutables()` with path inference
- Update `ExtractBinaries()` in types.go to search for `outputs`
- Update unit tests

### Phase 2: Recipe Migration
- Automated script to rename `binaries` → `outputs` in `install_binaries` steps only
- Manual review of edge cases
- Update testdata recipes

### Phase 3: Documentation and Deprecation
- Update CONTRIBUTING.md with new parameter guidance
- Update inline code documentation
- Add deprecation warning for `binaries` parameter
- Add lint warning for outputs outside `bin/` or `lib/`

### Phase 4: Cleanup (Future)
- Remove deprecated `binaries` support (post-migration, after release cycle)

## Security Considerations

### Download Verification

**Not applicable** - this design does not change how files are downloaded or verified. The `install_binaries` action operates on files already present in the work directory after previous download/extract steps. Checksum and signature verification happen in upstream actions (`download`, `download_file`, etc.) before `install_binaries` executes.

### Execution Isolation

**Minimal change with slight improvement:**

The current behavior applies `chmod 0755` to ALL files listed in `binaries` when using `install_mode = "binaries"`. This is overly permissive for libraries.

The new behavior:
- Only files in `bin/` (or explicitly listed in `executables`) receive `chmod 0755`
- Library files in `lib/` retain their original permissions (typically `0644` or `0755` from archive)

This is a **security improvement**: libraries should not be marked executable in most cases. While shared libraries are technically executable by the dynamic linker, they do not need the user-execute bit for normal operation.

### Supply Chain Risks

**No new risks introduced:**

This design changes parameter naming and permission logic, not the source of files. Supply chain trust continues to depend on:
- Recipe review (PR process)
- Checksum verification of downloads
- Optional signature verification

The only supply chain consideration is ensuring the migration script correctly transforms all recipes. A malicious migration could theoretically alter which files are made executable, but:
- Migration is reviewed as a PR
- Changed recipes are diffable
- Test coverage validates correct behavior

### User Data Exposure

**Not applicable** - this design does not access or transmit user data. The `install_binaries` action only:
- Reads files from the work directory (downloaded artifacts)
- Writes files to the installation directory (`$TSUKU_HOME/tools/`)
- Sets file permissions

No network access, no user data access, no telemetry impact.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Incorrect permission on executables | Path inference defaults to conservative (only `bin/`) | Edge cases need explicit `executables` |
| Migration script errors | PR review, automated tests, diffable changes | Human error possible |
| Confusion about executable status | Clear documentation, lint warnings | New users may be confused initially |

## Consequences

### Positive

- **Semantic clarity**: Parameter names accurately describe their contents
- **Static analyzability**: Tools can reliably identify executables via path prefix or explicit list
- **Zero migration burden**: Path inference means recipes work without changes beyond renaming
- **Future flexibility**: The `executables` override handles edge cases without recipe verbosity

### Negative

- **Implicit behavior**: New recipe authors must understand the `bin/` convention
- **Two code paths**: Supporting both inference and explicit list adds minor complexity
- **Deprecation churn**: All 35+ recipes need renaming (automated)

### Mitigations

- **Documentation**: Add clear guidance in CONTRIBUTING.md about `outputs` semantics
- **Lint checks**: Add preflight validation to warn about outputs not in `bin/` or `lib/`
- **Automation**: Provide script for `binaries` → `outputs` migration
