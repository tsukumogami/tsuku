# Lead: What versioning model fits tsuku's registry format?

## Findings

### Current State: Two Schemas, Two Versioning Approaches

The codebase already has two distinct schema-versioned formats, using different versioning models:

1. **Recipe manifest (`recipes.json`)**: Uses semver string `"1.2.0"` (`scripts/generate-registry.py:23`, `internal/registry/manifest.go:28`). The `Manifest` struct declares `SchemaVersion string`, but the CLI **never reads or validates it**. The `parseManifest` function (`manifest.go:158-164`) deserializes into the struct and returns -- no version check. Only the test suite asserts the value matches `"1.2.0"` (`internal/registry/manifest_test.go:44`).

2. **Discovery registry**: Uses a single integer `schema_version: 1` (`internal/discover/registry.go:30`). This format **actively validates** at parse time (`registry.go:52-53`): if `SchemaVersion != 1`, parsing fails with a clear error. This is the only format in the codebase that enforces schema compatibility at runtime.

### Recipe TOML: No Schema Version at All

Individual recipe TOML files have no schema version field. The `Recipe` struct (`internal/recipe/types.go:14-21`) contains `Metadata`, `Version`, `Resources`, `Patches`, `Steps`, and `Verify` -- no version marker. Recipes are parsed by `toml.Unmarshal` into the struct directly (`loader.go:310`), which silently drops unknown fields. There's no mechanism for a recipe to declare "I require CLI version X" or "I use schema version Y."

### Breaking vs. Additive Changes

Given the current parsing behavior:

**Additive (safe without version checks):**
- New optional fields on `MetadataSection`, `VersionSection`, `VerifySection` -- TOML and JSON both silently drop unknown keys during deserialization
- New optional fields on `ManifestRecipe` -- same JSON silent-drop behavior
- New `WhenClause` dimensions (already demonstrated: `libc`, `gpu` were added after `platform`/`os`)
- New action types -- the action registry handles unknown actions at execution time, not parse time

**Breaking (would require version negotiation):**
- Removing or renaming existing fields (`dependencies` -> `build_deps`, for example) -- old CLIs would silently lose data
- Changing a field's type (string to array, array to map) -- TOML unmarshal would fail
- Making an optional field required -- old recipes without it would fail validation (`validate()` in `loader.go:664-687`)
- Changing step parameter semantics (e.g., `outputs` replacing `binaries` in `install_binaries`) -- already happened, handled via fallback logic at `types.go:870-873`
- Restructuring `[[steps]]` from flat params to nested sections -- `UnmarshalTOML` depends on the flat map structure (`types.go:395-529`)

**Subtle breakage (silent data loss):**
- Adding a new required section (e.g., `[security]`) that old CLIs don't know about -- TOML silently drops the entire section, and the CLI proceeds without it
- Moving dependency declarations from metadata to steps -- old CLIs would see empty dependency lists

### Where the Version Should Live

**Manifest-level version (current `schema_version` in `recipes.json`):**
- Covers the JSON envelope format (field names, nesting structure)
- Already exists, just unenforced
- Controls: which fields `ManifestRecipe` contains, how the recipes array is structured
- Cannot express per-recipe schema requirements

**Per-recipe version:**
- Would live in `[metadata]` as something like `schema_version = 2` or `min_cli_version = "0.5.0"`
- Needed if recipes evolve independently (some recipes use new features while others don't)
- The embedded registry freezes recipes at build time -- a per-recipe version would let the CLI know when an embedded recipe is too new for its parser

**Both is the correct answer.** The manifest needs its own version because its structure (JSON envelope) can evolve independently of recipe content. Recipes need their own version because recipe format changes (new action params, new sections) are orthogonal to manifest structure. The discovery registry already demonstrates this separation -- it has its own `schema_version` independent of the manifest.

### Model Comparison

**Semver (current manifest approach: `"1.2.0"`):**
- Pros: familiar, can express "additive change" (minor bump) vs "breaking" (major bump)
- Cons: minor/patch distinction is meaningless for a schema -- you either can parse it or you can't. What does a patch-level schema change mean?
- Current usage: the `"1.2.0"` value has never been bumped in a meaningful way; it was set at creation time

**Single integer (current discovery registry approach: `1`):**
- Pros: simple, unambiguous. Each bump is a new format the CLI must understand. The discovery registry's `if reg.SchemaVersion != 1` check is dead simple
- Cons: no way to express "this is additive, old CLIs are fine" -- every bump forces old CLIs to reject the format
- Works well when changes are rare and always breaking

**Min-version / capability-based:**
- Instead of schema versions, declare what the recipe needs: `min_cli_version = "0.5.0"` or `requires = ["when.gpu", "patches"]`
- Pros: recipes can declare exactly what they need; CLI can give actionable "upgrade to 0.5.0 for GPU support" messages
- Cons: more complex to implement; needs a capability registry; `min_cli_version` couples recipes to release cadence

**Recommended: Integer schema version with range acceptance.**
- Manifest: `schema_version: 2` (integer, not semver). CLI accepts `[1, MAX_SUPPORTED]`.
- Recipes: `schema_version = 2` in `[metadata]`. CLI accepts `[1, MAX_SUPPORTED]`. Recipes without it default to version 1.
- This gives a clean upgrade path: bump the integer when a breaking change happens. Additive changes don't need a bump because TOML/JSON silently drops unknown fields. Old CLIs reject new formats with a clear "please upgrade" message.

The discovery registry already uses this exact model successfully.

## Implications

1. **The manifest version is dead code today.** The `SchemaVersion` field is parsed into the struct and never examined. Any manifest format change deployed today would either silently succeed (additive) or hard-crash on JSON parse error (breaking), with no useful diagnostic.

2. **Recipe format changes are already happening without versioning.** The `outputs` vs `binaries` migration in `install_binaries` (`types.go:870-873`) was handled with fallback code in the struct. The `gpu` and `libc` when-clause fields were added without a version bump. This works because TOML drops unknown fields, but it means there's no way to warn a user that their CLI is too old for a given recipe.

3. **Three recipe sources need different strategies.** Embedded recipes are frozen at build time (always compatible). Local recipes are user-controlled (may be ahead of or behind the CLI). Remote registry recipes can change at any time. A per-recipe `schema_version` lets the CLI give appropriate error messages for each: "this local recipe requires a newer CLI" vs "the registry has been updated, run `tsuku self-update`."

4. **Semver is overkill for schema versioning.** The manifest's `"1.2.0"` carries no actionable information beyond what a single integer would. The minor/patch distinction doesn't map to any real compatibility semantics. The discovery registry's integer approach is simpler and already battle-tested in this codebase.

5. **Backward compatibility is the default, forward compatibility is the gap.** TOML's silent field-dropping means old CLIs can always read new recipes with additive changes. The missing piece is the other direction: new recipes that *require* new CLI features. That's what a schema version check solves.

## Surprises

1. **The discovery registry already does this right.** `internal/discover/registry.go:52-53` validates `SchemaVersion != 1` and fails with a clear error. This pattern exists in the codebase but wasn't applied to the manifest or recipes.

2. **The manifest semver has never been bumped meaningfully.** `SCHEMA_VERSION = "1.2.0"` in the generation script suggests it was set once and forgotten. There's no changelog or documentation of what 1.0 vs 1.1 vs 1.2 changed.

3. **Step parameter evolution is already happening through soft deprecation.** The `outputs` parameter coexists with `binaries` via fallback logic, not version gating. This is a pragmatic choice but creates long-lived compatibility code that can't be cleaned up without a breaking version bump.

4. **The `validate()` function in `loader.go:664-687` is the de facto schema enforcer.** It checks required fields (`metadata.name`, at least one step, `verify.command` for non-libraries). Any change to these requirements is a breaking change, but there's no version gate to control when the new requirements take effect.

## Open Questions

1. **What triggers a schema version bump?** If additive changes are safe (TOML drops unknown fields), and the existing validation catches missing required fields, when exactly would a recipe schema version need to increment? The answer is likely: when the *semantics* of existing fields change, or when a new required field is added. But this needs a clear written policy.

2. **How should the CLI communicate "upgrade needed" for distributed registries?** For the central registry, the CLI can point users to `tsuku self-update`. For distributed registries (future), the CLI doesn't control the recipe source. Should the error message be generic ("this recipe requires schema version 3, your CLI supports up to 2") or should the recipe declare a minimum CLI version?

3. **Should embedded recipes carry a schema version?** Embedded recipes are compiled into the binary, so by definition they're compatible with that binary's parser. Adding a schema version to embedded recipes only matters if the embedded recipe is extracted and shared (e.g., cached to disk and read by a different CLI version).

4. **What about the `generated_at` field?** The manifest already has a timestamp. Should freshness be part of the compatibility story? (e.g., "this manifest is from 2024, your CLI was built in 2025 -- the schema may have changed")

5. **How does `reorder` schema versioning relate?** The batch/reorder subsystem uses its own `schema_version: 1` integer (`internal/batch/` and `internal/reorder/`). Should there be a unified versioning strategy across all JSON formats, or is per-format independence the right call?

## Summary

The codebase has two schema versioning approaches: the manifest uses an unenforced semver string (`"1.2.0"`) that the CLI never validates, while the discovery registry uses a simple integer (`1`) with hard validation at parse time -- and the discovery registry's approach is strictly better. Recipes have no schema version at all, relying on TOML's silent field-dropping for forward compatibility, which works for additive changes but provides no signal when a recipe requires CLI features that don't exist yet. The recommended model is integer schema versions on both the manifest and per-recipe (in `[metadata]`), with range-based acceptance (`[1, MAX_SUPPORTED]`), following the pattern the discovery registry already uses successfully.
