# Phase 4 + Phase 8 Review: Optional Executables in Compile Actions

## Design Under Review

`/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/designs/DESIGN-optional-executables.md`

---

## Phase 4: Options Review

### 1. Problem Statement Specificity

The problem statement is well-grounded. It names the three affected actions (`configure_make`, `cmake_build`, `meson_build`), the three workaround recipes (gmp, abseil, cairo), and the concrete costs of the workaround (no version pinning, no self-containment via `apk_install`). The scope section explicitly bounds what is and isn't being changed.

I verified the claims against the codebase:

- **Confirmed**: All three actions fail with `requires 'executables' parameter with at least one executable` when the parameter is absent. See lines 93-96 of `configure_make.go`, 66-69 of `cmake_build.go`, and 71-73 of `meson_build.go`.
- **Confirmed**: The three library recipes (`recipes/g/gmp.toml`, `recipes/a/abseil.toml`, `recipes/c/cairo.toml`) use `apk_install` for musl Linux while using Homebrew bottles for glibc/macOS.
- **Confirmed**: `ExtractBinaries()` in `types.go:898-912` handles missing `executables` via an `ok` check (`if executablesRaw, ok := step.Params["executables"]; ok`).

The problem statement is specific enough to evaluate solutions.

### 2. Missing Alternatives

No significant alternatives are missing. The design covers:

- Skip verification (chosen)
- Add `outputs` to compile actions (rejected)
- Require `executables = []` (rejected)

One alternative the design doesn't mention is introducing a separate action variant (e.g., `configure_make_lib`) that omits the executables requirement. This would be worse than the chosen approach because it duplicates the entire compile action with a trivial parameter difference. Not worth adding to the document since it's obviously inferior and would create a parallel pattern.

### 3. Rejection Rationale

Both rejections are fair and specific:

- **`outputs` parameter**: Rejected because it duplicates `install_binaries` logic. This is correct -- the recipe pattern is already compile-then-install_binaries, and library recipes already specify outputs in the `install_binaries` step. Adding it to compile actions would require listing outputs twice.
- **`executables = []`**: Rejected because it reads awkwardly in TOML and contradicts the existing pattern where optional parameters are omitted. This is consistent with how other optional parameters work across all actions.

Neither rejection is a strawman.

### 4. Unstated Assumptions

One assumption worth making explicit:

**Assumption: Library recipes always have a subsequent `install_binaries` step that validates outputs.** The design relies on this for its safety argument ("verification already happens elsewhere"). This is true for the three named recipes, and the design document shows the expected recipe pattern. But there's no enforcement that a library compile step must be followed by `install_binaries`. A recipe with only a `configure_make` step and no `install_binaries` would silently succeed even if the build produced nothing.

This is a minor concern, not a blocking one. Recipe CI and sandbox validation catch this during PR review. The same risk already exists for other optional parameters. But the design's mitigations section should acknowledge that the safety net depends on recipe authors following the established pattern.

### 5. Strawman Check

Neither alternative is a strawman. Both are plausible approaches that have real drawbacks the document explains concretely. The `outputs` approach in particular would be a reasonable choice in a codebase that didn't already have `install_binaries` as a separate step.

---

## Phase 8: Architecture Review

### 1. Architecture Clarity

The design is clear enough to implement. The code change pattern is shown explicitly (before/after), the three files are listed with specific modifications per file, and the "What Doesn't Change" section prevents scope creep.

One clarity gap: the design mentions updating log output (step 5 in Implementation Approach) to say "Build completed (library only, no executables)" but doesn't specify the condition for this message. Presumably `len(executables) == 0`, but this should be explicit since `configure_make.go:297` currently prints `Installed %d executable(s)` which would print "Installed 0 executable(s)" -- technically correct but potentially confusing.

### 2. Missing Components or Interfaces

**The design correctly identifies that `cmake_build` and `meson_build` don't implement `Preflight`.** Only `configure_make` has a `Preflight()` method (verified: no `Preflight` method exists on `CMakeBuildAction` or `MesonBuildAction`). So the Preflight change is correctly scoped to `configure_make` only.

**Test impact is understated.** The design mentions adding new tests (step 4) but doesn't mention that existing tests will break. Three tests assert that missing executables produces an error:

- `TestConfigureMakeAction_Execute_MissingExecutables` (`configure_make_test.go:41-72`)
- `TestCMakeBuildAction_Execute_MissingExecutables` (`cmake_build_test.go:41-72`)
- `TestMesonBuildAction_Execute_MissingExecutables` (`meson_build_test.go:41-72`)

These tests assert `err.Error() != "<action> action requires 'executables' parameter with at least one executable"`. After the change, these tests will fail because Execute won't return that error. The implementation needs to either remove these tests or change them to assert that Execute proceeds without error when executables is absent.

Additionally, the Preflight test in `validator_test.go:107-113` has a mock that validates `configure_make` requires `executables`. This is a test-only mock (not the real Preflight), but it would need updating to match the new behavior if the test suite is expected to reflect actual validation behavior.

**`ExtractBinaries()` interaction needs acknowledgment.** The design correctly says `ExtractBinaries()` handles missing executables gracefully. But `ExtractBinaries()` still lists `configure_make`, `cmake_build`, and `meson_build` as `installActions` (types.go:832-834). For a library recipe that compiles with `configure_make` and then uses `install_binaries`, `ExtractBinaries()` will skip the compile step (no executables) and pick up the outputs from the `install_binaries` step. This is correct behavior, but it means library recipes won't get symlinks to `$TSUKU_HOME/bin/` from the compile step (expected) and will instead get their outputs registered via `install_binaries` (also expected). No change needed -- just confirming the interaction is correct.

### 3. Implementation Phase Sequencing

The design says "single-phase change" with all three files modified together. This is correct -- the three actions are independent of each other and share no state. The only dependency is between the Preflight change and the Execute change in `configure_make.go`, and both are in the same file.

The sequencing of implementation steps is also correct: code changes before tests, since the tests need to assert the new behavior.

### 4. Simpler Alternatives

No simpler alternative exists. The design already found the minimum change: remove three error checks, let empty-slice iteration handle the rest. The code change is ~3 lines of deletion across three files (plus the Preflight adjustment). There's nothing to simplify further.

---

## Findings Summary

### Blocking Issues

None.

### Advisory Issues

1. **Test breakage not documented.** Three existing unit tests (`TestConfigureMakeAction_Execute_MissingExecutables`, `TestCMakeBuildAction_Execute_MissingExecutables`, `TestMesonBuildAction_Execute_MissingExecutables`) and one mock validator in `validator_test.go:111-112` will break. The implementation approach should mention updating these alongside adding new tests.

2. **Unstated assumption about recipe structure.** The safety argument depends on library recipes always having an `install_binaries` step after the compile step. This is a convention, not an enforcement. The mitigations section should note this.

3. **Log message for empty executables.** The implementation approach step 5 mentions updating the log message but doesn't specify the exact condition. The current code at `configure_make.go:297`, `cmake_build.go:179`, and `meson_build.go:252` all print `Installed %d executable(s)` which would say "Installed 0 executable(s)". The design should specify the conditional branch more precisely.

### Out of Scope

- Code quality of the existing compile actions (e.g., the extensive git-specific debugging in `configure_make.go:271-294`) -- this is a maintainability concern, not a structural one.
- Whether `apk_install` should be deprecated for library recipes -- that's a separate follow-up as the design correctly notes.
