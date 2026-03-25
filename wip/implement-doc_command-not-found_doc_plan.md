# Documentation Plan: command-not-found

Generated from: docs/plans/PLAN-command-not-found.md
Issues analyzed: 5
Total entries: 5

---

## doc-1: README.md
**Section**: Usage
**Prerequisite issues**: #1
**Update type**: modify
**Status**: updated
**Details**: Add a `tsuku suggest` usage example under a new "Command suggestions" subsection. Show both the single-match output (`Command 'jq' not found. Install with: tsuku install jq`) and the `--json` flag. Note that the command is also invoked automatically by the shell hook.

---

## doc-2: README.md
**Section**: Installation
**Prerequisite issues**: #4
**Update type**: modify
**Status**: pending
**Details**: Document the `--no-hooks` flag for the install script alongside the existing `--no-modify-path` flag. Explain that the installer registers the command-not-found hook for the detected shell by default, and that `--no-hooks` skips this step along with the manual registration command users can run later (`tsuku hook install`).

---

## doc-3: README.md
**Section**: Usage
**Prerequisite issues**: #2, #3
**Update type**: modify
**Status**: updated
**Details**: Add a "Command-not-found hook" subsection documenting `tsuku hook install`, `tsuku hook uninstall`, and `tsuku hook status`. Cover the `--shell=<shell>` flag, the three supported shells (bash, zsh, fish), and which rc files are modified. Mention that hook files live in `$TSUKU_HOME/share/hooks/` and are updated automatically when tsuku upgrades.

---

## doc-4: docs/GUIDE-command-not-found.md
**Section**: (new file)
**Prerequisite issues**: #1, #2, #3, #4
**Update type**: new
**Status**: pending
**Details**: Write an end-to-end guide for the command-not-found integration. Cover: how suggestions appear at the terminal, how the hook is installed automatically vs. manually, how it chains with existing handlers (detect-and-wrap), how to uninstall, and how to verify status with `tsuku hook status`. Include the `--no-hooks` install option for users who prefer manual setup.

---

## doc-5: docs/designs/DESIGN-command-not-found.md
**Section**: Status
**Prerequisite issues**: #1, #2, #3, #4
**Update type**: modify
**Status**: pending
**Details**: Update the frontmatter `status` field from `Planned` to `Implemented` once all implementation issues are merged.

---
