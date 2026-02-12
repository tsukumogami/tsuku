---
status: Proposed
problem: |
  Two overlapping implementations extract system packages from recipes and plans:
  executor/system_deps.go (5 actions, packages only) and sandbox/packages.go
  (11 actions, packages and repositories). The original DESIGN-recipe-driven-ci-testing
  intended shared code between tsuku info and sandbox, but implementations diverged.
  The info --deps-only --system command now misses repository configurations that
  recipes can declare, causing install-recipe-deps.sh to fail when recipes need
  third-party repositories.
decision: |
  Consolidate extraction into a single SystemRequirements type in internal/executor
  that handles all system dependency actions (packages and repositories). Both
  info --deps-only and sandbox will use this unified extraction. The info command
  gains --repos flag for JSON output of repository configurations, and the helper
  script is extended to set up repositories before installing packages.
rationale: |
  Placing the consolidated code in internal/executor follows the original design
  intent and keeps package-level dependencies clean (sandbox can import executor,
  but not vice versa). The SystemRequirements struct from sandbox already has the
  right shape. Backward compatibility is preserved: the default text output of
  --deps-only --system remains package names only, while --repos adds repository
  output in JSON format.
---

# Consolidate System Dependency Extraction

## Status

**Proposed**

## Upstream Design Reference

This design supersedes the "Code Reuse with Sandbox" section of [DESIGN-recipe-driven-ci-testing.md](current/DESIGN-recipe-driven-ci-testing.md), which established the original intent for shared extraction code between `tsuku info` and sandbox mode.

## Context and Problem Statement

The recipe-driven CI testing design (PR #1572) created `internal/executor/system_deps.go` as the shared library for extracting system packages from recipes. The design explicitly stated:

> **Decision 3: Code Reuse with Sandbox**
>
> Create `internal/executor/system_deps.go` with extraction functions that both consumers use.

But the implementations diverged. Sandbox mode evolved independently with `internal/sandbox/packages.go`, which handles more action types and extracts repository configurations that the executor version ignores.

**Current state:**

| Component | Location | Input | Actions | Output |
|-----------|----------|-------|---------|--------|
| info --deps-only | executor/system_deps.go | Recipe | 5 (packages only) | []string |
| sandbox | sandbox/packages.go | Plan | 11 (packages + repos) | SystemRequirements |

The executor version handles:
- apt_install, apk_install, dnf_install, pacman_install, zypper_install

The sandbox version adds:
- apt_repo, apt_ppa, dnf_repo (repository setup)
- brew_install, brew_cask, brew_tap (Homebrew)

**The gap:** When a recipe declares `apt_repo` to add a third-party repository before installing packages from it, `tsuku info --deps-only --system` only returns the packages. The `install-recipe-deps.sh` helper script can't set up the repository first, so package installation fails.

**Note:** No production recipes currently use repository actions (apt_repo, apt_ppa, dnf_repo). This design prepares for that capability and fulfills the original DESIGN-recipe-driven-ci-testing intent for shared extraction code. The functional gap is 3 Linux-relevant actions (apt_repo, apt_ppa, dnf_repo). The sandbox also handles 3 macOS-only Homebrew actions (brew_install, brew_cask, brew_tap) which aren't relevant to the install-recipe-deps.sh use case.

**Example scenario:**

```toml
# Recipe that needs a custom repository
[[steps]]
action = "apt_repo"
url = "https://custom.repo/debian stable main"
key_url = "https://custom.repo/key.gpg"
key_sha256 = "abc123..."
when = { linux_family = ["debian"] }

[[steps]]
action = "apt_install"
packages = ["custom-package"]
when = { linux_family = ["debian"] }
```

Running `install-recipe-deps.sh debian custom-tool` only installs `custom-package` without first adding the repository. The install fails because the package isn't in the default apt sources.

### Scope

**In scope:**
- Consolidate extraction logic into a single implementation
- Add repository output to `tsuku info --deps-only --system`
- Extend `install-recipe-deps.sh` to handle repository setup
- Maintain backward compatibility with existing consumers

**Out of scope:**
- Changing the sandbox execution model
- Adding new system dependency action types
- Changes to recipe format

## Decision Drivers

- **Original design intent**: DESIGN-recipe-driven-ci-testing specified shared extraction code
- **Complete coverage**: CI testing should handle all system dependencies recipes can declare
- **Package dependencies**: sandbox imports executor, not vice versa
- **Backward compatibility**: Existing `--deps-only --system` consumers expect package lists
- **Helper script simplicity**: install-recipe-deps.sh should remain a shell script

## Considered Options

### Decision 1: Where to Place Consolidated Code

The extraction code needs a home. Three options were considered.

#### Chosen: Move SystemRequirements to internal/executor

Relocate the `SystemRequirements` type and extraction logic from `sandbox/packages.go` to `executor/system_deps.go`. The sandbox package imports the consolidated type.

This follows the original design intent. The executor package already has `FilterStepsByTarget()` and the partial `SystemPackageActions` map. Moving the comprehensive logic there creates a single source of truth.

**Package dependency flow:**
```
cmd/tsuku/info.go
       │
       ▼
internal/executor/system_deps.go  ◄── Single extraction implementation
       │
       ▼
internal/sandbox/packages.go  ◄── Thin wrapper (if needed)
       │
       ▼
internal/sandbox/executor.go
```

#### Alternatives Considered

**Keep sandbox/packages.go as authoritative, add wrapper in executor:**
This would require executor to import sandbox, inverting the expected package dependency. Rejected because it creates a circular dependency risk and violates the natural layering where sandbox builds on executor.

**Minimal patch: extend executor/system_deps.go only:**
Add the 3 missing Linux actions (apt_repo, apt_ppa, dnf_repo) to the existing `SystemPackageActions` map and add a separate `ExtractSystemRepositories` function. This keeps the existing separation between executor and sandbox.

Rejected because it perpetuates code duplication. The sandbox has a more complete implementation with `SystemRequirements` struct that captures both packages and repositories. Maintaining two separate implementations leads to continued divergence risk and makes the codebase harder to understand.

**Create new internal/sysdeps package:**
A neutral package that both import. Rejected because it fragments the executor package's responsibility for step handling. The executor package already has `FilterStepsByTarget()` and `SystemPackageActions`, making it the natural home for the consolidated code.

### Decision 2: How to Handle Recipe vs Plan Input

The info command works with `Recipe` objects (needs platform filtering). The sandbox works with `InstallationPlan` objects (already filtered during plan generation).

#### Chosen: Dual entry points with shared core

Provide two entry points that share extraction logic:

```go
// Entry point for Recipe (info command)
// Filters steps by target, then extracts
func ExtractSystemRequirementsFromRecipe(r *recipe.Recipe, target platform.Target) *SystemRequirements

// Entry point for Plan (sandbox)
// Plan is pre-filtered, extract directly
func ExtractSystemRequirementsFromPlan(plan *InstallationPlan) *SystemRequirements

// Shared core - works on filtered steps
func extractFromSteps(steps []Step) *SystemRequirements
```

The info command calls `ExtractSystemRequirementsFromRecipe`, which filters steps then delegates to the shared core. The sandbox calls `ExtractSystemRequirementsFromPlan`, which passes plan steps directly to the shared core.

#### Alternatives Considered

**Convert Recipe to Plan before extraction:**
Generate a throwaway plan just to extract system requirements. Rejected because plan generation is heavyweight (involves dependency resolution, checksums) and the info command needs to be fast.

**Only support Plan input, require eval before info:**
Force users to run `tsuku eval | tsuku info --deps-only`. Rejected because it breaks the simple `tsuku info --deps-only --system --family alpine zlib` workflow.

### Decision 3: How to Output Repository Information

The current `--deps-only --system` outputs package names, one per line. Repository configurations are structured (URL, key URL, key hash) and don't fit this format.

#### Chosen: Add --repos flag for JSON output

Add `--repos` flag that outputs the complete `SystemRequirements` as JSON:

```bash
# Current behavior preserved
tsuku info --deps-only --system --family debian some-tool
custom-package

# New: include repositories in JSON
tsuku info --deps-only --system --family debian --repos some-tool
{
  "packages": {"apt": ["custom-package"]},
  "repositories": [
    {
      "manager": "apt",
      "type": "repo",
      "url": "https://custom.repo/debian stable main",
      "key_url": "https://custom.repo/key.gpg",
      "key_sha256": "abc123..."
    }
  ],
  "family": "debian"
}
```

The `--repos` flag implies JSON output (can be combined with `--json` but not required). This keeps the simple text-based workflow for packages-only recipes, while enabling structured output when repositories are involved.

#### Alternatives Considered

**Always output JSON when repositories exist:**
Detect presence of repositories and switch output format. Rejected because it creates unpredictable behavior that breaks shell scripts.

**Text format for repositories (one per line):**
Output repositories as structured text lines. Rejected because repository configs have multiple fields (URL, key_url, key_sha256, type) that don't serialize cleanly to single lines.

### Decision 4: How to Extend install-recipe-deps.sh

The helper script needs to set up repositories before installing packages.

#### Chosen: Parse JSON when --repos is available

Update the script to:
1. First check if repositories are needed using `--repos`
2. If repositories exist, parse JSON and set up each one
3. Then install packages as before

```bash
# Check for repositories
JSON=$("$TSUKU" info --deps-only --system --family "$FAMILY" --repos "$RECIPE")

# Parse repositories and set them up
REPOS=$(echo "$JSON" | jq -r '.repositories[]? | @base64')
for repo in $REPOS; do
  TYPE=$(echo "$repo" | base64 -d | jq -r '.type')
  case "$TYPE" in
    repo) setup_apt_repo "$repo" ;;
    ppa) setup_apt_ppa "$repo" ;;
  esac
done

# Install packages
PKGS=$(echo "$JSON" | jq -r ".packages.$PM // [] | .[]")
if [ -n "$PKGS" ]; then
  install_packages "$FAMILY" $PKGS
fi
```

This approach requires `jq` for JSON parsing. Most CI environments have jq available, and it's a small dependency.

#### Alternatives Considered

**Generate shell script from tsuku:**
Add `--shell` flag that outputs a ready-to-execute shell script. Rejected because generating correct shell code for 5 different Linux families is complex and error-prone. Each family has different package manager syntax, repository setup procedures, and error handling. JSON output with a standard parser (jq) is more testable and maintainable than shell code generation.

**Build container image directly:**
Have tsuku build a container with dependencies instead of outputting text. Rejected because it duplicates sandbox functionality and doesn't fit the CI use case where containers are already running.

## Decision Outcome

**Chosen: Consolidate in executor with dual entry points and --repos flag**

### Summary

Move `SystemRequirements` and extraction logic from `sandbox/packages.go` to `executor/system_deps.go`, creating two entry points: one for Recipe input (info command) and one for Plan input (sandbox). Both share a common step-extraction core that handles all 11 action types.

The `tsuku info --deps-only --system` command gains a `--repos` flag that outputs the complete SystemRequirements as JSON, including repository configurations. Without `--repos`, the output remains package names only for backward compatibility.

The `install-recipe-deps.sh` script is extended to call with `--repos`, parse the JSON, set up any repositories, then install packages. This requires jq as a dependency.

**Migration path:**
1. Add `SystemRequirements` type to executor (copied from sandbox)
2. Implement `ExtractSystemRequirementsFromRecipe` and `ExtractSystemRequirementsFromPlan`
3. Update sandbox to use the executor functions
4. Add `--repos` flag to info command
5. Update helper script to handle repositories
6. Delete now-empty sandbox/packages.go (or leave as thin wrapper)
7. Update DESIGN-recipe-driven-ci-testing to reference this design

### Rationale

Consolidating in executor follows the original design intent and respects package layering. The dual entry points handle the Recipe vs Plan difference without forcing either consumer to change their input types. The `--repos` flag adds capability without breaking existing callers.

## Solution Architecture

### Component 1: Consolidated SystemRequirements

**Location**: `internal/executor/system_deps.go`

```go
// SystemRequirements contains all system-level dependencies extracted from a recipe or plan.
type SystemRequirements struct {
    Packages     map[string][]string // Package manager -> package names
    Repositories []RepositoryConfig  // Repository configurations
}

// RepositoryConfig describes a package repository to be added.
type RepositoryConfig struct {
    Manager   string // Package manager: "apt", "dnf", etc.
    Type      string // Repository type: "repo", "ppa", "tap"
    URL       string // Repository URL (for "repo" type)
    KeyURL    string // GPG key URL
    KeySHA256 string // Expected SHA256 hash of GPG key
    PPA       string // PPA identifier (for "ppa" type)
    Tap       string // Homebrew tap name (for "tap" type)
}
```

### Component 2: Extraction Functions

**Location**: `internal/executor/system_deps.go`

The two entry points work with different step types (`recipe.Step` vs `executor.ResolvedStep`), but both have the same relevant fields: `Action string` and `Params map[string]interface{}`. The extraction logic uses only these fields, so we define variants for each input type that share the core switch statement.

```go
// ExtractSystemRequirementsFromRecipe extracts system dependencies from a recipe.
// It filters steps by target platform before extraction.
func ExtractSystemRequirementsFromRecipe(r *recipe.Recipe, target platform.Target) *SystemRequirements {
    filtered := FilterStepsByTarget(r.Steps, target)
    return extractFromRecipeSteps(filtered)
}

// ExtractSystemRequirementsFromPlan extracts system dependencies from a plan.
// The plan is already platform-filtered during generation.
func ExtractSystemRequirementsFromPlan(plan *InstallationPlan) *SystemRequirements {
    return extractFromPlanSteps(plan.Steps)
}

// extractFromRecipeSteps extracts from recipe.Step slices.
func extractFromRecipeSteps(steps []recipe.Step) *SystemRequirements

// extractFromPlanSteps extracts from ResolvedStep slices.
func extractFromPlanSteps(steps []ResolvedStep) *SystemRequirements
```

Both extraction functions implement the same switch statement logic. The duplication is minimal (one switch with 11 cases) and avoids introducing a new interface abstraction for the step types.

**Transitive extraction:** The `--repos` flag also walks transitive dependencies to extract repositories, not just the root recipe. This matches the current behavior of `extractSystemPackagesFromTree()` in `info.go`.

### Component 3: Info Command Changes

**Location**: `cmd/tsuku/info.go`

New flag:
- `--repos`: Include repository configurations (implies JSON output)

Modified behavior in `runDepsOnly()`:
```go
if repos {
    reqs := executor.ExtractSystemRequirementsFromRecipe(r, target)
    printJSON(struct {
        Packages     map[string][]string            `json:"packages"`
        Repositories []executor.RepositoryConfig    `json:"repositories,omitempty"`
        Family       string                         `json:"family,omitempty"`
    }{
        Packages:     reqs.Packages,
        Repositories: reqs.Repositories,
        Family:       family,
    })
} else {
    // Existing behavior: extract packages only
    packages := extractSystemPackagesFromTree(ctx, r, toolName, target)
    // Output one per line...
}
```

### Component 4: Updated Helper Script

**Location**: `.github/scripts/install-recipe-deps.sh`

The script checks for jq availability. If jq is present, it uses `--repos` to get the full picture. If not, it falls back to the package-only mode (which won't work for recipes needing repositories, but maintains backward compatibility).

```bash
if command -v jq &> /dev/null; then
  # Full mode with repository support
  JSON=$("$TSUKU" info --deps-only --system --family "$FAMILY" --repos "$RECIPE")
  setup_repositories "$JSON" "$FAMILY"
  install_packages_from_json "$JSON" "$FAMILY"
else
  # Fallback: packages only (original behavior)
  DEPS=$("$TSUKU" info --deps-only --system --family "$FAMILY" "$RECIPE")
  install_packages "$FAMILY" $DEPS
fi
```

**Shell security requirements:**
- All variable expansions must be double-quoted to prevent injection
- Use `mktemp` for temporary GPG key files
- Verify `key_url` uses HTTPS before downloading
- Verify `key_sha256` is present before trusting downloaded keys

### Component 5: Sandbox Integration

**Location**: `internal/sandbox/packages.go`

The sandbox package has no external consumers outside the tsuku repo. The file will be converted to a thin wrapper for internal consistency, then deprecated:

```go
package sandbox

import "github.com/tsukumogami/tsuku/internal/executor"

// Re-export types for internal compatibility
type SystemRequirements = executor.SystemRequirements
type RepositoryConfig = executor.RepositoryConfig

// ExtractSystemRequirements delegates to executor.
// Deprecated: Call executor.ExtractSystemRequirementsFromPlan directly.
func ExtractSystemRequirements(plan *executor.InstallationPlan) *executor.SystemRequirements {
    return executor.ExtractSystemRequirementsFromPlan(plan)
}
```

In a follow-up PR, update `sandbox/executor.go` to import executor directly and delete `sandbox/packages.go`.

**Note on dnf_repo mapping:** The `dnf_repo` action uses a `gpgkey` parameter that maps to `KeyURL` in `RepositoryConfig`. This mapping is preserved in the consolidated extraction.

## Implementation Approach

### Phase 1: Add Types to Executor

1. Copy `SystemRequirements` and `RepositoryConfig` from sandbox/packages.go to executor/system_deps.go
2. Expand `SystemPackageActions` map to include all 11 action types
3. Implement `extractFromSteps()` with the comprehensive switch statement from sandbox

### Phase 2: Add Entry Points

1. Implement `ExtractSystemRequirementsFromRecipe()` that filters then delegates
2. Implement `ExtractSystemRequirementsFromPlan()` that delegates directly
3. Add tests covering all action types and both entry points

### Phase 3: Update Sandbox

1. Change sandbox/executor.go to call `executor.ExtractSystemRequirementsFromPlan()`
2. Either delete sandbox/packages.go or convert to thin wrapper
3. Update imports in any other sandbox consumers

### Phase 4: Extend Info Command

1. Add `--repos` flag definition
2. Modify `runDepsOnly()` to handle repos mode
3. Update help text and examples
4. Add tests for new flag combinations

### Phase 5: Update Helper Script

1. Add jq detection
2. Implement repository setup functions per family
3. Update package installation to use JSON when available
4. Add fallback path for jq-less environments
5. Test across all five families

### Phase 6: Documentation Updates

1. Update DESIGN-recipe-driven-ci-testing to reference this design as superseding Decision 3
2. Update CLI help and README with new --repos flag
3. Document jq dependency in CI workflow documentation

## Security Considerations

### Download Verification

The `install-recipe-deps.sh` script downloads GPG keys from `key_url` fields when setting up repositories. These downloads are verified:

- **SHA256 verification**: The `key_sha256` field is required for apt_repo and dnf_repo actions. The script verifies downloaded keys match the expected hash before importing.
- **HTTPS enforcement**: The existing `apt_actions.go` and `dnf_actions.go` enforce HTTPS for key URLs during preflight validation. The script should also verify URLs use HTTPS.

**Trust model asymmetry**: PPAs (`apt_ppa`) and Homebrew taps (`brew_tap`) don't have `key_sha256` verification. PPAs rely on Launchpad's infrastructure for key distribution. Homebrew taps use GitHub's trust model. This is acceptable given these platforms' existing security practices, but means PPA/tap recipes have a different trust profile than apt_repo/dnf_repo recipes.

### Execution Isolation

The info command is read-only. It loads recipes and outputs text. No execution, no privilege escalation.

The helper script runs package manager commands with whatever privileges are available (typically root in CI containers). Shell injection is mitigated by:
- **Proper quoting**: All shell variable expansions must use double quotes to prevent word splitting and glob expansion
- **Secure temporary files**: Use `mktemp` for temporary key files instead of predictable `/tmp` paths
- **JSON parsing**: Using jq for structured data prevents injection through field values

### Supply Chain Risks

Recipes control what repositories are added. A malicious recipe could point to an attacker-controlled repository. Mitigations:
- Recipes are reviewed before merge to the registry
- GPG key verification (with sha256 check) prevents MITM attacks
- CI runs in ephemeral containers, limiting blast radius

The `--repos` flag surfaces repository information that was previously hidden in sandbox internals, improving auditability. Repository URLs may reveal organizational infrastructure (private repository servers), so `--repos` output shouldn't be piped to public logs without review.

### User Data Exposure

No change. The command reads recipe files and outputs package/repository names. No data access or transmission beyond recipe loading.

## Consequences

### Positive

- Single source of truth for system dependency extraction
- CI testing can handle recipes with third-party repositories
- `install-recipe-deps.sh` covers all cases sandbox handles
- Follows the original design intent from DESIGN-recipe-driven-ci-testing
- Repository configurations are now auditable via `--repos` flag

### Negative

- Helper script now requires jq for full functionality. This is acceptable because: (a) all major CI providers (GitHub Actions, GitLab CI, CircleCI) pre-install jq, (b) the fallback handles the common case of packages-only recipes, (c) recipes requiring repositories are currently rare
- `--repos` output is JSON-only (can't be piped to xargs like packages)
- Minor code churn in sandbox to update imports
- Slight increase in executor package size

### Migration

Existing consumers of `tsuku info --deps-only --system` continue to work unchanged. The packages-only text output remains the default. The `--repos` flag is opt-in.

Sandbox consumers of `sandbox.ExtractSystemRequirements()` continue to work if we leave a re-export wrapper. If we delete the wrapper, sandbox/executor.go needs an import path change.

### Documents Superseded

This design supersedes Decision 3 ("Code Reuse with Sandbox") in DESIGN-recipe-driven-ci-testing.md. That design specified shared extraction code but didn't anticipate the divergence. This design fulfills the original intent with a concrete implementation.
