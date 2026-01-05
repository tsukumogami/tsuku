# Issue 802 Implementation Plan

## Summary

Migrate `test-checksum-pinning.sh` and `test-homebrew-recipe.sh` to use the sandbox container pattern established in PR #804, replacing hardcoded apt-get calls with `tsuku eval --install-deps` and `tsuku install --plan --sandbox`.

## Approach

Follow the reference implementation from `test-cmake-provisioning.sh` and `test-readline-provisioning.sh`:

1. Replace custom Dockerfile generation with `tsuku eval --linux-family <family> --install-deps`
2. Use `tsuku install --plan plan.json --sandbox --force` to execute in isolated containers
3. Add multi-family iteration (debian, rhel, arch, alpine, suse)

This approach validates the sandbox container building functionality end-to-end and ensures tests work across Linux distribution families.

### Alternatives Considered

- **Create dedicated test recipes (e.g., `build-essentials.toml`)**: Not chosen because existing recipes already provide the dependencies needed. The migrated cmake/readline scripts don't use test recipes.
- **Rewrite tests to run on host instead of containers**: Not chosen because container isolation is essential for testing the sandbox functionality and ensuring reproducibility.
- **Create a jq recipe for `test-checksum-pinning.sh`**: Not chosen for initial scope. The test can be rewritten to use grep-based JSON validation or `tsuku verify` output, avoiding the need for a jq dependency.

## Files to Modify

- `test/scripts/test-checksum-pinning.sh` - Remove Dockerfile/apt-get pattern, use sandbox container pattern, rewrite jq-based validation to use grep or tsuku verify output
- `test/scripts/test-homebrew-recipe.sh` - Remove Dockerfile/apt-get pattern, use sandbox container pattern with patchelf as a dependency

## Files to Create

None - this is a refactor of existing files.

## Implementation Steps

### Step 1: Migrate test-checksum-pinning.sh

- [ ] Remove the embedded Dockerfile generation (lines 26-50)
- [ ] Remove apt-get dependency installation (wget, curl, ca-certificates, jq)
- [ ] Replace jq-based JSON validation with grep-based checks or tsuku verify output parsing
- [ ] Add multi-family iteration structure (FAMILIES array)
- [ ] Use `tsuku eval fzf --os linux --linux-family "$family" --install-deps > fzf-$family.json` to generate plans
- [ ] Use `tsuku install --plan fzf-$family.json --sandbox --force` to run tests in isolated containers
- [ ] Update test assertions to work within sandbox execution context
- [ ] Add cleanup of plan files after each family iteration

### Step 2: Migrate test-homebrew-recipe.sh

- [ ] Remove the embedded Dockerfile generation (lines 29-52)
- [ ] Remove apt-get dependency installation (wget, curl, ca-certificates, patchelf)
- [ ] Add multi-family iteration structure (FAMILIES array)
- [ ] Generate plan with patchelf dependency: `tsuku eval $TOOL_NAME --os linux --linux-family "$family" --install-deps > plan-$family.json`
- [ ] Use `tsuku install --plan plan-$family.json --sandbox --force` for sandbox execution
- [ ] Update verification commands to work within sandbox context
- [ ] Add cleanup of plan files after each family iteration

### Step 3: Validate changes

- [ ] Run both migrated scripts locally with `--family debian` argument
- [ ] Verify scripts pass for multiple families (debian, alpine at minimum)
- [ ] Run `go test ./...` to ensure no regressions

## Testing Strategy

- **Manual verification**: Run each migrated script with `debian` family to verify basic functionality
- **Multi-family testing**: Run scripts with `--family alpine` to verify cross-distribution support
- **Integration test**: Run both scripts end-to-end to verify complete flow works
- **CI validation**: PR CI will run the updated scripts

## Risks and Mitigations

- **jq dependency removal may break test accuracy**: Mitigation - Use `tsuku verify` command output which provides human-readable status, or use grep to check for "binary_checksums" string in JSON
- **Homebrew recipes may have family-specific issues**: Mitigation - patchelf recipe already exists and was tested; start with debian family which has best homebrew support
- **Sandbox container builds may be slow**: Mitigation - This is expected and acceptable for CI tests; container images are cached after first build

## Success Criteria

- [ ] `test-checksum-pinning.sh` runs successfully without apt-get calls
- [ ] `test-homebrew-recipe.sh` runs successfully without apt-get calls
- [ ] Both scripts use the `tsuku eval + tsuku install --plan --sandbox` pattern
- [ ] Both scripts iterate over multiple Linux families (at minimum debian)
- [ ] All tests pass in CI
- [ ] No remaining `apt-get` or hardcoded Dockerfile generation in these scripts

## Open Questions

None - all blockers from the original issue (#805, #703) have been resolved and the implementation pattern is well-established from PR #804.
