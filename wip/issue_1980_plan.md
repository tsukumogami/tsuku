# Issue 1980 Plan

## Approach
Queue-only fix: update 6 entries in priority-queue.json to prevent the batch pipeline from attempting to build library-only crates.

## Files to Modify
- `data/queues/priority-queue.json` — 6 queue entry updates

## Steps

### Step 1: Exclude pure-library crates
Set `status: "excluded"` for bitcoin, blake3, boring, bsdiff (4 entries).
blake3 is excluded because b3sum already covers the CLI use case.

### Step 2: Fix companion CLI crate sources
- bindgen: change source to `crates.io:bindgen-cli` (binary name `bindgen` stays correct)
- boringtun: change name to `boringtun-cli`, source to `crates.io:boringtun-cli`

### Step 3: Verify changes
- Confirm 4 entries have status "excluded"
- Confirm bindgen source is "crates.io:bindgen-cli"
- Confirm boringtun entry updated to name "boringtun-cli" with source "crates.io:boringtun-cli"
- Run go test to ensure no regressions

## Risks
- Low risk: data-only change to queue entries
- No code changes required

## Testing Strategy
- Verify JSON remains valid after edits
- Run `go test ./...` for baseline confirmation
- Visual inspection of the 6 modified entries
