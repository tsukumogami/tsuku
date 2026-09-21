# Frozen snapshot: label references at 2026-09-20

The eleven workflow files here are copies of `.github/workflows/` as it stood on
2026-09-20, before the label manifest bootstrap created the eight missing label names.
`labels.yml` lists the 34 labels the repository had at that moment.

Run against this pair, `label-references.py` reports **nine undeclared names across
fifteen call sites** and exits 1. That is the finding the check was built to make, and
freezing it here is what keeps the finding reproducible now that the repair has landed —
otherwise the criterion could only ever be demonstrated once, against a repository state
that no longer exists.

**Do not edit these files.** They are not inputs to anything that runs; changing them to
match a later version of a workflow would destroy the only evidence that the check
detects the condition it was written for. When a workflow changes, the live file changes
and this copy does not.

The set is the ten workflows that referenced a label at that date, plus
`container-build.yml`, which is here because its `labels:` value is wholly a `${{ }}`
expression feeding an image build. The check must keep excluding it by the shape of the
value rather than by name, and the fixture is where that stays observable.
