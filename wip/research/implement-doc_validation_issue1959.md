# Issue #1959 Validation Report

**Date**: 2026-03-01
**Tested Scenarios**: scenario-3, scenario-4, scenario-5, scenario-6
**All Passed**: Yes

## Executive Summary

All four testable scenarios for issue #1959 passed validation. The implementation of FlattenDependencies, GenerateFoundationDockerfile, and FoundationImageName functions is complete and correct.

## Test Results

### Scenario 3: FlattenDependencies returns empty slice for plans with no dependencies

**Status**: PASSED

**Test Command**:
```bash
go test ./internal/sandbox/ -run TestFlattenDependencies_Empty -v -count=1
```

**Output**:
```
=== RUN   TestFlattenDependencies_Empty
=== PAUSE TestFlattenDependencies_Empty
=== RUN   TestFlattenDependencies_EmptySlice
=== PAUSE TestFlattenDependencies_EmptySlice
=== CONT  TestFlattenDependencies_Empty
--- PASS: TestFlattenDependencies_Empty (0.00s)
=== CONT  TestFlattenDependencies_EmptySlice
--- PASS: TestFlattenDependencies_EmptySlice (0.00s)
PASS
ok  	github.com/tsukumogami/tsuku/internal/sandbox	0.003s
```

**Validation**: Function correctly returns a non-nil empty slice `[]FlatDep{}` for both nil and empty `Dependencies` fields. No panics or errors.

### Scenario 4: FlattenDependencies produces correct topological order with deduplication

**Status**: PASSED

**Test Command**:
```bash
go test ./internal/sandbox/ -run 'TestFlattenDependencies_(LeavesFirst|AlphabeticalSiblings|Deduplication|PreservesSubtree|StripsTimestamp)' -v -count=1
```

**Output**:
```
=== RUN   TestFlattenDependencies_LeavesFirst
=== PAUSE TestFlattenDependencies_LeavesFirst
=== RUN   TestFlattenDependencies_AlphabeticalSiblings
=== PAUSE TestFlattenDependencies_AlphabeticalSiblings
=== RUN   TestFlattenDependencies_Deduplication
=== PAUSE TestFlattenDependencies_Deduplication
=== RUN   TestFlattenDependencies_DeduplicationDifferentVersions
=== PAUSE TestFlattenDependencies_DeduplicationDifferentVersions
=== RUN   TestFlattenDependencies_PreservesSubtree
=== PAUSE TestFlattenDependencies_PreservesSubtree
=== RUN   TestFlattenDependencies_StripsTimestamp
=== PAUSE TestFlattenDependencies_StripsTimestamp
=== CONT  TestFlattenDependencies_LeavesFirst
--- PASS: TestFlattenDependencies_LeavesFirst (0.00s)
=== CONT  TestFlattenDependencies_AlphabeticalSiblings
--- PASS: TestFlattenDependencies_AlphabeticalSiblings (0.00s)
=== CONT  TestFlattenDependencies_StripsTimestamp
=== CONT  TestFlattenDependencies_DeduplicationDifferentVersions
=== CONT  TestFlattenDependencies_Deduplication
--- PASS: TestFlattenDependencies_StripsTimestamp (0.00s)
--- PASS: TestFlattenDependencies_Deduplication (0.00s)
=== CONT  TestFlattenDependencies_PreservesSubtree
--- PASS: TestFlattenDependencies_PreservesSubtree (0.00s)
--- PASS: TestFlattenDependencies_DeduplicationDifferentVersions (0.00s)
PASS
ok  	github.com/tsukumogami/tsuku/internal/sandbox	0.004s
```

**Validation**:
- LeavesFirst: Dependencies appear before their parents in the topological order
- AlphabeticalSiblings: Sibling dependencies at the same depth are sorted alphabetically
- Deduplication: Duplicate tools (same tool+version) are deduplicated to first occurrence
- DeduplicationDifferentVersions: Same tool with different versions are kept separate
- PreservesSubtree: Each converted plan retains its nested dependency subtree
- StripsTimestamp: GeneratedAt field is zeroed in all output plans

### Scenario 5: GenerateFoundationDockerfile produces valid Dockerfile structure

**Status**: PASSED

**Test Command**:
```bash
go test ./internal/sandbox/ -run 'TestGenerateFoundationDockerfile' -v -count=1
```

**Output**:
```
=== RUN   TestGenerateFoundationDockerfile_NoDeps
=== PAUSE TestGenerateFoundationDockerfile_NoDeps
=== RUN   TestGenerateFoundationDockerfile_SingleDep
=== PAUSE TestGenerateFoundationDockerfile_SingleDep
=== RUN   TestGenerateFoundationDockerfile_MultipleDeps
=== PAUSE TestGenerateFoundationDockerfile_MultipleDeps
=== RUN   TestGenerateFoundationDockerfile_ZeroPaddedIndex
=== PAUSE TestGenerateFoundationDockerfile_ZeroPaddedIndex
=== RUN   TestGenerateFoundationDockerfile_Deterministic
=== PAUSE TestGenerateFoundationDockerfile_Deterministic
=== CONT  TestGenerateFoundationDockerfile_NoDeps
--- PASS: TestGenerateFoundationDockerfile_NoDeps (0.00s)
=== CONT  TestGenerateFoundationDockerfile_SingleDep
--- PASS: TestGenerateFoundationDockerfile_SingleDep (0.00s)
=== CONT  TestGenerateFoundationDockerfile_Deterministic
=== CONT  TestGenerateFoundationDockerfile_ZeroPaddedIndex
=== CONT  TestGenerateFoundationDockerfile_MultipleDeps
--- PASS: TestGenerateFoundationDockerfile_Deterministic (0.00s)
--- PASS: TestGenerateFoundationDockerfile_ZeroPaddedIndex (0.00s)
--- PASS: TestGenerateFoundationDockerfile_MultipleDeps (0.00s)
PASS
ok  	github.com/tsukumogami/tsuku/internal/sandbox	0.003s
```

**Validation**:
- NoDeps: Handles empty dependencies correctly
- SingleDep and MultipleDeps: Correctly generates Dockerfile structure
- ZeroPaddedIndex: Dependency plan files are indexed with zero-padding (e.g., dep-00, dep-01)
- Deterministic: Same inputs always produce identical output

The generated Dockerfiles contain:
- `FROM` statement with package image
- `COPY tsuku /usr/local/bin/tsuku` instruction
- `TSUKU_HOME` and `PATH` environment variables
- Interleaved COPY+RUN pairs per dependency
- Cleanup instruction at the end

### Scenario 6: FoundationImageName produces deterministic content-based tags

**Status**: PASSED

**Test Command**:
```bash
go test ./internal/sandbox/ -run 'TestFoundationImageName' -v -count=1
```

**Output**:
```
=== RUN   TestFoundationImageName_Format
=== PAUSE TestFoundationImageName_Format
=== RUN   TestFoundationImageName_Deterministic
=== PAUSE TestFoundationImageName_Deterministic
=== RUN   TestFoundationImageName_SensitiveToContent
=== PAUSE TestFoundationImageName_SensitiveToContent
=== RUN   TestFoundationImageName_SensitiveToFamily
=== PAUSE TestFoundationImageName_SensitiveToFamily
=== RUN   TestFoundationImageName_MultipleCallsConsistent
=== PAUSE TestFoundationImageName_MultipleCallsConsistent
=== CONT  TestFoundationImageName_Format
--- PASS: TestFoundationImageName_Format (0.00s)
=== CONT  TestFoundationImageName_MultipleCallsConsistent
--- PASS: TestFoundationImageName_MultipleCallsConsistent (0.00s)
=== CONT  TestFoundationImageName_Deterministic
--- PASS: TestFoundationImageName_Deterministic (0.00s)
=== CONT  TestFoundationImageName_SensitiveToFamily
--- PASS: TestFoundationImageName_SensitiveToFamily (0.00s)
=== CONT  TestFoundationImageName_SensitiveToContent
--- PASS: TestFoundationImageName_SensitiveToContent (0.00s)
PASS
ok  	github.com/tsukumogami/tsuku/internal/sandbox	0.003s
```

**Validation**:
- Format: Output matches pattern `tsuku/sandbox-foundation:{family}-{16 hex chars}`
- Deterministic: Same family + same Dockerfile content always produces the same tag
- SensitiveToContent: Different Dockerfiles produce different tags
- SensitiveToFamily: Different families produce different tags
- MultipleCallsConsistent: Multiple calls with same inputs produce same tag

## Test Environment

- **Working Directory**: /home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku
- **Go Version**: Standard system Go (verified by test execution)
- **Test Framework**: Go's standard testing package
- **Execution Method**: Direct `go test` invocation without isolation (unit tests only)

## Conclusion

Issue #1959 implementation is complete and fully functional. All testable scenarios passed validation:
- Dependency flattening with correct topological ordering and deduplication works correctly
- Dockerfile generation produces valid, deterministic output
- Foundation image naming is deterministic and content-based

The feature is ready for integration with higher-level scenarios (7-12) that depend on this foundation.
