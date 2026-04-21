<!-- decision:start id="download-progress-granularity" status="assumed" -->
### Decision: Download Progress Granularity in Unified Status Line

**Context**

tsuku's current download output uses a separate `progress.Writer` widget
(`internal/progress/progress.go`) that renders a full progress bar inline:
`[========>] 78% (31.2MB/40MB) 8.5MB/s ETA: 00:01`. This widget operates
independently of step output, writing directly to stdout via carriage returns.

The install-ux redesign eliminates this widget. All output routes through
`Reporter.Status()` calls, which animate in-place on TTY and degrade to plain
lines in non-TTY. The download step becomes one status update among many in
the install flow, not a separate rendering subsystem.

Three levels of detail are possible. At the high end: percentage plus bytes
when Content-Length is present, falling back to bytes-only when absent. At the
middle: bytes transferred only, always available regardless of Content-Length.
At the low end: spinner with filename only, requiring no instrumentation of the
download path at all.

**Assumptions**

- The Reporter tick rate will be approximately 100ms (matching the current
  progress.Writer rate limit). If significantly faster, the bytes-only option
  would require a rate-limit guard in the instrumented io.Writer.
- Most tool downloads complete in under 60 seconds on a modern connection.
  Downloads that take longer (large IDE extensions, heavy toolchains) benefit
  more from byte counters than short downloads do.
- The new design wires download progress through a callback or instrumented
  io.Writer passed alongside the destination writer, not by modifying
  httputil. This is consistent with how the current progress.Writer is used.
- "Small file" threshold for suppressing progress is 100 KB. Files below this
  complete fast enough that no status update is needed.

**Chosen: Percentage + bytes when Content-Length available, bytes-only fallback**

When `resp.ContentLength > 0`, the status line reads:
`Downloading kubectl 1.29.3 (12.5 / 40.0 MB, 78%)`.
When Content-Length is absent, it reads:
`Downloading kubectl 1.29.3 (12.5 MB...)`.
For files under 100 KB, no byte counter appears: the status line shows the
filename and the spinner animates until completion.

Implementation requires an instrumented `io.Writer` that tracks bytes written
and calls a provided callback (or updates a shared atomic counter) on each
write. The Reporter tick loop reads the counter and formats the status string.
This is effectively what `progress.Writer` already does, but with the rendering
path replaced by `Reporter.Status()` instead of direct `fmt.Fprint`.

**Rationale**

Option 2 (percentage + bytes fallback) preserves the substantive information
users already see today, while fitting it into the new unified status line.
For large downloads — the case where progress feedback matters most — it
provides the same orientation as the current widget: "I'm 78% through a 40 MB
file." For servers that omit Content-Length (some CDNs, redirected URLs), it
gracefully degrades to bytes-only, which still tells the user something is
happening and how much has arrived.

Option 1 (bytes-only) is simpler to implement but provides less information
for no benefit: the percentage calculation is trivial once the total is known,
and omitting it makes a 78%-complete download look identical to a 5%-complete
one. Users waiting on a large download lose orientation.

Option 3 (spinner-only) is attractive for its simplicity — zero instrumentation
of the download path. But it fails for downloads over ~30 seconds. A 500 MB
binary on a 10 Mbps connection takes roughly 7 minutes; a spinner with no
counter gives no signal that it's making progress. The constraint "must work for
downloads ranging from 2 KB to 500 MB" rules this out.

The implementation cost difference between options 1 and 2 is negligible: both
require the same instrumented `io.Writer`. Option 2 adds a conditional format
string and one integer division. Option 1 saves nothing meaningful.

**Alternatives Considered**

- **Bytes transferred only**: Always shows transferred bytes regardless of
  Content-Length. Simpler format string. Rejected because when total size is
  known, omitting the percentage degrades orientation for no gain — the
  conditional format costs nothing and the percentage is the most useful
  single number a user can see mid-download.
- **Spinner-only with filename**: No instrumentation of the download path at
  all. Rejected because it fails for large, slow downloads. A 500 MB download
  on a slow connection can take several minutes; a spinner alone gives no
  confidence that transfer is progressing. The 2 KB–500 MB range in the
  constraints explicitly covers cases where a counter is necessary.

**Consequences**

- The download path in `download_file.go` and `download.go` gains a thin
  instrumentation layer: an `io.Writer` wrapper that counts bytes and invokes a
  progress callback. This replaces the existing `progress.NewWriter` call.
- `internal/progress/progress.go` can be deleted once all call sites migrate.
- The Reporter interface gains a `Status(msg string)` call that accepts a
  pre-formatted string. The download layer formats the string; Reporter
  renders it.
- The small-file threshold (100 KB) suppresses the counter for checksum files
  and other tiny downloads, reducing noise without behavioral changes for large
  tools.
- Non-TTY output: the status line is emitted once at download start
  (`Downloading kubectl 1.29.3 (40.0 MB)`) and once at completion
  (`Downloaded kubectl 1.29.3`). Mid-download byte counters are not emitted
  in non-TTY mode since there's no in-place update capability.
<!-- decision:end -->
