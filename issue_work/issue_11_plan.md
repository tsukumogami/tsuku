# Issue 11 Implementation Plan

## Summary

Implement automated release workflow using GoReleaser with GitHub Actions, triggered on version tags (e.g., `v0.1.0`).

## Approach

Use GoReleaser for cross-compilation and release management. GoReleaser is the de facto standard for Go releases, providing:
- Multi-platform builds with a single configuration
- Automatic checksum generation
- GitHub Release creation and asset uploads
- Changelog generation from commits

### Alternatives Considered
- **Manual GOOS/GOARCH cross-compilation**: More control but requires more maintenance, custom scripts for checksums and uploads. Not chosen due to higher maintenance burden.
- **Custom GitHub Actions workflow**: Possible but reinvents features GoReleaser provides out of the box. Not chosen because GoReleaser is well-tested and widely adopted.

## Files to Create

- `.goreleaser.yaml` - GoReleaser configuration defining build targets, archive formats, and checksum settings
- `.github/workflows/release.yml` - GitHub Actions workflow to run GoReleaser on version tags

## Implementation Steps

- [ ] Create `.goreleaser.yaml` with multi-platform build configuration
- [ ] Create `.github/workflows/release.yml` workflow
- [ ] Test with a dry-run (snapshot) to verify configuration

## Configuration Details

### Target Platforms (per issue requirements)
- linux/amd64
- linux/arm64
- darwin/amd64
- darwin/arm64

### Binary Naming Convention (to match install.sh expectations)
The existing `install.sh` expects binaries named `tsuku-${OS}-${ARCH}` (e.g., `tsuku-linux-amd64`).

GoReleaser's default archive naming differs, but the archives contain the binary itself. The install.sh would need to be updated to handle archives, OR we can use GoReleaser's `binary` uploads (raw binaries without archives).

**Chosen approach**: Use raw binary uploads (no archives) to maintain compatibility with existing `install.sh`. Configure GoReleaser to upload bare binaries named to match the expected pattern.

### Checksum Generation
GoReleaser auto-generates checksums. The install.sh expects individual `.sha256` files per binary. GoReleaser generates a single `checksums.txt` by default, but individual checksums can be achieved with post-hooks or adjusting install.sh to use the combined checksum file.

**Chosen approach**: Generate combined `checksums.txt` and update install.sh to parse it.

## Testing Strategy

- **Local dry-run**: Run `goreleaser release --snapshot --clean` locally to verify builds
- **Manual verification**: After first release, test install.sh against the created release

## Risks and Mitigations

- **Install.sh compatibility**: Binary naming must match what install.sh expects. Mitigation: Configure GoReleaser binary naming or update install.sh to handle archives.
- **Checksum format**: Install.sh expects individual `.sha256` files. Mitigation: Update install.sh to parse `checksums.txt`.

## Success Criteria

- [ ] Workflow triggers on version tags (v*)
- [ ] Builds for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
- [ ] Generates SHA256 checksums
- [ ] Creates GitHub release with all artifacts
- [ ] install.sh can download and install from release

## Open Questions

None - requirements are clear from the issue description.
