---
status: Accepted
problem: |
  The configure_make, cmake_build, and meson_build actions require an executables
  parameter with at least one binary name. This prevents using them for library-only
  packages that produce .so/.a files and headers but no executables. Three library
  recipes (gmp, abseil, cairo) work around this by using apk_install on musl Linux,
  which loses version pinning and self-containment.
decision: |
  Make executables optional in all three compile actions. When omitted, the actions
  skip post-build binary verification and RPATH fixup on executables. The subsequent
  install_binaries step with its outputs parameter still validates that the build
  produced expected files. configure_make's Preflight validation downgrades
  executables from required to optional.
rationale: |
  This is the minimum change that unblocks library compilation. The post-build
  executable check exists as a convenience verification, not a security gate -- the
  install_binaries step already validates outputs independently. Making executables
  optional rather than introducing new parameters or action variants keeps the
  surface area small and avoids recipe-level churn for existing packages.
---

# Optional Executables in Compile Actions

## Status

**Accepted**

## Context and Problem Statement

The three compile-from-source actions (`configure_make`, `cmake_build`, `meson_build`) all require an `executables` parameter containing at least one binary name. After the build completes, each action verifies that the named binaries exist in `<install_dir>/bin/`. The `meson_build` action also uses the list to apply RPATH fixups on those binaries.

This requirement blocks the actions from being used for library-only packages. Libraries produce shared objects (`.so`/`.dylib`), static archives (`.a`), headers, and pkg-config files, but no executables. Three recipes currently work around this:

- **gmp** (autotools): Uses `apk_install` on musl instead of `configure_make`
- **abseil** (cmake): Uses `apk_install` on musl instead of `cmake_build`
- **cairo** (meson): Uses `apk_install` on musl instead of `meson_build`

The `apk_install` workaround delegates to Alpine's system package manager. This works but has costs: no version pinning (you get whatever Alpine ships), no self-containment (files land in system directories rather than `$TSUKU_HOME`), and a different behavior model than every other platform variant of the same recipe.

On glibc Linux and macOS, these same recipes use Homebrew bottles with `install_binaries install_mode="directory"`, which copies the full build tree and lists library outputs explicitly. There's no reason musl Linux can't do the same with compile-from-source -- except the compile actions refuse to run without executables.

### Scope

**In scope:**
- Making `executables` optional in `configure_make`, `cmake_build`, and `meson_build`
- Updating `configure_make`'s Preflight validation
- Adjusting post-build behavior when `executables` is absent

**Out of scope:**
- Migrating existing library recipes from `apk_install` to compile actions (separate follow-up)
- Adding new verification mechanisms for library outputs inside compile actions
- Changes to `install_binaries`, `ExtractBinaries()`, or recipe validation (they already handle this)

## Decision Drivers

- **Backward compatibility**: Existing recipes with `executables` must behave identically
- **Minimal change**: The fix should be small and obvious, not a redesign of compile actions
- **Correct by default**: Omitting executables shouldn't silently swallow real build failures
- **Existing patterns**: Library recipes already use `type = "library"` and optional verify sections; the codebase already handles libraries as a distinct category

## Considered Options

### Decision 1: How to Handle Missing Executables

When a compile action runs without `executables`, it needs to decide what to do after the build completes. The current post-build step verifies that named binaries exist in `bin/`. Without any binaries to check, there's a question of whether to substitute a different verification or skip verification entirely.

#### Chosen: Skip Post-Build Verification

When `executables` is absent or empty, the compile action runs the build (configure/make, cmake, or meson) normally but skips the final step that checks for binaries in `bin/`. For `meson_build`, it also skips the RPATH fixup loop since there are no executables to patch.

The rationale is that verification already happens elsewhere. Library recipes follow this pattern:

```toml
[[steps]]
action = "configure_make"
source_dir = "gmp-{version}"
configure_args = ["--enable-shared"]
when = { os = ["linux"], libc = ["musl"] }

[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["lib/libgmp.so", "lib/libgmp.a", "include/gmp.h"]
when = { os = ["linux"], libc = ["musl"] }
```

The `install_binaries` step that follows the compile action already copies the install tree and registers outputs. If the build didn't produce the expected library files, `install_binaries` will fail when those outputs don't exist. This provides equivalent confidence without requiring fake executable names.

#### Alternatives Considered

**Add an `outputs` parameter to compile actions for library verification**: Each compile action would accept an optional `outputs` list (like `install_binaries` does) and verify those paths exist after the build. Rejected because it duplicates logic that `install_binaries` already handles, and recipe authors would need to list outputs in two places.

**Require `executables = []` (explicit empty list) instead of omitting the key**: This would make the opt-in explicit rather than implicit. Rejected because the TOML representation is awkward (`executables = []` reads as "there should be executables but there happen to be zero"), and it contradicts the existing pattern where optional parameters are simply omitted.

## Decision Outcome

**Chosen: Skip post-build verification when executables is absent**

### Summary

The change touches the same code path in all three compile actions. Each action's `Execute` method currently fails early if `executables` is missing or empty. Instead, when `executables` isn't provided, the action continues with an empty list. The post-build verification loop (which iterates over executables to stat `bin/<name>`) naturally does nothing with an empty list. The RPATH fixup in `meson_build` also iterates over executables and does nothing when empty.

For `configure_make`, the `Preflight` method also needs updating. It currently reports a validation error when `executables` is absent. The fix changes this to a no-op when the parameter is missing.

The `ExtractBinaries()` method in `internal/recipe/types.go` already handles missing `executables` gracefully -- the `if executablesRaw, ok := step.Params["executables"]; ok` check skips the block when the key is absent. No change needed there.

The security validation for executable names (path traversal checks) still runs when `executables` is provided. It's just not reached when the parameter is absent, which is correct since there are no names to validate.

### Rationale

This is the smallest possible change that solves the problem. The verification concern is addressed by existing infrastructure: `install_binaries` validates outputs, and library recipes already skip the `[verify]` section (the recipe validator allows this when `type = "library"`). Adding new verification inside compile actions would duplicate what `install_binaries` does. The existing recipe pattern of compile-then-install_binaries already separates "building" from "registering outputs."

## Solution Architecture

### Overview

Three files change, with the same pattern in each: remove the hard error on missing `executables` and let the build proceed with an empty list.

### Changes by File

**`internal/actions/configure_make.go`**

1. `Preflight()`: Remove the error for missing `executables`. The parameter becomes optional.
2. `Execute()`: Replace the early-return error with a fallback to an empty slice. The rest of the function (build steps, post-build verification loop) works unchanged since iterating an empty slice is a no-op.

**`internal/actions/cmake_build.go`**

1. `Execute()`: Same change as configure_make -- replace the early error with an empty slice fallback.

**`internal/actions/meson_build.go`**

1. `Execute()`: Same change as cmake_build. Both the RPATH fixup loop (step 4) and the verification loop (step 5) iterate over executables, so both become no-ops with an empty list.

### Code Change Pattern

Each action's executables extraction changes from:

```go
executables, ok := GetStringSlice(params, "executables")
if !ok || len(executables) == 0 {
    return fmt.Errorf("<action> action requires 'executables' parameter...")
}
```

To:

```go
executables, _ := GetStringSlice(params, "executables")
```

The subsequent path-traversal validation loop and post-build verification loop remain unchanged. They iterate over `executables`, which is `nil` (or empty) when not provided, so they simply don't execute.

### What Doesn't Change

- `ExtractBinaries()` in `types.go`: Already handles missing `executables` via an `ok` check
- Recipe validation in `validator.go`/`validate.go`: Validates compile action parameters through Preflight (only configure_make) and execution-time checks; both will accept the missing parameter after this change
- `install_binaries` action: Unchanged; continues to validate its own `outputs` parameter independently
- Library verify behavior: Already optional for `type = "library"` recipes
- Existing recipes: All current compile-action recipes provide `executables` and continue working identically

## Implementation Approach

This is a single-phase change. All three files can be modified together since they're independent.

1. Update `configure_make.go`: Modify `Preflight()` and `Execute()`
2. Update `cmake_build.go`: Modify `Execute()`
3. Update `meson_build.go`: Modify `Execute()`
4. Update existing tests: Three tests that assert failure on missing executables (`TestConfigureMakeAction_Execute_MissingExecutables`, `TestCMakeBuildAction_Execute_MissingExecutables`, `TestMesonBuildAction_Execute_MissingExecutables`) need to be updated to expect success instead of error. The mock validator test in `validator_test.go` that checks Preflight also needs updating.
5. Add tests: One test per action verifying that omitting `executables` results in a successful build that skips post-build binary verification

## Security Considerations

### Download Verification

Not affected. This change doesn't modify how source archives are downloaded or verified. The download and extraction steps happen before the compile action runs and are handled by separate actions (`download_file`, `extract`).

### Execution Isolation

Not affected. The compile actions run the same build commands (configure/make, cmake, meson) regardless of whether `executables` is provided. The build environment, permissions, and sandbox behavior are unchanged.

### Supply Chain Risks

Not affected. Source code still comes from the same upstream sources (release tarballs). The compile action doesn't influence where the code comes from or how it's authenticated.

### User Data Exposure

Not affected. Compile actions don't access or transmit user data. They compile source code in a work directory and install results to an install directory.

## Consequences

### Positive

- Library recipes can use compile-from-source on musl Linux with version pinning
- Removes the need for `apk_install` workarounds that break self-containment
- Consistent behavior across glibc/musl/macOS variants of the same recipe
- No new parameters, actions, or concepts to learn

### Negative

- A recipe author could accidentally omit `executables` from a tool recipe and not catch it until `install_binaries` fails (delayed error)
- The post-build verification in compile actions was a useful "did the build actually produce what we expected" check that's now opt-in rather than mandatory

### Mitigations

- The `install_binaries` step still catches missing outputs, so the failure is delayed by one step but still caught before installation completes. This relies on the convention that library recipes always include an `install_binaries` step after the compile step -- a pattern followed by all existing compile-action recipes and enforced by recipe review
- Sandbox validation also catches broken builds before they reach production
- Recipe CI validates recipes end-to-end, so a missing executable would be caught during PR review
