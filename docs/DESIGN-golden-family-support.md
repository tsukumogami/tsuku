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

**Note:** Static analysis of recipe content (checking for package manager actions) was considered but rejected because it would duplicate plan generation logic. Running eval is more reliable and reuses existing code.

#### Option 1C: Recipe Metadata Declaration

Add a `linux_family_aware: true` field to recipe metadata.

**Pros:**
- Explicit declaration (no guessing)
- Fast (no need to generate plans to detect)
- Recipe author decides

**Cons:**
- Schema change required
- Manual maintenance burden
- Authors may forget to set it or set it incorrectly

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

**Chosen: 1B + 2B + 3B + 4C**

### Summary

Auto-detect family variation by comparing plans across families during generation. Use optional family component in filenames (`{version}-{os}-{family}-{arch}.json` when needed). Use debian as canonical family for generation. Validate using a hybrid approach that enforces base coverage while validating any family-specific files that exist.

### Rationale

**Why 1B (auto-detect) over 1A (always) or 1C (metadata):**
- 1A creates 5x waste for most recipes, making diffs noisy and storage larger
- 1C requires schema changes and manual maintenance
- 1B is fully automatic: the presence of family-specific files self-documents that the recipe varies by family
- Detection cost is acceptable: we already run eval for validation, running it for 5 families is 5x more work but only for Linux and only during generation

**Why 2B (optional component) over 2A (always family) or 2C (directories):**
- 2A would require migrating all existing golden files and changing tooling
- 2C is a significant structural change with complex directory handling
- 2B is backwards compatible: existing `linux-amd64` files work unchanged
- The naming difference (`linux-amd64` vs `linux-debian-amd64`) clearly signals whether the recipe varies by family

**Why 3B (debian canonical) over 3A (no field) or 3C (no flag):**
- 3A requires conditional logic in plan generation
- 3C means validation generates plans differently than generation, risking mismatches
- 3B is simple: always generate with `--linux-family debian` for Linux, then compare plans across families
- Debian is the most common target (Ubuntu, Debian, Linux Mint are all debian-family)

**Why 4C (hybrid) over 4A (what exists) or 4B (metadata only):**
- 4A doesn't catch missing coverage (if generation forgot a platform, validation wouldn't notice)
- 4B doesn't know about family awareness
- 4C enforces base coverage (os+arch) while allowing family files to exist and be validated
- During transition, this handles both old-style and new-style files

**Base coverage definition:** For family-aware recipes, all 5 family files must exist (validated by checking for the presence of `linux-{family}-amd64.json` files). The decision to store family-specific files is made during generation; once made, validation expects all families.

**Platform content:** Non-family-aware golden files (`linux-amd64.json`) contain `linux_family: "debian"` in the platform object since they're generated with `--linux-family debian`. This is arbitrary but deterministic; the recipe produces identical plans regardless of family, so the stored family value doesn't affect correctness.

## Solution Architecture

### File Naming

**Non-family-aware recipes** (plans identical across Linux families):
```
testdata/golden/plans/f/fzf/
├── v0.46.0-linux-amd64.json      # Generated with --linux-family debian
├── v0.46.0-darwin-amd64.json
└── v0.46.0-darwin-arm64.json
```

**Family-aware recipes** (plans differ by Linux family):
```
testdata/golden/plans/b/build-tools-system/
├── v1.0.0-linux-debian-amd64.json    # apt_install steps
├── v1.0.0-linux-rhel-amd64.json      # dnf_install steps
├── v1.0.0-linux-arch-amd64.json      # pacman_install steps
├── v1.0.0-linux-alpine-amd64.json    # apk_install steps
├── v1.0.0-linux-suse-amd64.json      # zypper_install steps
├── v1.0.0-darwin-amd64.json
└── v1.0.0-darwin-arm64.json
```

### Platform Exclusions

Per [DESIGN-golden-plan-testing.md](DESIGN-golden-plan-testing.md), linux-arm64 is excluded from golden file generation and validation because GitHub Actions does not provide arm64 Linux runners. This design inherits that exclusion: family-specific files are only generated for linux-amd64.

### Family Variation Detection

During golden file generation, detect whether a recipe's plans vary by Linux family:

```bash
# Generate plans for all 5 families
for family in debian rhel arch alpine suse; do
    tsuku eval --recipe "$RECIPE" --os linux --arch amd64 \
        --linux-family "$family" --version "$VERSION" \
        | jq -S 'del(.generated_at, .recipe_source)' > "/tmp/plan-$family.json"
done

# Compare plans (ignoring linux_family field in platform, using sorted JSON)
VARIES=false
REFERENCE=$(jq -S 'del(.platform.linux_family)' /tmp/plan-debian.json)
for family in rhel arch alpine suse; do
    CURRENT=$(jq -S 'del(.platform.linux_family)' /tmp/plan-$family.json)
    if [ "$REFERENCE" != "$CURRENT" ]; then
        VARIES=true
        break
    fi
done
```

**Note:** The `-S` flag ensures sorted JSON output for reliable string comparison.

If `VARIES=true`, store all 5 family files. Otherwise, store only `linux-amd64.json` (generated with debian family).

### Generation Workflow Changes

Update `.github/workflows/generate-golden-files.yml`:

1. **Linux jobs**: Generate plans for all 5 families to detect variation
2. **Detection step**: Compare plans to determine if family-specific files are needed
3. **Artifact upload**: Upload either single `linux-amd64.json` or 5 family files

```yaml
strategy:
  matrix:
    platform:
      # Linux runs on ubuntu with all families
      - { runner: ubuntu-latest, os: linux, arch: amd64, families: "debian,rhel,arch,alpine,suse" }
      # macOS variants (no family concept)
      - { runner: macos-14, os: darwin, arch: arm64, families: "" }
      - { runner: macos-15-intel, os: darwin, arch: amd64, families: "" }

steps:
  - name: Generate and detect family variation
    run: |
      if [[ -n "${{ matrix.platform.families }}" ]]; then
        # Linux: generate all families, detect variation
        ./scripts/regenerate-golden.sh "${{ inputs.recipe }}" \
          --os linux --arch amd64 --detect-family-variation
      else
        # macOS: single file
        ./scripts/regenerate-golden.sh "${{ inputs.recipe }}" \
          --os "${{ matrix.platform.os }}" --arch "${{ matrix.platform.arch }}"
      fi
```

### Script Changes

**regenerate-golden.sh**:
- Add `--linux-family <family>` flag (already done in #819, reverted)
- Add `--detect-family-variation` flag for auto-detection mode
- When detecting: generate all 5 families, compare, output appropriate files

**validate-golden.sh**:
- Parse filenames to detect family component
- For non-family files (`linux-amd64.json`): validate with `--linux-family debian`
- For family files (`linux-debian-amd64.json`): validate with matching family
- Enforce base platform coverage from recipe metadata

### Validation Logic

Filename parsing extracts platform components from the end of the filename to handle version strings that may contain hyphens (e.g., `v1.0.0-rc.1`):

```bash
# Parse golden filenames to extract platform components
# Parse from the end: arch is last, then family (if 4 parts after version), then os
for golden_file in "$GOLDEN_DIR"/*.json; do
    filename=$(basename "$golden_file" .json)

    # Known architectures and families for reliable parsing
    FAMILIES="debian|rhel|arch|alpine|suse"

    # Check if filename ends with family-arch pattern
    if [[ "$filename" =~ -($FAMILIES)-(amd64|arm64)$ ]]; then
        # Family-specific: v1.0.0-rc.1-linux-debian-amd64
        family="${BASH_REMATCH[1]}"
        arch="${BASH_REMATCH[2]}"
        # Strip -family-arch suffix, then extract os from remaining
        remainder="${filename%-$family-$arch}"
        os="${remainder##*-}"
        version="${remainder%-$os}"
    elif [[ "$filename" =~ -(linux|darwin)-(amd64|arm64)$ ]]; then
        # Non-family: v1.0.0-rc.1-linux-amd64
        os="${BASH_REMATCH[1]}"
        arch="${BASH_REMATCH[2]}"
        version="${filename%-$os-$arch}"
        family=""
    else
        echo "Cannot parse filename: $filename" >&2
        continue
    fi

    # Generate plan with appropriate flags
    eval_args=(--recipe "$RECIPE" --os "$os" --arch "$arch" --version "${version#v}")
    if [[ -n "$family" ]]; then
        eval_args+=(--linux-family "$family")
    elif [[ "$os" == "linux" ]]; then
        eval_args+=(--linux-family debian)  # Canonical family
    fi

    tsuku eval "${eval_args[@]}" | jq -S 'del(.generated_at, .recipe_source)' > /tmp/actual.json
    # Compare with sorted JSON...
done
```

### Migration Path

1. **Phase 1**: Update scripts to support family detection and optional family naming
2. **Phase 2**: Regenerate golden files for recipes with system dependencies (creates family-specific files where needed)
3. **Phase 3**: Update CI workflows to use new generation logic
4. **Phase 4**: Validate all recipes pass with new system

Existing golden files remain valid:
- `linux-amd64.json` files are validated with `--linux-family debian`
- No immediate migration required for non-family-aware recipes

### Recipe Transition Handling

When a recipe changes from non-family-aware to family-aware (e.g., adding `apt_install` action):

1. **Generation** detects the variation and creates 5 family files
2. **The old `linux-amd64.json` is deleted** (generation replaces with family-specific files)
3. **PR shows the transition** as deletion of one file and addition of 5 files

When a recipe changes from family-aware to non-family-aware (rare):

1. **Generation** detects no variation and creates single `linux-amd64.json`
2. **Old family files are deleted** by the regeneration script
3. **PR shows the transition** as deletion of 5 files and addition of one file

## Implementation Approach

### Phase 1: Script Updates

1. Restore `--linux-family` flag to `regenerate-golden.sh` (from #819)
2. Add `--detect-family-variation` flag for auto-detection
3. Update `validate-golden.sh` to parse family from filename and validate accordingly
4. Add helper function for plan comparison (ignoring linux_family field)

### Phase 2: Workflow Updates

1. Update `generate-golden-files.yml` to run family detection on Linux
2. Update artifact naming to handle family-specific files
3. Ensure merge step handles both file patterns

### Phase 3: Documentation

1. Update CONTRIBUTING.md with family-aware golden file guidance
2. Add examples for family-aware vs non-family-aware recipes
3. Document the detection algorithm

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
- **Automatic**: No manual metadata or flags required

### Negative

- **Generation complexity**: Detection requires running eval 5x for Linux
- **CI time**: Family detection adds time for Linux generation
- **Mixed naming**: Directory contains both old-style and new-style filenames during transition

### Mitigations

- **Generation complexity**: Detection only runs during golden file regeneration, not validation. Cache generated plans to avoid redundant downloads.
- **CI time**: Detection can run in parallel for all 5 families. The additional time is acceptable given the correctness benefit.
- **Mixed naming**: Clear naming convention and documentation make the pattern understandable.
