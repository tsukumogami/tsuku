<!-- decision:start id="notice-schema-extension" status="assumed" -->
### Decision: Notice Schema Extension for Background Activity Results

**Context**

The notification system is pull-based and file-backed: background processes write JSON
files to `$TSUKU_HOME/notices/<toolname>.json`, and `DisplayNotifications` reads and
renders them on the next command invocation. Each file holds one `Notice` struct with
fields for tool name, attempted version, error string, timestamp, shown flag, and
consecutive failure count. The `shown` field gates display: written as `false`, set to
`true` after rendering.

Self-update already uses this pattern for async results — a background self-update
process writes a notice with an empty `Error` field; the next command displays
"tsuku has been updated to X.Y.Z". The rendering logic in `renderUnshownNotices`
already branches on `n.Error == ""` to distinguish success from failure for both self
and tool notices. When auto-apply moves to background, the background subprocess will
write notice files; the next foreground command reads and displays them.

The constraint is backward compatibility: existing notice files on user machines must
deserialize without error. Go's `json.Unmarshal` leaves missing fields at zero value,
so any new field with a zero-value sentinel is inherently backward-compatible. The
`ConsecutiveFailures int` field, added as `omitempty`, establishes this pattern in the
existing codebase.

**Assumptions**

- Background auto-apply success notices should be shown (not silently discarded).
  If a background update ran and failed, the user must know. If it succeeded, a brief
  confirmation ("foo updated to 1.2.0") is useful and matches what self-update shows
  today. The user who wants silence can set quiet mode or use CI environment variables.
- No migration mechanism exists for existing notice files. Any schema change must work
  via zero-value semantics alone.
- Future notice kinds (e.g., registry-refresh-complete) are plausible but not
  scheduled; extensibility is a secondary concern.

**Chosen: Option A — Add a Kind field with backward-compatible JSON unmarshaling**

Add a `Kind string` field to `Notice` with `omitempty` and an empty-string sentinel
meaning "legacy/update-result":

```go
type Notice struct {
    Tool                string    `json:"tool"`
    AttemptedVersion    string    `json:"attempted_version"`
    Error               string    `json:"error"`
    Timestamp           time.Time `json:"timestamp"`
    Shown               bool      `json:"shown"`
    ConsecutiveFailures int       `json:"consecutive_failures,omitempty"`
    Kind                string    `json:"kind,omitempty"`
}
```

Defined constants:

```go
const (
    // KindUpdateResult is the zero-value kind, used for all existing and new
    // tool/self-update notices. Legacy files with no "kind" field deserialize here.
    KindUpdateResult = ""

    // KindAutoApplyResult distinguishes a background-apply result from a notice
    // written by a synchronous foreground operation (e.g., rollback failure).
    KindAutoApplyResult = "auto_apply_result"
)
```

Background auto-apply writes `kind: "auto_apply_result"`. Self-update continues to
write `kind: ""` (omitted from JSON), preserving its current behavior with no changes
to existing paths. Rendering switches on Kind when needed but the default path (`Kind
== ""`) remains unchanged. Old notice files on user machines deserialize with
`Kind == ""` and render exactly as before.

**Rationale**

Option C (repurpose the Error field) is the minimal change but wrong semantically:
a successful update is not an error. The rendering logic already makes the
success/failure distinction via `n.Error == ""`; encoding a success message in the
Error field would require callers to inspect the string to know whether to format it
as an error or a confirmation, which is fragile. More concretely, the existing
`renderUnshownNotices` already handles `n.Error == ""` correctly for tool success:
it renders `"%s has been updated to %s"` — so no new behavior is needed for
background success notices. But it cannot distinguish a background-apply success from a
foreground-apply notice written by a different code path, which matters for rendering
and deduplication. A `Kind` field makes that distinction explicit and clean.

Option B (separate directory for activity notices) splits what is one conceptual entity
— "something happened to tool X" — into two file trees with two read paths. The added
complexity does not serve the constraints: backward compatibility is equally achievable
with a field addition, and TTY/CI suppression already works via `ShouldSuppressNotifications`
regardless of which directory the file lives in.

Option A's `Kind` field costs one field definition and one switch case. It's backward-
compatible by construction (zero value = legacy), extensible for future kinds, and
keeps the notice system's single-directory, single-struct model intact. The `omitempty`
tag means legacy success notices — which already work — continue to omit the field
entirely, so existing rendering code is unaffected until a caller explicitly checks Kind.

**Sub-questions resolved**

1. **Should success notices be shown?** Yes. Background auto-apply runs silently; the
   only signal that something changed is the notice on the next command. A one-line
   confirmation ("foo updated to 1.2.0") is appropriate. The user already sees this for
   self-updates. Failures are always shown (subject to consecutive-failure threshold).

2. **Rendering format for success/activity notices.** Match the existing format: plain
   stderr line, no box, no prefix. Self-update uses `"tsuku has been updated to X.Y.Z"`;
   tool updates should use the same pattern `"%s has been updated to %s"`. The `[notice]`
   prefix (pip) or boxed style (npm) adds visual weight that doesn't match tsuku's
   minimal stderr style.

3. **Deduplication for success notices.** No new deduplication mechanism needed. The
   existing `shown` gate is the dedup: a success notice is written with `Shown: false`,
   displayed once, then marked `Shown: true`. On next successful update the prior notice
   is overwritten (one file per tool). This matches the current behavior for self-update
   success and requires no new sentinel or throttle.

**Consequences**

- The `Notice` struct gains one field. All existing callers compile without change;
  `omitempty` means no JSON output changes for any existing write path.
- Background auto-apply writes `Kind: KindAutoApplyResult` so rendering logic can
  distinguish it from foreground-path notices if needed in the future.
- Self-update paths (`self.go`, `renderUnshownNotices`) require no changes; they
  continue using `Kind == ""` implicitly.
- The single-directory, single-struct notice model is preserved. The file GC logic
  (one file per tool, overwritten on new event) is unchanged.
- Future notice kinds (e.g., "registry_refresh") can be added by defining a new
  constant and a rendering branch, without affecting existing files on disk.
<!-- decision:end -->
