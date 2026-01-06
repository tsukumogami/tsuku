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
- The `tsuku eval` command accepts `--linux-family` to simulate a target family (e.g., generate a debian plan while on macOS)

**The problem:**
- Family-aware recipes produce plans that include `linux_family` in the platform object
- Recipes with package manager actions (apt_install, dnf_install) produce different steps per family
- The generation and validation workflows do not account for family variation
- Issue #818 (overwrite bug) was fixed, but family support was deferred pending design

**Why this matters:**
- Issue #774 requires generating golden files for all platform+linux_family combinations
- Milestone 29 (Full Golden Coverage) is blocked until all recipes have complete golden coverage
- Recipes with system dependencies cannot be validated without family-specific golden files

**Scope:**
- Extending golden file naming to include linux_family for family-aware recipes
- Updating generation workflow to produce family-specific files for family-aware recipes
- Updating validation scripts to handle both family-aware and family-agnostic recipes
- Defining how to determine if a recipe is family-aware (and thus needs multiple Linux files)

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

#### Option 3C: Let Recipe Nature Determine Output

Family-agnostic recipes produce plans without `linux_family` in the platform object (this is inherent to the recipe, not controlled by flags). Golden file generation omits the `--linux-family` flag for these recipes since it would have no effect.

**Pros:**
- Plan structure reflects recipe semantics (family-agnostic recipes have no family field)
- No arbitrary family injected into family-agnostic plans
- Flag usage matches intent: only used when simulating a target family matters

**Cons:**
- Plan structure intentionally differs between family-aware and family-agnostic recipes
- Validation must recognize both patterns

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

Extend `tsuku info` to expose linux_family awareness in supported_platforms metadata. Golden file tooling queries this metadata to determine which platform+family combinations need files. Use optional family component in filenames (family-aware recipes get `linux-debian-amd64.json`, family-agnostic recipes get `linux-amd64.json`). For family-aware recipes, use `--linux-family` to simulate each target family during cross-platform generation. Validate based on metadata (enforces completeness).

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

**Why 3C (recipe nature) over 3A (no field) or 3B (debian canonical):**
- 3A requires conditional logic to omit the field, adding complexity
- 3B injects an arbitrary "debian" value into plans that don't vary by family, which is confusing
- 3C reflects reality: family-agnostic recipes inherently produce plans without `linux_family`
- The `--linux-family` flag is for simulation, not for controlling plan structure
- Family-agnostic golden files correctly show no family field, matching what users see at runtime

**Why 4B (metadata-based) over 4A (what exists) or 4C (hybrid):**
- 4A doesn't catch missing coverage (if generation forgot a platform, validation wouldn't notice)
- 4C was designed for transition when detection was runtime-based
- With 1D, metadata is the source of truth for supported platforms
- 4B enforces completeness: if metadata says debian and rhel are supported, both files must exist
- Simpler than 4C: no need to handle mixed patterns

**Platform enumeration:** The metadata from `tsuku info` lists all supported platform+family combinations. Golden file tooling generates and validates exactly those combinations. No detection, no guessing.

## Solution Architecture

### Key Invariant: Recipe Nature Determines Plan Structure

**Family-aware recipes** (those with family-constrained actions like `apt_install` or `{{linux_family}}` interpolation) **always** include `linux_family` in their plan output. When `--linux-family` is not passed, the plan generator detects the system's family. When `--linux-family` is passed, it simulates the specified family.

**Family-agnostic recipes** (those without family-specific actions or interpolation) **never** include `linux_family` in their plan output. The `--linux-family` flag has no effect on these recipes - they produce identical plans regardless.

The flag is a **simulation mechanism** for cross-platform golden file generation, not a switch that controls plan structure. Golden file tooling uses it to generate debian plans on a macOS CI runner, not to "enable" family support.

### File Naming

**Family-agnostic recipes** (no family-specific actions or interpolation):
```
testdata/golden/plans/f/fzf/
├── v0.46.0-linux-amd64.json       # Recipe is family-agnostic: no linux_family field
├── v0.46.0-darwin-amd64.json
└── v0.46.0-darwin-arm64.json
```

**Family-aware recipes** (have family-constrained actions or `{{linux_family}}` interpolation):
```
testdata/golden/plans/b/build-tools-system/
├── v1.0.0-linux-debian-amd64.json    # Recipe is family-aware: linux_family: "debian"
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
│  - Calls AnalyzeRecipe() → RecipeFamilyPolicy               │
│  - Calls SupportedPlatforms() → []Platform                  │
│  - Hides step-level and action-level details from callers   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ step.Analysis()
┌─────────────────────────────────────────────────────────────┐
│  Step Analysis (pre-computed at load time)                  │
│  - StepAnalysis = Constraint + FamilyVarying                │
│  - Constraint: where can step run (OS, Arch, LinuxFamily)   │
│  - FamilyVarying: does output differ by family              │
└─────────────────────────────────────────────────────────────┘
```

**Key principle:** Step analysis is computed once at recipe load time and stored on the Step. Callers just call `step.Analysis()` - no registry, no runtime computation. The distinction between implicit (action type) and explicit (when clause) constraints is hidden.

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

### Step-Level Analysis

Every step has a pre-computed **analysis** that contains its constraint and variation status. Callers ask the step directly - no registry needed at query time.

```go
// Callers use this - simple and uniform (no error check needed)
analysis := step.Analysis()
// analysis.Constraint: where can this step run
// analysis.FamilyVarying: does output differ by family
```

The `Analysis()` method never returns nil - this is guaranteed by the `NewStep()` constructor which validates and computes analysis at creation time. Errors are surfaced during recipe loading, not at access time.

The analysis combines:
- The action's implicit constraint (if any) - e.g., `apt_install` implies debian
- The step's explicit `when` clause (if any) - e.g., `when.linux_family = "debian"`

#### Merge Semantics

When a step has both an implicit constraint (from action type) and an explicit `when` clause, the merge follows these rules:

1. **Compatible constraints (AND semantics)**: If both specify the same dimension, they must match.
   - `apt_install` (implicit: linux/debian) + `when: linux_family: debian` → linux/debian (valid, redundant)
   - `apt_install` (implicit: linux/debian) + `when: os: linux` → linux/debian (valid, redundant)

2. **Conflicting constraints (validation error)**: If implicit and explicit contradict on any dimension, the recipe is invalid.
   - `apt_install` (implicit: linux/debian) + `when: linux_family: rhel` → **ERROR** (family conflict)
   - `apt_install` (implicit: linux/debian) + `when: os: darwin` → **ERROR** (OS conflict)
   - This is a recipe authoring error - apt_install cannot run on darwin or rhel

3. **Explicit extends implicit**: Explicit constraints can add dimensions not covered by implicit.
   - `apt_install` (implicit: linux/debian) + `when: arch: amd64` → linux/debian/amd64

**Conflict detection implementation:**

```go
// cloneOrEmpty returns a copy of the constraint, or an empty constraint if nil.
// Private helper - avoids confusing nil-receiver methods.
func cloneOrEmpty(c *Constraint) *Constraint {
    if c == nil {
        return &Constraint{}
    }
    return &Constraint{OS: c.OS, Arch: c.Arch, LinuxFamily: c.LinuxFamily}
}

// mergeWhenClause merges explicit when clause with implicit constraint.
// Returns error if any dimension conflicts.
func mergeWhenClause(implicit *Constraint, when *WhenClause) (*Constraint, error) {
    result := cloneOrEmpty(implicit)

    // Check Platform array conflict (e.g., apt_install + when.platform: ["darwin/arm64"])
    // Platform entries are "os/arch" tuples that must be compatible with implicit OS
    if len(when.Platform) > 0 && result.OS != "" {
        compatible := false
        for _, p := range when.Platform {
            if os, _, _ := strings.Cut(p, "/"); os == result.OS {
                compatible = true
                break
            }
        }
        if !compatible {
            return nil, fmt.Errorf("platform conflict: action requires OS %q but when.platform specifies %v",
                result.OS, when.Platform)
        }
    }

    // Check OS conflict
    // Note: when.os = ["linux", "darwin"] (multi-OS) leaves result.OS empty
    // because we can't pick one. This is intentional - step runs on multiple OSes.
    if len(when.OS) > 0 {
        if result.OS != "" && !slices.Contains(when.OS, result.OS) {
            return nil, fmt.Errorf("OS conflict: action requires %q but when clause specifies %v",
                result.OS, when.OS)
        }
        if result.OS == "" && len(when.OS) == 1 {
            result.OS = when.OS[0]
        }
        // Multi-OS case: result.OS stays empty (unconstrained within the listed OSes)
    }

    // Check LinuxFamily conflict
    if when.LinuxFamily != "" {
        if result.LinuxFamily != "" && result.LinuxFamily != when.LinuxFamily {
            return nil, fmt.Errorf("linux_family conflict: action requires %q but when clause specifies %q",
                result.LinuxFamily, when.LinuxFamily)
        }
        result.LinuxFamily = when.LinuxFamily
    }

    // Check Arch conflict
    if when.Arch != "" {
        if result.Arch != "" && result.Arch != when.Arch {
            return nil, fmt.Errorf("arch conflict: action requires %q but when clause specifies %q",
                result.Arch, when.Arch)
        }
        result.Arch = when.Arch
    }

    // Validate final constraint (catches invalid combinations like darwin+debian)
    if err := result.Validate(); err != nil {
        return nil, err
    }

    return result, nil
}
```

**Rationale**: Implicit constraints are requirements (the action physically cannot run elsewhere). Explicit constraints are filters. A conflict means the recipe author made a mistake - they're asking an action to run where it cannot work. Failing at load time catches this early.

This merging happens once, inside `EffectiveConstraint()`. The caller sees one result.

**At the recipe level:** CLI users don't think about steps or actions. They query recipe metadata:

```bash
# User asks: is this recipe family-aware?
tsuku info myrecipe --metadata-only --json | jq '.supported_platforms'
```

The response lists supported platforms. If `linux_family` appears in the platform objects, the recipe is family-aware (its plans include `linux_family`).

### Why Step-Level, Not Action-Level

The existing `SystemAction.ImplicitConstraint()` interface is an implementation detail, not a caller-facing API. By surfacing constraints at the step level:

1. **Uniform interface** - All steps respond the same way, regardless of action type
2. **Single source of truth** - No need to query action + when clause separately
3. **Implicit/explicit distinction hidden** - Callers don't know or care where the constraint came from
4. **Extensible** - New constraint sources can be added without changing caller code

### Variable Interpolation Scanning

**Any action** can have parameters that use `{{linux_family}}` interpolation. Such steps are **family-varying** - they run on all families but produce different output for each.

```toml
# download action with family in URL
[[steps]]
action = "download"
url = "https://example.com/pkg-{{linux_family}}.tar.gz"

# extract action with family in path
[[steps]]
action = "extract"
dest = "$TSUKU_HOME/tools/{{linux_family}}/myapp"

# run action with family in command
[[steps]]
action = "run"
command = "setup-{{linux_family}}.sh"
```

The scanning is **action-agnostic**: it walks all string fields in the Step struct regardless of action type. Any action (including future or composite actions) that uses `{{linux_family}}` in any parameter will be detected.

This is distinct from family-constrained steps:
- **Family-constrained** (`apt_install`): Only runs on one family (debian)
- **Family-varying** (any action with `{{linux_family}}`): Runs on all families, output differs

Both cases require family-specific golden files, but they answer different questions:

| Concept | Question | Type |
|---------|----------|------|
| Constraint | Where can this step run? | Requirement |
| Variation | Does output differ by family? | Property |

These must be separate types - bundling them is a category error:

```go
// Constraint answers: "where can this step run?"
// Represents platform requirements. nil means unconstrained.
type Constraint struct {
    OS          string   // e.g., "linux", "darwin", or empty (any)
    Arch        string   // e.g., "amd64", "arm64", or empty (any)
    LinuxFamily string   // e.g., "debian", or empty (any linux)
}

// Validate returns an error if the constraint contains invalid combinations.
// Invalid state: LinuxFamily set when OS is not "linux" (or empty).
func (c *Constraint) Validate() error {
    if c == nil {
        return nil
    }
    if c.LinuxFamily != "" && c.OS != "" && c.OS != "linux" {
        return fmt.Errorf("invalid constraint: linux_family %q cannot be set when OS is %q",
            c.LinuxFamily, c.OS)
    }
    return nil
}

// StepAnalysis combines constraint with variation detection
// This is what computeAnalysis() returns, stored on Step.
type StepAnalysis struct {
    Constraint    *Constraint  // nil means unconstrained (runs anywhere)
    FamilyVarying bool         // true if step uses {{linux_family}} interpolation
}
```

Aggregation logic uses both fields:
- `Constraint.LinuxFamily` set → add that family to the set
- `FamilyVarying` true → expand to all families

**Edge case: constrained + varying.** A step can have both a family constraint AND use `{{linux_family}}` interpolation:

```toml
[[steps]]
action = "download"
url = "https://example.com/{{linux_family}}-tool.tar.gz"
when.linux_family = "debian"
```

This step:
- Only runs on debian (constraint)
- Uses interpolation (varying flag is true)

At the step level, `FamilyVarying=true` indicates the output differs. At the recipe level, aggregation respects the constraint - this step contributes `debian` to `familiesUsed`, not "all families". The interpolation happens at plan generation time within the constrained family.

If other steps in the recipe are unconstrained or vary by family, the recipe may still be `FamilyVarying` or `FamilyMixed`. The per-step constraint is honored during plan generation; the varying flag affects what appears in the plan for that family.

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
  - If metadata includes `linux_family` (recipe is family-aware): pass `--linux-family` to simulate target family, name file `{version}-{os}-{family}-{arch}.json`
  - If metadata omits `linux_family` (recipe is family-agnostic): omit flag (would have no effect), name file `{version}-{os}-{arch}.json`

**validate-golden.sh**:
- Query `tsuku info --metadata-only --json` for supported platforms
- Build expected file list from metadata (family-aware recipes have multiple Linux entries)
- Verify each expected file exists in golden directory
- For each file, generate plan by simulating the target platform (pass `--linux-family` when metadata specifies a family)
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

    # Generate plan by simulating target platform
    eval_args=(--recipe "$RECIPE" --os "$os" --arch "$arch" --version "${VERSION#v}")
    if [[ -n "$family" ]]; then
        # Family-aware recipe: simulate target family
        eval_args+=(--linux-family "$family")
    fi
    # Family-agnostic recipes have no linux_family in metadata, so no simulation needed

    tsuku eval "${eval_args[@]}" | jq -S 'del(.generated_at, .recipe_source)' > /tmp/actual.json
    # Compare with sorted JSON...
done
```

This approach ensures validation matches exactly what the recipe claims to support. Missing files are caught immediately.

### Migration Path

1. **Phase 1**: Extend `Constraint` type and `tsuku info` to expose family awareness
2. **Phase 2**: Update generation/validation scripts to use metadata
3. **Phase 3**: Regenerate golden files for family-aware recipes
4. **Phase 4**: Update CI workflows to use new logic
5. **Phase 5**: Validate all recipes pass with new system

Existing golden files remain valid:
- `linux-amd64.json` files for family-agnostic recipes remain unchanged
- Family-agnostic recipes continue to produce plans without `linux_family` field
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

### Phase 1: Step Analysis at Load Time

Resolve constraints during recipe loading and store them on the Step. This eliminates the need to pass a registry at query time - callers just ask the step.

**Go idiom: Constructor guarantee, not error-returning getter.** If analysis isn't computed, that's a programming bug, not a runtime condition. Use a factory function that guarantees validity.

```go
// In recipe/types.go - Step includes pre-computed analysis
type Step struct {
    Action string
    When   *WhenClause
    Params map[string]interface{}

    // Pre-computed during loading - never nil after construction
    analysis *StepAnalysis
}

// Analysis returns the pre-computed step analysis.
// Never returns nil - guaranteed by NewStep constructor.
func (s *Step) Analysis() *StepAnalysis {
    return s.analysis
}

// ConstraintLookup returns the implicit constraint for an action by name.
// Returns nil if the action has no implicit constraint (runs anywhere).
// Returns (nil, false) if the action is unknown (validation error).
type ConstraintLookup func(actionName string) (constraint *Constraint, known bool)

// NewStep creates a Step with pre-computed analysis.
// Returns error if:
// - Action is unknown (lookup returns known=false)
// - Constraint conflicts detected (OS, Arch, or LinuxFamily mismatch)
func NewStep(action string, when *WhenClause, params map[string]interface{},
             lookup ConstraintLookup) (*Step, error) {
    analysis, err := computeAnalysis(action, when, params, lookup)
    if err != nil {
        return nil, err
    }
    return &Step{
        Action:   action,
        When:     when,
        Params:   params,
        analysis: analysis,
    }, nil
}

func computeAnalysis(action string, when *WhenClause, params map[string]interface{},
                     lookup ConstraintLookup) (*StepAnalysis, error) {
    var constraint *Constraint

    // Get action's implicit constraint - also validates action exists
    implicit, known := lookup(action)
    if !known {
        return nil, fmt.Errorf("unknown action %q", action)
    }
    if implicit != nil {
        constraint = cloneOrEmpty(implicit)
    }

    // Merge explicit when clause (validates conflicts)
    if when != nil {
        merged, err := mergeWhenClause(constraint, when)
        if err != nil {
            return nil, err
        }
        constraint = merged
    }

    // Detect interpolated variables (e.g., {{linux_family}}, {{arch}})
    vars := detectInterpolatedVars(params)
    familyVarying := vars["linux_family"]

    return &StepAnalysis{
        Constraint:    constraint,
        FamilyVarying: familyVarying,
    }, nil
}
```

**Benefits of constructor guarantee:**
- Errors at construction time, not access time
- `Analysis()` never returns nil - no nil checks needed
- Unknown actions detected immediately during recipe loading
- Callers trust the Step is fully valid after construction

**Generalized variable detection** (no reflection needed):

```go
// Known interpolation variables that affect platform variance
var knownVars = []string{"linux_family", "os", "arch"}

// detectInterpolatedVars scans for {{var}} patterns in any string value.
// Returns a map of variable names found (e.g., {"linux_family": true}).
// Generalized to support future variables like {{arch}}.
func detectInterpolatedVars(v interface{}) map[string]bool {
    found := make(map[string]bool)
    detectVarsRecursive(v, found)
    return found
}

func detectVarsRecursive(v interface{}, found map[string]bool) {
    switch val := v.(type) {
    case string:
        for _, varName := range knownVars {
            if strings.Contains(val, "{{"+varName+"}}") {
                found[varName] = true
            }
        }
    case map[string]interface{}:
        for _, v := range val {
            detectVarsRecursive(v, found)
        }
    case []interface{}:
        for _, v := range val {
            detectVarsRecursive(v, found)
        }
    }
}
```

This pattern:
- Stores analysis on Step at load time (no runtime registry dependency)
- Uses minimal function type instead of broad interface
- Scans params via type switch (no reflection)
- Callers truly "just ask the step": `step.Analysis()`

### Phase 2: Extend WhenClause

Add `LinuxFamily` and `Arch` fields to allow explicit constraints:

```go
// In recipe/types.go
type WhenClause struct {
    Platform       []string `toml:"platform,omitempty"`
    OS             []string `toml:"os,omitempty"`
    Arch           string   `toml:"arch,omitempty"`          // NEW
    LinuxFamily    string   `toml:"linux_family,omitempty"`  // NEW
    PackageManager string   `toml:"package_manager,omitempty"`
}
```

**Upstream design reconciliation:** The original `DESIGN-system-dependency-actions.md` specified that `WhenClause` should remain generic (os, arch, platform only), with family constraints handled via implicit action constraints. This design extends `WhenClause` to allow explicit family constraints for non-PM actions.

**Rationale for the change:**
- PM actions (`apt_install`, `dnf_install`) have implicit family constraints
- Non-PM actions like `download` or `run` may need explicit family targeting
- Example: download different binaries per family without using `{{linux_family}}` interpolation

```toml
# Explicit family constraint on non-PM action
[[steps]]
action = "download"
url = "https://example.com/debian-specific-tool.tar.gz"
when.linux_family = "debian"
```

This is a deliberate extension of the upstream design to support the broader use case.

Note: `LinuxFamily` is singular (like `PackageManager`) since a step targets one family. Aggregation to multiple families happens at the recipe level via the `FamilyConstrained` policy.

### Phase 3: Recipe Family Policy

Name the five recipe types explicitly rather than computing them implicitly:

```go
// RecipeFamilyPolicy describes how a recipe relates to Linux families
type RecipeFamilyPolicy int

const (
    // FamilyDarwinOnly: No Linux-applicable steps exist.
    // Result: No Linux platforms at all
    FamilyDarwinOnly RecipeFamilyPolicy = iota

    // FamilyAgnostic: Has Linux steps, but no family constraints or variation.
    // Result: Generic Linux platforms (no family qualifier)
    FamilyAgnostic

    // FamilyVarying: At least one step uses {{linux_family}} interpolation.
    // Result: All families (each produces different output)
    FamilyVarying

    // FamilyConstrained: All Linux steps target specific families, no unconstrained steps.
    // Result: Only the families explicitly targeted
    FamilyConstrained

    // FamilyMixed: Has both family-constrained and unconstrained Linux steps.
    // Result: All families (some steps filtered per family)
    FamilyMixed
)

// String returns the policy name for debugging and logging.
func (p RecipeFamilyPolicy) String() string {
    switch p {
    case FamilyDarwinOnly:
        return "FamilyDarwinOnly"
    case FamilyAgnostic:
        return "FamilyAgnostic"
    case FamilyVarying:
        return "FamilyVarying"
    case FamilyConstrained:
        return "FamilyConstrained"
    case FamilyMixed:
        return "FamilyMixed"
    default:
        return fmt.Sprintf("RecipeFamilyPolicy(%d)", p)
    }
}

// MarshalText implements encoding.TextMarshaler for JSON serialization.
// Ensures `tsuku info --json` outputs "FamilyAgnostic" not 1.
func (p RecipeFamilyPolicy) MarshalText() ([]byte, error) {
    return []byte(p.String()), nil
}
```

### Phase 4: Metadata Aggregation

Compute recipe family policy, then derive platforms:

```go
// RecipeAnalysis contains the full analysis of a recipe's platform support.
// Returned by AnalyzeRecipe; used by SupportedPlatforms.
type RecipeAnalysis struct {
    Policy          RecipeFamilyPolicy
    FamiliesUsed    map[string]bool  // For FamilyConstrained/FamilyMixed
    SupportsDarwin  bool             // Derived from step analysis, not hardcoded
}

// AnalyzeRecipe computes the family policy and OS support for a recipe.
// Returns error if any step has conflicting constraints.
func AnalyzeRecipe(recipe *Recipe) (*RecipeAnalysis, error) {
    familiesUsed := make(map[string]bool)
    hasFamilyVaryingStep := false
    hasUnconstrainedLinuxSteps := false
    hasAnyLinuxSteps := false
    hasAnyDarwinSteps := false

    for _, step := range recipe.Steps {
        analysis := step.Analysis()  // Never nil - guaranteed by constructor

        // Track OS support from step constraints
        // Unconstrained (nil or empty OS) means both OSes
        // Explicit OS constraint means only that OS
        if analysis.Constraint == nil || analysis.Constraint.OS == "" {
            hasAnyLinuxSteps = true
            hasAnyDarwinSteps = true
        } else if analysis.Constraint.OS == "linux" {
            hasAnyLinuxSteps = true
        } else if analysis.Constraint.OS == "darwin" {
            hasAnyDarwinSteps = true
            continue  // Skip family analysis for darwin-only steps
        }

        // Family analysis (only for Linux-applicable steps)
        // Handle constrained+varying: interpolation within a family constraint
        if analysis.FamilyVarying {
            if analysis.Constraint != nil && analysis.Constraint.LinuxFamily != "" {
                // Constrained+varying: interpolation only happens within this family
                familiesUsed[analysis.Constraint.LinuxFamily] = true
            } else {
                // Unconstrained varying: needs all families
                hasFamilyVaryingStep = true
            }
        } else if analysis.Constraint != nil && analysis.Constraint.LinuxFamily != "" {
            familiesUsed[analysis.Constraint.LinuxFamily] = true
        } else if analysis.Constraint == nil || analysis.Constraint.OS == "" || analysis.Constraint.OS == "linux" {
            hasUnconstrainedLinuxSteps = true
        }
    }

    // Determine policy - no nil sentinel, explicit enum for each case
    var policy RecipeFamilyPolicy
    if !hasAnyLinuxSteps {
        policy = FamilyDarwinOnly
    } else if hasFamilyVaryingStep {
        policy = FamilyVarying
    } else if len(familiesUsed) == 0 {
        policy = FamilyAgnostic
    } else if hasUnconstrainedLinuxSteps {
        policy = FamilyMixed
    } else {
        policy = FamilyConstrained
    }

    return &RecipeAnalysis{
        Policy:         policy,
        FamiliesUsed:   familiesUsed,
        SupportsDarwin: hasAnyDarwinSteps,
    }, nil
}

// SupportedPlatforms returns all platforms the recipe supports.
func SupportedPlatforms(recipe *Recipe) ([]Platform, error) {
    analysis, err := AnalyzeRecipe(recipe)
    if err != nil {
        return nil, err
    }

    var platforms []Platform

    // Add darwin platforms only if recipe supports darwin (derived from analysis)
    if analysis.SupportsDarwin {
        platforms = append(platforms,
            Platform{OS: "darwin", Arch: "amd64"},
            Platform{OS: "darwin", Arch: "arm64"},
        )
    }

    // Add Linux platforms based on policy
    switch analysis.Policy {
    case FamilyDarwinOnly:
        // No Linux platforms - darwin-only recipe

    case FamilyAgnostic:
        // Generic Linux (no family qualifier)
        platforms = append(platforms,
            Platform{OS: "linux", Arch: "amd64"},
            Platform{OS: "linux", Arch: "arm64"},
        )

    case FamilyVarying, FamilyMixed:
        // All families needed
        for _, family := range AllLinuxFamilies {
            platforms = append(platforms,
                Platform{OS: "linux", Arch: "amd64", LinuxFamily: family},
                Platform{OS: "linux", Arch: "arm64", LinuxFamily: family},
            )
        }

    case FamilyConstrained:
        // Only specific families
        for family := range analysis.FamiliesUsed {
            platforms = append(platforms,
                Platform{OS: "linux", Arch: "amd64", LinuxFamily: family},
                Platform{OS: "linux", Arch: "arm64", LinuxFamily: family},
            )
        }
    }

    return platforms, nil
}
```

**Policy Examples:**

| Recipe Pattern | Policy | Darwin? | Linux Platforms |
|---------------|--------|---------|-----------------|
| Darwin-only steps (`when.os: darwin`) | FamilyDarwinOnly | Yes | None |
| Linux-only steps (`when.os: linux`) | FamilyAgnostic | No | Generic linux |
| `download` (no family var) | FamilyAgnostic | Yes | Generic linux |
| `download` with `{{linux_family}}` | FamilyVarying | Yes | All 5 families |
| `apt_install` only | FamilyConstrained | No | debian only |
| `apt_install` + `dnf_install` | FamilyConstrained | No | debian + rhel |
| `download` (plain) + `apt_install` | FamilyMixed | Yes | All 5 families |

The explicit policy enum makes the logic self-documenting - no nil sentinel values. Darwin support is now derived from step analysis rather than hardcoded.

**Precedence rationale:** `FamilyVarying` takes precedence over `FamilyConstrained` because interpolation creates distinct outputs for all families, regardless of any per-step family constraints. A recipe with both `{{linux_family}}` interpolation and `apt_install` still needs golden files for all 5 families - the apt_install step simply won't appear in non-debian plans.

Update `tsuku info --metadata-only --json` to include `supported_platforms`.

### Phase 4: Script Updates

1. Update `regenerate-golden.sh` to query metadata and generate appropriate files
2. Update `validate-golden.sh` to use metadata-driven validation
3. Add `--linux-family` flag handling to scripts for cross-platform simulation

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

**Recommended approach:** Use an interface to allow multiple types to satisfy matching:

```go
// Matchable provides platform dimensions for filtering.
// Both recipe.MatchTarget and platform.Target implement this.
type Matchable interface {
    GetOS() string
    GetArch() string
    GetLinuxFamily() string
}

func (w *WhenClause) Matches(target Matchable) bool
```

This pattern:
- Allows adding new dimensions (e.g., libc variant) by extending the interface
- Eliminates conversion between `platform.Target` and `recipe.MatchTarget`
- Both types implement the interface directly - no adapter methods needed

Since tsuku is pre-GA, this breaking change is acceptable.

**Implementation:**

```go
// In recipe package - lightweight struct for tests and simple cases
type MatchTarget struct {
    OS          string
    Arch        string
    LinuxFamily string
}

func (m MatchTarget) GetOS() string          { return m.OS }
func (m MatchTarget) GetArch() string        { return m.Arch }
func (m MatchTarget) GetLinuxFamily() string { return m.LinuxFamily }

// In platform package - Target already has these fields
func (t Target) GetOS() string          { return t.OS() }
func (t Target) GetArch() string        { return t.Arch() }
func (t Target) GetLinuxFamily() string { return t.LinuxFamily }
```

### Future Extensibility

Adding new constraint dimensions requires changes to:
1. `Constraint` type - add the new field
2. `StepAnalysis` type - add `*Varying bool` if the dimension supports interpolation
3. `WhenClause` type - add the new field
4. `Matchable` interface - add `Get*()` method for the new dimension
5. `mergeWhenClause()` - handle new dimension with conflict detection
6. `knownVars` slice - add the variable name (e.g., `"arch"`)
7. `AnalyzeRecipe()` / `SupportedPlatforms()` - expand platforms for new dimension
8. Golden file naming - incorporate the new dimension

**Interpolation variable detection is generalized:** The `detectInterpolatedVars()` function scans for all variables in `knownVars`. Adding a new interpolation variable (step 6) requires only adding the variable name to the slice - the scanning logic is already in place.

The step-level abstraction means most callers don't change - they call `step.Analysis()` and get a `StepAnalysis`. But adding a dimension requires coordinated changes across the constraint model.

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
- **Clean type separation**: `Constraint` (where can it run) vs `StepAnalysis` (constraint + variation) vs `RecipeFamilyPolicy` (aggregated behavior)
- **Load-time resolution**: Steps are self-describing after loading - no runtime registry dependency
- **Explicit policy names**: Four recipe types named, not computed implicitly
- **Conflict detection**: OS, arch, and family conflicts caught at load time with clear error messages
- **Future-proofed interfaces**: `MatchTarget` struct allows adding dimensions without signature changes

### Negative

- **Requires `WhenClause` extension**: Must add `linux_family` and `arch` fields to step conditions
- **Upstream design change**: Extends `WhenClause` beyond original generic scope
- **Mixed naming**: Directory contains both old-style and new-style filenames during transition
- **Extension not free**: Adding new constraint dimensions requires coordinated changes (see Future Extensibility)

### Mitigations

- **WhenClause extension**: Follows existing pattern. Documented as deliberate extension of upstream design.
- **Upstream reconciliation**: Rationale documented - non-PM actions need explicit family targeting.
- **Mixed naming**: Clear naming convention and documentation make the pattern understandable.
- **Extension cost**: Load-time resolution means most callers just call `step.Analysis()` - only the loader changes.
