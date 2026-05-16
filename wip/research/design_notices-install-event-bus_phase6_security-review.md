# Phase 6 Security Review: notices-install-event-bus

Second-pass review of the Security Considerations section authored in Phase 5
(see `DESIGN-notices-install-event-bus.md` lines 687-736 and the Phase 5
research note). Each numbered finding maps to a review question.

## 1. Attack vectors and integrity concerns the Phase 5 review missed

### 1a. `Source` enum values in user-facing output

The Phase 5 review treats `Source` as a closed enum and doesn't ask where its
string values surface. After tracing it:

- The renderer in `internal/updates/notify.go:71-111` switches on `Notice.Kind`
  and `Notice.Tool == SelfToolName`, **not** on `Source`. The `Source` value
  itself never lands in the notice file (the subscriber translates it via
  `kindFor(Source)` and discards the original).
- This is a non-finding for the current design — but it deserves an explicit
  callout in the doc, because a future "include Source in the notice file for
  attribution" change (very natural for a contributor adding telemetry) would
  start rendering the raw string. If that happens the `Source` enum becomes
  part of the user-facing surface and contributors must not put PII or
  attacker-influenceable strings in it.

### 1b. `kindFor(Source)` as a publisher-controlled rendering switch

The current `kindFor` mapping is:
- `SourceAutoUpdate` -> `KindAutoApplyResult`
- everything else -> `KindUpdateResult` (the empty-string legacy value)

A publisher chooses `Source` and thereby chooses how the renderer formats the
notice. Today the practical difference between the two kinds is minimal
(see `notify.go:96-102` — both go down the same success/failure rendering
path). But the mapping is one degree away from meaningful: `KindVersionFallback`
and `KindShellInitChange` are "single-view, remove after display" kinds
(`notify.go:104-109`). A future addition that maps a new `Source` to one of
those kinds, or adds a new kind with different semantics, makes the publisher
the deciding party for whether the notice persists or self-deletes.

**This is an integrity surface, not a confidentiality one.** The Phase 5
review missed it because it focused on inputs and disclosures, not on the
control-flow choice the mapping represents.

### 1c. `Source` parameter pollution by contributors

The implementation will thread `Source` through method parameters (Decision 3
+ "Implicit decision: Source passing convention"). A contributor calling
`Manager.Install` for a tool update could pass `SourceSelf` by mistake or
malice, which would:
- map through `kindFor` the same as everything except auto-update (today,
  identical rendering),
- but in the **renderer**, the tool name `"tsuku"` is the trigger for
  self-update phrasing (`notify.go:78-84`) — and the renderer uses `Notice.Tool`,
  not `Source`. So `SourceSelf` on a non-tsuku tool doesn't actually trigger
  self-update phrasing.

So today this is a no-op. But it deserves a unit test asserting that
`Source` values are not validated by `Manager.Install` against the tool name
(or, alternatively, that they **are** validated — the design should pick).
Without a constraint, a refactor that starts switching on `Source` (e.g., for
telemetry classification) will silently consume mislabeled events.

## 2. Sufficiency of the three proposed mitigations

### 2a. Extending tool-name validation to `RemoveNotice`

**Sufficient and correct.** Verified in `internal/notices/notices.go:57-59`
that `WriteNotice` checks but `RemoveNotice` (`notices.go:144-151`) does
`filepath.Join(noticesDir, toolName+".json")` with no validation. The fix
the design proposes (apply the same check) is the right one.

**Gap the design should add:** the validation in `WriteNotice` only checks
`/`, `\`, and `==".."`. It does **not** check for leading `.` (would create
dotfiles that `ReadAllNotices` then skips on `strings.HasPrefix(e.Name(), ".")`,
silently dropping a real notice), nor for embedded null bytes (`\x00`),
nor for absolute-path forms on Windows like `C:foo`. The path-separator-only
check is sufficient for the current provenance (recipe names from state.json),
but the design's "defense in depth for future call sites" rationale should
push to a tighter check: an explicit allowlist regex (kebab-case + digits,
matching what recipe loading already enforces) rather than a deny-list.

### 2b. Capping re-entrancy depth at 16

**Sufficient for the stated threat (runaway loop from a future buggy
subscriber).** Depth 16 is generous; 4 or 8 would be just as effective and
fail-faster on bugs.

**Missing:** the cap addresses *depth* (stack of nested handlers) but not
*breadth* (a subscriber that on each event publishes 100 new events that each
queue up). A subscriber publishing N siblings inside its `Handle` produces a
fanout, not a depth increase. A queue-size cap (e.g., refuse to enqueue if
the pending queue exceeds 1024) closes that case. Both caps together (depth
and queue size) is the standard guard.

### 2c. Routing bus logs to the trace file

**Necessary but not addressed in design text.** The doc says the logger
"should route to the same destination as other tsuku diagnostic output",
but `installevents.NewBus(log Logger)` takes a `Logger` interface and the
**caller** (`cmd/tsuku/events_wiring.go`) decides where it goes. If a future
test or alternate wiring passes a stderr logger, recovered panic values land
on user terminals.

The design should either (a) take a `*config.Config` and construct the
correct trace logger internally, or (b) specify in the wiring contract that
the production wiring must use the trace logger and add a comment to that
effect in `events_wiring.go`. The current pseudocode (`bus := installevents.NewBus(log)`)
is ambiguous about which logger.

## 3. "Not applicable" justifications that are actually applicable

The Phase 5 review marks three dimensions N/A. Re-examining:

- **External Artifact Handling** — correctly N/A. Events are built from
  in-process data already accepted by recipe/state validation.
- **Supply Chain or Dependency Trust** — correctly N/A. No new third-party
  imports. (Spot-checked: `internal/installevents` is purely first-party,
  uses only `sync`, `time`, an internal `Logger` interface.)
- **Trust Boundary Inside the Process** — correctly N/A **today**. Worth a
  one-line note that this property is preserved by Decision 4's rejection
  of `init()`-time registration; if that decision is ever reversed (plugin
  subscribers), the boundary analysis changes. Phase 5 actually does say
  this (review lines 215-219); the design doc itself does not. **Move that
  caveat into the design doc.**

No N/A is mis-classified.

## 4. Residual risk for end users and future contributors

### 4a. End-user (running as themselves) risks

- **Self-update failure surfacing for the first time.** Phase 5 flags this
  but understates the severity for one case: a self-update that fails because
  of a transient network or signature problem will now print the full
  `Err.Error()` text on **every subsequent `tsuku` invocation** until a
  successful self-update (because the renderer marks `Shown:true` but a
  subsequent failure rewrites with `Shown:false`). If the error string is
  large or noisy, this is a user-experience regression at minimum, and a
  log-spam-as-DoS vector if the error contains attacker-influenceable strings
  (e.g., redirect target URL from a hijacked update endpoint). Mitigation:
  truncate `InstallFailed.Err.Error()` at a fixed length (e.g., 512 bytes)
  in the subscriber before persisting.

- **Notice store unbounded growth.** With the new bus, every failed install
  attempt for every tool produces a notice file. Today the same is true,
  but the design opens new publishers (e.g., a future `tsuku run` autoinstall
  path that fails repeatedly). The notices directory has no GC. Not a Phase 6
  blocker, but worth a note for a future "notice retention" issue.

### 4b. Future-contributor risks (silent breakage)

- **Forgetting to call `bus.Publish` in a new state-mutating method.** The
  design says "every code path that mutates state.json publishes." There's
  no compile-time enforcement. A new `Manager.SoftRemove` method that
  doesn't publish will silently leave stale notices. The unit test
  recommended in the design ("a unit test that asserts no event for a no-op
  Activate") is insufficient — it tests one direction. A complementary test
  per mutation method (or a `state.UpdateTool` linter) is warranted, even if
  initially the linter is just a `golangci-lint` `forbidigo` rule on
  `state.SetTool` outside `installevents`-publishing methods.

- **Publishing on the wrong side of the state write.** The contract is
  "publish after the mutation succeeds". A contributor who moves the
  publish above the state write would publish an event the on-disk state
  doesn't reflect. The notices subscriber would write `AttemptedVersion: X`
  while state still says `Y`. The drift bug returns. This needs an
  explicit comment at each publish site and ideally a test that verifies
  state file content immediately on event observation (a synchronous-bus
  property the design already commits to).

## 5. Race conditions / TOCTOU between foreground and background

The Phase 5 review covers `os.Rename` atomicity and the `MarkShown`
read-mutate-write window. Two cases it doesn't fully address:

### 5a. Background writes, foreground renders, background re-writes

Sequence:
1. Background auto-apply succeeds, writes `Shown:false` for `niwa@0.11.1`.
2. Foreground command starts, `renderUnshownNotices` prints "niwa updated
   to 0.11.1", then calls `MarkShown`.
3. Background, on a different tool, completes another install — but as part
   of its run it also rewrites notices (if a subscriber misbehaves and
   touches more than its own tool). With a single notices subscriber
   reacting only to events for the published tool, this is fine. With a
   future "summary" subscriber that rewrites all notices, this becomes a
   real lost-update window.

This is a **future risk** the design should note: subscribers must touch
only the notice for the tool in the event they're handling. The current
subscriber satisfies this; the contract should be stated.

### 5b. Concurrent fresh write vs MarkShown

Already in Phase 5 (review lines 242-247). The Phase 5 conclusion ("This race
exists today with the ad-hoc writes; the event bus does not introduce it")
is correct, but the design doc should explicitly say the bus **does not fix**
this — a reader might assume the new structural approach addresses it. It
does not; a future fix needs a per-tool advisory lock or a CAS-like
write-if-not-shown semantic.

## 6. Notice file content sanitization

The Phase 5 review notes `Err.Error()` may include URLs/paths but stops short
of a recommendation. Confirming the surface:

- `apply.go:189` already stores `applyErr.Error()` directly.
- The renderer prints `n.Error` directly to stderr (`notify.go:100`).

Real errors in this codebase already include:
- HTTP status text and target URLs (download failures from `internal/actions/download.go`).
- Full filesystem paths under `$TSUKU_HOME` and `/tmp`.
- Recipe-derived URLs and signature/checksum byte strings.

This is **pre-existing** but the design changes the visibility profile in
two ways:
1. Self-update failures become visible for the first time.
2. The structural fix means notices stay accurate, so they're more likely
   to actually be read.

**Sanitization recommendations** (none of which the design currently makes):
- **Length cap.** Truncate `Err.Error()` to e.g. 512 bytes with a "..." suffix
  before persisting. Prevents HTTP-body or stack-trace contamination.
- **Newline normalization.** Replace `\n` and `\r` in the error string with
  ` / ` before writing to the notice file. Today the renderer happily prints
  multi-line errors, which can hide subsequent lines under terminal scrolling
  and which look strange in `tsuku notices` output.
- **No new content.** Document in the subscriber (not just in Security
  Considerations) that future error wrapping must not include HTTP response
  bodies, stack traces, or environment variables. A test that asserts
  the notice's `Error` field never contains `\n` would enforce the
  normalization rule.

## Recommendations

### DESIGN-CHANGE (must be incorporated before approval)

1. **Specify logger destination.** Change `NewBus(log Logger)` contract so
   the production wiring is required to pass a trace-file logger, not stderr.
   Either bake the logger choice into `NewBus` from a `*config.Config`, or
   add a wiring-level comment + test that asserts the logger destination.
   (Question 2c.)

2. **Add a queue-size cap alongside the depth cap.** The depth-16 cap stops
   nested loops but not fanout-loops. Cap pending queue size (e.g., 1024)
   and log-drop beyond. (Question 2b.)

3. **Sanitize `InstallFailed.Err.Error()` before persisting.** Truncate to
   512 bytes and replace newlines with ` / `. Add a unit test asserting
   `Notice.Error` never contains `\n`. (Question 6.)

4. **Document the "publish after state write" invariant at each publish
   site.** A code comment at each `bus.Publish` call plus a test per
   mutation method that observes state-then-event ordering. The drift bug
   returns if this invariant is silently broken. (Question 4b.)

5. **Capture in the design doc that re-introducing `init()`-time subscriber
   registration would invalidate the "no internal trust boundary" finding.**
   One-line note; preserves the audit trail for future contributors.
   (Question 3.)

6. **State the subscriber-locality contract explicitly:** a subscriber may
   only touch the notice for the tool named in the event it is handling.
   This preserves the per-tool atomicity story for future subscribers.
   (Question 5a.)

### IMPLEMENTER-NOTE (implementer should be aware of)

7. **Tighten `WriteNotice`/`RemoveNotice` validation beyond path separators.**
   Reject leading `.`, embedded null bytes, and Windows-drive prefixes;
   prefer an explicit kebab-case allowlist regex. (Question 2a.)

8. **`Source` enum values must remain non-PII, non-attacker-influenced strings.**
   They aren't currently rendered, but a natural future change (include
   `Source` in the notice file for attribution) would expose them.
   Add a code comment on the enum definition. (Question 1a.)

9. **Add a no-op test ensuring a refactor doesn't start switching renderer
   behavior on `Source`.** If `kindFor` is ever extended to map `Source` to
   single-view kinds, the publisher becomes the deciding party for notice
   persistence; the design should explicitly forbid that mapping pattern.
   (Question 1b.)

10. **Validate `Source` against `Tool` at publish time, or document why not.**
    A contributor passing `SourceSelf` for a non-`tsuku` tool should either
    fail loudly or be explicitly tolerated. Pick one. (Question 1c.)

11. **Note that the bus does not fix the `MarkShown` write-clobber race.**
    The race pre-exists; the design's structural fix is orthogonal. A
    future issue can address per-tool locking. (Question 5b.)

12. **Future work: notice-store GC.** The structural fix means notices
    accumulate as designed. A separate issue for retention/cleanup is
    warranted but out of scope here. (Question 4a.)
