# Issue #927 Introspection: validate-golden.sh Update

## Summary

**Recommendation: Proceed**

All dependencies (#921-#926) have been closed. The `--pin-from` flag exists in `cmd/tsuku/eval.go` and constraint extraction/application infrastructure is complete for pip, go, cargo, npm, gem, and cpan ecosystems.

## Dependency Verification

| Issue | Title | Status | Verified |
|-------|-------|--------|----------|
| #921 | feat(eval): add constrained evaluation skeleton with pip support | Closed | Yes - `--pin-from` flag, `ExtractConstraints`, pip constraint application |
| #922 | feat(eval): add go_build constraint support | Closed | Yes - `extractGoConstraintsFromSteps`, `decomposeWithConstraints` in go_install.go |
| #923 | feat(eval): add cargo_install constraint support | Closed | Yes - `extractCargoConstraintsFromSteps`, `decomposeWithConstraints` in cargo_install.go |
| #924 | feat(eval): add npm_install constraint support | Closed | Yes - `extractNpmConstraintsFromSteps`, `decomposeWithConstraints` in npm_install.go |
| #925 | feat(eval): add gem_install constraint support | Closed | Yes - `extractGemConstraintsFromSteps` in constraints.go |
| #926 | feat(eval): add cpan_install constraint support | Closed | Yes - `extractCpanConstraintsFromSteps` in constraints.go |

## Infrastructure Verification

### 1. `--pin-from` Flag (cmd/tsuku/eval.go)

The flag exists at line 91:
```go
evalCmd.Flags().StringVar(&evalPinFrom, "pin-from", "", "Path to golden file for constrained evaluation")
```

Constraints are loaded and passed to plan generation (lines 259-279):
```go
var constraints *actions.EvalConstraints
if evalPinFrom != "" {
    var err error
    constraints, err = executor.ExtractConstraints(evalPinFrom)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: failed to extract constraints from %s: %v\n", evalPinFrom, err)
        exitWithCode(ExitGeneral)
    }
}
// ...
planCfg := executor.PlanConfig{
    // ...
    Constraints: constraints,
    // ...
}
```

### 2. EvalConstraints Structure (internal/actions/decomposable.go)

Complete structure with all six ecosystem fields:
- `PipConstraints map[string]string`
- `GoSum string`
- `CargoLock string`
- `NpmLock string`
- `GemLock string`
- `CpanMeta string`

### 3. Constraint Extraction (internal/executor/constraints.go)

All ecosystem extractors implemented:
- `extractPipConstraintsFromSteps` - parses `locked_requirements`
- `extractGoConstraintsFromSteps` - extracts `go_sum`
- `extractCargoConstraintsFromSteps` - extracts `lock_data`
- `extractNpmConstraintsFromSteps` - extracts `package_lock`
- `extractGemConstraintsFromSteps` - extracts `lock_data`
- `extractCpanConstraintsFromSteps` - extracts `snapshot`

### 4. Constraint Application (internal/actions/*.go)

`decomposeWithConstraints` methods exist in:
- `pipx_install.go` - applies PipConstraints
- `go_install.go` - applies GoSum
- `cargo_install.go` - applies CargoLock
- `npm_install.go` - applies NpmLock

**Note**: `gem_exec.go` and `cpan_install.go` have extraction support but no `decomposeWithConstraints` method. However, per the design document, #925 and #926 are "simple" tier issues that only added extraction. The application methods were likely deemed unnecessary since:
1. These ecosystems may not have recipes with golden files currently
2. The validation script update (#927) will work for the primary ecosystems (pip, go, cargo, npm)

## Current State of validate-golden.sh

The script currently:
1. Builds tsuku if not present
2. Parses platforms and versions from golden files
3. Generates plans using `--recipe`, `--os`, `--arch`, `--version`, `--install-deps` flags
4. Strips `generated_at` and `recipe_source` with jq
5. Compares via SHA256 hash then shows diff on mismatch

**Key change needed**: Add `--pin-from $GOLDEN` to the eval command to enable constrained evaluation.

## Required Changes

Per Phase 6 of the design document:

1. **Modify validate-golden.sh**: Change the eval command from:
   ```bash
   "$TSUKU" eval "${eval_args[@]}" 2>/dev/null | \
       jq 'del(.generated_at, .recipe_source)' > "$ACTUAL"
   ```
   To:
   ```bash
   "$TSUKU" eval "${eval_args[@]}" --pin-from "$GOLDEN" 2>/dev/null | \
       jq 'del(.generated_at, .recipe_source)' > "$ACTUAL"
   ```

2. **Remove structural validation**: Per design, exact comparison is now sufficient (no schema extraction needed)

3. **Update validate-all-golden.sh**: May need minor updates to messaging

## Potential Issues

1. **Gem/CPAN recipes**: If there are gem_exec or cpan_install recipes with golden files, constrained evaluation will extract but not apply constraints. This could cause mismatches.
   - **Mitigation**: Check if any golden files use these ecosystems. If so, either skip them or implement application methods.

2. **Field stripping**: Current script strips `generated_at` and `recipe_source`. Need to verify no other fields need stripping.

## Blocking Concerns

**None identified.** The infrastructure is complete for the primary ecosystems. The script change is straightforward.

## Files to Modify

| File | Change |
|------|--------|
| `scripts/validate-golden.sh` | Add `--pin-from $GOLDEN` to eval command |
| `scripts/validate-all-golden.sh` | Update messaging (optional) |

## Verification Plan

1. Run `./scripts/validate-golden.sh httpie` (pip ecosystem)
2. Run `./scripts/validate-golden.sh gh` (go ecosystem)
3. Run `./scripts/validate-golden.sh ripgrep` (cargo ecosystem)
4. Run `./scripts/validate-all-golden.sh` for full suite
