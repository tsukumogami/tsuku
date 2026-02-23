# Maintainer Review: Issue #1945 -- test-recipe.yml sandbox migration

**0 blocking, 2 advisory.**

## Summary

This commit replaces Docker-based Linux testing in `test-recipe.yml` with the sandbox CLI (`tsuku install --sandbox --json --target-family`). The workflow now has four jobs: two Linux sandbox jobs (x86_64 and arm64), and two macOS native runner jobs (arm64 and x86_64). The Linux jobs iterate over families and parse structured JSON from the sandbox to determine pass/fail.

The overall structure is clear. The header comment (lines 1-13) accurately describes what the workflow does and its error philosophy. The `--json` output contract is well-defined in `sandboxJSONOutput` (in `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/install_sandbox.go:17-25`), and the workflow correctly parses `.passed` and `.install_exit_code` from it.

## Findings

### 1. Divergent twins: test-linux-x86_64 and test-linux-arm64 are ~70 lines of near-identical shell

**Advisory.**

`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/.github/workflows/test-recipe.yml` lines 128-198 vs 217-288.

The two Linux jobs differ in exactly three things:
- The binary name (`tsuku-linux-amd64` vs `tsuku-linux-arm64`)
- The FAMILIES array (`"debian" "rhel" "arch" "suse" "alpine"` vs `"debian" "rhel" "suse" "alpine"`)
- The summary header and architecture label strings

Everything else -- the counter variables, the jq parsing, the result file naming, the markdown summary generation -- is duplicated line for line. The next developer who needs to change the JSON field name from `.passed` to something else, or add a new column to the summary table, must update both blocks identically.

This isn't blocking because the blocks are in separate jobs (so a keyword search will find both), and the differences are intentional (no arch arm64 image, different binary). But it's the kind of thing where a future change to one block but not the other creates a silent behavior difference.

If you want to reduce the risk, one option is extracting the shell body into `.github/scripts/test-recipe-linux.sh` that takes binary path and family list as arguments. That way there's one copy of the parsing logic and the jobs just call the script with different parameters.

### 2. Silent swallowing of stderr makes sandbox failures harder to debug

**Advisory.**

`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/.github/workflows/test-recipe.yml` lines 155 and 245:

```bash
./tsuku-linux-amd64 install --sandbox --force --recipe "$recipe_path" \
  --target-family "$family" \
  --env GITHUB_TOKEN="$GITHUB_TOKEN" \
  --json > "$RESULT_FILE" 2>/dev/null || true
```

The `2>/dev/null` discards all stderr. If the sandbox fails to start (e.g., no container runtime, image pull failure, permission error), the JSON result file will be empty or absent, and the workflow falls through to the `else` branch on line 160/250 which sets `SANDBOX_PASSED="false"` and `EXIT_CODE=1`. The developer debugging the failure sees `FAIL: recipe on family (exit 1)` with no indication of what actually went wrong.

The `--json` mode in `emitSandboxJSON` already suppresses human-readable output from stdout. The stderr is where diagnostic messages (container pull progress, sandbox execution errors) would appear. Redirecting stderr to a per-test log file instead of `/dev/null` would let the `::group::` blocks contain useful debugging info:

```bash
STDERR_FILE=".stderr-${recipe_name}-${family}.log"
./tsuku-linux-amd64 install --sandbox ... \
  --json > "$RESULT_FILE" 2>"$STDERR_FILE" || true

# After parsing result, dump stderr for context
if [ -s "$STDERR_FILE" ]; then
  echo "--- stderr ---"
  cat "$STDERR_FILE"
fi
```

This is advisory because the JSON result's `.error` field should contain the meaningful error message for most failure modes. But infrastructure failures (image pull timeout, OOM kill) may not produce a JSON result at all, and those are the hardest to debug.

## What's clear

- The header comment (lines 1-13) accurately describes the error philosophy: platform failures indicate which `when` filters to add, not required fixes.
- The `continue-on-error: true` at the job level matches that philosophy.
- The JSON parsing is defensive: the `if [ -f "$RESULT_FILE" ] && jq -e . "$RESULT_FILE"` guard (lines 157, 247) correctly handles both missing and malformed JSON.
- The macOS jobs (lines 290-440) remain as native runner tests without sandbox, which is correct since sandbox is Linux-container-only.
- The family lists match `container-images.json` (5 families for x86_64, 4 for arm64 minus arch).
- The `--env GITHUB_TOKEN` passthrough prevents rate-limiting for recipes that download from GitHub.
