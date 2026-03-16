---
status: Accepted
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

Accepted

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
recipes. Extended forms:
- `owner/repo:recipe-name` selects a specific recipe when the repo has multiple.
- `owner/repo@version` pins the tool version, identical to `tsuku install
  tool@version` for central recipes. The `@version` always refers to the tool
  version, never the recipe's git ref.

**R2. Auto-registration.** When a user installs from a distributed source for
the first time, tsuku registers that source as a known registry under the name
`owner/repo`. Subsequent commands (update, outdated) use the registered source
without the user repeating the full path. Auto-registering an already-registered
source is a no-op.

**R3. Registry convention.** A repository becomes a tsuku registry by containing
a `.tsuku-recipes/` directory with one or more TOML recipe files. When no
`:recipe-name` is specified in the install command:
- If exactly one `.toml` file exists in `.tsuku-recipes/`, use it.
- If multiple `.toml` files exist, fail with an error listing available recipes.
An optional manifest (format defined by the downstream design doc) enables
richer metadata for multi-recipe registries.

**R4. Recipe format compatibility.** Distributed recipes use the same TOML
format as central registry recipes. No new fields, no format changes. A recipe
that works in the central registry works identically as a distributed recipe.

**R5. Central registry priority.** Unqualified names (`tsuku install ripgrep`)
always resolve from the central registry first, then embedded recipes.
Distributed sources are never consulted for unqualified names, even if a
registered distributed source contains a recipe with the same name. This
prevents name confusion attacks.

**R6. Source tracking.** Each installed tool records its source in state.json.
Source values: `"central"` for the central registry, `"embedded"` for built-in
recipes, a file path for local recipes, or `"owner/repo"` for distributed
sources. Existing state.json entries without a source field default to
`"central"` on first access (lazy migration, no user action required).

**R7. Source attribution.** All user-facing commands that display tool
information (`list`, `info`, `outdated`, `verify`) include the source in their
output.

**R8. Registry management commands.**
- `tsuku registry list` shows all known registries (auto-registered and
  explicitly added).
- `tsuku registry add <name> <source>` registers a source explicitly. `<name>`
  is a user-chosen alias. `<source>` accepts `owner/repo` shorthand (GitHub) or
  a full HTTPS URL.
- `tsuku registry remove <name>` unregisters a source. Removing a registry
  doesn't uninstall tools that came from it. Removing a non-existent registry
  is a no-op.

**R9. Strict mode.** A configuration option (`strict_registries`, off by
default) blocks auto-registration. When enabled, `tsuku install owner/repo`
fails with an error message that includes the exact `tsuku registry add`
command to run. The config location is determined by the design doc.

**R10. Update across registries.** `tsuku update-registry` refreshes metadata
from all registered distributed sources (in addition to the central registry).
`tsuku update <tool>` checks the tool's recorded source for new versions.

**R11. Version resolution.** Distributed recipes use the same version provider
system as central recipes. The recipe is always fetched from HEAD of the repo's
default branch. `@version` in the install command pins the tool version, not the
recipe. `tsuku update` re-fetches the recipe from HEAD and resolves the latest
tool version via the recipe's `[version]` section.

**R12. Recipe listing.** `tsuku recipes` shows available recipes from all
registered sources, grouped by source. Central registry recipes appear first.
Distributed sources appear after, labeled by registry name.

**R13. Remove behavior.** `tsuku remove <tool>` works the same regardless of
source. The tool is uninstalled and its state.json entry removed. The registry
that provided the recipe remains registered (removing tools doesn't unregister
sources). The tool's installed name is the recipe name, not the `owner/repo`
qualifier.

**R14. Verify behavior.** `tsuku verify <tool>` uses the cached recipe to
verify the installation. If no cached recipe exists and the source is
unreachable, verification reports the source as unavailable rather than failing
silently.

**R15. Graceful degradation.** Behavior when a distributed source is unreachable:
- Batch commands (`tsuku outdated`, `tsuku update-registry`) emit a warning per
  unreachable source and complete for reachable sources.
- Targeted commands (`tsuku update <tool>`) fail with a clear error if that
  tool's source is unreachable.
- `tsuku install owner/repo` fails with a connection error.
- Already-installed tools work regardless of source availability.

**R16. Error handling.** Clear, actionable error messages for:
- `owner/repo` where the repo doesn't exist or is inaccessible.
- Repo exists but has no `.tsuku-recipes/` directory.
- `.tsuku-recipes/` contains no valid TOML files or malformed recipes.
- Multiple recipes in `.tsuku-recipes/` without a `:recipe-name` qualifier.

### Non-Functional Requirements

**R17. No new binary dependencies.** Fetching distributed recipes must use HTTP
(GitHub archive or raw content API) with Go's standard library. The `git`
binary must not be required on the user's system. The design doc determines
the specific fetching mechanism.

**R18. Backward compatibility.** Existing `tsuku install <tool>` behavior,
CLI output format, and exit codes are unchanged. Existing state.json files
are migrated lazily (source field defaults to `"central"` on first access).

**R19. Minimal author friction.** A tool author MUST be able to make their
tool installable via tsuku by creating one directory (`.tsuku-recipes/`) and
one TOML file. No accounts, no registration, no manifest, no additional
configuration.

## Acceptance Criteria

### Happy path

- [ ] `tsuku install owner/repo` fetches `.tsuku-recipes/` from the repo,
  installs the tool, creates a binary in `$TSUKU_HOME/bin/`, and records
  `source: "owner/repo"` in state.json
- [ ] `tsuku install owner/repo:recipe@v1.0` installs a specific recipe at
  tool version v1.0
- [ ] `tsuku install ripgrep` continues to resolve from the central registry
  with identical behavior to today
- [ ] `tsuku registry list` shows auto-registered sources after a distributed
  install
- [ ] `tsuku registry add myname owner/repo` registers a source explicitly
- [ ] `tsuku registry remove myname` unregisters without uninstalling tools;
  removing a non-existent registry produces no error
- [ ] `tsuku list` shows the source for each installed tool
- [ ] `tsuku info <distributed-tool>` shows source, cached recipe ref, and
  recipe metadata
- [ ] `tsuku update <tool>` checks the correct source based on the recorded
  source in state.json
- [ ] `tsuku remove <tool>` uninstalls a distributed tool identically to a
  central tool; the source registry remains registered
- [ ] `tsuku verify <tool>` verifies a distributed tool using the cached recipe
- [ ] `tsuku recipes` shows recipes from all registered sources, grouped by
  source name
- [ ] `tsuku update-registry` refreshes metadata from all registered sources

### Strict mode

- [ ] With `strict_registries` enabled, `tsuku install owner/repo` fails with
  an error message containing the exact `tsuku registry add` command to run
- [ ] With `strict_registries` enabled, `tsuku install ripgrep` (central)
  works unchanged

### Error handling

- [ ] `tsuku install owner/nonexistent` fails with a clear "repository not
  found" error
- [ ] `tsuku install owner/repo-without-recipes` fails with "no .tsuku-recipes/
  directory found"
- [ ] `tsuku install owner/multi-recipe-repo` (no `:recipe-name`) fails with
  an error listing available recipes
- [ ] When a registered distributed source is unreachable, `tsuku outdated`
  emits a warning for that source and completes for other sources
- [ ] When a tool's source is unreachable, `tsuku update <tool>` fails with
  a connection error naming the source

### Migration and compatibility

- [ ] Existing state.json files without source fields work without errors;
  tools default to `source: "central"` on first access
- [ ] A recipe moved from the central registry to a distributed source
  (`owner/repo/.tsuku-recipes/tool.toml`) installs and updates correctly
- [ ] `go.mod` contains no new external module dependencies after implementation
- [ ] A new distributed recipe requires exactly one directory and one TOML file;
  no other files, accounts, or configuration needed

## Out of Scope

- **Discovery integration (#1301).** Whether `tsuku install tool-name` can
  discover tools in distributed registries is deferred. Only qualified names
  (`owner/repo`) reach distributed sources.
- **Recipe contribution workflow (#1299).** The `tsuku contribute` and
  `tsuku export/import` commands are a separate feature.
- **Cryptographic signing.** No Sigstore, cosign, or signature verification
  in v1.
- **Content-hash pinning.** Recording the SHA of recipe content at install time
  and detecting changes on update is deferred to the design doc.
- **Private repository authentication.** Fetching from private GitHub repos
  (token management, credential storage) is deferred.
- **Enterprise SSO/SAML.** Organization-level auth for private registries.
- **Cross-registry search.** `tsuku search` continues to search the central
  registry only.
- **Recipe version pinning.** Pinning the recipe to a specific git ref
  (e.g., `owner/repo[:ref]@version`) is deferred. v1 always fetches the recipe
  from HEAD. A future version may allow ref pinning with a distinct syntax to
  avoid conflating recipe ref with tool version.
- **Recipe generation from distributed sources.** The `--from` builder path
  for auto-generating recipes is unrelated.
- **Non-GitHub host configuration.** Configuring a default host other than
  GitHub is deferred. Non-GitHub sources can use full HTTPS URLs via
  `tsuku registry add`.

## Open Questions

1. **Manifest schema for multi-recipe repos:** What does the optional manifest
   look like? JSON or TOML? What fields beyond a recipe list? This is a design
   question for the downstream design doc.

## Known Limitations

- **No offline distributed install.** Distributed recipes require network access
  to fetch from the remote repo. Once fetched and cached, subsequent operations
  work offline.

- **GitHub-first.** The `owner/repo` shorthand assumes GitHub. Non-GitHub
  sources require a full HTTPS URL via `tsuku registry add`. This is acceptable
  for v1 given GitHub's dominance in the target user base.

- **No recipe change detection.** Without content-hash pinning (deferred),
  tsuku can't detect if a recipe was modified at the same git ref. A malicious
  registry could silently change a recipe's content. This is mitigated by the
  explicit trust decision (auto-register or strict mode) and will be addressed
  by content-hash pinning in a future version.

- **`tsuku search` won't find distributed recipes.** Users must know the
  `owner/repo` to install from a distributed source. `tsuku recipes` (after
  registration) provides some discoverability, but there's no cross-registry
  search.
