<!-- decision:start id="tool-versions-compat" status="assumed" -->
### Decision: .tool-versions Compatibility

**Context**

The .tool-versions format originated with asdf and is also supported by mise. It's a plain text file with `tool_name version` on each line. Many projects already have one checked in. The question is whether tsuku should read this format at runtime -- as a fallback when no .tsuku.toml exists -- or stick to its native TOML format only.

The impedance mismatch between the two ecosystems is significant. asdf uses plugin names ("nodejs", "python") that don't correspond to tsuku recipe names. Tsuku has ~1400 recipes with its own kebab-case naming. There's no "node" or "python" recipe -- only specialized tools like "node-red" and "python-tabulate". A translation layer would need a maintained mapping table covering hundreds of asdf plugin names, and any gap produces confusing errors. Version syntax differences compound the problem: asdf's "ref:", "path:", and "system" specifiers have no tsuku equivalent.

The parent design (DESIGN-shell-integration-building-blocks.md) flagged this as an open uncertainty. ProjectConfig does not yet exist in code, so this decision shapes the initial interface rather than constraining a refactor.

**Assumptions**

- Tsuku's recipe naming will not converge with asdf plugin naming. If wrong, a mapping table becomes trivial and the cost argument weakens.
- Migration from asdf/mise to tsuku will be a deliberate team decision, not an incremental drift. Users who choose tsuku accept learning its conventions.
- Demand for automated migration tooling is currently speculative. Real demand can be measured post-launch through support requests and telemetry.

**Chosen: No .tool-versions support (native TOML only)**

Tsuku reads only .tsuku.toml for project configuration. The ProjectConfig loader has a single parser and a single format. No name-mapping table, no version-syntax translation, no fallback file discovery.

If demand emerges for migration assistance, a `tsuku migrate .tool-versions` command can be added later as a separate feature. This command would perform best-effort name matching and generate a .tsuku.toml that the user reviews and edits. Because it's a one-time import (not a runtime dependency), it doesn't affect the ProjectConfig interface or the 50ms shell integration budget.

**Rationale**

The decision drivers strongly favor simplicity and interface stability over migration convenience.

*Simplicity wins.* A runtime compat layer adds a second parser, a translation layer, and a maintenance commitment to a mapping table. This complexity is permanent -- once projects depend on .tool-versions being read, removing support is a breaking change. The benefit is narrowly scoped: it only helps teams that already have a .tool-versions file AND want to use tsuku AND don't want to spend the few minutes creating a .tsuku.toml.

*The naming gap is structural, not incidental.* Tsuku is a curated registry with its own naming scheme. asdf is a plugin system where anyone can create a plugin with any name. Bridging these two models requires either maintaining a large mapping table (high burden) or accepting partial coverage (confusing UX). Neither outcome aligns with tsuku's conventions.

*Reversibility favors the simpler choice.* Starting with TOML-only and adding a migration command later is straightforward. Starting with runtime .tool-versions support and removing it later is a breaking change. The asymmetry in reversibility makes TOML-only the safer starting point.

*Competitive precedent supports either path.* mise supports .tool-versions because it's explicitly an asdf successor -- backwards compatibility is central to its value proposition. devbox and volta don't support it because they're different paradigms. Tsuku is closer to the latter camp: a curated, self-contained tool manager, not an asdf replacement.

**Alternatives Considered**

- **Full runtime .tool-versions support**: Read .tool-versions as a fallback with a name-mapping table. Rejected because the mapping table is a permanent maintenance burden, partial coverage creates confusing errors, and the two-format code path complicates ProjectConfig. The benefit (drop-in migration) serves a narrow use case that doesn't justify the ongoing cost.

- **One-time `tsuku migrate` command**: Best-effort import from .tool-versions to .tsuku.toml. Not rejected outright -- this is a reasonable future addition. Deferred because demand is speculative and the command can be added without changing the ProjectConfig design. If added, it should use fuzzy matching and require user review of the output.

- **Documentation-only migration guide**: Provide a mapping table in docs without any tooling. Viable as an interim step. Less useful for projects with many tools, but adequate for early adopters. This is the implicit minimum that ships with the TOML-only choice.

**Consequences**

Teams migrating from asdf/mise must manually create .tsuku.toml files. For projects with many tools, this takes 5-10 minutes and requires looking up tsuku recipe names. Documentation should include a "migrating from asdf" section with common name mappings.

ProjectConfig stays clean: one format, one parser, one code path. Downstream consumers (#1681 shell activation, #2168 exec wrapper) get a stable, simple interface.

If a `tsuku migrate` command is built later, the mapping data can also power a documentation page that helps users find the right recipe names. This deferred work has no architectural prerequisites.
<!-- decision:end -->
