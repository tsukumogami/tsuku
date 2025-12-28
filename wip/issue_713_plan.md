# Implementation Plan: Issue #713

## Goal

Add `--version <ver>` flag to `tsuku eval` to specify the version when using `--recipe` mode.

## Analysis

### Current Behavior

The `eval` command has two modes:

1. **Registry mode**: `tsuku eval <tool>[@version]` - loads recipe from registry, supports inline version via `@` syntax
2. **Recipe mode**: `tsuku eval --recipe <path>` - loads recipe from local file, does NOT support version specification

In registry mode (lines 137-147 of eval.go):
```go
if strings.Contains(toolName, "@") {
    parts := strings.SplitN(toolName, "@", 2)
    toolName = parts[0]
    reqVersion = parts[1]
}
```

The `reqVersion` variable is used when creating the executor (lines 193-197):
```go
if reqVersion != "" {
    exec, err = executor.NewWithVersion(r, reqVersion)
} else {
    exec, err = executor.New(r)
}
```

### Key Code Locations

| Location | Role |
|----------|------|
| `cmd/tsuku/eval.go` | Command definition, flag handling, version parsing |
| `internal/executor/executor.go` | `NewWithVersion()` creates executor with requested version |
| `internal/executor/plan_generate.go` | `GeneratePlan()` resolves version and generates plan |

### Existing Pattern

The command already has four flags defined using the same pattern:
```go
func init() {
    evalCmd.Flags().StringVar(&evalOS, "os", "", "Target operating system (linux, darwin)")
    evalCmd.Flags().StringVar(&evalArch, "arch", "", "Target architecture (amd64, arm64)")
    evalCmd.Flags().BoolVar(&evalYes, "yes", false, "Auto-accept installation of eval-time dependencies")
    evalCmd.Flags().StringVar(&evalRecipePath, "recipe", "", "Path to a local recipe file (for testing)")
}
```

## Implementation Approach

### Approach: Add `--version` flag for recipe mode

Add a new flag `--version` that works specifically with `--recipe` mode. This is cleaner than trying to reuse the `@version` syntax (which doesn't make sense for file paths).

**Trade-offs:**
- Pro: Clean separation of concerns - `--version` for recipe mode, `@` for registry mode
- Pro: Follows existing flag pattern (`--os`, `--arch`, etc.)
- Pro: Self-documenting - the flag name clearly indicates its purpose
- Con: Slight redundancy - version can be specified two ways (but in different modes)

**Mutual Exclusivity:**
- `--version` without `--recipe` should produce an error (version only makes sense with recipe mode)
- In registry mode, users should continue using `tool@version` syntax

## Implementation Steps

### Step 1: Add flag variable and registration

**File**: `cmd/tsuku/eval.go`

Add a new package-level variable and register the flag in `init()`:

```go
// After line 35
var evalVersion string

// In init(), after line 73
evalCmd.Flags().StringVar(&evalVersion, "version", "", "Version to use (only with --recipe)")
```

### Step 2: Add mutual exclusivity validation

**File**: `cmd/tsuku/eval.go`

In `runEval()`, add validation after the existing mutual exclusivity checks (after line 119):

```go
// Validate --version requires --recipe
if evalVersion != "" && evalRecipePath == "" {
    printError(fmt.Errorf("--version flag requires --recipe flag"))
    exitWithCode(ExitUsage)
}
```

### Step 3: Use the version in recipe mode

**File**: `cmd/tsuku/eval.go`

In the recipe mode branch (around line 134), set `reqVersion` from the flag:

```go
if evalRecipePath != "" {
    // Recipe file mode: load from local file using shared function
    var err error
    r, err = loadLocalRecipe(evalRecipePath)
    if err != nil {
        printError(fmt.Errorf("failed to load recipe: %w", err))
        exitWithCode(ExitGeneral)
    }
    recipeSource = evalRecipePath
    reqVersion = evalVersion  // Use version from flag
} else {
    // Registry mode: existing behavior (unchanged)
    ...
}
```

### Step 4: Update command documentation

**File**: `cmd/tsuku/eval.go`

Update the `Long` description to document the new flag usage (around line 56):

```go
Use --recipe with --version to evaluate at a specific version:
  tsuku eval --recipe ./my-recipe.toml --version v1.2.0
  tsuku eval --recipe /path/to/recipe.toml --version v1.2.0 --os darwin --arch arm64
```

Update examples section (around line 64):

```go
Examples:
  tsuku eval kubectl
  tsuku eval kubectl@v1.29.0
  tsuku eval ripgrep --os linux --arch arm64
  tsuku eval netlify-cli --yes
  tsuku eval --recipe ./my-recipe.toml --os darwin --arch arm64
  tsuku eval --recipe ./my-recipe.toml --version v1.2.0
```

### Step 5: Add unit tests

**File**: `cmd/tsuku/eval_test.go`

Add test cases for the new validation:

```go
func TestEvalVersionFlagValidation(t *testing.T) {
    tests := []struct {
        name        string
        version     string
        recipePath  string
        wantErr     bool
        errContains string
    }{
        {
            name:       "version with recipe is valid",
            version:    "v1.2.0",
            recipePath: "./test.toml",
            wantErr:    false,
        },
        {
            name:        "version without recipe is invalid",
            version:     "v1.2.0",
            recipePath:  "",
            wantErr:     true,
            errContains: "--version flag requires --recipe",
        },
        {
            name:       "no version with recipe is valid",
            version:    "",
            recipePath: "./test.toml",
            wantErr:    false,
        },
    }
    // ... test implementation
}
```

## Verification Plan

### Manual Testing

1. Build tsuku:
   ```bash
   go build -o tsuku ./cmd/tsuku
   ```

2. Test valid usage:
   ```bash
   # Should generate plan for v0.46.0
   ./tsuku eval --recipe internal/recipe/recipes/f/fzf.toml --version 0.46.0

   # Verify version in output
   ./tsuku eval --recipe internal/recipe/recipes/f/fzf.toml --version 0.46.0 | jq .version
   # Expected: "0.46.0"
   ```

3. Test error cases:
   ```bash
   # Should error: --version requires --recipe
   ./tsuku eval fzf --version v1.0.0
   # Expected: Error: --version flag requires --recipe flag

   # Should still work: registry mode with @version
   ./tsuku eval fzf@0.46.0
   ```

4. Test with other flags:
   ```bash
   # Should work: version + os + arch
   ./tsuku eval --recipe internal/recipe/recipes/f/fzf.toml --version 0.46.0 --os linux --arch arm64
   ```

### Unit Tests

```bash
go test ./cmd/tsuku -run TestEvalVersionFlagValidation -v
```

### Integration Verification

```bash
# Verify the version appears in the plan output
./tsuku eval --recipe internal/recipe/recipes/f/fzf.toml --version 0.46.0 | jq '{version: .version, tool: .tool}'
```

## Files Modified

| File | Change |
|------|--------|
| `cmd/tsuku/eval.go` | Add `--version` flag, validation, and documentation |
| `cmd/tsuku/eval_test.go` | Add tests for version flag validation |

## Rollback Plan

If issues arise, revert the changes to `eval.go` and `eval_test.go`. The flag is additive and does not modify existing behavior when unused.

## Acceptance Criteria Mapping

| Criteria | Implementation |
|----------|----------------|
| `tsuku eval --recipe <path> --version <ver>` generates plan for specified version | Step 3: Set `reqVersion = evalVersion` in recipe mode |
| Version is reflected in plan output (tool_version field) | Automatic: `executor.NewWithVersion()` propagates version to plan |
| Invalid version handling produces clear error | Automatic: `executor.resolveVersionWith()` returns version resolution errors |
| Flag works in combination with `--os` and `--arch` | No changes needed: flags are independent, combined in `GeneratePlan()` |
