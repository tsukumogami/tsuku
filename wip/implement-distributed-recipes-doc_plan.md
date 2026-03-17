# Documentation Plan: Distributed Recipes

Generated from: docs/plans/PLAN-distributed-recipes.md
Issues analyzed: 13
Total entries: 7

---

## doc-1: README.md
**Section**: "Usage" (add "Install from distributed sources" subsection)
**Prerequisite issues**: #7
**Update type**: modify
**Status**: pending
**Details**: Add a new subsection under Usage showing the `tsuku install owner/repo` syntax, including examples with `:recipe` and `@version` qualifiers. Mention the first-install confirmation prompt and the `-y` flag for scripted use. Keep it brief -- this is the entry point, not the full reference.

---

## doc-2: README.md
**Section**: "Usage" (add "Manage recipe registries" subsection)
**Prerequisite issues**: #4
**Update type**: modify
**Status**: pending
**Details**: Add a short subsection showing `tsuku registry list`, `tsuku registry add <owner/repo>`, and `tsuku registry remove <owner/repo>`. Mention `strict_registries` mode for CI/team environments.

---

## doc-3: README.md
**Section**: Commands table in CLAUDE.local.md
**Prerequisite issues**: #4
**Update type**: modify
**Status**: pending
**Details**: Add `tsuku registry list`, `tsuku registry add`, and `tsuku registry remove` to the commands table in CLAUDE.local.md so the project context stays accurate.

---

## doc-4: docs/GUIDE-distributed-recipes.md
**Section**: (new file)
**Prerequisite issues**: #7, #8, #9, #10
**Update type**: new
**Status**: pending
**Details**: New user guide covering the full distributed recipes workflow. Sections: what distributed recipes are, how to install from a distributed source (`owner/repo` syntax with all format variants), the trust/confirmation model, managing registries (`tsuku registry` subcommands), how `update`/`outdated`/`verify` work with distributed tools, how source is shown in `list`/`info`/`recipes` output, `update-registry` refreshing distributed caches, and the `strict_registries` config option for locked-down environments. Reference `$TSUKU_HOME/config.toml` for registry storage.

---

## doc-5: docs/GUIDE-distributed-recipe-authoring.md
**Section**: (new file)
**Prerequisite issues**: #11
**Update type**: new
**Status**: pending
**Details**: Guide for recipe authors who want to host recipes in their own GitHub repos. Cover: creating a `.tsuku-recipes/` directory at the repo root, recipe TOML format (same schema as central registry), naming conventions (kebab-case filenames matching recipe name), no manifest or registration required, how users install from the repo, and how caching/refreshing works from the author's perspective.

---

## doc-6: docs/GUIDE-recipe-verification.md
**Section**: Existing verification guide
**Prerequisite issues**: #8
**Update type**: modify
**Status**: pending
**Details**: Add a note that `tsuku verify` now checks the recorded source for each tool. For distributed tools, verification uses the cached recipe from the distributed source rather than the central registry. No change to verification syntax or behavior within recipes -- this is a source-resolution change only.

---

## doc-7: docs/designs/DESIGN-distributed-recipes.md
**Section**: "Status" field
**Prerequisite issues**: #7, #8, #9, #10
**Update type**: modify
**Status**: pending
**Details**: Update the design doc status from "Planned" to "Implemented" (or "In Progress" if partial) once the command integration issues land. This is a bookkeeping update to keep the design index accurate.
