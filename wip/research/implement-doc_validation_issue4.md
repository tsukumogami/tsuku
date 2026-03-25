# QA Validation: Issue 4

Validated via static analysis of `website/install.sh` (branch `docs/shell-integration-auto-install`, commit `cf3ab9a4`) and `bash -n` syntax check. Full live execution is environment-dependent (requires real binary download at runtime).

---

## scenario-18
**Result**: PASS
**Evidence**: Lines 208-232 of install.sh. When `INSTALL_HOOKS=true` (the default, since `--no-hooks` was not passed) and `$SHELL` is set to a recognized shell (bash, zsh, or fish), the script enters the `if [ -n "$DETECTED_SHELL" ]` branch (line 225) and calls `"$BIN_DIR/tsuku" hook install --shell="$DETECTED_SHELL"`. On a successful exit code from that command (line 227), the script prints:

```
Registered command-not-found hook for ${DETECTED_SHELL}.
```

This matches the scenario expectation that "Registered command-not-found hook for bash." (or the detected shell) appears in install output. The default path sets `INSTALL_HOOKS=true` (line 14) and the `--no-hooks` flag is the only way to set it to false (lines 21-23).

---

## scenario-19
**Result**: PASS
**Evidence**: Lines 233-237 of install.sh. When `--no-hooks` is passed, the argument parser at line 22 sets `INSTALL_HOOKS=false`. The outer `if/else` at line 208 evaluates to the `else` branch (lines 233-237), which prints:

```
Skipped hook registration (--no-hooks)
Run 'tsuku hook install' manually to register the command-not-found hook.
```

No `tsuku hook install` call is made. The rc file is never written. The output includes both the skip message and the manual command, matching both expected outputs from the scenario.

---

## scenario-20
**Result**: PASS
**Evidence**: Lines 210-223 of install.sh. When `$SHELL` is unset, the guard `[ -n "${SHELL:-}" ]` at line 210 evaluates to false. The `else` branch at lines 221-223 runs:

```
echo "WARNING: \$SHELL is not set; skipping hook registration. Run 'tsuku hook install' manually to register the hook."
```

`DETECTED_SHELL` remains the empty string set at line 209. The `if [ -n "$DETECTED_SHELL" ]` guard at line 225 then prevents any call to `tsuku hook install`. The script does not exit early — the warning is printed and execution continues past the hook block to the telemetry notice section. Exit code 0 is expected since no `exit` call is present in this path.

Note: The outer `if [ "$INSTALL_HOOKS" = true ]` block (line 208) is entered even when `$SHELL` is unset, because the flag defaults to `true`. The `$SHELL` guard is inside that block, so the behavior is correct regardless of whether `--no-hooks` was passed (in that case the outer else fires first and the $SHELL check is never reached anyway).

---

## scenario-30
**Result**: PASS
**Evidence**: Lines 225-231 of install.sh. The `tsuku hook install` call is wrapped in an `if/else`:

```sh
if "$BIN_DIR/tsuku" hook install --shell="$DETECTED_SHELL"; then
    echo "Registered command-not-found hook for ${DETECTED_SHELL}."
else
    echo "WARNING: Hook registration failed. Run 'tsuku hook install' manually to register the command-not-found hook." >&2
fi
```

If `tsuku hook install` returns a non-zero exit code, the `else` branch prints a warning to stderr and the script continues. There is no `exit` call in the else branch. The script proceeds past line 231 to the telemetry notice section. Since the binary was already installed (lines 110-116) before this block is reached, a hook registration failure does not roll back or abort the install. The use of `set -euo pipefail` at line 2 does NOT cause this failure to abort the script because the failing command is the condition of an `if` statement — per POSIX/bash rules, a command used as an `if` condition is exempt from `set -e` termination.
