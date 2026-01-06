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

This design separates three concerns with uniform interfaces at each boundary:

```
┌─────────────────────────────────────────────────────────────┐
│  Golden File Tooling (scripts, workflows)                   │
│  - Queries: "what platforms does this recipe support?"      │
│  - Generates one file per supported platform                │
│  - No knowledge of actions or how constraints work          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ tsuku info --metadata-only
┌─────────────────────────────────────────────────────────────┐
│  Recipe Metadata (tsuku info)                               │
│  - Aggregates constraints from all actions                  │
│  - Returns supported_platforms list                         │
│  - Hides action-level details from callers                  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ Action.ImplicitConstraint()
┌─────────────────────────────────────────────────────────────┐
│  Action Constraints                                         │
│  - Uniform interface: all actions implement same method     │
│  - Returns Constraint or nil                                │
│  - How each action determines its constraint is internal    │
└─────────────────────────────────────────────────────────────┘
```

**Key principle:** Each layer has a uniform interface. CLI users ask about recipes, not actions. Code querying actions uses the same method regardless of action type. Implementation details don't leak upward.

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

### Uniform Constraint Interface

**At the action level:** All actions expose constraints through the same interface (`ImplicitConstraint()`). The caller asks "what are your constraints?" and gets a `Constraint` back (or `nil`). The caller doesn't know or care *how* the action determined its constraints - whether it's hardcoded in the action type or derived from instance configuration is an internal implementation detail.

**At the recipe level:** CLI users don't think about actions at all. They query recipe metadata:

```bash
# User just asks: does this recipe have family-specific plans?
tsuku info myrecipe --metadata-only --json | jq '.supported_platforms'
```

The response tells them what platforms are supported. If `linux_family` appears in the platform objects, the recipe is family-aware. Users never interact with individual action constraints.

### Constraint Sources (Implementation Detail)

Internally, family awareness is aggregated from two sources:

1. **`Action.ImplicitConstraint()`** - Every action type implements this uniformly. Some return hardcoded constraints (e.g., `apt_install` returns debian), others analyze their instance configuration. The interface is identical.

2. **`WhenClause.LinuxFamily`** - Steps can have explicit `when.linux_family` conditions.

If any constraint specifies `LinuxFamilies`, the recipe is family-aware. This aggregation logic is internal to `tsuku info` - callers just see the final `supported_platforms` list.

**Note:** Variable interpolation scanning (e.g., detecting `{{linux_family}}` in URL strings) is explicitly excluded. It's too fragile and prone to false positives/negatives. Family awareness must be expressed through explicit constraints, not implicit string patterns.

### Valid Linux Families

The current set of supported families is: `debian`, `rhel`, `arch`, `alpine`, `suse`.

This list is an **extension point**. Adding a new family (e.g., `nixos`) requires:
1. Adding it to `ValidLinuxFamilies` constant
2. Implementing support in relevant `SystemAction` types
3. No interface changes required

Golden file tooling automatically picks up new families via metadata.

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

### Phase 1: Extend Constraint Type

Extend the existing `Constraint` type to support multiple families:

```go
// In internal/actions/system_action.go
type Constraint struct {
    OS            string
    LinuxFamilies []string  // nil = any family; populated = specific families
}
```

This reuses the existing `ImplicitConstraint()` method on `SystemAction`. Package manager actions return their family constraint:

```go
func (a *AptInstallAction) ImplicitConstraint() *Constraint {
    return &Constraint{OS: "linux", LinuxFamilies: []string{"debian"}}
}
```

### Phase 2: Extend WhenClause

Add `LinuxFamily` field to allow explicit family constraints in recipe steps:

```go
// In recipe/types.go
type WhenClause struct {
    Platform       []string `toml:"platform,omitempty"`
    OS             []string `toml:"os,omitempty"`
    LinuxFamily    []string `toml:"linux_family,omitempty"`  // NEW
    PackageManager string   `toml:"package_manager,omitempty"`
}
```

### Phase 3: Metadata Aggregation

Add recipe-level family awareness computation to `tsuku info`:

```go
func (e *Executor) SupportedPlatforms() []Platform {
    // Walk all steps
    // Collect constraints from SystemAction.ImplicitConstraint()
    // Collect constraints from step.When.LinuxFamily
    // If any constraint specifies LinuxFamilies, expand Linux platforms
    // Return platform list
}
```

Update `--metadata-only --json` output to include `supported_platforms`.

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

### Future Extensibility

The `Constraint` type can accommodate future targeting dimensions without interface changes:

```go
type Constraint struct {
    OS            string
    Arch          string      // could constrain to specific arches
    LinuxFamilies []string
    // Future dimensions:
    // LibcVariant   string   // glibc, musl
    // DarwinVersion string   // minimum macOS version
}
```

Golden file tooling automatically handles new dimensions via the metadata layer - no script changes required when new dimensions are added.

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
- **Reuses existing patterns**: Extends `Constraint` type and `ImplicitConstraint()` method rather than adding new interfaces
- **Future-proof**: `Constraint` type can accommodate new targeting dimensions without interface changes
- **Clean separation**: Golden file tooling is decoupled from constraint detection logic
- **Useful beyond golden files**: Other tooling can query family support (documentation, installers, CI)

### Negative

- **Requires `Constraint` extension**: Must extend existing type to support multiple families
- **Requires `WhenClause` extension**: Must add `linux_family` field to step conditions
- **Mixed naming**: Directory contains both old-style and new-style filenames during transition

### Mitigations

- **Constraint extension**: Simple field addition (`LinuxFamilies []string`) with clear semantics. Existing code continues to work.
- **WhenClause extension**: Follows existing pattern for `os` and `platform` fields. No new concepts introduced.
- **Mixed naming**: Clear naming convention and documentation make the pattern understandable.
