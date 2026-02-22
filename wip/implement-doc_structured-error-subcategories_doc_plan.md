# Documentation Plan: structured-error-subcategories

Generated from: docs/designs/DESIGN-structured-error-subcategories.md
Issues analyzed: 4
Total entries: 0

---

No documentation updates are needed for this design. Here's why each issue was skipped:

- **#1856** (CLI subcategory output): Adds a `subcategory` field to `tsuku install --json` error output. This is the only user-facing change, but no existing documentation covers the `--json` error output format. The README doesn't mention `--json` for install, and there's no command reference doc for `tsuku install`. Creating a new command reference is outside the scope of this design.
- **#1857** (orchestrator normalization): Internal batch orchestrator category renames and subcategory passthrough. Not user-facing.
- **#1858** (CI workflow alignment): Updates inline jq in a GitHub Actions workflow. CI/build infrastructure.
- **#1859** (dashboard update): Internal dashboard deserialization and category remap logic. Not user-facing.
