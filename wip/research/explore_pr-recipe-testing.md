# PR Recipe Testing Research

## Summary

Tsuku uses **three distinct PR validation workflows** for recipes, each triggered by different file path patterns.

## Testing Workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `test-changed-recipes.yml` | `internal/recipe/recipes/**/*.toml` | Functional testing: Install recipes on Linux and macOS |
| `validate-golden-recipes.yml` | `internal/recipe/recipes/**/*.toml` | Plan validation: Verify recipe plans match golden files |
| `validate-golden-code.yml` | 35 specific Go files | Code impact: Ensure code changes don't break plan generation |
| `validate-golden-execution.yml` | `testdata/golden/plans/**/*.json` | Execution: Run stored plans and verify they execute |

## Changed Recipe Detection

**For recipe file changes:**
```bash
git diff --name-only --diff-filter=d origin/main...HEAD -- 'internal/recipe/recipes/**/*.toml'
```
- Uses `--diff-filter=d` to exclude deleted files
- Extracts recipe name from filename: `internal/recipe/recipes/f/fzf.toml` -> `fzf`

**For golden plan file changes:**
```bash
git diff --name-only --diff-filter=d origin/main...HEAD -- 'testdata/golden/plans/**/*.json'
```
- Parses platform from filename: `v1.0.0-linux-debian-amd64.json` -> `linux-debian-amd64`
- Extracts recipe name from directory: `testdata/golden/plans/f/fzf/` -> `fzf`

## Recipe Changes vs Code Changes

**Recipe file changes:**
- Triggers both `test-changed-recipes.yml` (functional) and `validate-golden-recipes.yml` (plan validation)
- Uses `validate-golden.sh` to regenerate and compare plans
- Filters recipes: skips library recipes and system dependencies
- Tests only changed recipes (not all)

**Code changes (35 tracked files):**
- Triggers `validate-golden-code.yml` which validates **ALL recipes with golden files**
- Does NOT trigger on execution-only files

**Golden plan file changes:**
- Triggers `validate-golden-execution.yml` (execution validation)
- Executes stored plans on supported platforms

## Filtering Logic

**In test-changed-recipes.yml (functional):**
```bash
# Skip library recipes
grep -q 'type = "library"' -> skip

# Skip system dependencies
grep -q 'action = "require_system"' -> skip

# Skip execution-excluded recipes
jq '.exclusions[] | select(.recipe == $tool)' execution-exclusions.json

# Skip Linux-only recipes for macOS testing
grep -q 'supported_os.*=.*\["linux"\]' -> linux_only=true
```

**In validate-golden-recipes.yml (plan validation):**
```bash
# Check if recipe has golden files
if [ ! -d "testdata/golden/plans/$letter/$recipe" ]; then
  # Allow if recipe is fully excluded; else error
fi
```

## Key Insight

On a PR, **only changed recipes are tested** - not all recipes. This is true for both functional testing and plan validation. The exception is when critical code changes, which triggers full recipe validation.
