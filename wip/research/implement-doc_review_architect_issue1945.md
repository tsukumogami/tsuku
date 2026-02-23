# Architect Review: Issue #1945 -- Migrate test-recipe.yml Linux Jobs to Sandbox

**0 blocking, 0 advisory.**

## Summary

This change fits the architecture cleanly. The two Linux jobs (`test-linux-x86_64`, `test-linux-arm64`) now use `tsuku install --sandbox` instead of `docker run`, exactly as the design doc specified. The migration delegates container management, family-specific setup, and verification to the sandbox -- the same code path users run locally. No parallel patterns introduced.

## Verification Checklist

### No docker run in Linux jobs
Confirmed. Zero `docker run` matches in `.github/workflows/test-recipe.yml`. The old pattern (container-images.json lookup, volume mounting, per-family package installation, `.tsuku-exit-code` marker files) has been completely removed.

### Required flags present
Both Linux jobs use the correct sandbox invocation pattern at lines 152-155 (x86_64) and 242-245 (arm64):

```bash
./tsuku-linux-amd64 install --sandbox --force --recipe "$recipe_path" \
  --target-family "$family" \
  --env GITHUB_TOKEN="$GITHUB_TOKEN" \
  --json > "$RESULT_FILE" 2>/dev/null || true
```

All three required flags are present: `--json`, `--env GITHUB_TOKEN`, `--target-family`.

### Result table in GITHUB_STEP_SUMMARY preserved
Both Linux jobs write the same markdown table format (Recipe | Family | Status) to `$GITHUB_STEP_SUMMARY` at lines 180-193 (x86_64) and 269-283 (arm64). The table is built from the sandbox JSON output via `jq .passed` and `jq .install_exit_code`, replacing the old marker-file-based approach.

### macOS jobs untouched
`test-darwin-arm64` (line 291) and `test-darwin-x86_64` (line 367) are unchanged. They still use native runner execution with `gtimeout`, `TSUKU_HOME` isolation, and no sandbox flags. This is correct -- macOS has no container runtime for sandbox.

### Build job untouched
The `build` job (line 37) is unchanged. It still cross-compiles four binaries and detects recipes.

## Structural Observations

**Pattern consistency with design doc.** The sandbox call matches the "After" example in `docs/designs/DESIGN-sandbox-ci-integration.md` (lines 412-417) almost exactly. The only difference is the result file naming convention (`.result-${recipe_name}-${family}.json` vs `.result-$family.json`), which is an improvement since it handles multiple recipes per run without filename collisions.

**JSON result handling is defensive.** Lines 157-163 check both file existence and JSON validity before extracting fields. If the sandbox crashes before producing output, the fallback is `SANDBOX_PASSED="false"` and `EXIT_CODE=1`. This handles edge cases the old docker-based approach also needed to handle.

**No container-images.json dependency removed.** Before this commit, test-recipe.yml read `container-images.json` to resolve family images for `docker run`. That dependency is gone -- the sandbox handles image selection internally via `containerimages.ImageForFamily()`. This is the intended direction: CI workflows stop managing container images directly.

**continue-on-error semantics preserved.** All four test jobs (both Linux, both macOS) retain `continue-on-error: true` at the job level (lines 116, 205, 295, 371). Platform failures still don't block merge.

No architectural concerns. The change follows the established sandbox migration pattern from the design doc, uses the CLI's public flags rather than bypassing any abstractions, and scopes the migration precisely to the two Linux jobs specified in issue #1945.
