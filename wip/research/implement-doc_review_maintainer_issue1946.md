# Maintainer Review: Issue #1946 -- recipe-validation-core.yml Linux sandbox migration

**0 blocking, 3 advisory.**

## Summary

This commit replaces Docker-based Linux validation in `recipe-validation-core.yml` with `tsuku install --sandbox --json --target-family`. The two Linux jobs (`validate-linux-x86_64` and `validate-linux-arm64`) now use the same sandbox CLI pattern as `test-recipe.yml`, with one important addition: retry logic for network errors (exit code 5, up to 3 attempts with exponential backoff). The report job, auto-constraint step, and macOS jobs are unchanged.

The migration is structurally clean. The retry logic is easy to follow. The `--json` output contract is consumed correctly: `install_exit_code` is the container's inner tsuku exit code, so checking for `5` (the `ExitNetwork` constant from `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/exitcodes.go:24`) is semantically correct for retrying network failures inside the sandbox.

## Findings

### 1. Divergent twins: validate-linux-x86_64 and validate-linux-arm64 are ~55 lines of near-identical shell

**Advisory.**

`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/.github/workflows/recipe-validation-core.yml` lines 129-183 vs 218-272.

The two Linux jobs differ in exactly three things:
- The binary name (`tsuku-linux-amd64` vs `tsuku-linux-arm64`)
- The matrix family list (`[debian, rhel, arch, suse, alpine]` vs `[debian, rhel, suse, alpine]`)
- The architecture suffix in the platform string and output filename (`x86_64` vs `arm64`)

The retry loop, JSON parsing, result aggregation, and backoff timing are duplicated verbatim. This is the same pattern flagged as advisory in issue #1945's `test-recipe.yml` review. Now there are four near-identical shell blocks across two workflows (two in `test-recipe.yml`, two here). A future change to the JSON field name, retry threshold, or backoff strategy requires updating all four in lockstep.

This stays advisory because the blocks are in clearly labeled jobs, searchable by field name, and the differences are intentional. But the duplication surface area is growing -- four copies is the point where extracting a shared script (e.g., `.github/scripts/sandbox-validate.sh` accepting binary path, family list, and architecture as arguments) starts paying for itself.

### 2. Divergent JSON field usage between test-recipe.yml and recipe-validation-core.yml

**Advisory.**

`test-recipe.yml` (lines 158, 165) determines pass/fail by reading the `.passed` boolean field from the sandbox JSON output:
```bash
SANDBOX_PASSED=$(jq -r '.passed' "$RESULT_FILE")
if [ "$SANDBOX_PASSED" = "true" ]; then
```

`recipe-validation-core.yml` (lines 160, 165) determines pass/fail by checking `.install_exit_code`:
```bash
EXIT_CODE=$(jq -r '.install_exit_code' "$RESULT_FILE")
if [ "$EXIT_CODE" = "0" ]; then
```

These are semantically equivalent today -- `buildSandboxJSONOutput` in `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/install_sandbox.go:159-197` sets `passed = true` exactly when `install_exit_code == 0`. But the next developer maintaining both workflows will see two different approaches to the same question ("did the install succeed?") and wonder if there's a case where they disagree.

The recipe-validation-core.yml approach is arguably better here because it needs the exit code anyway for retry decisions (checking for code 5). But the inconsistency across workflows is a readability trap. Consider using `.passed` for the pass/fail decision and `.install_exit_code` only for the retry check, matching the pattern in test-recipe.yml. Or add a one-line comment explaining why `install_exit_code` is used instead of `passed`.

### 3. Silent swallowing of stderr with `2>/dev/null`

**Advisory.**

`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/.github/workflows/recipe-validation-core.yml` lines 157 and 246:

```bash
./tsuku-linux-amd64 install --sandbox --force --recipe "$recipe_path" \
  --target-family "${{ matrix.family }}" \
  --env GITHUB_TOKEN="$GITHUB_TOKEN" \
  --json > "$RESULT_FILE" 2>/dev/null || true
```

Same pattern flagged in the #1945 review. When the sandbox fails to start (image pull failure, no container runtime, OOM), no JSON file is produced. The workflow falls through to `EXIT_CODE=1` and `STATUS="fail"` with no diagnostic output. The developer debugging the failure sees the recipe marked as failed but has no way to distinguish "recipe doesn't work on this platform" from "sandbox infrastructure broke."

In a retry context this matters more: if the sandbox infrastructure is down, every recipe will burn through 3 retry attempts (with backoff sleeps of 2s, 4s) before reporting failure. For a workflow validating dozens of recipes across 9 Linux matrix entries, silent infrastructure failures compound into long, uninformative runs.

Redirecting stderr to a file instead of `/dev/null` and dumping it on failure would make infrastructure problems immediately visible in the logs:

```bash
STDERR_FILE=".stderr-${recipe_name}.log"
./tsuku-linux-amd64 install --sandbox ... \
  --json > "$RESULT_FILE" 2>"$STDERR_FILE" || true

# On failure, show stderr for diagnostics
if [ "$STATUS" = "fail" ]; then
  [ -s "$STDERR_FILE" ] && echo "--- stderr ---" && cat "$STDERR_FILE"
fi
```

## What's clear

- The retry logic (lines 150-176, 239-265) is straightforward: try up to 3 times, retry only on exit code 5 (network error), exponential backoff, break on any other outcome. The `attempt` variable counting from 0 with the display offset `$((attempt+2))` is mildly awkward but correct.
- The platform string construction (`linux-$family-$libc-$arch`) matches the format the report job expects and produces meaningful artifact names.
- The libc derivation (`LIBC="glibc"` with an alpine override to `musl`) is correct and documented with a comment.
- The matrix strategy differs correctly between x86_64 (5 families including arch) and arm64 (4 families, no arch), matching `container-images.json` and the test-recipe.yml pattern.
- The result JSON schema (`{"recipe", "platform", "status", "exit_code", "attempts"}`) is consumed correctly by the report job's `jq` filters.
- The macOS jobs and report job are unchanged by this migration, which is correct -- they don't use sandbox and their contract with the report job is preserved.
