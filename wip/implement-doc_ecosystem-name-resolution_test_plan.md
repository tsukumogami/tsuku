# Test Plan: Ecosystem Name Resolution

Generated from: docs/designs/DESIGN-ecosystem-name-resolution.md
Issues covered: 4
Total scenarios: 14

---

## Scenario 1: Recipe with satisfies field parses correctly
**ID**: scenario-1
**Testable after**: #1826
**Category**: infrastructure
**Commands**:
- `go test ./internal/recipe/... -run "Satisfies|satisfies" -v -count=1`
**Expected**: Unit tests pass confirming that a recipe TOML with a `[metadata.satisfies]` section (e.g., `homebrew = ["openssl@3"]`) is deserialized into `MetadataSection.Satisfies` as `map[string][]string`. The `Satisfies` field exists on `MetadataSection` in `internal/recipe/types.go` with tag `toml:"satisfies,omitempty"`.
**Status**: passed

---

## Scenario 2: Recipes without satisfies field remain backward compatible
**ID**: scenario-2
**Testable after**: #1826
**Category**: infrastructure
**Commands**:
- `go test ./internal/recipe/... -count=1`
- `go test ./... -count=1`
**Expected**: The full test suite passes with no regressions. Recipes that omit `[metadata.satisfies]` continue to parse and load. The `Satisfies` field defaults to nil/empty map and does not affect any existing behavior.
**Status**: passed (2 pre-existing failures in builders/discover unrelated to satisfies changes)

---

## Scenario 3: Loader satisfies fallback resolves ecosystem names
**ID**: scenario-3
**Testable after**: #1826
**Category**: infrastructure
**Commands**:
- `go test ./internal/recipe/... -run "Satisfies" -v -count=1`
**Expected**: A unit test demonstrates the full fallback path: when `GetWithContext()` cannot find a recipe by exact name through the 4-tier chain (cache, local, embedded, registry), it calls `lookupSatisfies()` on the lazy-built index and returns the satisfying recipe. The test uses a test-only satisfies entry (not `openssl@3`, since `recipes/o/openssl@3.toml` still exists at this point and would be found by exact match).
**Status**: passed

---

## Scenario 4: Exact name match takes priority over satisfies fallback
**ID**: scenario-4
**Testable after**: #1826
**Category**: infrastructure
**Commands**:
- `go test ./internal/recipe/... -run "Satisfies" -v -count=1`
**Expected**: A unit test verifies that when a recipe exists with an exact name match, the loader returns that recipe directly without consulting the satisfies index. This confirms the fallback is only reached when the 4-tier chain fails.
**Status**: passed

---

## Scenario 5: Validation rejects malformed satisfies entries
**ID**: scenario-5
**Testable after**: #1826
**Category**: infrastructure
**Commands**:
- `go test ./internal/recipe/... -run "Satisfies|Validate" -v -count=1`
**Expected**: `ValidateStructural()` rejects: (a) ecosystem names with uppercase or special characters (only lowercase alphanumeric and hyphens allowed), (b) self-referential entries where a recipe's own canonical name appears in its satisfies list, (c) satisfies entries that collide with another recipe's canonical name. Each case produces a `ValidationError` with a clear message.
**Status**: passed (note: canonical name collisions are a cross-recipe CI check, not per-recipe structural validation)

---

## Scenario 6: Embedded openssl recipe declares satisfies homebrew entry
**ID**: scenario-6
**Testable after**: #1826
**Category**: infrastructure
**Commands**:
- `grep -q '\[metadata\.satisfies\]' internal/recipe/recipes/openssl.toml`
- `grep -q 'homebrew.*=.*\["openssl@3"\]' internal/recipe/recipes/openssl.toml`
- `go build ./...`
**Expected**: `internal/recipe/recipes/openssl.toml` contains a `[metadata.satisfies]` section with `homebrew = ["openssl@3"]`. The recipe compiles into the binary without errors (embedded recipes are `go:embed`).
**Status**: passed

---

## Scenario 7: getEmbeddedOnly also applies satisfies fallback
**ID**: scenario-7
**Testable after**: #1826
**Category**: infrastructure
**Commands**:
- `go test ./internal/recipe/... -run "Satisfies.*Embedded" -v -count=1`
**Expected**: When `RequireEmbedded` is set in `LoaderOptions`, the loader's `getEmbeddedOnly()` path also falls back to the satisfies index but restricts results to embedded-only recipes. A test-only embedded recipe with a satisfies entry is resolvable via this path.
**Status**: passed

---

## Scenario 8: tsuku create detects existing recipe via satisfies index
**ID**: scenario-8
**Testable after**: #1826, #1827
**Category**: infrastructure
**Commands**:
- `go test ./cmd/tsuku/... -run "TestCreate" -v -count=1`
**Expected**: Unit tests confirm that before generating a recipe, `runCreate` calls the recipe loader (with satisfies fallback). When an existing recipe satisfies the requested name, the command prints a message like `Recipe '<canonical>' already satisfies '<requested>'. Use --force to create anyway.` and exits with a non-zero exit code. The check runs before the builder session is created.
**Status**: pending

---

## Scenario 9: tsuku create --force bypasses satisfies check
**ID**: scenario-9
**Testable after**: #1826, #1827
**Category**: infrastructure
**Commands**:
- `go test ./cmd/tsuku/... -run "TestCreate" -v -count=1`
**Expected**: Unit tests confirm that with `--force`, the satisfies duplicate check is skipped entirely and recipe generation proceeds as normal.
**Status**: pending

---

## Scenario 10: Duplicate openssl@3.toml deleted and apr-util fixed
**ID**: scenario-10
**Testable after**: #1826, #1828
**Category**: infrastructure
**Commands**:
- `test ! -f recipes/o/openssl@3.toml`
- `grep -v 'openssl@3' recipes/a/apr-util.toml | grep -q '"openssl"' || grep -q '"openssl"' recipes/a/apr-util.toml`
- `go test ./... -count=1`
**Expected**: `recipes/o/openssl@3.toml` no longer exists. `recipes/a/apr-util.toml` references `"openssl"` (not `"openssl@3"`) in its `runtime_dependencies`. All tests pass, confirming the loader resolves `openssl` correctly through all code paths.
**Status**: pending

---

## Scenario 11: dep-mapping.json entries migrated to satisfies fields
**ID**: scenario-11
**Testable after**: #1826, #1828
**Category**: infrastructure
**Commands**:
- `grep -q 'homebrew.*=.*\["gcc"\]' internal/recipe/recipes/gcc-libs.toml`
- `grep -q 'homebrew.*=.*\["python@3"\]' internal/recipe/recipes/python-standalone.toml`
- `grep -q 'homebrew.*=.*\["sqlite3"\]' recipes/s/sqlite.toml`
- `grep -q 'homebrew.*=.*\["curl"\]' recipes/l/libcurl.toml`
- `grep -q 'homebrew.*=.*\["nghttp2"\]' recipes/l/libnghttp2.toml`
- `grep -q '_deprecated' data/dep-mapping.json`
**Expected**: Each non-trivial mapping from `dep-mapping.json` (gcc -> gcc-libs, python@3 -> python-standalone, sqlite3 -> sqlite, curl -> libcurl, nghttp2 -> libnghttp2) is now declared as a `[metadata.satisfies]` entry on the corresponding recipe. `data/dep-mapping.json` contains a `_deprecated` field pointing users to the per-recipe `satisfies` field. Identity mappings (cmake -> cmake, etc.) don't need satisfies entries.
**Status**: pending

---

## Scenario 12: Registry manifest includes satisfies data
**ID**: scenario-12
**Testable after**: #1826, #1829
**Category**: infrastructure
**Commands**:
- `python3 scripts/generate-registry.py`
- Check `_site/recipes.json` for `satisfies` fields on recipes that declare them
- Check `_site/recipes.json` schema_version is `1.2.0`
**Expected**: The generated `recipes.json` manifest includes a `satisfies` object on recipes that have `[metadata.satisfies]` in their TOML. Recipes without satisfies omit the field (no empty objects). Schema version is bumped from `1.1.0` to `1.2.0`. Cross-recipe duplicate detection causes the script to exit with error if two recipes claim the same package name.
**Status**: pending

---

## Scenario 13: End-to-end ecosystem name resolution for openssl@3
**ID**: scenario-13
**Testable after**: #1826, #1828
**Category**: use-case
**Environment**: manual (requires built tsuku binary)
**Commands**:
- `make build-test`
- `cp tsuku-test "$QA_HOME/bin/tsuku"`
- `run_isolated "$QA_HOME" tsuku info openssl@3`
**Expected**: After `openssl@3.toml` is deleted (#1828) and the satisfies fallback is in place (#1826), running `tsuku info openssl@3` resolves to the embedded `openssl` recipe and displays its information (name: openssl, description containing "SSL/TLS"). The user sees the canonical recipe rather than an error about a missing recipe. This validates the full user-facing behavior: a Homebrew formula name resolves to the correct tsuku recipe through the loader fallback.
**Status**: pending

---

## Scenario 14: tsuku create with ecosystem name shows satisfies warning
**ID**: scenario-14
**Testable after**: #1826, #1827, #1828
**Category**: use-case
**Environment**: manual (requires built tsuku binary with all three issues merged)
**Commands**:
- `make build-test`
- `cp tsuku-test "$QA_HOME/bin/tsuku"`
- `run_isolated "$QA_HOME" tsuku create openssl@3 --from homebrew:openssl@3`
**Expected**: The command exits with a non-zero exit code and prints a message indicating that the `openssl` recipe already satisfies `openssl@3`, with instructions to use `--force` to override. This validates the full user flow: a user trying to create a recipe for a Homebrew formula name that's already covered by an existing recipe gets clear feedback instead of silently generating a duplicate.
**Status**: pending
