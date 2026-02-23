# Architect Review: Issue #1947

## Blocking Findings

None.

## Advisory Findings

### 1. Dual JSON format detection diverges from prior migration pattern

`batch-generate.yml` lines 276-283 (x86_64) and 428-435 (arm64) use a two-step exit code extraction that falls back from `install_exit_code` (sandbox JSON) to `exit_code` (error JSON):

```bash
# Sandbox JSON has install_exit_code; error JSON has exit_code
if jq -e '.install_exit_code' "$RESULT_FILE" > /dev/null 2>&1; then
  EXIT_CODE=$(jq -r '.install_exit_code' "$RESULT_FILE")
elif jq -e '.exit_code' "$RESULT_FILE" > /dev/null 2>&1; then
  EXIT_CODE=$(jq -r '.exit_code' "$RESULT_FILE")
else
  EXIT_CODE=1
fi
```

This is functionally correct. The sandbox `--json` output uses `install_exit_code` (`sandboxJSONOutput` in `cmd/tsuku/install_sandbox.go:21`), but when the sandbox call fails during plan generation (e.g., exit code 8 for missing dependencies), the error falls through to `handleInstallError` which emits `installError` with `exit_code` (`cmd/tsuku/install.go:338`). The `blocked_by` / `missing_recipes` extraction at line 287 depends on this fallback path.

However, `recipe-validation-core.yml` (line 160) and `test-recipe.yml` (line 159) only read `.install_exit_code` from the sandbox JSON and don't handle the `exit_code` fallback. This works for those workflows because they don't need to handle exit code 8 / missing dependency cases. But it introduces two different JSON parsing patterns across the migrated workflows.

This is **advisory, not blocking**, because: (a) the dual-format detection is necessitated by the distinct Go types for success vs. error JSON output, (b) the simpler workflows genuinely don't need the fallback, and (c) the comment on line 276 documents the reason. No other code will copy the batch-generate pattern unless it also needs `missing_recipes` extraction. The two patterns don't diverge -- they handle different output shapes from the same CLI.

### 2. validate-golden-execution.yml sandbox call doesn't use --json

The `validate-linux-containers` job at line 653 runs:

```bash
./tsuku install --sandbox --force --plan "$file" \
  --target-family "$FAMILY" \
  --env GITHUB_TOKEN="$GITHUB_TOKEN"
```

This omits `--json`, while all other migrated sandbox calls in `batch-generate.yml`, `recipe-validation-core.yml`, and `test-recipe.yml` use `--json` for structured output. The golden execution job checks the exit code of the process directly (via `if !` on line 653) rather than parsing JSON results, which is sufficient for its binary pass/fail reporting.

This is **advisory** because the job only needs pass/fail (no per-recipe exit code aggregation, no retry on exit code 5, no `blocked_by` extraction). Using `--json` and parsing results would add complexity without benefit. The non-`--json` invocation is the simpler and correct choice for this use case. If the job ever needs retry or richer reporting, it should add `--json` at that time.

### 3. validate-linux-containers subshell may lose failed_tools state

At line 650, the `while` loop runs in a pipeline (`echo "$FILE_LIST" | while ...`), which means it executes in a subshell. Writes to `/tmp/failed_tools` inside the subshell persist (since it's a file, not a variable), so this works correctly. However, the pattern is fragile -- a refactoring that switched to a variable-based accumulator would silently lose failures. This is contained to a single step and not a structural concern.
