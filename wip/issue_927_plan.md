# Implementation Plan: Issue #927

## Summary

Update `scripts/validate-golden.sh` to use constrained evaluation via `--pin-from` flag.

## Current State

The script (line 273-284) currently runs:
```bash
eval_args=(--recipe "$RECIPE_PATH" --os "$os" --arch "$arch" --version "$VERSION_NO_V" --install-deps)
# ...
"$TSUKU" eval "${eval_args[@]}" | jq 'del(.generated_at, .recipe_source)' > "$ACTUAL"
```

## Required Change

Add `--pin-from "$GOLDEN"` to the eval command to enable constrained evaluation:
```bash
eval_args=(--recipe "$RECIPE_PATH" --os "$os" --arch "$arch" --version "$VERSION_NO_V" --install-deps --pin-from "$GOLDEN")
```

## Why This Works

With constrained evaluation:
1. Constraints are extracted from the golden file (pip versions, go.sum, cargo.lock, etc.)
2. `tsuku eval` runs the full evaluation code path with these constraints
3. Output is deterministic because dependency versions are pinned
4. Exact comparison should match (not structural validation needed)

## Files to Modify

1. `scripts/validate-golden.sh` - Add `--pin-from "$GOLDEN"` to eval_args

## Testing

1. Run `./scripts/validate-golden.sh fzf` (deterministic recipe - should pass)
2. Run `./scripts/validate-golden.sh httpie` (pip recipe - now should pass with constrained eval)
3. Run `./scripts/validate-all-golden.sh` to validate all recipes

## Risks

- Some golden files may still fail if constraint application doesn't fully reproduce the output
- This will be caught by testing and can be addressed in follow-up issues
