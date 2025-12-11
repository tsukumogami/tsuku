# Issue 398 Baseline: Split install.go into focused modules

## Starting Point

- **Branch**: `refactor/398-split-install-go`
- **Base**: `main`
- **File**: `cmd/tsuku/install.go` (603 lines)

## Build Status

- `go build ./cmd/tsuku`: PASS
- `go test -short ./...`: Tests pass (pre-existing failures in LLM integration and cleanup tests unrelated to this refactoring)

## File Structure Analysis

### Current install.go (603 lines)

The file contains three distinct domains:

1. **CLI concerns** (~100 lines)
   - `installCmd` command definition (lines 22-69)
   - `init()` with flag definitions (lines 71-74)
   - `isInteractive()` and `confirmInstall()` (lines 76-95)
   - `runDryRun()` (lines 546-568)

2. **Dependency handling** (~200 lines)
   - `ensurePackageManagersForRecipe()` (lines 101-153)
   - `findDependencyBinPath()` (lines 155-181)
   - `installWithDependencies()` (lines 183-474) - main installation logic
   - `resolveRuntimeDeps()` (lines 570-594)
   - `mapKeys()` helper (lines 596-603)

3. **Library installation** (~65 lines)
   - `installLibrary()` (lines 476-544)

### Proposed Split (per acceptance criteria)

| New File | Contents | Approx Lines |
|----------|----------|--------------|
| `install.go` | CLI concerns, command, flags, prompts | ~100 |
| `install_deps.go` | Dependency resolution and handling | ~200 |
| `install_lib.go` | Library installation logic | ~70 |

## Dependencies

The file imports:
- `bufio`, `fmt`, `os`, `path/filepath`, `strings` (stdlib)
- `github.com/spf13/cobra`
- `github.com/tsukumogami/tsuku/internal/actions`
- `github.com/tsukumogami/tsuku/internal/config`
- `github.com/tsukumogami/tsuku/internal/executor`
- `github.com/tsukumogami/tsuku/internal/install`
- `github.com/tsukumogami/tsuku/internal/recipe`
- `github.com/tsukumogami/tsuku/internal/telemetry`

## Key Functions to Extract

### To `install_deps.go`:
- `runInstallWithTelemetry()` - entry point for installation with telemetry
- `installWithDependencies()` - core recursive installation
- `ensurePackageManagersForRecipe()` - package manager bootstrap
- `findDependencyBinPath()` - locate dependency bin directories
- `resolveRuntimeDeps()` - resolve runtime dependencies
- `mapKeys()` - helper function

### To `install_lib.go`:
- `installLibrary()` - library installation logic

### Keep in `install.go`:
- `installCmd` variable and command definition
- `installDryRun`, `installForce` flags
- `init()` function
- `isInteractive()` and `confirmInstall()` helpers
- `runDryRun()` function
