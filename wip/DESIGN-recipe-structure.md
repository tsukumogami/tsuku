# Recipe Structure Evolution

## Status

Proposed

## Context and Problem Statement

Tsuku recipes follow a four-section structure: `[metadata]`, `[version]`, `[[steps]]`, and `[verify]`. The codebase already implements version inference from ecosystem actions via `ProviderFactory` strategies at priority 10, with explicit `[version]` blocks taking precedence at priority 80-100. This means recipes can already omit `[version]` when using ecosystem installers like `cargo_install`, `pipx_install`, `npm_install`, `gem_install`, `go_install`, or `github_archive`.

However, there's a gap between implementation and practice. Of 157 recipes:
- 69 use `github_archive` (correctly omit `[version]` - inference works)
- 8 use `npm_install` (correctly omit `[version]` - inference works)
- 8 use `go_install` with explicit `source = "goproxy"` (required - no inference for Go)
- 12 ecosystem recipes have redundant `[version]` blocks that could be removed:
  - 6 with `source = "pypi"` + `pipx_install`
  - 3 with `source = "crates_io"` + `cargo_install`
  - 3 with `source = "rubygems"` + `gem_install`

Issues #177 and #178 address this gap by:
- Removing the 12 redundant version blocks (issue #177)
- Adding validation to prevent future redundancy (issue #178)

This design explores whether these issues are sufficient or whether a deeper structural change is warranted while tsuku is still pre-GA.

### Scope

**In scope:**
- Evaluating whether the existing inference system is sufficient
- Defining validation rules for recipe consistency
- Clarifying recipe author guidance

**Out of scope:**
- Changing the verification system
- Modifying dependency handling
- Altering the action decomposition mechanism (works well)
- Breaking existing recipes

## Decision Drivers

- **Recipe author experience**: Recipes should be concise; redundancy creates confusion about what's required vs. optional
- **Consistency**: Similar recipes should look similar; the mix of explicit and inferred patterns should be intentional, not accidental
- **Discoverability**: Recipe authors should understand what's optional without reading source code
- **Validation clarity**: The system should provide clear errors when recipes are misconfigured
- **Backward compatibility**: Existing recipes must continue to work

## Implementation Context

### Current Inference System

The `ProviderFactory` in `internal/version/provider_factory.go` implements a priority-based strategy system:

| Priority | Strategy Type | Example |
|----------|---------------|---------|
| 100 | Known registries | `source = "pypi"` → `PyPIProvider` |
| 90 | Explicit hints | `github_repo = "..."` → `GitHubProvider` |
| 80 | Explicit source | `source = "custom"` → `CustomProvider` |
| 10 | Inferred from action | `cargo_install` → `CratesIOProvider` |

This means explicit `[version]` blocks always take precedence over inference, enabling override cases.

### Recipe Breakdown by Pattern

| Pattern | Count | Example | Notes |
|---------|-------|---------|-------|
| `github_archive` | 69 | duf.toml | Version inferred from `repo` param |
| `download_archive` | 25 | nodejs.toml | Explicit `[version]` required |
| Homebrew action | 16 | cmake.toml | Explicit `[version]` required |
| `go_install` | 8 | gofumpt.toml | Requires `[version]` (no inference) |
| `npm_install` | 8 | serve.toml | Already uses inference |
| `pipx_install` | 6 | black.toml | Could drop `[version]` (inference works) |
| `cargo_install` | 3 | cargo-audit.toml | Could drop `[version]` (inference works) |
| `gem_install` | 3 | bundler.toml | Could drop `[version]` (inference works) |
| Other | ~19 | Various | Mixed patterns |

### Edge Cases

Some recipes legitimately need explicit `[version]` despite using ecosystem actions:

**dlv.toml**: Resolves version from `github.com/go-delve/delve` but installs from `github.com/go-delve/delve/cmd/dlv`. The module path for version resolution differs from the install path:

```toml
[version]
source = "goproxy"
module = "github.com/go-delve/delve"  # Version from here

[[steps]]
action = "go_install"
module = "github.com/go-delve/delve/cmd/dlv"  # Install from here
```

This override pattern is intentional and must continue to work.

## Considered Options

### Option 0: Status Quo + Issues #177/#178

Execute issues #177 and #178 as planned:
- Remove 12 redundant `[version]` blocks
- Add validation to warn on redundancy and error on conflicts
- Improve recipe documentation

**Pros:**
- Minimal code changes
- No new concepts
- Validates the existing inference system works
- Addresses the immediate symptom

**Cons:**
- Inference rules remain implicit - recipe authors must understand the priority system
- Documentation burden to explain when `[version]` is required vs. optional
- No mechanism to make inference explicit for readers

### Option 1: Explicit Action Registration

Add an `action_registry.toml` file that documents which actions infer version sources. Recipe validation can reference this registry to provide helpful messages.

```toml
# internal/action/action_registry.toml
[cargo_install]
infers_version_from = "crates_io"
package_param = "crate"

[pipx_install]
infers_version_from = "pypi"
package_param = "package"

[github_archive]
infers_version_from = "github_releases"
package_param = "repo"
```

**Pros:**
- Makes inference rules discoverable outside Go code
- Enables better error messages ("cargo_install infers version from crates.io, remove [version] block or add override reason")
- Could generate documentation automatically
- Single source of truth for validation rules

**Cons:**
- Introduces another configuration file to maintain
- Duplication between registry and Go code (must stay in sync)
- Over-engineering if issues #177/#178 are sufficient

### Option 2: Recipe-Level Intent Declaration

Add an optional `intent` field to `[metadata]` that explicitly declares what pattern the recipe follows:

```toml
[metadata]
name = "cargo-audit"
intent = "ecosystem"  # vs. "composite" or "custom"

[[steps]]
action = "cargo_install"
crate = "cargo-audit"
```

**Pros:**
- Makes recipe author's intent explicit
- Enables intent-specific validation rules
- Helps contributors understand recipe structure at a glance
- Future-proof for new patterns

**Cons:**
- Adds a new concept (intent types) that must be documented
- Migration burden for 157 existing recipes
- Risk of intent/structure mismatch if not validated

### Option 3: Formalize Override Syntax

Keep inference as default but require an explicit comment or field when overriding:

```toml
[version]
source = "github_releases"
github_repo = "go-delve/delve"
override_reason = "Version from main repo, install from cmd subdirectory"

[[steps]]
action = "go_install"
module = "github.com/go-delve/delve/cmd/dlv"
```

**Pros:**
- Makes intentional overrides explicit
- Self-documenting recipes
- Validation can require reason for overrides
- Helps future maintainers understand non-obvious structures

**Cons:**
- Adds friction to legitimate override cases
- "override_reason" is unusual in configuration files
- May feel bureaucratic for advanced users

### Evaluation Against Decision Drivers

| Option | Author Experience | Consistency | Discoverability | Validation | Backward Compat |
|--------|-------------------|-------------|-----------------|------------|-----------------|
| 0. Status Quo | Fair | Good | Poor | Good | Good |
| 1. Action Registry | Good | Good | Good | Good | Good |
| 2. Intent Declaration | Fair | Good | Good | Good | Poor |
| 3. Override Syntax | Fair | Good | Good | Good | Good |

### Uncertainties

- **Is the existing inference sufficient?** Issues #177/#178 will test this. If confusion persists afterward, Option 1 becomes more attractive.
- **How many legitimate overrides exist?** Currently only `dlv.toml` is known. If more emerge, Option 3's overhead may be justified.
- **Will contributors understand inference?** Unclear without user research. The `github_archive` pattern (69 recipes) suggests inference is accepted for GitHub but may be surprising for ecosystems.

## Decision Outcome

**Chosen: Option 0 (Status Quo) with enhanced validation**

The existing inference system already works correctly. The immediate problem is redundant `[version]` blocks in 12 recipes, not a fundamental structural issue. Issues #177 and #178 address this directly.

### Rationale

This option was chosen because:
- **Backward compatibility**: The inference system already exists and works; no structural changes needed
- **Minimal complexity**: No new concepts, files, or migration paths
- **Validation clarity**: Enhanced validation (#178) will provide clear feedback when recipes are misconfigured
- **Evidence-based**: 77 recipes (69 `github_archive` + 8 `npm_install`) already use inference successfully

Alternatives were rejected because:
- **Option 1 (Action Registry)**: Over-engineering. The inference rules are already documented in Go code; a TOML registry would duplicate this without clear benefit. If discoverability becomes a problem after #177/#178, we can revisit.
- **Option 2 (Intent Declaration)**: Poor backward compatibility. Requiring 157 recipes to add `intent` field creates migration burden without proportional benefit.
- **Option 3 (Override Syntax)**: Adds friction to legitimate edge cases. The `dlv.toml` pattern is rare and already works; requiring `override_reason` would be bureaucratic.

### Trade-offs Accepted

By choosing this option, we accept:
- Inference rules remain in Go code rather than a discoverable configuration file
- Recipe authors must understand priority system for edge cases (version override)
- No explicit "intent" declaration in recipes

These are acceptable because:
- Most recipes (87%) either use `github_archive` (where inference is obvious) or `download_archive` (where explicit `[version]` is required)
- The remaining 13% are ecosystem installers where inference is straightforward
- Edge cases like `dlv.toml` are rare and the existing pattern works

## Solution Architecture

### Overview

The solution leverages the existing `ProviderFactory` inference system and adds validation to prevent redundant or conflicting configurations. No new components are introduced.

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                      Recipe (TOML)                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ [metadata]  │  │ [version]?  │  │ [[steps]]           │  │
│  │ name = ...  │  │ source = ?  │  │ action = "..."      │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                 ProviderFactory (existing)                  │
│  ┌──────────────────┐  ┌──────────────────┐                 │
│  │ Priority 100     │  │ Priority 10      │                 │
│  │ Explicit source  │  │ Inferred from    │                 │
│  │ strategies       │  │ action strategies│                 │
│  └──────────────────┘  └──────────────────┘                 │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                  Validation (enhanced)                      │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Check: Is explicit [version] redundant with action?  │   │
│  │ Check: Does action imply a version source?           │   │
│  │ Emit: Warning (--strict) or Error (conflict)         │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Key Interfaces

**Validation function** (to be added in #178):

```go
// ValidateVersionRedundancy checks if a recipe has redundant [version] configuration
func ValidateVersionRedundancy(r *recipe.Recipe) []ValidationResult {
    results := []ValidationResult{}

    // If no explicit version, nothing to check
    if r.Version.Source == "" && r.Version.GitHubRepo == "" {
        return results
    }

    // Check each step for version-inferring actions
    for _, step := range r.Steps {
        inferred := inferVersionSourceFromAction(step)
        if inferred == "" {
            continue
        }

        explicit := r.Version.Source
        if r.Version.GitHubRepo != "" {
            explicit = "github_releases"
        }

        if inferred == explicit {
            // Redundant: explicit matches what would be inferred
            results = append(results, ValidationResult{
                Level:   Warning,
                Message: fmt.Sprintf("[version] source=%q is redundant; %s infers this automatically",
                    explicit, step.Action),
                Fix:     "Remove the [version] section",
            })
        } else if explicit != "" && isConflict(explicit, inferred) {
            // Conflict: explicit contradicts inference (e.g., source="pypi" with cargo_install)
            results = append(results, ValidationResult{
                Level:   Error,
                Message: fmt.Sprintf("[version] source=%q conflicts with action %s (expects %s)",
                    explicit, step.Action, inferred),
            })
        }
    }

    return results
}
```

**Action to version source mapping**:

| Action | Inferred Source | Package Param | Notes |
|--------|-----------------|---------------|-------|
| `cargo_install` | `crates_io` | `crate` | Inference available |
| `pipx_install` | `pypi` | `package` | Inference available |
| `npm_install` | `npm` | `package` | Inference available |
| `gem_install` | `rubygems` | `gem` | Inference available |
| `go_install` | N/A | `module` | **No inference** - requires explicit `source="goproxy"` |
| `cpan_install` | `metacpan` | `distribution` | Inference available |
| `github_archive` | `github_releases` | `repo` | Inference available |
| `github_file` | `github_releases` | `repo` | Inference available |

**Note**: `go_install` has no inference strategy because Go module install paths often differ from version resolution paths (e.g., `github.com/go-delve/delve/cmd/dlv` vs `github.com/go-delve/delve`). The `[version] module = "..."` field allows specifying a different module for version resolution.

**Implementation note**: Extend the existing `VersionValidator` interface rather than adding a standalone function. This keeps all version validation logic in `internal/version/` and uses the existing dependency injection pattern.

### Data Flow

1. **Recipe parsing**: TOML parsed into `Recipe` struct (unchanged)
2. **Validation** (new in #178):
   - `tsuku validate` runs redundancy checks
   - Warnings emitted for redundant configurations
   - Errors for conflicting configurations
   - `--strict` mode treats warnings as errors
3. **Version resolution** (unchanged):
   - `ProviderFactory.ProviderFromRecipe()` selects strategy by priority
   - Explicit `[version]` wins over inference
4. **Installation** (unchanged):
   - Actions receive version via `EvalContext`

## Implementation Approach

### Phase 1: Clean Up Redundant Recipes (Issue #177)

Remove redundant `[version]` blocks from 12 ecosystem recipes:
- 6 `pipx_install` recipes with `source = "pypi"`
- 3 `cargo_install` recipes with `source = "crates_io"`
- 3 `gem_install` recipes with `source = "rubygems"`

**Note**: The 8 `go_install` recipes with `source = "goproxy"` are NOT redundant - `go_install` has no inference strategy and requires explicit version configuration.

**Verification**: All recipes continue to work after removal (inference provides same version source).

### Phase 2: Add Validation (Issue #178)

Implement `ValidateVersionRedundancy` function:
- Add to `internal/recipe/validate.go`
- Integrate with `tsuku validate` command
- Add `--strict` flag support
- Clear error messages with fix suggestions

**CI Integration**: Run `tsuku validate --strict` on all recipes in CI.

### Phase 3: Documentation

Update recipe authoring documentation:
- Explain when `[version]` is required vs. optional
- Document the priority system
- Provide examples of override patterns (like `dlv.toml`)
- Add troubleshooting section for validation errors

## Consequences

### Positive

- **Simpler recipes**: 12 recipes become more concise
- **Clear validation**: Recipe authors get immediate feedback on misconfiguration
- **No migration required**: Existing recipes with explicit `[version]` continue to work
- **Leverage existing code**: No new inference logic needed; `ProviderFactory` already handles this

### Negative

- **Implicit behavior**: Version inference remains in Go code, not a discoverable config
- **Learning curve**: New contributors must understand priority system for edge cases
- **Validation complexity**: Must correctly distinguish redundancy (warning) from override (valid) from conflict (error)

### Mitigations

- **Documentation**: Clear examples in recipe authoring guide
- **Error messages**: Validation messages explain what's happening and how to fix
- **Gradual rollout**: Warning mode before strict enforcement in CI

## Security Considerations

### Download Verification

**Not applicable** - This design does not change how artifacts are downloaded or verified. The existing checksum verification in `DownloadAction.Decompose()` continues to work unchanged. Version inference affects only *which* version is resolved, not *how* the download is verified.

### Execution Isolation

**Not applicable** - This design does not change execution permissions or isolation. The validation feature runs during `tsuku validate` (a read-only operation) and does not execute installed tools or modify file permissions.

### Supply Chain Risks

**Unchanged risk profile** - The design maintains the existing trust model:

| Source | Trust Model | This Design's Impact |
|--------|-------------|---------------------|
| GitHub Releases | Trust repo owner, verify checksums | No change |
| PyPI | Trust package owner, verify checksums | No change |
| crates.io | Trust crate owner, verify Cargo.lock | No change |
| npm registry | Trust package owner, verify package-lock | No change |
| RubyGems | Trust gem owner, verify lockfile | No change |

The inference system chooses the *same* version source that would be used with explicit configuration. Recipe authors can still override to use a different source (e.g., GitHub releases instead of crates.io) when needed.

**Potential concern**: A recipe author might accidentally use inference when they intended a specific version source. Mitigation: The validation system (#178) will warn on redundant configurations and error on conflicts, making intent explicit.

### User Data Exposure

**Not applicable** - This design does not access or transmit user data. The validation feature reads recipe files (already in the codebase) and produces diagnostic output. No network calls are made during validation; version resolution happens during `tsuku install`, not `tsuku validate`.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Accidental version source mismatch | Validation warns/errors on conflicts | Recipe author could still intend a non-default source without realizing inference exists |
| Contributor confusion about what's validated | Documentation explains inference priority | New contributors may not read documentation |
| Multi-ecosystem tool version confusion | Documentation when explicit config is recommended | Tools available on multiple registries may have inconsistent versions |

### Future Improvements (Non-Blocking)

Based on security review, consider for future iterations:
- **`--explain` flag**: Show which version source was selected and why during `tsuku install`
- **Debug logging**: Log when inference selects a version source
- **Documentation**: Explicitly recommend explicit `[version]` for tools available on multiple registries

