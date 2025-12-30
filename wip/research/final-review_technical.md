# Technical Review: System Dependency Design Documents

## Overview

This review evaluates two design documents against the current tsuku implementation:

1. **DESIGN-system-dependency-actions.md** - Action vocabulary for system dependencies
2. **DESIGN-structured-install-guide.md** - Sandbox container building

Both documents propose replacing the current polymorphic `require_system` action with typed, composable actions and enabling sandbox container building for recipes with system dependencies.

---

## 1. Implementation Feasibility

### Current Architecture Compatibility

The proposed changes are **architecturally compatible** with the existing codebase. The current structure supports:

1. **Action registration pattern** (`internal/actions/action.go:116-133`): New actions like `apt_install`, `apt_repo`, `brew_cask`, `dnf_install`, `group_add`, `service_enable` can be registered using the existing `Register()` function.

2. **Existing stub implementations** (`internal/actions/system_packages.go`): There are already stub implementations for `apt_install`, `yum_install`, and `brew_install` that print what would be installed. These provide a foundation to build upon.

3. **Preflight validation pattern** (`internal/actions/preflight.go`): Existing actions already implement `Preflight(params) *PreflightResult` for parameter validation without side effects.

### Required Code Modifications

| File | Modification Required |
|------|----------------------|
| `internal/recipe/types.go` | Add `Distro` field to `WhenClause`, update `Matches()` and `UnmarshalTOML()` |
| `internal/actions/system_packages.go` | Expand stubs to full implementations, add `Describe()` method |
| `internal/actions/require_system.go` | Deprecate or remove after migration |
| `internal/sandbox/executor.go` | Add `ExtractPackages()` integration and container building |
| `internal/sandbox/requirements.go` | Add system dependency detection in `ComputeSandboxRequirements()` |
| NEW: `internal/platform/distro.go` | Implement `/etc/os-release` parsing |

### Blocking Technical Issues

**Issue 1: No `internal/platform` package exists**

The design proposes creating `internal/platform/distro.go` for Linux distribution detection. This package does not exist and must be created.

**Recommendation:** Create the package with:
- `OSRelease` struct for parsed data
- `Detect()` function for current system
- `ParseFile(path)` for testability with fixtures

**Issue 2: `Describe()` method not in Action interface**

The current `Action` interface (`internal/actions/action.go:68-82`) does not include a `Describe()` method:

```go
type Action interface {
    Name() string
    Execute(ctx *ExecutionContext, params map[string]interface{}) error
    IsDeterministic() bool
    Dependencies() ActionDeps
}
```

**Recommendation:** Either:
- Add `Describe()` to the base `Action` interface (breaking change, all actions need update)
- Create a new `Describable` interface (preferred, follows `NetworkValidator` pattern at line 84-92)

**Issue 3: `ExecuteInSandbox()` method not in Action interface**

The design proposes `ExecuteInSandbox(ctx *SandboxContext) error` but this doesn't exist. Current sandbox execution goes through `buildSandboxScript()` which generates shell scripts.

**Recommendation:** The current approach of generating shell scripts in `buildSandboxScript()` (`internal/sandbox/executor.go:269-311`) should be extended rather than adding `ExecuteInSandbox()` to actions. The shell script approach is simpler and matches how package managers naturally work.

---

## 2. WhenClause Extension

### Current WhenClause Structure

From `internal/recipe/types.go:191-195`:

```go
type WhenClause struct {
    Platform       []string `toml:"platform,omitempty"`
    OS             []string `toml:"os,omitempty"`
    PackageManager string   `toml:"package_manager,omitempty"`
}
```

### Proposed Distro Field Integration

The design proposes adding:

```go
Distro []string `toml:"distro,omitempty"` // e.g., ["ubuntu", "debian"]
```

**Correctness Assessment:**

The matching logic proposed in the design is correct:

```go
func (w *WhenClause) matchesDistro(distroID string, idLike []string) bool {
    for _, d := range w.Distro {
        if d == distroID {
            return true
        }
        for _, like := range idLike {
            if d == like {
                return true
            }
        }
    }
    return false
}
```

This correctly implements:
1. Exact match on `ID` field from `/etc/os-release`
2. Fallback match on `ID_LIKE` chain (handles derivatives)

### Mutual Exclusivity Rules

The design correctly identifies mutual exclusivity between:
- `Distro` and `OS` (cannot specify both)
- `Distro` and `Platform` (cannot specify both)

**Current validation** (`internal/recipe/types.go:299-302`):

```go
// Validate mutual exclusivity
if len(s.When.Platform) > 0 && len(s.When.OS) > 0 {
    return fmt.Errorf("when clause cannot have both 'platform' and 'os' fields")
}
```

**Required update:** Add similar validation for `Distro`:

```go
if len(s.When.Distro) > 0 && len(s.When.OS) > 0 {
    return fmt.Errorf("when clause cannot have both 'distro' and 'os' fields")
}
if len(s.When.Distro) > 0 && len(s.When.Platform) > 0 {
    return fmt.Errorf("when clause cannot have both 'distro' and 'platform' fields")
}
```

### Matches() Update Required

Current `Matches()` method (`internal/recipe/types.go:205-233`) must be extended to handle `Distro`. The proposed detection via `/etc/os-release` is correct, but the `Matches()` function signature would need to change to accept distro information:

**Current:**
```go
func (w *WhenClause) Matches(os, arch string) bool
```

**Required:**
```go
func (w *WhenClause) Matches(os, arch, distroID string, idLike []string) bool
```

This is a **breaking change** that affects all callers of `Matches()`.

---

## 3. Action Interface

### Current Interface Pattern

From `internal/actions/action.go:68-82`:

```go
type Action interface {
    Name() string
    Execute(ctx *ExecutionContext, params map[string]interface{}) error
    IsDeterministic() bool
    Dependencies() ActionDeps
}
```

Additional optional interfaces:
- `NetworkValidator` (line 84-92): `RequiresNetwork() bool`
- `Decomposable` (`internal/actions/decomposable.go:18-23`): `Decompose(ctx *EvalContext, params) ([]Step, error)`

### Proposed Describe() Method

The design proposes:

```go
type Action interface {
    Describe() string  // Returns human-readable instructions
}
```

**Compatibility Assessment:**

Adding to base interface is problematic - all 40+ actions would need updates. Better approach:

```go
// Describable is implemented by actions that can generate documentation.
// System dependency actions (apt_install, brew_cask, etc.) implement this
// to generate human-readable installation instructions.
type Describable interface {
    Describe(params map[string]interface{}) string
}
```

Note: The method should accept `params` because the description depends on the packages being installed.

### ExecuteInSandbox() Consideration

The design proposes:

```go
type Action interface {
    ExecuteInSandbox(ctx *SandboxContext) error
}
```

**This is not recommended.** The current sandbox executor (`internal/sandbox/executor.go`) works by:

1. Building a shell script (`buildSandboxScript()`)
2. Running the script in a container via the `Runtime` interface

Adding `ExecuteInSandbox()` to actions would require:
- A new execution path through all actions
- Complex plumbing of container execution context
- Duplication of logic between `Execute()` and `ExecuteInSandbox()`

**Better approach:** Extend `buildSandboxScript()` to inject package installation commands based on extracted packages. This is what the design's `ExtractPackages()` function supports.

---

## 4. Sandbox Integration

### ExtractPackages() Design

From the design:

```go
func ExtractPackages(plan *InstallationPlan) (map[string][]string, error) {
    packages := make(map[string][]string)
    for _, step := range plan.Steps {
        switch step.Action {
        case "apt_install":
            packages["apt"] = append(packages["apt"], step.Packages...)
        case "brew_install", "brew_cask":
            packages["brew"] = append(packages["brew"], step.Packages...)
        // ...
        }
    }
    return packages, nil
}
```

**Correctness Issues:**

1. **Type mismatch:** `step.Packages` doesn't exist on `ResolvedStep`. The actual structure (`internal/executor/plan.go:93-116`) is:

```go
type ResolvedStep struct {
    Action string                 `json:"action"`
    Params map[string]interface{} `json:"params"`
    // ...
}
```

Packages would be in `step.Params["packages"]`.

**Corrected implementation:**

```go
func ExtractPackages(plan *executor.InstallationPlan) (map[string][]string, error) {
    packages := make(map[string][]string)
    for _, step := range plan.Steps {
        switch step.Action {
        case "apt_install":
            if pkgs, ok := step.Params["packages"].([]interface{}); ok {
                for _, p := range pkgs {
                    if s, ok := p.(string); ok {
                        packages["apt"] = append(packages["apt"], s)
                    }
                }
            }
        // ... similar for other package managers
        }
    }
    return packages, nil
}
```

### Container Building Integration

The current `Runtime` interface (`internal/validate/runtime.go:18-27`) only supports:

```go
type Runtime interface {
    Name() string
    IsRootless() bool
    Run(ctx context.Context, opts RunOptions) (*RunResult, error)
}
```

**Missing capability:** There's no `Build()` method for building custom container images.

**Required additions:**

```go
type Runtime interface {
    // ... existing methods ...

    // Build creates a container image from a Dockerfile.
    // Returns the image ID/name that can be used in Run().
    Build(ctx context.Context, opts BuildOptions) (string, error)

    // ImageExists checks if an image with the given name exists locally.
    ImageExists(ctx context.Context, imageName string) (bool, error)
}

type BuildOptions struct {
    Dockerfile string            // Dockerfile content
    ContextDir string            // Build context directory
    ImageName  string            // Target image name:tag
    Labels     map[string]string // Image labels
}
```

### Container Caching

The design proposes caching by package set hash:

```go
func ContainerImageName(spec *ContainerSpec) string {
    // Sort packages for deterministic hash
    // ...
    hash := sha256.Sum256([]byte(strings.Join(parts, "\n")))
    return fmt.Sprintf("tsuku/sandbox-cache:%s", hex.EncodeToString(hash[:8]))
}
```

**What's missing:**

1. **Cache location:** No specification of where cached images are stored
2. **Cache invalidation:** No mechanism to invalidate stale images
3. **Base image versioning:** When the base image changes, all cached images need rebuild
4. **CI caching:** No design for sharing cache across CI runs

**Recommendation:** Add these considerations to the design:

```go
type ContainerCache struct {
    // BaseImageDigest is SHA256 of the base image.
    // Cached images are invalidated when this changes.
    BaseImageDigest string

    // PackageSetHash identifies the package combination.
    PackageSetHash string

    // CreatedAt for LRU eviction.
    CreatedAt time.Time
}
```

---

## 5. Specific Technical Issues

### Issue 1: WhenClause.Matches() signature change

**File:** `internal/recipe/types.go:205-233`

**Problem:** Adding `distro` field requires changing `Matches()` signature, breaking all callers.

**Recommendation:** Create a `PlatformInfo` struct to encapsulate all detection:

```go
type PlatformInfo struct {
    OS       string
    Arch     string
    DistroID string   // empty on non-Linux
    IDLike   []string // empty on non-Linux
}

func (w *WhenClause) Matches(p *PlatformInfo) bool
```

### Issue 2: Missing Preflight() on new actions

**File:** Design does not specify `Preflight()` for new actions.

**Problem:** All registered actions need `Preflight()` for parameter validation.

**Recommendation:** Add explicit preflight validation specs for each action:

```go
func (a *AptRepoAction) Preflight(params map[string]interface{}) *PreflightResult {
    result := &PreflightResult{}
    if _, ok := GetString(params, "url"); !ok {
        result.AddError("apt_repo requires 'url' parameter")
    }
    if _, ok := GetString(params, "key_url"); !ok {
        result.AddError("apt_repo requires 'key_url' parameter")
    }
    if _, ok := GetString(params, "key_sha256"); !ok {
        result.AddError("apt_repo requires 'key_sha256' for content-addressing")
    }
    return result
}
```

### Issue 3: ActionEvaluability map needs update

**File:** `internal/executor/plan.go:132-163`

**Problem:** New actions need entries in `ActionEvaluability` map.

**Recommendation:** Add:

```go
var ActionEvaluability = map[string]bool{
    // ... existing entries ...

    // System dependency actions - not evaluable (require privileged ops)
    "apt_repo":       false,
    "apt_ppa":        false,
    "dnf_install":    false,
    "dnf_repo":       false,
    "brew_cask":      false,
    "pacman_install": false,
    "group_add":      false,
    "service_enable": false,
    "service_start":  false,
    "require_command": true,  // Just checks existence, deterministic
    "manual":          true,  // Just displays text, deterministic
}
```

### Issue 4: Primitives map needs update

**File:** `internal/actions/decomposable.go:76-103`

**Problem:** New actions need to be registered as primitives.

**Recommendation:** Add to primitives map:

```go
var primitives = map[string]bool{
    // ... existing entries ...

    // System dependency primitives
    "apt_install":     true,
    "apt_repo":        true,
    "apt_ppa":         true,
    "dnf_install":     true,
    "dnf_repo":        true,
    "brew_cask":       true,
    "pacman_install":  true,
    "group_add":       true,
    "service_enable":  true,
    "service_start":   true,
    "require_command": true,
    "manual":          true,
}
```

### Issue 5: Existing require_system deprecation path

**File:** `internal/actions/require_system.go`

**Problem:** The design doesn't specify how to migrate from `require_system` to new actions.

**Current behavior:**
- `require_system` checks if a command exists
- Returns `SystemDepMissingError` with `InstallGuide` if missing
- Supports version checking with `version_flag`, `version_regex`, `min_version`

**Recommendation:** The `require_command` action in the design should preserve version checking:

```go
type RequireCommandAction struct{ BaseAction }

func (a *RequireCommandAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    command, _ := GetString(params, "command")
    versionFlag, _ := GetString(params, "version_flag")
    versionRegex, _ := GetString(params, "version_regex")
    minVersion, _ := GetString(params, "min_version")

    // Reuse existing detectVersion() and versionSatisfied() from require_system.go
}
```

### Issue 6: buildSandboxScript() hardcoded packages

**File:** `internal/sandbox/executor.go:279-291`

**Problem:** Current implementation hardcodes package installation:

```go
packages := []string{}
if reqs.RequiresNetwork {
    packages = append(packages, "ca-certificates", "curl")
}
if hasBuildActions(plan) {
    packages = append(packages, "build-essential")
}
```

**Recommendation:** This should be extended to use `ExtractPackages()`:

```go
func (e *Executor) buildSandboxScript(
    plan *executor.InstallationPlan,
    reqs *SandboxRequirements,
) string {
    // Extract packages from plan
    pkgs, _ := ExtractPackages(plan)

    var sb strings.Builder
    sb.WriteString("#!/bin/bash\n")
    sb.WriteString("set -e\n\n")

    // Install apt packages if any
    if aptPkgs := pkgs["apt"]; len(aptPkgs) > 0 {
        sb.WriteString("apt-get update -qq\n")
        sb.WriteString(fmt.Sprintf("apt-get install -qq -y %s\n",
            strings.Join(aptPkgs, " ")))
    }
    // ... rest of script
}
```

---

## 6. Implementation Approach Adjustments

### Recommended Phase Ordering

The designs propose phases in a reasonable order, but some adjustments:

**Phase 1: Infrastructure (correct as-is)**
1. Create `internal/platform/distro.go`
2. Add `Distro` to `WhenClause` with validation
3. Update `Matches()` with `PlatformInfo` struct

**Phase 2: Action Vocabulary (needs adjustment)**

Before implementing all actions, start with the subset needed for current recipes:

1. `require_command` (replacement for command-checking part of `require_system`)
2. `apt_install` (expand existing stub)
3. `brew_cask` (new, commonly used)
4. `manual` (fallback action)

Hold off on `apt_repo`, `dnf_install`, etc. until there's a recipe that needs them.

**Phase 3: Documentation Generation (correct as-is)**

Add `Describable` interface and implement `Describe()` on system actions.

**Phase 4: Sandbox Integration (needs significant work)**

1. Implement `Build()` method on `Runtime` interface
2. Implement `ExtractPackages()`
3. Create minimal base container Dockerfile
4. Update `buildSandboxScript()` to use extracted packages
5. Add container caching logic

### Risk Areas

| Risk | Mitigation |
|------|------------|
| `WhenClause.Matches()` signature change affects many callers | Use `PlatformInfo` struct to minimize breaking changes |
| Container building adds complexity | Start with simple Dockerfile generation, cache later |
| Recipe migration could break existing recipes | Add `require_system` deprecation warning first, migrate incrementally |
| `Describe()` output quality | Add tests comparing output to expected instructions |

---

## 7. Summary

The design documents are **technically sound** and compatible with the current architecture. The main implementation challenges are:

1. **Missing infrastructure**: `internal/platform/distro.go` needs to be created
2. **Interface evolution**: Need `Describable` interface for documentation generation
3. **Runtime enhancement**: `Build()` method needed for container building
4. **Breaking changes**: `WhenClause.Matches()` signature needs careful migration

The proposed implementation phases are reasonable, though Phase 4 (sandbox integration) needs the most design elaboration around container caching and `Runtime` interface changes.

**Recommended next steps:**

1. Create `internal/platform/distro.go` with tests
2. Add `Distro` field to `WhenClause` with validation
3. Implement `require_command` action (extracted from `require_system`)
4. Expand `apt_install` stub with `Describe()` method
5. Create `Describable` interface
6. Update one recipe (e.g., docker.toml) as proof of concept
