# Issue 1014 Implementation Plan

## Summary
Implement minimal dlopen verification skeleton: Rust helper with actual dlopen functionality, Go module for invocation, and integration point in verify flow.

## Files to Modify/Create

| File | Action | Purpose |
|------|--------|---------|
| `cmd/tsuku-dltest/Cargo.toml` | Modify | Add libc, serde, serde_json dependencies |
| `cmd/tsuku-dltest/src/main.rs` | Modify | Implement dlopen/dlerror/dlclose logic with JSON output |
| `internal/verify/dltest.go` | Create | DlopenResult struct, EnsureDltest(), InvokeDltest() |
| `internal/verify/dltest_test.go` | Create | Unit tests for JSON parsing |
| `cmd/tsuku/verify.go` | Modify | Add integration point for Level 3 (gated) |

## Implementation Steps

### Step 1: Update Rust Dependencies
Add to Cargo.toml:
- `libc = "0.2"` - for dlopen/dlclose/dlerror FFI
- `serde = { version = "1.0", features = ["derive"] }` - for JSON serialization
- `serde_json = "1.0"` - for JSON output

### Step 2: Implement Rust dlopen Logic
Replace stub main.rs with:
- `Output` struct with serde derive (path, ok, error fields)
- `try_load(path)` function: dlerror() clear -> dlopen -> check handle -> dlclose
- Main: iterate paths, collect results, output JSON array
- Exit codes: 0 (all pass), 1 (any fail), 2 (usage error)
- Key: call dlerror() BEFORE dlopen to clear stale errors (per design)

### Step 3: Create Go Module
Create `internal/verify/dltest.go`:
- `DlopenResult` struct matching JSON schema
- `var pinnedDltestVersion = "dev"` (for skeleton, accept any version)
- `EnsureDltest() (string, error)` - for skeleton, check common locations or return error
- `InvokeDltest(ctx, paths) ([]DlopenResult, error)` - exec helper, parse JSON

### Step 4: Add Go Unit Tests
Create `internal/verify/dltest_test.go`:
- Test JSON parsing with mock output (success case)
- Test JSON parsing with error case
- Test empty results handling

### Step 5: Add Integration Point
Modify `cmd/tsuku/verify.go`:
- Add placeholder for Level 3 in verifyLibrary function
- For skeleton: log "Level 3 (dlopen): not yet integrated" (similar to existing patterns)
- Accept paths from Tier 1/2 results

### Step 6: Validate
Run the validation script from the issue:
- Build Rust binary
- Test --version flag
- Test dlopen on libc.so.6
- Check Go module compiles
- Check required exports exist

## Testing Strategy
- Rust: cargo build --release succeeds, --version works, dlopen on libc works
- Go: unit tests for JSON parsing
- Integration: build/test cycle validates skeleton works

## Risks
- None significant - this is a skeleton with intentional scope limitations
- dlopen on test library might behave differently across systems (use libc.so.6 which is universal)

## Scope Limitations (per issue)
- No batch processing (single invocation is fine)
- No timeout handling (caller sets context timeout)
- No environment sanitization (trust caller)
- No path validation (trust caller)
- Hardcoded helper path acceptable
- Minimal error handling
