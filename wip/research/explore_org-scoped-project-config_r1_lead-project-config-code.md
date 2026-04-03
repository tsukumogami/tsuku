# Lead: How does tsuku's current .tsuku.toml parsing and project config system work?

## Findings

### Data Structures

The project config system lives in `internal/project/config.go`. The core types are:

```go
type ProjectConfig struct {
    Tools map[string]ToolRequirement `toml:"tools"`
}

type ToolRequirement struct {
    Version string `toml:"version"`
}
```

`ToolRequirement` has a custom `UnmarshalTOML` that accepts both string shorthand (`node = "20.16.0"`) and inline table form (`python = { version = "3.12" }`).

`ConfigResult` wraps the parsed config with the absolute path and directory where the `.tsuku.toml` was found.

### Config Discovery

`LoadProjectConfig(startDir)` walks up from `startDir`, resolving symlinks first, looking for a `.tsuku.toml` file. It stops at `$HOME` (unconditionally) and any additional directories in `TSUKU_CEILING_PATHS`. Max 256 tools per config file.

### Project Install Flow (no-args `tsuku install`)

In `cmd/tsuku/install.go`, when `len(args) == 0` (and no --plan/--recipe/--from/--sandbox flags), it calls `runProjectInstall(cmd)` in `cmd/tsuku/install_project.go`.

`runProjectInstall`:
1. Calls `project.LoadProjectConfig(cwd)` to find and parse the config
2. Iterates `result.Config.Tools` sorted by name
3. For each tool, calls `runInstallWithTelemetry(t.Name, resolveVersion, ...)` where `t.Name` is the **map key** from the TOML `[tools]` section

This is the critical connection: the TOML map key is used directly as the `toolName` argument to `runInstallWithTelemetry`, which passes it to `installWithDependencies` in `cmd/tsuku/install_deps.go`, which ultimately passes it to `loader.Get(toolName, ...)`.

### Recipe Loader (how tool name becomes recipe lookup)

In `internal/recipe/loader.go`, `GetWithContext` handles the name:

1. If the name matches `owner/repo:recipe` format (contains `:` and the part before `:` has exactly one `/`), it routes to `getFromDistributed()` which finds the matching distributed provider
2. Otherwise, it checks the in-memory cache, then resolves through the provider chain (local -> embedded -> registry), with a satisfies-index fallback

**Key detail**: A bare `owner/repo` name (no colon) does NOT trigger the qualified-name path in the loader. `splitQualifiedName` requires a colon separator. So `"tsukumogami/koto"` as a TOML key would fall through to the normal provider chain, which would try to find a recipe literally named `"tsukumogami/koto"` in local/embedded/registry providers -- and fail.

### Normal Install Path (with args)

When `tsuku install tsukumogami/koto` is run on the command line, the `parseDistributedName` function in `cmd/tsuku/install_distributed.go` detects the `/` and parses it as a distributed install: `source=tsukumogami/koto, recipeName=koto`. This triggers the distributed install path which registers the source, fetches from the distributed provider, and installs.

But `runProjectInstall` does NOT call `parseDistributedName`. It passes the tool name directly to `runInstallWithTelemetry`, bypassing the distributed-name detection entirely.

### Resolver (shell integration / autoinstall path)

`internal/project/resolver.go` provides a `Resolver` that maps commands (binary names) to project-pinned versions. It uses a `LookupFunc` (binary index lookup) to find which recipe provides a command, then checks if that recipe appears in `config.Tools`. This path uses the recipe name (not the tool map key) as the lookup key into `config.Tools`.

### TOML Key Constraints

Experimentally verified: TOML bare keys cannot contain `/`. An org-scoped name like `tsukumogami/koto` must be quoted:

```toml
[tools]
"tsukumogami/koto" = "1.0.0"    # works (quoted key)
tsukumogami/koto = "1.0.0"      # TOML parse error
```

The BurntSushi/toml library correctly parses quoted keys with `/` into the `map[string]ToolRequirement` map, so the config parsing itself works fine.

## Implications

1. **Project install is broken for org-scoped names.** The `runProjectInstall` function passes tool names straight to `runInstallWithTelemetry` without any distributed-name detection. An org-scoped name like `tsukumogami/koto` would hit the recipe loader, fail `splitQualifiedName` (no colon), and then try to find `tsukumogami/koto` as a bare recipe name in local/embedded/registry -- which would fail with "recipe not found".

2. **The fix is localized.** The gap is specifically in `runProjectInstall` (in `cmd/tsuku/install_project.go`). It needs to detect org-scoped names and either call `parseDistributedName` + the distributed install path, or replicate the distributed setup logic before calling `runInstallWithTelemetry`. The underlying loader already supports distributed recipes through the qualified-name path.

3. **The resolver path also needs attention.** The `Resolver.ProjectVersionFor` method matches on `m.Recipe` (the recipe name from the binary index), not on the original TOML key. If a distributed recipe `tsukumogami/koto` installs a binary called `koto`, the binary index would store `recipe: "koto"` (the bare name). But the config key is `"tsukumogami/koto"`. The resolver would fail to match because `config.Tools["koto"]` doesn't exist -- the key is `"tsukumogami/koto"`.

4. **TOML quoting is a minor UX friction.** Users must write `"tsukumogami/koto" = "1.0.0"` with quotes. This is standard TOML but less ergonomic than bare keys. An alternative representation (like dotted keys `tsukumogami.koto`) would collide with TOML's nested table syntax.

## Surprises

1. **Two completely separate code paths for distributed vs project install.** The command-line install path has full distributed-name handling (`parseDistributedName`, `ensureDistributedSource`, source collision checks, recipe hash recording). The project install path has none of this -- it's a straight loop calling `runInstallWithTelemetry`.

2. **The resolver's lookup key mismatch.** The resolver matches on recipe name from the binary index, but the config map is keyed by the TOML key (which for org-scoped tools would be `tsukumogami/koto`, not `koto`). This is a semantic mismatch that would affect shell integration / autoinstall even if the project install path were fixed.

3. **No validation of tool names in the config.** The config parser accepts any string as a tool key. There's no validation that the key is a valid recipe name, a valid distributed name, or even non-empty. Bad keys just silently fail at install time.

## Open Questions

1. **Should the config distinguish distributed source from recipe name?** Currently the map key serves as both the display name and the recipe lookup key. For distributed recipes, these diverge (source is `tsukumogami/koto`, recipe name is `koto`). Should there be a `source` field in `ToolRequirement`?

2. **How should the resolver handle org-scoped keys?** Should it strip the org prefix when matching against the binary index? Or should the binary index store the full source-qualified name?

3. **Should `ToolRequirement` grow a `source` field?** Something like `"koto" = { version = "1.0.0", source = "tsukumogami/koto" }` would keep bare recipe names as keys while recording the source. But this changes the config format.

4. **What about source registration and collision checks?** The CLI install path handles unregistered sources interactively. How should this work for batch project install? Should all sources be pre-registered?

## Summary

The `.tsuku.toml` system parses a `[tools]` map of name-to-version entries using BurntSushi/toml, walks parent directories to find the config, and feeds each tool name directly into `runInstallWithTelemetry` during project install. There is zero handling of org-scoped names (containing `/`) in the project install path -- the distributed-name detection (`parseDistributedName`) only exists in the CLI arg parsing path, making this a localized gap in `cmd/tsuku/install_project.go` rather than a deep architectural issue. A secondary mismatch exists in the resolver, where the binary index stores bare recipe names but the config map would be keyed by the full org-scoped name, breaking shell integration version pinning.
