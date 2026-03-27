<!-- decision:start id="toml-schema-version-constraints" status="assumed" -->
### Decision: TOML Schema and Version Constraint Syntax

**Context**

Tsuku needs a per-directory project configuration file (`.tsuku.toml`) that declares which tools a project requires and optionally constrains their versions. This is Block 4 of the shell integration building blocks design. The chosen schema must produce a stable `ProjectConfig` Go interface consumed by two downstream building blocks: shell environment activation (#1681) and the project-aware exec wrapper (#2168).

The project already uses TOML throughout -- recipe definitions, user config at `$TSUKU_HOME/config.toml` -- and version providers already support exact matching and prefix/fuzzy resolution (e.g., "1.22" resolves to "1.22.5"). The schema decision therefore focuses on TOML structure and what constraint formats to support, not on parsing infrastructure.

Four tools in the same space (mise, asdf, devbox, volta) were examined. None use semver range expressions. Most support exact versions and some form of prefix or "latest" keyword.

**Assumptions**

- Semver range constraints (">= 1.0, < 2.0") are not needed initially. Prefix matching ("1.22") covers the vast majority of real-world pinning needs. If this assumption is wrong, a constraint parser can be added later behind the same `ToolRequirement.Version` field without breaking the schema.
- The `[tools]` section is the only required section for v1. Additional sections (env, settings, scripts) can be added later as separate TOML tables without migrating existing files.
- Per-tool extended options (registry source overrides, post-install hooks) will be needed eventually. This assumption drives the choice toward an extensible schema rather than a flat string map.

**Chosen: Mixed Map with String Shorthand**

The `[tools]` section maps recipe names to either a version string (shorthand) or an inline table with a `version` field (extended form):

```toml
[tools]
node = "20.16.0"                          # exact version
go = "1.22"                               # prefix (resolves to latest 1.22.x)
ripgrep = "latest"                        # latest stable version
jq = ""                                   # empty = latest
python = { version = "3.12" }             # inline table (extensible)
```

Go types:

```go
type ProjectConfig struct {
    Tools map[string]ToolRequirement `toml:"tools"`
}

type ToolRequirement struct {
    Version string `toml:"version"`
}
```

`ToolRequirement` implements a custom `UnmarshalTOML` method (~20 lines) that accepts either a string (converted to `ToolRequirement{Version: s}`) or a table (decoded normally). This is a well-established pattern in Go TOML libraries and in TOML-based tools like Cargo.

Version constraint formats supported:
- **Exact**: `"20.16.0"` -- passed directly to `ResolveVersion`
- **Prefix**: `"1.22"` -- handled by existing provider fuzzy matching
- **Latest**: `"latest"` or `""` -- triggers `ResolveLatest`
- **No semver ranges** in v1 -- not implemented by providers, not used by comparable tools

**Rationale**

The mixed map approach satisfies all five decision drivers:

1. *Simplicity*: The common case (pin to a version) is `tool = "version"` -- one line, no braces.
2. *Compatibility*: Uses TOML, kebab-case recipe names, BurntSushi/toml library already in the codebase.
3. *Interface stability*: The Go types match the parent design's `ProjectConfig` and `ToolRequirement` exactly. Downstream consumers get a stable API that won't change when per-tool options are added.
4. *Performance*: A small TOML file parses in microseconds -- well within 50ms.
5. *Ecosystem awareness*: Follows mise's proven pattern (string or table) while staying simpler (no "lts:", "ref:", "sub:" prefixes).

The key advantage over a flat string map (Alternative 1) is forward compatibility. When per-tool options are needed -- registry source overrides, post-install commands -- they slot into the inline table form without any migration. Users who only need version pinning never see the table syntax.

**Alternatives Considered**

- **Flat String Map** (`map[string]string`): Simplest possible schema. Rejected because it can't be extended with per-tool metadata without a breaking schema change. When the inevitable need for per-tool options arises, every existing `.tsuku.toml` would need migration.
- **All Inline Tables** (`tool = { version = "..." }`): Uniform structure, no custom unmarshaling. Rejected because the common case becomes verbose -- `{ version = "20.16.0" }` vs `"20.16.0"` -- violating the simplicity driver.
- **TOML Sub-Tables** (`[tools.node]\nversion = "20.16.0"`): Most structured, best for many per-tool options. Rejected because it takes 3 lines per tool minimum, making the tool list hard to scan. The inline table form handles the rare case where extended options are needed without the verbosity penalty for every tool.
- **Semver Range Syntax** (as a constraint format): Considered adding ">= 1.0, < 2.0" style constraints. Rejected because no comparable tool uses ranges, the version provider infrastructure doesn't support range filtering, and prefix matching ("1.22") covers the practical use case. Can be added later behind the same `Version` field if needed.

**Consequences**

- A custom `UnmarshalTOML` method is required on `ToolRequirement`. This is ~20 lines of straightforward code with clear test cases.
- The `"latest"` keyword needs handling in the version resolution path: if `Version == "latest" || Version == ""`, call `ResolveLatest` instead of `ResolveVersion`.
- Adding new per-tool fields later (e.g., `source`, `postinstall`) requires only adding fields to `ToolRequirement` -- no schema version bump, no migration.
- Users familiar with mise or Cargo.toml dependencies will find the mixed format immediately recognizable.
<!-- decision:end -->
