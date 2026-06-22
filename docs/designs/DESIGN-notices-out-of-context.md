---
status: Planned
problem: |
  tsuku's deferred-notice system flushes all unshown success notices at the head of
  the next foreground command (PersistentPreRun -> renderUnshownNotices), rendering
  them identically to the current command's own output. A foreground command prints
  success lines for tools the user never named (the prior background auto-apply
  batch), and a foreground install/update success the user already saw inline is
  re-announced as a banner on the next, unrelated command. Observed on tsuku 0.12.1
  (issue #2422).
decision: |
  Two composable changes keyed on the single existing discriminator source ==
  SourceAuto. (D1) When the install lifecycle bus writes a SUCCESS notice for a
  foreground operation (not background auto-apply), mark it Shown=true at write time
  so it never re-surfaces as a head-of-output banner — it was already reported inline
  — while still appearing once in `tsuku notices` per #2379. (D2) For the success
  notices that legitimately remain at the head (background auto-apply, carrying
  KindAutoApplyResult, plus background self-update), group them under a single
  "Background updates applied:" section header with indented per-tool lines instead
  of bare lines, so they read as background/prior activity.
rationale: |
  The data to distinguish notice origin already exists (kindFor maps SourceAuto ->
  KindAutoApplyResult; everything else is foreground). D1 fixes the double-report at
  its source with a one-field change in the subscriber and preserves the #2379
  `tsuku notices` record (which keys on the notice file, not on Shown). D2 fixes
  attribution for the remaining head-of-output notices using the Kind already on
  disk, and is a pure-presentation change that composes with #2409's failure-notice
  header into one coherent head region. Both are small, reversible, and confined to
  internal/notices and internal/updates.
---

# DESIGN: Notices Out of Context

## Status

Planned

Resolves tsukumogami/tsuku#2422.

## Context and Problem Statement

`DisplayNotifications` runs from `PersistentPreRun` (`cmd/tsuku/main.go:78`) and
flushes all unshown success notices via `renderUnshownNotices`
(`internal/updates/notify.go:76`) *before* the current command does its own work.
`renderToolNotice` (`notify.go:159`) renders every success notice identically as
`"<tool> has been updated to <version>"`, regardless of `notices.Kind`. Two
user-visible defects result (both observed on tsuku 0.12.1, issue #2422):

1. **Out-of-context batch.** A foreground command prints success lines for tools
   the user never named — the deferred batch from a prior background auto-apply.
   `tsuku update niwa` printed six unrelated `updated/installed` lines before
   `✅ niwa@0.14.2`.
2. **Foreground op re-announced one command later.** A foreground `install`/`update`
   success the user already saw inline (`✅ tool@version`) reappears as a
   head-of-output banner on the next, unrelated command. `tsuku update koto`
   printed `shirabe has been installed (0.12.0)` — the previous command's work.

The data to distinguish notice origin already exists. The install lifecycle bus
publishes events with a `Source` (`internal/installevents`): `SourceManual`,
`SourceAuto`, `SourceProjectAuto`. The notices `Subscriber.Handle`
(`internal/notices/subscriber.go:33`) writes a notice per event; `kindFor`
(`subscriber.go:120`) maps `SourceAuto` to `KindAutoApplyResult` and everything else
to `KindUpdateResult` (`""`). Today every success event is written with
`Shown: false`, and the renderer ignores `Kind` for success framing.

This is the success-notice counterpart to the failure-notice placement work in
#2409 and the install-vs-update wording work in #2408; the three must compose.

## Decision Drivers

- **Attribution.** A reader must be able to tell which lines the current command
  produced and which are background/prior activity.
- **No double-reporting.** A foreground success already shown inline must not
  reappear as a banner on the next command.
- **Preserve the `tsuku notices` record (#2379).** Foreground manual update/install
  success notices were added deliberately so `tsuku notices` carries a record; the
  fix must not erase that record, only stop the misleading next-command banner.
  `tsuku notices` reads all notice files (`ReadAllNotices`) regardless of `Shown`,
  so marking a notice shown at write time keeps the record intact.
- **Compose with #2409 / #2408.** No regression to failure-notice placement or to
  the install-vs-update banner wording; shared renderer code paths.
- **Small, reversible change.** Confined to `internal/notices/subscriber.go` and
  `internal/updates/notify.go`; no new components, no schema change.

## Considered Options

### Decision 1 — Foreground success-notice lifecycle (stop the next-command re-banner)

| Option | Mechanism | Verdict |
|--------|-----------|---------|
| **A — write foreground success notices `Shown=true`** | In `Subscriber.Handle`, set `Shown` from the event `Source` for success events (Installed / Updated / RolledBack / LibraryInstalled): foreground sources write `Shown=true`; `SourceAuto` keeps `Shown=false`. `ReadUnshownNotices` (notices.go:145) then never returns them, so no head banner; `ReadAllNotices` still does, so `tsuku notices` is unchanged (#2379). | **Chosen.** One-field change at the single write site; preserves #2379; failures untouched (#2409). |
| B — don't write a foreground success notice at all | Drop the WriteNotice for foreground success events. | Rejected — regresses #2379 (`tsuku notices` loses the manual-op record). |
| C — new foreground Kind, skip foreground success at the banner stage | Add a Kind, branch in `renderUnshownNotices` to skip it. | Rejected — more surface (new constant + `kindFor` change + renderer branch + invariant-test churn) for the same outcome as A. |
| D — keep current behavior | — | Rejected — leaves the bug. |

### Decision 2 — Framing for notices that legitimately surface at the head

| Option | Mechanism | Verdict |
|--------|-----------|---------|
| **a — "Background updates applied:" section header** | In `renderUnshownNotices`, partition `unshown` into background-success (`Kind == KindAutoApplyResult`, plus background self-update) vs the rest. Emit one header line followed by indented per-tool child lines for the background set; render the rest as today. | **Chosen.** Reuses the Kind already on disk; matches the indented-block grammar already used for `KindVersionFallback`/`KindShellInitChange`; stacks with #2409's failure header into one head region. |
| b — per-line `(background)` prefix | Prefix each background line. | Rejected — noisy with several tools; prefix grammar composes weakly with #2409's header grammar. |
| c — neutral combined wording, no grouping | Reword without separating. | Rejected — no visual separation from command output; diverges from #2409. |
| d — suppress at the head, rely on `tsuku notices` | Don't render background success at the head at all. | Rejected — drops a user-relevant event the user never saw inline; asymmetric with #2409 keeping failures at the head. |
| e — group and relocate to PostRun tail | Move the block to `PersistentPostRun`. | Rejected — PostRun is skipped on command error (notice loss) and splits successes-at-tail from #2409 failures-at-head. |

## Decision Outcome

Adopt **D1 = Option A** and **D2 = Option a**. They compose on the single existing
discriminator `source == SourceAuto`:

- **Foreground success** (`SourceManual`, and project-auto where it produces inline
  output — see Open Question): `Shown=true` at write time → no head banner; appears
  once in `tsuku notices` then clears (#2379).
- **Background auto-apply success** (`SourceAuto`, `Kind == KindAutoApplyResult`):
  stays `Shown=false` → surfaces at the head, now grouped under
  "Background updates applied:".

After both changes, a foreground command shows only its own work inline; background
and prior activity appears under a clearly labelled header. The two sibling fixes
slot in: #2409 contributes a parallel failure header in the same head region, and
#2408's install-vs-update wording now only needs to handle the background banner
(foreground successes no longer banner at all) — flag the #2408 owner so a
foreground banner is not reintroduced.

### Open Question for /plan

`SourceProjectAuto` is the edge case. `kindFor` currently maps it to
`KindUpdateResult` (not `KindAutoApplyResult`), so today it would be treated as
foreground by both decisions. The plan must determine whether project-level
auto-apply produces inline output at the time it runs:
- If it runs silently (background-like), it should be `Shown=false` and grouped —
  meaning `kindFor` should map `SourceProjectAuto` to `KindAutoApplyResult` (or the
  partition predicate must include it).
- If it runs inline during a foreground command, the foreground (`Shown=true`)
  treatment is correct and no `kindFor` change is needed.
Resolve by tracing the `SourceProjectAuto` apply path before implementing.

## Solution Architecture

Two touch points, both already on the notice path; no new components, no notice
schema change.

```
install lifecycle bus (internal/installevents)
        │  Event{Source: Manual|Auto|ProjectAuto}
        ▼
notices.Subscriber.Handle           ── D1: set Shown from Source for SUCCESS events
  (internal/notices/subscriber.go)      foreground → Shown=true ; SourceAuto → Shown=false
        │  writes $TSUKU_HOME/notices/<tool>.json
        ▼
PersistentPreRun → DisplayNotifications → renderUnshownNotices
  (internal/updates/notify.go)        ── D2: partition unshown into
                                          [background-success] vs [rest];
                                          emit "Background updates applied:" header
                                          + indented child lines for the former
        │
        ▼
   stderr (TTY/CI/quiet-gated by ShouldSuppressNotifications)
```

- **D1 — `internal/notices/subscriber.go`.** Add a small helper
  `shownForSuccess(source) bool` (foreground sources → true; `SourceAuto` → false;
  `SourceProjectAuto` per the Open Question) and use it for the `Shown` field of the
  four success branches (`Installed`, `Updated`, `RolledBack`, `LibraryInstalled`).
  Failure branches (`writeFailure`) keep `Shown=false` unchanged so #2409 is
  untouched. Self-update flows through the same bus, so foreground `tsuku
  self-update` is covered automatically and background self-update keeps its
  `SourceAuto` head treatment.
- **D2 — `internal/updates/notify.go`.** In `renderUnshownNotices`, partition the
  `unshown` slice into background-success (`n.Error == "" && n.Kind ==
  KindAutoApplyResult`) and the rest. Emit the background set under one
  `"Background updates applied:"` header with indented child lines; render the rest
  with the existing per-notice switch. Keep all `MarkShown` / `RemoveNotice`
  bookkeeping unchanged. Factor the partition into a helper so #2409 can consume the
  same partition for its failure block.

## Implementation Approach

1. **D1 — subscriber `Shown` by source.** Add `shownForSuccess` and apply it to the
   four success branches in `subscriber.go`. Resolve the `SourceProjectAuto` Open
   Question first. Unit-test: foreground success notice is written `Shown=true`;
   `SourceAuto` success stays `Shown=false`; failures stay `Shown=false`.
2. **D2 — header grouping in renderer.** Add the partition helper and the
   "Background updates applied:" header + indented child rendering in
   `renderUnshownNotices`. Unit-test the renderer (seeded notice files → captured
   stderr) for: foreground success not shown at head; `SourceAuto` success grouped
   under the header; failures still rendered as today.
3. **End-to-end validation.** Reproduce the #2422 acceptance criteria with the
   seeded-notice validation script: a foreground command shows only its own work;
   a seeded `KindAutoApplyResult` success appears under the header.
4. **Sibling-fix note.** Leave a comment / issue note on #2408 and #2409 that the
   head region is now partitioned (background-success header here, failure header
   there) so the three fixes compose.

## Security Considerations

No new attack surface. Both changes operate on notice files already written and read
under `$TSUKU_HOME/notices/`; no new inputs, no new file paths, no new external
calls. Notice content is already sanitized (`sanitizeError`,
`subscriber.go:138`) and tool/version strings already flow to the renderer today —
D2 only changes their grouping and indentation, not their source or escaping. The
`Shown` flag is an internal persistence detail, not attacker-controlled. No secrets,
credentials, or privilege boundaries are involved. Security review outcome: **N/A —
presentation and persistence-flag change with no new surface.**

## Consequences

**Positive**
- Foreground commands show only their own work inline; the misleading
  out-of-context batch and the next-command re-banner are both gone.
- Background auto-apply activity is still surfaced (the user learns what was
  auto-updated) but is now clearly labelled as background.
- Reuses the existing `Source`/`Kind` data; no schema change, no new component.
- Composes with #2409 (failure header) and narrows #2408 to the background banner.

**Negative / risks**
- Two files change behavior that several tests assert against (notice `Shown`
  defaults; renderer output). Test churn is expected and bounded.
- The `SourceProjectAuto` mapping must be resolved correctly, or project-auto
  updates could either be hidden (if wrongly marked shown) or appear ungrouped.

**Mitigations**
- Resolve the `SourceProjectAuto` Open Question by tracing the apply path before
  implementing (step 1).
- Cover both decisions with the seeded-notice unit tests and the #2422 end-to-end
  validation script before merge.
