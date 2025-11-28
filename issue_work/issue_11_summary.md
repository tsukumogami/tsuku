# Issue 11 Summary

## What Was Implemented

Automated release workflow using GoReleaser with GitHub Actions, triggered on version tags (v*). When a tag is pushed, the workflow builds binaries for all supported platforms, generates checksums, and creates a GitHub release.

## Changes Made

- `.goreleaser.yaml`: GoReleaser configuration for multi-platform builds
  - Targets: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
  - Raw binary output (no archives) for install.sh compatibility
  - Binary naming: `tsuku-{os}-{arch}` pattern
  - SHA256 checksums in `checksums.txt`
  - Changelog from commit messages (excludes docs/test/chore)

- `.github/workflows/release.yml`: GitHub Actions workflow
  - Triggers on version tags (v*)
  - Uses goreleaser-action@v6 with GoReleaser v2
  - Requires `contents: write` permission for release creation

- `install.sh`: Updated to use combined checksums file
  - Downloads `checksums.txt` instead of individual `.sha256` files
  - Parses checksum by matching binary name in the file

## Key Decisions

- **GoReleaser over manual scripts**: Industry-standard tool with proven reliability
- **Raw binaries over archives**: Maintains backward compatibility with existing install.sh
- **Combined checksums.txt**: Standard GoReleaser format, simpler than individual files

## Trade-offs Accepted

- **No local dry-run verification**: GoReleaser not installed in development environment; will be verified on first real release via CI

## Test Coverage

- No new Go code tests (configuration files only)
- Workflow will be tested on first tag push

## Known Limitations

- Release workflow only runs on ubuntu-latest; cross-compilation handles platform differences
- Windows binaries not included (not in original requirements)

## Future Improvements

- Add Windows support if needed
- Consider adding Docker image builds
- Add changelog customization for release notes
