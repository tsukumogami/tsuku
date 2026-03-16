# Phase 2 Research: User Researcher

## Lead 1: User Personas and Priorities

### Findings

Three personas with overlapping but sometimes conflicting needs:

**Tool author** -- wants zero-friction distribution. Their journey:
1. Has a tool with releases on GitHub (or similar)
2. Creates a `.tsuku-recipes/tool.toml` in their repo
3. Users can now `tsuku install owner/repo`
4. Success: no PR to central registry, no coordination with tsuku maintainers

Key needs: minimal setup (one file), no manifest overhead for single-recipe
repos, clear documentation on recipe format, ability to test the recipe locally.

**End user** -- wants uniform UX across all sources. Their journey:
1. Discovers a tool that's installable via tsuku
2. Runs `tsuku install owner/repo` (or `tsuku install tool` from central)
3. Tool works. Doesn't care about source.
4. Later: `tsuku update tool` checks the right source automatically

Key needs: no distinction in UX between central and distributed (same install,
update, remove flow), clear output showing where a tool came from, graceful
degradation when a registry is unavailable.

**Enterprise/team lead** -- needs granular control. Their journey:
1. Configures `strict_registries = true` in system config
2. Pre-registers approved registries: `tsuku registry add company-tools <url>`
3. Team members can only install from approved sources
4. Audits: `tsuku list` shows source for every installed tool

Key needs: allowlist/blocklist for registries, no auto-registration, policy
enforcement via config or env vars, audit trail of where each tool came from.

### Implications for Requirements

- The PRD must define behavior for all three personas, with end user as primary
- Tool author friction is the adoption gate -- if it's too hard to create a
  distributed recipe, nobody will
- Enterprise features (strict mode) can be v1 but don't need to be the default
- Source attribution must appear in all user-facing output (list, info, etc.)

### Open Questions

- Should there be a `tsuku init-recipe` command to scaffold a recipe in a repo?
- How does a tool author test their recipe before publishing?

## Lead 6: Affected Commands

### Findings

Nine of ten main CLI commands need modifications:

| Command | Current Behavior | Needed Change |
|---------|-----------------|---------------|
| `install` | Resolves from priority chain | Detect `owner/repo` syntax, auto-register, fetch from distributed source |
| `remove` | Removes by tool name | Needs to track source for cleanup; removing a tool shouldn't unregister its source |
| `list` | Shows name + version | Add source column (central, owner/repo, local, embedded) |
| `update` | Checks version provider | Route to correct source for version check |
| `info` | Shows recipe metadata | Display source information |
| `outdated` | Checks all tools | Version resolution from distributed sources |
| `verify` | Checks installed binaries | Verify against correct source's checksums |
| `update-registry` | Fetches tsuku.dev/recipes.json | Fetch manifests from all registered registries |
| `search` | Searches central manifest | Optionally span all registered registries |

New command family needed: `tsuku registry add/list/remove`

The `recipes` command (lists available recipes) also needs to show distributed
recipes from registered sources.

### Implications for Requirements

- Every command that displays tool information should show source attribution
- `update` and `outdated` are the most complex -- they need to check potentially
  many remote sources
- `search` across registries is a nice-to-have but adds latency; could be
  opt-in (`tsuku search --all-registries`)
- `update-registry` should update all registered registries, not just central
- Graceful degradation: if a distributed source is unreachable, show a warning
  but don't fail the entire command

### Open Questions

- Should `tsuku search` span all registries by default or only central?
- How does `tsuku outdated` handle unreachable distributed sources?
- Should `tsuku recipes` show distributed recipes?

## Summary

Distributed recipes require three personas (tool author, end user, enterprise lead) with the end user as primary. Nine CLI commands need modifications, with source attribution in all output, graceful degradation for unreachable sources, and a new `tsuku registry` command family. Critical requirements are: uniform UX regardless of source, transparent source information, and enterprise policy enforcement via strict mode config.
