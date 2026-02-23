# Architect Review: #1946 - Migrate recipe-validation-core.yml Linux Jobs to Sandbox

**0 blocking, 0 advisory.**

## Summary

The change correctly migrates the two Linux validation jobs (`validate-linux-x86_64` and `validate-linux-arm64`) from direct docker run calls to `tsuku install --sandbox` while preserving every structural contract the report job and downstream consumers depend on.

## Verification Checklist

### No docker run remains
Confirmed. Zero matches for `docker run` in the file. No `container-images.json` references either -- the sandbox handles image selection internally via `--target-family`, which is the correct delegation.

### --sandbox with --json/--env/--target-family used
Confirmed at lines 154-157 (x86_64) and 243-246 (arm64). The invocation pattern matches the design doc exactly:

```bash
./tsuku-linux-amd64 install --sandbox --force --recipe "$recipe_path" \
  --target-family "${{ matrix.family }}" \
  --env GITHUB_TOKEN="$GITHUB_TOKEN" \
  --json > "$RESULT_FILE" 2>/dev/null || true
```

This also matches the pattern established by `test-recipe.yml` (#1945), so there's no parallel pattern divergence between the two migrated workflows.

### Retry logic preserved with exit code 5 check
Confirmed. The retry loop at lines 150-176 (x86_64) and 239-265 (arm64) retains:
- Three attempts (`for attempt in 0 1 2`)
- Exit code extraction from sandbox JSON (`jq -r '.install_exit_code'`)
- Exit code 5 check with `attempt -lt 2` guard
- Exponential backoff (`sleep $((2 ** (attempt + 1)))`)
- Fallback to `EXIT_CODE=1` when JSON parsing fails

The retry wrapping is consistent with the design's intent: "Retry logic and result aggregation stay in the workflow layer but consume sandbox JSON output instead of raw exit codes."

### JSON result aggregation format unchanged for report job
Confirmed. Both Linux jobs produce the same `{recipe, platform, status, exit_code, attempts}` object shape:

```bash
RESULTS=$(echo "$RESULTS" | jq --arg r "$recipe_name" --arg p "$PLATFORM" \
  --arg s "$STATUS" --argjson e "$EXIT_CODE" --argjson a "$ATTEMPTS" \
  '. + [{"recipe": $r, "platform": $p, "status": $s, "exit_code": $e, "attempts": $a}]')
```

The artifact names (`validation-results-linux-$family-$arch.json`) and the report job's `jq -s 'add' validation-results-*.json` aggregation at line 438 are unchanged.

### macOS jobs untouched
Confirmed. `validate-darwin-arm64` (lines 282-348) and `validate-darwin-x86_64` (lines 351-417) use the same native runner pattern: `gtimeout 300 ./tsuku-darwin-* install --force --recipe`, no `--sandbox`, no `--json`, no `--target-family`. The report job, auto-constraint generation, and PR creation steps are also unchanged.

## Architectural Fit

This change follows the migration pattern established by #1945 (test-recipe.yml). Both workflows now use the same sandbox invocation shape for Linux jobs. The key structural properties hold:

1. **No bypass of the sandbox abstraction.** Container image selection, package installation, and volume mounting are delegated to the sandbox. The workflow layer handles only retry, aggregation, and reporting.

2. **Consistent contract with the report job.** The JSON artifact format is unchanged, so the report job's aggregation, summary rendering, and auto-constraint generation all work without modification.

3. **Clean separation between Linux (sandboxed) and macOS (native).** macOS jobs remain native-runner installs with `gtimeout`. This matches the design's scope: "macOS CI jobs are out of scope because macOS tests run on native runners (no containers)."

No structural issues found.
