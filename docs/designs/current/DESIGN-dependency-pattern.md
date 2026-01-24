---
status: Current
problem: Dependencies between tools are handled inconsistently across actions, creating opacity about what each tool needs and making dependency graphs impossible to compute statically.
decision: Implement fully implicit dependencies where actions declare their install-time and runtime requirements, with step and recipe-level overrides for edge cases.
rationale: This approach eliminates boilerplate for the common case (95% of recipes need no declarations) while allowing explicit control when needed, and makes dependencies statically analyzable for features like dependency trees and dependant warnings.
---

# Design: Implicit Dependency Pattern

## Status

Current
**Created**: 2025-12-07
**Milestone**: [Implicit Dependency Pattern](https://github.com/tsukumogami/tsuku/milestone/11)

## Context and Problem Statement

Tsuku lacks a consistent pattern for handling dependencies between tools. Currently, different actions handle dependencies differently:

| Action | Current Behavior | Problem |
|--------|------------------|---------|
| `npm_install` | Calls `EnsureNpm()` internally | Opaque - recipe doesn't show nodejs dependency |
| `pipx_install` | Calls `EnsurePipx()` internally | Hidden dependency on python-standalone |
| `go_install` | Calls internal resolution | Inconsistent - errors if Go not installed |
| `cargo_install` | Calls internal resolution | Same inconsistency as go_install |
| `cpan_install` | Expects perl pre-installed | No bootstrapping at all |

This creates several problems:

1. **Opacity**: Can't tell from a recipe what dependencies are needed
2. **Inconsistency**: Different actions handle dependencies differently
3. **Runtime vs Install-time confusion**: No clear distinction - nodejs is needed both to install and run npm packages, but Go is only needed to compile (not run) Go binaries
4. **Boilerplate**: Recipe authors shouldn't need to declare obvious dependencies (npm packages need nodejs)

### Why Now

The current bootstrap code in `internal/install/bootstrap.go` works but:
- Hides the dependency relationship from users and tooling
- Makes dependency graphs impossible to compute statically
- Prevents features like `tsuku info <tool>` showing dependency trees
- Creates inconsistent UX (some tools auto-install deps, others error)

### Scope

**In scope:**
- Implicit dependency declarations for ecosystem actions (npm, pip, cargo, go, gem, cpan)
- Step-level overrides for edge cases (compiled npm binaries like esbuild)
- Recipe-level version pinning
- Runtime wrapper generation with correct PATH
- State tracking of dependency relationships

**Out of scope:**
- Conditional/platform-specific dependencies
- Optional dependencies
- Build-flag dependencies (CGO_ENABLED implications)
- Version constraint solving (only simple pinning)

## Decision Drivers

- **Minimize boilerplate**: 95% of recipes should need zero dependency declarations
- **Explicit when needed**: Edge cases must have escape hatches
- **Distinguish install vs runtime**: Compiled binaries (Go, Rust) don't need runtime deps
- **Static analyzability**: Dependency graphs should be computable without execution
- **Backward compatibility**: Existing recipes must continue working

## Considered Options

### Option A: Fully Explicit Dependencies

Every recipe must declare all dependencies.

```toml
[metadata]
name = "turbo"
dependencies = ["nodejs"]
runtime_dependencies = ["nodejs"]
```

**Pros:**
- Maximum transparency
- Simple mental model

**Cons:**
- Massive boilerplate for common cases
- Easy to forget runtime_dependencies
- Duplicates what actions already know

### Option B: Fully Implicit Dependencies (SELECTED)

Actions declare their dependencies; recipes inherit them automatically.

```toml
# turbo.toml - no declarations needed
[[steps]]
action = "npm_install"
package = "turbo"
```

The `npm_install` action knows it needs `nodejs` for both install and runtime.

**Pros:**
- Zero boilerplate for common case
- Actions encapsulate their contracts
- Recipe authors focus on tool-specific config

**Cons:**
- Less visible without tooling
- Need escape hatches for edge cases

### Option C: Hybrid with Recipe-Level Defaults

Recipe metadata sets defaults, actions can add to them.

```toml
[metadata]
ecosystem = "nodejs"  # Implies nodejs for install+runtime
```

**Pros:**
- Explicit ecosystem declaration
- Still reduces boilerplate

**Cons:**
- New concept (ecosystem) to learn
- Doesn't map cleanly to multi-action recipes

## Decision Outcome

**Selected: Option B - Fully Implicit Dependencies** with override escape hatches.

The key insight: **Actions that install into an ecosystem runtime (npm->nodejs, pip->python) should imply that runtime. Actions that compile standalone binaries (go_install, cargo_install) should not imply runtime dependencies.**

This eliminates boilerplate for 95% of recipes while preserving explicit control for edge cases.

## Solution Architecture

### Action Dependency Registry

A central registry declares what each action needs:

```go
// internal/actions/dependencies.go

type ActionDeps struct {
    InstallTime []string  // Needed during tsuku install
    Runtime     []string  // Needed when tool runs
}

var ActionDependencies = map[string]ActionDeps{
    // Ecosystem actions: both install-time and runtime
    "npm_install":   {InstallTime: []string{"nodejs"}, Runtime: []string{"nodejs"}},
    "pipx_install":  {InstallTime: []string{"pipx"},   Runtime: []string{"python"}},
    "gem_install":   {InstallTime: []string{"ruby"},   Runtime: []string{"ruby"}},
    "cpan_install":  {InstallTime: []string{"perl"},   Runtime: []string{"perl"}},

    // Compiled binary actions: install-time only
    "go_install":    {InstallTime: []string{"go"},     Runtime: nil},
    "cargo_install": {InstallTime: []string{"rust"},   Runtime: nil},
    "nix_install":   {InstallTime: []string{"nix-portable"}, Runtime: nil},

    // Download/extract actions: no dependencies
    "download":         {InstallTime: nil, Runtime: nil},
    "extract":          {InstallTime: nil, Runtime: nil},
    "github_archive":   {InstallTime: nil, Runtime: nil},
    // ... etc
}
```

### Override and Extension Mechanisms

Dependencies can be **replaced** (override implicit entirely) or **extended** (add to implicit).

**Step-level replace** (replaces action's runtime deps):
```toml
[[steps]]
action = "npm_install"
package = "esbuild"
runtime_dependencies = []  # esbuild is a compiled binary, no node needed
```

**Step-level extend** (adds to action's runtime deps):
```toml
[[steps]]
action = "npm_install"
package = "some-tool"
extra_runtime_dependencies = ["bash"]  # needs bash + nodejs (implicit)
```

**Recipe-level replace** (version pinning):
```toml
[metadata]
dependencies = ["nodejs@20"]  # Pin to Node 20, replaces implicit
```

**Recipe-level extend** (add without replacing):
```toml
[metadata]
extra_dependencies = ["wget"]  # adds wget to implicit install deps
extra_runtime_dependencies = ["bash"]  # adds bash to implicit runtime deps
```

### Dependency Resolution Algorithm

```
resolve_dependencies(recipe):
    install_deps = {}
    runtime_deps = {}

    for step in recipe.steps:
        action = ActionDependencies[step.action]

        # Install-time: action implicit + step extra
        for dep in action.InstallTime:
            install_deps[dep] = "latest"
        for dep in step.extra_dependencies:
            install_deps[parse(dep).name] = parse(dep).version

        # Runtime: replace OR extend
        if step.runtime_dependencies is defined:
            # Replace: use only what's declared
            for dep in step.runtime_dependencies:
                runtime_deps[parse(dep).name] = parse(dep).version
        else:
            # Implicit from action
            for dep in action.Runtime:
                runtime_deps[dep] = "latest"
            # Extend: add extra
            for dep in step.extra_runtime_dependencies:
                runtime_deps[parse(dep).name] = parse(dep).version

    # Recipe-level replace (if set, overrides everything above)
    if recipe.metadata.dependencies is defined:
        install_deps = {}  # clear implicit
        for dep in recipe.metadata.dependencies:
            install_deps[parse(dep).name] = parse(dep).version

    if recipe.metadata.runtime_dependencies is defined:
        runtime_deps = {}  # clear implicit
        for dep in recipe.metadata.runtime_dependencies:
            runtime_deps[parse(dep).name] = parse(dep).version

    # Recipe-level extend (adds to current set)
    for dep in recipe.metadata.extra_dependencies:
        install_deps[parse(dep).name] = parse(dep).version
    for dep in recipe.metadata.extra_runtime_dependencies:
        runtime_deps[parse(dep).name] = parse(dep).version

    # Resolve transitively (max depth 10)
    install_deps = resolve_transitive(install_deps)
    runtime_deps = resolve_transitive(runtime_deps)

    return install_deps, runtime_deps
```

### Runtime Integration

**Wrapper scripts** include runtime deps in PATH:
```sh
#!/bin/sh
PATH="$HOME/.tsuku/tools/nodejs-20.10.0/bin:$PATH"
exec "$HOME/.tsuku/tools/turbo-1.10.0/bin/turbo" "$@"
```

**State tracking** records both dependency types:
```json
{
  "turbo": {
    "version": "1.10.0",
    "install_dependencies": ["nodejs"],
    "runtime_dependencies": ["nodejs"]
  }
}
```

### Examples by Ecosystem

**Standard npm package (no declarations):**
```toml
[metadata]
name = "turbo"

[[steps]]
action = "npm_install"
package = "turbo"
```
Result: `install_deps=["nodejs"]`, `runtime_deps=["nodejs"]`

**Compiled npm binary (esbuild):**
```toml
[[steps]]
action = "npm_install"
package = "esbuild"
runtime_dependencies = []  # Override: compiled binary
```
Result: `install_deps=["nodejs"]`, `runtime_deps=[]`

**Go tool (standard):**
```toml
[[steps]]
action = "go_install"
module = "github.com/jesseduffield/lazygit"
```
Result: `install_deps=["go"]`, `runtime_deps=[]`

**Go interpreter (yaegi - needs Go at runtime):**
```toml
[[steps]]
action = "go_install"
module = "github.com/traefik/yaegi/cmd/yaegi"
runtime_dependencies = ["go"]  # Override: interpreter
```
Result: `install_deps=["go"]`, `runtime_deps=["go"]`

## Security Considerations

### Dependency Injection Risk

**Risk**: Malicious recipe declares dependency that shadows legitimate tool.

**Mitigation**:
- Dependencies come from same trusted registry
- No user-provided dependency sources
- Recipe validation can flag unexpected deps

### Transitive Dependency Attack

**Risk**: Deep dependency chain introduces vulnerability.

**Mitigation**:
- Max transitive depth of 10
- All deps from trusted registry
- `tsuku info` shows full tree for audit

### PATH Manipulation

**Risk**: Installed dependency manipulates PATH for subsequent steps.

**Mitigation**:
- PATH constructed by tsuku, not recipes
- Wrapper scripts use absolute paths
- Dependencies only prepend to PATH

## Implementation Plan

### Implementation Structure

This feature is delivered as a **single milestone** with multiple implementation phases. The phases are ordered for incremental development but all are required to deliver user value.

#### Phase 1: Core Resolution

Build the foundation for dependency resolution.

- `ActionDeps` struct and `ActionDependencies` registry
- Resolution algorithm with precedence rules
- Transitive resolution with cycle detection (max depth 10)
- Version constraint parsing

#### Phase 2: Override Mechanisms

Add escape hatches for edge cases.

- Step-level `runtime_dependencies` override
- Recipe-level `dependencies` version pinning
- Validation and helpful error messages

#### Phase 3: Runtime Integration

Wire dependencies into the runtime - this is where users see value.

- Wrapper script generation with runtime deps in PATH
- State tracking of install/runtime dependencies
- `tsuku info` dependency tree display
- Uninstall warns about dependents

#### Phase 4: Migration and Cleanup

Remove legacy code and validate recipes.

- Remove `EnsureNpm()`, `EnsurePipx()`, etc. from actions
- Audit recipes for edge cases needing overrides
- Update documentation

### Implementation Issues

- [#234](https://github.com/tsukumogami/tsuku/issues/234): feat(deps): create action dependency registry
- [#235](https://github.com/tsukumogami/tsuku/issues/235): feat(deps): implement dependency resolution algorithm
- [#236](https://github.com/tsukumogami/tsuku/issues/236): feat(deps): add override and extension mechanisms
- [#237](https://github.com/tsukumogami/tsuku/issues/237): feat(deps): implement transitive resolution with cycle detection
- [#238](https://github.com/tsukumogami/tsuku/issues/238): feat(deps): generate wrappers with runtime dependencies in PATH
- [#239](https://github.com/tsukumogami/tsuku/issues/239): feat(deps): track dependencies in state.json
- [#240](https://github.com/tsukumogami/tsuku/issues/240): feat(info): display dependency tree
- [#241](https://github.com/tsukumogami/tsuku/issues/241): feat(uninstall): warn about dependent tools
- [#242](https://github.com/tsukumogami/tsuku/issues/242): refactor(deps): remove legacy bootstrap functions
- [#243](https://github.com/tsukumogami/tsuku/issues/243): chore(recipes): add dependency overrides for edge cases

### Acceptance Criteria

All phases must be complete for the milestone to deliver value:

- [ ] All ecosystem actions registered with correct deps
- [ ] Override syntax works for step and recipe level
- [ ] Transitive deps resolve correctly (pipx -> python-standalone)
- [ ] Cycles detected with clear error
- [ ] Wrappers prepend runtime dep paths
- [ ] state.json records both dep types
- [ ] `tsuku info <tool>` shows dependency tree
- [ ] Removing tool warns about dependents
- [ ] No internal bootstrap functions remain
- [ ] All recipes validated with new system
- [ ] Edge cases have correct overrides

## References

- Current bootstrap code: `internal/install/bootstrap.go`
- Recipe types: `internal/recipe/types.go:23-24`
- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Debian Policy: Package Relationships](https://www.debian.org/doc/debian-policy/ch-relationships.html)
