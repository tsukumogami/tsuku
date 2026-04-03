# Phase 2 Research: Maintainer Perspective

## Lead 1: Skill Maintenance Protocol

### Findings

#### Action Registry and Interfaces
Tsuku's action system is defined in `internal/actions/action.go` with a registry-based pattern:
- **Action Interface**: Every action must implement `Action` interface with:
  - `Name() string`: action identifier
  - `Execute(ctx *ExecutionContext, params map[string]interface{}) error`: execution logic
  - `IsDeterministic() bool`: whether action produces identical output given identical inputs
  - `Dependencies() ActionDeps`: declares install-time, runtime, eval-time, and platform-specific dependencies
- **Registry**: Centralized `registry` map (thread-safe) holds all registered actions, populated by `init()` function in each action file
- **Decomposable Interface**: Composite actions (cargo_install, npm_install, go_build, etc.) implement `Decomposable` with `Decompose()` method that expands to primitive steps during plan generation

**Critical for skill drift**: Recipe authors rely on:
1. Action names and parameter schemas (used in recipe `[[steps]]` sections)
2. Action behavior during decomposition (composite actions expand to primitives)
3. Implicit dependencies from actions (declared via `Dependencies()` method)

#### Recipe Type System
Recipes are defined in `internal/recipe/types.go` with structured TOML:
- **Recipe struct**: Contains `Metadata`, `Version`, `Resources`, `Patches`, `Steps`, `Verify`
- **Metadata section**: name, description, homepage, type ("tool" or "library"), version_format, unsupported_platforms, tier
- **Step struct**: action name + flattened params map. When clause for conditional execution (platform/gpu/libc filters)
- **Version section**: source (github_repo, github_file, custom, npm, pypi, etc.), tag_prefix, formula (homebrew)
- **Verify section**: command + pattern for installation verification

**Critical for skill drift**: Recipe structure changes affect:
- Parameter encoding/decoding (param types and names)
- When clause syntax for conditional steps
- Metadata fields that AI authors reference in exemplar recipes

#### Version Provider Registry
Version resolution in `internal/version/registry.go` and `internal/version/provider.go`:
- **VersionResolver interface**: Minimal interface with `ResolveLatest()`, `ResolveVersion()`, `SourceDescription()`
- **VersionLister interface**: Extends VersionResolver with optional `ListVersions()` for providers that support it
- **Factory pattern**: `ProviderFactory` in `provider_factory.go` registers strategies for each source type (github_file, github_archive, npm, pypi, homebrew, custom, etc.)
- **Registry pattern**: Custom sources can be registered via `version.Registry.Register()` for extensibility

**Critical for skill drift**: Recipe authors reference version sources in `[version]` section:
- `source = "npm"`, `source = "github_file"`, etc.
- Breaking changes: source removal, parameter name changes, fallback strategy changes

#### Plan Generation and Executor
Plan generation in `internal/executor/plan_generator.go` drives recipe evaluation:
- **GeneratePlan()**: Takes `PlanConfig` (OS, Arch, LinuxFamily, GPU, AutoAcceptEvalDeps) and recipe, produces `InstallationPlan`
- **Step evaluation**: For each recipe step:
  1. If action is `Decomposable`, call `Decompose()` to expand to primitive steps
  2. For primitive actions, validate parameters and call `Execute()` (if action supports deterministic mode)
  3. For composite actions, handle parameter evaluation (e.g., go_build needs go version resolution)
  4. Handle `When` clauses for platform/gpu/libc filtering
- **Dependencies resolution** in `internal/actions/resolver.go`:
  - `ResolveDependencies()`: Collects deps from all steps' actions, with recipe-level overrides
  - `ResolveTransitive()`: Recursively expands deps to full closure, detects cycles
  - Dependencies are merged from: action implicit + platform-specific + step-level extras, with recipe-level replacements taking precedence

**Critical for skill drift**: Recipe authors rely on:
- Action parameter evaluation during plan generation
- Composite action decomposition behavior
- Dependency resolution precedence and transitive closure
- When clause matching logic for platform/gpu/libc

#### Golden File Validation CI Pattern
CI in `.github/workflows/validate-golden-code.yml` validates that recipe plans remain stable:
- **Trigger**: When core action/executor/recipe/version code changes
- **Process**: Regenerates golden files (JSON plans for all recipes on each platform)
- **Exclusions**: `testdata/golden/code-validation-exclusions.json` lists recipes that can change freely
- **Failure mode**: Plan drift detected; contributors must regenerate with `scripts/validate-all-golden.sh --os linux --category embedded`

### Implications for Requirements

**CLAUDE.md Section: "tsuku-recipes Plugin Maintenance"**

Maintenance protocol should alert contributors to skill drift when changes affect:

1. **Action registration** (`internal/actions/resolver.go`, action files):
   - Adding/removing actions: recipe-test skill examples referencing old actions become invalid
   - Changing action parameter schemas: recipe-author exemplars may use deprecated params
   - Changing `Decomposable.Decompose()` output: decomposition exemplars in skills produce wrong primitive steps
   - Changing `Dependencies()` return values: recipe-test skill must re-validate dependencies

2. **Recipe structure** (`internal/recipe/types.go`, loader):
   - Recipe field name changes (e.g., `step.action` → `step.operation`)
   - When clause syntax changes (e.g., removing gpu filter support)
   - Metadata field additions/removals (e.g., new tier field)
   - Version source name changes (e.g., `github_file` → `github_release`)

3. **Version provider strategies** (`internal/version/provider*.go`):
   - Removing or renaming version sources
   - Changing source parameter names in `[version]` section
   - Changing fallback behavior (e.g., if npm fails, try github_file)

4. **Plan generation logic** (`internal/executor/plan_generator.go`):
   - Changes to When clause matching that filter steps differently
   - Changes to dependency resolution precedence
   - Changes to how composite actions decompose

### Open Questions

1. **Exemplar recipe scope**: Should recipe-author skill only generate SIMPLE recipes (single-step primitives) to reduce drift surface? Or should it support composites?
   - Answer affects: How often examples break with action changes

2. **Golden file baseline**: Should recipe-test skill snapshot current golden files to detect plan drift, or only validate against live generation?
   - Answer affects: Whether skill can detect when a recipe's expected plan has changed

3. **Skill content version pinning**: Should skills declare a minimum tsuku version or action API version?
   - Answer affects: Whether old skill exemplars can be retroactively "broken" by new versions

---

## Lead 2: CI Freshness Checks

### Findings

#### Existing CI Pattern: Validate Golden Code
`.github/workflows/validate-golden-code.yml` is the existing pattern for detecting when code changes invalidate recipe content:
- **Scope**: Validates ALL golden files when action/executor/recipe/version code changes
- **Method**: Regenerates plans for all recipes; if diffs, workflow fails with guidance to regenerate
- **Exclusions**: Allows selective exemption of recipes that legitimately drift (e.g., due to API changes)

#### Simplest CI for Skill Content Freshness
For recipe-author and recipe-test skills, the simplest check is **file existence + basic structure validation**:

**Option A: File Existence Check** (minimal, most practical)
- Verify all `.md` files referenced in skill docs still exist
- Verify all recipe `.toml` files referenced in exemplars still exist
- Verify skill metadata references correct file paths
- Can be implemented as simple bash script in workflow

**Option B: Golden Plan Snapshot** (medium complexity)
- On skill creation/update: snapshot golden files for exemplar recipes
- On PR with action/executor changes: compare new plans against snapshot
- Reuses existing golden file diff logic from validate-golden-code.yml
- Detects when exemplar recipes' plans change unexpectedly

**Option C: Full Skill Content Evaluation** (high complexity, like koto's approach)
- Actually run skill execution in CI environment
- Verify skill outputs parse and validate correctly
- Verify exemplar recipes generate valid plans
- Requires containerized test harness

#### Tsuku vs Koto Patterns
Koto (mentioned in context) likely uses **Option C** (eval harness) to validate skill examples at generation time.
Tsuku's golden file pattern suggests **Option A or B** would integrate cleanly:
- Option A reuses existing `scripts/validate-*.sh` pattern
- Option B reuses existing golden diff logic from `validate-golden-code.yml`
- Both avoid building a new harness

### Implications for Requirements

**CI Workflow: "recipe-author-test" and "recipe-test-verify"**

For Phase 2 implementation, recommend:

1. **Create `.github/workflows/validate-skill-exemplars.yml`**:
   - Trigger: On changes to `docs/skills/recipe-author.md` or `docs/skills/recipe-test.md`
   - Check 1: All referenced recipe `.toml` files exist
   - Check 2: All referenced section links resolve
   - Check 3 (optional): Run `tsuku validate` on all exemplar recipes to catch TOML errors
   
   Implementation: ~50 lines bash, uses existing `tsuku` binary

2. **For recipe-author skill specifically**:
   - Add step to verify generated exemplar recipes are valid TOML + have required fields
   - Use: `tsuku validate --quiet {exemplar_file}`
   - Prevents skill from teaching invalid syntax

3. **For recipe-test skill specifically**:
   - Add step to verify golden file snapshots for exemplar recipes still exist
   - Compare golden files' action lists against current action registry
   - Detects when recipes reference actions that no longer exist

4. **Error messaging**:
   - Link back to this CLAUDE.md maintenance section
   - Suggest: "If skill examples broke due to tsuku code changes, regenerate with [command]"

---

## Summary

### Key Maintenance Trigger Areas
Recipe skills will drift when these tsuku internals change:
1. **Action interface/registration** (name, params, Dependencies(), Decompose())
2. **Recipe structure** (field names, When syntax, version sources)
3. **Plan generation** (decomposition logic, dependency resolution, When matching)

### Recommended CLAUDE.md Section
Add "tsuku-recipes Plugin Maintenance" with:
- Explicit list of files that trigger skill assessment (above 3 areas)
- Decision tree: "Did I change [action params / recipe fields / plan logic]? → Regenerate skill exemplars"
- Link to skill regeneration procedure (in separate skill authoring doc)

### Recommended CI Setup
Implement simple file existence + structure validation (`validate-skill-exemplars.yml`):
- Checks exemplar `.toml` files exist and are valid
- ~50 LOC, reuses existing `tsuku validate` command
- Runs on skill doc changes
- Prevents drift from being deployed

### Implementation Path for Phase 2
1. Document maintenance protocol in CLAUDE.md (done above)
2. Create `validate-skill-exemplars.yml` workflow (new)
3. Add skill regeneration commands to skill docs (future, when skills created)
4. Optional: Add golden file snapshot validation for recipe-test skill (future enhancement)
