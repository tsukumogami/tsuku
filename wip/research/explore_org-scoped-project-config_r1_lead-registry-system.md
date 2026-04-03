# Lead: How does tsuku's registry system currently handle org-scoped recipes?

## Findings

### 1. Two distinct registry systems

Tsuku has two separate registry systems that share the word "registry" but work differently:

**Central Registry** (`internal/registry/`): A single hardcoded GitHub-hosted registry at `https://raw.githubusercontent.com/tsukumogami/tsuku/main`. Recipes live at `recipes/{first-letter}/{name}.toml`. This system has no concept of org-scoping -- it maps recipe names like "kubectl" to paths like `recipes/k/kubectl.toml`.

**Distributed Registries** (`internal/distributed/`, `internal/userconfig/`): Third-party recipe sources stored as `owner/repo` entries in `$TSUKU_HOME/config.toml` under `[registries]`. Each entry is a `RegistryEntry` with a URL and an `auto_registered` flag.

### 2. Org-scoped detection: the `/` in the name

In `cmd/tsuku/install.go` (line 213), the install command loops through args and calls `parseDistributedName(arg)` on each one. This function (in `install_distributed.go`) checks for `/` in the name -- if present, it's treated as a distributed install request.

The parsing supports four formats:
- `owner/repo` -> source=owner/repo, recipe=repo, version=""
- `owner/repo:recipe` -> source=owner/repo, recipe=recipe, version=""
- `owner/repo@version` -> source=owner/repo, recipe=repo, version=version
- `owner/repo:recipe@version` -> all fields explicit

Path traversal (`..`) is rejected by returning nil to fall through to normal lookup.

### 3. Auto-registration flow

When `parseDistributedName` returns non-nil, the install command calls `ensureDistributedSource(source, autoApprove, sysCfg)` before fetching the recipe. This function:

1. Validates format via `validateRegistrySource()` (must be exactly `owner/repo`)
2. Checks if a provider already exists in the loader (short-circuit)
3. Loads user config and checks if already registered in `config.toml`
4. If not registered and `strict_registries` is enabled, returns an error
5. If not registered and not strict, prompts user for confirmation (or auto-approves with `--yes`)
6. Calls `autoRegisterSource()` which writes to `config.toml` with `AutoRegistered: true`
7. Calls `addDistributedProvider()` to dynamically register a `DistributedRegistryProvider` in the loader for the current session

### 4. Recipe resolution in the loader

`internal/recipe/loader.go` uses a provider chain. Qualified names (`owner/repo:recipe`) are routed directly to the matching distributed provider via `getFromDistributed()`. Bare names go through the full chain: local -> embedded -> registry -> satisfies fallback.

The install flow builds the qualified name (`dArgs.Source + ":" + dArgs.RecipeName`) and calls `loader.GetWithContext()` with it. After loading, it also caches the recipe under the bare name for dependency resolution.

### 5. Project install does NOT handle org-scoped names

The `runProjectInstall()` function in `install_project.go` loads `.tsuku.toml` via `project.LoadProjectConfig()`, which parses a simple `[tools]` map of `name -> ToolRequirement{Version}`. It then calls `runInstallWithTelemetry(t.Name, ...)` for each tool.

Critically, `runProjectInstall` does NOT call `parseDistributedName` or `ensureDistributedSource`. It passes tool names directly to the normal install flow. If you put `"tsukumogami/koto"` as a key in `.tsuku.toml`, it would be treated as a bare recipe name -- which would fail at the registry level since the central registry has no recipe named `tsukumogami/koto`.

### 6. The `ToolRequirement` model is minimal

`ProjectConfig.Tools` is `map[string]ToolRequirement` where `ToolRequirement` only has a `Version` field. There is no `source`, `registry`, or `from` field. The TOML key is the tool name.

### 7. Registry data model

A registered registry in `config.toml` looks like:
```toml
[registries.tsukumogami/koto]
url = "https://github.com/tsukumogami/koto"
auto_registered = true
```

The `RegistryEntry` struct has `URL` (string) and `AutoRegistered` (bool). The map key is the `owner/repo` string.

### 8. Distributed provider initialization

At startup, registries from config.toml are NOT automatically loaded as providers. They are loaded on-demand when `ensureDistributedSource` detects the source is registered but has no provider yet. This lazy initialization means the project install path would need explicit logic to bootstrap providers for any distributed sources referenced in `.tsuku.toml`.

## Implications

1. **Gap in project install**: `.tsuku.toml` currently cannot represent org-scoped tools. The project install path (`runProjectInstall`) has no distributed source handling -- it skips the `parseDistributedName` / `ensureDistributedSource` flow entirely.

2. **Two possible approaches**: Either (a) extend `ToolRequirement` to include a `source` field and add distributed logic to `runProjectInstall`, or (b) allow `/` in tool keys in `.tsuku.toml` and teach `runProjectInstall` to detect and handle them the same way the CLI arg path does.

3. **Auto-registration from config**: The auto-registration machinery exists and works well for CLI usage. For project config, the question is whether running `tsuku install` (no args) should auto-register distributed sources found in `.tsuku.toml`, or whether those should be pre-registered.

4. **Lazy provider init matters**: Since distributed providers aren't loaded at startup from config.toml, the project install codepath would need to call `ensureDistributedSource` (or equivalent) for each org-scoped tool before attempting to load its recipe.

## Surprises

1. **Distributed providers are NOT loaded at startup.** Even if a registry is in config.toml, the provider is only created when `ensureDistributedSource` runs during an install. This means adding an org-scoped tool to `.tsuku.toml` and running `tsuku install` would fail even if the registry was previously registered, because `runProjectInstall` doesn't call `ensureDistributedSource`.

2. **The central registry (`internal/registry/`) is completely separate from distributed registries.** They share no code or data structures. The central registry uses letter-bucketed paths; distributed registries use GitHub raw content with manifest.json discovery.

3. **Recipe caching under bare name is explicit.** After loading a distributed recipe via the qualified name, `install.go` explicitly calls `loader.CacheRecipe(dArgs.RecipeName, r)` so that dependency resolution (which uses bare names) can find it. This dual-caching pattern would need to be replicated in the project install path.

## Open Questions

1. What should the `.tsuku.toml` syntax look like for org-scoped tools? Options include `"tsukumogami/koto" = "latest"` (slash in key), `koto = { source = "tsukumogami/koto" }` (explicit source field), or something else.

2. Should `.tsuku.toml` have a `[registries]` section to pre-declare required registries, or should it rely on auto-registration during `tsuku install`?

3. Should `strict_registries` mode block project-config-triggered auto-registration? If a `.tsuku.toml` declares an org-scoped tool, is that sufficient "registration" or should the user still run `tsuku registry add` first?

4. How should version pinning work for distributed recipes? The current `ToolRequirement` only has a `Version` field -- should it also support pinning the recipe hash (which `recordDistributedSource` already computes)?

## Summary

Tsuku detects org-scoped tools via the `/` character in `parseDistributedName()` during CLI install, then auto-registers the source in `config.toml` and dynamically creates a distributed provider for the session. The project install path (`runProjectInstall` reading `.tsuku.toml`) completely lacks this distributed source handling -- it passes tool names directly to the standard install flow without org-scope detection, provider bootstrapping, or auto-registration. Bridging this gap requires teaching both the `.tsuku.toml` data model and the project install codepath about distributed sources, including lazy provider initialization and the dual-caching pattern used by the CLI install path.
