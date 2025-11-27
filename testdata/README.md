# Test Data

This directory contains test recipes for manual testing that should NOT be embedded in the production binary.

## Testing Dependency Management

To test the dependency management features:

```bash
# 1. Copy test recipes to bundled folder temporarily
cp testdata/recipes/*.toml bundled/recipes/

# 2. Rebuild tsuku
go install ./cmd/tsuku

# 3. Run tests (see test scenarios below)

# 4. Clean up - remove test recipes from bundled folder
rm bundled/recipes/tool-a.toml bundled/recipes/tool-b.toml
```

## Test Scenarios

### Test 1: Auto-install dependencies
```bash
rm -rf ~/.tsuku
tsuku install tool-a
# Should install both tool-a and tool-b
tsuku list
cat ~/.tsuku/state.json
# Should show tool-b with is_explicit=false, required_by=["tool-a"]
```

### Test 2: Dependency protection
```bash
tsuku remove tool-b
# Should fail with: "Error: tool-b is required by: tool-a"
```

### Test 3: Orphan cleanup
```bash
tsuku remove tool-a
# Should auto-remove both tool-a and tool-b
tsuku list
# Should show no tools
```

### Test 4: Explicit install protection
```bash
rm -rf ~/.tsuku
tsuku install tool-b  # Explicitly install dependency first
tsuku install tool-a  # tool-b already present
tsuku remove tool-a   # Should remove only tool-a, tool-b remains
tsuku list
# Should show only tool-b
cat ~/.tsuku/state.json
# Should show tool-b with is_explicit=true
```
