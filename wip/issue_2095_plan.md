# Issue 2095 Plan

## Problem
922 homebrew recipes have redundant `source = "homebrew"` and `formula` in `[version]`
that duplicate what the `[[steps]] action = "homebrew"` already infers.

## Approach
Write a script to clear `source` and `formula` in `[version]` sections of affected recipes.
The homebrew action infers these automatically.

## Steps
1. Script: for each recipe, if `[version] source = "homebrew"` AND a matching
   `[[steps]] action = "homebrew"` exists, set `source = ""` and `formula = ""`
2. Validate: run `tsuku validate --strict` on all modified recipes
3. Spot-check: verify a few recipes still resolve versions correctly
