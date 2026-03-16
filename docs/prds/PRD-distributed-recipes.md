---
status: Draft
problem: |
  Every tsuku recipe lives in the central registry. Tool authors who want their
  software installable via tsuku must contribute a recipe to a third-party repo,
  creating friction that slows ecosystem growth. Internally, the CLI handles
  embedded, central, and local recipes through separate code paths with no shared
  abstraction, making it harder to add new source types.
goals: |
  Let tool authors ship tsuku recipes in their own repositories, installable with
  a single command. Unify all recipe sources behind a common model so that
  embedded, central, local, and distributed recipes are interchangeable.
source_issue: 2073
---

# PRD: Distributed Recipes

## Status

Draft

## Problem Statement

tsuku's recipe registry is centralized. Every recipe lives in the monorepo's
`recipes/` directory, and tool authors must submit a PR there to make their
software installable. This creates two problems:

**For tool authors:** They already manage their own releases and tags. Maintaining
a recipe in a third-party repo adds coordination overhead, and the recipe can
drift from the tool's release cadence.

**For tsuku's architecture:** The CLI handles recipe sources (embedded in the
binary, fetched from the central registry, cached locally) through separate code
paths. Adding a new source type means threading it through each path individually.
There's no `RecipeProvider` interface or shared abstraction.

These problems compound. Without distributed recipes, tsuku's growth is bottlenecked
by central registry contributions. Without a unified source model, adding distributed
recipes requires touching every command that resolves recipes.

## Goals

1. A tool author can make their software installable via tsuku by adding a single
   TOML file to their repository. No PR to the central registry required.

2. An end user can install from any source with a uniform experience. `tsuku install
   ripgrep` (central) and `tsuku install owner/repo` (distributed) feel the same.

3. All recipe sources (embedded, central, local, distributed) share a common
   abstraction. Adding a new source type doesn't require modifying every command.

4. Enterprise teams can restrict which sources are allowed through configuration.

## User Stories

**As a tool author**, I want to add a `.tsuku-recipes/` directory to my repo
with a recipe TOML file, so that users can install my tool with `tsuku install
myorg/mytool` without me submitting anything to the central registry.

**As an end user**, I want `tsuku install owner/repo` to just work without a
separate registration step, so that I can install tools from any source with
minimal friction.

**As an end user**, I want `tsuku list` and `tsuku info` to show where each
tool was installed from, so that I can understand my tool sources at a glance.

**As an end user**, I want `tsuku update` to check the correct source for each
installed tool automatically, so that I don't need to remember where things
came from.

**As a team lead**, I want to configure tsuku to only allow installation from
pre-approved registries, so that my team doesn't install tools from untrusted
sources.

## Requirements

### Functional Requirements

**R1. Install syntax.** `tsuku install owner/repo` installs a tool from a
distributed source. The slash distinguishes distributed from central registry
recipes. Extended forms: `owner/repo:recipe-name` (when repo has multiple
recipes) and `owner/repo@ref` (pin to a git tag or branch).

**R2. Auto-registration.** When a user installs from a distributed source for
the first time, tsuku automatically registers that source as a known registry.
Subsequent commands (update, outdated) use the registered source without the
user repeating the full path.

**R3. Registry convention.** A repository becomes a tsuku registry by containing
a `.tsuku-recipes/` directory with one or more TOML recipe files. No manifest
file is required for single-recipe repos. An optional manifest enables richer
metadata for multi-recipe registries.

**R4. Recipe format compatibility.** Distributed recipes use the same TOML
format as central registry recipes. No new fields, no format changes. A recipe
that works in the central registry works identically as a distributed recipe.

**R5. Central registry priority.** Unqualified names (`tsuku install ripgrep`)
always resolve from the central registry first, then embedded recipes. Distributed
sources are only consulted when the user provides a qualified name (`owner/repo`).
This prevents name confusion attacks.

**R6. Source tracking.** Each installed tool records its source (central, embedded,
local path, or `owner/repo`) in state.json. This enables correct routing for
update, outdated, and remove operations.

**R7. Source attribution.** All user-facing commands that display tool information
(`list`, `info`, `outdated`) show the source. Users can always tell where a tool
was installed from.

**R8. Registry management commands.** `tsuku registry list` shows all known
registries. `tsuku registry add <name> <source>` explicitly registers a source.
`tsuku registry remove <name>` unregisters a source. Removing a registry doesn't
uninstall tools that came from it.

**R9. Strict mode.** A system configuration option (`strict_registries`, off by
default) blocks auto-registration. When enabled, `tsuku install owner/repo`
fails with a message to run `tsuku registry add` first. This enables enterprise
lockdown.

**R10. Update across registries.** `tsuku update-registry` refreshes metadata
from all registered sources, not just the central registry. `tsuku update <tool>`
checks the tool's recorded source for new versions.

**R11. Version resolution.** Distributed recipes use the same version provider
system as central recipes. The `@ref` in the install command controls which
version of the recipe definition to fetch (git tag/branch). Tool version
resolution is handled by the recipe's `[version]` section as usual. For v1,
`tsuku update` always fetches the latest recipe (HEAD or latest tag) and then
resolves the tool version. Recipe-level pinning and two-dimension version
tracking are deferred.

**R16. Recipe listing.** `tsuku recipes` shows available recipes from all
registered sources, grouped by source. Central registry recipes appear first.
Distributed sources appear after, labeled with their source name.

**R12. Graceful degradation.** When a distributed source is unreachable, commands
that check it (update, outdated) show a warning but don't fail entirely. Already-
installed tools continue to work regardless of source availability.

### Non-Functional Requirements

**R13. No new dependencies.** Fetching distributed recipes must not add new
binary dependencies. Use git operations or HTTP fetching with existing stdlib.

**R14. Backward compatibility.** Existing `tsuku install <tool>` behavior is
unchanged. Existing state.json files are migrated transparently (existing
installations default to central registry source).

**R15. Minimal author friction.** A tool author should go from "no tsuku
support" to "installable via tsuku" by creating one directory and one file.
No accounts, no registration, no manifest.

## Acceptance Criteria

- [ ] `tsuku install owner/repo` fetches `.tsuku-recipes/` from the repo and
  installs the tool
- [ ] `tsuku install owner/repo:recipe@v1.0` installs a specific recipe at a
  specific git ref
- [ ] `tsuku install ripgrep` continues to resolve from the central registry
  (no behavior change)
- [ ] `tsuku registry list` shows auto-registered sources after a distributed
  install
- [ ] `tsuku registry add myname https://github.com/owner/repo` registers a
  source explicitly
- [ ] `tsuku registry remove myname` unregisters without uninstalling tools
- [ ] `tsuku list` shows the source for each installed tool
- [ ] `tsuku update <tool>` checks the correct source based on where the tool
  was originally installed
- [ ] With `strict_registries = true`, `tsuku install owner/repo` fails and
  tells the user to register first
- [ ] Existing state.json files migrate transparently (no user action needed)
- [ ] koto's recipe works when moved from central registry to
  `tsukumogami/koto/.tsuku-recipes/koto.toml`
- [ ] A distributed source being unreachable produces a warning, not a failure,
  for commands like `tsuku outdated`
- [ ] `tsuku recipes` shows recipes from all registered sources, grouped by source

## Out of Scope

- **Discovery integration (#1301).** Whether `tsuku install tool-name` can
  discover tools in distributed registries is deferred. Only qualified names
  (`owner/repo`) reach distributed sources.
- **Recipe contribution workflow (#1299).** The `tsuku contribute` and
  `tsuku export/import` commands are a separate feature.
- **Cryptographic signing.** No Sigstore, cosign, or signature verification in
  v1. Content-hash pinning is sufficient for now.
- **Private repository authentication.** Fetching from private GitHub repos
  (token management, credential storage) is deferred.
- **Enterprise SSO/SAML.** Organization-level auth for private registries.
- **Cross-registry search.** `tsuku search` continues to search the central
  registry only. Searching across all registered sources is a future enhancement.
- **Recipe version pinning.** Tracking recipe git ref independently from tool
  version, and detecting recipe-level updates, is deferred. v1 always uses the
  latest recipe.
- **Recipe generation from distributed sources.** The `--from` builder path
  for auto-generating recipes is unrelated.

## Open Questions

1. **Directory name:** Should the convention be `.tsuku-recipes/` or
   `.tsuku/recipes/`? The former is more discoverable at the repo root. The
   latter groups under a tsuku-specific directory. Leaning toward `.tsuku-recipes/`
   for visibility.

2. **Registry state persistence:** Should the list of known registries live in
   state.json alongside tool state, or in a separate config file
   (`$TSUKU_HOME/config.toml`)? A separate file is more inspectable and
   version-controllable. But state.json already exists and avoids a new file.

3. **Manifest schema for multi-recipe repos:** What does the optional manifest
   look like? JSON or TOML? What fields beyond a recipe list? This is a design
   question that the downstream design doc should address.

## Known Limitations

- **No offline distributed install.** Distributed recipes require network access
  to fetch from the remote repo. Central registry recipes can work from cache.
  Once a distributed recipe is cached, subsequent operations work offline.

- **Git hosting assumption.** The `owner/repo` syntax assumes GitHub (or a
  configured default host). Non-GitHub sources need a full URL or host override
  in config. This is acceptable for v1 given GitHub's dominance in the target
  user base.

- **No recipe pinning beyond git refs.** Content-hash pinning (recording the
  SHA of the recipe TOML at install time and detecting changes on update) is
  mentioned in the exploration research but deferred to the design doc.
