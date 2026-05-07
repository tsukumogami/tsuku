# Explore Scope: update-warnings-notifications

## Visibility

Public

## Core Question

How should tsuku implement a context-aware notification routing system where the execution channel (interactive vs auto-update background) determines the sink (terminal output vs inbox persistence), with appropriate lifecycle semantics per notice type? The concrete trigger is issue #2386 (version fallback with no user-visible warning), but the goal is a platform capability that eliminates the current duplication between "write to terminal" and "write to notices file."

## Context

When tsuku runs in auto-update background mode there is no terminal, so any warning or error that would normally print inline is silently lost. When running interactively, inline output is shown but not persisted — meaning if the terminal scrolls or the user wasn't watching, important events vanish. The fix isn't two separate codepaths for each mode; it's a single notification API with a configurable sink. In auto-update mode the sink routes to the inbox; in interactive mode the sink routes to the terminal (and optionally also to the inbox for events that warrant persistence). Two notice types drive lifecycle: persistent errors (cleared only when the tool is updated, rolled back, or removed) and single-view notifications (cleared after the user views them in `tsuku notices`).

## In Scope

- Reporter/sink abstraction: how to configure the notification channel per execution context
- Notice taxonomy: which events are persistent errors, single-view notifications, or transient noise
- Version-fallback notification (#2386): the concrete first use case
- Lifecycle semantics for each notice type
- Consistency between explicit `tsuku update` and background auto-apply paths

## Out of Scope

- Rearchitecting the existing error-notice file format (extend, don't replace)
- Notifications for non-update events (install, remove, verify) — these can adopt the platform later
- Push notifications or external alerting

## Research Leads

1. **How does the existing reporter/progress system work, and where would a configurable sink abstraction fit without requiring all call sites to change?**
   The reporter pattern drives all update output (DeferWarn, Log, etc.). Understanding its interface is the key to knowing whether we can add a pluggable sink transparently.

2. **How does the auto-apply background path currently write notices, and what would the "inbox sink" path look like?**
   `apply.go` already writes failure notices. We need to understand how to generalize this so any notification-worthy event routes through a shared channel rather than being hardcoded in specific places.

3. **What should the notice taxonomy look like — which events are persistent errors, single-view notifications, or transient noise?**
   Not everything that prints inline warrants inbox persistence. This lead defines the behavioral contract the platform capability needs to enforce.

4. **What does the version-fallback code path look like specifically, and where would the notification emit happen?**
   Concrete first use case from #2386: when the version resolver skips a release with no asset and falls back, where in the code does that happen, and where should the notification call go?

5. **How do other CLI ecosystems handle context-aware notification routing — different output modes for background vs interactive execution?**
   External validation of the pattern and any gotchas (e.g., buffering, ordering, failure modes when the inbox sink is unavailable).
