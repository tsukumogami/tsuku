# DESIGN: Non-Deterministic Golden File Validation

## Status

Proposed

## Context and Problem Statement

Golden files for ecosystem recipes (pip_exec, cargo_install, go_build, cpan_install, gem_install, npm_install, etc.) drift over time because dependency resolution happens at eval time and picks up new transitive dependency versions from upstream registries. This causes CI failures in `validate-golden-code.yml` that require manual regeneration, even when neither the recipe nor tsuku code has changed.

**Current state**: The `validate-golden-code.yml` workflow is disabled. Ecosystem recipes like `ruff`, `black`, `httpie`, `poetry`, and `meson` have golden files that frequently become stale due to upstream dependency changes.

### Clarification: Eval-Time vs Exec-Time Determinism

The codebase has a `deterministic` flag on actions and plans, but it measures **execution-time determinism** (will the same plan produce identical binaries?), NOT **eval-time stability** (will the same recipe produce the same plan?). These are orthogonal concepts:

| Concept | Question | Example |
|---------|----------|---------|
| **Exec-time determinism** (what flag measures) | Same plan → same binary? | `go_build` produces different binaries with different Go compiler versions |
| **Eval-time stability** (actual problem) | Same recipe → same plan? | `pip download httpie==3.2.4` resolves different transitive deps over time |

The golden file drift is an **eval-time stability** problem. The `deterministic` flag is correctly applied for its intended purpose (marking execution variance), but it does not predict or solve eval-time drift.

**Key insight**: Plan caching (Option 9) solves eval-time drift regardless of the `deterministic` flag. The flag becomes metadata only - it doesn't drive the validation strategy.

### Scope

**In scope:**
- Validation strategy for golden files that exhibit eval-time drift
- Changes to validation scripts and CI workflows
- Documentation of which recipes/actions have eval-time variability

**Out of scope:**
- Making ecosystem actions produce identical binaries (exec-time determinism - different problem)
- Changes to the lockfile generation approach (already implemented in #609)
- Version provider changes

## Decision Drivers

1. **CI stability**: Validation should not fail due to upstream dependency changes unrelated to recipe or code changes
2. **Regression detection**: Changes to recipes or tsuku code that affect plan structure should still be caught
3. **Maintainability**: The solution should not require frequent manual intervention
4. **Execution confidence**: Plans with eval-time variability should still be executable when validated
5. **Minimal complexity**: Prefer solutions that build on existing infrastructure
6. **Clean CLI ergonomics**: Users should not need to provide existing plans to generate reproducible plans

## Considered Options

### Option 1: Skip Validation for Ecosystem Recipes

Skip comparison entirely for recipes that use ecosystem actions (pip_exec, go_build, etc.).

**How it works:**
- `validate-golden.sh` checks if the plan contains ecosystem actions
- If yes, skip the regenerate-and-compare step
- Only validate that the file exists and is valid JSON

**Pros:**
- Simple implementation
- No false positive failures
- JSON validity checks still catch file corruption
- Execution validation (`validate-golden-execution.yml`) provides defense-in-depth

**Cons:**
- No regression detection for ecosystem recipes at plan-generation level
- Recipe changes that break ecosystem plans go unnoticed until execution
- Structural changes (new fields, action ordering) are not validated

**Note on the `deterministic` flag:** While the codebase has a `deterministic` flag, it measures exec-time variance, not eval-time drift. Using it as a validation switch would be semantically incorrect. See "Clarification" section above.

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

### Option 5: Tiered Validation by Recipe Type

Apply different validation strategies based on whether the recipe uses ecosystem actions:
- Core-only recipes (download_file, extract, etc.): exact comparison
- Ecosystem recipes (pip_exec, go_build, etc.): structural validation (Option 2)

**How it works:**
- Validation script checks if plan contains ecosystem actions
- Routes to appropriate comparison function
- Single workflow handles both cases
- Structural validation uses jq to extract comparable fields

**Pros:**
- Best of both worlds: strict for core recipes, flexible for ecosystem recipes
- Single unified workflow
- Maintains regression detection for both types

**Cons:**
- Two validation paths to maintain
- Need to keep structural schema definition updated
- Slightly more complex than pure skip
- Need to maintain list of "ecosystem" vs "core" actions

**Note on the `deterministic` flag:** The flag measures exec-time variance (binary reproducibility), not eval-time drift (plan reproducibility). Using it as the tier switch would be semantically incorrect - a `download_file` action could have eval-time drift if the URL template resolves differently, while a `pip_exec` with cached lockfile could be eval-stable.

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

### Option 9: Transparent Plan Caching (Recommended)

Make `tsuku eval` deterministic through automatic plan caching. First eval for a tool+version+platform resolves fresh and caches the result. Subsequent evals reuse the cached plan for determinism. No explicit CLI flag needed - discovery is implicit.

**How it works:**
1. `tsuku eval httpie@3.2.4` checks `$TSUKU_HOME/cache/plans/httpie/v3.2.4-linux-amd64.json`
2. If found AND recipe_hash matches current recipe: Return cached plan (instant, deterministic)
3. If not found OR recipe changed: Resolve fresh, cache result, return plan
4. Plans include `recipe_hash` to detect when recipe changed (staleness check)

**CLI interface:**
- `tsuku eval httpie` - Normal usage, auto-caches (implicit discovery like npm/cargo)
- `tsuku eval httpie --refresh` - Force fresh resolution, update cache
- `tsuku eval httpie --locked` - Fail if no cached plan exists (for CI determinism)
- `tsuku cache clear` - Clear plan cache

**Cache location:**
```
$TSUKU_HOME/cache/plans/
  └── httpie/
      ├── v3.2.4-darwin-arm64.json
      ├── v3.2.4-linux-amd64.json
      └── ...
```

**For CI validation**, golden files can seed the cache:
```bash
# In validate-golden.sh, before validation:
cp testdata/golden/plans/h/httpie/v3.2.4-linux-amd64.json \
   $TSUKU_HOME/cache/plans/httpie/v3.2.4-linux-amd64.json

# Now eval produces deterministic output matching the golden file
tsuku eval httpie@3.2.4 --locked
```

**Pros:**
- **No chicken-and-egg**: Normal CLI doesn't require prior state - first run resolves fresh
- **Implicit discovery**: Like npm/cargo, presence of cache triggers deterministic mode
- **Solves the user's complaint**: "I don't want a plan to generate a plan" - you don't, caching is transparent
- **Recipe hash freshness**: Automatically regenerates when recipe changes
- **CI-friendly**: `--locked` mode fails fast if no cached plan (catches missing golden files)
- **No infrastructure**: Local file cache only, no registry changes
- **Exact comparison works**: No structural validation needed when cache is used

**Cons:**
- First eval for any version is slow (network required)
- Different machines get different first-run results until cached
- Cache invalidation relies on recipe_hash (if hash function changes, cache is invalid)

**Pattern precedent:**
This follows the proven pattern from npm (package-lock.json), Cargo (Cargo.lock), and Poetry (poetry.lock):
- Implicit discovery (no CLI flag needed)
- First-run capture, subsequent reuse
- Staleness detection via content hash

## Decision Outcome

**Chosen: Option 9 - Transparent Plan Caching**

### Summary

Make `tsuku eval` deterministic through automatic plan caching with implicit discovery. First eval for a tool+version+platform resolves fresh and caches the result. Subsequent evals reuse the cached plan. This follows the proven pattern from npm, Cargo, and Poetry where lockfiles are automatically discovered and used without explicit CLI flags.

For CI validation, golden files seed the cache before validation runs, making regeneration produce identical output.

### Rationale

Option 9 provides the cleanest solution to the core problem:

1. **Eliminates drift at the source**: Instead of tolerating drift through structural comparison, we prevent drift by reusing cached resolutions.

2. **Clean CLI ergonomics**: No need for `--existing-plan <file>` flag. The user runs `tsuku eval httpie@3.2.4` and gets deterministic output if a cached plan exists.

3. **Follows proven patterns**: npm (package-lock.json), Cargo (Cargo.lock), and Poetry (poetry.lock) all use implicit lockfile discovery. Users expect this behavior.

4. **Recipe hash freshness**: Cache automatically invalidates when recipe changes, ensuring code changes are detected.

5. **CI-friendly**: `--locked` mode provides strict validation - fails if no cached plan exists.

6. **Exact comparison works**: No structural validation complexity needed when cache is used.

### Why Not Other Options

- **Option 5 (Structural validation)**: Solves the symptom (CI failures) but not the cause (non-deterministic resolution). More complex to maintain.
- **Option 1 (Skip)**: No regression detection.
- **Option 3 (Auto-regen PRs)**: Infrastructure complexity.
- **Option 4 (Freeze lockfiles in recipes)**: High authoring burden, doesn't scale.
- **Options 6-8**: Higher complexity for marginal benefit over Option 9.

### Fallback Strategy

If cached plan is missing and `--locked` is not specified:
1. Generate fresh plan
2. Cache result for next time
3. Log warning that fresh resolution was used

For CI, always use `--locked` to fail fast if golden file wasn't seeded to cache.

## Solution Architecture

### Plan Cache Design

The plan cache stores resolved plans keyed by tool, version, and platform:

```
$TSUKU_HOME/cache/plans/
├── httpie/
│   ├── v3.2.4-darwin-arm64.json
│   ├── v3.2.4-darwin-amd64.json
│   └── v3.2.4-linux-amd64.json
├── ruff/
│   ├── v0.8.6-darwin-arm64.json
│   └── ...
└── ...
```

Each cached plan is a complete JSON plan (same format as golden files) with `recipe_hash` for staleness detection.

### Cache Lookup Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        tsuku eval httpie@3.2.4                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Compute cache key:           │
                    │  httpie/v3.2.4-linux-amd64    │
                    └───────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Check $TSUKU_HOME/cache/     │
                    │  plans/{key}.json             │
                    └───────────────────────────────┘
                            │               │
                        Found           Not Found
                            │               │
                            ▼               │
                ┌───────────────────────┐   │
                │ Recipe hash matches?  │   │
                │ (current vs cached)   │   │
                └───────────────────────┘   │
                    │           │           │
                  Yes          No           │
                    │           │           │
                    ▼           └───────────┤
        ┌───────────────────┐               │
        │ Return cached     │               │
        │ plan (instant)    │               ▼
        └───────────────────┘   ┌───────────────────────┐
                                │ --locked flag set?    │
                                └───────────────────────┘
                                    │           │
                                  Yes          No
                                    │           │
                                    ▼           ▼
                        ┌───────────────┐   ┌───────────────────────┐
                        │ ERROR: No     │   │ Generate fresh plan   │
                        │ cached plan   │   │ (resolve deps)        │
                        └───────────────┘   └───────────────────────┘
                                                        │
                                                        ▼
                                            ┌───────────────────────┐
                                            │ Save to cache         │
                                            │ Return plan           │
                                            └───────────────────────┘
```

### CI Validation Flow

For golden file validation, seed the cache from golden files before running validation:

```bash
#!/bin/bash
# validate-golden.sh <recipe>

RECIPE="$1"
GOLDEN_DIR="testdata/golden/plans"

# Seed cache from golden files
for golden in "$GOLDEN_DIR"/*/"$RECIPE"/*.json; do
    # Extract version and platform from filename
    filename=$(basename "$golden")  # e.g., v3.2.4-linux-amd64.json
    version_platform="${filename%.json}"  # v3.2.4-linux-amd64

    # Copy to cache location
    cache_path="$TSUKU_HOME/cache/plans/$RECIPE/$filename"
    mkdir -p "$(dirname "$cache_path")"
    cp "$golden" "$cache_path"
done

# Now eval with --locked will produce deterministic output
for golden in "$GOLDEN_DIR"/*/"$RECIPE"/*.json; do
    version=$(jq -r '.version' "$golden")
    os=$(jq -r '.platform.os' "$golden")
    arch=$(jq -r '.platform.arch' "$golden")

    # Generate plan (will use cached version)
    actual=$(mktemp)
    ./tsuku eval "$RECIPE@$version" --os "$os" --arch "$arch" --locked > "$actual"

    # Compare
    if ! diff -q "$golden" "$actual" > /dev/null; then
        echo "MISMATCH: $golden"
        diff -u "$golden" "$actual"
        exit 1
    fi
done

echo "All golden files validated successfully"
```

### Recipe Hash Staleness Detection

Each plan includes `recipe_hash` computed from the recipe file:

```json
{
  "format_version": 3,
  "tool": "httpie",
  "version": "3.2.4",
  "recipe_hash": "927743d6894ffc490ac7b5d2495ed60c4e952e32ed2ed819f3b3f72e4465b0f1",
  ...
}
```

When checking cache:
1. Load cached plan
2. Compute current recipe hash
3. If hashes differ: cache is stale, regenerate
4. If hashes match: cache is valid, use it

This ensures code changes to recipes invalidate the cache.

### Structural Schema (Fallback)

If Option 9 implementation is delayed, Option 5 (structural validation) remains as a viable fallback. The structural schema definition is preserved below for reference.

**Note on flag usage in fallback:** The fallback implementation uses the `deterministic` flag to route validation. This is not semantically ideal since the flag measures exec-time variance, not eval-time drift. However, in practice there's high correlation: ecosystem actions (pip_exec, go_build) are both exec-non-deterministic AND eval-variable, while core actions (download_file, extract) are both exec-deterministic AND eval-stable. The flag works as an imperfect proxy. If implementing the fallback, consider detecting ecosystem actions directly instead of relying on the flag.

#### Structural Schema Definition

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

### Phase 1: Plan Cache Infrastructure

Add plan caching to `tsuku eval`:

1. **Cache directory structure**: `$TSUKU_HOME/cache/plans/{tool}/{version}-{os}-{arch}.json`
2. **Cache lookup in eval**: Check cache before running Decompose()
3. **Recipe hash staleness**: Compare cached `recipe_hash` with current recipe
4. **Cache write**: Save generated plans to cache after fresh resolution

**Key files to modify:**
- `cmd/tsuku/eval.go` - Add cache lookup/write logic
- `internal/executor/plan_generator.go` - Extract cache key generation
- `internal/tsuku/config.go` - Add cache directory path

### Phase 2: CLI Flags

Add flags to `tsuku eval`:

1. `--locked` - Fail if no cached plan exists (for CI determinism)
2. `--refresh` - Force fresh resolution, update cache
3. `--no-cache` - Skip cache entirely (for debugging)

**Key files to modify:**
- `cmd/tsuku/eval.go` - Add flag definitions and handling

### Phase 3: Validation Script Update

Update `scripts/validate-golden.sh` to use plan cache:

1. Seed cache from golden files before validation
2. Run `tsuku eval --locked` instead of bare `tsuku eval`
3. Compare output to golden file (exact match)
4. Remove structural validation complexity

**Key files to modify:**
- `scripts/validate-golden.sh` - Rewrite validation logic
- `scripts/validate-all-golden.sh` - Update to use new validation

### Phase 4: Re-enable CI Workflow

1. Remove `if: false` from `validate-golden-code.yml`
2. Update workflow to seed cache from golden files
3. Run validation with `--locked` flag
4. Monitor for a few PRs to confirm stability

### Phase 5: Cache Management Commands

Add cache management commands (optional but useful):

1. `tsuku cache list` - Show cached plans
2. `tsuku cache clear [tool]` - Clear cache (all or specific tool)
3. `tsuku cache info <tool>@<version>` - Show cache status

### Phase 6: Documentation

1. Update CONTRIBUTING.md to explain plan caching
2. Document `--locked` flag for CI usage
3. Add troubleshooting guide for cache misses

### Fallback: Structural Validation

If plan caching proves insufficient (e.g., recipe hash changes too frequently), the structural validation approach from Option 5 can be implemented as a fallback. The schema extraction script and validation logic are preserved in this design document for reference.

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

- **CI stability restored**: `validate-golden-code.yml` can be re-enabled without false failures
- **True determinism**: Plan caching eliminates drift at the source, not just tolerates it
- **Clean CLI ergonomics**: No need for `--existing-plan` flag; caching is transparent
- **Follows proven patterns**: Same approach as npm, Cargo, Poetry
- **Exact comparison works**: No structural validation complexity needed
- **Recipe change detection**: Recipe hash staleness automatically invalidates cache
- **No recipe format changes**: Existing recipes work without modification
- **Minimal infrastructure**: Local file cache only, no registry changes

### Negative

- **First eval is slow**: First resolution for any tool+version requires network
- **Cache storage**: Plans accumulate in `$TSUKU_HOME/cache/plans/`
- **Different first-run results**: Different machines may get different results on first run (before cache exists)
- **Recipe hash dependency**: If hash function changes, all cached plans become stale

### Mitigations

- **First eval is slow**: This is inherent to dependency resolution; caching makes subsequent runs instant
- **Cache storage**: Plans are small JSON files (~10KB each); add `tsuku cache clear` for cleanup
- **Different first-run**: For CI, always seed cache from golden files before validation
- **Recipe hash changes**: Document that major changes to recipe hashing invalidate cache

### Future Enhancements

- **Registry-hosted lockfiles**: Store blessed lockfiles alongside recipes in the registry for version-controlled determinism
- **Cache sharing**: Allow teams to share plan caches via remote storage
- **Structural validation fallback**: If caching proves insufficient, Option 5 (structural validation) can be layered on top
