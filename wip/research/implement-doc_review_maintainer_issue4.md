---
issue: 4
review_focus: maintainer
---

# Maintainer Review: Issue 4 — feat(install): add hook registration to install script

## Overview

The change adds a hook registration block at the end of `website/install.sh` (lines 207–232). The logic reads `$SHELL`, classifies it into bash/zsh/fish, calls `tsuku hook install --shell=<name>`, and falls through gracefully when the shell is unrecognized or unset.

The overall structure is clear and the acceptance criteria are well-matched. There is one blocking clarity issue and one advisory item.

---

## Findings

### [BLOCKING] `SHELL_NAME` is shadowed silently — the next developer will not see it

**File**: `website/install.sh`, lines 138 and 211

The variable `SHELL_NAME` is assigned in two separate `if` blocks:
- Line 138: `SHELL_NAME=$(basename "$SHELL")` inside the `if [ "$MODIFY_PATH" = true ]` block
- Line 211: `SHELL_NAME=$(basename "$SHELL")` inside the `if [ "$INSTALL_HOOKS" = true ]` block

The two blocks are separate `if` statements with no `else`, and the outer `set -euo pipefail` is in force. The assignment is duplicated rather than reused, which looks intentional — but it isn't obviously so. The next person reading this will ask: "Is there a bug where the hooks block relies on the stale value from the path-configuration block?" and then spend time tracing scope to confirm these are independent writes to the same variable.

More specifically: if `--no-modify-path` is passed, `SHELL_NAME` is never set before line 211. With `set -u`, reading an unset variable would abort the script. But `SHELL_NAME` is only read *after* the second assignment on line 212, so it works. However, if someone later extracts or reorders the hook block — which is the most likely edit a maintainer will make — the latent `set -u` risk becomes real.

**Fix**: Hoist the shell detection to a named variable set unconditionally at the top of the file (alongside `MODIFY_PATH` / `INSTALL_HOOKS`), or at minimum add a comment above line 211 explaining why this re-derives `SHELL_NAME` rather than reusing the one set in the path block.

Example comment (minimal fix):
```bash
# Re-derive SHELL_NAME here: SHELL_NAME may be unset if --no-modify-path was passed.
SHELL_NAME=$(basename "$SHELL")
```

Or, preferred refactor — detect shell once and share between both blocks:
```bash
# Detect shell name once; used by both path configuration and hook registration
DETECTED_SHELL_NAME=""
if [ -n "${SHELL:-}" ]; then
    DETECTED_SHELL_NAME=$(basename "$SHELL")
fi
```

---

### [ADVISORY] Output for the hook step is asymmetric with the path step

**File**: `website/install.sh`, lines 217 and 226

When the hook is registered, the `echo "Registered command-not-found hook for ${DETECTED_SHELL}."` line (226) comes *after* the `tsuku hook install` call. The path configuration section uses a different pattern: `echo "Configuring bash..."` *before* the work, with `add_to_config` printing per-file results.

This isn't a bug, but a developer adding a new shell to the hook case will look at the path block for an example and use a different ordering than what the hook block uses. The inconsistency is minor but will produce a small "did I follow the right pattern?" pause.

**Fix**: Either add a pre-call echo like `echo "Registering command-not-found hook for ${DETECTED_SHELL}..."` before line 225, or accept the current pattern and add a brief comment noting the hook command itself emits progress. Not blocking.

---

### [ADVISORY] `--no-hooks` is listed in the usage comment but not in the README or any usage helper

**File**: `website/install.sh`, lines 7–10

The file-level usage comment on lines 7–10 now documents `--no-hooks` alongside `--no-modify-path`. This matches the acceptance criterion. However, there is no `--help` flag or `usage()` function — the only documentation is the comment. This was already true for `--no-modify-path`, so this is pre-existing. Noting it here because the addition of a second undiscoverable flag makes the pattern slightly more painful, but this is out of scope for this issue's diff.

---

## Summary

The implementation is clear, correctly gated, and handles the unset-`$SHELL` edge case as required. The only blocking concern is `SHELL_NAME` being assigned twice without a comment — a future maintainer extracting or reordering the hook block will encounter a `set -u` failure that is not obvious from reading either block in isolation. This is a one-line comment fix or a small refactor to hoist shell detection. The output asymmetry between the path and hook blocks is advisory and has no correctness impact.
