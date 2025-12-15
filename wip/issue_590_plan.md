# Issue 590 Implementation Plan

## Summary
Implement deterministic bottle inspection in the HomebrewBuilder, falling back to LLM only when validation fails.

## Approach

### Current Flow
```
formula JSON → LLM guesses → validate → LLM repairs (if fail) → recipe
```

### New Flow
```
formula JSON → inspect bottle → deterministic recipe → validate
    ↓ (if validation fails)
LLM repairs → validate → recipe
```

The key insight is that we can determine everything deterministically:
- **Dependencies**: Already available from Homebrew JSON API
- **Binary names**: Discover by inspecting bottle `bin/` directory
- **Verify command**: Pattern-based (`<binary> --version`)

## Implementation Steps

### Step 1: Add bottle inspection functions to HomebrewBuilder
- [ ] Add `downloadBottleToTemp` - downloads bottle to temp directory
- [ ] Add `extractBottleTarball` - extracts bottle tarball
- [ ] Add `listBottleBinaries` - lists files in bin/ directory
- [ ] Refactor `inspectBottle` to actually download and inspect bottle contents

### Step 2: Add deterministic recipe generation
- [ ] Add `generateDeterministicRecipe` method that:
  1. Fetches formula info (already exists)
  2. Downloads and inspects bottle to get binary names
  3. Generates verify command from binary name
  4. Creates recipe without LLM

### Step 3: Update Generate flow
- [ ] Modify `generateBottle` to try deterministic generation first
- [ ] Run sandbox validation on deterministic recipe
- [ ] Fall back to LLM if validation fails
- [ ] Track whether LLM was used for telemetry

### Step 4: Add tests
- [ ] Test `listBottleBinaries` with known formula
- [ ] Test deterministic generation for a simple formula (e.g., jq, ripgrep)
- [ ] Test fallback to LLM when deterministic fails

## Files to Modify

- `internal/builders/homebrew.go` - Add inspection functions, modify generate flow
- `internal/builders/homebrew_test.go` - Add tests for new functionality

## Key Decisions

1. **Reuse existing GHCR code**: The `HomebrewAction` already has `getGHCRToken`, `getBlobSHA`, `downloadBottle` - we can reuse this logic or create similar functions in the builder.

2. **Temp directory for inspection**: Download bottle to temp dir, extract, list bin/, then clean up.

3. **Fallback strategy**: If deterministic generation produces invalid recipe (fails sandbox), then invoke LLM to repair.

4. **Verify command pattern**: Use `<binary> --version` as default pattern. Most CLI tools support this.

## Success Criteria

- [ ] `tsuku create foo --from=homebrew:bar` works without LLM for most formulas
- [ ] LLM is only invoked when validation fails
- [ ] All existing tests pass
- [ ] New tests for deterministic path
