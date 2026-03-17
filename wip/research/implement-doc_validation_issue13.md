# E2E Validation: Issue #13 - Distributed Recipes

**Date**: 2026-03-17
**Binary**: `/tmp/tsuku-test` (built 2026-03-17)
**Isolation**: `TSUKU_HOME` set to temp directory, `TSUKU_TELEMETRY=0`

## Blocking Dependency

PR #57 in `tsukumogami/koto` (adds `.tsuku-recipes/` directory to main branch) is still **OPEN**. This PR is a prerequisite for scenarios 2-7, which all depend on installing from a distributed recipe source. Until PR #57 is merged, only Scenario 1 can be validated.

## Results Summary

| # | Scenario | Status | Notes |
|---|----------|--------|-------|
| 1 | Registry list (empty) | PASS | Output: "No registries configured." Exit code: 0 |
| 2 | Install from distributed source | SKIP | PR #57 not merged; koto repo has no `.tsuku-recipes/` on main |
| 3 | Registry list (after install) | SKIP | Depends on scenario 2 |
| 4 | List shows source | SKIP | Depends on scenario 2 |
| 5 | Info shows source | SKIP | Depends on scenario 2 |
| 6 | Recipes shows distributed | SKIP | Depends on scenario 2 |
| 7 | Registry remove | SKIP | Depends on scenario 2 |

## Scenario Details

### Scenario 1: Registry list (empty) - PASS

```
$ tsuku registry list
No registries configured.
$ echo $?
0
```

The `registry list` subcommand exists, returns exit code 0, and produces the expected empty-state message. This confirms the distributed registry infrastructure is wired up in the CLI.

### Scenarios 2-7: SKIPPED

All remaining scenarios require `tsuku install tsukumogami/koto -y` to succeed, which needs the koto repo to have a `.tsuku-recipes/` directory on its main branch. PR #57 (`gh pr view 57 --repo tsukumogami/koto`) is still in OPEN state.

**Action needed**: Merge PR #57 in `tsukumogami/koto`, then re-run this validation.

## Counts

- **Pass**: 1
- **Fail**: 0
- **Skip**: 6
