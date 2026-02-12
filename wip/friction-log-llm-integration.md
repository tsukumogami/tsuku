# Friction Log: Using tsuku to Build tsuku-llm

**Date**: 2026-02-12
**Context**: Attempting to build the Rust tsuku-llm addon and run integration tests

## Frictions Encountered

### 1. PATH Configuration After Installation

**What happened**: After `tsuku install rust`, the output said:
```
To use the installed tool, add this to your shell profile:
  export PATH="/home/dangazineu/.tsuku/tools/current:$PATH"
```

But:
- `~/.tsuku/bin/cargo` didn't exist
- Had to use full path: `~/.tsuku/tools/rust-1.93.0/bin/cargo`
- The `~/.tsuku/tools/current` directory wasn't mentioned in the output

**Expected**: Either symlinks in `~/.tsuku/bin/` or clearer guidance on where binaries are.

**Impact**: Had to manually find the binary location.

### 2. No protobuf/protoc Recipe

**What happened**: Building tsuku-llm requires `protoc` (protobuf compiler). Tried:
```bash
tsuku install protobuf  # "recipe not found"
tsuku install protoc    # "recipe not found"
tsuku search proto      # "No cached recipes found"
```

**Workaround**: Had to manually download protoc from GitHub releases:
```bash
curl -sL "https://github.com/protocolbuffers/protobuf/releases/download/v28.3/protoc-28.3-linux-x86_64.zip" -o /tmp/protoc.zip
unzip -o /tmp/protoc.zip -d /tmp/protoc
```

**Expected**: A `protoc` or `protobuf` recipe in the registry.

**Impact**: Can't fully self-contain the build process with tsuku.

### 3. tsuku Binary Not in Agent's PATH

**What happened**: User confirmed `tsuku` is in their PATH, but my bash environment couldn't find it.
```bash
which tsuku  # empty
tsuku search rust  # "command not found"
```

Had to use: `~/.tsuku/bin/tsuku`

**Root cause**: My bash environment doesn't inherit the user's shell profile PATH modifications.

**Impact**: Confusing UX when tsuku is installed but not accessible.

### 4. Integration Tests Compilation Error

**What happened**: After building tsuku-llm, running integration tests failed:
```
internal/llm/lifecycle_integration_test.go:6:2: "context" imported and not used
```

**Root cause**: Unused import in the test file I wrote.

**Impact**: Minor - just needed to remove the import.

### 5. Integration Tests Still Failing

**What happened**: After fixing the import, tests still show FAIL but the specific failure isn't clear from the truncated output. Need to investigate which test is failing and why.

## Recommendations

1. **PATH handling**: Consider creating symlinks in `~/.tsuku/bin/` for installed tools, or make the `current` symlink pattern clearer
2. **Add protoc recipe**: Common dependency for gRPC projects
3. **Improve install output**: Show the actual binary locations created
