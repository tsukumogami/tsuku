# Action System Research

## 1. Action Definition and Registration

All actions implement the minimal `Action` interface: `Name()` and `Execute()`. There are 49 total actions registered in a central registry map. Registration happens in `init()` via `Register()` function.

Actions split into categories:
- Core primitives (11): download, extract, chmod, install_binaries, etc.
- Package managers (9): apt_install, brew_install, etc.
- Ecosystem primitives (4): cargo_build, go_build, npm_exec, cpan_install
- Composites (6): github_archive, homebrew_bottle, homebrew_source, etc.

## 2. Interfaces Implemented by Actions

- **Core**: All implement `Action` interface with `Name()` and `Execute()`
- **Optional**: Composite actions implement `Decomposable` interface with `Decompose()` method
- Primitives do NOT implement Decomposable

## 3. Metadata Beyond Name() and Execute()

No direct metadata methods! Instead, actions use **central registry maps**:
- `primitives` map - which are primitives
- `deterministicActions` map - determinism classification
- `ActionDependencies` map - runtime/install dependencies
- `ActionEvaluability` map (in executor) - reproducibility classification

## 4. IsDeterministic() Function

Static per-action classification in `decomposable.go`. Two tiers of primitives:

**Tier 1 (Core) - Deterministic:**
- download, extract, chmod, install_binaries, set_env, set_rpath
- link_dependencies, install_libraries, apply_patch_file, text_replace

**Tier 2 (Ecosystem) - Non-deterministic:**
- cargo_build, cmake_build, configure_make, cpan_install
- gem_exec, go_build, nix_realize, npm_exec, pip_install

Reason: compiler versions, native extensions, platform variation.

## 5. IsActionEvaluable() Function

Located in `executor/plan.go`:
- **Evaluable** = can be deterministically reproduced (URLs/checksums captured)
- **Deterministic** = produces identical results (different concept)
- Most package installs (npm_install, cargo_install, cpan_install) are NOT evaluable
- Only primitives and npm_exec are evaluable

## 6. Step Struct (decomposable.go)

```go
type Step struct {
    Action   string                 // Action name
    Params   map[string]interface{} // Fully resolved params
    Checksum string                 // SHA256 for downloads
    Size     int64                  // File size
}
```

Used during decomposition (evaluation phase), returned by `Decomposable.Decompose()`.

## 7. ResolvedStep Struct (executor/plan.go)

```go
type ResolvedStep struct {
    Action        string
    Params        map[string]interface{}
    Evaluable     bool  // From IsActionEvaluable
    Deterministic bool  // From IsDeterministic
    URL           string
    Checksum      string
    Size          int64
}
```

## 8. ActionDependencies Pattern

The system uses lookup-based metadata from static maps:

```go
type ActionDeps struct {
    InstallTime []string
    Runtime     []string
}

var ActionDependencies = map[string]ActionDeps{
    "npm_install":  {InstallTime: []string{"nodejs"}, Runtime: []string{"nodejs"}},
    "cpan_install": {InstallTime: []string{"perl"}, Runtime: []string{"perl"}},
    "go_build":     {InstallTime: []string{"go"}, Runtime: nil},
}
```

## 9. Design Pattern for Network/Build Requirements

Following the existing pattern, new metadata could be added as:

```go
var ActionNetworkRequirements = map[string]bool{
    "cpan_install": true,    // needs network for CPAN modules
    "go_build": true,        // needs network for module download
    "cargo_build": true,     // needs network for crates
    "npm_exec": false,       // modules already installed
    "download": false,       // files pre-cached
}

var ActionBuildToolRequirements = map[string][]string{
    "configure_make": []string{"autoconf", "automake", "libtool", "pkg-config"},
    "cmake_build": []string{"cmake", "ninja-build"},
    "cpan_install": []string{"perl", "cpanminus"},
}
```

## 10. Key Files

- Core: `action.go`, `decomposable.go`, `dependencies.go`
- Executor: `executor/plan.go`, `executor/plan_generator.go`
- Examples: `go_build.go`, `cpan_install.go` (Tier 2 ecosystem primitives)

## Summary

The action system uses static registry maps for metadata rather than instance methods. This is elegant and extensible - any new metadata dimension (network requirements, build tools) can be added as a new map following the `ActionDependencies` pattern.
