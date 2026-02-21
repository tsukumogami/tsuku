# Exploration Summary: Ecosystem Name Resolution

## Problem (Phase 1)
Tsuku has no mechanism to resolve ecosystem-specific package names (e.g., Homebrew's `openssl@3`) to tsuku recipe names (e.g., `openssl`). This causes duplicate recipes, false pipeline blockers, and confusing `tsuku create` behavior.

## Decision Drivers (Phase 1)
- Must work for `tsuku create`, not just the batch pipeline
- Must scale without manual maintenance
- Must be integral to the CLI's recipe resolution system
- Should follow existing patterns (TOML metadata fields)
- Must be auditable (committed, reviewed)

## Research Findings (Phase 2)
- Zero name normalization exists in the entire recipe resolution chain
- `dep-mapping.json` maps names but no code consumes it
- The `@` symbol has conflicting semantics (version constraint vs formula name)
- Only 4 blockers on the dashboard; only `openssl@3` is a name mismatch
- `tsuku create` doesn't check embedded recipes before generating

## Options (Phase 3)
- `satisfies` metadata field (chosen): recipes declare ecosystem names they satisfy
- Static mapping file: separate JSON file (rejected: doesn't scale, proven failure)
- Convention-based stripping: strip @N suffix (rejected: only works for Homebrew convention)
- Reverse index from step formulas (rejected: implicit contract, Homebrew-only)

## Decision (Phase 5)

**Problem:**
Tsuku's recipe resolution uses exact name matching with no fallback. When Homebrew calls a package `openssl@3` but the tsuku recipe is named `openssl`, the system fails to connect them. This produces duplicate recipes from the batch pipeline, false blockers on the dashboard, and `tsuku create` generating inferior copies of existing recipes. A static mapping file was tried (#1200) but never wired in, and the approach doesn't scale.

**Decision:**
Add a `satisfies` map field to recipe metadata where recipes declare which ecosystem package names they fulfill. Integrate this as a fallback step in the recipe loader, after the existing 4-tier lookup chain. This gives every caller -- install, create, dependency resolution, batch pipeline -- automatic ecosystem name resolution. Delete the duplicate `openssl@3` registry recipe and migrate `dep-mapping.json` entries to `satisfies` fields on the corresponding recipes.

**Rationale:**
Co-locating the mapping with the recipe means no separate file to maintain and no indirection to forget. The loader fallback is the natural integration point because all code paths use it. Convention-based stripping was simpler but only covers Homebrew's `@` pattern, while `satisfies` handles all naming mismatches uniformly (openssl@3, sqlite3, gcc, curl). The explicit declaration approach is auditable through normal PR review.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-21
