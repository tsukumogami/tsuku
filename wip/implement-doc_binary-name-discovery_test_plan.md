# Test Plan: binary-name-discovery

Generated from: docs/designs/DESIGN-binary-name-discovery.md
Issues covered: 6
Total scenarios: 18

---

## Scenario 1: Cargo builder reads bin_names from crates.io API for workspace crate
**ID**: scenario-1
**Testable after**: #1936
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestCargo' ./internal/builders/`
**Expected**: Unit tests pass confirming that `discoverExecutables()` reads `bin_names` from the crates.io version API response. Test cases for `sqlx-cli` (produces `sqlx`, `cargo-sqlx`), `probe-rs-tools` (produces `probe-rs`, `cargo-flash`, `cargo-embed`), and `fd-find` (produces `fd`) all pass using mock HTTP responses with `bin_names` arrays on version objects.
**Status**: pending

---

## Scenario 2: Cargo builder falls back to crate name when bin_names is empty or all versions are yanked
**ID**: scenario-2
**Testable after**: #1936
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestCargo.*Fallback|TestCargo.*Yanked|TestCargo.*Empty' ./internal/builders/`
**Expected**: When `bin_names` is empty (library-only crate) or all versions are yanked, the builder falls back to using the crate name as the executable. Warning messages are emitted. No panics or errors.
**Status**: pending

---

## Scenario 3: Cargo builder dead code is removed
**ID**: scenario-3
**Testable after**: #1936
**Category**: Infrastructure
**Commands**:
- `grep -E 'func.*buildCargoTomlURL|func.*fetchCargoTomlExecutables|type cargoToml struct|type cargoTomlBinSection struct|maxCargoTomlSize|cargoTomlFetchTimeout' /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go`
**Expected**: No matches found. The functions `buildCargoTomlURL()`, `fetchCargoTomlExecutables()`, and structs `cargoToml`, `cargoTomlBinSection`, plus the constants `maxCargoTomlSize` and `cargoTomlFetchTimeout` are all removed. The `github.com/BurntSushi/toml` import is removed from `cargo.go` if no other usage remains.
**Status**: pending

---

## Scenario 4: Cargo builder caches API response for BinaryNameProvider
**ID**: scenario-4
**Testable after**: #1936
**Category**: Infrastructure
**Commands**:
- `grep -E 'cachedCrateInfo|cachedResponse|lastCrateInfo|crateInfoCache' /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go`
**Expected**: At least one match found, confirming the builder caches the `fetchCrateInfo()` response so that `AuthoritativeBinaryNames()` (added in #1938) can access the `bin_names` data without re-fetching.
**Status**: pending

---

## Scenario 5: npm parseBinField handles string-type bin with unscoped package
**ID**: scenario-5
**Testable after**: #1937
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestParseBinField' ./internal/builders/`
**Expected**: When `bin` is a string value like `"./bin/tool.js"` and the package name is `my-tool`, `parseBinField()` returns `["my-tool"]`. The function's signature now accepts a `packageName string` parameter.
**Status**: pending

---

## Scenario 6: npm parseBinField handles string-type bin with scoped package
**ID**: scenario-6
**Testable after**: #1937
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestParseBinField.*scoped|TestParseBinField.*Scoped' ./internal/builders/`
**Expected**: When `bin` is a string and the package name is `@scope/tool`, `parseBinField()` strips the scope prefix and returns `["tool"]`. The returned name passes `isValidExecutableName()` validation.
**Status**: pending

---

## Scenario 7: BinaryNameProvider interface exists and is implemented by Cargo and npm builders
**ID**: scenario-7
**Testable after**: #1938
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestBinaryNameProvider|TestValidateBinaryNames' ./internal/builders/`
**Expected**: The `BinaryNameProvider` interface is defined with `AuthoritativeBinaryNames() []string`. Both `CargoBuilder` and `NpmBuilder` implement it. Unit tests confirm correct type assertion and method behavior with mock API responses.
**Status**: pending

---

## Scenario 8: Orchestrator validateBinaryNames corrects mismatched executables
**ID**: scenario-8
**Testable after**: #1938
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestOrchestrator.*BinaryName|TestValidateBinaryNames.*Mismatch' ./internal/builders/`
**Expected**: When a builder produces recipe executables that differ from the `BinaryNameProvider` output, the orchestrator's `validateBinaryNames()` step corrects the recipe's executable list before sandbox validation. A warning is logged and a telemetry event is emitted. When executables match, no correction is applied. When the builder does not implement `BinaryNameProvider`, the step is skipped. When the provider returns an empty slice, the step is also skipped.
**Status**: pending

---

## Scenario 9: PyPI wheel-based discovery extracts console_scripts
**ID**: scenario-9
**Testable after**: #1939
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestPyPI.*Wheel|TestPyPI.*EntryPoints|TestPyPI.*ConsoleScripts' ./internal/builders/`
**Expected**: The PyPI builder downloads the wheel artifact, reads the `entry_points.txt` file from the ZIP, and parses `[console_scripts]` to extract executable names. Test cases for `black` (produces `black`, `blackd`) and `httpie` (produces `http`, `https`) pass using mock HTTP servers with pre-built ZIP payloads.
**Status**: pending

---

## Scenario 10: PyPI builder falls back to pyproject.toml when wheel is unavailable
**ID**: scenario-10
**Testable after**: #1939
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestPyPI.*Fallback|TestPyPI.*NoWheel' ./internal/builders/`
**Expected**: When no wheel artifact is available in the PyPI API response, when the download fails, when the download exceeds the 20MB size limit, when the hash mismatches, or when `entry_points.txt` is missing from the wheel, the builder falls back to the existing `buildPyprojectURL()` / `fetchPyprojectExecutables()` path. The fallback functions remain in the codebase (not deleted).
**Status**: pending

---

## Scenario 11: Shared artifact download helper enforces security constraints
**ID**: scenario-11
**Testable after**: #1939
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestArtifact' ./internal/builders/`
**Expected**: The artifact download helper at `internal/builders/artifact.go` passes tests for: HTTPS-only enforcement (rejects HTTP URLs), configurable size limit rejection, SHA256 hash mismatch detection, content-type verification, and successful download with correct hash.
**Status**: pending

---

## Scenario 12: RubyGems gem-based discovery extracts executables from metadata.gz
**ID**: scenario-12
**Testable after**: #1940
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestGemBuilder.*Artifact|TestGemBuilder.*Metadata|TestGemBuilder.*Bundler' ./internal/builders/`
**Expected**: The gem builder downloads the `.gem` file, reads `metadata.gz` from the tar archive, decompresses and parses the YAML, and extracts the `executables` array. Test case for `bundler` produces `["bundle", "bundler"]`. All executable names are validated through `isValidExecutableName()`. Download is bounded at 50MB.
**Status**: pending

---

## Scenario 13: RubyGems builder falls back to gemspec-from-GitHub
**ID**: scenario-13
**Testable after**: #1940
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestGemBuilder.*Fallback' ./internal/builders/`
**Expected**: When `.gem` download fails, when `metadata.gz` is missing from the tar, or when YAML parsing fails, the builder falls back to the existing `buildGemspecURL()` / `fetchGemspecExecutables()` path. Fallback functions remain in the codebase.
**Status**: pending

---

## Scenario 14: Go builder discovers cmd/ binaries from module proxy ZIP
**ID**: scenario-14
**Testable after**: #1941
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestGo.*Proxy|TestGo.*DiscoverBinaries|TestGo.*Cmd' ./internal/builders/`
**Expected**: The Go builder's `discoverBinariesFromProxy()` fetches the module ZIP from the proxy, scans for `cmd/*/main.go` directory entries, and returns subdirectory names as binary targets. Test case for `golangci-lint` root module with mock ZIP containing `cmd/golangci-lint/main.go` produces `golangci-lint`. The builder does NOT implement `BinaryNameProvider`.
**Status**: pending

---

## Scenario 15: Go builder falls back to last-segment heuristic
**ID**: scenario-15
**Testable after**: #1941
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test -v -run 'TestGo.*Fallback|TestGo.*NoCmdDir|TestGo.*ProxyError' ./internal/builders/`
**Expected**: When the proxy returns non-200 for the ZIP download, when no `cmd/` directories exist in the ZIP, or when the ZIP exceeds the size limit, the builder falls back to `inferGoExecutableName()` (last path segment). A warning is emitted for the fallback. Existing `inferGoExecutableName()` function and all existing tests pass unchanged.
**Status**: pending

---

## Scenario 16: Full builder test suite passes with no regressions
**ID**: scenario-16
**Testable after**: #1936, #1937, #1938, #1939, #1940, #1941
**Category**: Infrastructure
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test ./internal/builders/...`
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && go test ./...`
**Expected**: All tests pass. No regressions in any builder. The full project test suite completes with exit code 0.
**Status**: pending

---

## Scenario 17: End-to-end Cargo binary name discovery for workspace monorepo crate
**ID**: scenario-17
**Testable after**: #1936, #1938
**Category**: Use-case
**Environment**: manual (requires network access to crates.io and sandbox container runtime)
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && make build-test`
- `./tsuku-test create sqlx-cli --from crates.io`
**Expected**: The generated recipe contains `executables: ["sqlx"]` (or `["sqlx", "cargo-sqlx"]` depending on the crate's `bin_names`) instead of the incorrect `["sqlx-cli"]`. The recipe's verify command references the correct binary name (`sqlx --version` or `cargo sqlx --version`). This validates the entire flow from crates.io API response through `discoverExecutables()` through `validateBinaryNames()` to the final recipe output. If the orchestrator's `validateBinaryNames()` fires (because the builder initially produced the wrong name and the provider corrected it), a warning is logged. If sandbox is available, the recipe should pass sandbox validation.
**Status**: pending

---

## Scenario 18: End-to-end npm binary name discovery for string-type bin package
**ID**: scenario-18
**Testable after**: #1937, #1938
**Category**: Use-case
**Environment**: manual (requires network access to npm registry and sandbox container runtime)
**Commands**:
- `cd /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku && make build-test`
- `./tsuku-test create typescript --from npm`
**Expected**: The generated recipe for `typescript` contains `executables: ["tsc", "tsserver"]` (from the map-type `bin` field). This validates the `parseBinField()` fix for the map path is not regressed, and that the `BinaryNameProvider` on the npm builder agrees with the recipe's executables. The verify command references one of the discovered binaries. If sandbox is available, the recipe should pass sandbox validation.
**Status**: pending
