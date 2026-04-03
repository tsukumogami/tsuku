# Lead: What TOML-valid syntax options exist for representing org/tool pairs?

## Findings

### TOML Spec Key Rules

The TOML spec (v1.0.0 and v1.1.0) defines three key types:

1. **Bare keys**: May only contain `A-Za-z0-9_-`. The `/` character is not valid. This is why `tsukumogami/koto = "latest"` is a parse error.

2. **Quoted keys**: Follow basic string (`"..."`) or literal string (`'...'`) rules. Any Unicode character valid in strings is allowed. So `"tsukumogami/koto" = "latest"` is perfectly valid TOML.

3. **Dotted keys**: Sequences of bare or quoted keys joined with `.`. Each dot creates a nested table. So `tools.tsukumogami.koto` creates `tools -> tsukumogami -> koto`.

### Option 1: Quoted Keys

```toml
[tools]
node = "20"
"tsukumogami/koto" = "latest"
```

**Spec compliance**: Fully valid. Quoted keys can contain any character valid in TOML strings, including `/`.

**Readability**: Good. The org/tool pair reads naturally. The quotes are a minor visual speed bump but most developers understand quoted keys.

**Backward compatibility**: Full. Existing bare keys like `node = "20"` continue working unchanged. The `Tools map[string]ToolRequirement` struct in `config.go` already handles this -- the map key would just be the string `"tsukumogami/koto"`.

**Parsing complexity**: Minimal. BurntSushi/toml already handles quoted keys. The map key arrives as `tsukumogami/koto`. The resolver (`resolver.go` line 36) does `r.config.Config.Tools[m.Recipe]`, so the `Recipe` field from the binary index match would need to contain the full `org/tool` name, or the resolver needs to check both `tool` and `org/tool` forms.

**Trade-offs**: The key contains the org prefix, so the system must parse it out (split on `/`) to determine registry vs tool name. But this is a one-liner.

### Option 2: Dotted Keys (Sub-tables)

```toml
[tools]
node = "20"

[tools.tsukumogami]
koto = "latest"
```

Or equivalently with inline dotted keys:

```toml
[tools]
node = "20"
tsukumogami.koto = "latest"
```

**Spec compliance**: Fully valid. `tsukumogami` is a valid bare key, `koto` is a valid bare key, and dotted keys create nested tables.

**Readability**: The sub-table form (`[tools.tsukumogami]`) groups org tools nicely but separates them visually from default-registry tools. The inline dotted form (`tsukumogami.koto = "latest"`) is compact but may confuse users who read the dot as a version separator or config nesting.

**Backward compatibility**: Breaking. The current struct is `Tools map[string]ToolRequirement`. With dotted keys, the TOML parser produces `Tools map[string]interface{}` where `tsukumogami` maps to another nested map `{"koto": "latest"}`. The current `UnmarshalTOML` on `ToolRequirement` would receive a `map[string]any` and try to find a `version` key, which would fail since it finds `koto` instead. This requires a significant parser rewrite.

**Parsing complexity**: High. The `Tools` field must change from `map[string]ToolRequirement` to a custom type that can differentiate between:
- `node = "20"` (bare key -> string value = default registry tool)
- `tsukumogami.koto = "latest"` (dotted key -> nested map = org-scoped tool)
- `python = { version = "3.12" }` (bare key -> table value = default registry tool with options)

The disambiguation logic is non-trivial because a nested map could be either an org scope or a tool config table.

**Trade-offs**: Elegant grouping syntax, but the ambiguity between "is this a nested org or a tool config table?" creates real parsing problems. Issue #2230 already flagged this as "misinterpreted as nested config."

### Option 3: Inline Tables with Source Field

```toml
[tools]
node = "20"
koto = { source = "tsukumogami/koto", version = "latest" }
```

**Spec compliance**: Fully valid. Inline tables are standard TOML.

**Readability**: Clear and self-documenting. The `source` field makes the org origin explicit. However, it's verbose compared to the simple `tool = "version"` shorthand.

**Backward compatibility**: Full. The existing `UnmarshalTOML` already handles `map[string]any`. Adding a `source` field to `ToolRequirement` is additive. Existing configs with bare string values or `{ version = "..." }` tables continue working.

**Parsing complexity**: Low. Add a `Source string` field to `ToolRequirement`. In `UnmarshalTOML`, check for `source` key in the map alongside `version`. The resolver then uses `Source` (if set) instead of the map key for registry lookup.

**Trade-offs**: The map key (`koto`) becomes a local alias while `source` is the canonical identifier. This means two different projects could alias the same tool differently, which could cause confusion. Also, if two orgs have a tool named `koto`, the user must choose different aliases.

### Option 4: Value-Side Encoding

```toml
[tools]
node = "20"
koto = "tsukumogami/koto@latest"
```

Or with a prefix convention:

```toml
[tools]
node = "20"
koto = "tsukumogami:koto@latest"
```

**Spec compliance**: Fully valid. The value is just a string.

**Readability**: Compact and familiar to users of npm (`@scope/pkg@version`) or Docker (`registry/image:tag`). The `@` separates source from version.

**Backward compatibility**: Requires careful parsing. A value like `"20"` is a version. A value like `"tsukumogami/koto@latest"` needs to be detected and split. The heuristic could be: if the value contains `/`, it's an org-scoped reference. But this means version strings can never contain `/`, which seems safe.

**Parsing complexity**: Medium. The `UnmarshalTOML` for strings needs to check for `/` and `@` patterns and split accordingly. Error messages need to be clear when the format is wrong. This is stringly-typed, which some developers consider fragile.

**Trade-offs**: Same aliasing issue as Option 3 (the key `koto` is a local name). But very compact syntax. The downside is that version strings and source references share the same field, making the format less self-documenting.

### Option 5: Separate Registries Section

```toml
[registries]
tsukumogami = "github.com/tsukumogami/tsuku-registry"

[tools]
node = "20"
koto = { version = "latest", registry = "tsukumogami" }
```

**Spec compliance**: Fully valid.

**Readability**: Very clear separation of concerns. The `[registries]` section declares where to find things, `[tools]` declares what to install.

**Backward compatibility**: Full. New sections don't affect existing `[tools]` parsing. Tools without a `registry` field use the default registry.

**Parsing complexity**: Low-medium. Add a `Registries` map to `ProjectConfig`. Add a `Registry` field to `ToolRequirement`. The resolver checks the registry field and routes to the appropriate source.

**Trade-offs**: Verbose for the common case of "I just want one tool from another org." Requires two sections even for a single org-scoped tool. Good for projects that use many tools from the same org. This is exactly how Cargo handles alternative registries.

### Option 6: Array of Tables

```toml
[[tools]]
name = "node"
version = "20"

[[tools]]
name = "tsukumogami/koto"
version = "latest"
```

**Spec compliance**: Fully valid. `[[tools]]` creates an array of tables.

**Readability**: Very explicit but extremely verbose. Every tool requires 3+ lines.

**Backward compatibility**: Completely breaking. Changes `Tools` from a map to an array. All existing `.tsuku.toml` files would need rewriting.

**Parsing complexity**: Low once the struct changes. But the migration cost is high.

**Trade-offs**: Not worth the verbosity and migration pain for this use case.

### Option 7: Hybrid -- Quoted Keys with Conventions

```toml
[tools]
node = "20"
"tsukumogami/koto" = "latest"
"tsukumogami/koto" = { version = "latest" }
```

This combines Option 1 (quoted keys) with the existing shorthand/table duality. The key contains the full `org/tool` identifier, and the value format remains the same as today.

**Spec compliance**: Fully valid.

**Readability**: Good. The full identifier in the key is unambiguous.

**Backward compatibility**: Full. Existing bare keys work. New quoted keys with `/` are additive.

**Parsing complexity**: Minimal. The only change is in the resolver: when looking up a tool, check if the key contains `/` to determine if it's org-scoped. The key itself serves as both display name and lookup identifier.

**Trade-offs**: This is the simplest option. No new fields, no new sections, no struct changes. The only question is whether the registry system can resolve `tsukumogami/koto` from the key alone.

### Comparison with Other Tools

| Tool | Format | Namespace Syntax | Config Type |
|------|--------|-----------------|-------------|
| npm | JSON | `"@scope/pkg": "^1.0"` | Quoted key with `@` prefix |
| Cargo | TOML | `crate = { version = "1.0", registry = "my-reg" }` | Separate registries section |
| Homebrew | CLI | `brew tap user/repo && brew install tool` | No config file, CLI commands |
| mise | TOML | `asdf:yarn`, `vfox:elixir` | Backend prefix in key |
| devcontainer | JSON | `"ghcr.io/user/repo/feature:1": {}` | Full URI as quoted key |

The npm and devcontainer patterns are closest to Option 1/7 (quoted keys with the full identifier). Cargo is closest to Option 5 (separate registries). mise is closest to Option 4 (value-side encoding via prefix).

## Implications

1. **Option 7 (quoted keys with conventions) is the lowest-friction path.** It requires zero struct changes, zero new config sections, and zero migration. The key `"tsukumogami/koto"` contains all the information needed for resolution. The only work is teaching the resolver to handle keys containing `/`.

2. **Option 3 (inline table with source) is the most extensible path.** If org-scoped tools need additional metadata later (registry URL, authentication, channel), the inline table approach scales naturally. But it's more verbose and introduces the aliasing problem.

3. **Option 5 (separate registries) is the most Cargo-like path.** It's the right choice if tsuku expects many third-party registries. But it's heavyweight for the common case.

4. **Options 2 and 6 should be eliminated.** Dotted keys create parsing ambiguity (nested config vs. org scope), and array-of-tables is too verbose with a breaking migration.

5. **Options 4 (value-side encoding) is clever but stringly-typed.** It works but makes the format less self-documenting and harder to validate.

6. The current `config.go` code needs minimal changes for Option 7. The `Tools map[string]ToolRequirement` map key naturally holds `"tsukumogami/koto"`. The resolver at `resolver.go:36` already does a map lookup by recipe name -- it just needs the recipe name to match the full org-scoped key.

## Surprises

1. **TOML v1.1.0 didn't change the bare key rules.** Bare keys are still restricted to `A-Za-z0-9_-`. There was no push to add `/` or other characters. This means quoted keys are the only way to use `/` in keys, and this won't change.

2. **The dotted key ambiguity is worse than expected.** `tsukumogami.koto = "latest"` and `koto = { version = "latest" }` both produce `map[string]any` at the TOML level, but with completely different semantics. There's no reliable way to distinguish "this is an org scope" from "this is a config table" without additional conventions (like reserved field names).

3. **devcontainer.json uses full URIs as keys** (`"ghcr.io/devcontainers/features/go:1": {}`). This is the most aggressive version of the "put everything in the key" approach. It suggests that developers are comfortable with long quoted keys when the identifier needs to be globally unique.

4. **npm solved this at the package name level**, not the config level. The `@scope/package` name is a valid npm package name that just happens to work as a JSON key. TOML's bare key restrictions make this approach impossible without quoting.

## Open Questions

1. **How does tsuku's registry resolution currently handle the `org/tool` form?** The scope doc says `tsuku install tsukumogami/koto` auto-registers the registry. Does the recipe name stored after installation include the org prefix? If so, Option 7 may "just work" with the resolver.

2. **Should the key in `[tools]` be the recipe name or an alias?** Option 7 uses the key as the recipe name. Options 3/4 use the key as a local alias. This affects whether two projects can refer to the same tool by different names.

3. **What happens with `tsuku init`?** When `tsuku init` generates a `.tsuku.toml`, should it emit quoted keys for org-scoped tools? The UX of typing quoted keys manually is slightly worse than bare keys.

4. **Can Options 1/7 and 3 coexist?** A config could support both `"tsukumogami/koto" = "latest"` (quoted key shorthand) and `koto = { source = "tsukumogami/koto", version = "latest" }` (explicit source). This gives users the choice of concise or explicit. But supporting two ways to do the same thing adds cognitive load.

## Summary

There are seven TOML-valid syntax options for representing org/tool pairs, ranging from quoted keys (`"tsukumogami/koto" = "latest"`) to separate registry sections to value-side encoding -- but only three are practical after filtering for backward compatibility and parsing simplicity. The quoted-key approach (Option 7) is the clear winner for tsuku's case: it requires zero struct changes to `ProjectConfig`, preserves full backward compatibility with existing `.tsuku.toml` files, and follows the same pattern that npm and devcontainer.json use for namespaced identifiers. The main implementation work is teaching the resolver to match recipe names that contain an org prefix.
