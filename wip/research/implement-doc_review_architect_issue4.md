# Architect Review: Issue 4 — feat(install): add hook registration to install script

**File reviewed:** `website/install.sh`
**Review focus:** architect (design patterns, separation of concerns)
**Source doc:** `docs/plans/PLAN-command-not-found.md` / `docs/designs/DESIGN-command-not-found.md`

---

## Summary

The implementation fits the architecture correctly. No blocking findings.

---

## Structural Analysis

### 1. Delegation to `tsuku hook install` — CORRECT

The install script does not implement hook registration logic itself. Line 225:

```bash
"$BIN_DIR/tsuku" hook install --shell="$DETECTED_SHELL"
```

This is the correct structural boundary. The install script is the delivery vehicle; `tsuku hook install` owns hook registration. This respects the design's component boundary (Phase 4 calls Phase 3, doesn't duplicate it).

### 2. Pattern consistency with `--no-modify-path` — CORRECT

The `--no-hooks` flag follows the same structural pattern as `--no-modify-path`:

- Flag parsed at the top into a boolean variable (`INSTALL_HOOKS`, lines 14–22)
- Checked at execution time with a matching `if/else` block (lines 208–232)
- Skip path prints a message and the manual command to the user

This is consistent with the pre-existing `MODIFY_PATH` pattern. No parallel pattern introduced.

### 3. Shell detection in the hook block — CORRECT

The `MODIFY_PATH` block derives `SHELL_NAME` inside its branch (line 138). The hook block independently re-derives `DETECTED_SHELL` from `$SHELL` inside its branch (lines 209–222).

These two blocks don't share state, which is intentional — either block may be skipped independently via `--no-modify-path` or `--no-hooks`. The hook block being self-contained means it works correctly when `--no-modify-path` is passed but hooks are still enabled.

The hook block also handles fish (line 213: `bash|zsh|fish`) while the `MODIFY_PATH` block intentionally does not (fish has its own PATH management). This is not a divergence — the design explicitly calls for fish support in hook registration but not in PATH setup.

### 4. Guard for unset/unrecognized `$SHELL` — CORRECT

Lines 210–222 correctly handle:
- `$SHELL` unset: warns and skips without failing
- `$SHELL` set to an unrecognized value: warns and skips without failing

Both cases print the manual fallback command (`tsuku hook install`). This matches the acceptance criteria and the design's "warn and skip" requirement.

### 5. Potential double-output on success — ADVISORY

Line 226 prints "Registered command-not-found hook for ${DETECTED_SHELL}." immediately after calling `tsuku hook install`. `tsuku hook install` (implemented in Issue 3) likely also prints confirmation output. The combined output may be:

```
[output from tsuku hook install]
Registered command-not-found hook for bash.
```

This is not a structural violation — the install script is allowed to add its own framing — but it could produce redundant messages depending on what `tsuku hook install` prints. The acceptance criteria says "Install output prints which shell was configured" without specifying whether the tsuku binary or the install script is the source. Given that the install script controls the install UX and `tsuku hook install` is a lower-level operation, the current approach is reasonable.

This is advisory and can be resolved when Issue 3's output is finalized, if needed.

---

## Verdict

No blocking architectural issues. The implementation correctly delegates hook logic to `tsuku hook install`, follows the existing `--no-modify-path` structural pattern, and handles fish as a distinct case without duplicating PATH-setup logic. The separation between the install script (delivery) and `tsuku hook install` (registration) is preserved.
