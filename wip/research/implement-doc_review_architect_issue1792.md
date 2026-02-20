# Architect Review: Issue #1792

**test(ci): add recipe validation for GPU when clauses and dependency chains**

**File changed:** `internal/executor/plan_generator_test.go`

## Summary

This issue adds test coverage for GPU `WhenClause` filtering at plan generation time and recursive dependency chain resolution for GPU recipes. The tests are placed in the `executor` package's existing test file and exercise the plan generator through the established `GeneratePlan()` / `FilterStepsByTarget()` entry points.

## Findings

### No blocking issues found.

The implementation respects the existing architecture. Specific observations:

**1. Correct use of established interfaces.** The new tests use `PlanConfig.GPU` to drive GPU filtering through `GeneratePlan()`, which is exactly how the production code path works (lines 1976-1981, 2160-2166, 2513-2519). The `mockRecipeLoader` implements `actions.RecipeLoader` (the `GetWithContext` signature), consistent with the interface at `internal/actions/resolver.go:25`. No dispatch bypass.

**2. FilterStepsByTarget usage for download step tests (lines 2295-2410).** The comment at line 2301 explains the decision to test download step filtering via `FilterStepsByTarget` rather than through `GeneratePlan`, since `github_file` is a composite action whose decomposition requires network access. This is architecturally sound -- the filtering logic runs before decomposition in the production path, so testing filtering independently is valid. The function is a public API in the `executor` package and already tested in `filter_test.go` (lines 267-340).

**3. Non-composite actions for dependency chain tests.** The dependency chain tests (lines 2412-2835) use `chmod` and `install_binaries` instead of the real `github_file`/`download_archive` actions. The comment at line 2421-2423 explains this avoids network access while exercising the full recursive dependency resolution path. This is a good trade-off: the code under test is the dependency resolution logic in `GeneratePlan`, not the action decomposition logic. Using non-composite actions isolates what's being tested without introducing a parallel testing pattern.

**4. Recipe structure mirrors design doc.** The mock recipes (tsuku-llm, cuda-runtime, nvidia-driver, vulkan-loader, mesa-vulkan-drivers) follow the dependency chains specified in the design doc: `tsuku-llm -> cuda-runtime -> nvidia-driver` and `tsuku-llm -> vulkan-loader -> mesa-vulkan-drivers`. Step-level `Dependencies` on when-filtered steps and metadata-level `Dependencies` on the recipe both exercise the production resolution path.

**5. Constructor usage matches codebase patterns.** `platform.NewTarget("linux/amd64", "debian", "glibc", tt.gpu)` uses the 4-parameter constructor (line 2382), matching the signature at `internal/platform/target.go:45`. `recipe.NewMatchTarget(tt.targetOS, tt.targetArch, "", "", "")` uses the 5-parameter constructor (line 169), matching `internal/recipe/types.go:210`. No constructors are bypassed or fields set directly.

### Advisory observations

**A1. Version resolution dependency on `nodejs_dist`.** All tests that go through `GeneratePlan()` use `Source: "nodejs_dist"` for version resolution and rely on `t.Skipf` when network is unavailable (e.g., lines 2522, 2679, 2809). This is a pre-existing pattern throughout the file (see lines 544, 622, 682, etc.), not introduced by this change. No action needed, but worth noting that these tests are skipped in offline CI environments. The dependency chain tests do successfully exercise recursive resolution because the mock recipes' `nodejs_dist` version source resolves when online, and the step filtering/dependency logic runs regardless.

**A2. Test file size.** The file is now ~2835 lines. This is a pre-existing growth trajectory (the file was already large before this change). The tests are well-organized by function name prefix (`TestGeneratePlan_GPU*`), so discoverability is acceptable. Not a structural issue.

## Verdict

The change fits the existing architecture. Tests exercise GPU filtering and dependency chain resolution through the established entry points (`GeneratePlan`, `FilterStepsByTarget`) without introducing parallel patterns. The `mockRecipeLoader` correctly implements the `actions.RecipeLoader` interface. Constructor usage is consistent with the codebase. No blocking issues.
