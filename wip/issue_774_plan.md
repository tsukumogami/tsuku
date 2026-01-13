# Issue 774 Implementation Plan (Final)

## Summary

Generate golden files for all 162 recipes across all platform+family combinations, validate execution in sandbox containers (which automatically select the correct family-specific base image), and iteratively fix recipes that fail by adding missing system dependencies using typed actions.

## Key Insight

The `--sandbox` flag already handles family-specific containers via `internal/sandbox/container_spec.go`:

```go
var familyToBaseImage = map[string]string{
    "debian": "debian:bookworm-slim",
    "rhel":   "fedora:41",
    "arch":   "archlinux:base",
    "alpine": "alpine:3.19",
    "suse":   "opensuse/leap:15",
}
```

No Dockerfile creation needed - sandbox automatically derives the container spec from the plan's typed actions.

## Current State

- **162 recipes** in `internal/recipe/recipes/`
- **128 golden files** exist across **19 recipe directories**
- **~143 recipes** missing golden files
- **Container infrastructure** already complete (`internal/sandbox/`)

## Implementation Steps

### Step 1: Generate Golden Files for All Recipes

For each recipe without golden files:
```bash
./scripts/regenerate-golden.sh <recipe>
```

This generates golden files for all supported platform+family combinations.

### Step 2: Run Sandbox Validation

For each golden file:
```bash
./tsuku install --sandbox --plan testdata/golden/plans/<letter>/<recipe>/<version>-<platform>.json
```

The sandbox will:
1. Parse the plan to identify required system packages
2. Select the appropriate base image for the target family
3. Build/cache a container with those packages
4. Execute the installation inside the container

### Step 3: Fix Failing Recipes

When sandbox execution fails (missing system dep):
1. Identify the missing dependency from error output
2. Add the appropriate typed action to the recipe:
   ```toml
   [[steps]]
   action = "apt_install"  # or dnf_install, pacman_install, apk_install, zypper_install
   packages = ["missing-package"]
   ```
3. Regenerate golden files: `./scripts/regenerate-golden.sh <recipe>`
4. Re-run sandbox validation

### Step 4: Iterate Until All Pass

Repeat Steps 2-3 until all recipes pass sandbox validation for all their supported platforms.

### Step 5: Commit and Create PR

Once all recipes pass:
1. Commit all golden files
2. Commit recipe changes (added system deps)
3. Create PR with summary of changes

## Workflow Automation

For efficiency, batch process:

```bash
# Generate all missing golden files
for recipe in internal/recipe/recipes/*/*.toml; do
  name=$(basename "$recipe" .toml)
  letter="${name:0:1}"
  if [ ! -d "testdata/golden/plans/$letter/$name" ]; then
    ./scripts/regenerate-golden.sh "$name"
  fi
done

# Validate all golden files
find testdata/golden/plans -name "*.json" -exec \
  ./tsuku install --sandbox --plan {} \;
```

## Expected Outcomes

1. **All 162 recipes** have golden files for all supported platform+family combinations
2. **All recipes pass** sandbox execution validation
3. **No hidden system dependencies** - all deps are explicitly declared with typed actions
4. **CI can enforce** golden file coverage going forward

## Risks

| Risk | Mitigation |
|------|------------|
| Some recipes may need many system deps | Accept iteration; group related deps |
| Sandbox execution may be slow | Use container caching (`ContainerImageName` generates cache keys) |
| Version resolution may fail for some recipes | Use `--version` flag or add `[version]` section |
