# Design: go_install Version Inference

**Status**: Accepted

## Context and Problem Statement

PR #688 implemented version inference for ecosystem installers, allowing recipes to omit explicit `[version]` blocks when the version source can be inferred from the action. This works for:

- `cargo_install` → `crates_io`
- `pipx_install` → `pypi`
- `npm_install` → `npm`
- `gem_install` → `rubygems`
- `cpan_install` → `metacpan`
- `github_archive` / `github_file` → `github_releases`

However, `go_install` was excluded because Go's module system has a fundamental distinction between **install paths** and **module paths** that the other ecosystems don't have.

### The Problem

For Go tools, the install path (what you `go install`) often differs from the module path (what has version tags):

| Recipe | Install Path | Module Path | Paths Match? |
|--------|-------------|-------------|--------------|
| gofumpt | `mvdan.cc/gofumpt` | `mvdan.cc/gofumpt` | Yes |
| gopls | `golang.org/x/tools/gopls` | `golang.org/x/tools/gopls` | Yes (submodule) |
| cobra-cli | `github.com/spf13/cobra-cli` | `github.com/spf13/cobra-cli` | Yes |
| dlv | `github.com/go-delve/delve/cmd/dlv` | `github.com/go-delve/delve` | No |
| goimports | `golang.org/x/tools/cmd/goimports` | `golang.org/x/tools` | No |
| staticcheck | `honnef.co/go/tools/cmd/staticcheck` | `honnef.co/go/tools` | No |
| gore | `github.com/x-motemen/gore/cmd/gore` | `github.com/x-motemen/gore` | No |

Of the 8 go_install recipes in tsuku:
- **4 have matching paths** (simple case, inference works directly)
- **4 have differing paths** (need explicit `[version] module = "..."`)

When version inference tries to use the install path for version resolution on differing-path recipes, installation fails because `go mod download <install-path>@<version>` expects a module at that path.

### Scope

**In scope:**
- Inferring version source from `go_install` action
- Reducing redundant `source = "goproxy"` in Go tool recipes
- Leveraging existing `Recipe.Version.Module` for complex cases

**Out of scope:**
- Changes to how version resolution queries proxy.golang.org
- Changes to `go_build` primitive action
- Support for replace directives or private modules
- New recipe schema fields (architecture review found existing infrastructure sufficient)

## Decision Drivers

- **Consistency**: `go_install` should work like other ecosystem actions
- **Minimal recipe authoring**: Recipe authors shouldn't need to specify redundant information
- **Correct installation**: The install path must be used for `go install`, module path for versioning
- **No breaking changes**: Existing recipes with explicit `[version]` must continue to work
- **Maintainability**: Solution should be understandable and debuggable
- **Leverage existing infrastructure**: Use `Recipe.Version.Module` instead of new schema fields

## Implementation Context

### Existing Patterns

**Version inference for other ecosystems** (`internal/version/redundancy.go`):
- Maps action names to inferred version sources
- Checks if explicit `[version]` duplicates inference
- Used by `tsuku validate --strict` to warn about redundancy

**GoInstallAction.Decompose()** (`internal/actions/go_install.go:374-377`):
- Already handles module path distinction correctly
- Uses `ctx.Recipe.Version.Module` for `go get` (versioning)
- Uses action's `module` param for `go install` (installation)

**Existing recipe pattern** (dlv.toml):
```toml
[version]
source = "goproxy"
module = "github.com/go-delve/delve"  # Module path for versioning

[[steps]]
action = "go_install"
module = "github.com/go-delve/delve/cmd/dlv"  # Install path
```

The `source = "goproxy"` is redundant and can be inferred from `go_install`. The `module = "..."` provides essential non-redundant information when paths differ.

### Key Insight from Architecture Review

The architecture review identified that the originally proposed `version_module` step param duplicates existing `Recipe.Version.Module` functionality. The simpler approach is:

1. Add `go_install` to inference (enables removing `source = "goproxy"`)
2. Use existing `Recipe.Version.Module` for differing-path cases
3. No schema changes needed

## Considered Options

### Option 1: Extend VersionInfo with ResolvedModule

Add a `ResolvedModule` field to `VersionInfo` for Go-specific module path tracking.

**Pros:** Single source of truth for resolved module
**Cons:** Pollutes generic struct with Go-specific fields, affects all providers

### Option 2: Automatic Module Discovery in Decompose

Have `Decompose()` probe proxy.golang.org at decomposition time.

**Pros:** No schema changes
**Cons:** Additional network requests, potential divergence between resolution and installation

### Option 3: Leverage Existing Recipe.Version.Module (Chosen)

Use existing infrastructure: add inference mapping, rely on `Recipe.Version.Module` for complex cases.

**Pros:** No schema changes, leverages tested code, simpler implementation
**Cons:** Complex recipes still need `[version] module = "..."` (but this is non-redundant)

### Option 4: Probe and Cache Module Path

Cache module path discovery results during inference.

**Pros:** Reduces network overhead after first probe
**Cons:** Cache management complexity, implicit state

## Decision Outcome

**Chosen option: Option 3 (Leverage Existing Recipe.Version.Module)**

This approach requires the least code change while achieving the goal: eliminating redundant `source = "goproxy"` from recipes.

### Rationale

- **Leverages existing infrastructure**: `Recipe.Version.Module` already handles the module path distinction in `Decompose()`
- **No schema changes**: No new `version_module` param needed
- **Simpler implementation**: Just add inference mapping and strategy
- **Clear semantics**: `[version] module = "..."` is non-redundant when it differs from install path

### What Changes for Each Recipe Category

**Simple cases (matching paths):** Remove `[version]` entirely
- gofumpt, gopls, cobra-cli

**Complex cases (differing paths):** Remove only `source = "goproxy"`, keep `module = "..."`
- dlv, goimports, staticcheck, gore

Before:
```toml
[version]
source = "goproxy"
module = "github.com/go-delve/delve"
```

After:
```toml
[version]
module = "github.com/go-delve/delve"
```

## Solution Architecture

### Overview

Add `go_install` to the version inference system. When a recipe uses `go_install`:
1. If no `[version]` block: infer `goproxy` using step's `module` param
2. If `[version]` has only `module = "..."`: infer `goproxy` using that module path
3. Existing `Decompose()` logic handles the rest

### Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         Recipe TOML                              │
│  [version]                                                       │
│  module = "github.com/go-delve/delve"  ← optional, for complex  │
│                                                                  │
│  [[steps]]                                                       │
│  action = "go_install"                                           │
│  module = "github.com/go-delve/delve/cmd/dlv"  ← install path   │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                 InferredGoProxyStrategy                          │
│  CanHandle: recipe has go_install action                         │
│  Create: use Recipe.Version.Module if set, else step's module   │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                    GoProxyProvider                               │
│  Resolves version from proxy.golang.org using module path       │
│  Returns VersionInfo{Tag: "v1.26.0", Version: "1.26.0"}         │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                  GoInstallAction.Decompose()                     │
│  Uses Recipe.Version.Module for go get (already implemented)    │
│  Uses step's module for go install (already implemented)        │
└─────────────────────────────────────────────────────────────────┘
```

### Key Interfaces

**Version inference mapping:**
```go
// internal/version/redundancy.go
var actionInference = map[string]string{
    // ... existing mappings ...
    "go_install": "goproxy",  // NEW
}
```

**InferredGoProxyStrategy (provider_factory.go):**
```go
type InferredGoProxyStrategy struct{}

func (s *InferredGoProxyStrategy) Priority() int { return PriorityInferred }

func (s *InferredGoProxyStrategy) CanHandle(r *recipe.Recipe) bool {
    for _, step := range r.Steps {
        if step.Action == "go_install" {
            if _, ok := step.Params["module"].(string); ok {
                return true
            }
        }
    }
    return false
}

func (s *InferredGoProxyStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
    // Use Recipe.Version.Module if explicitly set (for differing paths)
    if r.Version.Module != "" {
        return NewGoProxyProvider(resolver, r.Version.Module), nil
    }
    // Otherwise use module from go_install step (simple case)
    for _, step := range r.Steps {
        if step.Action == "go_install" {
            if module, ok := step.Params["module"].(string); ok {
                return NewGoProxyProvider(resolver, module), nil
            }
        }
    }
    return nil, fmt.Errorf("no Go module found in go_install steps")
}
```

## Implementation Approach

### Phase 1: Add go_install to Inference Mapping

Add `go_install` to the `actionInference` map in `redundancy.go`.

**Files changed:**
- `internal/version/redundancy.go`: Add mapping
- `internal/version/redundancy_test.go`: Add test case

**Dependencies:** None

### Phase 2: Add InferredGoProxyStrategy

Implement the strategy that enables version inference for go_install recipes.

**Files changed:**
- `internal/version/provider_factory.go`: Add InferredGoProxyStrategy
- `internal/version/provider_factory_test.go`: Add test cases

**Dependencies:** Phase 1

### Phase 3: Update Recipes

Migrate existing go_install recipes:
- Simple cases: Remove `[version]` entirely
- Complex cases: Remove `source = "goproxy"`, keep `module = "..."`

**Files changed:**
- `internal/recipe/recipes/g/gofumpt.toml`: Remove [version]
- `internal/recipe/recipes/g/gopls.toml`: Remove [version]
- `internal/recipe/recipes/c/cobra-cli.toml`: Remove [version]
- `internal/recipe/recipes/d/dlv.toml`: Remove source, keep module
- `internal/recipe/recipes/g/goimports.toml`: Remove source, keep module
- `internal/recipe/recipes/s/staticcheck.toml`: Remove source, keep module
- `internal/recipe/recipes/g/gore.toml`: Remove source, keep module

**Dependencies:** Phase 2

### Phase 4: Update Documentation

Update CONTRIBUTING.md to explain the go_install inference pattern.

**Files changed:**
- `CONTRIBUTING.md`: Document inference for go_install

**Dependencies:** Phase 3

## Consequences

### Positive

- **Reduced redundancy**: `source = "goproxy"` eliminated from all go_install recipes
- **Simple cases fully inferred**: gofumpt, gopls, cobra-cli need no `[version]` block
- **Consistency**: go_install now participates in version inference
- **Leverages existing code**: Decompose() already handles Recipe.Version.Module correctly
- **No schema changes**: Simpler to implement and maintain

### Negative

- **Complex recipes still need `[version] module`**: Not fully automatic for differing-path cases
- **Two patterns**: Authors need to understand when `module = "..."` is needed

### Mitigations

- **Documentation**: Clear guidance in CONTRIBUTING.md
- **Validation**: `tsuku validate --strict` can detect if inference would fail

## Security Considerations

### Download Verification

**No new risks introduced.** This design does not change how Go modules are downloaded or verified. The existing security model applies:

- **Checksum verification**: Go's module proxy enforces checksums via `go.sum`. The `Decompose()` method captures `go.sum` content at eval time, and `go_build` verifies checksums at install time via `go mod verify`.
- **GOSUMDB**: All module downloads are verified against `sum.golang.org` (Go's checksum database).
- **Failure handling**: If checksum verification fails, `go mod verify` returns an error and installation aborts.

### Execution Isolation

**No changes to execution model.** This design:

- Does not add new file system access patterns
- Does not change network access scope (still queries proxy.golang.org)
- Does not require elevated privileges
- Does not modify the sandbox or isolation mechanisms

### Supply Chain Risks

**No additional risk beyond existing `[version] module` field.** The `Recipe.Version.Module` field already exists and has the same trust implications. Recipe review remains the primary defense.

**Trust model**: Unchanged. Trust flows from:
1. Recipe author → tsuku registry
2. Module author → proxy.golang.org
3. Go team → go.sum / sum.golang.org

### User Data Exposure

**No user data exposure.** The only data transmitted is the module path sent to proxy.golang.org, which is already part of the existing design.

### Residual Risk Assessment

**No additional risk.** This design:
- Reuses existing security infrastructure
- Adds no new network endpoints or data flows
- No schema changes that could introduce confusion
- Maintains backward compatibility
