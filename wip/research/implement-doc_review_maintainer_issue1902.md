# Maintainer Review: Issue #1902

## Review Scope

Issue #1902 migrates CI workflow container image references to the centralized `container-images.json` config created in #1901. Seven files changed:

- `.github/workflows/recipe-validation-core.yml`
- `.github/workflows/test-recipe.yml`
- `.github/workflows/batch-generate.yml`
- `.github/workflows/validate-golden-execution.yml`
- `.github/workflows/platform-integration.yml`
- `.github/workflows/release.yml`
- `test/scripts/test-checksum-pinning.sh`

## Findings

### 1. Divergent jq patterns for reading container-images.json -- Advisory

Three different jq idioms are used across the migrated files to read images from `container-images.json`:

**Pattern A** (recipe-validation-core.yml, lines 143 and 248):
```yaml
run: echo "ref=$(jq -r '.${{ matrix.family }}' container-images.json)" >> "$GITHUB_OUTPUT"
```
Uses GitHub template expansion inside the jq filter with dot-access syntax.

**Pattern B** (test-recipe.yml lines 136, 244; batch-generate.yml lines 232, 394):
```bash
IMAGES+=("$(jq -r --arg f "$f" '.[$f]' container-images.json)")
```
Uses jq `--arg` with bracket-access syntax, iterating over a shell array.

**Pattern C** (validate-golden-execution.yml line 639; test-checksum-pinning.sh line 56):
```bash
CONTAINER_IMAGE=$(jq -r --arg f "$FAMILY" '.[$f] // empty' container-images.json)
if [ -z "$CONTAINER_IMAGE" ]; then
  echo "Unknown family: $FAMILY (not found in container-images.json)"
  exit 1
fi
```
Uses jq `--arg` with `// empty` fallback and explicit error handling.

The next developer modifying one of these workflows will see a different pattern than the one they saw in another workflow and wonder which is correct. Pattern A (dot-access with template expansion) would break if a family name contained a hyphen or dot; Pattern B would return "null" as a string silently; Pattern C handles errors properly.

The real risk is subtle: if someone adds a new family to a workflow's FAMILIES array but not to `container-images.json`, Pattern A and B pass the literal string "null" to `docker run`, which produces a confusing Docker pull error ("pull access denied for null") rather than a clear message about the missing family. Pattern C catches this properly.

That said, the family names are controlled by the hardcoded arrays in the workflows (or the matrix in recipe-validation-core.yml), and they all match the current keys in `container-images.json`. The drift risk is low in practice because issue #1903 will add a drift-check CI job. This is advisory because it won't cause a misread of the code's intent, but standardizing on Pattern C's error handling would make failures easier to debug.

**Suggestion**: Consider adding a guard to Pattern B workflows (test-recipe.yml, batch-generate.yml) that checks for empty/null images after the jq loop. A single check like `[[ "${IMAGES[*]}" == *"null"* ]] && echo "ERROR: missing image in container-images.json" && exit 1` after the loop would catch the issue early. Or standardize on `// empty` in the jq filter across all files.

### 2. Parallel structure in batch-generate.yml NAMES vs LIBCS arrays -- Advisory

`batch-generate.yml` lines 229-234 (validate-linux-x86_64 job):
```bash
NAMES=("debian" "rhel" "arch" "suse" "alpine")
IMAGES=()
for f in "${NAMES[@]}"; do
  IMAGES+=("$(jq -r --arg f "$f" '.[$f]' container-images.json)")
done
LIBCS=("glibc" "glibc" "glibc" "glibc" "musl")
```

The NAMES and LIBCS arrays are parallel -- their elements correspond by index. If someone reorders NAMES or adds a new family, they must also update LIBCS at the corresponding index. This was a pre-existing pattern (LIBCS was already there before this issue), but the addition of the dynamic IMAGES array loaded via jq makes the parallel structure slightly harder to follow. There are now three arrays that must stay in sync by index: NAMES, IMAGES, and LIBCS.

This is advisory because the pre-existing pattern didn't change, and the IMAGES array is derived from NAMES, so it automatically stays in sync. But the next person editing this section has three parallel arrays to keep aligned instead of two.

### 3. Inline comments explaining the pattern -- Good

Several files include helpful inline comments explaining why images come from the config file:

- `test-recipe.yml:132`: `# Five Linux x86_64 families (images from container-images.json)`
- `test-recipe.yml:240`: `# Images from container-images.json`
- `platform-integration.yml:80`: `# Alpine image read from container-images.json (shared with release.yml)`
- `platform-integration.yml:113`: `# Read container images from centralized config for the integration matrix`
- `release.yml:125`: `# Alpine image read from container-images.json (shared with platform-integration.yml)`
- `test-checksum-pinning.sh:23`: `# Validate jq is available (required for reading container-images.json)`

These comments are good. A developer landing in any of these files will immediately understand where images come from and why jq is being used. The cross-references between release.yml and platform-integration.yml are especially helpful.

### 4. test-checksum-pinning.sh jq validation -- Good

The script at `test/scripts/test-checksum-pinning.sh` (lines 24-28, 50-61) has the most complete error handling of all consumers:

1. Checks that `jq` is available before proceeding (line 24)
2. Checks that `container-images.json` exists (line 51)
3. Checks that the requested family key returns a value (line 57)
4. Provides actionable error messages including the list of valid families (line 59)

This is the gold standard for how consumers should handle the config file. It's a shell script that runs locally, so the jq check matters (CI runners always have jq, but developer machines might not). The error messages point the developer to the right fix.

### 5. platform-integration.yml load-images job uses sparse-checkout well -- Good

`platform-integration.yml:121-122`:
```yaml
- uses: actions/checkout@...
  with:
    sparse-checkout: container-images.json
```

The `load-images` job only needs the JSON file, so sparse-checkout avoids cloning the entire repo. Same approach in `release.yml:144-146` for the musl build job. This shows awareness of CI efficiency.

### 6. recipe-validation-core.yml removed image names from matrix cleanly -- Good

The matrix blocks at lines 113-128 (x86_64) and 220-234 (arm64) now contain only `family`, `libc`, and `install_cmd` -- the image field is gone. The image is loaded in a separate step via jq, then referenced via `${{ steps.image.outputs.ref }}`. This cleanly separates the "what to install inside the container" config (install_cmd, libc) from the "which container to use" config (loaded from JSON).

### 7. No hardcoded image references remain in migrated files -- Good

A grep for literal image references (`debian:bookworm`, `fedora:`, `archlinux:`, `alpine:3`, `opensuse/`) across all `.github/workflows/*.yml` files and `test/scripts/` returns only a single hit: a commented-out block in `test.yml:206` (`# image: alpine:3.19`), which is not in scope for this issue.

## Summary

The migration is clean and consistent with the design doc's intent. The three consumption patterns (matrix-based, array-based, inline lookup) match the three categories described in the design doc's Decision 3. Comments explain the pattern in each file, making it easy for the next developer to understand why jq is being used.

The main maintainability observation is the divergence in error handling: two of the three patterns silently pass "null" to `docker run` if a family key is missing from `container-images.json`, while the third pattern catches this with a clear error message. This won't cause a bug today (the family names all match), but standardizing the error handling would make future misconfigurations easier to debug. Issue #1903's drift-check CI job will catch most of these scenarios anyway.
