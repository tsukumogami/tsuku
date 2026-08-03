# Crystallize: storage-plan-fields

## Artifact type: Design doc, then plan

**Complexity: Medium.** What to build is settled by #2468 and by the findings -- carry
six fields through the converter and guard the converter structurally. That part has
no design content worth a document.

One question does: what a read of a plan record written before the fix means. Three
viable answers, they disagree about a schema-invalidation cost that lands on 110
golden files, and the choice is close to irreversible once a `format_version` value is
published in a release. That is design-doc territory and it routes through the
decision framework first.

## Not a PRD

Requirements are not the gap. #2468 states them, the acceptance criteria are concrete,
and the findings sharpened rather than contested them. The one correction -- that the
cache-hit framing is overstated and `plan export` is the wide exposure -- changes which
test to write, not what to build.

## Not straight to work-on

The backfill question decides whether existing installs quietly keep the bug. Deciding
it inside an implementation session buries the reasoning in a commit message, and the
`format_version` half of it is not cheaply reversible.

## Decisions carried forward

- Six fields, not five. `Platform.LinuxFamily` joins the list.
- The load-bearing exposure is `plan export` -> `install --plan`, not the cache-hit
  reinstall. Tests target that path.
- `Phase`, `LinuxFamily`, and `Binaries` have correct zero values; only
  `Dependencies`, `Verify`, and `RecipeType` need a backfill answer.
- Restoring `Dependencies` does not unblock #2469. Say so; do not imply otherwise.
- The guard is a reflection field census plus a whole-value JSON round trip, not more
  assertions.

## Next

1. `shirabe:decision` on the pre-fix record question.
2. Design doc folding in the decision.
3. Plan.
