# Architect Review: Issue #1644

**Issue**: #1644 test(llm): add end-to-end integration test without cloud keys
**Focus**: architecture (design patterns, separation of concerns)
**File reviewed**: `internal/llm/local_e2e_test.go`

## Design Alignment

The test correctly exercises the architectural flow described in the design doc's "first-time user" data flow: no cloud API keys -> factory falls through to LocalProvider -> `Complete()` returns structured tool calls. The test sits in `internal/llm` (same package as LocalProvider, Factory, and the existing `lifecycle_integration_test.go`), which is the right location for a test that validates the provider-level contract.

The test uses existing architectural constructs properly:
- Factory options pattern (`WithLocalEnabled`, `WithPrompter`) from #1632
- `AutoApprovePrompter` from the addon package (#1642)
- `IdleTimeoutEnvVar` constant from lifecycle (#1630)
- `buildToolDefs()` from tools.go for the tool schema
- Provider interface (`Name()`, `Complete()`) -- the test validates through the interface, not through implementation internals

The build tag `e2e` establishes a new test tier separate from the existing `integration` tag. This is appropriate: the `integration` tests (in `lifecycle_integration_test.go`) test daemon lifecycle at the gRPC protocol level, while this E2E test validates the full provider flow including factory fallback and response semantics. Two distinct concerns, two distinct tags.

## Finding 1: Parallel prompt construction functions (Advisory)

**File**: `internal/llm/local_e2e_test.go:243-276`

The test defines `recipeGenerationSystemPrompt()` and `recipeGenerationUserPrompt()` which are simplified versions of the production `buildSystemPrompt()` and `buildUserMessage()` in `internal/llm/client.go` (and separately in `internal/builders/github_release.go`). This creates three locations with prompt construction logic.

However, the test's prompts are intentionally different:
- The test prompt is simplified for a specific tool (`jq`) with hardcoded release assets, designed to get a deterministic result from a small model
- The production prompts are generic, handling dynamic release data, README truncation, and multi-tool patterns
- The test prompts include inline release asset data that the production code gets from the GitHub API

This is a test fixture, not a parallel pattern. The test doesn't claim to replicate the production prompt -- it constructs a minimal prompt that exercises the same tool schema. The tool definitions themselves (`buildToolDefs()`) are shared, which is the structural part that matters. The prompt text is test data.

**Severity**: Advisory. This doesn't create pattern divergence. If the production prompt changes, the test still validates that the local provider can produce structured tool calls from a recipe-like prompt. The shared `buildToolDefs()` ensures the tool schema stays in sync.

## Finding 2: Duplicated addon binary discovery logic (Advisory)

**File**: `internal/llm/local_e2e_test.go:157-196`

The `findAddonBinary()` function duplicates logic from `getAddonBinary()` in `lifecycle_integration_test.go:168-194`. Both:
1. Check `TSUKU_LLM_BINARY` env var
2. Walk up to find `go.mod` (workspace root)
3. Try `tsuku-llm/target/release/tsuku-llm` then `tsuku-llm/target/debug/tsuku-llm`

The semantic difference is that `findAddonBinary` returns `""` on failure (test skips), while `getAddonBinary` calls `t.Skip()` directly. This is a minor behavioral difference.

Since both functions are in test files with different build tags (`e2e` vs `integration`), they can't share code through a helper in the same package without a third file (no build tag) that both include. The duplication is contained to test infrastructure and has no structural impact on production code.

**Severity**: Advisory. The duplication is bounded to two test files and doesn't affect the production architecture. A shared test helper file (no build tag) could eliminate this, but it's not structurally necessary.

## Finding 3: Test correctly uses Provider interface, not bypassing factory (No finding)

The test creates the factory via `NewFactory()` and retrieves the provider via `factory.GetProvider()` -- the same path production code takes. It doesn't instantiate `LocalProvider` directly for the inference call. The cleanup path (`provider.(*LocalProvider)` type assertion for `Shutdown` and `Close`) is specific to the test's lifecycle needs, not a bypass of the provider abstraction.

## Finding 4: No dependency direction violations (No finding)

The test imports:
- `github.com/tsukumogami/tsuku/internal/llm/addon` (lower-level package, correct direction)
- Standard library packages
- `github.com/stretchr/testify/require` (test dependency)

No circular or upward dependencies.

## Finding 5: Build tag isolation is consistent with existing patterns (No finding)

The existing integration tests use `//go:build integration`. This test uses `//go:build e2e`. The distinction is meaningful: `integration` tests validate daemon lifecycle (start, shutdown, lock files), while `e2e` tests validate the full application flow (factory -> provider -> inference -> response validation). Different hardware requirements (E2E needs a model loaded; integration tests work with lighter-weight daemon operations) justify separate tags.

## Overall Assessment

The implementation fits the existing architecture cleanly. It uses the factory and provider interface as designed, reuses shared tool definitions, and follows the test organization patterns established by the integration tests. The two advisory findings (duplicated prompt construction and addon binary discovery) are contained to test code and don't introduce structural patterns that production code would copy.
