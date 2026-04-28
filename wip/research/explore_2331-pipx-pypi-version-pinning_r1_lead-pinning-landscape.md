# Issue #2331 Research: Recipe-Level Version Pinning Landscape

**Lead:** L1 Research  
**Date:** 2026-04-28  
**Scope:** Identify all version-pinning primitives in tsuku's recipe schema and version providers

---

## Executive Summary

The tsuku recipe schema has **no recipe-level version pinning mechanism**. All version control flows exclusively through user-level command-line pins (e.g., `tsuku install foo@1.2`). This is a real gap, not a deliberate design choice—neither the `VersionSection` nor any provider constructor accepts constraints at recipe definition time.

---

## Finding 1: VersionSection Fields (Recipe-Level Control)

**File:** `internal/recipe/types.go:178–208`

The `VersionSection` struct defines these fields:

| Field | Purpose | Constrains Version? |
|-------|---------|-------------------|
| `Source` | Selects version provider (e.g., "pypi", "npm", "github_releases") | **No** — selects source, not version |
| `GitHubRepo` | Specifies GitHub repo for tag/release discovery | **No** — selects source, not version |
| `TagPrefix` | Filters tags by prefix and strips it (e.g., "ruby-" strips "ruby-3.3.0" → "3.3.0") | **Yes** — filters upstream tags, but doesn't constrain final resolved version |
| `Module` | Go module path for goproxy resolution | **No** — selects source |
| `Formula` / `Cask` / `Tap` | Homebrew metadata | **No** — selects source |
| `StableQualifiers` | Defines what counts as a "stable" release (e.g., ["release", "final"] treats 1.0.0-RELEASE as stable, not prerelease) | **Yes** — filters prerelease vs stable, but doesn't pin to a version range |
| `URL` / `VersionPath` | http_json source configuration | **No** — selects source |

**Verdict:** Fields select the *source* (provider) or filter upstream metadata. **None constrain the final resolved version to a specific version or range.**

---

## Finding 2: Recipe TOML Usage Survey

**File Scan:** All recipes in `recipes/`

Examined `recipes/a/aider.toml`, `recipes/b/black.toml`, `recipes/h/httpie.toml`, and 5 others with `pipx_install`.

**Result:** No recipe today uses version pinning in its TOML:
- No literal version strings in `[version]` section
- No min/max bounds
- No version range expressions
- Recipes rely entirely on upstream "latest" resolution

Example (aider):
```toml
[metadata]
name = "aider"

[[steps]]
action = "pipx_install"
package = "aider-chat"

[version]
# No pinning fields used; will always resolve to latest PyPI release
```

---

## Finding 3: Provider Constructors and Constraint Acceptance

**Files:** `internal/version/provider_*.go`

Examined all 12+ provider implementations (GitHub, PyPI, npm, crates.io, RubyGems, etc.).

### PyPI Provider
**File:** `internal/version/provider_pypi.go:1–65`

Constructor: `NewPyPIProvider(resolver *Resolver, packageName string)`
- Takes only package name
- **No constraint parameter** (e.g., no min/max Python version, no version range)
- `ResolveLatest()` always returns the absolute latest PyPI release
- `ResolveVersion(ctx, version string)` accepts fuzzy matching (e.g., "1.2" matches "1.2.3") but only at resolution time, never at construction

### GitHub Provider
**File:** `internal/version/provider_github.go:20–63`

Constructor: `NewGitHubProvider(resolver *Resolver, repo string, stableQualifiers []string)`
- Takes repo, optional tag prefix, optional stable qualifiers
- **No version constraint at construction time**
- `stableQualifiers` (e.g., ["release", "final"]) filters *what counts as stable* but doesn't pin to a version range
- Example: `NewGitHubProviderWithPrefix(resolver, "ruby-lang/ruby", "ruby-", nil)` filters tags starting with "ruby-" but doesn't constrain to "3.10.x" or "3.10.20"

### Pattern Across All Providers
All 12+ providers follow this identical pattern:
- Constructor accepts metadata to identify the package (name, repo, URL) and filtering hints (tag prefix, stable qualifiers)
- **No constructor accepts a version constraint, range, or boundary**
- Version resolution is always "latest matching criteria"

**Verdict:** Providers are constructed with *source identity* and *filter metadata*, not version constraints.

---

## Finding 4: Version Resolution Flow

**Files:**
- `internal/executor/executor.go:89–128` (ResolveVersion)
- `internal/version/resolve.go:10–53` (ResolveWithinBoundary)
- `internal/install/pin.go:9–106` (PinLevel, VersionMatchesPin)

### User-Level Pin Path (Only Path for Version Control)

The only mechanism to pin versions is the `reqVersion` field:

1. **User Command:** `tsuku install foo@1.2.3`
2. **Executor stores:** `reqVersion = "1.2.3"` (`internal/executor/executor.go:32`)
3. **Resolution Flow:**
   ```go
   // internal/executor/executor.go:111
   func (e *Executor) ResolveVersion(ctx context.Context, constraint string) (string, error) {
       return version.ResolveWithinBoundary(ctx, provider, constraint)
   }
   ```
4. **Boundary-Aware Resolution** (`internal/version/resolve.go:17–53`):
   - Empty constraint → `provider.ResolveLatest()`
   - "@lts" (channel) → provider-specific channel handling
   - "1.2" (major.minor pin) → find highest version matching "1.2.*"
   - "1.2.3" (exact pin) → exact version lookup
   - **All routing happens post-provider-construction, using the generic VersionResolver interface**

**Critical Path:** Pin logic in `internal/install/pin.go` (lines 52–84):
```go
// PinLevelFromRequested: "" → PinLatest, "1.2" → PinMinor, "1.2.3" → PinExact
func PinLevelFromRequested(requested string) PinLevel { ... }

// VersionMatchesPin: checks if version falls within pin boundary
func VersionMatchesPin(version, requested string) bool {
    return version == requested || strings.HasPrefix(version, requested+".")
}
```

**Verdict:** Version pinning is exclusively a **user-level CLI concern** (`tsuku install foo@<pin>`). Recipes never participate in version selection.

---

## Finding 5: Recipe-Level Control Absent in Factory

**File:** `internal/version/provider_factory.go:95–178`

The provider factory's routing logic:

```go
func (f *ProviderFactory) ProviderFromRecipe(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
    for _, strategy := range f.strategies {
        if strategy.CanHandle(r) {
            return strategy.Create(resolver, r)  // ← Creates provider with recipe metadata
        }
    }
}
```

Each strategy (PyPISourceStrategy, GitHubRepoStrategy, etc.) extracts source identity from the recipe but **never reads or passes a version constraint:**

- `PyPISourceStrategy.Create()` (lines 169–178): reads `pipx_install.package`, creates `NewPyPIProvider(resolver, pkg)` — **no pin**
- `GitHubRepoStrategy.Create()` (lines 189–194): reads `Version.GitHubRepo` and `Version.TagPrefix`, creates `NewGitHubProvider(...)` — **no pin**
- All 12+ strategies follow identical pattern: **source identity only, no constraint**

**Verdict:** Recipe parsing and provider factory routing have zero awareness of version constraints.

---

## Finding 6: Fallback to Inferred Providers

**File:** `internal/version/provider_factory.go:250–275`

When recipe has no explicit `[version]` section, the factory infers from actions:

```go
type InferredPyPIStrategy struct{}
func (s *InferredPyPIStrategy) CanHandle(r *recipe.Recipe) bool {
    // If recipe has pipx_install action, assume PyPI
    for _, step := range r.Steps {
        if step.Action == "pipx_install" && step.Params["package"] != nil {
            return true
        }
    }
    return false
}
```

Even inferred providers are constructed identically: `NewPyPIProvider(resolver, pkg)` with **no constraint**.

**Verdict:** Inferred path also lacks version pinning.

---

## Deliberate vs. Gap: Why No Recipe-Level Pinning?

Evidence that this is a **gap, not deliberate design**:

1. **User-level pins are fully implemented and mature:**
   - Pin boundary matching (`VersionMatchesPin`, `PinLevelFromRequested`)
   - Providers support fuzzy matching and channel resolution
   - Version resolver has boundary-aware logic
   - **But none of this flows through recipe definitions**

2. **Architecture supports adding constraints:**
   - Provider constructors use clean dependency injection (`NewPyPIProvider(resolver, packageName)`)
   - Would be trivial to add: `NewPyPIProvider(resolver, packageName, minPythonVersion)` or similar
   - `StableQualifiers` shows the team already uses recipe-level filtering for qualitative metadata

3. **No explicit documentation of "recipes cannot pin versions":**
   - Would expect a design doc explaining this intentional constraint
   - Instead, the schema simply has no fields for it

4. **The problem solves a real need:**
   - Issue #2331 describes pipx recipes breaking when upstream drops Python 3.10 support
   - Recipe author could preemptively pin to working versions instead of waiting for breakage

**Conclusion:** Recipe-level version pinning is an **architectural gap**, not a deliberate omission.

---

## Summary Table: Version-Pinning Mechanisms Today

| Mechanism | Location | Constraint Type | User/Recipe | Status |
|-----------|----------|-----------------|-------------|--------|
| `reqVersion` | `internal/executor/executor.go:32` | Major/Minor/Exact | **User only** | ✓ Implemented |
| `PinLevel` + `VersionMatchesPin` | `internal/install/pin.go` | Boundary matching | **User only** | ✓ Implemented |
| Provider-level constraints | `internal/version/provider_*.go` | **None** | N/A | ✗ Missing |
| `VersionSection` fields | `internal/recipe/types.go:179–208` | Source selection + filtering | **Recipe** | ✓ Partial (source only) |
| `StableQualifiers` | `internal/recipe/types.go:207` | Prerelease filtering | **Recipe** | ✓ Yes |

---

## Files Cited

- `internal/recipe/types.go:178–208` — VersionSection definition
- `internal/version/provider_pypi.go` — PyPI provider (no constraint support)
- `internal/version/provider_github.go:20–63` — GitHub provider constructor
- `internal/version/provider_factory.go:95–178` — Factory routing logic
- `internal/install/pin.go:52–84` — Pin boundary matching (user-level only)
- `internal/executor/executor.go:111–128` — ResolveVersion path
- `internal/version/resolve.go:17–53` — ResolveWithinBoundary (user-level routing)

