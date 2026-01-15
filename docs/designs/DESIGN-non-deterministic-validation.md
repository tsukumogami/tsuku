# DESIGN: Non-Deterministic Golden File Validation

## Status

Proposed

## Context and Problem Statement

Golden files for non-deterministic recipes (pip_exec, cargo_install, go_build, cpan_install, gem_install, npm_install, etc.) drift over time because dependency resolution happens at eval time and picks up new transitive dependency versions from upstream registries. This causes CI failures in `validate-golden-code.yml` that require manual regeneration, even when neither the recipe nor tsuku code has changed.

The plans are already marked `deterministic: false` at both the step and plan level, but the current validation performs exact byte-for-byte content comparison regardless of this flag. This creates a maintenance burden where the golden-code validation workflow had to be disabled entirely (see line 67: `if: false` in validate-golden-code.yml).

**Current state**: The `validate-golden-code.yml` workflow is disabled. Non-deterministic recipes like `ruff`, `black`, `httpie`, `poetry`, and `meson` have golden files that frequently become stale due to upstream dependency changes.

### Scope

**In scope:**
- Validation strategy for non-deterministic golden files
- Changes to validation scripts and CI workflows
- Documentation of which recipes/actions produce non-deterministic output

**Out of scope:**
- Making non-deterministic actions deterministic (covered by separate designs)
- Changes to the lockfile generation approach (already implemented in #609)
- Version provider changes

## Decision Drivers

1. **CI stability**: Validation should not fail due to upstream dependency changes unrelated to recipe or code changes
2. **Regression detection**: Changes to recipes or tsuku code that affect plan structure should still be caught
3. **Maintainability**: The solution should not require frequent manual intervention
4. **Execution confidence**: Non-deterministic plans should still be executable when validated
5. **Minimal complexity**: Prefer solutions that build on existing infrastructure

## Considered Options

### Option 1: Skip Validation for Non-Deterministic Plans

Read the `deterministic` flag from each golden file and skip comparison entirely for non-deterministic plans.

**How it works:**
- `validate-golden.sh` reads `jq '.deterministic'` from each golden file
- If `false`, skip the regenerate-and-compare step
- Only validate that the file exists and is valid JSON

**Pros:**
- Simple implementation
- No false positive failures
- Uses existing metadata
- JSON validity checks still catch file corruption
- Execution validation (`validate-golden-execution.yml`) provides defense-in-depth

**Cons:**
- No regression detection for non-deterministic recipes at plan-generation level
- Recipe changes that break non-deterministic plans go unnoticed until execution
- Structural changes (new fields, action ordering) are not validated

**When this option makes sense:**
- Early-stage development where execution validation coverage is strong
- When structural validation produces too many false positives due to high variance
- When the cost of maintaining schema definitions exceeds the benefit

### Option 2: Structural Validation (Schema Comparison)

Validate non-deterministic plans by comparing structure only: tool, version, action types, and step ordering. Ignore content that varies (checksums, locked_requirements, go_sum).

**How it works:**
- Define a "structural schema" extraction that keeps:
  - `tool`, `version`, `platform`, `format_version`
  - Step `action` types and ordering
  - Dependency structure (tool names, versions, action types)
- Ignore fields known to vary:
  - `checksum` values in download steps
  - `locked_requirements` in pip_exec
  - `go_sum` in go_build
  - `size` fields
- Compare extracted schemas instead of full content

**Pros:**
- Catches structural regressions (action ordering, missing steps, wrong dependencies)
- Tolerates expected variation in non-deterministic content
- Recipe changes that affect structure are detected

**Cons:**
- Requires defining and maintaining the "varying fields" list
- May miss subtle content changes that matter
- More complex than simple skip
- New non-deterministic fields need to be added to the ignore list

### Option 3: Automated Regeneration with Diff Review

Automatically regenerate non-deterministic golden files in CI and create a PR if they differ, rather than failing the build.

**How it works:**
- `validate-golden-code.yml` regenerates all golden files
- If non-deterministic files differ, create an automated PR with the updates
- Deterministic file differences still fail the build
- Human reviews the automated PR before merge

**Pros:**
- No manual regeneration burden
- Changes are visible in PR for review
- CI never fails due to upstream drift

**Cons:**
- Requires bot/automation infrastructure for PR creation
- Review burden shifts to approving automated PRs
- Merge conflicts if multiple PRs touch golden files
- Does not distinguish intentional changes from drift

### Option 4: Freeze Lockfiles at Authoring Time

Require non-deterministic recipes to include frozen lockfiles in the recipe itself, making them deterministic at eval time.

**How it works:**
- Recipes with `pip_exec`, `cargo_install`, etc. must include the full lockfile content
- Eval phase reads lockfile from recipe instead of resolving from registry
- Golden files become deterministic
- Version bumps require updating the recipe's embedded lockfile

**Pros:**
- Golden files become fully deterministic
- No validation strategy changes needed
- Execution uses exact same dependencies as golden file
- Well-established pattern: Cargo uses `Cargo.lock` with `--locked --offline`, Go uses `go.sum` with MVS
- Many production systems already commit lockfiles as standard practice

**Cons:**
- Significant recipe authoring burden for pip/npm where lockfiles are verbose
- Lockfiles can be large (hundreds of lines for complex deps)
- Recipe format changes required
- Does not help with build actions (go_build, cargo_build) where output varies due to compiler
- Requires tooling to update lockfiles during version bumps

**When this option makes sense:**
- Registry dominated by Cargo/Go recipes where lockfile freezing is well-supported
- When reproducibility is paramount and the authoring cost is acceptable
- For recipes with minimal transitive dependencies where lockfiles are small

### Option 5: Tiered Validation by Determinism

Apply different validation strategies based on the plan's determinism status:
- Deterministic plans: exact comparison (current behavior)
- Non-deterministic plans: structural validation (Option 2)

**How it works:**
- Validation script checks `deterministic` flag first
- Routes to appropriate comparison function
- Single workflow handles both cases
- Structural validation uses jq to extract comparable fields

**Pros:**
- Best of both worlds: strict for deterministic, flexible for non-deterministic
- Single unified workflow
- Leverages existing `deterministic` flag
- Maintains regression detection for both types

**Cons:**
- Two validation paths to maintain
- Need to keep structural schema definition updated
- Slightly more complex than pure skip

### Option 6: Semantic Diff with Equivalence Classes

Parse lockfile formats natively (Cargo.lock TOML, pip requirements) and compare package sets semantically rather than as strings. Only flag changes that are semantically significant: new packages added, packages removed, or major version changes.

**How it works:**
- For pip: parse `locked_requirements` to extract package names and versions, compute digest
- For cargo: parse `Cargo.lock` semantically, extract crate names and versions
- For go: parse `go.sum` to extract module paths and versions
- Compare semantic fingerprints rather than raw content
- Only fail on semantic drift (new deps, removed deps, major version changes)

**Pros:**
- Catches real issues (dependency changes) while ignoring expected drift (hash updates)
- More precise than pure structural validation
- Maintains security visibility into dependency changes

**Cons:**
- Requires parsing multiple lockfile formats (pip requirements, Cargo.lock, go.sum, package-lock.json)
- Additional complexity per ecosystem
- May need ecosystem-specific "significant change" definitions

### Option 7: Split Plan Format

Separate plans into distinct sections: `immutable` (recipe-derived, exactly validated) and `mutable` (resolved dependencies, structurally validated). The plan format itself encodes validation intent.

**How it works:**
- Plan JSON gains `immutable` and `mutable` top-level sections
- Immutable section: tool, version, platform, action types, ordering
- Mutable section: checksums, lockfile content, sizes
- Validation applies exact comparison to immutable, structural to mutable

**Pros:**
- Architecturally clean, makes validation intent explicit in the format
- No heuristics about what varies - it's declared in the schema
- Self-documenting plans

**Cons:**
- Requires plan format version bump
- All golden files need regeneration
- Higher implementation effort
- Adds complexity to plan generation code

### Option 8: Snapshot Testing with Time-Boxing

Accept that golden files have limited validity and introduce a freshness model. Fresh files use exact match; older files use structural match; expired files trigger automatic regeneration.

**How it works:**
- Golden files include a `valid_until` timestamp (e.g., 30 days from generation)
- Fresh files (within validity): exact comparison catches immediate regressions
- Stale files (past validity): structural comparison tolerates expected drift
- Expired files (far past validity): automatic regeneration with review

**Pros:**
- Pragmatic approach that matches reality (dependencies do drift)
- Good developer ergonomics - recent changes are strictly validated
- Automatic freshness without constant manual regeneration

**Cons:**
- Time-based logic can be confusing to debug
- CI behavior changes based on when it runs
- Requires infrastructure for "time since generation" tracking

## Decision Outcome

**Chosen: Option 5 - Tiered Validation by Determinism (with enhanced structural schema)**

### Summary

Apply different validation strategies based on the plan's determinism status. Deterministic plans use exact byte-for-byte comparison. Non-deterministic plans use structural validation that compares tool identity, action types, step ordering, structurally significant params, and dependency structure while ignoring fields known to vary (checksums, lockfile content text, size).

### Rationale

Option 5 provides the best balance between CI stability and regression detection:

1. **Maintains regression detection**: Unlike Option 1 (skip), structural validation still catches recipe bugs that change action ordering, add/remove steps, or break dependencies.

2. **Tolerates expected drift**: Unlike exact comparison, structural validation ignores checksums and lockfile content that vary due to upstream changes.

3. **Builds on existing infrastructure**: Uses the `deterministic` flag already computed during plan generation, requiring no recipe format changes (unlike Option 4).

4. **Avoids automation complexity**: Unlike Option 3, does not require bot infrastructure for PR creation or review of automated changes.

5. **Lower implementation cost than alternatives**: Option 6 (semantic diff) requires parsing multiple lockfile formats. Option 7 (split format) requires a plan format change and full regeneration. Option 8 (time-boxing) introduces time-based complexity.

The main trade-off is maintaining two validation paths, but this complexity is localized to the validation scripts and uses a well-defined schema extraction.

### Why Not Other Options

- **Option 1 (Skip)**: Viable for early-stage, but we have sufficient coverage to benefit from structural validation.
- **Option 2 (Structural only)**: Would lose exact comparison for deterministic plans.
- **Option 3 (Auto-regen PRs)**: Infrastructure complexity for marginal benefit.
- **Option 4 (Freeze lockfiles)**: Good for Cargo/Go-heavy registries, but tsuku has diverse ecosystem coverage and the authoring burden is high.
- **Option 6 (Semantic diff)**: More precise but higher implementation cost; consider as future enhancement.
- **Option 7 (Split format)**: Architecturally cleaner but requires format change and full regeneration.
- **Option 8 (Time-boxing)**: Pragmatic but introduces time-based complexity.

## Solution Architecture

### Structural Schema Definition

The structural schema extracts comparable elements while filtering out varying content. Critically, it includes **structurally significant params** that identify what is being installed, while ignoring content that varies (checksums, lockfile text).

```
Structural Schema:
├── format_version (required to match)
├── tool (required to match)
├── version (required to match)
├── platform.os (required to match)
├── platform.arch (required to match)
├── deterministic (metadata, not compared)
├── recipe_hash (required to match - same recipe should produce same structure)
├── dependencies[] (recursive structural schema)
│   ├── tool
│   ├── version
│   ├── recipe_hash
│   └── steps[] (structural + significant params)
└── steps[] (structural + significant params)
    ├── action
    ├── evaluable
    ├── deterministic
    └── significant_params (per action type - see below)

Significant Params by Action Type:
├── pip_exec: package, version, executables, python_version
├── go_build: module, install_module, version, executables, go_version
├── cargo_install/cargo_build: crate, version, executables
├── npm_install/npm_exec: package, version, executables
├── gem_install/gem_exec: gem, version, executables
├── cpan_install: distribution, executables
├── download_file: dest (not checksum)
├── extract: archive, format, strip_dirs
├── install_binaries: binaries, install_mode

Ignored Fields (vary for non-deterministic plans):
├── steps[].params.checksum
├── steps[].params.checksum_algo
├── steps[].params.locked_requirements (full text)
├── steps[].params.go_sum (full text)
├── steps[].params.has_native_addons
├── steps[].url
├── steps[].checksum
├── steps[].size
└── dependencies[].steps[] (same rules recursively)

Lockfile Fingerprint (for semantic change detection):
For pip_exec: extract package names + versions from locked_requirements, compute digest
For go_build: extract module paths + versions from go_sum, compute digest
This fingerprint IS included in structural comparison to detect semantic dep changes.
```

**Why significant params matter:**

Without including significant params, the schema would match two plans that:
- Install different packages (e.g., `package: "black"` vs `package: "ruff"`)
- Pull from different modules (e.g., `module: "github.com/legit/tool"` vs `module: "github.com/attacker/tool"`)
- Produce different executables

This would be a security issue. The significant params ensure we catch changes to **what** is installed while tolerating changes to **how it resolves** (checksums, lockfile text).

### Validation Flow

**Important clarification**: Structural validation exists for **CI stability**, not security. The security backstop is execution-time checksum verification, which remains unchanged. Structural validation catches regressions in plan generation while tolerating expected drift in non-deterministic content.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        validate-golden.sh <recipe>                          │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  For each golden file:        │
                    │  1. Regenerate to temp file   │
                    │  2. Read deterministic flags  │
                    │     from BOTH golden & actual │
                    └───────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Determinism transition?      │
                    │  (golden.det != actual.det)   │
                    └───────────────────────────────┘
                            │               │
                          Yes               No
                            │               │
                            ▼               ▼
                ┌───────────────┐   ┌───────────────────────────────┐
                │ FAIL: Require │   │ Route by deterministic flag   │
                │ human review  │   └───────────────────────────────┘
                └───────────────┘               │
                                    ┌──────────┴──────────┐
                                    │                     │
                            deterministic: true    deterministic: false
                                    │                     │
                                    ▼                     ▼
                        ┌───────────────────┐   ┌───────────────────────┐
                        │ Exact Comparison  │   │ Structural Comparison │
                        │ (byte-for-byte)   │   │ (schema extraction)   │
                        └───────────────────┘   └───────────────────────┘
                                    │                     │
                                    └──────────┬──────────┘
                                               ▼
                                    ┌───────────────────────────────┐
                                    │  Exit codes:                  │
                                    │  0 = match                    │
                                    │  1 = mismatch (show diff)     │
                                    │  2 = error (parse failure)    │
                                    └───────────────────────────────┘
```

### Schema Extraction Script

Create `scripts/extract-structural-schema.sh` to extract the comparable schema with significant params:

```bash
#!/bin/bash
# scripts/extract-structural-schema.sh
# Extracts structural schema from a golden plan file for comparison
# Usage: ./scripts/extract-structural-schema.sh <plan.json>

jq '
# Known action types - fail if we encounter unknown actions
def known_actions: [
  "pip_exec", "pip_install", "go_build", "go_install",
  "cargo_install", "cargo_build", "npm_install", "npm_exec",
  "gem_install", "gem_exec", "cpan_install",
  "download_file", "extract", "install_binaries", "chmod",
  "set_env", "set_rpath", "link_dependencies", "text_replace", "apply_patch",
  "configure_make", "cmake_build", "meson_build"
];

# Extract significant params based on action type
# MVP scope: no lockfile fingerprinting (can be added later)
def extract_significant_params:
  if .action == "pip_exec" then
    { package: .params.package, version: .params.version,
      executables: .params.executables, python_version: .params.python_version }
  elif .action == "go_build" then
    { module: .params.module, install_module: .params.install_module,
      version: .params.version, executables: .params.executables,
      go_version: .params.go_version }
  elif .action == "cargo_install" or .action == "cargo_build" then
    { crate: .params.crate, version: .params.version, executables: .params.executables }
  elif .action == "npm_install" or .action == "npm_exec" then
    { package: .params.package, version: .params.version, executables: .params.executables }
  elif .action == "gem_install" or .action == "gem_exec" then
    { gem: .params.gem, version: .params.version, executables: .params.executables }
  elif .action == "cpan_install" then
    { distribution: .params.distribution, executables: .params.executables }
  elif .action == "download_file" then
    { dest: .params.dest }
  elif .action == "extract" then
    { archive: .params.archive, format: .params.format, strip_dirs: .params.strip_dirs }
  elif .action == "install_binaries" then
    { binaries: .params.binaries, install_mode: .params.install_mode }
  elif .action == "chmod" then
    { files: .params.files }
  elif (.action | IN(known_actions[])) then
    # Known action but no security-critical params
    {}
  else
    # Unknown action type - halt extraction to prevent silent security degradation
    error("Unknown action type: \(.action) - update extract-structural-schema.sh")
  end;

def extract_step_schema:
  {
    action: .action,
    evaluable: .evaluable,
    deterministic: .deterministic,
    significant_params: extract_significant_params
  };

def extract_dep_schema:
  {
    tool: .tool,
    version: .version,
    recipe_hash: .recipe_hash,
    steps: [.steps[] | extract_step_schema]
  };

{
  format_version: .format_version,
  tool: .tool,
  version: .version,
  platform: .platform,
  recipe_hash: .recipe_hash,
  dependencies: [.dependencies[]? | extract_dep_schema],
  steps: [.steps[] | extract_step_schema]
}
' "$1"
```

The `lockfile_fingerprint` and `gosum_fingerprint` fields extract package/module names and versions while ignoring exact hashes, providing semantic change detection without exact string matching.

### Modified Validation Script

Update `scripts/validate-golden.sh` to use tiered validation:

```bash
# In validate-golden.sh, after regenerating to $ACTUAL:

# Check determinism flag
IS_DETERMINISTIC=$(jq -r '.deterministic' "$GOLDEN")

if [[ "$IS_DETERMINISTIC" == "true" ]]; then
    # Exact comparison for deterministic plans
    GOLDEN_HASH=$(sha256sum "$GOLDEN" | cut -d' ' -f1)
    ACTUAL_HASH=$(sha256sum "$ACTUAL" | cut -d' ' -f1)

    if [[ "$GOLDEN_HASH" != "$ACTUAL_HASH" ]]; then
        MISMATCH=1
        echo "MISMATCH (exact): $GOLDEN"
        diff -u "$GOLDEN" "$ACTUAL" || true
    fi
else
    # Structural comparison for non-deterministic plans
    GOLDEN_SCHEMA=$(./scripts/extract-structural-schema.sh "$GOLDEN")
    ACTUAL_SCHEMA=$(./scripts/extract-structural-schema.sh "$ACTUAL")

    if [[ "$GOLDEN_SCHEMA" != "$ACTUAL_SCHEMA" ]]; then
        MISMATCH=1
        echo "MISMATCH (structural): $GOLDEN"
        diff -u <(echo "$GOLDEN_SCHEMA" | jq -S .) <(echo "$ACTUAL_SCHEMA" | jq -S .) || true
    fi
fi
```

### Handling Determinism Transitions

When a recipe changes between deterministic and non-deterministic (e.g., adding a pip_exec step), the validation script must handle this correctly:

```bash
# In validate-golden.sh, check for determinism transition FIRST

GOLDEN_DET=$(jq -r '.deterministic' "$GOLDEN")
ACTUAL_DET=$(jq -r '.deterministic' "$ACTUAL")

if [[ "$GOLDEN_DET" != "$ACTUAL_DET" ]]; then
    echo "DETERMINISM CHANGE: $GOLDEN"
    echo "  Golden: deterministic=$GOLDEN_DET"
    echo "  Actual: deterministic=$ACTUAL_DET"
    echo "  This change requires review."
    MISMATCH=1
    # Show full diff for review
    diff -u "$GOLDEN" "$ACTUAL" || true
    continue
fi
```

Determinism transitions are flagged for explicit review because:
- A non-deterministic → deterministic change may indicate the recipe now produces stable output (good)
- A deterministic → non-deterministic change may indicate a regression or intentional recipe change (needs review)

### Validation Mode Override

For cases where a non-deterministic plan should have exact comparison (e.g., maintainer intentionally froze the lockfile), add a `validation_mode` field:

```json
{
  "format_version": 3,
  "tool": "black",
  "deterministic": false,
  "validation_mode": "exact",  // Override: force exact comparison
  ...
}
```

The validation script respects this override:
```bash
VALIDATION_MODE=$(jq -r '.validation_mode // "auto"' "$GOLDEN")
if [[ "$VALIDATION_MODE" == "exact" ]]; then
    # Use exact comparison regardless of deterministic flag
    ...
elif [[ "$VALIDATION_MODE" == "structural" ]]; then
    # Use structural comparison regardless of deterministic flag
    ...
else
    # Auto: use deterministic flag to decide
    ...
fi
```

This is an optional enhancement - the MVP can launch without it.

### CI Workflow Re-enablement

After implementing tiered validation, re-enable `validate-golden-code.yml`:

```yaml
# Remove the `if: false` condition from line 67
jobs:
  validate-all:
    # if: false  # REMOVE THIS LINE
    name: Validate All Golden Files
    runs-on: ubuntu-latest
```

## Implementation Approach

### MVP Scope Decision

The design supports two scopes:

**MVP (Recommended)**: Structural comparison WITHOUT lockfile fingerprinting
- Catches: action ordering, step injection, wrong package/module/version, dependency changes
- Simpler implementation, faster iteration
- Fingerprinting can be added later based on real-world experience

**Full Scope**: Structural comparison WITH lockfile fingerprinting
- Additional catches: transitive dependency changes via fingerprint
- More complex jq parsing, may need Go migration later

Start with MVP scope and add fingerprinting if false negatives become problematic.

### Phase 1: Schema Extraction Script

1. Create `scripts/extract-structural-schema.sh` with MVP scope (no fingerprinting)
2. Add unknown action type detection - fail if action type not in significant params list
3. Create test fixtures in `testdata/schema-extraction/` with expected outputs
4. Verify script handles edge cases (empty dependencies, nested deps)

### Phase 2: Modify Validation Script

1. Update `scripts/validate-golden.sh` with tiered comparison logic
2. Add determinism transition detection (flag change = require review)
3. Implement error handling with exit codes: 0=match, 1=mismatch, 2=error
4. Update `scripts/validate-all-golden.sh` to report deterministic vs structural mismatches separately
5. Test locally with both deterministic and non-deterministic recipes

### Phase 3: Dry-Run CI Validation

1. Re-enable `validate-golden-code.yml` in NON-BLOCKING mode
2. Run new validation for 1-2 weeks, collecting false positive/negative metrics
3. Tune structural schema based on observed failures
4. Document any false positives as exclusions or schema adjustments

### Phase 4: Full CI Enforcement

1. Switch `validate-golden-code.yml` to blocking mode (fail build on mismatch)
2. Monitor CI for a few PRs to confirm stability
3. Address any remaining false positives

### Phase 5: Documentation

1. Update CONTRIBUTING.md to explain the two validation modes
2. Document which actions produce non-deterministic output
3. Add troubleshooting guide for validation failures
4. Clarify that structural validation is for CI stability, not security (execution validates checksums)

## Security Considerations

### Download Verification

**Not affected.** This design changes validation comparison logic only. Actual download verification using checksums remains unchanged during execution. The structural schema still validates that plans have the expected structure for download steps.

### Execution Isolation

**Not affected.** Validation changes do not affect how plans are executed. The `validate-golden-execution.yml` workflow continues to run `tsuku install --plan` which performs full checksum verification.

### Supply Chain Risks

**Minimal impact.** Structural validation ignores checksum values and lockfile text for non-deterministic plans during CI comparison. However:

- **Checksums validated at execution**: Checksums are still verified during actual installation
- **Deterministic plans retain exact comparison**: Binary downloads have full checksum validation
- **Recipe hash comparison**: Ensures recipe logic hasn't changed
- **Significant params validated**: Package names, module paths, and executables are compared exactly
- **Lockfile fingerprints**: Semantic changes (new packages added, packages removed) are detected via fingerprint comparison
- **Step injection detection**: Unexpected steps or dependencies are caught by structural comparison

The trade-off is accepting that checksum values and hash strings within lockfiles may change between golden file generation and execution. This is inherent to non-deterministic builds and acceptable because:
1. Execution still verifies checksums
2. Semantic changes (different packages) are detected via fingerprints
3. Package identity (names, versions) is validated structurally

**What IS caught:**
- Different package installed (e.g., `black` vs `ruff`)
- Different module source (e.g., `github.com/legit/tool` vs `github.com/attacker/tool`)
- Different version installed
- New transitive dependency added (via fingerprint change)

**What is NOT caught during CI (caught at execution):**
- Same package with different checksum (e.g., republished tarball)

### User Data Exposure

**Not applicable.** Golden file validation operates only on plan JSON files in the repository. No user data is accessed or transmitted.

## Consequences

### Positive

- **CI stability restored**: `validate-golden-code.yml` can be re-enabled without frequent false failures
- **Regression detection maintained**: Structural changes to non-deterministic plans are still caught
- **Security maintained**: Significant params (package names, module paths) are validated; lockfile fingerprints detect semantic changes
- **No recipe format changes**: Existing recipes work without modification
- **Minimal infrastructure**: No bot/automation for PR creation needed
- **Clear separation**: Deterministic plans retain exact validation guarantee
- **Determinism transition detection**: Changes between deterministic/non-deterministic are flagged for review

### Negative

- **Two validation paths**: Slightly more complex validation logic to maintain
- **Schema maintenance**: New actions require adding their significant params to the extraction script
- **Lockfile fingerprint complexity**: jq-based parsing of lockfile formats may be fragile for edge cases
- **Reduced coverage for non-deterministic**: Exact checksum changes not detected until execution

### Mitigations

- **Schema maintenance**: Document significant params per action type; review during action additions
- **Two paths**: Both paths share most logic; only the comparison function differs
- **Lockfile parsing**: Keep fingerprint extraction simple (package names only); fall back to structural-only on parse errors
- **Reduced coverage**: Execution validation (`validate-golden-execution.yml`) provides defense-in-depth

### Future Enhancements

If the jq-based fingerprinting proves insufficient, consider:
- **Option 6 (Semantic Diff)**: Full lockfile parsing in Go for more robust semantic comparison
- **Validation mode override**: Allow recipes to opt into exact comparison when lockfiles are intentionally frozen
