# Documentation Plan: pipeline-dashboard-overhaul

Generated from: docs/designs/DESIGN-pipeline-dashboard-overhaul.md
Issues analyzed: 7
Total entries: 1

---

## doc-1: docs/runbooks/batch-operations.md
**Section**: 1. Batch Success Rate Drop
**Prerequisite issues**: #1927, #1929, #1930
**Update type**: modify
**Status**: updated
**Details**: Update the investigation step 1 description of the pipeline dashboard to reflect the new three-widget layout (Pipeline Health, Ecosystem Health, Ecosystem Pipeline) instead of the single combined health panel. Mention that circuit breaker probes now prefer pending entries for half-open recovery, which makes the "automatic recovery in progress, monitor" guidance more reliable. Update the description of what operators see on the dashboard: per-ecosystem circuit breaker details are now in the Ecosystem Health widget, per-ecosystem queue counts are in the Ecosystem Pipeline widget, and timestamps display in ET. Also update the circuit breaker recovery description in the Resolution section to note that half-open probes bypass per-entry backoff and prefer untried entries, making self-recovery more effective when pending entries exist.
