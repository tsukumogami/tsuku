# QA Validation: Issue 5

Issue: `test(hooks): add container shell integration tests`
Branch: `docs/shell-integration-auto-install`
Date: 2026-03-24

## Build and Static Analysis

**go build ./...**: PASS (no output, exit 0)
**go vet ./internal/hook/...**: PASS (no output, exit 0)

## Infrastructure Checks

**mock_tsuku exists and is executable**: PASS
- Path: `internal/hooks/testdata/mock_tsuku`
- Permissions: `-rwxrwxr-x`
- Content: minimal sh stub that handles `suggest <command>` and prints `Command '<cmd>' not found. Install with: tsuku install <cmd>`

**_TSUKU_BASH_HOOK_LOADED guard in tsuku.bash**: PASS
- Lines 5-7 of `internal/hooks/tsuku.bash` check `${_TSUKU_BASH_HOOK_LOADED:-}` and return early if set to `1`
- Line 26 sets the guard variable after registering the handler
- This guard is what scenario-24 relies on to prevent double-sourcing

**container-images.json exists**: PASS
- File present at repo root; `debianImage()` helper in test reads the `debian` key to resolve the pinned image reference

**skipIfNoDocker present on all Docker tests**: PASS
- All five Docker-dependent tests (scenarios 21-26) call `skipIfNoDocker(t)` as their first statement
- `skipIfNoDocker` calls `exec.Command("docker", "info").Run()` and calls `t.Skipf` on error
- Docker was available in this environment so all tests ran rather than skipped

---

## scenario-21

**Test**: `TestHookBash_NoPreExistingHandler`
**Result**: PASS
**Evidence**: Test ran and passed in 0.38s. Script sources `tsuku.bash` with no pre-existing `command_not_found_handle`, then invokes `jq`. The mock tsuku binary prints `Command 'jq' not found. Install with: tsuku install jq`. Test asserts `strings.Contains(out, "Command 'jq' not found.")` which is satisfied.

---

## scenario-22

**Test**: `TestHookBash_WrapsExistingHandler`
**Result**: PASS
**Evidence**: Test ran and passed in 0.37s. Script defines `command_not_found_handle() { echo "original-handler-called"; }` before sourcing `tsuku.bash`. The hook wraps via `eval "$(declare -f command_not_found_handle | sed 's/^command_not_found_handle/__tsuku_original_command_not_found_handle/')"`. After invoking `jq`, both `Command 'jq' not found.` and `original-handler-called` appear in output, confirming both handlers fired without the original being clobbered.

---

## scenario-23

**Test**: `TestHookBash_RecursionGuard`
**Result**: PASS
**Evidence**: Test ran and passed in 0.35s. Script sources `tsuku.bash` (registering handler), then resets `PATH` to exclude `/tmp/bin` (where mock tsuku lives). When `jq` is invoked, `command -v tsuku` inside the handler fails, so `tsuku suggest` is never called. The assertion `!strings.Contains(out, "Command 'jq' not found.")` is satisfied, confirming the guard prevented the suggestion. Note: the test comment says "prevents any call to tsuku suggest when tsuku is not in PATH" which matches the `command -v tsuku` guard on lines 12 and 20 of `tsuku.bash`.

---

## scenario-24

**Test**: `TestHookBash_DoubleSource`
**Result**: PASS
**Evidence**: Test ran and passed in 0.42s. Script sources `tsuku.bash` twice. The `_TSUKU_BASH_HOOK_LOADED` guard in `tsuku.bash` (lines 5-7) causes the second source to return immediately without re-registering the handler. After invoking `jq`, `strings.Count(out, "Command 'jq' not found.")` equals exactly 1. No nested wrapper, no duplicate output.

---

## scenario-25

**Test**: `TestHookZsh`
**Result**: PASS
**Evidence**: Test ran and passed in 3.92s. Container installs zsh via `apt-get`, copies `tsuku.zsh` to the hooks directory, then runs `zsh -c 'source /tmp/tsuku-test/share/hooks/tsuku.zsh; jq'`. Output contains `Command 'jq' not found.`, confirming the zsh `command_not_found_handler` fires and calls mock tsuku.

---

## scenario-26

**Test**: `TestHookFish`
**Result**: PASS
**Evidence**: Test ran and passed in 16.44s (longer due to fish installation via apt). Container installs fish, copies `tsuku.fish`, then runs `fish -c 'source ...; jq'`. Output contains `Command 'jq' not found.`, confirming the fish `fish_command_not_found` handler fires.

---

## scenario-29

**Test**: `TestHookBash_UninstallRestores`
**Result**: PASS
**Evidence**: Test ran directly via `go test -v -run TestHookBash_UninstallRestores ./internal/hook/...` and passed in 0.00s. Test uses `t.TempDir()` for both home directory and share hooks directory (no Docker required). Sequence: write initial `.bashrc` with `# existing content\n`, call `hook.Install("bash", ...)`, verify `# tsuku hook` marker is present, call `hook.Uninstall("bash", ...)`, verify `.bashrc` is byte-for-byte identical to original. All assertions passed.

---

## Summary

All 7 scenarios passed. The build is clean, static analysis has no issues, and all infrastructure prerequisites (mock_tsuku, double-source guard, container-images.json, Docker skip logic) are correctly in place.
