# CI Rules for Recipe Batch PRs

These rules apply to all PRs in the system library backfill effort.

## Rule 1: Local validation before pushing

Before pushing any recipe changes (new or modified) to trigger CI,
run them locally against all Linux families on amd64. Use the sandbox
test infrastructure to validate every recipe in the PR passes on
debian, rhel, alpine, and suse.

Only push when we know the recipes pass locally across all families.
No "push and see what happens."

### How to run local tests

**Setup:**

```bash
# Build the binary (if stale)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o tsuku-linux-amd64 ./cmd/tsuku

# Source credentials to avoid GitHub API rate limiting
source .local.env
```

**Test a single recipe in one family:**

```bash
FAMILY_HOME="/tmp/tsuku-debian-myrecipe"
mkdir -p "$FAMILY_HOME"
TSUKU_HOME="$FAMILY_HOME" TSUKU_TELEMETRY=0 GH_TOKEN="$GH_TOKEN" \
./tsuku-linux-amd64 install --sandbox --force \
  --dangerously-suppress-security \
  --recipe ./recipes/m/myrecipe.toml \
  --target-family debian \
  --json
rm -rf "$FAMILY_HOME"
```

**Test all recipes across all families in parallel:**

Launch one agent per family (debian, rhel, alpine, suse), each testing
all recipes sequentially. The agents run in parallel so we get 4x
throughput. Always pass `GH_TOKEN` to avoid the unauthenticated
60 req/hr GitHub API rate limit (authenticated gets 5,000 req/hr).

Key flags:
- `--sandbox`: run in isolated container
- `--target-family FAMILY`: debian, rhel, alpine, or suse
- `--json`: structured output for parsing
- `--force`: skip interactive prompts
- `--dangerously-suppress-security`: skip symlink/permission checks

Environment variables:
- `TSUKU_HOME`: isolated directory per test (prevents state conflicts)
- `TSUKU_TELEMETRY=0`: disable telemetry
- `GH_TOKEN`: GitHub token for API rate limits (REQUIRED)

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
