---
summary:
  constraints:
    - Must verify all 12 artifacts exist before publishing
    - Hardcoded artifact list (not dynamically generated)
    - Fail fast on first missing artifact
    - Generate checksums from downloaded artifacts (matches user downloads)
    - Release only publishes after all verification passes
  integration_points:
    - .github/workflows/release.yml (modify finalize-release job)
    - finalize-release already depends on integration-test
  risks:
    - Artifact names must exactly match what build jobs produce
    - sha256sum vs shasum differences between Linux and macOS (finalize runs on ubuntu)
  approach_notes: |
    Expand finalize-release job to:
    1. Download all artifacts from draft release
    2. Verify all 12 expected artifacts are present
    3. Generate SHA256 checksums.txt
    4. Upload checksums.txt to release
    5. Publish release (remove draft status)
---

# Implementation Context: Issue #1031

**Source**: docs/designs/DESIGN-release-workflow-native.md (Step 6)

## Expected Artifacts (12 total)

**glibc variants (8):**
- tsuku-linux-amd64, tsuku-linux-arm64, tsuku-darwin-amd64, tsuku-darwin-arm64
- tsuku-dltest-linux-amd64, tsuku-dltest-linux-arm64, tsuku-dltest-darwin-amd64, tsuku-dltest-darwin-arm64

**musl variants (4):**
- tsuku-linux-amd64-musl, tsuku-linux-arm64-musl
- tsuku-dltest-linux-amd64-musl, tsuku-dltest-linux-arm64-musl

## Verification Logic

From design doc:
```bash
EXPECTED=(
  # glibc variants
  tsuku-linux-amd64
  tsuku-linux-arm64
  tsuku-darwin-amd64
  tsuku-darwin-arm64
  tsuku-dltest-linux-amd64
  tsuku-dltest-linux-arm64
  tsuku-dltest-darwin-amd64
  tsuku-dltest-darwin-arm64
  # musl variants
  tsuku-linux-amd64-musl
  tsuku-linux-arm64-musl
  tsuku-dltest-linux-amd64-musl
  tsuku-dltest-linux-arm64-musl
)
for binary in "${EXPECTED[@]}"; do
  if ! gh release view "$TAG" --json assets -q ".assets[].name" | grep -q "^${binary}$"; then
    echo "ERROR: Missing artifact: $binary"
    exit 1
  fi
done
```

## Checksum Generation

After verification, generate checksums:
```bash
sha256sum * > checksums.txt
gh release upload "$TAG" checksums.txt --clobber
```

Users verify with:
```bash
shasum -a 256 -c checksums.txt
```

## Current finalize-release Structure

```yaml
finalize-release:
  name: Finalize Release
  needs: [integration-test]
  if: success()
  runs-on: ubuntu-latest
  steps:
    - name: Publish release
      env:
        GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      run: |
        TAG="${GITHUB_REF_NAME}"
        gh release edit "$TAG" --draft=false
        echo "Published release $TAG"
```
