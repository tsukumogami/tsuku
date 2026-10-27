# release extension: tsuku

tsuku's declarations for shirabe's `/shirabe:release`, read when it cuts a
tsuku release.

## After release

Run in order once the release is published.

- `tsuku self-update`: replace the installed tsuku with the newest release.
  It exits 0 without changing anything when it already reports that version,
  or when GitHub doesn't list the new release as the latest yet, so the next
  check is what catches an update that didn't land.
- `test "$(tsuku --version)" = "tsuku version $RELEASE_VERSION"`: fail unless
  the binary on PATH reports the released version, which `tsuku --version`
  prints as `tsuku version X.Y.Z`.
