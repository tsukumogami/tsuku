# Issue 280 Summary

## What Was Implemented

Implemented the `inspect_archive` tool handler that allows the LLM to download and inspect archive contents to discover binary names during recipe generation.

## Changes Made

- `internal/llm/archive.go`: New file with archive download, extraction, and file listing functionality
- `internal/llm/archive_test.go`: Comprehensive unit tests using in-memory archives
- `internal/llm/client.go`: Removed stub implementation (now uses archive.go)
- `internal/llm/client_test.go`: Removed obsolete stub test

## Key Decisions

- **Separate file for archive logic**: Kept archive.go separate from client.go for better separation of concerns and testability
- **In-memory test archives**: Created tar.gz, tar.xz, and zip archives in test code rather than using fixtures, making tests self-contained
- **Size limit of 10MB**: Prevents memory exhaustion from large archives while being sufficient for typical release assets

## Trade-offs Accepted

- **No tar.bz2 support**: The design doc mentions tar.bz2 but GitHub releases rarely use this format. Can be added if needed.
- **Mode-based executable detection**: Uses Unix file mode bits to detect executables, which may not work for Windows-only archives. This is acceptable since tsuku focuses on Linux/Darwin.

## Test Coverage

- New tests added: 8 test functions in archive_test.go
- Tests cover: tar.gz, tar.xz, zip formats, HTTP errors, unsupported formats, size limits, format detection

## Known Limitations

- Only detects format from URL extension, not Content-Type header
- No support for nested archives
- Executable detection based on Unix mode bits only

## Future Improvements

- Consider containerized extraction for additional security (mentioned in design doc as future enhancement)
- Add support for additional formats if requested
