# Lead: How do embedded recipes interact with schema evolution?

## Findings

### Embedded Recipe Implementation

Recipes are embedded via a `//go:embed` directive in `internal/recipe/embedded.go:13`:

```go
//go:embed recipes/*.toml
var embeddedRecipes embed.FS
```

The embedded recipes live in `internal/recipe/recipes/` (19 TOML files as of now). They are stored as raw bytes in a `map[string][]byte` by `EmbeddedRegistry` and parsed on demand via `toml.Unmarshal` into the same `Recipe` struct used for all recipe sources.

There is no metadata embedded alongside the recipes -- no schema version, no build timestamp, no "minimum CLI version" marker. The binary carries only the raw TOML files.

### Build Process: When Are Embedded Recipes Frozen?

Embedded recipes are frozen at `go build` time. Go's `//go:embed` directive reads files from the source tree during compilation. There is no Makefile step, no code generation, and no preprocessing. Whatever TOML files exist in `internal/recipe/recipes/` when `go build` runs become part of the binary. This means:

- A release tag freezes exactly those 19 recipes at that commit
- There is no mechanism to update embedded recipes without rebuilding
- The binary has no awareness of what schema version its embedded recipes conform to

### Resolution Chain Interaction

The loader (`internal/recipe/loader.go`) implements a 4-tier resolution chain:

1. In-memory cache
2. Local recipes (`$TSUKU_HOME/recipes/`)
3. Embedded recipes (frozen in binary)
4. Registry (disk cache or remote fetch)

All tiers feed into the same `parseBytes()` method (line 308), which does:
1. `toml.Unmarshal` into `Recipe` struct
2. `validate()` -- checks name, steps, verify command
3. Optional `computeStepAnalysis()`

There is **no schema version check** at any tier. The parser relies entirely on Go's TOML unmarshaling behavior:
- Unknown fields in TOML are silently ignored by `github.com/BurntSushi/toml`
- Missing fields get Go zero values (empty string, nil, false, 0)

### The Manifest Has a Schema Version, But Nobody Checks It

The registry manifest (`recipes.json`) has a `schema_version` field (currently `"1.2.0"`, set in `scripts/generate-registry.py:23`). The `Manifest` struct in `internal/registry/manifest.go:28` stores it:

```go
type Manifest struct {
    SchemaVersion string           `json:"schema_version"`
    ...
}
```

But **no code path reads or validates this field**. It is parsed and discarded. The CLI will happily consume a manifest with `schema_version: "99.0.0"` as long as the JSON structure unmarshals into the `Manifest` struct.

### Individual Recipes Have No Schema Version

Recipe TOML files contain no `schema_version` or `format_version` field. The `Recipe` struct in `internal/recipe/types.go` has no such field. There is no way for a recipe to declare "I require CLI version X or later to be parsed correctly."

### Version Mismatch Scenarios

**Scenario 1: Binary built with schema v1, registry serves schema v2 recipes**

If schema v2 adds a new required field (say `metadata.license`), the binary's `validate()` function won't check for it, so registry v2 recipes will parse fine -- they just have an extra field the CLI ignores. If v2 removes a field the CLI expects (say making `verify.command` optional for all types), the CLI would still require it and reject valid v2 recipes. If v2 changes the meaning of an existing field, the CLI would misinterpret it silently.

**Scenario 2: Binary supports schema v2, distributed registry still serves v1**

Since Go zero-values missing fields, old recipes parse without error. The risk is that the CLI assumes a non-nil value for a new field that v1 recipes don't provide. If the CLI adds new required validation (e.g., "all recipes must have `supported_os`"), it would reject valid v1 recipes that predate the requirement.

**Scenario 3: Embedded recipe has fewer fields than registry version**

This already happens in practice. Embedded recipes are a small subset (19 recipes) and can have different content than the registry version of the same recipe. The resolution chain means embedded wins over registry -- a user with an outdated binary gets the embedded version even if a better registry version exists. The `warnIfShadows()` mechanism only warns when *local* recipes shadow embedded/registry, not the other way around.

### The `RequireEmbedded` Flag

The `LoaderOptions.RequireEmbedded` flag restricts resolution to embedded-only. This is used during dependency validation to ensure action dependencies (Go, Rust, etc.) can bootstrap without network access. If a schema change makes an embedded recipe unparseable, this path would fail hard with no fallback.

## Implications

1. **The current system is accidentally forward-compatible** because TOML's unmarshaling silently ignores unknown fields and Go zero-values missing ones. This works until a schema change modifies semantics of existing fields or adds required validation.

2. **The manifest's `schema_version` field is theater** -- it exists for documentation but provides no runtime protection. A CLI from 6 months ago will consume today's manifest without complaint, even if the format has changed incompatibly.

3. **Embedded recipes are the highest risk surface** because they can't be updated. A schema change that makes the 19 embedded recipes invalid would require every user to upgrade their binary. Since embedded recipes include bootstrap dependencies (Go, Rust, OpenSSL, etc.), a parsing failure here would brick the tool.

4. **The resolution chain's priority order (local > embedded > registry) means embedded recipes can mask registry improvements**, but this is by design -- offline reliability is more important than freshness for bootstrap tools.

5. **There is no deprecation signaling mechanism**. If a field is renamed or removed, old binaries will silently use zero values. There is no way for the registry to tell an old CLI "you need to upgrade to understand this recipe."

## Surprises

1. **The embed directive pattern changed.** The design doc at `docs/designs/current/DESIGN-recipe-registry-separation.md:87` shows `//go:embed recipes/*/*.toml` (with letter subdirectories), but the actual code at `internal/recipe/embedded.go:13` uses `//go:embed recipes/*.toml` (flat directory). The separation design was implemented with flat embedded recipes, not the nested structure.

2. **Only 19 recipes are embedded.** The design doc mentions 171 recipes were embedded before the separation. The vast majority moved to the registry, meaning schema evolution primarily affects registry recipes. But the 19 embedded ones are the most critical (bootstrap dependencies).

3. **No test validates schema compatibility.** There is no test that parses embedded recipes with a "minimum expected fields" check. The `ListWithInfo()` method in `embedded.go:84-102` silently skips recipes that fail to parse (`continue` on error), meaning a schema-incompatible embedded recipe would vanish from the list without any error.

4. **The `satisfiesIndex` mixes embedded and registry data** with embedded taking priority. A schema change to the `satisfies` field format could cause the index to build incorrectly, with embedded recipes masking the correct registry entries.

## Open Questions

1. **Should individual TOML recipes carry a `schema_version` field?** This would let the CLI reject recipes it can't fully understand rather than silently dropping fields. The cost is complexity in every recipe file and a migration burden.

2. **Should the CLI binary embed a "supported schema range"?** Something like `const MinSchemaVersion = "1.0.0"` and `const MaxSchemaVersion = "1.2.0"` that gets checked against the manifest's `schema_version`. This is simpler than per-recipe versioning but only protects the manifest, not individual TOML files.

3. **What should the CLI do when it encounters an incompatible recipe?** Options: (a) refuse and suggest upgrading, (b) attempt best-effort parsing with warnings, (c) fall back to an embedded version if available. The answer depends on whether silent degradation or loud failure is preferred.

4. **How should the `ListWithInfo()` silent-skip behavior change?** Currently a schema-incompatible embedded recipe just disappears. Should it surface an error instead?

5. **Is the embedded-over-registry priority correct for schema mismatches?** If a registry recipe has been updated for schema v2 but the embedded version is v1, should the CLI prefer the (parseable but outdated) embedded version or attempt the (potentially incompatible) registry version?

## Summary

The CLI has no schema version checking anywhere -- the manifest's `schema_version` field is parsed but never validated, individual recipes carry no version metadata, and the TOML parser silently ignores unknown fields while zero-valuing missing ones. This means the system is accidentally forward-compatible for additive changes but has no protection against semantic changes, field renames, or new required validation. The biggest open question is whether to add version checking at the manifest level (cheap, protects the index), the individual recipe level (thorough, high migration cost), or both -- and what the failure mode should be when an old binary encounters a new-format recipe.
