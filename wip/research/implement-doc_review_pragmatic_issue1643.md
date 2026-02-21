# Pragmatic Review: Issue #1643

**Issue**: feat(llm): implement tsuku llm download command
**Focus**: pragmatic (simplicity, YAGNI, KISS)
**Files**: `cmd/tsuku/llm.go`, `cmd/tsuku/llm_test.go`, `cmd/tsuku/main.go`, `internal/llm/local.go`

---

## Findings

### 1. BLOCKING: `--force` flag does not re-download anything

`cmd/tsuku/llm.go:130` -- `--force` bypasses the "already present" early return at line 130, then falls through to prompt the user and call `TriggerModelDownload`. But `TriggerModelDownload` sends a trivial Complete request to the already-running server with the already-loaded model. The server does not re-download or re-verify the model -- it just runs inference on "OK". The addon binary path also comes from `EnsureAddon`, which checks `cachedPath` and returns early if the binary exists.

The flag promises "Re-download even if files already exist" (line 47) but delivers "re-prompt and run a throwaway inference request." Either remove `--force` entirely (YAGNI -- no caller needs forced re-download yet) or wire it to actually re-download. A flag that doesn't do what it says is worse than no flag.

**Fix**: Remove `--force`. If re-download is needed later, add it when the addon gRPC API supports it.

### 2. ADVISORY: `printDownloadSummary` is a single-caller helper that adds little

`cmd/tsuku/llm.go:191-201` -- Called from two places (lines 133 and 186), but the function is 7 lines of `fmt.Fprintf`. It's small and named clearly enough, so this is minor, but inlining at both sites would be equally readable and avoid the scroll.

### 3. ADVISORY: Redundant nil-check on `globalCtx`

`cmd/tsuku/llm.go:60-62` -- `globalCtx` is set in `main()` before any command runs (line 104 of main.go). The cobra framework guarantees `RunE` only executes after `main()` initializes the context. The nil guard adds dead code.

### 4. ADVISORY: Test suite only checks command registration, not behavior

`cmd/tsuku/llm_test.go` -- All tests verify cobra command structure (flag existence, descriptions, RunE presence). None test `runLLMDownload` behavior. The `TriggerModelDownload` tests in `internal/llm/local_test.go` cover the gRPC layer, but the CLI glue (error handling paths, prompt flow, status display) is untested. Not blocking because the CLI layer is thin, but the tests are low-value -- they'll pass even if the command is broken.

---

## Summary

| Severity | Count |
|----------|-------|
| Blocking | 1 |
| Advisory | 3 |

The `--force` flag is the only blocking issue. It claims to re-download but doesn't -- the addon server has no mechanism to force re-download, and `EnsureAddon` caches the binary path. Shipping a flag that silently does nothing misleads CI scripts that rely on it for clean-slate builds. Remove the flag until the infrastructure supports it.

The rest is clean. The command correctly downloads the addon, starts the server, queries status, prompts for model download, triggers model download via `TriggerModelDownload`, and verifies readiness. The `--model` flag was correctly removed (the scrutiny review flagged it as display-only). The prompter reuse from #1642 is appropriate.
