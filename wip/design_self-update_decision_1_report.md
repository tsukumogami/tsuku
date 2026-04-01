<!-- decision:start id="self-update-asset-resolution" status="assumed" -->
### Decision: Self-Update Asset Resolution Strategy

**Context**

Tsuku needs a self-update command that resolves the latest stable release from
GitHub, downloads the correct platform binary, and verifies its checksum before
handing it to the replacement mechanism. GoReleaser produces binaries named
`tsuku-{os}-{arch}` (e.g., `tsuku-linux-amd64`, `tsuku-darwin-arm64`) alongside
a `checksums.txt` file containing SHA256 hashes for all assets. The existing
codebase has version resolution infrastructure (`internal/version`), asset
fetching and pattern matching (`FetchReleaseAssets`, `MatchAssetPattern`), and
download infrastructure (`download_file` action with HTTPS enforcement, retries,
and checksum verification).

The PRD (D5) requires self-update to be a separate, simple code path (~30 lines)
rather than treating tsuku as a managed tool. This rules out reusing the full
recipe/action pipeline but does not prevent calling lower-level library functions
from the version and download packages.

**Assumptions**

- GoReleaser's binary naming convention (`tsuku-{os}-{arch}`) will remain stable
  across releases. If it changes, the asset name construction would break, but
  since tsuku controls its own `.goreleaser.yaml`, this is low risk.
- The `checksums.txt` file will always be present in releases and will use SHA256.
  GoReleaser produces this by default and the config explicitly sets `algorithm: sha256`.
- GitHub releases will remain the sole distribution channel. If a CDN or mirror
  is added later, the resolution logic would need updating.

**Chosen: Direct asset name construction with checksums.txt parsing**

Construct the expected asset name deterministically from `runtime.GOOS` and
`runtime.GOARCH` using the known GoReleaser naming pattern, then download
`checksums.txt` to extract the expected SHA256 hash before downloading the binary.

The flow:

1. **Resolve latest version**: Use `GitHubProvider.ResolveLatest()` against
   `tsukumogami/tsuku`. This returns a `VersionInfo` with the tag (e.g., `v0.5.0`).
   Compare against `buildinfo.Version()` -- if equal, report "already up to date"
   and exit.

2. **Construct asset name**: Build the binary name as
   `fmt.Sprintf("tsuku-%s-%s", runtime.GOOS, runtime.GOARCH)`. This matches the
   GoReleaser template `tsuku-{{ .Os }}-{{ .Arch }}` exactly because Go's
   `runtime.GOOS`/`runtime.GOARCH` values are what GoReleaser uses.

3. **Construct download URLs**: Use the standard GitHub release download URL
   pattern:
   ```
   https://github.com/tsukumogami/tsuku/releases/download/{tag}/tsuku-{os}-{arch}
   https://github.com/tsukumogami/tsuku/releases/download/{tag}/checksums.txt
   ```

4. **Download and parse checksums.txt**: Fetch `checksums.txt` (small file, a few
   hundred bytes). Parse it line-by-line -- each line is `{sha256hash}  {filename}`.
   Find the line matching the target asset name and extract the hash.

5. **Download binary with verification**: Download the binary to a temp file. Use
   the existing `VerifyChecksum()` function from the actions package (or equivalent)
   to validate the SHA256 hash matches. If verification fails, delete the temp file
   and abort.

6. **Hand off to replacement mechanism**: Pass the verified temp file path to
   Decision 2's binary replacement logic.

This approach keeps the self-update code simple (under 30 lines for the core
logic, excluding helpers) because it avoids the asset discovery API call entirely
and computes everything from known conventions.

**Rationale**

Direct name construction is the right fit because tsuku controls both sides of
the equation -- the GoReleaser config that names assets and the code that looks
them up. There's no ambiguity about which asset to download. The naming convention
is a 1:1 mapping from Go's runtime constants to GoReleaser's template variables,
so construction is trivial and guaranteed correct.

Parsing `checksums.txt` is more reliable than the GitHub API for getting checksums:
it's a single small HTTP request (vs. paginated API calls), doesn't count against
API rate limits, and works without authentication. Since GoReleaser produces
`checksums.txt` by default with SHA256, this is the canonical source of truth
for integrity verification.

Using `GitHubProvider.ResolveLatest()` for version resolution reuses proven
infrastructure that already handles rate limits, network errors, pre-release
filtering, and tag normalization. Writing a separate resolution path would
duplicate all of that.

**Alternatives Considered**

- **GitHub Releases API asset listing**: Use `FetchReleaseAssets()` to list all
  assets in the release, then pattern-match with `MatchAssetPattern()` to find
  the right binary. This works but adds an unnecessary API call. Since we control
  the naming convention, discovery adds no value -- we already know the exact
  filename. The API call also counts against rate limits and requires parsing
  asset metadata to get download URLs. Rejected because it adds complexity and
  API dependency for no benefit.

- **Full recipe pipeline**: Define tsuku as a recipe and use the standard
  install/update path. This contradicts PRD decision D5 which explicitly chose a
  separate code path to avoid bootstrap risk (a broken recipe system can't update
  itself). It would also mean tsuku appears in `tsuku list` output alongside
  managed tools, blurring the line between the manager and the managed. Rejected
  per PRD constraint.

- **Hardcoded latest-release redirect**: Use GitHub's
  `https://github.com/{owner}/{repo}/releases/latest/download/{asset}` redirect
  URL to skip version resolution entirely. While simpler (no API call at all),
  this loses the ability to compare the remote version against the local version
  before downloading. Users would download the binary on every invocation even
  when already up to date. It also doesn't provide the version string needed for
  user-facing output ("Updated to v0.5.0"). Rejected because it sacrifices the
  "already up to date" check and version reporting.

**Consequences**

The self-update code couples to the GoReleaser naming convention. If the naming
template in `.goreleaser.yaml` changes, the self-update code must change too. This
is acceptable because both live in the same repository and would naturally be
updated together.

The `checksums.txt` parsing adds a small amount of code (parse lines, match
filename, extract hash) but eliminates the need for a separate checksum storage
mechanism or API call. The format is stable and widely used across GoReleaser
projects.

Version comparison uses string equality between `buildinfo.Version()` and the
resolved version. Both go through the same normalization (GoReleaser injects via
ldflags, resolver strips the `v` prefix), so this is reliable.
<!-- decision:end -->
