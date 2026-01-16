# Issue 943 Implementation Plan

## Summary

Add library type detection to the `verify` command with new flags for library-specific verification options.

## Files to Modify

1. `cmd/tsuku/verify.go` - Add flags and library detection/routing

## Implementation Steps

### Step 1: Add command flags

Add two new flags to the verify command:

```go
var (
    verifyIntegrityFlag  bool // --integrity
    verifySkipDlopenFlag bool // --skip-dlopen
)

func init() {
    verifyCmd.Flags().BoolVar(&verifyIntegrityFlag, "integrity", false, "Enable checksum verification for libraries")
    verifyCmd.Flags().BoolVar(&verifySkipDlopenFlag, "skip-dlopen", false, "Skip dlopen load testing for libraries")
}
```

### Step 2: Add LibraryVerifyOptions type

Define options struct for library verification:

```go
// LibraryVerifyOptions controls library verification behavior
type LibraryVerifyOptions struct {
    CheckIntegrity bool // Enable Level 4: checksum verification
    SkipDlopen     bool // Disable Level 3: dlopen load testing
}
```

### Step 3: Add library detection and routing

After loading the recipe, check if it's a library and route appropriately:

```go
if r.IsLibrary() {
    // Verify library instead of tool
    if err := verifyLibrary(toolName, state, r, cfg); err != nil {
        fmt.Fprintf(os.Stderr, "Library verification failed: %v\n", err)
        exitWithCode(ExitVerifyFailed)
    }
    printInfof("%s is working correctly\n", toolName)
    return
}
```

### Step 4: Implement stub verifyLibrary function

Create the library verification function that:
1. Looks up library in state.Libs (not state.Installed)
2. Gets the library directory from config
3. Verifies the directory exists
4. Prints stub output

```go
func verifyLibrary(name string, state *install.State, r *recipe.Recipe, cfg *config.Config) error {
    // Look up library in state.Libs
    libVersions, ok := state.Libs[name]
    if !ok {
        return fmt.Errorf("library '%s' is not installed", name)
    }

    // Get first version (libraries typically have one active version)
    var version string
    var libState install.LibraryVersionState
    for v, ls := range libVersions {
        version = v
        libState = ls
        break
    }

    libDir := cfg.LibDir(name, version)

    printInfof("Verifying library %s (version %s)...\n", name, version)

    // Verify directory exists
    if _, err := os.Stat(libDir); os.IsNotExist(err) {
        return fmt.Errorf("library directory not found: %s", libDir)
    }

    printInfo("  Library directory exists\n")
    printInfo("  (Full verification not yet implemented)\n")

    // Use libState for future integrity verification
    _ = libState

    return nil
}
```

### Step 5: Update command help text

Update the Long description to mention library verification:

```go
Long: `Verify that an installed tool or library is working correctly.

For tools (visible):
  1. Running the recipe's verification command
  2. Checking that the tool's bin directory is in PATH
  3. Verifying PATH resolution finds the correct binary
  4. Checking binary integrity against stored checksums

For tools (hidden):
  Only the verification command is run.

For libraries (--integrity, --skip-dlopen flags):
  Verifies the library directory exists.
  Full verification (header validation, dependency checking, dlopen testing)
  will be implemented in future updates.`,
```

## Testing Strategy

Since there's no existing verify_test.go, the testing approach focuses on:

1. Build verification (`go build`)
2. Manual testing with a library recipe
3. Verify existing tool verification still works

## Risk Assessment

- **Low risk**: Changes are additive (new code path for libraries)
- **No impact on existing tools**: Tool verification path unchanged
- **Stub implementation**: Full verification logic deferred to downstream issues
