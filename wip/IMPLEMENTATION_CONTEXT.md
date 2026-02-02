## Problem

`tsuku search` only searches cached registry recipes, ignoring embedded recipes. This means users searching for tools that ship with tsuku get told they don't exist.

## Reproduction

```bash
tsuku search go
```

**Expected**: Results include `go` (embedded recipe)
**Actual**: "No cached recipes found for 'go'."

Meanwhile, other commands handle embedded recipes correctly:
- `tsuku recipes` lists go as `[embedded]`
- `tsuku info go` shows recipe details
- `tsuku install go` finds the recipe

## Impact

Users who search before installing will be told a tool doesn't exist, even though it's available. This creates a confusing experience where `search` and `info`/`install` disagree.

## Environment

- Built from source (main branch, commit cfe5cae)
- Platform: linux/amd64
- Isolated environment (setup-env.sh)
