# Documentation Plan: registry-versioning

Generated from: docs/plans/PLAN-registry-versioning.md
Issues analyzed: 3
Total entries: 1

---

## doc-1: README.md
**Section**: Cache Management
**Prerequisite issues**: Issue 1, Issue 2
**Update type**: modify
**Status**: updated
**Details**: Add a "Registry Compatibility" paragraph after the `update-registry` examples. Explain that the CLI validates the registry's schema version on every fetch or cache read. If the format is newer than the CLI supports, tsuku errors with a suggestion to upgrade. If the registry includes a deprecation notice for an upcoming format change, the CLI prints a one-time stderr warning with the timeline, required CLI version, and upgrade URL. The `--quiet` flag suppresses these warnings. Keep it to 3-4 sentences -- the error and warning messages are self-documenting, so the README note just needs to set expectations.
