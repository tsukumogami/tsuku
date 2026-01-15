## Problem

When generating plans on Linux (CI), the planner incorrectly adds Linux-specific action dependencies (like patchelf) to plans for Darwin targets.

### Expected Behavior
- `HomebrewRelocateAction.Dependencies()` returns `LinuxInstallTime: []string{"patchelf"}`
- When generating a plan for darwin-arm64 target, patchelf should NOT be included
- When generating a plan for linux-amd64 target, patchelf SHOULD be included

### Actual Behavior
- Plans generated on CI (Linux runner) for darwin targets include patchelf as a nested dependency
- Plans generated locally (macOS) for darwin targets do NOT include patchelf
- This causes golden file mismatches between local generation and CI validation

### Affected Recipes
- libcurl (via brotli dependency)
- ncurses (via make dependency)

### Root Cause Hypothesis
The dependency resolver may be using `runtime.GOOS` instead of the target OS when filtering platform-specific action dependencies.

### Workaround
These recipes are temporarily excluded from golden file validation until this is fixed.

Related: This affects any recipe that has dependencies using homebrew_relocate action.
