# Phase 3 Research: Version Transition Mechanics

## Questions Investigated

1. What happens when Go's `json.Unmarshal` encounters an integer where it expects a string?
2. What happens when Go's `json.Unmarshal` encounters a string where it expects an integer?
3. What's in existing users' cached `manifest.json` files?
4. Can we use `json.Number` or `interface{}` as an intermediate type to handle both during transition?
5. Should the transition be immediate or staged?
6. What does the generation script need to change?
7. Are there any tests that assert on the string value "1.2.0"?

## Findings

### 1. Go `json.Unmarshal` type mismatch behavior

When Go's `json.Unmarshal` encounters a JSON integer (e.g., `1`) but the target struct field is `string`, **it returns an error**. The error message is:

```
json: cannot unmarshal number into Go struct field Manifest.schema_version of type string
```

Go's JSON decoder does NOT coerce types. It requires an exact type match for primitive types. The reverse is also true: a JSON string `"1"` into an `int` field produces:

```
json: cannot unmarshal string into Go struct field DiscoveryRegistry.schema_version of type int
```

This means a naive type change from `string` to `int` on the `Manifest.SchemaVersion` field will hard-fail when parsing any cached `manifest.json` that still has `"1.2.0"` as a string.

### 2. String-to-int direction (same answer)

`json.Unmarshal` of `"1.2.0"` (string) into an `int` field fails. Even `"1"` (a valid integer in string form) fails. Go does not attempt string-to-int conversion during JSON unmarshaling.

### 3. Existing cached manifest.json content

Users' cached manifest files (at `$TSUKU_HOME/registry/manifest.json`) contain `"schema_version": "1.2.0"` as a JSON string value. This is written by:

- `scripts/generate-registry.py` (line 23: `SCHEMA_VERSION = "1.2.0"`)
- Served from `https://tsuku.dev/recipes.json`
- Cached locally by `Registry.FetchManifest()` which writes raw bytes to disk (`manifest.go:140-155`)

If the struct changes to `int`, any user with a cached manifest will get a parse error on their next operation that reads the cache (`GetCachedManifest`). The error path in `GetCachedManifest` (`manifest.go:45-60`) returns the error to the caller -- it does not silently ignore it.

### 4. Intermediate types for transition

Two viable options exist:

**Option A: `json.Number`**

```go
SchemaVersion json.Number `json:"schema_version"`
```

`json.Number` is a string type that accepts both JSON strings and JSON numbers. It stores the raw token. You can then call `.Int64()` or `.String()` on it. However, `json.Number` only works when the decoder has `UseNumber()` enabled, which `json.Unmarshal` does NOT enable by default. With default `json.Unmarshal`, a `json.Number` field receives a string from a JSON string and the raw number text from a JSON number -- but only if the decoder is configured with `UseNumber()`. Without it, numbers become `float64` internally.

Actually, `json.Number` as a struct field type does work with `json.Unmarshal` without `UseNumber()`. The decoder recognizes `json.Number` as a target type and stores the raw token. But it only accepts JSON numbers, not JSON strings. A JSON string `"1.2.0"` into a `json.Number` field would fail.

**Option B: `interface{}` with custom unmarshaling**

```go
type Manifest struct {
    SchemaVersionRaw interface{} `json:"schema_version"`
    // ...
}

func (m *Manifest) SchemaVersion() (int, error) {
    switch v := m.SchemaVersionRaw.(type) {
    case float64:
        return int(v), nil
    case string:
        // Legacy: parse "1.2.0" -> treat as version 1
        // or return an error asking user to update
    }
}
```

This works. `interface{}` accepts any JSON type. The downside is exposing a raw field and needing a method to extract the typed value.

**Option C: Custom `UnmarshalJSON` on Manifest**

The cleanest approach. Keep `SchemaVersion int` as the public field, but implement `UnmarshalJSON` on `Manifest`:

```go
func (m *Manifest) UnmarshalJSON(data []byte) error {
    type raw Manifest
    var r struct {
        raw
        SchemaVersion interface{} `json:"schema_version"`
    }
    if err := json.Unmarshal(data, &r); err != nil {
        return err
    }
    *m = Manifest(r.raw)
    switch v := r.SchemaVersion.(type) {
    case float64:
        m.SchemaVersion = int(v)
    case string:
        // Legacy string format -- treat as version 0 (pre-versioning era)
        m.SchemaVersion = 0
    default:
        return fmt.Errorf("schema_version: unexpected type %T", v)
    }
    return nil
}
```

This lets the struct field be `int` while accepting both formats during the transition window.

### 5. Immediate vs. staged transition

**Immediate (change to int, bump to 1)** would break any user with a cached `manifest.json` unless a custom unmarshaler handles the string case. The breakage is recoverable: `tsuku update-registry` would fetch the new integer-versioned manifest and overwrite the cache. But the initial error on cache read could confuse users.

**Staged approach:**

1. **Release N**: Add custom `UnmarshalJSON` that accepts both string and int. Don't change the generation script yet. No user-visible change.
2. **Release N+1**: Change `generate-registry.py` to emit `"schema_version": 1` (integer). The custom unmarshaler handles both old cached strings and new integers.
3. **Release N+2** (optional): Remove string handling from the unmarshaler.

The staged approach is safer but may be unnecessary given the recovery path. If we implement the custom `UnmarshalJSON` and change the generation script simultaneously, the worst case is: a user with an old CLI and new manifest gets a parse error, runs `update-registry`, and gets the new manifest. But wait -- an old CLI would fail to parse the integer `schema_version` because the old struct expects a string. That's not recoverable without updating the CLI.

**Key insight**: The direction of compatibility matters. Old CLI (string field) + new manifest (integer value) = parse error with no recovery path except upgrading the CLI. New CLI (int field + custom unmarshaler) + old cached manifest (string value) = works fine via the unmarshaler. The generation script change MUST lag behind the CLI change by at least one release cycle.

### 6. Generation script changes needed

In `scripts/generate-registry.py`:

- Line 23: Change `SCHEMA_VERSION = "1.2.0"` to `SCHEMA_VERSION = 1`
- Line 281-282 in `generate_json()`: The `"schema_version": SCHEMA_VERSION` assignment works for both string and int since Python's `json.dump` handles both types correctly.

That's the only change needed in the script. The output changes from `"schema_version": "1.2.0"` to `"schema_version": 1` (no quotes around the value in JSON).

### 7. Tests asserting on string value "1.2.0"

Multiple test files assert on the string `"1.2.0"`:

**`internal/registry/manifest_test.go`** (6 occurrences):
- Line 16: JSON literal `"schema_version": "1.2.0"` in `TestParseManifest_WithSatisfies`
- Line 44-45: `manifest.SchemaVersion != "1.2.0"` assertion
- Line 110: `SchemaVersion: "1.2.0"` struct literal in `TestGetCachedManifest_ValidCachedFile`
- Line 167: JSON literal in `TestFetchManifest_RemoteSuccess`
- Lines 201-202, 219-220: Assertions on `.SchemaVersion` after fetch and cache read
- Line 280: JSON literal in `TestFetchManifest_LocalRegistry`
- Line 360: JSON literal in `TestCacheManifest_WritesFile`

**`internal/recipe/satisfies_test.go`** (3 occurrences):
- Lines 581, 641, 720: JSON literals in manifest test fixtures

All of these will need updating when the type changes. The struct literals change from `SchemaVersion: "1.2.0"` to `SchemaVersion: 1`, the JSON literals change from `"schema_version": "1.2.0"` to `"schema_version": 1`, and the assertions change from string comparison to int comparison.

Additionally, tests should be added for the backward-compatible unmarshaling of old string-format manifests.

## Implications for Design

1. **The custom `UnmarshalJSON` approach (Option C) is the right path.** It keeps the public API clean (`SchemaVersion int`) while handling the transition transparently. The discovery registry at `internal/discover/registry.go` already uses `int` with validation (lines 30, 52-53), so this aligns the codebase.

2. **The generation script must change AFTER the CLI ships with the custom unmarshaler.** If the script emits an integer before users have the new CLI, old CLIs will fail to parse the manifest with no recovery path. The recommended sequence:
   - Release N: Ship CLI with `int` field + custom `UnmarshalJSON` that accepts both. Script still emits `"1.2.0"`.
   - Release N+1: Change script to emit `1`. All CLIs in the wild (N or later) handle it.

3. **String-format versions should map to schema version 0.** The old `"1.2.0"` string was never validated, so it carries no semantic meaning. Treating it as "version 0" (pre-versioning) is accurate and lets the `[MinVersion, MaxVersion]` range check work correctly with `MinVersion = 0` or `MinVersion = 1`.

4. **Test updates are mechanical but widespread.** Nine test locations across two files need updating. New tests should cover: (a) integer `schema_version` parses correctly, (b) string `schema_version` parses as version 0, (c) missing `schema_version` behaves sensibly (zero value = 0, which matches "pre-versioning").

5. **The `parseManifest` function is the single choke point.** All manifest parsing flows through `parseManifest()` at `manifest.go:158-164`. The custom `UnmarshalJSON` on the `Manifest` struct will be called automatically by `json.Unmarshal` within this function. No caller changes needed.

## Surprises

1. **Go's `json.Unmarshal` is strict about type matching.** Unlike JavaScript or Python, it will not coerce `1` to `"1"` or vice versa. This makes the transition inherently breaking without a custom unmarshaler.

2. **`json.Number` is not a silver bullet.** It only accepts JSON numbers, not JSON strings, when used as a struct field type with default `json.Unmarshal`. It would handle the new integer format but fail on the old string format -- the opposite of what we need for backward compatibility.

3. **The cached manifest is raw bytes, not re-serialized.** `CacheManifest()` writes the original fetched bytes (`manifest.go:150`), not a re-marshaled struct. This means the cache format matches whatever the server sent. During transition, a user could have either format cached depending on when they last ran `update-registry`.

4. **No version validation exists anywhere in the manifest code path.** The discovery registry validates `schema_version == 1` immediately after parsing (`registry.go:52-53`), but the manifest's `parseManifest()` does nothing. The version transition is an opportunity to add validation in the same function, matching the pattern already established by the discovery registry.

## Summary

Go's `json.Unmarshal` does not coerce between string and integer types, so changing `Manifest.SchemaVersion` from `string` to `int` will break parsing of any cached manifest containing `"1.2.0"`. The fix is a custom `UnmarshalJSON` on `Manifest` that accepts both types, mapping legacy strings to version 0. The generation script change must ship one release after the CLI change to avoid breaking old CLIs that still expect a string. Nine test locations across two files need mechanical updates.
