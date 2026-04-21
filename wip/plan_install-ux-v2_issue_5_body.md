---
complexity: testable
complexity_rationale: Tests exercise three independent correctness properties; incorrect implementation of issues 1–4 would go undetected without explicit assertions
---

## Goal

Add property-based tests across three files verifying the three correctness properties from the test scope: (1) Manager produces no stdout when reporter is set, (2) action sub-steps go to the right Reporter channel (Status/Log/silence), (3) a shared reporter instance is used across recursive installs and Stop is called exactly once at the top level. Also update existing `TestNonTTYInstallLogLines` assertions to match the reduced Log count after issues 2–4.

## Test map

**`internal/install/manager_test.go`**
- `TestManagerGetReporter_NilReturnsNoop` — `getReporter()` on a freshly constructed Manager returns `progress.NoopReporter{}`
- `TestManagerInstallWithOptions_NoStdoutEscape` — redirect `os.Stdout` to a pipe; call `InstallWithOptions` with a pre-staged temp dir and a testReporter; assert 0 bytes written to the pipe and `testReporter.Logs` is empty (emoji/path lines are silenced)

**`internal/executor/install_output_test.go`** (new scenarios, existing `testReporter` reused)
- `TestExtractActionReporterClassification` — plan with extract step on a local fixture tarball; assert `hasStatus("Extracting:")`, assert `!hasLog("Extracting:")`, `!hasLog("Format:")`, `!hasLog("Strip dirs:")`
- `TestRunCommandReporterClassification` — plan with `run_command` running `echo hello`; assert `hasStatus("Running:")`, `!hasLog("Running:")`, `!hasLog("Description:")`, `!hasLog("Command executed successfully")`, `hasLog("Output:")`
- `TestInstallBinariesReporterClassification` — plan with `install_binaries` for one file; assert no per-file "Installed" lines appear in `Logs`
- `TestLinkDependenciesReporterClassification` — plan with `link_dependencies` on a pre-staged libs/ dir; assert per-file lines absent from `Logs`; assert bulk count line present in `Logs`
- Update `TestNonTTYInstallLogLines` — remove any assertions on lines that are now Status-only or silence

**`cmd/tsuku/install_deps_test.go`** (extend testReporter to track StopCount)
- `TestInstallWithDependencies_SingleReporterStop` — set up one pre-installed dep; call `installWithDependencies` with a testReporter tracking `StopCount`; assert `StopCount == 0` after return (Stop is owned by the caller, not the recursive function)
- `TestInstallWithDependencies_NoStdoutEscape` — same setup, redirect `os.Stdout` to pipe; assert 0 bytes

## Acceptance Criteria

- All eight new test functions exist and pass
- `TestNonTTYInstallLogLines` updated to not assert on Status-only lines
- `testReporter` in `install_output_test.go` has `StopCount` field incremented by `Stop()`
- `go test ./...` passes with no new failures

## Dependencies

<<ISSUE:1>>, <<ISSUE:2>>, <<ISSUE:3>>, <<ISSUE:4>>
