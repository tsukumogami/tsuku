# Documentation Plan: ecosystem-name-resolution

Generated from: docs/designs/DESIGN-ecosystem-name-resolution.md
Issues analyzed: 4
Total entries: 2

---

## doc-1: CONTRIBUTING.md
**Section**: Recipe Format
**Prerequisite issues**: #1826
**Update type**: modify
**Status**: updated
**Details**: Add the optional `[metadata.satisfies]` field to the recipe format documentation. Show how recipe authors declare which ecosystem package names their recipe fulfills (e.g., `homebrew = ["openssl@3"]`). Include a brief explanation of when to use this field: when a recipe covers a tool that other ecosystems name differently.

---

## doc-2: README.md
**Section**: Create recipes from package ecosystems
**Prerequisite issues**: #1826, #1827
**Update type**: modify
**Status**: pending
**Details**: Document the new duplicate detection behavior in `tsuku create`. When an existing recipe already satisfies the requested name (via the satisfies index or direct match), the command prints a message and exits. Mention that `--force` overrides this check. Add a short example showing the expected output when a user runs `tsuku create openssl@3 --from homebrew` and the existing `openssl` recipe already covers that name.
