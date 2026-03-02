# Issue 1968 Implementation Plan

## Summary

Add an `EnvFile()` path helper and `EnsureEnvFile()` method to Config, call it from `InstallWithOptions`, and remove the per-dependency `ENV PATH` lines from `GenerateFoundationDockerfile` since the global `ENV PATH` at line 116 already covers `tools/current` and `bin`.

## Approach

The env file (`$TSUKU_HOME/env`) is currently only created by `website/install.sh`. In containerized contexts (sandbox, CI, Docker-based builds), the install script never runs, so `$TSUKU_HOME/env` doesn't exist. This causes problems for anything that sources it.

The chosen approach adds env file creation to the `Config` package (where directory management already lives) and calls it from `InstallWithOptions` alongside the existing `EnsureDirectories()` call. This keeps the env file in sync with the directory structure and makes it available in every context where `tsuku install` runs, including inside sandbox containers.

For the Dockerfile simplification: since issue #1967 fixed `tools/current` symlinks, the per-dependency `ENV PATH` lines in `GenerateFoundationDockerfile` (line 127) are no longer needed. The global `ENV PATH` at line 116 already includes both `tools/current` and `bin`, which is sufficient because `install_binaries` creates symlinks in `tools/current` and the executor's skip logic finds pre-installed tools via state.json.

### Alternatives Considered

- **Alternative 1: Create env file in `EnsureDirectories()` instead of a separate method**: This would tightly couple directory creation with file creation. `EnsureDirectories` currently only creates directories (it's idempotent and side-effect-free regarding file content). Adding file writes would change its contract. Rejected because it conflates two distinct operations and makes testing harder.

- **Alternative 2: Create env file only in `cmd/tsuku/plan_install.go` at the top-level install command**: This would put the env file creation closer to where the user interacts. However, it would miss cases where `InstallWithOptions` is called from other paths (dependency installs, `install_deps.go`, future callers). Rejected because the install manager is the single funnel point for all installations.

- **Alternative 3: Replace `source $TSUKU_HOME/env` approach in foundation Dockerfile with Docker ENV directives only**: The foundation Dockerfile already uses `ENV` directives (which persist across layers). Since `source` in a `RUN` command doesn't persist to subsequent layers, using `ENV` is actually the correct Docker approach. The per-dep `ENV PATH` lines can simply be removed because the global `ENV PATH` already covers the right directories. This is the approach taken -- no need to add `source` to the Dockerfile at all.

## Files to Modify

- `internal/config/config.go` - Add `EnvFile()` path method and `EnsureEnvFile()` method
- `internal/config/config_test.go` - Add tests for `EnvFile()` and `EnsureEnvFile()`
- `internal/install/manager.go` - Call `EnsureEnvFile()` in `InstallWithOptions` after `EnsureDirectories()`
- `internal/sandbox/foundation.go` - Remove the per-dep `ENV PATH` line (line 127) from `GenerateFoundationDockerfile`
- `internal/sandbox/foundation_test.go` - Update tests that assert on per-dep `ENV PATH` lines

## Files to Create

None.

## Implementation Steps

- [ ] Add `EnvFile()` method to `Config` struct in `internal/config/config.go` that returns `filepath.Join(c.HomeDir, "env")`
- [ ] Add `EnsureEnvFile()` method to `Config` struct that writes the env file content matching `website/install.sh` format: `export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"`. The method should be idempotent (write if missing or content differs, skip if already correct).
- [ ] Add unit tests for `EnvFile()` and `EnsureEnvFile()` in `internal/config/config_test.go`: test path correctness, test file creation from scratch, test idempotent re-creation, test content matches expected format.
- [ ] Call `m.config.EnsureEnvFile()` in `InstallWithOptions` (internal/install/manager.go, line 62) right after the existing `EnsureDirectories()` call. Log a warning but don't fail the install if env file creation fails (the env file is a convenience, not critical to installation).
- [ ] Remove the per-dep `ENV PATH` line from `GenerateFoundationDockerfile` in `internal/sandbox/foundation.go`. Specifically, remove lines 122-127 (the comment and the `sb.WriteString(fmt.Sprintf("ENV PATH=..."))` inside the `for` loop). The COPY+RUN pair per dependency stays; only the ENV line is removed.
- [ ] Update foundation test expectations in `internal/sandbox/foundation_test.go`: `TestGenerateFoundationDockerfile_SingleDep` (remove expected `ENV PATH=/workspace/tsuku/tools/rust-1.82.0/bin:$PATH` line), `TestGenerateFoundationDockerfile_MultipleDeps` (remove assertions for per-dep ENV PATH lines for openssl and rust).
- [ ] Run `go test ./internal/config/ ./internal/install/ ./internal/sandbox/` to verify all tests pass.
- [ ] Run `go vet ./...` and `golangci-lint run --timeout=5m ./...` to verify no lint issues.
- [ ] Manual verification: Build tsuku and run `tsuku install --sandbox` on a cargo recipe with dependencies (e.g., `cargo-audit` which has `extra_dependencies = ["zig"]`). Confirm the sandbox test passes without per-dep ENV PATH lines.

## Testing Strategy

- **Unit tests (config)**:
  - `TestEnvFile` - verify path returns `$TSUKU_HOME/env`
  - `TestEnsureEnvFile_CreatesFile` - verify file is created with correct content
  - `TestEnsureEnvFile_Idempotent` - verify calling twice doesn't error, content stays the same
  - `TestEnsureEnvFile_ContentFormat` - verify the file starts with `# tsuku shell configuration` and contains the PATH export line

- **Unit tests (foundation)**:
  - Update `TestGenerateFoundationDockerfile_SingleDep` to expect COPY+RUN pairs without trailing ENV PATH
  - Update `TestGenerateFoundationDockerfile_MultipleDeps` to not assert on per-dep ENV PATH lines
  - Verify `TestGenerateFoundationDockerfile_NoDeps` still passes (no per-dep lines expected anyway)

- **Integration/manual tests**:
  - Build tsuku: `go build -o tsuku ./cmd/tsuku`
  - Install a tool and verify `$TSUKU_HOME/env` exists with correct content
  - Run `tsuku install --sandbox` on `cargo-audit` or `cargo-watch` (both have `extra_dependencies = ["zig"]`) to confirm foundation image works without per-dep ENV PATH lines

## Risks and Mitigations

- **Risk**: Env file format diverges from `website/install.sh` over time.
  **Mitigation**: Add a comment in both locations referencing the other, so future maintainers know to keep them in sync. The env file content uses `$TSUKU_HOME` variable expansion (not hardcoded paths), making it portable.

- **Risk**: Removing per-dep `ENV PATH` breaks tools that install binaries outside `tools/current`.
  **Mitigation**: The `install_binaries` action creates symlinks in `tools/current`, which is already on the global PATH. Tools like patchelf (used by `homebrew_relocate`) are found via `tools/current` symlinks. The per-dep PATH lines were only needed before #1967 fixed the `tools/current` symlink mechanism.

- **Risk**: `EnsureEnvFile` fails on read-only filesystems or permission issues inside containers.
  **Mitigation**: The env file write is non-fatal -- log a warning and continue. The sandbox already has the correct PATH set via Docker ENV directives and the sandbox script's `export PATH=...` line.

## Success Criteria

- [ ] `go test ./internal/config/ ./internal/install/ ./internal/sandbox/` passes with all new and updated tests
- [ ] `go vet ./...` reports no issues
- [ ] After `tsuku install <any-tool>`, the file `$TSUKU_HOME/env` exists and contains `export PATH="..."` with bin and tools/current entries
- [ ] `GenerateFoundationDockerfile` output contains no per-dep `ENV PATH` lines (only the global one at the top)
- [ ] `tsuku install --sandbox` on `cargo-audit` (which depends on zig via `extra_dependencies`) passes on at least one Linux family

## Open Questions

None. All design decisions are clear from the implementation context and issue discussion.
