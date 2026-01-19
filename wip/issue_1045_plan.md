# Issue 1045 Implementation Plan

## Summary

Add `--require-embedded` flag to `eval` and `install` commands that restricts action dependency loading to embedded recipes only.

## Approach

The flag needs to flow through to `recipe.LoaderOptions{RequireEmbedded: true}` when loading dependencies. There are two main code paths:

1. **eval command** → `PlanConfig` → `plan_generator.go:generateSingleDependencyPlan` → `cfg.RecipeLoader.GetWithContext()`
2. **install command** → `install_deps.go:installWithDependencies` → uses `loader.Get()` and doesn't call `ResolveTransitive` directly

For the eval command, we need to:
1. Add `RequireEmbedded bool` to `PlanConfig`
2. Pass it through to `GetWithContext` in `generateSingleDependencyPlan`

For the install command, the requirement is simpler - the flag just needs to be exposed for now. The actual enforcement happens when the loader is called with `RequireEmbedded: true`.

## Files to Modify

| File | Changes |
|------|---------|
| `cmd/tsuku/eval.go` | Add `--require-embedded` flag, pass to PlanConfig |
| `cmd/tsuku/install.go` | Add `--require-embedded` flag |
| `internal/executor/plan_generator.go` | Add `RequireEmbedded` field to `PlanConfig`, use in `generateSingleDependencyPlan` |

## Implementation Steps

### Step 1: Add RequireEmbedded to PlanConfig

Update `internal/executor/plan_generator.go`:
```go
type PlanConfig struct {
    // ... existing fields
    // RequireEmbedded restricts action dependency loading to embedded recipes only.
    // When true, dependency resolution fails if a dependency is not in the embedded registry.
    RequireEmbedded bool
}
```

### Step 2: Use RequireEmbedded in generateSingleDependencyPlan

Update line 701 in `plan_generator.go`:
```go
depRecipe, err := cfg.RecipeLoader.GetWithContext(ctx, depName, recipe.LoaderOptions{
    RequireEmbedded: cfg.RequireEmbedded,
})
```

### Step 3: Add flag to eval command

Add to `cmd/tsuku/eval.go`:
```go
var evalRequireEmbedded bool

func init() {
    // ... existing flags
    evalCmd.Flags().BoolVar(&evalRequireEmbedded, "require-embedded", false,
        "Require action dependencies to resolve from embedded registry")
}
```

Update `runEval` to pass to PlanConfig:
```go
planCfg := executor.PlanConfig{
    // ... existing fields
    RequireEmbedded: evalRequireEmbedded,
}
```

### Step 4: Add flag to install command

Add to `cmd/tsuku/install.go`:
```go
var installRequireEmbedded bool

func init() {
    // ... existing flags
    installCmd.Flags().BoolVar(&installRequireEmbedded, "require-embedded", false,
        "Require action dependencies to resolve from embedded registry")
}
```

Note: For install, we need to pass this through to the install flow. Let me trace where that would go.

## Test Strategy

1. Verify `--require-embedded` flag is recognized by both commands
2. Verify help text displays correctly
3. Run existing tests to ensure no regressions
4. Manual test: `tsuku eval gofumpt --require-embedded` should work (go/rust are embedded)

## Notes

- The flag defaults to `false` to preserve existing behavior
- CI will use this flag to validate embedded recipe completeness (in #1048)
