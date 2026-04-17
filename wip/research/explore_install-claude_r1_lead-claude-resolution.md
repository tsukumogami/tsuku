# Lead: Claude-specific resolution

## Findings

### A) Recipe content needed for `claude-code`

The recipe must use `npm_install` with the scoped package name. A precedent for scoped npm packages exists in `recipes/a/amplify.toml` (which uses `@aws-amplify/cli`). The `NpmBuilder` (`internal/builders/npm.go`) explicitly handles scoped packages: `npmPackageNameRegex` accepts `@scope/package`, `parseBinField` calls `unscopedPackageName()` to strip the `@anthropic-ai/` prefix, and `fetchPackageInfo` constructs the API URL with `baseURL.JoinPath(packageName)` which handles the `@` character correctly.

Claude Code publishes the `claude` binary via its `bin` field in `package.json`. The NpmBuilder `discoverExecutables()` method will read the `bin` map from the npm registry metadata for `@anthropic-ai/claude-code` and extract `claude` automatically.

A valid handcrafted recipe would be:

```toml
[metadata]
name = "claude"
description = "Claude Code - Anthropic's AI coding assistant"
homepage = "https://github.com/anthropic/claude-code"

[[steps]]
action = "npm_install"
package = "@anthropic-ai/claude-code"
executables = ["claude"]

[verify]
command = "claude --version"
pattern = ""
```

The recipe name `claude` (not `claude-code`) matches the installed binary name. The `npm_install` action requires nodejs as a runtime dependency (declared in `NpmInstallAction.Dependencies()`), so tsuku will auto-install it. There are no GitHub release binaries for Claude Code — it's npm-only.

### B) End-to-end install flow after a discovery registry hit

**Step 1: Recipe lookup miss**
`tsuku install claude` → `loader.Get("claude", ...)` returns an error (no recipe exists in embedded registry or `$TSUKU_HOME/recipes/`).

**Step 2: Discovery fallback**
`tryDiscoveryFallback("claude")` is invoked. This calls `runDiscoveryWithOptions("claude", ...)` which runs the resolver chain:
- `RegistryLookup.Resolve()`: reads `recipes/discovery/c/cl/claude.json` (if present). Returns `DiscoveryResult{Builder: "npm", Source: "@anthropic-ai/claude-code", Confidence: ConfidenceRegistry}`. Chain stops here.
- If no registry entry: falls through to `EcosystemProbe.Resolve()`, which queries all ecosystems in parallel. The npm prober would call `fetchPackageInfo(ctx, "claude")` — but this would find an unrelated npm package named "claude" (the probe uses `strings.EqualFold(result.Source, toolName)` to match, so it would only match if the source equals "claude" exactly, not "@anthropic-ai/claude-code"). This is exactly the wrong-match problem the discovery entry solves.

**Step 3: Recipe generation**
`tryDiscoveryFallback` constructs `fromArg = "npm:@anthropic-ai/claude-code"` and calls `runCreate(nil, []string{"claude"})` with `createFrom = "npm:@anthropic-ai/claude-code"`.

`runCreate` → `parseFromFlag("npm:@anthropic-ai/claude-code")` → builder="npm", sourceArg="@anthropic-ai/claude-code". `normalizeEcosystem("npm")` → "npm". Fetches `NpmBuilder`. Calls `builder.CanBuild(ctx, {Package: "claude", SourceArg: "@anthropic-ai/claude-code"})` — but note: `CanBuild` uses `req.Package` which is "claude" not the scoped name. This is a potential issue: `CanBuild` would query `https://registry.npmjs.org/claude`, not `@anthropic-ai/claude-code`.

Looking more closely: `Build()` also uses `req.Package` for the `fetchPackageInfo` call and as the `package` field in the recipe step. With `sourceArg = "@anthropic-ai/claude-code"` and `req.Package = "claude"`, the NpmBuilder would fetch the wrong package. The discovery registry entry format uses `source` = the npm package name (`@anthropic-ai/claude-code`), but the `buildReq.Package` is the tool name (`claude`). The `NpmBuilder.Build()` method uses `req.Package` not `req.SourceArg` for the npm fetch.

This means the flow actually expects either: (a) a handcrafted recipe is found before discovery runs (recipe path `recipes/c/claude.toml` would be loaded by the embedded registry), or (b) the discovery registry entry must use `source = "claude"` to match a real npm package named "claude". For Claude Code specifically, a handcrafted recipe in `recipes/c/claude.toml` is the correct approach — the discovery entry alone won't produce a correct recipe because NpmBuilder's `Build()` would query the wrong npm package name.

**Step 4: Installation**
If `runCreate` succeeds, the generated/cached recipe is stored at `$TSUKU_HOME/recipes/claude.toml`. Then `runInstallWithTelemetry("claude", "", "", true, "", telemetryClient)` is called. This loads the recipe, resolves the npm version, calls `NpmInstallAction.Execute()` → runs `npm install -g --prefix=$TSUKU_HOME/tools/claude-{version} @anthropic-ai/claude-code@{version}`, verifies the `claude` binary exists in `$TSUKU_HOME/tools/claude-{version}/bin/`, then creates the symlink in `$TSUKU_HOME/bin/`.

### sync-disambiguations.yml workflow

This workflow triggers on pushes to main that modify `data/discovery-seeds/disambiguations.json`. It builds and runs `cmd/sync-disambiguations`, which reads the seed file and writes per-tool JSON files to `data/disambiguations/` (specifically `data/disambiguations/curated.jsonl` and the `audit/` subdirectory). The `curated.jsonl` file is used by the runtime for deterministic builder selection when multiple ecosystem probers return matches. Adding "claude" or "claude-code" entries to `disambiguations.json` would be appropriate if the tool name is ambiguous at ecosystem probe time.

## Implications

1. **A handcrafted recipe is necessary.** The NpmBuilder's `Build()` method uses `req.Package` (the tool name, "claude") not `req.SourceArg` (the scoped npm name) when calling `fetchPackageInfo`. Without a handcrafted recipe, the auto-generation path would fetch metadata for the wrong npm package.

2. **A discovery registry entry is also necessary** but for a different reason: without it, the ecosystem probe stage would search for a package literally named "claude" in all ecosystems. It would find an unrelated npm package. The discovery entry `{builder: "npm", source: "@anthropic-ai/claude-code"}` short-circuits this and routes directly to recipe generation.

3. **The recipe name should be `claude`, not `claude-code`.** The binary name is `claude`. File: `recipes/c/claude.toml`. Discovery entry: `recipes/discovery/c/cl/claude.json`.

4. **The `npm_install` action handles scoped packages** — the `amplify` recipe (`@aws-amplify/cli`) demonstrates this already works. `isValidNpmPackage` allows `@` characters. The recipe's `package` field must be the full scoped name `@anthropic-ai/claude-code`.

5. **nodejs is auto-installed** as a dependency because `NpmInstallAction.Dependencies()` declares it as an InstallTime dependency. Users don't need to pre-install Node.

## Surprises

- The `filterMatches` function in `EcosystemProbe` uses `strings.EqualFold(outcome.result.Source, toolName)` for name matching. The probe result's `Source` is the exact npm package name returned from the registry. For the probe to match "claude" it would need to probe `fetchPackageInfo("claude")` and get back `Source: "claude"` — not `Source: "@anthropic-ai/claude-code"`. So even if there's a real npm package named "claude", the ecosystem probe would find it, not Claude Code. This completely confirms why a discovery registry entry (which stores the correct scoped source) is essential.

- The `NpmBuilder.Build()` method has a disconnect: it uses `req.Package` for the npm fetch, but the discovery result's `source` field contains `@anthropic-ai/claude-code`. The `sourceArg` in the build request is meant for builders like GitHub that need it (owner/repo), but npm's `Build()` ignores `req.SourceArg` entirely. This is a gap: for tools where the tool name differs from the npm package name (like `claude` vs `@anthropic-ai/claude-code`), auto-generation from discovery won't work — a handcrafted recipe is the only path.

- The `data/disambiguations/curated.jsonl` is distinct from `recipes/discovery/`. Disambiguation data is for ecosystem probe disambiguation (when multiple probers return results for the same tool). Discovery registry entries are for pre-empting the probe entirely. "claude" needs a discovery entry, not a disambiguation entry, because the probe would find the wrong package.

## Open Questions

1. Does `NpmBuilder.CanBuild()` need to use `req.SourceArg` when it's set, falling back to `req.Package`? This would fix the auto-generation gap for scoped packages and tools with non-matching names.

2. Should the discovery registry support a `recipe_name` field distinct from `source`, so that `source` holds the npm package name and the tool name is derived from the file path? The current schema has no such separation.

3. Is there a GitHub releases page for Claude Code that would allow a `github_archive` recipe instead of (or in addition to) `npm_install`? The npm path requires nodejs as a runtime dependency; a prebuilt binary would be lighter. As of April 2026, Anthropic hasn't published standalone binaries — the npm package is the only distribution.

4. Should `claude-code` be added as a `satisfies` alias pointing to the `claude` recipe, so that `tsuku install claude-code` works alongside `tsuku install claude`?

## Summary (3 sentences)

A handcrafted recipe at `recipes/c/claude.toml` using `npm_install` with `package = "@anthropic-ai/claude-code"` and `executables = ["claude"]` is the correct approach; auto-generation from discovery won't work because `NpmBuilder.Build()` uses the tool name ("claude") not the scoped npm name for the registry fetch. A companion discovery entry at `recipes/discovery/c/cl/claude.json` with `{builder: "npm", source: "@anthropic-ai/claude-code"}` is also required to prevent the ecosystem probe from matching an unrelated npm package named "claude". After a registry hit, the install command calls `tryDiscoveryFallback` → `runCreate` (recipe generation) → `runInstallWithTelemetry` (npm install with prefix isolation, nodejs auto-installed as a dependency, binary symlinked into `$TSUKU_HOME/bin/`).
