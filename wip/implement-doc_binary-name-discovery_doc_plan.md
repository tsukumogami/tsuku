# Documentation Plan: binary-name-discovery

Generated from: docs/designs/DESIGN-binary-name-discovery.md
Issues analyzed: 6
Total entries: 3

---

## doc-1: docs/designs/DESIGN-binary-name-discovery.md
**Section**: Status, Implementation Issues
**Prerequisite issues**: #1936, #1937, #1938, #1939, #1940, #1941
**Update type**: modify
**Status**: updated
**Details**: Update the design doc status from "Planned" to "Implemented" (or partial status if not all issues complete). Update the dependency graph node classes from "ready"/"blocked" to "done" for completed issues. This should happen after all issues are done, since partial updates to the mermaid graph mid-implementation create churn.

---

## doc-2: docs/designs/current/DESIGN-recipe-builders.md
**Section**: Context and Problem Statement, The Opportunity
**Prerequisite issues**: #1938
**Update type**: modify
**Status**: updated
**Details**: Add a cross-reference to DESIGN-binary-name-discovery.md in the recipe-builders design doc. The current text mentions that builders query registry APIs for metadata including executable names, but doesn't note the binary name validation layer or the BinaryNameProvider interface that sits between builders and the orchestrator. Add a brief note after the ecosystem list that binary name discovery has been improved with registry-authoritative sources and a validation step (linking to the binary name discovery design doc for details).

---

## doc-3: docs/deterministic-builds/ecosystem_cargo.md
**Section**: Recommended Primitive Interface, Implementation Recommendations
**Prerequisite issues**: #1936
**Update type**: modify
**Status**: updated
**Details**: The ecosystem_cargo.md doc references `detectExecutables(crate, metadata)` in the eval-time workflow pseudocode (line 594). After #1936, Cargo binary discovery reads `bin_names` from the crates.io version API response instead of fetching Cargo.toml from GitHub. Update the eval-time pseudocode comment and the surrounding text to reflect that executable names come from the crates.io API `bin_names` field, not from repository manifest parsing.
