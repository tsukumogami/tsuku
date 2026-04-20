# Explore Scope: background-updates

## Visibility

Public

## Core Question

Tsuku's auto-update and cache-refresh operations block commands the user actually
asked for, making the tool feel broken. We need to understand the full shape of the
blocking problem and what approach — background processes, deferred work, scheduling,
or something else — best eliminates the wait without sacrificing reliability or
adding system footprint.

## Context

Users experience waits up to a minute when running commands like `tsuku install`
while tsuku performs cache refreshes or update checks first. Tsuku already has a
notification system that may be a natural channel for surfacing background activity
status. A lighter footprint is preferred over persistent daemons or system services,
but tradeoffs are worth evaluating. The right UX and design are explicitly unknown.

## In Scope

- Identifying which commands trigger blocking update/refresh operations
- Understanding the existing notification mechanism
- Evaluating background execution patterns (detached processes, goroutines, OS scheduling)
- How peer CLI tools handle non-blocking update checks
- Platform implications for background process spawning (Linux, macOS, Windows)

## Out of Scope

- Changes to the recipe format or version providers
- Telemetry or website changes
- Network-level optimizations (CDN, caching headers)

## Research Leads

1. **Which tsuku commands trigger blocking update or cache-refresh operations, and for how long?**
   Map where in the codebase update checks and registry refreshes are initiated, whether
   they block the main command, and what the timing looks like. This grounds the rest of
   the exploration in concrete facts about what needs to change.

2. **How does tsuku's existing notification system work, and could it carry background activity status?**
   The user noted we already have a notification mechanism. Understand its model (push/pull,
   per-command, persistent), what it currently surfaces, and whether it's a natural fit for
   "update running in background" or "cache refreshed since last run" messages.

3. **What background or async execution patterns already exist in the codebase?**
   Find any goroutines, deferred work, or detached subprocesses already in use. Knowing
   what patterns are established makes it easier to evaluate what a new approach would need
   to introduce versus extend.

4. **How do peer CLI tools (brew, npm, rustup, gh, etc.) handle non-blocking update checks?**
   Several mature CLI tools have solved this. Document their patterns — what they defer, how
   they communicate status, what footprint they require — so we can learn from their tradeoffs.

5. **What are the platform and process-model implications of spawning background work from a CLI?**
   Detached process spawning, goroutine lifetime, OS-level scheduling, and shell interaction
   all behave differently on Linux, macOS, and Windows. Understand the constraints before
   committing to a mechanism.
