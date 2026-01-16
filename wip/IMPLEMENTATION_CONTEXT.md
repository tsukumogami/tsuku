# Implementation Context for Issue #919

## Summary

The metacpan version provider can list available versions but fails to resolve specific versions, falling back to "dev".

## Reproduction

```bash
# Lists versions correctly
./tsuku versions carton
# Available versions (39 total):
#   v1.0.901
#   v1.0.900
#   ...

# But eval fails to resolve
./tsuku eval --recipe internal/recipe/recipes/c/carton.toml --os linux --arch amd64 --version 1.0.901
# Warning: version resolution failed: version 1.0.901 not found for distribution Carton, using 'dev'
```

## Impact

- Cannot generate valid golden files for carton recipe
- The golden file version field shows "dev" instead of the actual version
- This causes golden file validation to fail

## Expected Behavior

When a version like `1.0.901` is passed to `--version`, the resolver should find the corresponding release on metacpan and use it.

## Root Cause Investigation Needed

The metacpan provider's version listing and version resolution may use different API endpoints or have different version normalization logic.

## Affected Recipes

- carton (and potentially other metacpan-based recipes)

## Tier

Simple (no design doc reference)
