# Lead: How does Claude's settings.json and plugin marketplace model namespaced plugin references?

## Findings

### The `name@marketplace` Convention

Claude Code uses a two-part identifier format: `plugin-name@marketplace-name`. This appears consistently across all configuration surfaces:

**In `~/.claude/settings.json` (user-level enablement):**
```json
{
  "enabledPlugins": {
    "gopls-lsp@claude-plugins-official": true,
    "rust-analyzer-lsp@claude-plugins-official": true,
    "superpowers@claude-plugins-official": true,
    "tsukumogami@tsukumogami": true,
    "shirabe@shirabe": true
  }
}
```

**In project-level `.claude/settings.local.json`:**
```json
{
  "enabledPlugins": {
    "shirabe@shirabe": true,
    "tsukumogami@tsukumogami": true
  }
}
```

**In `~/.claude/plugins/installed_plugins.json`:**
```json
{
  "version": 2,
  "plugins": {
    "gopls-lsp@claude-plugins-official": [{ "scope": "user", ... }],
    "tsukumogami@tsukumogami": [{ "scope": "project", "projectPath": "..." }]
  }
}
```

The `@` separator creates an unambiguous two-part key: the plugin name is always scoped by its marketplace origin. This avoids collision -- two marketplaces could each define a plugin named "lint" and they'd be `lint@marketplace-a` vs `lint@marketplace-b`.

### Marketplace Registry: Separating "Where" from "What"

The marketplace system cleanly separates source resolution from plugin identity through two layers:

**Layer 1: Marketplace Registration** (`settings.json` -> `extraKnownMarketplaces`)

This maps a marketplace name to a source location. Sources can be GitHub repos or local filesystem paths:

```json
{
  "extraKnownMarketplaces": {
    "shirabe": {
      "source": { "source": "github", "repo": "tsukumogami/shirabe" },
      "autoUpdate": true
    },
    "tsukumogami": {
      "source": { "source": "file", "path": "/path/to/tools/.claude-plugin/marketplace.json" }
    },
    "anthropic-agent-skills": {
      "source": { "source": "github", "repo": "anthropics/skills" }
    }
  }
}
```

**Layer 2: Marketplace Manifest** (`.claude-plugin/marketplace.json` in each marketplace)

Each marketplace has a manifest listing the plugins it provides:

```json
{
  "name": "koto",
  "description": "Workflow skills for koto",
  "owner": { "name": "tsukumogami" },
  "plugins": [
    {
      "name": "koto-skills",
      "description": "Workflow skills for koto",
      "version": "0.5.1-dev",
      "source": "./plugins/koto-skills"
    }
  ]
}
```

**Layer 3: Plugin Manifest** (`.claude-plugin/plugin.json` in each plugin directory)

The actual plugin declares its skills, agents, and hooks:

```json
{
  "name": "tsukumogami",
  "version": "0.1.0",
  "description": "Workflow skills for the tsuku project",
  "skills": "./skills/",
  "agents": ["./agents/architect-reviewer.md", ...]
}
```

### Cache Directory Structure

Installed plugins land in `~/.claude/plugins/cache/{marketplace}/{plugin}/{version}/`:

```
~/.claude/plugins/cache/
  claude-plugins-official/
    gopls-lsp/1.0.0/
    superpowers/{commit-hash}/
  tsukumogami/
    tsukumogami/0.1.0/
  shirabe/
    shirabe/0.3.1-dev/
  koto/
    koto-skills/{version}/
```

The directory hierarchy mirrors the `name@marketplace` identifier: `cache/{marketplace}/{plugin}/{version}`.

### Skill Namespacing at Runtime

When skills are loaded, they appear as `marketplace:skill-name` in the UI (e.g., `shirabe:explore`, `tsukumogami:implement`). This uses `:` as the runtime separator, distinct from the `@` in configuration files. The colon convention is for user-facing invocation; the at-sign is for machine-readable config.

### Blocklist Uses Same Convention

Even the blocklist (`~/.claude/plugins/blocklist.json`) uses the same `name@marketplace` format:
```json
{
  "plugins": [
    { "plugin": "code-review@claude-plugins-official", "reason": "just-a-test" },
    { "plugin": "fizz@testmkt-marketplace", "reason": "security" }
  ]
}
```

### Known Marketplaces Tracking

`~/.claude/plugins/known_marketplaces.json` merges user-defined marketplaces with discovered ones, tracking their install locations and last-updated timestamps. This acts as a local cache of the registry.

## Implications

For `.tsuku.toml` project config design:

1. **The `@` pattern is proven for config files.** Claude Code uses `name@scope` as flat JSON keys without any issues. The at-sign is unambiguous and doesn't conflict with common identifier characters.

2. **TOML can use the same pattern.** While `org/tool` fails as a bare TOML key, `org@tool` or `tool@org` would work as TOML string values (in arrays or as values). Alternatively, quoted keys like `"tsukumogami/koto"` are valid TOML.

3. **Separating registry from identity is valuable.** Claude Code doesn't embed the GitHub URL in the plugin reference. The config says *what* to install (`tsukumogami@tsukumogami`) and a separate registry says *where* to find it. For tsuku, the project config should reference tools by name, and the registry/recipe system resolves sources.

4. **Scope metadata belongs with installation records, not identity.** Claude Code tracks `scope: "user"` vs `scope: "project"` in the installed_plugins manifest, not in the identifier itself. Tool identity stays clean.

5. **Two separator conventions for two contexts.** Config files use `@` for machine parsing; runtime invocation uses `:` for human ergonomics. Tsuku could similarly have `org/tool` in CLI commands but a different representation in TOML files.

## Surprises

1. **Marketplace name can equal plugin name.** The `tsukumogami@tsukumogami` identifier shows that when a marketplace contains a single primary plugin, they share the same name. This creates a slightly odd-looking but functional reference.

2. **No version in the reference key.** Plugin references in `enabledPlugins` carry no version information -- just `name@marketplace: true`. Version resolution is handled entirely by the installation system. This keeps config files simple.

3. **The `data/` directory uses a different convention.** Inside `~/.claude/plugins/data/`, directories are named `{plugin}-{marketplace}` (hyphen-joined, e.g., `gopls-lsp-claude-plugins-official`), while `cache/` uses nested `{marketplace}/{plugin}` paths. Two different flattening strategies in the same system.

4. **File-based marketplaces.** Claude supports `"source": "file"` alongside `"source": "github"`, meaning marketplaces can be local directories. This is analogous to local recipe development in tsuku.

## Open Questions

1. **Would TOML support `tool@org` as a bare key?** TOML bare keys allow alphanumerics, dashes, and underscores but not `@` or `/`. So any namespaced reference in TOML must use quoted keys or be a string value, not a bare key.

2. **How does Claude resolve conflicts when the same plugin name exists in multiple marketplaces?** The `@marketplace` suffix appears to make this unambiguous, but what happens if a user omits the `@marketplace` part?

3. **Is the marketplace concept analogous to tsuku's recipe registry?** Claude's marketplace = a collection of plugins from a source. Tsuku's registry = a collection of recipes. The parallel might suggest `tool@registry` or using the org name as an implicit registry scope.

4. **Should `.tsuku.toml` reference recipes by flat name or by scoped name?** If tsuku ever supports multiple recipe registries (community, official, private), scoped names become necessary.

## Summary

Claude Code uses a `name@marketplace` convention as the universal plugin identifier across all config surfaces -- settings.json, installed_plugins.json, and blocklist. The system cleanly separates identity (the `name@marketplace` string) from source resolution (marketplace manifests that map names to GitHub repos or local paths) and from versioning (tracked only in installation records, never in the reference key). For `.tsuku.toml`, the key takeaway is that namespaced identifiers work well as flat string values or quoted keys, and the config should reference tools by scoped name while delegating source resolution to the registry layer.
