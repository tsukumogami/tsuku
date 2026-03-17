# Test Plan: Distributed Recipes

Generated from: docs/plans/PLAN-distributed-recipes.md
Issues covered: 13
Total scenarios: 30

---

## Scenario 1: RecipeProvider interface exists and Loader compiles
**ID**: scenario-1
**Testable after**: Issue 1
**Category**: automatable
**Commands**:
- `go build ./...`
- `go test ./internal/recipe/...`
- `go vet ./...`
**Expected**: All commands exit 0. The RecipeProvider interface is defined in `internal/recipe/provider.go`. Loader uses `[]RecipeProvider` internally. No compilation errors.
**Status**: passed

---

## Scenario 2: Existing install behavior unchanged after Loader refactor
**ID**: scenario-2
**Testable after**: Issue 1
**Category**: automatable
**Commands**:
- `go test ./...`
- `tsuku install actionlint --force`
- `actionlint -version`
- `tsuku list`
**Expected**: All unit tests pass. Installing a tool from the central registry still works. `tsuku list` shows the installed tool. No behavior change from the refactor.
**Status**: passed

---

## Scenario 3: NewLoader constructor accepts provider slices
**ID**: scenario-3
**Testable after**: Issue 1
**Category**: automatable
**Commands**:
- `go test ./internal/recipe/ -run TestNewLoader`
**Expected**: `NewLoader(providers ...RecipeProvider)` replaces the three old constructors. Tests confirm the Loader resolves recipes by walking the provider slice in priority order.
**Status**: passed

---

## Scenario 4: Satisfies index tagged by source
**ID**: scenario-4
**Testable after**: Issue 1
**Category**: automatable
**Commands**:
- `go test ./internal/recipe/ -run TestSatisfies`
**Expected**: `satisfiesIndex` entries use `satisfiesEntry{recipeName, source}` structs. Source-filtered lookups return only entries from the requested source. Shadowing detection works across providers.
**Status**: passed

---

## Scenario 5: ToolState source field serialization
**ID**: scenario-5
**Testable after**: Issue 2
**Category**: automatable
**Commands**:
- `go test ./internal/install/ -run TestSource`
**Expected**: `ToolState.Source` field round-trips through JSON. A state.json without `Source` loads without error. After `Load()`, entries without `Source` get `"central"` by default. Migration is idempotent (loading twice produces identical state).
**Status**: pending

---

## Scenario 6: Source populated on new install
**ID**: scenario-6
**Testable after**: Issue 2
**Category**: automatable
**Commands**:
- `tsuku install actionlint --force`
- Check `$TSUKU_HOME/state.json` for `"source"` field on the actionlint entry
**Expected**: After a fresh install, the tool's state entry contains `"source": "central"` (or `"embedded"` if it came from the embedded registry). The source field is persisted to disk.
**Status**: pending

---

## Scenario 7: Registry config round-trips through TOML
**ID**: scenario-7
**Testable after**: Issue 3
**Category**: automatable
**Commands**:
- `go test ./internal/userconfig/ -run TestRegistries`
**Expected**: A config with `[registries]` section loads correctly. `StrictRegistries` and `Registries` map fields are preserved on save/reload. An empty registries map does not produce spurious TOML output. Backward-compatible with configs that have no registries section.
**Status**: pending

---

## Scenario 8: GetFromSource routes to correct provider
**ID**: scenario-8
**Testable after**: Issue 3
**Category**: automatable
**Commands**:
- `go test ./internal/recipe/ -run TestGetFromSource`
**Expected**: `GetFromSource(ctx, "foo", "central")` delegates to the central registry provider. `GetFromSource(ctx, "foo", "unknown")` returns a clear error. Cache is not consulted or populated by `GetFromSource`.
**Status**: pending

---

## Scenario 9: tsuku registry list with no registries
**ID**: scenario-9
**Testable after**: Issue 4
**Category**: automatable
**Commands**:
- `tsuku registry list`
**Expected**: Exit code 0. Clean output indicating no distributed registries are configured.
**Status**: pending

---

## Scenario 10: tsuku registry add and remove
**ID**: scenario-10
**Testable after**: Issue 4
**Category**: automatable
**Commands**:
- `tsuku registry add alice/tools`
- `tsuku registry list`
- `tsuku registry remove alice/tools`
- `tsuku registry list`
**Expected**: After add, `registry list` shows `alice/tools`. After remove, it no longer appears. Adding is idempotent (re-adding the same source does not error). Removing a non-existent registry exits gracefully (not a hard error).
**Status**: pending

---

## Scenario 11: tsuku registry add validates owner/repo format
**ID**: scenario-11
**Testable after**: Issue 4
**Category**: automatable
**Commands**:
- `tsuku registry add "not-a-valid-format"`
- `tsuku registry add "../traversal/attempt"`
- `tsuku registry add "user:pass@owner/repo"`
**Expected**: All commands exit with a non-zero code and produce a clear error message about invalid format. No entry is added to config.
**Status**: pending

---

## Scenario 12: tsuku registry with no subcommand shows help
**ID**: scenario-12
**Testable after**: Issue 4
**Category**: automatable
**Commands**:
- `tsuku registry`
**Expected**: Prints help text listing available subcommands (list, add, remove). Does not error.
**Status**: pending

---

## Scenario 13: GitHub HTTP client hostname allowlist
**ID**: scenario-13
**Testable after**: Issue 5
**Category**: automatable
**Commands**:
- `go test ./internal/distributed/ -run TestHostnameAllowlist`
**Expected**: Unit tests confirm that `download_url` values are validated. Only `raw.githubusercontent.com` and `objects.githubusercontent.com` are accepted. Requests to other hostnames are rejected with a clear error. HTTPS is required.
**Status**: pending

---

## Scenario 14: Auth token only sent to api.github.com
**ID**: scenario-14
**Testable after**: Issue 5
**Category**: automatable
**Commands**:
- `go test ./internal/distributed/ -run TestTokenSecurity`
**Expected**: Unit tests with mocked HTTP confirm the authenticated client sends `Authorization` header only to `api.github.com`. The unauthenticated client never includes auth headers. Token is never sent to `raw.githubusercontent.com`.
**Status**: pending

---

## Scenario 15: Rate limit handling and fallback
**ID**: scenario-15
**Testable after**: Issue 5
**Category**: automatable
**Commands**:
- `go test ./internal/distributed/ -run TestRateLimit`
**Expected**: When the Contents API returns 403/429 with rate limit headers, the client falls back to trying `main` then `master` branch raw URLs on cold cache. Error messages include rate limit reset time and suggest setting `GITHUB_TOKEN`.
**Status**: pending

---

## Scenario 16: Cache stores and retrieves recipes
**ID**: scenario-16
**Testable after**: Issue 5
**Category**: automatable
**Commands**:
- `go test ./internal/distributed/ -run TestCache`
**Expected**: Fetched recipes are stored at `$TSUKU_HOME/cache/distributed/{owner}/{repo}/{recipe}.toml`. Cache lookup returns cached data when fresh. `_source.json` stores branch name, directory listing, and fetch timestamp. Stale cache triggers re-fetch.
**Status**: pending

---

## Scenario 17: Input validation rejects path traversal
**ID**: scenario-17
**Testable after**: Issue 5
**Category**: automatable
**Commands**:
- `go test ./internal/distributed/ -run TestInputValidation`
**Expected**: Inputs like `../etc/passwd`, `owner/../repo`, and `owner/repo/../../etc` are rejected. Credentials in the input string (e.g., `user:pass@owner/repo`) are rejected. Standard `owner/repo` patterns pass validation.
**Status**: pending

---

## Scenario 18: DistributedProvider implements RecipeProvider
**ID**: scenario-18
**Testable after**: Issue 6
**Category**: automatable
**Commands**:
- `go test ./internal/distributed/ -run TestDistributedProvider`
- `go vet ./...`
**Expected**: `DistributedProvider` compiles and satisfies both `RecipeProvider` and `RefreshableProvider` interfaces. `Source()` returns `SourceDistributed`. `Get()` and `List()` work with mocked HTTP responses.
**Status**: pending

---

## Scenario 19: Qualified name routing in Loader
**ID**: scenario-19
**Testable after**: Issue 6
**Category**: automatable
**Commands**:
- `go test ./internal/recipe/ -run TestQualifiedName`
**Expected**: Names containing `/` are routed to the DistributedProvider. The Loader strips the `owner/repo:` prefix when calling `Get()`, but uses the full qualified name `"owner/repo:foo"` as the in-memory cache key. A central recipe named `foo` and a distributed recipe `alice/tools:foo` don't collide.
**Status**: pending

---

## Scenario 20: Install from distributed source with name parsing
**ID**: scenario-20
**Testable after**: Issue 7
**Category**: environment-dependent (requires GITHUB_TOKEN for API access)
**Commands**:
- `tsuku registry add tsukumogami/koto`
- `tsuku install tsukumogami/koto -y`
- `tsuku list`
**Expected**: The install command parses `tsukumogami/koto` as a distributed source. Since the source is pre-registered, no confirmation prompt appears. The tool installs successfully and `tsuku list` shows it with a `[tsukumogami/koto]` source suffix. State.json records `"source": "tsukumogami/koto"`.
**Status**: pending

---

## Scenario 21: Install from unregistered source with strict mode
**ID**: scenario-21
**Testable after**: Issue 7
**Category**: automatable
**Commands**:
- `tsuku config set strict_registries true`
- `tsuku install alice/tools -y`
**Expected**: Exit code is non-zero. Error message suggests using `tsuku registry add alice/tools` first. No install occurs. No registry entry is auto-created.
**Status**: pending

---

## Scenario 22: Install name format parsing
**ID**: scenario-22
**Testable after**: Issue 7
**Category**: automatable
**Commands**:
- `go test ./cmd/tsuku/ -run TestNameParsing`
**Expected**: Unit tests confirm parsing of all supported formats: `owner/repo`, `owner/repo:recipe`, `owner/repo@version`, `owner/repo:recipe@version`. Invalid formats produce clear errors.
**Status**: pending

---

## Scenario 23: Source collision detection
**ID**: scenario-23
**Testable after**: Issue 7
**Category**: automatable
**Commands**:
- `go test ./cmd/tsuku/ -run TestCollision`
**Expected**: When a tool named `foo` is installed from central and the user attempts `tsuku install alice/tools:foo`, a collision prompt appears. `--force` skips the prompt. Reinstalling from the same source does not trigger the collision prompt.
**Status**: pending

---

## Scenario 24: Update uses source-directed loading
**ID**: scenario-24
**Testable after**: Issue 8
**Category**: automatable
**Commands**:
- `go test ./cmd/tsuku/ -run TestUpdateSourceDirected`
**Expected**: `update` reads `ToolState.Source` and calls `GetFromSource`. Central/embedded sources use the existing chain. When the source is empty (pre-migration state), it defaults to `"central"`. When the source is unreachable, it falls back gracefully instead of failing hard.
**Status**: passed

---

## Scenario 25: Outdated handles unreachable sources as warnings
**ID**: scenario-25
**Testable after**: Issue 8
**Category**: automatable
**Commands**:
- `go test ./cmd/tsuku/ -run TestOutdatedUnreachable`
**Expected**: When checking installed tools, unreachable distributed sources produce warnings in output but do not cause a fatal error. Exit code remains 0 (or the existing exit code for "some tools are outdated"). Central tools continue to be checked normally.
**Status**: passed

---

## Scenario 26: List shows source for distributed tools
**ID**: scenario-26
**Testable after**: Issue 9
**Category**: automatable
**Commands**:
- `go test ./cmd/tsuku/ -run TestListSource`
**Expected**: Human-readable `tsuku list` output shows source suffix for distributed tools (e.g., `ripgrep 14.1.1 [alice/tools]`). `tsuku list --json` includes a `"source"` field. Central tools do not show a source suffix (backward compatible).
**Status**: pending

---

## Scenario 27: update-registry refreshes distributed sources
**ID**: scenario-27
**Testable after**: Issue 10
**Category**: automatable
**Commands**:
- `go test ./cmd/tsuku/ -run TestUpdateRegistryDistributed`
**Expected**: `tsuku update-registry` iterates all providers and calls `Refresh()` on those implementing `RefreshableProvider`. Errors from individual distributed sources are reported but do not block refresh of other sources. Output indicates which distributed sources were refreshed.
**Status**: pending

---

## Scenario 28: Koto repo has valid .tsuku-recipes directory
**ID**: scenario-28
**Testable after**: Issue 11
**Category**: environment-dependent (requires access to tsukumogami/koto repo)
**Commands**:
- Check that `.tsuku-recipes/` exists at the root of the koto repository
- Validate that at least one `.toml` file is present
- Parse the TOML file to confirm it uses the same schema as central registry recipes
**Expected**: The `.tsuku-recipes/` directory exists with at least one valid recipe. Recipe files use kebab-case names and have a `[version]` section with a version provider.
**Status**: pending

---

## Scenario 29: Koto recipes removed from central registry
**ID**: scenario-29
**Testable after**: Issue 12
**Category**: automatable
**Commands**:
- Check that no koto-related TOML files remain in `recipes/`
- `go test ./...`
**Expected**: The central `recipes/` directory no longer contains koto recipes. CI passes. The central registry manifest does not reference koto entries.
**Status**: pending

---

## Scenario 30: End-to-end distributed install, lifecycle, and removal
**ID**: scenario-30
**Testable after**: Issue 13
**Category**: manual (requires released binary + GITHUB_TOKEN + koto repo with .tsuku-recipes)
**Commands**:
- `tsuku install tsukumogami/koto`
- (accept confirmation prompt)
- `tsuku list`
- `tsuku info <koto-tool>`
- `tsuku registry list`
- `tsuku recipes`
- `tsuku outdated`
- `tsuku verify <koto-tool>`
- `tsuku update <koto-tool>`
- `tsuku remove <koto-tool>`
- `tsuku install tsukumogami/koto -y`
- Set `strict_registries = true`, remove registry, try install again
**Expected**: First install shows confirmation prompt; accepting auto-registers `tsukumogami/koto` in config.toml. `list` shows the tool with source annotation. `info` includes a `Source:` line. `registry list` shows `tsukumogami/koto`. `recipes` includes distributed recipes. `outdated`/`verify`/`update` work correctly. `remove` cleanly removes the tool. Second install with `-y` skips the prompt (source already registered). With `strict_registries = true` and the source removed from registries, install fails with a clear error.
**Status**: pending
