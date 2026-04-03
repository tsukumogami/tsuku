# Lead: How do other CLI tools represent namespaced packages in config files?

## Findings

### 1. npm/pnpm -- package.json (JSON)

**Format:** JSON

**Namespace representation:** `@scope/package` as a string key in the dependencies object.

```json
{
  "dependencies": {
    "@myorg/mypackage": "^1.3.0"
  }
}
```

JSON allows any string as an object key, so the `/` in `@scope/package` works without any special handling. The `@` prefix disambiguates scoped from unscoped packages.

**Registry declaration:** `.npmrc` file declares scope-to-registry mappings: `@myorg:registry=https://registry.myorg.com/`. The package.json itself doesn't contain registry URLs -- it relies on external config for resolution.

**Self-contained:** No. Requires `.npmrc` for non-default registries.

**pnpm catalogs** add a layer: `pnpm-workspace.yaml` defines named version catalogs, and packages reference them via `catalog:react17` strings. This is a version indirection layer, not a namespace mechanism.

### 2. Cargo -- Cargo.toml (TOML)

**Format:** TOML

**Namespace representation:** Dependencies use a flat name as the key, with a `registry` attribute pointing to a named registry:

```toml
[dependencies]
secret-crate = { version = "1.0", registry = "my-registry" }
```

There is no org/package namespace in Cargo.toml keys. Crate names are globally unique within a registry. The separation between "which registry" and "which crate" uses a field-based approach rather than encoding the registry into the key name.

**Registry declaration:** Separate config file `.cargo/config.toml`:

```toml
[registries.my-registry]
index = "https://my-intranet:8080/git/index"
```

**Self-contained:** No. Registry definitions live in `.cargo/config.toml`, not `Cargo.toml`. This is a deliberate split -- the manifest declares what you need, the config declares where to find it.

### 3. Homebrew -- Brewfile (Ruby DSL)

**Format:** Ruby DSL (not a data format)

**Namespace representation:** Two-step approach: declare the tap, then use a fully-qualified formula name:

```ruby
tap "apple/apple"
brew "apple/apple/game-porting-toolkit"
```

The fully-qualified name uses `/` freely because it's a string argument to a function, not a key. The format is `user/repo/formula` (three segments).

**Registry declaration:** The `tap` directive registers a third-party repository. It's inline in the Brewfile.

**Self-contained:** Yes. The Brewfile contains both tap declarations and package references. A fresh machine with Homebrew can resolve everything from the Brewfile alone.

### 4. mise -- mise.toml (TOML)

**Format:** TOML

**Namespace representation:** Uses a `backend:identifier` prefix scheme with quoted TOML keys:

```toml
[tools]
node = "20"
"cargo:ripgrep" = "latest"
"npm:prettier" = "3"
"github:cli/cli" = "2.40.1"
```

For the GitHub backend specifically, the key becomes `"github:owner/repo"` -- a quoted TOML key containing both `/` and `:`. When extra options are needed:

```toml
[tools."github:yt-dlp/yt-dlp"]
version = "latest"
asset_pattern = "yt-dlp_linux.zip"
```

This is the closest analog to tsuku's problem. mise chose to:
- Use `:` as the backend/namespace delimiter (not `/`)
- Use quoted keys throughout when special characters are present
- Embed the full identifier (including `/`) in the key string
- Maintain a built-in registry that maps short names (like `node`) to full backend identifiers (like `core:node`)

**Registry declaration:** Built-in. The mise registry maps short names to backends. No separate registry file needed for built-in tools.

**Self-contained:** Yes for built-in tools. For arbitrary GitHub tools, the full `github:owner/repo` key is self-describing.

### 5. Go modules -- go.mod (custom format)

**Format:** Custom line-based format (not TOML/JSON/YAML)

**Namespace representation:** Module paths are URLs that double as identifiers:

```
require (
    github.com/gorilla/mux v1.8.0
    golang.org/x/tools v0.1.0
)
```

The `/` is natural because module paths are URL-derived. There's no ambiguity because the format is purpose-built.

**Registry declaration:** Implicit. The module path IS the resolution mechanism -- Go tools fetch from the path (or a proxy like proxy.golang.org). Custom resolution uses `GOPROXY` and `GONOSUMCHECK` environment variables, or `GOFLAGS` in `.go-env`.

**Self-contained:** Yes. The `go.mod` file contains everything needed. Registry configuration is environment-level, but the default (proxy.golang.org) works without config.

### 6. Poetry -- pyproject.toml (TOML)

**Format:** TOML

**Namespace representation:** Python package names don't have formal namespaces (no org scopes). When a package needs to come from a non-default source, it uses a `source` field:

```toml
[tool.poetry.dependencies]
requests = { version = "^2.13.0", source = "private" }

[[tool.poetry.source]]
name = "private"
url = "https://private.pypi.org/simple/"
priority = "primary"
```

This is the "declare sources separately, reference by name" pattern. The package key stays flat; the source is an attribute.

**Registry declaration:** Inline in pyproject.toml via `[[tool.poetry.source]]` array of tables.

**Self-contained:** Yes. Both sources and dependencies are in pyproject.toml.

### 7. Docker Compose -- docker-compose.yml (YAML)

**Format:** YAML

**Namespace representation:** Image references encode the full registry/namespace/image path as a string value (not a key):

```yaml
services:
  app:
    image: registry.example.com/myorg/myapp:latest
```

The format is `registry/namespace/image:tag`. Docker Hub is the implicit default, so `nginx:latest` resolves to `docker.io/library/nginx:latest`.

**Registry declaration:** None in the compose file. Registry auth is in `~/.docker/config.json`.

**Self-contained:** Partially. Image references are self-describing (the URL is the identifier), but auth is external.

## Patterns Summary

| Tool | Format | Namespace in Key? | Slash in Key? | Registry Declaration | Self-contained? |
|------|--------|-------------------|---------------|---------------------|-----------------|
| npm | JSON | Yes (`@scope/pkg`) | Yes (JSON allows it) | External (.npmrc) | No |
| Cargo | TOML | No (flat name + `registry` field) | No | External (.cargo/config.toml) | No |
| Homebrew | Ruby DSL | N/A (string args) | Yes (string values) | Inline (`tap`) | Yes |
| mise | TOML | Yes (`"backend:owner/repo"`) | Yes (quoted keys) | Built-in + inline | Yes |
| Go | Custom | N/A (URL paths) | Yes (custom format) | Implicit (URL-based) | Yes |
| Poetry | TOML | No (flat name + `source` field) | No | Inline (`[[source]]`) | Yes |
| Docker | YAML | N/A (string values) | Yes (string values) | External (docker config) | Partial |

## Implications

### Pattern A: Quoted keys with embedded namespace (mise approach)

```toml
[tools]
"tsukumogami/koto" = "latest"
```

Pros: Simple, self-describing, single location for the tool reference. The org prefix directly implies the registry.
Cons: Quoted keys feel foreign in TOML. Dotted key access in code requires special handling.

### Pattern B: Flat key + source attribute (Cargo/Poetry approach)

```toml
[tools]
koto = { version = "latest", registry = "tsukumogami" }

[[registry]]
name = "tsukumogami"
url = "https://github.com/tsukumogami/tsuku-registry"
```

Pros: Clean TOML keys, familiar from Cargo/Poetry. Registry declaration is explicit.
Cons: Verbose. Name collision risk if two registries have a tool with the same name. Not immediately clear where `koto` comes from when scanning the file.

### Pattern C: Backend prefix with colon delimiter (mise approach)

```toml
[tools]
"tsukumogami:koto" = "latest"
```

Pros: Avoids the `/` character (uses `:` instead). Visually separates org from tool.
Cons: Still requires quoted keys. Introduces a non-standard delimiter that differs from the `org/tool` syntax used on the command line.

### Pattern D: Dotted table sections (TOML-native)

```toml
[tools.tsukumogami]
koto = "latest"

[tools]
serve = "latest"
```

Pros: Pure TOML, no quoted keys. The org becomes a natural grouping.
Cons: Mixes two styles for tools depending on whether they're org-scoped. Reading the full tool list requires scanning multiple sections.

### Pattern E: Separate registries section + fully-qualified string values

```toml
[registries]
tsukumogami = "https://github.com/tsukumogami/tsuku-registry"

[tools]
serve = "latest"
koto = { version = "latest", from = "tsukumogami" }
```

Pros: Clean separation. Registry URLs are declared once. Self-contained.
Cons: Two sections to maintain. The `from` field is less discoverable than having the org in the tool name.

### Best fit for tsuku

The strongest patterns for tsuku's use case are:

1. **mise's quoted-key approach** (Pattern A or C) -- because tsuku already uses `org/tool` on the command line, and TOML quoted keys handle `/` fine. The key question is whether the parsing code can handle quoted keys with slashes.

2. **Homebrew's self-contained approach** -- declaring registries inline in the same file. This is critical for CI use cases.

3. **Cargo/Poetry's separation** -- keeping the tool key clean and using a field to reference the registry. This is the most TOML-idiomatic but the most verbose.

## Surprises

1. **mise already solved this exact problem in TOML.** Their `"github:owner/repo"` syntax in mise.toml demonstrates that quoted TOML keys with slashes work in practice at scale. This is the strongest precedent.

2. **No tool uses TOML dotted keys for namespacing.** I expected at least one tool to use `[tools.org]` table nesting, but none do. Every tool that uses TOML either avoids namespaced keys entirely (Cargo, Poetry) or uses quoted strings (mise).

3. **The self-contained vs. split-config divide is sharp.** Tools either put everything in one file (Homebrew, Poetry, Go) or split registry config into a separate file (npm, Cargo, Docker). There's little middle ground. The tools that split tend to be older; newer tools prefer self-contained configs.

4. **npm's `@scope/package` works only because JSON allows any string as a key.** The `@` prefix convention was designed specifically for JSON -- it has no direct analog in TOML bare keys.

## Open Questions

1. Does tsuku's current TOML parser (which Go library?) handle quoted keys with `/` correctly? The TOML spec allows it, but parser implementations vary.

2. Should the org prefix in `.tsuku.toml` match the command-line syntax (`tsukumogami/koto`) or use a different delimiter (`tsukumogami:koto`) to avoid the TOML slash issue?

3. How should registry auto-discovery work? Should `tsukumogami/koto` in the config automatically imply a GitHub-based registry lookup for the `tsukumogami` org, similar to how Go modules resolve `github.com/org/repo`?

4. Is there a need for an explicit `[registries]` section, or can tsuku infer registries from the org prefix in tool names (convention over configuration)?

5. How do these patterns interact with the existing `tsuku install tsukumogami/koto` auto-registration behavior described in issue #2230?

## Summary

CLI tools handle namespaced packages in config files through three main patterns: quoted string keys with embedded namespace (mise), flat keys with a separate registry/source attribute (Cargo, Poetry), or string values in non-key positions (Homebrew, Docker, npm). The most directly relevant precedent is mise's TOML config, which uses `"github:owner/repo"` as quoted keys and proves this approach works at scale. For tsuku, the strongest path forward combines mise's quoted-key pattern with Homebrew's self-contained registry declaration, since CI-friendliness requires no external state.
