# Documentation Plan: project-aware-exec

Generated from: docs/plans/PLAN-project-aware-exec.md
Issues analyzed: 4
Total entries: 5

---

## doc-1: cmd/tsuku/cmd_run.go
**Section**: Long help text
**Prerequisite issues**: #1
**Update type**: modify
**Status**: pending
**Details**: Add project-as-consent explanation to the `tsuku run` long help. Document that when `.tsuku.toml` declares a tool, the consent mode is overridden to auto (project config is the consent). Mention that the resolver maps commands to recipes via the binary index and looks up pinned versions in `.tsuku.toml`. Keep the existing consent mode table and add a note about project override above or below it.

---

## doc-2: docs/GUIDE-command-not-found.md
**Section**: How It Works
**Prerequisite issues**: #1, #2
**Update type**: modify
**Status**: pending
**Details**: Update the "How It Works" section to describe the two-path behavior: when `.tsuku.toml` declares the missing command's recipe, the hook calls `tsuku run <command> [args]` (auto-install with pinned version); when the command is not declared or no `.tsuku.toml` exists, the hook calls `tsuku suggest` as before. Add a short section explaining the project-aware upgrade and link to the new project-aware exec guide.

---

## doc-3: cmd/tsuku/cmd_shim.go
**Section**: Long help text
**Prerequisite issues**: #3
**Update type**: new
**Status**: pending
**Details**: Write complete help text for `tsuku shim` and its subcommands (`install`, `uninstall`, `list`). Include usage examples, explain that shims are static shell scripts calling `tsuku run`, describe PATH precedence ($TSUKU_HOME/bin shims vs tools/current symlinks vs system PATH), and note that shims defer version resolution to runtime so they don't need regeneration when `.tsuku.toml` changes.

---

## doc-4: docs/GUIDE-project-aware-exec.md
**Section**: (new file)
**Prerequisite issues**: #1, #2, #3
**Update type**: new
**Status**: pending
**Details**: New end-to-end guide covering project-aware execution. Sections: overview of the consent model (`.tsuku.toml` as authorization), how `tsuku run` resolves project-pinned versions, the upgraded command-not-found hook behavior (run vs suggest), shim setup for CI and scripts, a CI workflow example using `tsuku shim install`, PATH precedence explanation, and escape hatches for untrusted repos (global mode override, no hooks, TSUKU_CEILING_PATHS). Link back to the command-not-found guide and README.

---

## doc-5: README.md
**Section**: Shell integration / command-not-found paragraph
**Prerequisite issues**: #1, #2, #3
**Update type**: modify
**Status**: pending
**Details**: Update the command-not-found hook description (line 27 area and line 796 area) to mention that in projects with `.tsuku.toml`, the hook auto-installs the pinned version instead of just suggesting. Add a brief mention of `tsuku shim` for CI/script usage. Link to the new project-aware exec guide for details.
