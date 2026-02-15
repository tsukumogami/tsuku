# Test Plan: Pipeline Dashboard Enhancement

Generated from: docs/designs/DESIGN-pipeline-dashboard.md
Issues covered: 4
Total scenarios: 14

---

## Scenario 1: QueueEntry struct exists with all required fields
**ID**: scenario-1
**Testable after**: #1697
**Commands**:
- `grep -q "type QueueEntry struct" internal/batch/queue_entry.go`
- `for field in Name Source Priority Status Confidence DisambiguatedAt FailureCount NextRetryAt; do grep -q "$field" internal/batch/queue_entry.go || exit 1; done`
**Expected**: File `internal/batch/queue_entry.go` exists and defines a `QueueEntry` struct containing all eight fields: Name, Source, Priority, Status, Confidence, DisambiguatedAt, FailureCount, NextRetryAt
**Status**: passed
**Executed**: 2026-02-15
**Output**: Struct `QueueEntry` found in `internal/batch/queue_entry.go`. All eight fields (Name, Source, Priority, Status, Confidence, DisambiguatedAt, FailureCount, NextRetryAt) are present.

---

## Scenario 2: QueueEntry JSON round-trip marshaling works
**ID**: scenario-2
**Testable after**: #1697
**Commands**:
- `go test -v ./internal/batch/... -run QueueEntry`
**Expected**: Unit tests pass demonstrating that a QueueEntry can be marshaled to JSON and unmarshaled back with all fields preserved, including nullable time fields (DisambiguatedAt, NextRetryAt) serializing as null when nil
**Status**: passed
**Executed**: 2026-02-15
**Output**: All 16 QueueEntry-related tests pass (0.007s). Round-trip marshaling preserves all fields. Null time fields (`disambiguated_at: null`, `next_retry_at: null`) deserialize correctly to nil pointers.

---

## Scenario 3: QueueEntry validation rejects invalid entries
**ID**: scenario-3
**Testable after**: #1697
**Commands**:
- `go test -v ./internal/batch/... -run Validation`
**Expected**: Validation method rejects entries with empty name, empty source, invalid status values (anything other than pending/success/failed/blocked/requires_manual/excluded), and invalid confidence values (anything other than auto/curated). Tests pass with non-zero coverage of the validation path.
**Status**: passed
**Executed**: 2026-02-15
**Output**: All 9 Validate tests pass. Covers: empty name, empty source, invalid priority (0, -1, 4, 100), invalid status ("in_progress"), invalid confidence ("manual"), negative failure_count, whitespace-only name, and multiple simultaneous errors. Note: test plan command used `-run Validation` which matches no tests; corrected to `-run Validate`.

---

## Scenario 4: QueueEntry JSON tags use snake_case field names
**ID**: scenario-4
**Testable after**: #1697
**Commands**:
- `grep 'json:"name"' internal/batch/queue_entry.go`
- `grep 'json:"source"' internal/batch/queue_entry.go`
- `grep 'json:"disambiguated_at"' internal/batch/queue_entry.go`
- `grep 'json:"failure_count"' internal/batch/queue_entry.go`
- `grep 'json:"next_retry_at"' internal/batch/queue_entry.go`
**Expected**: All JSON struct tags use snake_case naming matching the design document schema (name, source, priority, status, confidence, disambiguated_at, failure_count, next_retry_at)
**Status**: passed
**Executed**: 2026-02-15
**Output**: All eight JSON tags confirmed: `json:"name"`, `json:"source"`, `json:"priority"`, `json:"status"`, `json:"confidence"`, `json:"disambiguated_at"`, `json:"failure_count"`, `json:"next_retry_at"`. All use snake_case matching the design document schema.

---

## Scenario 5: Bootstrap script builds successfully
**ID**: scenario-5
**Testable after**: #1697, #1698
**Commands**:
- `go build -o bootstrap-queue ./cmd/bootstrap-queue`
**Expected**: The bootstrap command builds without errors, producing a `bootstrap-queue` binary
**Status**: passed
**Executed**: 2026-02-15
**Output**: Binary `bootstrap-queue` compiled without errors (exit code 0). Go 1.25.7 on linux/amd64.

---

## Scenario 6: Bootstrap produces valid unified queue from existing data
**ID**: scenario-6
**Testable after**: #1697, #1698
**Commands**:
- `go build -o bootstrap-queue ./cmd/bootstrap-queue`
- `./bootstrap-queue`
- `jq empty data/queues/priority-queue.json`
- `jq -e '.entries[0] | has("name") and has("source") and has("status") and has("confidence")' data/queues/priority-queue.json`
**Expected**: Running the bootstrap script produces `data/queues/priority-queue.json` containing valid JSON. Each entry has the required fields (name, source, status, confidence). The file is a JSON array of QueueEntry objects.
**Status**: passed
**Executed**: 2026-02-15
**Output**: Bootstrap ran successfully (exit code 0), producing 5275 entries. Output is valid JSON (`jq empty` passed). First entry has all required fields (name, source, status, confidence). Note: output format is a `UnifiedQueue` wrapper with `schema_version`, `updated_at`, and `entries` fields -- not a bare array. The test plan command was adjusted from `.[0]` to `.entries[0]` accordingly. 7 recipes skipped with warnings (no source found in steps: cuda, curl, docker, iterm2, ncurses, pipx, test-tuples).

---

## Scenario 7: Bootstrap assigns correct status by data source
**ID**: scenario-7
**Testable after**: #1697, #1698
**Commands**:
- `jq '[.entries[] | select(.status == "success")] | length' data/queues/priority-queue.json`
- `jq '[.entries[] | select(.status == "pending")] | length' data/queues/priority-queue.json`
- `jq '[.entries[] | select(.confidence == "curated")] | length' data/queues/priority-queue.json`
**Expected**: Entries derived from existing recipes have `status: "success"` and `confidence: "curated"` (at least 100 entries). Entries from the homebrew queue have `status: "pending"` and `confidence: "auto"`. Curated override entries not in recipes have `status: "pending"` and `confidence: "curated"`. No duplicate entries exist (each name appears once).
**Status**: passed
**Executed**: 2026-02-15
**Output**: Status and confidence assignments are correct across all three data sources. Recipe entries: 267 with status=success, confidence=curated (exceeds 100 threshold). Curated override entries: 10 with status=pending, confidence=curated. Homebrew entries: 4998 with status=pending, confidence=auto. Totals cross-check: 267+10=277 curated, 4998 auto; 267 success + 5008 pending = 5275 total. Note: test plan jq commands adjusted from `.[]` to `.entries[]` due to UnifiedQueue wrapper format.

---

## Scenario 8: Bootstrap deduplicates entries with correct precedence
**ID**: scenario-8
**Testable after**: #1697, #1698
**Commands**:
- `jq '.entries | group_by(.name) | map(select(length > 1)) | length' data/queues/priority-queue.json`
**Expected**: The count of duplicate name groups is 0, confirming that each package name appears exactly once in the queue. Recipe entries take precedence over curated overrides, which take precedence over homebrew entries.
**Status**: passed
**Executed**: 2026-02-15
**Output**: Duplicate name group count is 0 -- each package name appears exactly once across all 5275 entries. Precedence verified: `gh` (present in both recipes and homebrew) resolved to the recipe entry (status=success, source=homebrew:gh from recipe's homebrew action, confidence=curated). Note: test plan jq command adjusted from bare `group_by` to `.entries | group_by` due to UnifiedQueue wrapper format.

---

## Scenario 9: Orchestrator uses pkg.Source field directly
**ID**: scenario-9
**Testable after**: #1697, #1699
**Commands**:
- `grep -c 'pkg\.Source\|pkg\.source\|entry\.Source' internal/batch/orchestrator.go`
- `grep -cE 'fmt\.Sprintf.*homebrew:%s' internal/batch/orchestrator.go`
**Expected**: The orchestrator references the Source field from queue entries (count > 0). No hardcoded homebrew source construction patterns remain (count = 0). The `--from` flag in the exec.Command call uses the queue entry's Source value.
**Status**: pending

---

## Scenario 10: Orchestrator implements exponential backoff on failures
**ID**: scenario-10
**Testable after**: #1697, #1699
**Commands**:
- `go test -v ./internal/batch/... -run 'Backoff|Retry'`
**Expected**: Unit tests pass verifying the exponential backoff schedule: 1st failure has no delay, 2nd failure sets next_retry_at to +24 hours, 3rd failure sets +72 hours, 4th+ failures double the delay up to a 7-day maximum. The failure_count field increments on failure and resets to 0 on success.
**Status**: pending

---

## Scenario 11: Orchestrator skips packages with future next_retry_at
**ID**: scenario-11
**Testable after**: #1697, #1699
**Commands**:
- `go test -v ./internal/batch/... -run 'Select|Candidate|Skip'`
**Expected**: Unit tests pass verifying that selectCandidates() excludes packages whose next_retry_at is in the future. Packages with next_retry_at in the past or null are included in candidate selection.
**Status**: pending

---

## Scenario 12: Recipe merge workflow file exists with correct triggers
**ID**: scenario-12
**Testable after**: #1697, #1700
**Commands**:
- `test -f .github/workflows/update-queue-status.yml`
- `grep -q 'push:' .github/workflows/update-queue-status.yml`
- `grep -q 'recipes/' .github/workflows/update-queue-status.yml`
**Expected**: Workflow file `.github/workflows/update-queue-status.yml` exists, triggers on push to main, and has a path filter for the `recipes/` directory.
**Status**: passed
**Executed**: 2026-02-15
**Output**: Workflow file `.github/workflows/update-queue-status.yml` exists (204 lines). Triggers on `push:` to `branches: [main]` with path filter `'recipes/**'`. All three checks confirmed.

---

## Scenario 13: Recipe merge workflow extracts sources from recipe actions
**ID**: scenario-13
**Testable after**: #1697, #1700
**Commands**:
- `grep -cE '(github_archive|github_file|cargo_install|pipx_install|npm_install|gem_install|homebrew_bottle)' .github/workflows/update-queue-status.yml`
**Expected**: The workflow contains logic to extract source identifiers from at least four recipe action types (github_archive, cargo_install, pipx_install, homebrew_bottle). Each extraction maps the action to the `ecosystem:identifier` format used by queue entries.
**Status**: passed
**Executed**: 2026-02-15
**Output**: The workflow contains extraction logic for 8 action types across 6 ecosystem mappings: github_archive/github_file -> `github:<repo>`, cargo_install -> `cargo:<crate>`, pipx_install -> `pypi:<pkg>`, npm_install -> `npm:<pkg>`, gem_install -> `rubygems:<gem>`, homebrew/homebrew_bottle -> `homebrew:<formula>`. All four required types (github_archive, cargo_install, pipx_install, homebrew_bottle) are present plus four additional types.

---

## Scenario 14: Recipe merge workflow updates queue with source match logic
**ID**: scenario-14
**Testable after**: #1697, #1700
**Commands**:
- `grep -q 'priority-queue.json' .github/workflows/update-queue-status.yml`
- `grep -cE '(success|curated)' .github/workflows/update-queue-status.yml`
**Expected**: The workflow references `priority-queue.json` for updates. It implements the dual-path logic: when queue source matches a recipe source, status is set to "success"; when queue source does not match any recipe source, status is set to "success" AND confidence is set to "curated". The source field is never modified (preserved as historical provenance).
**Status**: passed
**Executed**: 2026-02-15
**Output**: Queue file referenced as `QUEUE_FILE="data/queues/priority-queue.json"` (lines 54, 179). Dual-path logic confirmed: source match sets `.status = "success"` (line 157); source mismatch sets `.status = "success" | .confidence = "curated"` (line 162). The `.source` field is never modified in any jq expression -- only `.status` and `.confidence` are written. Source field preserved as historical provenance.
