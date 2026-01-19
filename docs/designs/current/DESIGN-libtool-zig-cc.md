---
status: Current
problem: Libtool-based autotools projects fail to build with zig cc because libtool calls the compiler with `-print-prog-name=ld` to discover the linker, but zig cc doesn't support this GCC-specific flag.
decision: Enhance the cc wrapper script to detect and handle `-print-prog-name` introspection flags by returning paths to the corresponding tool wrappers (ld, ar, ranlib).
rationale: This approach directly fixes the root cause with minimal changes to a single function in util.go. The wrapper enhancement follows existing patterns in the codebase and provides genuine GCC compatibility, enabling libtool-based builds and re-enabling the No-GCC Container CI test.
---

# DESIGN: Libtool Compatibility with Zig CC

**Status:** Current

**Upstream Tracking:** [ziglang/zig#17273](https://github.com/ziglang/zig/issues/17273) - When this is resolved, we can remove our wrapper workaround.

## Context and Problem Statement

Tsuku uses zig as a fallback C/C++ compiler when no system compiler (gcc/cc) is available. This enables building from source in minimal environments. The zig cc wrapper works well for simple autotools builds, but fails with projects that use libtool.

The No-GCC Container test (`test-no-gcc` in `build-essentials.yml`) validates that `configure_make` can build tools without system gcc. This test has been disabled since PR #747 because libtool-based builds (like gdbm) fail with zig cc.

The root cause: libtool runs `$CC -print-prog-name=ld` to discover the linker, but zig cc does not support this GCC-specific flag. Even though tsuku creates an `ld` wrapper in `$TSUKU_HOME/tools/zig-cc-wrapper/`, libtool asks the compiler rather than searching PATH:

```
checking for ld used by /github/home/.tsuku/tools/zig-cc-wrapper/cc... no
configure: error: no acceptable ld found in $PATH
```

This matters because:
1. Many autotools projects use libtool (gdbm, curl, expat, etc.)
2. The zig cc fallback is a key capability for hermetic builds
3. The disabled test reduces CI coverage for no-system-compiler environments

### Scope

**In scope:**
- Making the zig cc wrapper respond to `-print-prog-name=ld`
- Enabling the No-GCC Container test to pass
- Libtool compatibility for `configure_make` action

**Out of scope:**
- Full GCC compatibility for zig cc (only targeting libtool detection)
- Other GCC-specific flags beyond `-print-prog-name`
- Changes to zig itself

## Decision Drivers

- **Minimal changes**: Solution should be small and focused
- **Maintainability**: Avoid complex shell script logic in wrappers
- **Correctness**: The linker path returned must work with zig's lld
- **Test coverage**: Re-enable the disabled CI test
- **No upstream dependency**: Cannot wait for zig to add this flag

## Implementation Context

### Existing Patterns

**Similar implementations:**
- `setupZigWrappers()` in `internal/actions/util.go:531-590`: Creates cc, c++, ar, ranlib, ld wrapper scripts
- The `ld` wrapper already exists and invokes `zig ld.lld`
- Environment setup in `SetupCCompilerEnv()` prepends wrapper directory to PATH
- Libtool cache variable pattern: `configure_make.go:345` already sets `lt_cv_sys_lib_dlsearch_path_spec=` to control libtool behavior, demonstrating this approach is established

**Conventions to follow:**
- Wrapper scripts are simple shell scripts with `exec`
- Error handling is minimal (wrappers should be transparent)
- Comments explain non-obvious behavior (like `-fPIC` and `-Wno-date-time`)

**Anti-patterns to avoid:**
- Complex conditional logic in wrapper scripts
- Hardcoded paths that don't account for `$TSUKU_HOME`

### Applicable Specifications

**Standard:** GCC `-print-prog-name` behavior
**Relevance:** The cc wrapper must respond to `-print-prog-name=ld` identically to GCC
**Behavior:** GCC outputs the linker path to stdout and exits with 0

Example:
```bash
$ gcc -print-prog-name=ld
/usr/bin/ld
```

### Research Summary

**Upstream constraints:**
- Libtool uses `-print-prog-name=ld` from GCC's option set
- Zig does not plan to support GCC-specific flags (different compiler architecture)

**Patterns to follow:**
- Shell wrapper scripts can intercept arguments before passing to zig
- The wrapper already knows its own directory path

**Implementation approach:**
- Enhance the cc wrapper to detect `-print-prog-name=ld` argument
- Return the path to the ld wrapper in the same directory
- Pass all other arguments through to zig cc unchanged

## Considered Options

### Option 1: Enhance CC Wrapper to Handle -print-prog-name

Modify the cc wrapper script to detect `-print-prog-name=ld` and return the ld wrapper path. All other arguments pass through to zig cc unchanged.

The wrapper would change from:
```bash
#!/bin/sh
exec "/path/to/zig" cc -fPIC -Wno-date-time "$@"
```

To something like:
```bash
#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    -print-prog-name=ld)
      echo "/path/to/zig-cc-wrapper/ld"
      exit 0
      ;;
  esac
done
exec "/path/to/zig" cc -fPIC -Wno-date-time "$@"
```

**Pros:**
- Directly fixes the libtool detection issue
- Minimal code change (wrapper script enhancement)
- Returns the correct ld wrapper path that invokes `zig ld.lld`
- No changes to configure_make action needed

**Cons:**
- Adds conditional logic to wrapper scripts
- Only handles `-print-prog-name=ld`, not other GCC introspection flags
- Wrapper script becomes slightly more complex

### Option 2: Set Libtool Cache Variables in Environment

Use libtool's cache variables to bypass compiler introspection entirely. Set `lt_cv_path_LD` and `lt_cv_prog_gnu_ld` in the environment before configure runs.

In `buildAutotoolsEnv()`:
```go
env = append(env, fmt.Sprintf("lt_cv_path_LD=%s", filepath.Join(wrapperDir, "ld")))
env = append(env, "lt_cv_prog_gnu_ld=yes")
```

**Pros:**
- No changes to wrapper scripts
- Standard libtool approach (documented feature)
- Works even if libtool uses other introspection methods

**Cons:**
- Requires changes to configure_make action
- Only works when tsuku controls the environment
- May cause issues if libtool version changes cache variable names
- Assumes zig's lld is compatible with GNU ld (mostly true, some edge cases)

### Option 3: Use a Different Test Recipe

Change the No-GCC Container test to use a recipe that doesn't use libtool. Find or create a simpler autotools project that tests zig cc without libtool complexity.

**Pros:**
- No code changes needed to zig wrapper
- Test can be re-enabled immediately
- Avoids libtool complexity entirely

**Cons:**
- Doesn't actually fix the libtool compatibility issue
- Real-world recipes using libtool still won't work
- Reduces the value of the test (not testing realistic scenarios)
- Finding a suitable test recipe may be difficult

### Option 4: Combined Approach (Wrapper + Environment)

Implement both Option 1 (wrapper enhancement) and Option 2 (environment variables). The wrapper provides GCC-like behavior, while the environment variables ensure libtool doesn't need to ask.

**Pros:**
- Defense in depth (two layers of compatibility)
- Handles both active introspection and cache lookup paths
- Most robust solution

**Cons:**
- More code changes than necessary
- Redundant in most cases (either solution alone works)
- Increases maintenance burden

### Evaluation Against Decision Drivers

| Option | Minimal Changes | Maintainability | Correctness | Test Coverage | No Upstream Dep |
|--------|----------------|-----------------|-------------|---------------|-----------------|
| Option 1: Wrapper | Good | Fair | Good | Good | Good |
| Option 2: Cache Vars | Good | Good | Fair | Good | Good |
| Option 3: Different Recipe | Good | Good | Poor | Poor | Good |
| Option 4: Combined | Poor | Poor | Good | Good | Good |

### Assumptions

- **Linker compatibility**: `zig ld.lld` behaves compatibly with what libtool expects from a GNU-like linker
- **Argument format**: The `-print-prog-name=ld` format (with `=`) is the only form used; GCC also accepts `-print-prog-name ld` but libtool uses the `=` form
- **Shell portability**: The `for`/`case` constructs used are POSIX-compatible and work across target shells
- **Wrapper path stability**: The ld wrapper path remains valid for the duration of the configure run
- **Test environment**: Validated against libtool from Ubuntu 22.04 container; other versions may differ

### Uncertainties

- We haven't validated whether lld's behavior is compatible with what libtool expects after detection
- Other GCC introspection flags like `-print-search-dirs` may also cause issues (not observed yet)
- Some libtool versions may use different detection mechanisms

## Decision Outcome

**Chosen option: Option 1 (Enhance CC Wrapper)**

This option directly addresses the root cause with minimal, focused changes. The wrapper script enhancement is straightforward and follows the existing pattern of wrapper scripts handling edge cases (like `-Wno-date-time` for `__DATE__` macros).

### Rationale

This option was chosen because:
- **Minimal changes**: Single function modification in `util.go`
- **Correctness**: Returns the actual ld wrapper path that libtool will use
- **Test coverage**: Directly enables the disabled test
- **Follows patterns**: Similar argument handling exists in other compiler wrappers

Alternatives were rejected because:
- **Option 2 (Cache vars)**: Although this pattern exists in the codebase (`lt_cv_sys_lib_dlsearch_path_spec`), it bypasses detection rather than fixing it; the wrapper approach provides better GCC compatibility if users run the wrapper directly
- **Option 3 (Different recipe)**: Doesn't actually fix the problem, just avoids it
- **Option 4 (Combined)**: Over-engineered for the specific issue

### Trade-offs Accepted

By choosing this option, we accept:
- The cc wrapper becomes slightly more complex (6 lines of shell logic)
- Only `-print-prog-name=ld` is handled; other introspection flags remain unsupported

These are acceptable because:
- The added complexity is minimal and well-documented
- Only `-print-prog-name=ld` has been observed causing issues; other flags can be added if needed

## Solution Architecture

### Component Changes

The change affects a single function in `internal/actions/util.go`:

**File:** `internal/actions/util.go`
**Function:** `setupZigWrappers()`
**Change:** Modify the cc and c++ wrapper generation to include `-print-prog-name=ld` handling

### Wrapper Script Design

The enhanced cc wrapper:

```bash
#!/bin/sh
# Handle GCC-specific introspection flags for libtool compatibility
for arg in "$@"; do
  case "$arg" in
    -print-prog-name=ld)
      echo "/path/to/zig-cc-wrapper/ld"
      exit 0
      ;;
    -print-prog-name=ar)
      echo "/path/to/zig-cc-wrapper/ar"
      exit 0
      ;;
    -print-prog-name=ranlib)
      echo "/path/to/zig-cc-wrapper/ranlib"
      exit 0
      ;;
  esac
done
exec "/path/to/zig" cc -fPIC -Wno-date-time "$@"
```

Key design decisions:
1. **Check all arguments**: Loop through `$@` to find the flag anywhere in argument list
2. **Early exit on match**: Print path and exit, don't invoke zig
3. **Pass-through otherwise**: Non-matching invocations work exactly as before
4. **Same logic for c++**: Apply identical handling to c++ wrapper
5. **Proactive tool support**: Handle `ar` and `ranlib` since wrappers already exist and some build systems may query them

### Integration Points

No integration changes needed. The wrapper enhancement is transparent to:
- `configure_make` action
- `SetupCCompilerEnv()` function
- Any code that uses the zig cc wrapper

## Implementation Approach

### Step 1: Modify setupZigWrappers Function

Update the cc wrapper generation in `setupZigWrappers()` to handle `-print-prog-name`:

### Step 2: Explicitly Set AR and RANLIB in SetupCCompilerEnv

Update `SetupCCompilerEnv()` to set AR and RANLIB environment variables explicitly. This bypasses libtool's archiver detection which can fail with zig ar:

```go
arWrapper := filepath.Join(wrapperDir, "ar")
ranlibWrapper := filepath.Join(wrapperDir, "ranlib")
env = append(env, fmt.Sprintf("AR=%s", arWrapper))
env = append(env, fmt.Sprintf("RANLIB=%s", ranlibWrapper))
```

### Step 3: Wrapper Script Implementation

Update the cc wrapper generation in `setupZigWrappers()`:

```go
// Create cc wrapper with libtool compatibility
ccWrapper := filepath.Join(wrapperDir, "cc")
ldWrapper := filepath.Join(wrapperDir, "ld")
arWrapper := filepath.Join(wrapperDir, "ar")
ranlibWrapper := filepath.Join(wrapperDir, "ranlib")
ccContent := fmt.Sprintf(`#!/bin/sh
# Handle GCC-specific introspection flags for libtool compatibility
for arg in "$@"; do
  case "$arg" in
    -print-prog-name=ld)
      echo "%s"
      exit 0
      ;;
    -print-prog-name=ar)
      echo "%s"
      exit 0
      ;;
    -print-prog-name=ranlib)
      echo "%s"
      exit 0
      ;;
  esac
done
exec "%s" cc -fPIC -Wno-date-time "$@"
`, ldWrapper, arWrapper, ranlibWrapper, zigPath)
```

Apply the same pattern to the c++ wrapper.

### Step 4: Re-enable No-GCC Container Test

In `.github/workflows/build-essentials.yml`:
- Remove `if: false` from `test-no-gcc` job
- Update comment to note the fix

### Step 5: Verify Locally

Test the fix by:
1. Building tsuku
2. Running gdbm-source install in a container without gcc
3. Verifying libtool configure step succeeds

## Security Considerations

### Download Verification

**Not applicable.** This change modifies wrapper script generation, not download behavior. The zig toolchain is still downloaded and verified through existing mechanisms.

### Execution Isolation

**Low risk.** The wrapper scripts run with the same permissions as before. The only change is additional shell logic that returns a path string. No new binaries are executed.

### Supply Chain Risks

**Not applicable.** No new external dependencies are introduced. The ld wrapper path is internal to tsuku's `$TSUKU_HOME/tools/zig-cc-wrapper/` directory.

### User Data Exposure

**Not applicable.** The wrapper modification doesn't access or transmit any user data. It returns a static path when a specific flag is detected.

## Consequences

### Positive

- **Re-enables CI coverage**: The No-GCC Container test validates zig cc fallback works
- **Broader libtool support**: Recipes using libtool (gdbm, many others) can build with zig cc
- **Minimal invasiveness**: Small, focused change with clear purpose

### Negative

- **Wrapper complexity increase**: The cc wrapper grows from 2 lines to 8 lines
- **Partial GCC compatibility**: Only one introspection flag is handled; future issues may require additions

### Neutral

- **Test recipe unchanged**: gdbm-source remains the test target, which is appropriate for validating libtool builds
