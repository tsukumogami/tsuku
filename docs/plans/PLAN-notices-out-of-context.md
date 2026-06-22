---
schema: plan/v1
status: Active
execution_mode: single-pr
upstream: docs/designs/DESIGN-notices-out-of-context.md
milestone: "Notices out-of-context fix"
issue_count: 3
---

# PLAN: Notices Out of Context

## Status

Active

## Scope Summary

Fix tsukumogami/tsuku#2422 in a single PR: stop deferred success notices from
surfacing out of context at the head of an unrelated foreground command. Two
composable changes keyed on the existing `source == SourceAuto` discriminator —
suppress the next-command re-banner for foreground successes (write-side) and group
the remaining background-success notices under a header (read-side).

## Decomposition Strategy

**Horizontal.** The fix splits into two loosely-coupled components with a stable
interface — the on-disk notice's `Shown` flag and `Kind`:

- **Write side** (`internal/notices/subscriber.go`): decide `Shown` from the event
  `Source` so foreground successes are never queued for the head banner.
- **Read side** (`internal/updates/notify.go`): group the background-success notices
  that do reach the head under a labelled section.

They meet only at the notice file's fields, so they build independently; a walking
skeleton adds no value for a two-file presentation/persistence fix. All three issues
land in one PR (P1: the whole fix is the unit of observable value — #2422 resolved).

## Issue Outlines

### Issue 1: fix(notices): mark foreground success notices Shown at write time

**Goal**: Foreground `install`/`update`/`self-update` successes (already reported
inline) no longer re-banner on the next command, while the `tsuku notices` record
(#2379) is preserved.

**Acceptance Criteria**:
- [ ] Resolve the `SourceProjectAuto` open question first: trace the project-level
      auto-apply path and document whether it produces inline output. Map it to
      foreground (`Shown=true`) if it runs inline, or background (`Shown=false`,
      and grouped per Issue 2 — adjust `kindFor`/the partition predicate) if silent.
- [ ] Add a `shownForSuccess(source)` helper in `internal/notices/subscriber.go`.
- [ ] The four success branches (`Installed`, `Updated`, `RolledBack`,
      `LibraryInstalled`) set `Shown` via the helper; `SourceManual` → `Shown=true`;
      `SourceAuto` → `Shown=false`; `SourceProjectAuto` → per the traced behavior.
- [ ] Failure branches (`writeFailure`) keep `Shown=false` (no #2409 regression).
- [ ] Unit tests: foreground success written `Shown=true`; `SourceAuto` success
      `Shown=false`; failures `Shown=false`.
- [ ] `tsuku notices` still shows a foreground success once then clears it (#2379
      preserved) — covered by a test reading via `ReadAllNotices`.
- [ ] `go test ./...`, `go vet ./...`, `gofmt`, `golangci-lint` pass.

**Dependencies**: None

**Type**: code
**Files**: `internal/notices/subscriber.go`, `internal/notices/subscriber_test.go`

### Issue 2: fix(notices): group background auto-apply success under a header

**Goal**: Deferred background-success notices that surface at the head of a later
command render under a single "Background updates applied:" section with indented
per-tool lines, visually distinct from the current command's own output.

**Acceptance Criteria**:
- [ ] `renderUnshownNotices` (`internal/updates/notify.go`) partitions `unshown`
      into background-success (`n.Error == "" && n.Kind == KindAutoApplyResult`) and
      the rest, via an extracted partition helper.
- [ ] Background-success notices render under one `Background updates applied:`
      header with indented child lines; the rest render via the existing switch.
- [ ] After Issue 1, foreground successes are absent from the head; a seeded
      `KindAutoApplyResult` success appears under the header.
- [ ] Failure notices render exactly as today (no #2409 regression); all
      `MarkShown` / `RemoveNotice` bookkeeping is unchanged.
- [ ] The partition helper is factored so #2409 can reuse it for its failure block.
- [ ] Renderer unit tests via seeded notice files → captured stderr cover: foreground
      success not at head, `SourceAuto` success under header, failures unchanged.
- [ ] `go test ./...`, `go vet ./...`, `gofmt`, `golangci-lint` pass.

**Dependencies**: Blocked by <<ISSUE:1>>

**Type**: code
**Files**: `internal/updates/notify.go`, `internal/updates/notify_test.go`

### Issue 3: test(notices): end-to-end #2422 validation and sibling-fix notes

**Goal**: Confirm the #2422 acceptance criteria end-to-end and record the now-
partitioned head region for the sibling fixes.

**Acceptance Criteria**:
- [ ] The #2422 seeded-notice validation script passes: a foreground command shows
      only its own work; a seeded `KindAutoApplyResult` success appears framed under
      the header.
- [ ] Add a note to #2408 (install-vs-update wording now only applies to the
      background banner) and #2409 (failure header shares the partitioned head
      region) so the three fixes compose.
- [ ] Assess the `tsuku-user` skill per the CLAUDE.md plugin-maintenance rule for
      `internal/updates/` changes; update it in the same PR if the update-output
      behavior it documents changed.

**Dependencies**: Blocked by <<ISSUE:1>>, Blocked by <<ISSUE:2>>

**Type**: task

## Dependency Graph

Single linear chain (single-pr — no GitHub issue nodes): Issue 1 → Issue 2 → Issue 3.
Issue 1 is the write-side root; Issue 2 depends on it; Issue 3 depends on both. See
Implementation Sequence for the critical-path rationale.

## Implementation Sequence

Single linear critical path: **Issue 1 → Issue 2 → Issue 3**, all in one PR.

- **Issue 1** is the write-side root; resolving the `SourceProjectAuto` open question
  here unblocks the partition predicate in Issue 2.
- **Issue 2** depends on Issue 1 so its head-of-output tests can assert that
  foreground successes are absent and only background-success is grouped.
- **Issue 3** depends on both: end-to-end validation and the cross-issue composition
  notes need the full behavior in place.

No parallelization within the PR (the chain is short and each step builds on the
prior). The work is sequenced but delivered as a single reviewable PR.
