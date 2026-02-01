# Issue 1258 Implementation Plan

## Summary

Replace the grep-based platform detection in the CI workflow's matrix step with `tsuku info --json --metadata-only --recipe <path>`, parsing the `supported_platforms` array to determine which runners each recipe needs.

## Approach

The matrix detection step already runs on ubuntu-latest. We add Go setup and tsuku build steps before the detection script, then replace the grep-based `linux_only` check (lines 98-110) with a `tsuku info` call that returns structured platform data. The script parses the `supported_platforms` JSON array to decide whether each recipe runs on Linux, macOS, or both. This approach uses the same platform resolution logic as the CLI itself, so it stays correct as platform constraints evolve.

### Alternatives Considered

- **Keep grep-based detection, add more patterns**: Fragile as constraint fields grow; duplicates logic already in the CLI binary.
- **Use a standalone jq-based TOML parser**: Would require installing extra tools and still wouldn't capture the full platform resolution logic (e.g., unsupported_platforms exclusions, family-aware policies).

## Files to Modify

- `.github/workflows/test-changed-recipes.yml` - Add Go/tsuku build to matrix job; replace grep-based platform detection with `tsuku info` calls; unify macOS recipe list format from simple strings to objects with path info.

## Files to Create

None.

## Implementation Steps

- [x] Add Go setup and tsuku build steps to the `matrix` job, before the "Get changed recipes" step
- [x] Replace the grep-based `linux_only` detection block (lines 98-110) with a `tsuku info --json --metadata-only --recipe "$path"` call that reads `supported_platforms`
- [x] Add platform-to-runner mapping logic: check if any platform has `os == "linux"` (-> include in Linux matrix), check if any has `os == "darwin"` (-> include in macOS list)
- [x] Handle empty `supported_platforms` array (no constraints) as "all runners" for backward compatibility
- [x] Unify the macOS recipe list format to use objects with `tool` and `path` fields (matching the Linux matrix format) instead of bare tool name strings
- [x] Update the `test-macos` job to consume the new object format, using `matrix.recipe.path` instead of deriving the path from the tool name
- [x] Verify the existing exclusion logic (library, require_system, execution-exclusions.json) remains unchanged and runs before the `tsuku info` call

## Testing Strategy

- **Manual verification**: Create a test branch with a Linux-only recipe change and a cross-platform recipe change; confirm the matrix job outputs correct runner assignments in the workflow log.
- **Local dry-run**: Build tsuku locally, run `tsuku info --json --metadata-only --recipe <path>` on several recipes with different constraint profiles, verify the `supported_platforms` output matches expectations.
- **Workflow syntax**: Run `actionlint` on the modified workflow file if available locally.
- **Backward compat**: Verify a recipe with no platform constraints (e.g., `ripgrep`) produces a non-empty `supported_platforms` covering both Linux and darwin.

## Risks and Mitigations

- **tsuku build adds time to matrix job**: The Go build adds ~30-60s. Mitigation: use Go module cache (`actions/setup-go` already does this). The matrix job is a lightweight detection step so this overhead is acceptable.
- **tsuku info fails on a recipe**: If `tsuku info` errors out (e.g., malformed recipe), the whole matrix step fails. Mitigation: add a fallback that treats a failed `tsuku info` call as "all platforms" (same as no constraints).
- **Embedded vs registry recipe paths**: The macOS job currently derives paths assuming `recipes/` prefix. After unifying to objects with explicit paths, both embedded (`internal/recipe/recipes/`) and registry (`recipes/`) paths are handled correctly.

## Success Criteria

- [ ] Changed Linux-only recipes produce a matrix with `linux_only=true` (or equivalent) and don't appear in the macOS list
- [ ] Changed cross-platform recipes appear in both Linux and macOS lists
- [ ] Changed darwin-only recipes appear only in macOS list and skip Linux
- [ ] Recipes with no platform constraints run on all runners
- [ ] Existing exclusions (library, require_system, execution-exclusions.json) still skip recipes correctly
- [ ] The macOS job no longer derives recipe paths from tool names -- it uses paths from the matrix output

## Open Questions

None. All blockers resolved during introspection.
