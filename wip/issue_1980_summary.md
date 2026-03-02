# Issue 1980 Summary

## What Was Implemented
Updated 6 queue entries in priority-queue.json that reference library-only crates (no binary targets). Four pure-library entries were excluded; two were corrected to point at their companion CLI crates.

## Changes Made
- `data/queues/priority-queue.json`: Updated 6 entries
  - bitcoin, blake3, boring, bsdiff: status changed to "excluded"
  - bindgen: source changed from `crates.io:bindgen` to `crates.io:bindgen-cli`
  - boringtun: renamed to `boringtun-cli`, source changed to `crates.io:boringtun-cli`

## Key Decisions
- blake3 excluded rather than deleted because `b3sum` recipe already covers this CLI
- boringtun entry renamed to `boringtun-cli` since the binary produced by that crate is `boringtun-cli`, not `boringtun`

## Trade-offs Accepted
- No prevention mechanism added (preflight check for library-only crates). The issue mentions this as future work but it wasn't in scope.

## Test Coverage
- No code changes; JSON data only
- Existing test suite passes unchanged

## Requirements Mapping

| AC | Status | Evidence / Reason |
|----|--------|-------------------|
| bindgen: change crate to bindgen-cli | Implemented | Queue source updated to crates.io:bindgen-cli |
| bitcoin: delete recipe | Implemented | Queue status set to excluded (no recipe file existed) |
| blake3: change crate to b3sum or delete | Implemented | Queue status set to excluded; b3sum.toml already exists |
| boring: delete recipe | Implemented | Queue status set to excluded |
| boringtun: change crate to boringtun-cli | Implemented | Queue entry renamed + source updated |
| bsdiff: delete recipe | Implemented | Queue status set to excluded |
