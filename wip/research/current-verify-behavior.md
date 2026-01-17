# Current Verify Command Behavior

**Date:** 2026-01-17
**Purpose:** Document current state of `tsuku verify` for tools and libraries

## Tool Verification

### Visible Tools (verifyVisibleTool)

**Location:** `cmd/tsuku/verify.go` lines 115-228

| Step | Name | What It Does |
|------|------|--------------|
| 1 | Installation | Runs recipe's verify command with adjusted PATH |
| 2 | PATH | Checks $TSUKU_HOME/current is in user's PATH |
| 3 | Resolution | Uses `which` to verify binary resolves correctly |
| 4 | Integrity | Compares SHA256 checksums against stored values |

### Hidden Tools (verifyWithAbsolutePath)

**Location:** `cmd/tsuku/verify.go` lines 79-113

- Only runs Step 1 with absolute paths
- Steps 2-4 skipped (tools not in PATH by design)

### What's NOT Verified for Tools

- DT_NEEDED entries
- Runtime library availability
- Recursive dependency validation

## Library Verification

### Current Implementation

**Location:** `cmd/tsuku/verify.go` function `verifyLibrary`

| Tier | Name | Status | Description |
|------|------|--------|-------------|
| 1 | Header | ✅ Implemented | Parse ELF/Mach-O, validate format |
| 2 | Dependencies | ❌ Placeholder | Log "not yet implemented" |
| 3 | dlopen | ❌ Placeholder | Log "not yet implemented" |
| 4 | Integrity | ✅ Implemented | SHA256 checksum verification |

### Tier 1 Details (internal/verify/header.go)

```go
type HeaderInfo struct {
    Format       string   // "ELF" or "Mach-O"
    Architecture string   // "amd64", "arm64", etc.
    Type         string   // "shared", "executable", etc.
    Dependencies []string // DT_NEEDED or LC_LOAD_DYLIB entries
}

func ValidateHeader(path string) (*HeaderInfo, error)
```

**Already extracts dependencies** - this is the input for Tier 2.

### Tier 2 Integration Point

**Location:** `cmd/tsuku/verify.go` around line 308

```go
// Current code (placeholder)
printInfo("  Tier 2 (deps): not yet implemented\n")
```

**Expected Tier 2 call:**
```go
// Proposed integration
for _, libFile := range libFiles {
    info, _ := verify.ValidateHeader(libFile)
    if len(info.Dependencies) > 0 {
        results := verify.ValidateDependencies(libFile, info.Dependencies, libsDir)
        // Report results
    }
}
```

## Flags

| Flag | Scope | Purpose |
|------|-------|---------|
| `--integrity` | Libraries | Enable Tier 4 checksum verification |
| `--skip-dlopen` | Libraries | Skip Tier 3 (currently unused) |

## Key Differences: Tools vs Libraries

| Aspect | Tools | Libraries |
|--------|-------|-----------|
| Primary method | Execute and check output | Parse binary headers |
| Dependency check | None | Tier 2 (not yet implemented) |
| Recursive | No | No (planned) |
| Integrity | Checksums | Checksums (--integrity flag) |

## Gaps Summary

1. **Tools have no binary dep analysis** - only execution verification
2. **Libraries have no Tier 2/3** - design complete, not implemented
3. **No recursive verification** - neither tools nor libraries
4. **No soname → recipe mapping** - can't trace deps back to recipes
5. **No warning for undeclared deps** - can't compare binary vs recipe

## Design Documents

- `DESIGN-library-verification.md` - Overall verification strategy
- `DESIGN-library-verify-deps.md` - Tier 2 design (issue #948)
- `wip/research/tier2-design-tracker.md` - Current discussion state
