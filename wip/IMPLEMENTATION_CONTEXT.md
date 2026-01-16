# Implementation Context: Issue #927

**Source**: docs/designs/DESIGN-non-deterministic-validation.md

## Issue Information

- **Title**: feat(scripts): update validate-golden.sh for constrained evaluation
- **Tier**: testable
- **Dependencies**: #921 (closed), #922 (closed), #923 (closed)

## Key Implementation Requirements

From the design document, Phase 6 (Validation Script Update):

1. **Modify validate-golden.sh**: Use `--pin-from` flag
2. **Exact comparison**: Constrained eval produces deterministic output
3. **Test full golden file suite**

## CI Validation Flow (from design)

```bash
#!/bin/bash
# validate-golden.sh <recipe>

RECIPE="$1"
GOLDEN_DIR="testdata/golden/plans"

for golden in "$GOLDEN_DIR"/*/"$RECIPE"/*.json; do
    version=$(jq -r '.version' "$golden")
    os=$(jq -r '.platform.os' "$golden")
    arch=$(jq -r '.platform.arch' "$golden")

    # Generate plan with constraints from golden file
    actual=$(mktemp)
    ./tsuku eval "$RECIPE@$version" \
        --pin-from "$golden" \
        --os "$os" \
        --arch "$arch" > "$actual"

    # Exact comparison - constrained eval should match golden
    if ! diff -q "$golden" "$actual" > /dev/null; then
        echo "MISMATCH: $golden"
        diff -u "$golden" "$actual"
        exit 1
    fi
done

echo "All golden files validated successfully"
```

## Key Files to Modify

- `scripts/validate-golden.sh` - Rewrite validation logic to use `--pin-from`
- `scripts/validate-all-golden.sh` - Update to use new validation

## What Gets Exercised

With constrained evaluation, validation exercises:
- Recipe TOML parsing
- Version provider calls
- Action Decompose()
- Template expansion
- Platform filtering
- Step ordering
- Determinism computation
- Constraint application

## Exit Criteria

- `validate-golden.sh` uses `--pin-from` flag for constrained evaluation
- Exact comparison (not structural) is used for all golden files
- All existing golden files pass validation with constrained eval
