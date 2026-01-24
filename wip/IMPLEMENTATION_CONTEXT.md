## Goal

Add CI validation to ensure registry recipes are placed in the directory matching their first letter.

## Context

The `validate-recipe-structure.yml` workflow validates that registry recipes use letter subdirectories, but doesn't verify that recipes are in the *correct* directory. For example, `recipes/a/fzf.toml` would pass validation even though `fzf` should be in `recipes/f/`.

This could lead to confusion and broken lookups if recipes are misplaced.

## Acceptance Criteria

- [ ] CI fails if a recipe file's basename doesn't start with its parent directory letter
- [ ] Error message clearly identifies misplaced recipes and their expected location
- [ ] Check added to existing `validate-recipe-structure.yml` workflow

## Validation

```bash
#!/usr/bin/env bash
set -euo pipefail

# Create a misplaced recipe
mkdir -p recipes/a
echo 'name = "fzf"' > recipes/a/fzf.toml

# Run the validation step (should fail)
# Extract and run the relevant step from validate-recipe-structure.yml

# Clean up
rm recipes/a/fzf.toml
rmdir recipes/a 2>/dev/null || true
```

## Dependencies

None
