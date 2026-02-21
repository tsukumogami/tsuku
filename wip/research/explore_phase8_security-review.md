# Security Review: DESIGN-requeue-on-recipe-merge.md

## Scope

Review of the Security Considerations section in the design document. The design replaces `scripts/requeue-unblocked.sh` (bash + jq) with a Go tool (`cmd/queue-maintain/`) that reads failure JSONL, checks recipe existence via queue status instead of filesystem lookups, and modifies a JSON queue file.

## Findings

### 1. "Not applicable" justifications are correct

**Download verification**: Correct. The tool reads/writes JSON files in the repo working tree. No downloads.

**User data exposure**: Correct. The data contains package names, ecosystem IDs, timestamps, and error messages. No credentials or personal information.

### 2. "Reduces attack surface" claim: justified, with a caveat

The design says (line 310):
> Since the queue status check replaces the filesystem check, there's no longer a code path where blocker names are used to construct file paths -- reducing the attack surface compared to the current bash script.

This is accurate. The bash script's `recipe_exists` function (line 48-53 of `requeue-unblocked.sh`) does:
```bash
local first="${name:0:1}"
first="$(echo "$first" | tr '[:upper:]' '[:lower:]')"
[[ -f "$RECIPES_DIR/$first/$name.toml" ]] || [[ -f "$EMBEDDED_DIR/$name.toml" ]]
```

The `$name` value comes from `blocked_by` fields in failure JSONL, which flow through `jq` without validation in the bash script. While `isValidDependencyName()` in `orchestrator.go:545` rejects `/`, `\`, `..`, `<`, `>` at the point where blocker names are *written* to failure records, the bash script doesn't enforce this itself -- it trusts what's in the JSONL. If a JSONL file were manually modified or produced by an older code path that lacked validation, the bash script would blindly use it in a path.

The Go tool eliminates this class entirely: blocker names are used as map keys for queue-status lookup, never interpolated into filesystem paths. The claim is justified.

**Caveat**: The design should note that `isValidDependencyName()` comment at `orchestrator.go:542-543` still references `requeue-unblocked.sh` as the downstream consumer. After the migration, this comment should be updated since the filesystem path construction reason is gone. The validation is still useful for general sanity, but the stated rationale will be stale.

### 3. bufio.Scanner line length limit (existing risk, not introduced by this design)

`internal/reorder/reorder.go:175` uses `bufio.NewScanner(file)` with the default buffer (64KB max line length). The failure JSONL files are one-line-per-file records: `homebrew-2026-02-21T07-22-39Z.jsonl` has a single line with ~17KB of content for ~43 failures. A batch with 200+ failures per ecosystem could exceed 64KB, causing `scanner.Scan()` to silently return false (the error would be `bufio.ErrTooLong` on `scanner.Err()`).

Since `loadBlockersFromFile` returns `scanner.Err()` and the caller (`loadBlockerMap`) does `continue` on error, an oversized JSONL file would be silently skipped, meaning those blockers wouldn't be counted. Packages that should stay blocked could be incorrectly requeued.

This is a pre-existing issue in `internal/reorder/` but the design proposes sharing this exact code for the requeue path. Worth calling out in the design as a known limitation to fix.

**Severity**: Advisory for the design review (pre-existing). Would be blocking if the implementation doesn't address it.

**Fix**: Add `scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)` to allow up to 1MB lines. Or switch to `json.NewDecoder` which handles arbitrary line lengths.

### 4. Unbounded blocker map growth

`loadBlockerMap` in `internal/reorder/reorder.go:147` reads all `*.jsonl` files in the failures directory and accumulates entries into an unbounded map. With 50 failure files (current count) each containing 40+ entries with multiple `blocked_by` values, this is fine. But the failures directory grows over time (timestamped filenames, never pruned by this tool). After months of batch runs, loading hundreds of stale failure files wastes memory and could cause stale blocker data to prevent requeuing.

This is out of scope for the design (the reorder tool has the same issue today), but the design should mention whether failure file pruning is expected. The requeue logic's correctness depends on the failure data being reasonably current.

**Severity**: Advisory. Not a security vulnerability but an operational concern that affects correctness.

### 5. No new attack vectors from the design

The tool:
- Reads JSON files from the local repo checkout (no network)
- Writes JSON back to the same checkout (no remote)
- Runs in GitHub Actions with `contents: write` permission (same as current)
- Uses a GitHub App token only for git push (already established pattern)

The `blocked_by` values in failure JSONL are validated at write time by `isValidDependencyName()`, and the new code only uses them as map keys (not file paths). Even if validation were bypassed, the worst case is a map key with unusual characters, which has no security impact in a Go `map[string][]string`.

The queue file is a single JSON blob read/written atomically (read all, modify in memory, write all). No TOCTOU issues.

### 6. Execution isolation claim is accurate

The design states "No network access, no shell execution, no elevated permissions." This is correct for the Go binary itself. It inherits whatever environment GitHub Actions provides, but the tool doesn't make outbound requests or exec subprocesses.

### 7. Malformed JSONL data resilience

The current `loadBlockersFromFile` handles malformed data gracefully:
- Empty lines: skipped (line 178-180)
- Invalid JSON: skipped via `json.Unmarshal` error (line 183-185)
- Missing fields: Go zero values, so nil `BlockedBy` arrays and empty `PackageID` strings are harmless

Extremely long `blocked_by` arrays: no limit in the reader. `extractBlockedByFromOutput` caps at 100 items when *writing* blocker names, but the reader has no cap. A manually crafted JSONL with thousands of `blocked_by` entries would cause proportional map growth. Practically irrelevant since the data is written by trusted CI code, not user input.

Special characters in names: validated at write time by `isValidDependencyName()`. Even if bypassed, the new code uses names only as map keys -- no injection risk.

## Summary Table

| Finding | Severity | Action |
|---------|----------|--------|
| "Not applicable" justifications | OK | No change needed |
| "Reduces attack surface" claim | OK | Justified; update `isValidDependencyName` comment post-migration |
| bufio.Scanner 64KB limit | Advisory | Note in design; fix during implementation |
| Unbounded failure file loading | Advisory | Document expected pruning strategy |
| No new attack vectors | OK | No change needed |
| Malformed JSONL resilience | OK | Existing handling is adequate |

## Conclusion

The security section is accurate and the "not applicable" justifications are correct. The "reduces attack surface" claim is well-founded -- replacing filesystem path construction with map-key lookup is a real improvement. The two advisory findings (scanner buffer limit, unbounded failure loading) are pre-existing issues in the code being reused, not introduced by this design.
