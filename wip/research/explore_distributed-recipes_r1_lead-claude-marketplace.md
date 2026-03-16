# Lead: Claude Code marketplace model

## Findings

### Architecture: Registry-of-Registries

Claude Code's plugin system uses a two-level distribution model:

1. **Marketplaces** are catalogs (registries) that list available plugins. Each marketplace is a git repository containing `.claude-plugin/marketplace.json`.
2. **Plugins** are the installable units, sourced from various locations defined within each marketplace.

Users add marketplaces first (`/plugin marketplace add owner/repo`), then install individual plugins from them (`/plugin install plugin-name@marketplace-name`). This is explicitly a registry-of-registries pattern: the official Anthropic marketplace (`claude-plugins-official`) is pre-registered by default, but any number of third-party marketplaces can coexist.

Source: [Create and distribute a plugin marketplace](https://code.claude.com/docs/en/plugin-marketplaces)

### Marketplace Schema

The `marketplace.json` file has a clean, minimal schema:

**Required fields:**
- `name` (string, kebab-case) - marketplace identifier, used in install commands like `plugin-name@marketplace-name`
- `owner` (object) - `name` (required) and `email` (optional)
- `plugins` (array) - list of plugin entries

**Optional metadata:**
- `metadata.description` - brief marketplace description
- `metadata.version` - marketplace version
- `metadata.pluginRoot` - base directory for relative paths (e.g., `./plugins` lets you write `"source": "formatter"` instead of `"source": "./plugins/formatter"`)

**Plugin entry fields (required):**
- `name` - plugin identifier (kebab-case)
- `source` - string (relative path) or object (remote source)

**Plugin entry fields (optional):**
- `description`, `version`, `author`, `homepage`, `repository`, `license`, `keywords`, `category`, `tags`
- `strict` (boolean) - controls whether plugin.json or marketplace entry is authoritative for component definitions
- Component configuration: `commands`, `agents`, `hooks`, `mcpServers`, `lspServers`

The schema is referenced via `"$schema": "https://anthropic.com/claude-code/marketplace.schema.json"`.

Source: [Official marketplace.json](https://github.com/anthropics/claude-plugins-official/blob/main/.claude-plugin/marketplace.json)

### Source Types

Plugin sources define where each plugin's content lives. This is separate from the marketplace source (where the catalog itself lives).

| Source Type | Key Fields | Notes |
|-------------|-----------|-------|
| Relative path | String starting with `./` | Plugin within the same repo as marketplace |
| `github` | `repo`, `ref?`, `sha?` | GitHub shorthand (`owner/repo`) |
| `url` | `url` (git URL), `ref?`, `sha?` | Any git host (GitLab, Bitbucket, self-hosted) |
| `git-subdir` | `url`, `path`, `ref?`, `sha?` | Sparse clone of a subdirectory in a monorepo |
| `npm` | `package`, `version?`, `registry?` | npm package installation |
| `pip` | `package`, `version?`, `registry?` | pip package installation |

Key design choice: the `ref` and `sha` fields allow both floating references (track a branch) and pinned versions (exact commit). This supports release channels where a "stable" marketplace pins to a `stable` branch and a "latest" marketplace pins to `latest`.

Source: [Plugin sources documentation](https://code.claude.com/docs/en/plugin-marketplaces)

### Trust Model

The trust model is layered but lightweight:

1. **Official marketplace auto-registered**: `claude-plugins-official` is available by default. Users don't need to add it.
2. **Anthropic Verified badge**: Plugins in the official marketplace that have undergone additional review get a verification badge, but Anthropic explicitly states there are limits to what they can review.
3. **User responsibility**: Users are warned to "only install plugins from developers you trust." Plugins can execute arbitrary code with user privileges.
4. **Organizational control via `strictKnownMarketplaces`**: Administrators can restrict which marketplaces users can add using managed settings. Options include:
   - Complete lockdown (empty array `[]`)
   - Allowlist of specific marketplace sources
   - Pattern-based matching via `hostPattern` (regex on git host) or `pathPattern` (regex on filesystem path)
5. **Auto-discovery via `extraKnownMarketplaces`**: Teams can configure project-level `.claude/settings.json` to prompt users to install specific marketplaces when they trust the project folder.
6. **Reserved names**: Official Anthropic marketplace names are reserved to prevent impersonation (`claude-code-marketplace`, `claude-plugins-official`, `anthropic-marketplace`, etc.).
7. **SHA pinning**: Plugin sources support 40-character SHA pinning for immutable version references.

There is no cryptographic signing, no content hashing beyond git SHA, and no automated security scanning of third-party plugins.

Source: [Security concerns](https://www.promptarmor.com/resources/hijacking-claude-code-via-injected-marketplace-plugins), [Claude Code security docs](https://code.claude.com/docs/en/security)

### Caching and Update Mechanism

- Plugins are copied to a local cache at `~/.claude/plugins/cache` rather than used in-place.
- Version fields determine cache paths and update detection. If the version doesn't change, updates are skipped.
- Auto-updates can be configured per-marketplace. Official marketplaces auto-update by default; third-party ones don't.
- Private repo authentication uses environment variables (`GITHUB_TOKEN`, `GITLAB_TOKEN`, `BITBUCKET_TOKEN`) for background auto-updates.

### Real-World Structure

Two official marketplaces exist:

1. **`claude-plugins-official`** (github.com/anthropics/claude-plugins-official) - curated, Anthropic-maintained. Contains LSP plugins, external integrations (GitHub, Slack, Sentry, etc.), and development tools. Uses `strict: false` extensively, meaning the marketplace entry fully defines each plugin's components.

2. **`claude-code-plugins`** (github.com/anthropics/claude-code, under `/plugins`) - demo/bundled marketplace. Contains workflow plugins like `commit-commands`, `code-review`, `feature-dev`. Uses relative paths for all plugins within the same repo.

Both use `./plugins/<name>` relative paths for their plugins, with plugin components defined inline in the marketplace entry or delegated to the plugin's own `plugin.json`.

### The `strict` Field Pattern

This is a notable design decision for registry unification. `strict: true` (default) means the plugin's own manifest (`plugin.json`) is authoritative -- the marketplace entry supplements it. `strict: false` means the marketplace entry is the complete definition -- if the plugin also has a manifest with component definitions, it's a conflict and loading fails.

This allows the same plugin to be presented differently in different marketplaces: one marketplace might expose all components, while another curates a subset.

## Implications

### For tsuku registry unification

1. **The two-level model maps well to tsuku.** Marketplaces are analogous to registries (the central registry, local registries, third-party registries). Recipes are analogous to plugins. The `plugin-name@marketplace-name` addressing scheme is worth adopting -- `tool@registry` gives unambiguous resolution.

2. **Source type flexibility is key.** Claude Code supports relative paths (embedded), GitHub repos, generic git URLs, git subdirectories, npm, and pip as source types. Tsuku should support analogous source types for recipes: embedded (in the registry repo), GitHub release assets, generic URLs, and potentially git-based recipe repos.

3. **The `strict` pattern enables curation.** A third-party registry could list recipes from upstream repos but override metadata, add custom validation, or restrict version ranges. This is relevant for enterprise use cases where an organization wants to curate approved tool versions.

4. **Trust is social, not cryptographic.** Claude Code relies on reserved names, user consent, organizational allowlists, and SHA pinning -- not code signing or content verification. This is pragmatic and avoids the complexity of key management, but it means the trust boundary is "do you trust the registry maintainer?"

5. **Auto-discovery via project config is powerful.** The `extraKnownMarketplaces` pattern (project-level settings that prompt users to add specific registries) maps to a tsuku feature where a project could declare "this project uses recipes from registry X" in its config.

6. **Organizational lockdown is a real need.** The `strictKnownMarketplaces` pattern with regex-based host/path matching is sophisticated and addresses enterprise security requirements. Tsuku should plan for this from the start.

### For the PRD structure

The PRD should define:
- A **registry** abstraction (analogous to marketplace) with a manifest file listing available recipes
- **Source types** for both registries (where the catalog lives) and recipes (where each tool's definition lives)
- An **addressing scheme** like `tool@registry` for disambiguation
- **Trust tiers**: embedded (highest), user-added with SHA pins, floating references (lowest)
- **Organizational controls** for restricting allowed registries

## Surprises

1. **No cryptographic signing at all.** The trust model is entirely social/organizational. Given that plugins execute arbitrary code, this is a notable gap that has already been exploited (PromptArmor published a research paper on hijacking Claude Code via injected marketplace plugins). Tsuku deals with binary installations, so the threat model is even more acute -- but the pragmatic approach of "trust the registry maintainer" may be appropriate for an initial version.

2. **The `strict` field is a registry-vs-source authority toggle.** This is a subtle but important pattern. It means a marketplace isn't just a dumb index -- it can be an opinionated curator that reshapes how plugins are presented. For tsuku, this could mean a registry can override recipe metadata (e.g., mark a tool as deprecated, pin to a specific version, add organization-specific dependencies).

3. **Multiple package manager backends (npm, pip) as source types.** Claude Code doesn't just support git -- it delegates to existing package managers. For tsuku recipes, this suggests source types could include "download from GitHub releases", "build from crates.io", "pull from OCI registry" etc., each as a distinct source type.

4. **Sparse clone support for monorepos (`git-subdir`).** This is a practical optimization for monorepo-hosted plugins and suggests tsuku should consider how to efficiently fetch individual recipes from large registry repos.

## Open Questions

1. **How does Claude Code handle name collisions across marketplaces?** The `@marketplace-name` suffix disambiguates, but what happens if two marketplaces offer a plugin with the same name? Is the user always required to disambiguate, or is there a priority order?

2. **Does the JSON schema at `https://anthropic.com/claude-code/marketplace.schema.json` have additional validation rules** not documented in the prose? Fetching and analyzing it could reveal constraints on field lengths, naming patterns, etc.

3. **How mature is the third-party marketplace ecosystem?** The model is well-designed, but adoption determines whether it's truly proven. Sites like claudemarketplaces.com exist, suggesting community traction, but the scale of non-Anthropic marketplaces is unclear.

4. **How does tsuku's recipe complexity compare to Claude Code's plugin simplicity?** Plugins are mostly static files (markdown, JSON config). Recipes involve build steps, platform-specific logic, version resolution from multiple providers, and binary verification. The registry metadata schema may need to be richer.

5. **Should tsuku adopt a JSON manifest or keep TOML?** Claude Code uses JSON throughout. Tsuku recipes are TOML. The registry manifest (analogous to marketplace.json) could be either format -- TOML would be consistent with recipes, JSON would be consistent with the broader ecosystem convention for registry manifests.

## Summary

Claude Code's plugin marketplace is a well-designed registry-of-registries system where marketplaces are git-hosted JSON catalogs that list plugins with flexible source types (relative paths, GitHub, git URLs, npm, pip), using social trust (organizational allowlists, reserved names, SHA pinning) rather than cryptographic verification. The key patterns transferable to tsuku's registry unification are the `tool@registry` addressing scheme, the source-type abstraction that lets the same registry model serve embedded/local/remote recipes, and the `strict` toggle that lets registries either index or curate their contents. The biggest open question is how to adapt this model for tsuku's more complex recipe format, which involves build steps, platform logic, and binary verification that go well beyond Claude Code's static-file plugins.
