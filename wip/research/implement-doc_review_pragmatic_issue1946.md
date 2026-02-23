# Pragmatic Review: #1946 ci(workflows): migrate recipe-validation-core.yml Linux jobs to sandbox

**0 blocking, 0 advisory.**

## Scope Verification

- **prepare** (lines 14-101): Unchanged. Recipe collection, cross-compilation, artifact upload all intact.
- **validate-linux-x86_64** (lines 104-190): Docker run replaced with `tsuku install --sandbox`. 5 families preserved.
- **validate-linux-arm64** (lines 192-279): Docker run replaced with `tsuku install --sandbox`. 4 families preserved (no arch).
- **validate-darwin-arm64** (lines 281-348): Untouched. Still uses `gtimeout`, native runner, no sandbox.
- **validate-darwin-x86_64** (lines 350-417): Untouched. Same native pattern.
- **report** (lines 419-538): Untouched. Same JSON aggregation, auto-constraint generation, PR creation.

## What Changed (Linux Jobs)

Each Linux job now:
1. Invokes `./tsuku-linux-{amd64,arm64} install --sandbox --force --recipe "$recipe_path" --target-family "$family" --env GITHUB_TOKEN="$GITHUB_TOKEN" --json > "$RESULT_FILE" 2>/dev/null || true`
2. Parses JSON result with `jq .install_exit_code` for retry decisions (exit code 5 = network error, up to 3 attempts)
3. Builds the same `{recipe, platform, status, exit_code, attempts}` JSON result format consumed by the report job

Retry logic preserved: exponential backoff (`sleep $((2 ** (attempt + 1)))`), 3 attempts max, exit code 5 triggers retry on attempts 0 and 1, all other failures break immediately.

## What Was Removed

- All `docker run` blocks (per-family package installation, volume mounting, exit code marker files)
- Docker image references and container management

No docker references remain in the file.

## Duplication Between Linux Jobs

The x86_64 and arm64 jobs share identical shell logic, differing only in binary name (`tsuku-linux-amd64` vs `tsuku-linux-arm64`), runner (`ubuntu-latest` vs `ubuntu-24.04-arm`), family list (5 vs 4), and output file suffix. This is expected duplication for workflow matrix jobs and not worth extracting.

## No Findings

The change is minimal and scoped precisely to the issue: replace docker run with sandbox calls in Linux jobs, preserve retry logic and JSON result format, leave macOS and report jobs alone.
