# Documentation Plan: shell-env-activation

Generated from: docs/plans/PLAN-shell-env-activation.md
Issues analyzed: 5
Total entries: 4

---

## doc-1: docs/GUIDE-shell-env-activation.md
**Section**: (new file)
**Prerequisite issues**: #1, #2, #3, #4
**Update type**: new
**Status**: pending
**Details**: New guide covering per-directory tool version activation. Should explain: what activation does (prepends project tool bin paths to PATH based on `.tsuku.toml`), automatic activation via prompt hooks (`tsuku hook install --activate`), explicit activation via `eval $(tsuku shell)`, deactivation when leaving a project directory, project-to-project switching behavior, the `_TSUKU_DIR` and `_TSUKU_PREV_PATH` env vars, skipped tools when a version isn't installed, and performance characteristics. Follow the structure and tone of `docs/GUIDE-command-not-found.md`. Cover bash, zsh, and fish.

---

## doc-2: README.md
**Section**: Usage
**Prerequisite issues**: #2
**Update type**: modify
**Status**: pending
**Details**: Add a "Per-project tool versions" subsection to the Usage section showing `eval $(tsuku shell)` for one-shot activation. Mention that `tsuku shell` reads `.tsuku.toml` from the current directory and adjusts PATH. Keep it brief -- link to the full guide (doc-1) for details.

---

## doc-3: README.md
**Section**: Command-not-found hook
**Prerequisite issues**: #4
**Update type**: modify
**Status**: pending
**Details**: Update the existing hook install examples to mention the `--activate` flag for prompt-based activation hooks. Add a short paragraph after the command-not-found hook section explaining that `tsuku hook install --activate` also registers per-directory activation hooks. Link to the full activation guide (doc-1).

---

## doc-4: docs/GUIDE-command-not-found.md
**Section**: Managing Hooks
**Prerequisite issues**: #4
**Update type**: modify
**Status**: pending
**Details**: Add a note or cross-reference in the "Install" section mentioning the `--activate` flag and linking to the shell environment activation guide. The command-not-found guide shouldn't duplicate activation docs, but should mention that `hook install` supports a second hook type via `--activate`.
