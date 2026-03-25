# Pragmatic Review: Issue 4 — feat(install): add hook registration to install script

## Diff scope

Single file changed: `website/install.sh`

New lines: argument parsing for `--no-hooks` (lines 8-9, 21-23), `INSTALL_HOOKS` variable (line 14), and the hook registration block (lines 207-232).

---

## Findings

### [BLOCKING] `tsuku hook install` failure aborts the whole install under `set -e`

**File**: `website/install.sh`, line 225

```bash
"$BIN_DIR/tsuku" hook install --shell="$DETECTED_SHELL"
```

The script runs under `set -euo pipefail` (line 2). If `tsuku hook install` exits non-zero for any reason — a write error, a transient failure, a permissions problem on the rc file — the entire installer aborts after the binary is already installed. The user is left with tsuku installed but an incomplete setup and a cryptic exit, with no hint that the binary itself is fine.

The binary download and installation (the critical part) has already succeeded by this point. Hook installation is a best-effort step. Wrap the call to prevent a hook failure from failing the install:

```bash
if ! "$BIN_DIR/tsuku" hook install --shell="$DETECTED_SHELL"; then
    echo "WARNING: Hook registration failed. Run 'tsuku hook install' manually to register the command-not-found hook." >&2
fi
```

---

### [ADVISORY] `SHELL_NAME` is set twice from the same source

**File**: `website/install.sh`, lines 138 and 211

`SHELL_NAME=$(basename "$SHELL")` is assigned inside the `MODIFY_PATH=true` block (line 138) and again inside the `INSTALL_HOOKS=true` block (line 211). Both read the same `$SHELL` variable so the result is always identical. The second assignment is dead when `MODIFY_PATH=true`. When `MODIFY_PATH=false`, the first assignment never runs and the second is necessary. The duplication is harmless but could be avoided by hoisting the assignment once before both blocks. Advisory; no behavior change.

---

### [ADVISORY] `tsuku hook install` may double-print confirmation

**File**: `website/install.sh`, line 226

```bash
echo "Registered command-not-found hook for ${DETECTED_SHELL}."
```

The `tsuku hook install` command (Issue 3) prints its own status output per its implementation. The install script then prints a second confirmation line on top of it. Depending on what Issue 3's command prints, users may see two similar success messages for the same operation. Consider either relying on `tsuku hook install`'s own output (and suppressing this echo) or silencing the subcommand's output with `-q` / redirect if a flag is available.

---

## Acceptance criteria check

All eight criteria from the plan are satisfied:
- `$SHELL` read for detection: yes
- `tsuku hook install` called as final step: yes
- `--no-hooks` flag skips call: yes
- Prints which shell configured: yes (line 226)
- `--no-hooks` prints skip message with manual command: yes (lines 229-231)
- Unset or unrecognized `$SHELL` warns and skips without failing: yes (lines 217-222)
- `--no-hooks` documented in usage: yes (lines 8-9)
- `--no-modify-path` behavior unchanged: yes

---

## Summary

One blocking issue: hook install failures abort the installer under `set -e` despite the binary being fully installed. Wrap the call with error handling. Two advisory items around variable duplication and potential double-print output.
