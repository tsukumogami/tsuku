# Documentation Plan: project-configuration

Generated from: docs/plans/PLAN-project-configuration.md
Issues analyzed: 4
Total entries: 5

---

## doc-1: README.md
**Section**: Usage (after "Install a tool")
**Prerequisite issues**: #2, #3
**Update type**: modify
**Status**: pending
**Details**: Add a "Project configuration" subsection to the Usage section covering: `tsuku init` to create `.tsuku.toml`, `tsuku install` with no args to batch-install from project config, and a brief `.tsuku.toml` example showing both string shorthand and inline table forms. Keep it concise with a cross-reference to the full format docs.

---

## doc-2: docs/ENVIRONMENT.md
**Section**: Core Configuration (new subsection)
**Prerequisite issues**: #1
**Update type**: modify
**Status**: pending
**Details**: Add `TSUKU_CEILING_PATHS` environment variable documentation. Describe it as a colon-separated list of directories that stop `.tsuku.toml` parent traversal. Note that `$HOME` is always a ceiling and cannot be removed by this variable.

---

## doc-3: docs/GUIDE-project-configuration.md
**Section**: (new file)
**Prerequisite issues**: #1, #2, #3
**Update type**: new
**Status**: pending
**Details**: Create a user guide for project configuration covering: getting started with `tsuku init`, `.tsuku.toml` format reference (version string shorthand, inline table form, "latest"/empty version, prefix matching), `tsuku install` no-args batch mode, interactive confirmation and `--yes` flag, error handling and partial failure behavior, exit codes (0, 6, 15), flag compatibility (`--dry-run`/`--force`/`--fresh` supported; `--plan`/`--recipe`/`--from`/`--sandbox` incompatible), unpinned version warnings, and directory traversal behavior with ceiling paths.

---

## doc-4: cmd/tsuku/init.go
**Section**: Cobra command help text (Short, Long, Example)
**Prerequisite issues**: #2
**Update type**: modify
**Status**: pending
**Details**: Ensure `tsuku init` Cobra command has complete Short description, Long description explaining what `.tsuku.toml` is and how it's used, and Example field showing typical usage including `--force` flag.

---

## doc-5: cmd/tsuku/install.go
**Section**: Cobra command help text
**Prerequisite issues**: #3
**Update type**: modify
**Status**: pending
**Details**: Update `tsuku install` help text to document the no-args project config mode alongside existing single-tool usage. Mention `.tsuku.toml` discovery, the `--yes`/`-y` flag for non-interactive confirmation, and the `ExitPartialFailure` (15) exit code.
