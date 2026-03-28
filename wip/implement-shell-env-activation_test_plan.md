# Test Plan: Shell Environment Activation

Generated from: docs/plans/PLAN-shell-env-activation.md
Issues covered: 5
Total scenarios: 18

---

## Scenario 1: hook-env outputs export statements for a project directory
**ID**: scenario-1
**Testable after**: #1
**Commands**:
- `mkdir -p "$QA_HOME/project" && printf '[tools]\ngo = "1.22.5"\n' > "$QA_HOME/project/.tsuku.toml"`
- `mkdir -p "$TSUKU_HOME/tools/go-1.22.5/bin"`
- `cd "$QA_HOME/project" && tsuku hook-env bash`
**Expected**: Exit code 0. Stdout contains `export PATH=` with a path segment including `tools/go-1.22.5/bin`. Stdout contains `export _TSUKU_DIR=` and `export _TSUKU_PREV_PATH=`.
**Status**: pending

---

## Scenario 2: hook-env early-exit when directory unchanged
**ID**: scenario-2
**Testable after**: #1
**Commands**:
- `mkdir -p "$QA_HOME/project" && printf '[tools]\ngo = "1.22.5"\n' > "$QA_HOME/project/.tsuku.toml"`
- `cd "$QA_HOME/project" && _TSUKU_DIR="$QA_HOME/project" tsuku hook-env bash`
**Expected**: Exit code 0. Stdout is empty (no output at all, since the directory has not changed).
**Status**: pending

---

## Scenario 3: hook-env with no .tsuku.toml produces no output
**ID**: scenario-3
**Testable after**: #1
**Commands**:
- `cd /tmp && tsuku hook-env bash`
**Expected**: Exit code 0. Stdout is empty (no project config found, no prior activation).
**Status**: pending

---

## Scenario 4: hook-env skips tools whose version is not installed
**ID**: scenario-4
**Testable after**: #1
**Commands**:
- `mkdir -p "$QA_HOME/project" && printf '[tools]\ngo = "1.22.5"\nnode = "20.16.0"\n' > "$QA_HOME/project/.tsuku.toml"`
- `mkdir -p "$TSUKU_HOME/tools/go-1.22.5/bin"`
- (do NOT create node-20.16.0 directory)
- `cd "$QA_HOME/project" && tsuku hook-env bash`
**Expected**: Exit code 0. Stdout contains `tools/go-1.22.5/bin` in the PATH export. Stdout does NOT contain `node-20.16.0`. Stderr contains a warning about skipped/uninstalled tool `node`.
**Status**: pending

---

## Scenario 5: hook-env is hidden from top-level help
**ID**: scenario-5
**Testable after**: #1
**Commands**:
- `tsuku --help`
**Expected**: Output does NOT contain `hook-env`. The command is hidden from the user-facing command list.
**Status**: pending

---

## Scenario 6: tsuku shell outputs activation exports
**ID**: scenario-6
**Testable after**: #1, #2
**Commands**:
- `mkdir -p "$QA_HOME/project" && printf '[tools]\ngo = "1.22.5"\n' > "$QA_HOME/project/.tsuku.toml"`
- `mkdir -p "$TSUKU_HOME/tools/go-1.22.5/bin"`
- `cd "$QA_HOME/project" && tsuku shell`
**Expected**: Exit code 0. Stdout contains `export PATH=` with `tools/go-1.22.5/bin`. Stdout contains `export _TSUKU_DIR=` and `export _TSUKU_PREV_PATH=`.
**Status**: pending

---

## Scenario 7: tsuku shell errors when no .tsuku.toml found
**ID**: scenario-7
**Testable after**: #1, #2
**Commands**:
- `cd /tmp && tsuku shell`
**Expected**: Exit code is non-zero. Stderr contains an error message about no `.tsuku.toml` found.
**Status**: pending

---

## Scenario 8: tsuku shell respects --shell flag
**ID**: scenario-8
**Testable after**: #1, #2
**Commands**:
- `mkdir -p "$QA_HOME/project" && printf '[tools]\ngo = "1.22.5"\n' > "$QA_HOME/project/.tsuku.toml"`
- `mkdir -p "$TSUKU_HOME/tools/go-1.22.5/bin"`
- `cd "$QA_HOME/project" && tsuku shell --shell=fish`
**Expected**: Exit code 0. Stdout contains fish-style variable setting (e.g., `set -gx PATH` or `set -gx _TSUKU_DIR`) rather than bash-style `export`.
**Status**: pending

---

## Scenario 9: tsuku shell auto-detects shell from SHELL env var
**ID**: scenario-9
**Testable after**: #1, #2
**Commands**:
- `mkdir -p "$QA_HOME/project" && printf '[tools]\ngo = "1.22.5"\n' > "$QA_HOME/project/.tsuku.toml"`
- `mkdir -p "$TSUKU_HOME/tools/go-1.22.5/bin"`
- `cd "$QA_HOME/project" && SHELL=/bin/bash tsuku shell`
**Expected**: Exit code 0. Stdout contains bash-style `export PATH=` statements.
**Status**: pending

---

## Scenario 10: tsuku shell is visible in top-level help
**ID**: scenario-10
**Testable after**: #1, #2
**Commands**:
- `tsuku --help`
**Expected**: Output contains `shell` as an available command.
**Status**: pending

---

## Scenario 11: Deactivation restores original PATH
**ID**: scenario-11
**Testable after**: #1, #3
**Commands**:
- `mkdir -p "$QA_HOME/project" && printf '[tools]\ngo = "1.22.5"\n' > "$QA_HOME/project/.tsuku.toml"`
- `mkdir -p "$TSUKU_HOME/tools/go-1.22.5/bin"`
- `cd /tmp && _TSUKU_DIR="$QA_HOME/project" _TSUKU_PREV_PATH="/usr/bin:/bin" tsuku hook-env bash`
**Expected**: Exit code 0. Stdout contains `export PATH="/usr/bin:/bin"` (original PATH restored). Stdout contains `unset _TSUKU_DIR` and `unset _TSUKU_PREV_PATH`.
**Status**: pending

---

## Scenario 12: Fish deactivation uses set -e
**ID**: scenario-12
**Testable after**: #1, #3
**Commands**:
- `cd /tmp && _TSUKU_DIR="$QA_HOME/project" _TSUKU_PREV_PATH="/usr/bin:/bin" tsuku hook-env fish`
**Expected**: Exit code 0. Stdout contains `set -e _TSUKU_DIR` and `set -e _TSUKU_PREV_PATH` (fish-style unset). Stdout does NOT contain `unset`.
**Status**: pending

---

## Scenario 13: Project-to-project switching uses _TSUKU_PREV_PATH as base
**ID**: scenario-13
**Testable after**: #1, #3
**Commands**:
- `mkdir -p "$QA_HOME/projA" && printf '[tools]\ngo = "1.22.5"\n' > "$QA_HOME/projA/.tsuku.toml"`
- `mkdir -p "$QA_HOME/projB" && printf '[tools]\nnode = "20.16.0"\n' > "$QA_HOME/projB/.tsuku.toml"`
- `mkdir -p "$TSUKU_HOME/tools/go-1.22.5/bin" "$TSUKU_HOME/tools/nodejs-20.16.0/bin"`
- `cd "$QA_HOME/projB" && _TSUKU_DIR="$QA_HOME/projA" _TSUKU_PREV_PATH="/usr/bin:/bin" tsuku hook-env bash`
**Expected**: Exit code 0. The exported PATH contains `tools/nodejs-20.16.0/bin` prepended to `/usr/bin:/bin`. It does NOT contain `tools/go-1.22.5/bin` (project A's tool is gone). `_TSUKU_PREV_PATH` is still `/usr/bin:/bin` (preserved, not overwritten with the project A PATH).
**Status**: pending

---

## Scenario 14: No-op when leaving non-project directory with no prior activation
**ID**: scenario-14
**Testable after**: #1, #3
**Commands**:
- `cd /tmp && tsuku hook-env bash`
**Expected**: Exit code 0. Stdout is empty. No deactivation output since there was no prior activation (_TSUKU_PREV_PATH and _TSUKU_DIR are unset).
**Status**: pending

---

## Scenario 15: hook install --activate adds activation marker block
**ID**: scenario-15
**Testable after**: #1, #3, #4
**Commands**:
- `tsuku hook install --activate --shell=bash`
- `cat ~/.bashrc`
**Expected**: Exit code 0. The rc file contains a marker block that sources an activation hook file (distinct from the command-not-found marker block). The block references `tsuku-activate.bash` or equivalent.
**Status**: pending
**Environment**: manual (modifies user rc files; run in isolated QA_HOME with custom HOME)

---

## Scenario 16: hook install --activate is idempotent
**ID**: scenario-16
**Testable after**: #1, #3, #4
**Commands**:
- `tsuku hook install --activate --shell=bash`
- `tsuku hook install --activate --shell=bash`
- `grep -c "tsuku-activate" ~/.bashrc`
**Expected**: The activation marker block appears exactly once in the rc file (count is 1, not 2).
**Status**: pending
**Environment**: manual (modifies user rc files; run in isolated QA_HOME with custom HOME)

---

## Scenario 17: hook uninstall removes activation marker blocks
**ID**: scenario-17
**Testable after**: #1, #3, #4
**Commands**:
- `tsuku hook install --activate --shell=bash`
- `tsuku hook uninstall --shell=bash`
- `cat ~/.bashrc`
**Expected**: The rc file no longer contains the activation marker block.
**Status**: pending
**Environment**: manual (modifies user rc files; run in isolated QA_HOME with custom HOME)

---

## Scenario 18: End-to-end activation with eval $(tsuku shell)
**ID**: scenario-18
**Testable after**: #1, #2, #3
**Commands**:
- `mkdir -p "$QA_HOME/project" && printf '[tools]\ngo = "1.22.5"\n' > "$QA_HOME/project/.tsuku.toml"`
- `mkdir -p "$TSUKU_HOME/tools/go-1.22.5/bin" && printf '#!/bin/sh\necho go-1.22.5' > "$TSUKU_HOME/tools/go-1.22.5/bin/go" && chmod +x "$TSUKU_HOME/tools/go-1.22.5/bin/go"`
- `cd "$QA_HOME/project" && eval "$(tsuku shell)" && echo "$PATH" && go`
**Expected**: After eval, `$PATH` starts with the go-1.22.5/bin directory. Running `go` executes the project-specific binary and prints `go-1.22.5`. `$_TSUKU_DIR` is set to the project directory. `$_TSUKU_PREV_PATH` is set to the pre-activation PATH.
**Status**: pending
**Environment**: manual (requires eval in a real shell session)
