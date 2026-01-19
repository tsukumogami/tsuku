---
status: Current
problem: Recipes must hardcode dependency versions in action parameters like RPATH configuration, creating maintenance burden when dependency versions change and contradicting tsuku's version-aware design.
decision: Extend variable expansion syntax to support dot-notation references like `{deps.openssl.version}` by flattening resolved dependency versions into the existing variable map.
rationale: This approach maintains consistency with existing `{variable}` syntax, requires minimal code changes by reusing `ExecutionContext.Dependencies`, and allows recipe authors to construct flexible path formats while automatically staying in sync with resolved dependency versions.
---

# Dependency Version References in Action Parameters

## Status

Current

## Context and Problem Statement

Recipes that build tools from source often need to reference the resolved versions of their dependencies in action parameters. Use cases include:

- **RPATH configuration**: Library paths must include the exact version-qualified directory names
- **Build flags**: Configure scripts may need dependency paths (e.g., `--with-openssl=/path/to/openssl-3.6.0`)
- **Environment variables**: Build environments may reference dependency locations

The most critical and common case is RPATH configuration. The curl recipe demonstrates this problem:

```toml
[metadata]
dependencies = ["openssl", "zlib"]

[[steps]]
action = "set_rpath"
rpath = "$ORIGIN/../lib:{libs_dir}/openssl-3.6.0/lib:{libs_dir}/zlib-1.3.1/lib"
```

The RPATH contains hardcoded version strings (`openssl-3.6.0`, `zlib-1.3.1`). When dependency versions change (e.g., OpenSSL updates to 3.7.0), the RPATH becomes invalid and the built binary fails to find its libraries at runtime.

This creates a maintenance burden: recipe authors must manually track dependency version changes and update all recipes that reference those dependencies. For a package manager that aims to be "version-aware" and handle updates automatically, hardcoded versions are a fundamental contradiction.

The variable expansion system (`internal/actions/util.go`) already supports `{version}`, `{os}`, `{arch}`, `{install_dir}`, `{work_dir}`, and `{libs_dir}`. The `ExecutionContext` already carries resolved dependency versions in the `Dependencies` field. The gap is connecting these two systems so recipes can use `{deps.openssl.version}` syntax.

### Scope

**In scope:**
- Variable expansion syntax for referencing dependency versions (e.g., `{deps.<name>.version}`)
- Integration with existing `ExecutionContext.Dependencies` data
- Updating curl recipe to use dynamic version references

**Out of scope:**
- Explicit version pinning in dependency declarations (`openssl@3.6.0`) - already supported
- Version range constraints - future enhancement
- Cross-platform library naming (`{lib_ext}`) - tracked in #653

## Decision Drivers

- **DRY principle**: Version information should not be duplicated across recipe metadata and action parameters
- **Backward compatibility**: Existing recipes with hardcoded versions must continue to work
- **Simplicity**: The syntax should be intuitive and consistent with existing variable patterns
- **Minimal code changes**: Leverage existing `ResolvedDeps` data rather than adding new resolution logic
- **Error clarity**: Invalid variable references should produce clear error messages

### Key Assumptions

1. **Dependencies are installed before the dependent recipe runs**: The dependency resolution and installation happens before the main recipe's steps execute, so resolved versions are available in `ExecutionContext.Dependencies`

2. **Library vs Tool directory distinction**: Libraries are installed to `$TSUKU_HOME/libs/`, tools to `$TSUKU_HOME/tools/`. The variable expansion provides versions; recipe authors construct appropriate paths using `{libs_dir}` or constructing tool paths as needed

3. **ResolvedDeps contains actual versions**: While the resolver initially stores version constraints (e.g., "latest"), by execution time the actual resolved versions (e.g., "3.6.0") must be populated in `ExecutionContext.Dependencies.InstallTime`. **Implementation note**: The executor must be modified to track installed dependency versions from `DependencyPlan` items and inject the actual versions (not constraints) into `ExecutionContext.Dependencies`

## Considered Options

### Option A: Dot-Notation Variables (`{deps.openssl.version}`)

Extend the variable expansion syntax to support nested access using dots:

```toml
rpath = "$ORIGIN/../lib:{libs_dir}/openssl-{deps.openssl.version}/lib:{libs_dir}/zlib-{deps.zlib.version}/lib"
```

**Implementation approach:**
1. Add a new function `GetStandardVarsWithDeps(version, installDir, workDir, libsDir string, deps ResolvedDeps)` that builds the vars map including `deps.<name>.version` entries
2. Update action call sites to pass `ctx.Dependencies` when expanding variables

**Pros:**
- Consistent with existing `{variable}` syntax
- Dot notation is intuitive (widely used in templating languages)
- Single function change, minimal code surface
- Clear variable naming: `deps.` prefix makes scope obvious

**Cons:**
- Simple string replacement doesn't understand nesting - must flatten to `"deps.openssl.version"` keys
- Silent failure if dependency not found - unexpanded variable causes subtle runtime errors

### Option B: Dedicated Expansion Function for Dependencies

Create a separate expansion pass specifically for dependency references:

```toml
rpath = "$ORIGIN/../lib:{libs_dir}/{dep:openssl}/lib:{libs_dir}/{dep:zlib}/lib"
```

Where `{dep:openssl}` expands to `openssl-3.6.0` (full directory name).

**Implementation approach:**
1. Add `ExpandDeps(s string, deps ResolvedDeps, libsDir string)` function
2. Call after `ExpandVars()` in actions that need it

**Pros:**
- Separation of concerns - dependency expansion is distinct from standard vars
- Expands to full directory name, not just version
- No risk of key collisions with standard variables
- Visually distinct syntax makes dependency references easy to identify

**Cons:**
- Different syntax from existing variables (`:` instead of `.`) - inconsistent
- Two-pass expansion adds complexity
- Less flexible - only provides full directory name, not individual components like version

### Option C: Template Function Syntax (`{dep("openssl", "version")}`)

Use function-call syntax for more explicit semantics:

```toml
rpath = "$ORIGIN/../lib:{libs_dir}/openssl-{dep("openssl", "version")}/lib"
```

**Pros:**
- Explicit about what's being accessed
- Extensible to other dependency properties in future

**Cons:**
- More verbose
- Requires parser changes to handle function syntax
- Inconsistent with simple `{variable}` pattern
- Over-engineered for current needs

### Option D: Build-Time Path Resolution

Instead of expanding version variables, have actions resolve full dependency paths at execution time:

```toml
rpath = "$ORIGIN/../lib:{dep_lib:openssl}:{dep_lib:zlib}"
```

Where `{dep_lib:openssl}` resolves to the actual `lib/` path of the installed dependency.

**Pros:**
- Abstracts away the directory naming convention entirely
- More robust if directory structure changes

**Cons:**
- Doesn't work for cases where version is needed separately (not just paths)
- More complex implementation (must look up actual installed paths)
- Less transparent - recipe author can't see exactly what path will be used

## Decision Outcome

**Chosen: Option A (Dot-Notation Variables)**

### Summary

Extend `GetStandardVars()` to include `deps.<name>.version` entries by flattening the dependency map into the existing variable map. This approach requires minimal code changes, maintains consistency with the existing `{variable}` syntax, and provides clear, readable recipes.

### Rationale

Option A was chosen because:

1. **Consistency**: The `{deps.openssl.version}` syntax follows the established pattern of `{variable}` replacement, making it intuitive for recipe authors already familiar with `{version}`, `{os}`, etc.

2. **Minimal implementation complexity**: By flattening dependency versions into the existing vars map (e.g., key = `"deps.openssl.version"`, value = `"3.6.0"`), we reuse the existing `ExpandVars()` function without any parsing changes.

3. **Flexibility**: Exposing just the version (not the full path) allows recipe authors to construct whatever path format they need. This is important because different contexts may need different path constructions.

4. **Future extensibility**: The `deps.<name>.<property>` pattern can be extended to support other dependency properties without syntax changes:
   - `{deps.openssl.version}` - version string (this design)
   - `{deps.openssl.dir}` - directory name like `openssl-3.6.0` (future convenience)
   - `{deps.openssl.path}` - full path handling lib vs tool distinction (future)

Option B was rejected because a separate expansion pass adds complexity without clear benefit. Option C was rejected as over-engineered for the current use case. Option D was rejected because it doesn't support cases where the version itself (not just paths) is needed.

## Solution Architecture

### Variable Map Extension

Modify `GetStandardVars()` to accept an optional `ResolvedDeps` parameter:

```go
// GetStandardVarsWithDeps returns standard variable mappings including dependency versions.
// For each dependency in deps.InstallTime, adds "deps.<name>.version" to the map.
func GetStandardVarsWithDeps(version, installDir, workDir, libsDir string, deps ResolvedDeps) map[string]string {
    vars := GetStandardVars(version, installDir, workDir, libsDir)

    // Add dependency version variables
    for depName, depVersion := range deps.InstallTime {
        key := fmt.Sprintf("deps.%s.version", depName)
        vars[key] = depVersion
    }

    return vars
}
```

### Action Integration Points

Actions that use `ExpandVars()` and may need dependency references:

| Action | File | Needs Update |
|--------|------|--------------|
| `set_rpath` | `set_rpath.go` | Yes - primary use case |
| `run_command` | `run_command.go` | Yes - command args may reference deps |
| `configure_make` | `configure_make.go` | Yes - configure flags may reference deps |
| `chmod` | `chmod.go` | Maybe - file paths could reference deps |

For each action, update the `GetStandardVars()` call to `GetStandardVarsWithDeps()` and pass `ctx.Dependencies`.

### Recipe Syntax

Recipes use the new syntax as follows:

```toml
# Before (hardcoded versions)
[[steps]]
action = "set_rpath"
rpath = "$ORIGIN/../lib:{libs_dir}/openssl-3.6.0/lib:{libs_dir}/zlib-1.3.1/lib"

# After (dynamic version references)
[[steps]]
action = "set_rpath"
rpath = "$ORIGIN/../lib:{libs_dir}/openssl-{deps.openssl.version}/lib:{libs_dir}/zlib-{deps.zlib.version}/lib"
```

### Error Handling

When a dependency referenced in `{deps.X.version}` is not found in `ResolvedDeps`:

1. The variable remains unexpanded (existing behavior for unknown variables)
2. This will cause a runtime error when the path doesn't exist
3. **Required mitigation**: After expansion, scan for remaining `{deps.*}` patterns and **fail the action** (not just warn). This prevents subtle runtime failures from typos or missing dependency declarations.

Example error:
```
Error: unexpanded dependency variable '{deps.openssl.version}' in rpath
  Hint: Is 'openssl' declared in recipe dependencies?
```

Additionally, validate dependency names before key construction using the existing kebab-case pattern validation (defense-in-depth).

### Migration Path

1. **Phase 1a**: Ensure executor propagates actual installed versions (not constraints) to `ExecutionContext.Dependencies`
2. **Phase 1b**: Add `GetStandardVarsWithDeps()` function in `util.go`
3. **Phase 1c**: Update `set_rpath.go` to use the new function with `ctx.Dependencies`
4. **Phase 2**: Update curl recipe to use `{deps.openssl.version}` and `{deps.zlib.version}`
5. **Phase 3**: Update other actions (`run_command`) as needed (note: `configure_make` already uses `ctx.Dependencies` directly for build paths)
6. **Phase 4**: Add post-expansion validation to fail on unexpanded `{deps.*}` patterns

### Future Enhancements

These extensions are compatible with the chosen syntax but out of scope for initial implementation:

- `{deps.NAME.dir}` - returns `NAME-VERSION` (convenience for directory construction)
- `{deps.NAME.path}` - returns full path, handling libs vs tools distinction automatically
- Build-time validation that warns about references to undeclared dependencies

## Implementation Approach

### Files to Modify

| File | Changes |
|------|---------|
| `internal/executor/executor.go` | Ensure installed dependency versions (not constraints) populate `ExecutionContext.Dependencies` |
| `internal/actions/util.go` | Add `GetStandardVarsWithDeps()` function and unexpanded variable detection |
| `internal/actions/set_rpath.go` | Use `GetStandardVarsWithDeps()` with `ctx.Dependencies` |
| `internal/recipe/recipes/c/curl.toml` | Replace hardcoded versions with `{deps.X.version}` |

Note: `configure_make.go` already correctly uses `ctx.Dependencies.InstallTime` directly for build flags and paths; it likely requires no changes.

### Testing Strategy

1. **Unit tests** for `GetStandardVarsWithDeps()`:
   - Empty deps returns standard vars only
   - Deps are correctly flattened to `deps.<name>.version` keys
   - Special characters in dep names are handled

2. **Unit tests** for unexpanded variable detection:
   - Expansion with valid deps succeeds
   - Expansion with missing dep reference fails with clear error message
   - `{deps.typo.version}` detected as unexpanded

3. **Integration test** for curl recipe:
   - Build curl with updated recipe
   - Verify RPATH contains correct resolved versions
   - Verify curl binary can find OpenSSL at runtime

## Security Considerations

### Download verification
Not applicable - this change affects variable expansion, not download or verification logic.

### Execution isolation
Not applicable - no change to execution permissions or isolation.

### Supply chain risks
**Low risk**: The dependency versions exposed via `{deps.X.version}` are the same versions already resolved and installed by tsuku. This change doesn't introduce new version resolution logic or external data sources.

### User data exposure
Not applicable - dependency version information is not sensitive and is already visible in installation output.

## Consequences

### Positive

- **Eliminates version drift**: Recipe RPATH configurations automatically use correct dependency versions
- **Reduces maintenance burden**: Dependency version updates don't require manual recipe edits
- **Improves recipe reliability**: No more broken binaries due to stale hardcoded versions
- **Consistent with existing patterns**: Uses familiar `{variable}` syntax

### Negative

- **Slight API change**: New function `GetStandardVarsWithDeps()` adds to the actions API surface
- **Additional validation overhead**: Post-expansion check for unexpanded `{deps.*}` patterns adds minor processing time

### Neutral

- **Backward compatible**: Existing recipes with hardcoded versions continue to work
- **Opt-in adoption**: Recipe authors choose when to migrate to dynamic references
