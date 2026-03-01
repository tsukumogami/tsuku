# CI Rules for Recipe Batch PRs

These rules apply to all PRs in the system library backfill effort.

## Rule 1: Local validation before pushing

Before pushing any recipe changes (new or modified) to trigger CI,
run them locally against all Linux families on amd64. Use the sandbox
test infrastructure to validate every recipe in the PR passes on
debian, rhel, alpine, and suse.

Only push when we know the recipes pass locally across all families.
No "push and see what happens."

## Rule 2: Don't reset CI for small fixes

Once CI is more than 30% complete and we identify a failure:

1. Investigate and fix locally.
2. Run the fix through local Linux family tests (same as Rule 1).
3. Calculate what percentage of recipes in the PR are affected by the
   fix.
4. Only push if the cumulative affected percentage exceeds 30%.
5. Otherwise, wait for CI to fully complete, then push all accumulated
   fixes at once.

This is cumulative across multiple failures discovered during a single
CI run. Example:

- Failure A affects 10% of recipes -- don't push, keep fixing locally.
- Failure B affects 20% more -- cumulative is 30%, now push.

The goal is to stop wasting CI cycles by resetting runs that are
mostly complete for fixes that only affect a small fraction of recipes.

## Rule 3: Never remove real dependencies

If a recipe has `runtime_dependencies` or `extra_dependencies`, those
declare real needs. If they cause CI failures, investigate the root
cause. Never strip dependencies to make CI pass -- that hides the
problem and ships broken recipes. If a recipe can't be fixed in the
current PR, revert the entire recipe to its main version and defer it
to a later PR.
