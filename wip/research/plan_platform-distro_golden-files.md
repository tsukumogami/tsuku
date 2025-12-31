# Platform-Distro Golden File Testing Analysis

## Executive Summary

The introduction of `target = (platform, distro)` in the system dependency actions design creates significant implications for golden file testing. While the current design correctly handles `(os, arch)` combinations, adding distro detection introduces a new dimension that could cause combinatorial explosion if not managed carefully.

**Key finding**: The `when = { distro = [...] }` clause affects **step filtering**, not **plan structure**. This means golden files represent platform-specific plans, where distro-specific steps are simply absent from platforms that don't match. The combinatorial explosion concern is partially mitigated by this design, but CI execution validation for distro variants remains challenging.

## Current Golden File Structure

From `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/docs/DESIGN-golden-plan-testing.md`:

```
testdata/golden/plans/{first-letter}/{recipe}/
    v{version}-{os}-{arch}.json
```

Current platform dimensions:
- `linux-amd64` (generated and validated)
- `linux-arm64` (excluded - no CI runner)
- `darwin-amd64` (generated, execution skipped - paid tier)
- `darwin-arm64` (generated and validated)

**File count**: ~600 golden files for 155 recipes x ~4 platforms (minus exclusions)

## Distro Dimension Analysis

### How Distro Affects Plans

The `distro` field in `when` clause filters which steps appear in a plan:

```toml
[[steps]]
action = "apt_install"
packages = ["docker.io"]
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "dnf_install"
packages = ["docker"]
when = { distro = ["fedora"] }
```

**Key insight**: When tsuku evaluates a recipe for a target platform, it:
1. Detects the distro (or uses a specified target distro)
2. Filters steps by their `when` clauses
3. Generates a plan containing only matching steps

This means:
- A plan for `linux-amd64-ubuntu` would contain `apt_install` steps
- A plan for `linux-amd64-fedora` would contain `dnf_install` steps
- A plan for `darwin-arm64` would contain `brew_cask` steps (no distro filtering)

### Naming Convention Extension

If distro becomes part of golden file naming:

```
testdata/golden/plans/{first-letter}/{recipe}/
    v{version}-linux-amd64.json              # No system deps (current)
    v{version}-linux-amd64-ubuntu.json       # Ubuntu-specific steps
    v{version}-linux-amd64-fedora.json       # Fedora-specific steps
    v{version}-darwin-arm64.json             # macOS (no distro)
```

### Combinatorial Analysis

**Current state** (4 platforms, ~155 recipes): ~600 golden files

**With distro dimension** (worst case):

| Platform | Distros | Files per recipe |
|----------|---------|------------------|
| linux-amd64 | ubuntu, debian, fedora, arch | 4 |
| linux-arm64 | ubuntu, debian | 2 (currently excluded) |
| darwin-amd64 | (none) | 1 |
| darwin-arm64 | (none) | 1 |

**Total**: 8 files per recipe with system deps = 1,240 files for 155 recipes

**Reality check**: Most recipes (90%+) have no system deps - they use `download`, `extract`, etc. Only recipes with `apt_install`, `dnf_install`, etc. need distro-specific golden files.

Estimated breakdown:
- ~150 recipes with no system deps: 600 files (4 platforms each, minus exclusions)
- ~5 recipes with system deps: 40 files (8 distro-platform combos each)
- **Total**: ~640 files (modest increase from 600)

## CI Execution Validation Concerns

### Current CI Platform Support

From workflow analysis:

| Platform | Runner | Available |
|----------|--------|-----------|
| linux-amd64 | ubuntu-latest | Yes |
| darwin-arm64 | macos-latest | Yes |
| darwin-amd64 | macos-13 | Yes (paid tier for execution) |
| linux-arm64 | (none) | No |

### Distro Variant Testing Challenge

**Problem**: GitHub Actions `ubuntu-latest` provides only Ubuntu. Testing Fedora/Arch golden files requires:

1. **Container-based execution**: Run `fedora:latest` or `archlinux:latest` containers on `ubuntu-latest` runner
2. **Self-hosted runners**: Maintain Fedora/Arch self-hosted runners (expensive, complex)
3. **Skip distro-specific execution**: Generate golden files but skip execution validation for non-Ubuntu distros

### Available Base Images

Common distro images for container-based testing:

| Distro | Docker Image | Package Manager |
|--------|--------------|-----------------|
| Ubuntu | `ubuntu:22.04` | apt |
| Debian | `debian:bookworm` | apt |
| Fedora | `fedora:latest` | dnf |
| Arch | `archlinux:latest` | pacman |
| Alpine | `alpine:latest` | apk |

**CI time estimate** per distro:
- Container pull + setup: ~30s
- Package installation: ~1-2min
- Tool installation: ~1min
- **Per-recipe overhead**: ~3-4min per distro variant

For 5 recipes with 4 distro variants each:
- **Parallel execution**: ~4min (matrix job per variant)
- **Sequential execution**: ~80min (unacceptable)

## Coverage Strategy Recommendations

### Recommendation 1: Distro-Conditional Golden Files

**Only generate distro-specific golden files when the recipe uses distro-specific steps.**

```go
// In plan generation
needsDistroVariant := false
for _, step := range plan.Steps {
    if step.When != nil && len(step.When.Distro) > 0 {
        needsDistroVariant = true
        break
    }
}

if needsDistroVariant {
    // Generate distro-specific golden files
} else {
    // Generate platform-only golden files (current behavior)
}
```

**Benefit**: Recipes using only `download`, `extract`, `brew_cask` (with `os` filtering, not `distro`) don't need distro variants.

### Recommendation 2: Tiered Execution Validation

| Tier | Distros | CI Strategy |
|------|---------|-------------|
| Primary | Ubuntu | Native execution on `ubuntu-latest` |
| Secondary | Debian, Fedora | Container-based execution on `ubuntu-latest` |
| Optional | Arch, Alpine | Golden file generation only, no execution validation |

**Implementation**:

```yaml
# Fedora execution via container
validate-fedora:
  runs-on: ubuntu-latest
  container:
    image: fedora:latest
  steps:
    - name: Install tsuku deps
      run: dnf install -y curl git
    - name: Execute golden plan
      run: ./tsuku install --plan testdata/golden/plans/.../v1.0.0-linux-amd64-fedora.json
```

### Recommendation 3: Distro-Specific Step Isolation

When a recipe has `distro`-filtered steps, those steps are essentially platform-specific. The plan structure for a given `(os, arch, distro)` tuple is deterministic.

**Key insight**: If a recipe only has:
```toml
when = { distro = ["ubuntu", "debian"] }
```

Then we only need golden files for Ubuntu/Debian. Other distros would produce plans with those steps filtered out (likely causing install failure).

**This suggests**: Golden files should only exist for distros the recipe explicitly supports.

### Recommendation 4: Centralized Distro Detection for CI

Rather than hardcoding distros in CI workflows, query recipe metadata:

```bash
# Get distros supported by a recipe
DISTROS=$(./tsuku info --recipe recipes/d/docker.toml --metadata-only --json | \
    jq -r '.supported_distros[]' 2>/dev/null || echo "")

# Generate matrix from actual support
if [[ -z "$DISTROS" ]]; then
    # No distro-specific steps, use platform-only testing
    MATRIX='[{"os":"linux","arch":"amd64"}]'
else
    # Generate distro-specific matrix
    MATRIX=$(echo "$DISTROS" | jq -R -s -c 'split("\n")[:-1] | map({os:"linux",arch:"amd64",distro:.})')
fi
```

## Implementation Considerations

### 1. Recipe Metadata Extension

Add `supported_distros` to recipe metadata (computed from step `when` clauses):

```go
type RecipeMetadata struct {
    // Existing
    SupportedPlatforms []string `json:"supported_platforms"`

    // New
    SupportedDistros   []string `json:"supported_distros,omitempty"`
}
```

### 2. Golden File Naming Convention

Options:

**Option A: Append distro to filename**
```
v1.0.0-linux-amd64-ubuntu.json
v1.0.0-linux-amd64-fedora.json
```
- Pro: Flat directory structure
- Con: Long filenames, sorting issues

**Option B: Subdirectory per distro**
```
ubuntu/v1.0.0-linux-amd64.json
fedora/v1.0.0-linux-amd64.json
default/v1.0.0-darwin-arm64.json
```
- Pro: Clear organization
- Con: Deeper nesting, more directories

**Recommendation**: Option A with convention that darwin files have no distro suffix (implicitly "no distro").

### 3. Plan Generation Tooling Changes

The `regenerate-golden.sh` script needs to:
1. Detect if recipe has distro-specific steps
2. Iterate over supported distros
3. Pass `--distro` flag to `tsuku eval` (new flag needed)

```bash
# New flag for cross-distro plan generation
./tsuku eval --recipe docker.toml --os linux --arch amd64 --distro ubuntu --version 1.0.0
```

### 4. Validation Script Changes

The `validate-golden.sh` script needs to:
1. Iterate over distro-specific golden files
2. Match each golden file to its intended distro target
3. Regenerate with correct `--distro` flag

## CI Time Impact Analysis

### Current CI Time (from workflows)

| Workflow | Time | Frequency |
|----------|------|-----------|
| Golden validation (recipe changes) | ~5min | Per PR |
| Golden execution validation | ~10min | Per PR (if golden files changed) |
| Full golden regeneration | ~30min | Rare (code changes affecting all) |

### Projected CI Time (with distro variants)

Assuming 5 recipes with system deps, 4 distro variants each:

| Scenario | Additional Jobs | Time Added |
|----------|-----------------|------------|
| Recipe change (system deps recipe) | +3 distro variants | +3min parallel |
| Recipe change (non-system deps) | 0 | 0 |
| Code change affecting plan gen | +15 distro variants total | +15min parallel |

**Conclusion**: Time impact is manageable if:
1. Distro variants run in parallel
2. Only recipes with system deps have distro variants
3. Container pull is cached

## Risks and Mitigations

### Risk 1: Distro Package Availability

**Problem**: A package available in Ubuntu may not exist in Fedora with the same name.

**Mitigation**: Recipe authors use distro-specific steps:
```toml
[[steps]]
action = "apt_install"
packages = ["docker.io"]
when = { distro = ["ubuntu"] }

[[steps]]
action = "dnf_install"
packages = ["docker-ce"]  # Different package name
when = { distro = ["fedora"] }
```

Golden files capture these per-distro differences.

### Risk 2: Distro Detection Failures

**Problem**: Container environments may not have `/etc/os-release` or may have unexpected values.

**Mitigation**:
1. Require `/etc/os-release` in sandbox containers
2. Fall back to "unknown" distro, which skips distro-specific steps
3. Add `require_distro` action that fails clearly if distro cannot be detected

### Risk 3: Derivative Distro Handling

**Problem**: Linux Mint, Pop!_OS, etc. are Ubuntu derivatives. Should they use Ubuntu golden files?

**Mitigation**: The `ID_LIKE` chain from `/etc/os-release`:
- Linux Mint: `ID=linuxmint`, `ID_LIKE=ubuntu`
- Pop!_OS: `ID=pop`, `ID_LIKE=ubuntu debian`

When `distro = ["ubuntu"]`, matching should check both `ID` and `ID_LIKE`.

### Risk 4: Golden File Explosion for Edge Cases

**Problem**: A recipe supporting many distros could generate many golden files.

**Mitigation**:
1. Most recipes will have 0 distro-specific steps (no explosion)
2. Cap supported distros in guidelines (e.g., "support at most 4 distros")
3. Allow `when = { os = ["linux"] }` for truly universal Linux steps (no distro filtering)

## Answers to Original Questions

### Q1: If a recipe only has `when = { distro = ["ubuntu"] }`, do we need golden files for other distros?

**Answer**: No. Golden files should only exist for distros the recipe supports. For unsupported distros, the plan would have those steps filtered out, likely resulting in an incomplete/broken installation.

### Q2: Do we run sandbox tests for ALL distro variants?

**Answer**: Recommend tiered approach:
- **Ubuntu**: Native execution validation
- **Debian, Fedora**: Container-based execution validation
- **Others**: Golden file generation only (no execution)

### Q3: How long would CI take?

**Answer**: Modest increase if:
- Matrix parallelization is used
- Only system-deps recipes have distro variants (~5 recipes)
- Container images are cached

Estimate: +3-5 minutes for typical PR, +15 minutes for code changes affecting all recipes.

### Q4: What base images are available?

**Answer**: All major distros have official Docker images:
- Ubuntu, Debian, Fedora, Arch, Alpine, CentOS Stream, RHEL UBI

These can run on `ubuntu-latest` runners via container feature.

## Recommendations Summary

1. **Only generate distro-specific golden files for recipes with distro-specific steps** - prevents explosion for 95% of recipes

2. **Use tiered execution validation** - Ubuntu native, Debian/Fedora via container, others generation-only

3. **Extend naming convention** to `v{version}-{os}-{arch}[-{distro}].json` - distro suffix only when needed

4. **Add `--distro` flag to `tsuku eval`** for cross-distro plan generation

5. **Compute `supported_distros` from recipe steps** - don't require manual declaration

6. **Cache container images in CI** - avoid repeated pulls for distro testing

7. **Document distro support policy** - recommend authors support ubuntu + one alternative (debian or fedora)
