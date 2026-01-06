# DESIGN: Linux Family Support for Golden Files

## Status

Proposed

## Upstream Design Reference

This design extends [DESIGN-golden-plan-testing.md](DESIGN-golden-plan-testing.md) to support Linux family-specific plans introduced in [DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md) and [DESIGN-system-dependency-actions.md](DESIGN-system-dependency-actions.md).

**Relevant upstream decisions:**
- Plans now include `linux_family` in the platform object
- Package manager actions have implicit constraints (e.g., `apt_install` implies `linux_family = "debian"`)
- Different families produce different installation steps

**Blocking relationship:** Issue #774 (enable golden files for system dependency recipes) requires this design to proceed.

## Context and Problem Statement

The golden file system validates that plan generation produces expected output by comparing generated plans against stored "golden" files. Recent work introduced Linux family support, where plans can vary by distribution family (debian, rhel, arch, alpine, suse) in addition to OS and architecture.

**Current state:**
- Golden files use naming pattern `{version}-{os}-{arch}.json` (e.g., `v0.46.0-linux-amd64.json`)
- Generation workflow runs 3 platforms: linux-amd64, darwin-arm64, darwin-amd64
- Validation scripts compare generated plans against stored files by os+arch
- The `tsuku eval` command accepts `--linux-family` to generate family-specific plans

**The problem:**
- Plans for Linux now include `linux_family` in the platform object
- Recipes with package manager actions (apt_install, dnf_install) produce different steps per family
- The generation and validation workflows do not account for family variation
- Issue #818 (overwrite bug) was fixed, but family support was deferred pending design

**Why this matters:**
- Issue #774 requires generating golden files for all platform+linux_family combinations
- Milestone 29 (Full Golden Coverage) is blocked until all recipes have complete golden coverage
- Recipes with system dependencies cannot be validated without family-specific golden files

**Scope:**
- Extending golden file naming to include linux_family
- Updating generation workflow to produce family-specific files for Linux
- Updating validation scripts to handle family-specific files
- Defining when family-specific files are needed vs. a single Linux file

**Out of scope:**
- Changes to plan generation logic (already implemented)
- Container building for sandbox execution (covered by DESIGN-structured-install-guide.md)
- Action vocabulary changes (covered by DESIGN-system-dependency-actions.md)

## Decision Drivers

1. **Correctness**: Golden files must accurately represent what tsuku produces for each platform+family combination.

2. **Minimal waste**: Generating 5 identical files for recipes that don't vary by family is wasteful and clutters diffs.

3. **Automation**: The system should automatically determine when family-specific files are needed.

4. **Backwards compatibility**: Existing golden files should remain valid during transition.

5. **CI efficiency**: Avoid multiplying CI job count by 5x when unnecessary.

## Considered Options

### Decision 1: When to Generate Family-Specific Files

How should the system determine whether a recipe needs family-specific golden files?

#### Option 1A: Always Generate All Families

Generate 5 Linux files for every recipe, regardless of whether plans differ.

**Pros:**
- Simple implementation (no detection logic)
- Complete coverage guaranteed
- Predictable file structure

**Cons:**
- 5x file count for most recipes (majority don't vary by family)
- Identical files waste storage and clutter diffs
- CI time multiplied for no benefit
- Makes it harder to see when plans actually differ

#### Option 1B: Auto-Detect from Plan Content

Compare plans across families; only store separate files when content differs.

**Pros:**
- Minimal storage (only store what's different)
- Clean diffs (file exists = plan varies)
- Self-documenting (presence of family file indicates family-specific behavior)

**Cons:**
- Requires running eval for all families to detect differences
- Detection logic adds complexity
- Duplicates knowledge already present in recipe structure

#### Option 1C: Manual Recipe Metadata Declaration

Add a `linux_family_aware: true` field to recipe metadata that authors must set.

**Pros:**
- Explicit declaration (no guessing)
- Fast (no need to generate plans to detect)
- Recipe author decides

**Cons:**
- Schema change required
- Manual maintenance burden
- Authors may forget to set it or set it incorrectly

#### Option 1D: Derive from Recipe Metadata

Extend `tsuku info` to analyze recipe actions and include linux_family in `supported_platforms` when the recipe uses family-specific actions (apt_install, dnf_install, etc.).

**Pros:**
- Clean separation of concerns (recipe metadata describes constraints, tooling follows)
- No runtime detection needed (derived from static recipe analysis)
- Single source of truth for platform support
- Aligns with existing metadata pattern (`tsuku info --metadata-only`)
- No manual maintenance (automatically derived from actions)
- Useful beyond golden files (other tooling can query family support)

**Cons:**
- Requires extending `tsuku info` command
- Metadata derivation logic must stay in sync with action semantics

### Decision 2: File Naming Convention

How should family-specific golden files be named?

#### Option 2A: Family as Suffix

Pattern: `{version}-{os}-{family}-{arch}.json`
Example: `v0.46.0-linux-debian-amd64.json`

**Pros:**
- Consistent with os-arch pattern (platform components together)
- Family clearly visible in filename
- Sorts well (versions, then platforms)

**Cons:**
- Changes pattern for family-aware recipes
- Existing `linux-amd64` files need migration or coexistence

#### Option 2B: Family as Optional Middle Component

Pattern: `{version}-{os}-{arch}.json` OR `{version}-{os}-{family}-{arch}.json`
- Non-family-aware: `v0.46.0-linux-amd64.json`
- Family-aware: `v0.46.0-linux-debian-amd64.json`

**Pros:**
- Backwards compatible (existing files unchanged)
- Clear distinction (has family = varies by family)
- No migration needed for non-family-aware recipes

**Cons:**
- Mixed naming in golden directory
- Slightly more complex parsing logic

#### Option 2C: Directory Per Family

Pattern: `{version}-{os}-{arch}/{family}.json` OR `{version}-{os}-{arch}.json`
- Non-family-aware: `v0.46.0-linux-amd64.json`
- Family-aware: `v0.46.0-linux-amd64/debian.json`, `v0.46.0-linux-amd64/rhel.json`

**Pros:**
- Clear grouping of family variants
- Base file represents "canonical" case

**Cons:**
- Significant structural change
- Complex directory handling
- Breaks existing tooling assumptions

### Decision 3: Canonical Family for Non-Family-Aware Recipes

When a recipe doesn't vary by family, what `linux_family` value should the golden file contain?

#### Option 3A: No Family Field

Omit `linux_family` from the platform object for non-family-aware recipes.

**Pros:**
- Clear signal (no field = no family variation)
- Smallest file size

**Cons:**
- Requires plan generation logic to conditionally include field
- Breaks consistency with family-aware plans

#### Option 3B: Debian as Canonical

Use "debian" as the canonical family for all Linux golden files.

**Pros:**
- Consistent plan structure (all Linux plans have linux_family)
- Debian is most common target (Ubuntu is debian-family)
- Simple implementation (always generate with debian family)

**Cons:**
- Arbitrary choice
- May confuse users expecting family-neutral plans

#### Option 3C: Generate Without Family Flag

Generate non-family-aware plans without `--linux-family` flag; validation uses same approach.

**Pros:**
- Matches current behavior
- No arbitrary family choice

**Cons:**
- Plan structure differs from family-aware plans
- Validation must handle both cases

### Decision 4: Validation Approach

How should validation determine which files to check?

#### Option 4A: Validate What Exists

Iterate over files in golden directory; for each file, parse platform from filename and validate.

**Pros:**
- Simple (directory is source of truth)
- No metadata query needed
- Works with any naming convention

**Cons:**
- Missing files not detected (coverage gaps invisible)
- Relies on generation to have created correct files

#### Option 4B: Validate Based on Recipe Metadata

Query recipe for supported platforms; check each platform has a golden file.

**Pros:**
- Catches missing coverage
- Enforces completeness

**Cons:**
- Metadata doesn't indicate family awareness
- Need additional logic to know when family files are expected

#### Option 4C: Hybrid Approach

Query metadata for platforms, then check directory for family variants. Missing platform = error; extra family files = validate them.

**Pros:**
- Catches missing base coverage
- Validates any family files that exist
- Flexible for transition

**Cons:**
- More complex logic
- Must handle both patterns during transition

## Decision Outcome

**Chosen: 1D + 2B + 3C + 4B**

### Summary

Extend `tsuku info` to expose linux_family awareness in supported_platforms metadata. Golden file tooling queries this metadata to determine which platform+family combinations need files. Use optional family component in filenames. Generate non-family-aware plans without the `--linux-family` flag. Validate based on metadata (enforces completeness).

### Rationale

**Why 1D (derived metadata) over 1A (always), 1B (auto-detect), or 1C (manual metadata):**
- 1A creates 5x waste for most recipes, making diffs noisy and storage larger
- 1B requires runtime detection by comparing plans, duplicating knowledge already in the recipe structure
- 1C requires manual maintenance and risks author error
- 1D derives family awareness from recipe actions via existing constraint mechanisms
- The metadata is automatically correct because it's derived from the same constraints that govern plan generation
- This knowledge is useful beyond golden files (installers, documentation, CI can all query platform support)

**Why 2B (optional component) over 2A (always family) or 2C (directories):**
- 2A would require migrating all existing golden files and changing tooling
- 2C is a significant structural change with complex directory handling
- 2B is backwards compatible: existing `linux-amd64` files work unchanged
- The naming difference (`linux-amd64` vs `linux-debian-amd64`) clearly signals whether the recipe varies by family

**Why 3C (no flag) over 3A (no field) or 3B (debian canonical):**
- 3A requires conditional logic to omit the field, adding complexity
- 3B injects an arbitrary "debian" value into plans that don't vary by family, which is confusing
- 3C is simplest: if the recipe doesn't vary by family, don't pass `--linux-family` at all
- The plan generator already handles the absence of the flag correctly
- Avoids users seeing "debian" in golden files for recipes that work on any family

**Why 4B (metadata-based) over 4A (what exists) or 4C (hybrid):**
- 4A doesn't catch missing coverage (if generation forgot a platform, validation wouldn't notice)
- 4C was designed for transition when detection was runtime-based
- With 1D, metadata is the source of truth for supported platforms
- 4B enforces completeness: if metadata says debian and rhel are supported, both files must exist
- Simpler than 4C: no need to handle mixed patterns

**Platform enumeration:** The metadata from `tsuku info` lists all supported platform+family combinations. Golden file tooling generates and validates exactly those combinations. No detection, no guessing.

## Solution Architecture

### File Naming

**Non-family-aware recipes** (plans identical across Linux families):
```
testdata/golden/plans/f/fzf/
├── v0.46.0-linux-amd64.json       # No linux_family in platform object
├── v0.46.0-darwin-amd64.json
└── v0.46.0-darwin-arm64.json
```

**Family-aware recipes** (plans differ by Linux family):
```
testdata/golden/plans/b/build-tools-system/
├── v1.0.0-linux-debian-amd64.json    # linux_family: "debian"
├── v1.0.0-linux-rhel-amd64.json      # linux_family: "rhel"
├── v1.0.0-linux-arch-amd64.json      # linux_family: "arch"
├── v1.0.0-linux-alpine-amd64.json    # linux_family: "alpine"
├── v1.0.0-linux-suse-amd64.json      # linux_family: "suse"
├── v1.0.0-darwin-amd64.json
└── v1.0.0-darwin-arm64.json
```

### Platform Exclusions

Per [DESIGN-golden-plan-testing.md](DESIGN-golden-plan-testing.md), linux-arm64 is excluded from golden file generation and validation because GitHub Actions does not provide arm64 Linux runners. This design inherits that exclusion: family-specific files are only generated for linux-amd64.

### Architecture Layers

This design separates concerns with step-level constraint resolution:

```
┌─────────────────────────────────────────────────────────────┐
│  Golden File Tooling (scripts, workflows)                   │
│  - Queries: "what platforms does this recipe support?"      │
│  - Generates one file per supported platform                │
│  - No knowledge of steps, actions, or how constraints work  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ tsuku info --metadata-only
┌─────────────────────────────────────────────────────────────┐
│  Recipe Metadata (tsuku info)                               │
│  - Iterates steps, collects effective constraints           │
│  - Returns supported_platforms list                         │
│  - Hides step-level and action-level details from callers   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ step.EffectiveConstraint()
┌─────────────────────────────────────────────────────────────┐
│  Step Constraint Resolution                                 │
│  - Each step has one effective constraint                   │
│  - Combines action's implicit constraint + step's when      │
│  - Caller asks the step, not the action                     │
└─────────────────────────────────────────────────────────────┘
```

**Key principle:** Constraints are resolved at the step level, not the action level. Whether a constraint comes from the action type (implicit) or the step's `when` clause (explicit) is an implementation detail. Callers just ask "what is this step's effective constraint?" and get one answer.

### Metadata Output Format

The `tsuku info` command exposes supported platforms:

```bash
# Non-family-aware recipe (e.g., fzf)
$ tsuku info fzf --metadata-only --json | jq '.supported_platforms'
[
  {"os": "linux", "arch": "amd64"},
  {"os": "linux", "arch": "arm64"},
  {"os": "darwin", "arch": "amd64"},
  {"os": "darwin", "arch": "arm64"}
]

# Family-aware recipe (e.g., build-tools-system)
$ tsuku info build-tools-system --metadata-only --json | jq '.supported_platforms'
[
  {"os": "linux", "arch": "amd64", "linux_family": "debian"},
  {"os": "linux", "arch": "amd64", "linux_family": "rhel"},
  {"os": "linux", "arch": "amd64", "linux_family": "arch"},
  {"os": "linux", "arch": "amd64", "linux_family": "alpine"},
  {"os": "linux", "arch": "amd64", "linux_family": "suse"},
  {"os": "linux", "arch": "arm64", "linux_family": "debian"},
  ...
  {"os": "darwin", "arch": "amd64"},
  {"os": "darwin", "arch": "arm64"}
]
```

### Step-Level Constraint Resolution

Every step has an **effective constraint** that combines all constraint sources into a single result. Callers ask the step for its constraint - they don't query actions directly or combine multiple sources themselves.

```go
// Callers use this - simple and uniform
constraint, err := EffectiveConstraint(step, actionRegistry)
if err != nil {
    // Recipe has conflicting constraints - fail at load time
}
```

The effective constraint combines:
- The action's implicit constraint (if any) - e.g., `apt_install` implies debian
- The step's explicit `when` clause (if any) - e.g., `when.linux_family = "debian"`

#### Merge Semantics

When a step has both an implicit constraint (from action type) and an explicit `when` clause, the merge follows these rules:

1. **Compatible constraints (AND semantics)**: If both specify the same dimension, they must match.
   - `apt_install` (implicit: debian) + `when: linux_family: debian` → debian (valid, redundant but allowed)
   - `apt_install` (implicit: debian) + `when: os: linux` → debian on linux (valid, additional filter)

2. **Conflicting constraints (validation error)**: If implicit and explicit contradict, the recipe is invalid.
   - `apt_install` (implicit: debian) + `when: linux_family: rhel` → **ERROR at recipe load time**
   - This is a recipe authoring error - apt_install cannot run on rhel

3. **Explicit extends implicit**: Explicit constraints can add dimensions not covered by implicit.
   - `apt_install` (implicit: debian) + `when: arch: amd64` → debian + amd64

**Rationale**: Implicit constraints are requirements (the action physically cannot run on other families). Explicit constraints are filters (restrict when the step should execute). A conflict means the recipe author made a mistake - they're asking an action to run in an environment where it cannot work. Failing at recipe load time catches this early.

This merging happens once, inside `EffectiveConstraint()`. The caller sees one result.

**At the recipe level:** CLI users don't think about steps or actions. They query recipe metadata:

```bash
# User just asks: does this recipe have family-specific plans?
tsuku info myrecipe --metadata-only --json | jq '.supported_platforms'
```

The response tells them what platforms are supported. If `linux_family` appears in the platform objects, the recipe is family-aware.

### Why Step-Level, Not Action-Level

The existing `SystemAction.ImplicitConstraint()` interface is an implementation detail, not a caller-facing API. By surfacing constraints at the step level:

1. **Uniform interface** - All steps respond the same way, regardless of action type
2. **Single source of truth** - No need to query action + when clause separately
3. **Implicit/explicit distinction hidden** - Callers don't know or care where the constraint came from
4. **Extensible** - New constraint sources can be added without changing caller code

**Note:** Variable interpolation scanning (e.g., detecting `{{linux_family}}` in URL strings) is explicitly excluded. Constraints must be expressed through the action type or `when` clause, not implicit string patterns.

### Valid Linux Families

The current set of supported families is: `debian`, `rhel`, `arch`, `alpine`, `suse`.

Adding a new family (e.g., `nixos`) requires:
1. Adding it to `ValidLinuxFamilies` constant
2. Implementing a new `SystemAction` type (e.g., `NixInstallAction`) with its `ImplicitConstraint()`
3. Adding detection logic in `platform/family.go`
4. Adding container image support for sandbox execution

Golden file tooling picks up new families automatically via metadata - no script changes needed for that part. But the underlying system (action types, detection, containers) requires new code.

### Generation Workflow Changes

Update `.github/workflows/generate-golden-files.yml`:

1. **Query metadata**: Get supported platforms from `tsuku info`
2. **Filter for runner**: Each runner handles platforms matching its os+arch
3. **Generate files**: Create golden file for each supported platform

```yaml
strategy:
  matrix:
    platform:
      - { runner: ubuntu-latest, os: linux, arch: amd64 }
      - { runner: macos-14, os: darwin, arch: arm64 }
      - { runner: macos-15-intel, os: darwin, arch: amd64 }

steps:
  - name: Generate golden files for this runner
    run: |
      # Query metadata for supported platforms
      # Filter to platforms matching this runner's os+arch
      # Generate golden file for each (including family variants if present)
      ./scripts/regenerate-golden.sh "${{ inputs.recipe }}" \
        --os "${{ matrix.platform.os }}" --arch "${{ matrix.platform.arch }}"
```

The script queries `tsuku info` to determine if family-specific files are needed for Linux, eliminating the need for runtime detection in the workflow.

### Script Changes

**regenerate-golden.sh**:
- Query `tsuku info --metadata-only --json` for supported platforms
- Filter to platforms matching the `--os` and `--arch` arguments
- For each matching platform:
  - If `linux_family` is present: generate with `--linux-family` flag, name file `{version}-{os}-{family}-{arch}.json`
  - If `linux_family` is absent: generate without `--linux-family` flag, name file `{version}-{os}-{arch}.json`

**validate-golden.sh**:
- Query `tsuku info --metadata-only --json` for supported platforms
- Build expected file list from metadata
- Verify each expected file exists in golden directory
- For each file, generate plan with flags matching the platform (include `--linux-family` only if metadata specifies it)
- Report missing files as errors (enforces completeness)

### Validation Logic

Validation is driven by metadata rather than filename parsing:

```bash
# Query metadata for supported platforms
PLATFORMS=$(tsuku info "$RECIPE" --metadata-only --json | jq -c '.supported_platforms[]')

for platform in $PLATFORMS; do
    os=$(echo "$platform" | jq -r '.os')
    arch=$(echo "$platform" | jq -r '.arch')
    family=$(echo "$platform" | jq -r '.linux_family // empty')

    # Determine expected filename
    if [[ -n "$family" ]]; then
        expected_file="$VERSION-$os-$family-$arch.json"
    else
        expected_file="$VERSION-$os-$arch.json"
    fi

    # Check file exists
    if [[ ! -f "$GOLDEN_DIR/$expected_file" ]]; then
        echo "Missing golden file: $expected_file" >&2
        exit 1
    fi

    # Generate plan with flags matching metadata
    eval_args=(--recipe "$RECIPE" --os "$os" --arch "$arch" --version "${VERSION#v}")
    if [[ -n "$family" ]]; then
        eval_args+=(--linux-family "$family")
    fi
    # Note: no --linux-family flag for non-family-aware recipes

    tsuku eval "${eval_args[@]}" | jq -S 'del(.generated_at, .recipe_source)' > /tmp/actual.json
    # Compare with sorted JSON...
done
```

This approach ensures validation matches exactly what the recipe claims to support. Missing files are caught immediately.

### Migration Path

1. **Phase 1**: Extend `Constraint` type and `tsuku info` to expose family awareness
2. **Phase 2**: Update generation/validation scripts to use metadata
3. **Phase 3**: Regenerate golden files for recipes with system dependencies
4. **Phase 4**: Update CI workflows to use new logic
5. **Phase 5**: Validate all recipes pass with new system

Existing golden files remain valid:
- `linux-amd64.json` files are validated without `--linux-family` flag
- Non-family-aware recipes continue to work unchanged
- Family-aware recipes get additional files without breaking existing ones

### Recipe Transition Handling

When a recipe changes from non-family-aware to family-aware (e.g., adding `apt_install` action):

1. **Metadata changes** - `tsuku info` now shows linux_family in supported_platforms
2. **Regeneration creates 5 family files** based on new metadata
3. **The old `linux-amd64.json` is deleted** (replaced with family-specific files)
4. **PR shows the transition** as deletion of one file and addition of 5 files

When a recipe changes from family-aware to non-family-aware (rare):

1. **Metadata changes** - `tsuku info` no longer shows linux_family
2. **Regeneration creates single `linux-amd64.json`** based on new metadata
3. **Old family files are deleted** by the regeneration script
4. **PR shows the transition** as deletion of 5 files and addition of one file

## Implementation Approach

### Phase 1: Add Step Constraint Resolution

Add constraint resolution logic that returns the unified effective constraint for a step.

**Package organization**: The `Step` type lives in `recipe` package, while `ActionRegistry` and `SystemAction` live in `actions` package. To avoid import cycles, use one of these patterns:

1. **Interface in recipe package**: Define `ActionLookup` interface in recipe that actions implements
2. **Free function in executor**: Put `EffectiveConstraint(step, registry)` in executor package (already imports both)
3. **Resolve at load time**: Resolve constraints during recipe loading, store on Step

The exact organization is an implementation detail. The key requirement is that callers get one method/function that returns the unified constraint.

```go
// Conceptual API - exact location TBD during implementation
func EffectiveConstraint(step *Step, actions ActionRegistry) (*Constraint, error) {
    var result Constraint

    // Get action's implicit constraint (if SystemAction)
    if action := actions.Get(step.Action); action != nil {
        if sysAction, ok := action.(SystemAction); ok {
            if c := sysAction.ImplicitConstraint(); c != nil {
                result.Merge(c)
            }
        }
    }

    // Merge step's explicit when clause
    if step.When != nil {
        if err := result.MergeWhen(step.When); err != nil {
            // Conflict detected - e.g., apt_install with when.linux_family: rhel
            return nil, fmt.Errorf("step %q: constraint conflict: %w", step.Action, err)
        }
    }

    return &result, nil
}
```

Note the error return: `MergeWhen` validates that explicit constraints don't conflict with implicit ones (see Merge Semantics above).

This is the **only** place that combines implicit and explicit constraints. Callers just call this function.

### Phase 2: Extend WhenClause

Add `LinuxFamily` field to allow explicit family constraints:

```go
// In recipe/types.go
type WhenClause struct {
    Platform       []string `toml:"platform,omitempty"`
    OS             []string `toml:"os,omitempty"`
    LinuxFamily    string   `toml:"linux_family,omitempty"`  // NEW - singular
    PackageManager string   `toml:"package_manager,omitempty"`
}
```

Note: `LinuxFamily` is singular (like `PackageManager`) since a step targets one family. The existing `Constraint.LinuxFamily` is also singular - each action targets one family. Aggregation to multiple families happens at the recipe level.

### Phase 3: Metadata Aggregation

Add recipe-level supported platforms computation:

```go
func SupportedPlatforms(recipe *Recipe, actions ActionRegistry) []Platform {
    // Collect families explicitly targeted by steps
    familiesUsed := make(map[string]bool)
    hasUnconstrainedLinuxSteps := false

    for _, step := range recipe.Steps {
        c := step.EffectiveConstraint(actions)
        if c.LinuxFamily != "" {
            familiesUsed[c.LinuxFamily] = true
        } else if c.OS == "" || c.OS == "linux" {
            // Step runs on any Linux (no family constraint)
            hasUnconstrainedLinuxSteps = true
        }
    }

    platforms := darwinPlatforms() // darwin/amd64, darwin/arm64

    if len(familiesUsed) == 0 {
        // No family constraints - generic Linux
        platforms = append(platforms,
            Platform{OS: "linux", Arch: "amd64"},
            Platform{OS: "linux", Arch: "arm64"},
        )
    } else if hasUnconstrainedLinuxSteps {
        // Mixed: some steps have family constraints, some don't
        // Plans differ by family because unconstrained steps run on all,
        // but constrained steps only run on their family
        // Example: [download, apt_install] on debian vs [download] on rhel
        for _, family := range AllLinuxFamilies {
            platforms = append(platforms,
                Platform{OS: "linux", Arch: "amd64", LinuxFamily: family},
                Platform{OS: "linux", Arch: "arm64", LinuxFamily: family},
            )
        }
    } else {
        // Only family-constrained steps - just those families
        // Example: apt_install only → only debian
        // Other families would produce empty plans (no steps match)
        for family := range familiesUsed {
            platforms = append(platforms,
                Platform{OS: "linux", Arch: "amd64", LinuxFamily: family},
                Platform{OS: "linux", Arch: "arm64", LinuxFamily: family},
            )
        }
    }

    return platforms
}
```

**Three cases:**

| Recipe Pattern | familiesUsed | hasUnconstrained | Result |
|---------------|--------------|------------------|--------|
| `download` only | {} | true | Generic linux (no family) |
| `apt_install` only | {debian} | false | debian only |
| `apt_install` + `dnf_install` | {debian, rhel} | false | debian + rhel |
| `download` + `apt_install` | {debian} | true | All 5 families |

The fourth case (mixed) expands to all families because the plans differ: debian gets `[download, apt_install]`, rhel gets `[download]`. Both are valid outputs that need golden files for validation.

Update `tsuku info --metadata-only --json` to include `supported_platforms`.

### Phase 4: Script Updates

1. Update `regenerate-golden.sh` to query metadata and generate appropriate files
2. Update `validate-golden.sh` to use metadata-driven validation
3. Add `--linux-family` flag to scripts for generating family-specific plans

### Phase 5: Workflow Updates

1. Update `generate-golden-files.yml` to use metadata-based generation
2. Ensure artifact naming handles both family and non-family files
3. Merge step handles both patterns

### Phase 6: Documentation

1. Update CONTRIBUTING.md with family-aware golden file guidance
2. Add examples for family-aware vs non-family-aware recipes
3. Document the metadata format

### WhenClause.Matches() Signature Change

The current `WhenClause.Matches()` signature is:

```go
func (w *WhenClause) Matches(os, arch string) bool
```

This must be extended to include `linuxFamily`:

```go
func (w *WhenClause) Matches(os, arch, linuxFamily string) bool
```

**Breaking change**: All callers of `Matches()` must be updated. Since tsuku is pre-GA, this is acceptable. The change enables filtering based on the new dimension.

Alternatively, use a target struct to future-proof against additional dimensions:

```go
type Target struct {
    OS          string
    Arch        string
    LinuxFamily string
}

func (w *WhenClause) Matches(target Target) bool
```

This pattern allows adding new dimensions (e.g., libc variant, CPU features) without changing the signature again.

### Future Extensibility

Adding new constraint dimensions requires changes to:
1. `Constraint` type - add the new field
2. `WhenClause` type - add the new field
3. `EffectiveConstraint()` - merge the new dimension
4. `Constraint.MergeWhen()` - handle new dimension with conflict detection
5. `WhenClause.Matches()` or `Target` struct - include new dimension
6. `SupportedPlatforms()` - expand platforms for the new dimension
7. Golden file naming - incorporate the new dimension

The step-level abstraction means most callers don't change - they still just call `EffectiveConstraint()`. But adding a dimension is not free; it requires coordinated changes across the constraint model.

## Security Considerations

### Download Verification

**Not affected.** This design only changes file naming and generation logic; checksums are still computed and stored in plans as before. The security properties of the golden plan system remain unchanged.

### Execution Isolation

**Not affected.** Execution validation still runs plans through `tsuku install --plan --force`. The isolation model is unchanged.

### Supply Chain Risks

**Not affected.** Golden files still capture checksums from real downloads. The addition of family-specific files increases coverage (more platform combinations are validated) but doesn't change the trust model.

### User Data Exposure

**Not applicable.** Golden file generation and validation do not access user data. All operations use recipe files and upstream artifact metadata.

## Consequences

### Positive

- **Complete coverage**: Recipes with system dependencies can have fully validated golden files
- **Self-documenting**: File naming indicates whether a recipe varies by family
- **Minimal waste**: Non-family-aware recipes retain single Linux file
- **Backwards compatible**: Existing golden files work without migration
- **Single source of truth**: Metadata is the authoritative source for platform support
- **Step-level abstraction**: Callers ask steps for constraints, don't need to know about implicit vs explicit
- **Uniform interface**: All steps respond the same way to `EffectiveConstraint()`
- **Clean separation**: Golden file tooling is decoupled from constraint resolution logic
- **Useful beyond golden files**: Other tooling can query family support (documentation, installers, CI)

### Negative

- **Requires `WhenClause` extension**: Must add `linux_family` field to step conditions
- **Mixed naming**: Directory contains both old-style and new-style filenames during transition
- **Extension not free**: Adding new constraint dimensions requires coordinated changes (see Implementation Approach)

### Mitigations

- **WhenClause extension**: Follows existing pattern for `os` and `platform` fields. Singular field like `PackageManager`.
- **Mixed naming**: Clear naming convention and documentation make the pattern understandable.
- **Extension cost**: The step-level abstraction localizes changes to `EffectiveConstraint()` - callers don't change.
