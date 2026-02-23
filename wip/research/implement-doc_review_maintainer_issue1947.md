# Maintainer Review: Issue #1947

## Blocking Findings

None

## Advisory Findings

### 1. Divergent twins now at 8 copies across 3 workflows

`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/.github/workflows/batch-generate.yml:264` (and lines 416, 563, 693)

The retry-loop-with-exit-code-parsing block is now copied 8 times across `batch-generate.yml` (4 copies), `recipe-validation-core.yml` (4 copies), and `test-recipe.yml` (2 copies). These copies are not identical -- they have three divergent patterns for extracting the exit code:

1. **Linux sandbox jobs** (batch-generate x86_64/arm64, recipe-validation-core, test-recipe): Parse JSON for `.install_exit_code` from sandbox output. batch-generate also falls back to `.exit_code` for the `handleInstallError` JSON format; the other workflows do not.
2. **macOS non-sandbox jobs** (batch-generate darwin-arm64/darwin-x86_64): Use process `$?` for exit code, capture both stdout and stderr into the same JSON file with `2>&1`.
3. **All three Linux variants**: Suppress stderr with `2>/dev/null`.

The next developer modifying the retry logic (e.g., adding exit code 9 handling for deterministic failures) will update some copies and miss others. The macOS/Linux behavioral difference is intentional (macOS doesn't use --sandbox) but the batch-generate dual-format fallback (`.install_exit_code` then `.exit_code`) exists only in batch-generate and not in test-recipe or recipe-validation-core. This inconsistency is the most likely source of confusion.

This is a repeat of findings from #1945 and #1946. I'm noting it for tracking but recognize consolidation may be a separate effort.

### 2. Stale comment in validate-golden-execution.yml

`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/.github/workflows/validate-golden-execution.yml:637`

```yaml
          # Filter and format files for this family (parse JSON outside container)
```

The parenthetical "parse JSON outside container" is a leftover from when this job used `docker run` directly. Now the sandbox is abstracted behind `tsuku install --sandbox`, and the JSON parsing happens on the host to filter the file list -- not to avoid running jq inside a container. A next developer reading this will wonder what "outside container" means when there's no explicit container invocation. Suggest changing to:

```yaml
          # Filter files for this family from the JSON list
```

### 3. Stderr silenced with `2>/dev/null` on sandbox calls (repeat finding)

`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/.github/workflows/batch-generate.yml:271` and line 423

```bash
                  --json > "$RESULT_FILE" 2>/dev/null || true
```

Same pattern flagged in #1945 and #1946. If the sandbox binary fails to start (e.g., missing container runtime, permissions error), stderr holds the diagnostic but is discarded. The JSON file will be empty, `EXIT_CODE` falls through to `1`, and the recipe is marked "fail" with no indication of what went wrong. The validate-golden-execution.yml job does *not* suppress stderr (line 653-655), which is the correct approach.
