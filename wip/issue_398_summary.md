# Issue 398 Summary: Split install.go into focused modules

## Changes Made

Split `cmd/tsuku/install.go` (603 lines) into three focused modules:

### install.go (114 lines) - CLI concerns
- `installCmd` command definition
- `installDryRun`, `installForce` flag variables
- `init()` with flag definitions
- `isInteractive()` - check if stdin is a terminal
- `confirmInstall()` - user confirmation prompt
- `runDryRun()` - dry-run execution

### install_deps.go (428 lines) - Dependency handling
- `runInstallWithTelemetry()` - entry point with telemetry
- `installWithDependencies()` - core recursive installation logic
- `ensurePackageManagersForRecipe()` - package manager bootstrap
- `findDependencyBinPath()` - locate dependency bin directories
- `resolveRuntimeDeps()` - resolve runtime dependencies
- `mapKeys()` - helper function

### install_lib.go (80 lines) - Library installation
- `installLibrary()` - handles library recipe installation

## Verification

- `go build ./cmd/tsuku` - PASS
- `go test ./cmd/tsuku` - PASS
- `go vet ./cmd/tsuku` - PASS
- `gofmt` - All files formatted

## Benefits

1. **Reduced merge conflicts**: High-churn dependency logic isolated from CLI code
2. **Better organization**: Each file has a single responsibility
3. **Easier navigation**: Developers can find relevant code faster
4. **Independent evolution**: CLI, dependencies, and libraries can change separately
