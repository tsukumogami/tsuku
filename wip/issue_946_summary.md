# Issue 946 Summary

## What Was Implemented

Extended the library installation process to compute SHA256 checksums for all installed library files and store them in `state.json` for later integrity verification. This enables the `tsuku verify <library> --integrity` feature planned in issue #950.

## Changes Made

- `internal/install/checksum.go`: Added `ComputeLibraryChecksums()` function that walks a library directory, skips symlinks, and computes SHA256 checksums for all regular files using relative paths as keys.
- `internal/install/state_lib.go`: Added `SetLibraryChecksums()` method for atomic checksum storage.
- `internal/install/library.go`: Integrated checksum computation into `InstallLibrary()` after file copying. Errors log warnings but do not fail installation.
- `internal/install/checksum_test.go`: Added 5 unit tests for `ComputeLibraryChecksums()`.
- `internal/install/state_test.go`: Added 2 unit tests for `SetLibraryChecksums()`.

## Key Decisions

- **Separate function vs reusing ComputeBinaryChecksums()**: Created dedicated `ComputeLibraryChecksums()` because libraries need to walk directories (tools use explicit binary lists) and skip symlinks entirely (tools follow symlinks).
- **Non-blocking error handling**: Checksum errors are logged as warnings but do not fail installation, matching the existing tool behavior and allowing graceful degradation.
- **Using filepath.Walk with Lstat**: The Walk callback uses `os.Lstat()` to detect symlinks since Walk's `info` parameter follows symlinks via Stat.

## Trade-offs Accepted

- **Symlinks are not checksummed**: Only real files are checksummed. This is intentional since symlink chains (common in library packages like `libfoo.so -> libfoo.so.1 -> libfoo.so.1.0.0`) should only checksum the real file.

## Test Coverage

- New tests added: 7 (5 for ComputeLibraryChecksums, 2 for SetLibraryChecksums)
- All existing library tests continue to pass

## Known Limitations

- Pre-existing libraries (installed before this change) will not have checksums stored. They will show "integrity: unknown" until reinstalled.

## Future Improvements

- Issue #950 will implement the actual integrity verification using these stored checksums.
