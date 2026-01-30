---
status: Current
problem: >
  The batch generation pipeline reads from data/priority-queue.json to decide which packages
  to generate recipes for, but that file doesn't exist yet. The seed script can create it from
  Homebrew analytics data, but there's no CI workflow to run it and no merge logic to prevent
  re-seeding from clobbering packages the pipeline has already processed.

  Beyond the initial bootstrapping gap, the queue needs a clear lifecycle model. Items move
  through statuses (pending, in_progress, success, failed) as they're consumed by the batch
  pipeline. The seed tool must understand this lifecycle so it only adds genuinely new
  packages and leaves existing entries untouched regardless of their processing state.

  Without automation, the queue becomes stale as Homebrew popularity rankings shift over time.
  A tool that's the 50th most downloaded formula this month might be the 20th next month, and
  new tools appear regularly. Manual seeding doesn't scale.
decision: >
  We built a Go tool (cmd/seed-queue) that fetches Homebrew popularity data, merges it
  additively into data/priority-queue.json, and writes validated output. A GitHub Actions
  workflow runs this tool via workflow_dispatch, with a path to graduate to weekly cron
  after meeting reliability criteria.

  The merge is always-on: the tool reads the existing queue file (or starts empty if none
  exists), fetches fresh data, and adds only packages whose IDs aren't already present.
  Existing entries keep their status, tier, and metadata. This ensures the batch pipeline's
  state is never disrupted by a re-seed.

  The queue lifecycle is explicitly modeled with clear ownership boundaries. The seed tool
  only creates pending entries. The batch pipeline owns all status transitions. Operators can
  manually re-queue failed items after fixes. Items are never removed from the file -- success
  and skipped entries serve as deduplication records.
rationale: >
  Go was the right language for this tool. The merge logic involves structured JSON
  manipulation (reading the queue, deduplicating by ID while preserving status fields,
  writing valid output) that's awkward in bash/jq but natural with Go structs. The existing
  seed-queue.sh was already 185 lines; adding merge logic would have pushed it past the point
  where bash is comfortable. A Go implementation gets type safety, proper error handling,
  unit-testable merge logic, and no runtime dependency on jq.

  Direct commits to main are acceptable because the output is a JSON data file validated
  before commit. PR-based updates would block the automation path since cron-triggered
  runs can't wait for human review. The graduation criteria (3 successful manual runs, merge
  verified, batch pipeline tested) provide confidence before enabling automation.

  Scoping to Homebrew only kept the initial implementation focused. The Source interface
  supports additional ecosystems when their fetch functions are added later.
---

# DESIGN: Seed Priority Queue Pipeline

## Status

Current

## Upstream Design Reference

This design implements part of [DESIGN-registry-scale-strategy.md](../DESIGN-registry-scale-strategy.md) (Phase 0 visibility infrastructure).

**Relevant sections:**
- Phase 0 deliverables: "Scripts to populate queue from Homebrew API (popularity data)"
- Decision 2: Popularity-based prioritization (Option 2A)
- M-BatchPipeline milestone, issue #1241

## Context and Problem Statement

The batch generation pipeline (#1189) reads from `data/priority-queue.json` to decide which packages to generate recipes for. Three things prevented this from working:

1. **The file didn't exist.** `scripts/seed-queue.sh` could create it from Homebrew data, but nobody had run it and committed the output.

2. **No automation.** Running the script manually doesn't scale. Homebrew's popularity rankings shift as new tools gain traction and others fall off.

3. **No merge logic.** The existing script always overwrites the output file. If the batch pipeline has marked some packages as `in_progress` or `success`, re-running the seed script would reset them to `pending`.

The existing `seed-queue.sh` was already 185 lines of bash with jq pipelines for JSON transformation. Adding merge logic (read existing file, deduplicate by ID, preserve statuses) would have pushed it past comfortable bash complexity. The merge logic involves structured data manipulation that's natural in Go but awkward in bash/jq.

## Decision Drivers

- Don't break existing status: re-running the seed must not reset packages the batch pipeline has already processed.
- Schema compliance: output must conform to `data/schemas/priority-queue.schema.json`.
- Observable before automated: the workflow should run manually first so operators can verify output before trusting a schedule.
- Testable merge logic: the merge/deduplication behavior must be unit-testable, not buried in jq pipelines.
- Extensible to more ecosystems: adding Cargo, npm, PyPI sources later should mean adding a Go function, not a new script.

## Considered Options

Four approaches were evaluated. We chose Option D (Go tool) because the merge logic deserves real unit tests, and Go gives type safety for the queue schema plus a natural extensibility path via the `Source` interface. The rejected alternatives:

- **Option A (extend seed-queue.sh with --merge):** Merge logic in jq is untestable. The script was already at the limit of comfortable bash complexity.
- **Option B (PR-based workflow):** Prevents full automation. A data file validated against a schema doesn't need human review on every update.
- **Option C (merge in workflow YAML):** Splits merge responsibility into YAML steps, making it untestable and unrunnable locally.

## Decision Outcome

We built a Go tool at `cmd/seed-queue` with supporting logic in `internal/seed/`. The tool always performs additive merge -- there's no flag to toggle this behavior. It loads the existing queue (or starts empty), fetches from the Homebrew API, adds only new packages, and writes the result.

The tradeoffs accepted:

- More upfront work than extending bash, but the merge logic needs to be correct (it protects batch pipeline state) and correctness comes from tests.
- `seed-queue.sh` becomes a reference artifact rather than the production tool.
- Direct commits to main for a data file. If this becomes a concern, PR-based updates can be added later.

## Solution Architecture

### Queue Lifecycle

The queue file serves two roles: it's the input for the batch pipeline (what to process next) and the record of what's been processed (what already succeeded or failed). Items are never removed. An item with status `success` stays as a record that prevents re-seeding and provides historical context.

**Status ownership:**

| Actor | Creates entries? | Changes status? | To which statuses? |
|-------|-----------------|-----------------|-------------------|
| seed-queue tool | Yes (pending only) | No | -- |
| batch pipeline | No | Yes | pending -> in_progress -> success/failed |
| operator (manual) | No | Yes | failed -> pending (re-queue), any -> skipped |

The seed tool never changes the status of an existing entry. It checks the `id` field of every existing entry regardless of status. If `homebrew:ripgrep` exists with any status, the tool won't add it again. This means a `success` item won't be re-queued just because it's still popular on Homebrew, a `failed` item won't be re-queued automatically (it needs manual intervention), and a `skipped` item stays skipped.

Re-queuing is not handled by the seed tool. When a blocking dependency is added or a builder bug is fixed, an operator changes the status from `failed` back to `pending`. Version-aware re-queuing is out of scope for this design.

The queue grows monotonically. At Homebrew scale with a limit of 100, the file stays under 200 entries. Even at 500 entries across multiple ecosystems, the file would be ~50 KB. Pruning isn't needed for the foreseeable future.

### Go Tool

The CLI entry point is at `cmd/seed-queue/main.go` (line 11). It accepts three flags:

- `-source`: ecosystem to fetch from, currently only `homebrew` (required)
- `-limit`: maximum packages to fetch, default 100
- `-output`: path to queue file, default `data/priority-queue.json`

The tool always performs merge: it calls `Load()` to read the existing file (returning an empty queue if the file doesn't exist), calls `Source.Fetch()` to get candidates, then `Merge()` to add new entries, then `Save()` to write the result. There's no overwrite mode or merge toggle.

### internal/seed package

**`queue.go`** contains the `PriorityQueue` and `Package` types matching `data/schemas/priority-queue.schema.json`. Three methods handle the lifecycle:

- `Load(path)` (line 31): reads the file, returns an empty queue with `SchemaVersion: 1` if the file doesn't exist, and returns an error if the file exists but can't be parsed. This is the "fail on corruption" behavior -- unlike the bash approach, corrupt files aren't silently ignored.
- `Save(path)` (line 50): stamps `UpdatedAt` with the current UTC time, marshals as indented JSON, and writes atomically via `os.WriteFile`.
- `Merge(newPackages)` (line 62): builds a set of existing IDs, then appends only packages whose ID isn't in the set. Returns the count of packages added.

**`homebrew.go`** implements `HomebrewSource` (line 36). It fetches from `https://formulae.brew.sh/api/analytics/install-on-request/30d.json` with a 30-second HTTP timeout and a 10 MB response body limit via `http.MaxBytesReader` (line 72 in `fetchWithRetry`). The `AnalyticsURL` field allows test overrides without network access.

HTTP requests retry with exponential backoff on 5xx and 429 responses. `fetchWithRetry()` (line 89) makes up to 3 attempts, starting with a 1-second delay that doubles each time. Non-retryable errors (4xx other than 429) fail immediately without retry.

Tier assignment uses `assignTier()` (line 128): tier 1 is a curated map of 31 developer tools (`tier1Formulas`, line 19), tier 2 is formulas with >= 40,000 installs over 30 days (`tier2Threshold`, line 15), and everything else is tier 3. The Homebrew API returns download counts as comma-separated strings, which `parseCount()` (line 138) strips and converts.

All fetched packages get `status: "pending"` and an ID of `homebrew:<formula-name>`.

**`source.go`** defines the `Source` interface with two methods: `Name() string` and `Fetch(limit int) ([]Package, error)`. Adding a new ecosystem means implementing this interface.

### Design choices

- **No in-tool schema validation.** The tool doesn't validate its output against the JSON schema before writing. Validation happens in the workflow via a `jq` check. Full JSON Schema validation could be added later if the basic check proves insufficient.
- **Merge is always-on.** There's no `--merge` flag or overwrite mode. Every invocation loads the existing queue and adds to it. This was a deliberate choice: there's no use case for overwrite mode in CI, and making merge the default eliminates a class of operator errors.

## Implementation Approach

### Workflow

`.github/workflows/seed-queue.yml` runs via `workflow_dispatch` only. The `source` input is a dropdown limited to `homebrew`; `limit` is a free-text field defaulting to `"100"`.

The workflow has four steps:

1. **Build**: `go build -o seed-queue ./cmd/seed-queue` (line 33)
2. **Run**: invokes the built binary with the input parameters (line 36)
3. **Validate**: checks that `data/priority-queue.json` exists and has `schema_version == 1` with at least one package, using a `jq -e` assertion (line 42). This is a basic structural check, not full JSON Schema validation.
4. **Commit and push**: diffs the queue file, skips if unchanged, otherwise commits with retry logic (3 attempts with exponential backoff, lines 50-63). Uses `github-actions[bot]` identity and `[skip ci]` in the commit message.

The workflow requests `contents: write` permission for pushing commits.

### Graduation to Cron

The workflow starts with `workflow_dispatch` only. Adding a cron schedule requires:

1. 3 successful manual runs with valid output
2. At least one merge run against an existing queue file confirming statuses are preserved
3. Batch pipeline (#1189) has successfully read from the seeded queue
4. No data corruption observed

Once met, adding `schedule: [{cron: '0 1 * * 1'}]` (weekly, Monday 1 AM UTC) to the workflow triggers section enables automation.

### Test Coverage

11 unit tests across two files, all using fixtures (no network calls):

**`queue_test.go`** (7 tests): `TestMerge_deduplicates` verifies that existing entries (including those with `success` status) are preserved and only genuinely new packages are added. `TestLoadSave_roundtrip` confirms serialization fidelity. `TestLoad_missingFile` verifies the empty-queue fallback. `TestAssignTier` and `TestParseCount` cover tier logic and comma-separated number parsing. `TestMerge_empty` handles the nil-input edge case. `TestSave_createsDirectory` verifies writing to a new path.

**`homebrew_test.go`** (4 tests): `TestHomebrewSource_Fetch` uses `httptest.NewServer` to serve fixture analytics data and verifies tier assignment and ID construction. `TestHomebrewSource_FetchError` verifies that HTTP 500 responses produce errors after exhausting retries. `TestHomebrewSource_RetryOnServerError` confirms that the tool succeeds after transient 503 failures. `TestHomebrewSource_NoRetryOn4xx` confirms that 4xx errors (other than 429) fail immediately without retry.

## Security Considerations

### Download Verification

Not applicable. The seed tool fetches popularity metadata (package names, download counts), not binaries or executable artifacts.

### Execution Isolation

The workflow runs in GitHub Actions with `contents: write` permission via `GITHUB_TOKEN`. It commits directly to the default branch. The committed file is a JSON data file -- not executable code. Both the tool's output and the workflow's `jq` validation step must pass before anything is committed.

### Supply Chain Risks

The Homebrew analytics API could be compromised to inject misleading package names. However, the seed tool only writes names and scores to the queue. The batch pipeline validates every generated recipe through its own gates before merging. A bad entry wastes CI time but can't introduce malicious code into the registry.

All API calls use HTTPS. The Go HTTP client verifies TLS certificates by default.

### User Data Exposure

Not applicable. The tool reads from a public API and writes to a file in the repository. No user data is accessed or transmitted.

## Consequences

### Positive

- `data/priority-queue.json` can be created with real Homebrew popularity data
- The batch pipeline (#1189) can start processing packages once it's ready
- Queue stays fresh automatically once cron is enabled
- Merge logic is unit-tested, protecting batch pipeline state from corruption
- Adding more ecosystems means implementing a two-method Go interface

### Negative

- `seed-queue.sh` becomes a reference artifact not actively used by CI
- Direct commits to main bypass PR review (acceptable for schema-validated data files)
- No in-tool schema validation; relies on the workflow's basic jq check

### Neutral

- The Go tool replaces the bash script for CI use. The bash script can still be run manually for one-off seeding.
