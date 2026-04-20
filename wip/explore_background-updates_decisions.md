# Exploration Decisions: background-updates

## Round 1

- OS schedulers (cron, systemd, launchd) eliminated: require system footprint and
  lifecycle management that contradicts the project's no-daemon philosophy.
- Persistent daemon eliminated: same reason as OS schedulers.
- Detached subprocess (trigger.go pattern) confirmed as the mechanism: proven,
  working, already in the codebase, no new infrastructure needed.
- Registry "cache refresh" narrowed out of scope as a primary concern: the blocking
  is from `MaybeAutoApply` (tool installs), not from automatic registry refresh
  (which has no auto-trigger). `update-registry` is user-invoked and intentionally
  synchronous.
- Auto mode decision — Ready to crystallize: all five leads converge on the same
  root cause and solution direction; no major gaps remain.
