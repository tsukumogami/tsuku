# Pragmatic Review: #1945 ci(workflows): migrate test-recipe.yml Linux jobs to sandbox

**0 blocking, 0 advisory.**

## Scope Verification

- **Build job** (lines 34-109): Unchanged. Cross-compilation, artifact upload, recipe detection all intact.
- **test-linux-x86_64** (lines 111-198): Docker run replaced with `tsuku install --sandbox`. 5 families preserved.
- **test-linux-arm64** (lines 200-288): Docker run replaced with `tsuku install --sandbox`. 4 families preserved (no arch).
- **test-darwin-arm64** (lines 290-364): Untouched. Still uses `gtimeout`, `TSUKU_HOME`, `brew install coreutils`, native runner.
- **test-darwin-x86_64** (lines 366-441): Untouched. Same native pattern.

## What Changed (Linux Jobs)

Each Linux job now:
1. Invokes `./tsuku-linux-{amd64,arm64} install --sandbox --force --recipe "$recipe_path" --target-family "$family" --env GITHUB_TOKEN="$GITHUB_TOKEN" --json > "$RESULT_FILE" 2>/dev/null || true`
2. Parses JSON result with `jq` for `.passed` and `.install_exit_code`
3. Builds the same `$GITHUB_STEP_SUMMARY` markdown table as before

This matches the design doc's "After" example exactly (DESIGN-sandbox-ci-integration.md lines 414-417).

## What Was Removed

- All `docker run` blocks (per-family package installation, volume mounting, exit code marker files)
- Docker image references and container management

No docker references remain in the file.

## Duplication Between Linux Jobs

The x86_64 and arm64 jobs share identical shell logic, differing only in binary name, runner, family list, and labels. This is expected duplication for workflow jobs and not worth extracting -- doing so would be scope creep.

## No Findings

The change is minimal and scoped precisely to the issue: replace docker run with sandbox calls in Linux jobs, leave everything else alone.
