# setup-tsuku

A composite action that installs a pinned tsuku release in a GitHub Actions job. It replaces
`curl -fsSL https://get.tsuku.dev/now | bash` in CI, where the version and the script both come
from a live endpoint at run time.

What runs is decided by committed content:

1. It downloads `checksums.txt` from the fixed release URL and fails unless its sha256 equals the
   `checksums-sha256` input. The default is committed in `action.yml`.
2. It downloads `tsuku-<os>-<arch>` for the runner and fails unless its sha256 matches that
   asset's line in the verified `checksums.txt`.
3. Only then does it install the binary to `$TSUKU_HOME/bin` (default `$HOME/.tsuku/bin`), add
   `bin` and `tools/current` to `GITHUB_PATH`, and set `TSUKU_NO_TELEMETRY=1` for later steps
   through `GITHUB_ENV`.

A mismatch at either step exits non-zero before anything is installed or added to PATH. The log
shows both verification lines:

```
Verified checksums.txt for tsuku v0.15.2: sha256 1dadbf2b...
Verified tsuku-linux-amd64 against checksums.txt: sha256 5475cad8...
```

The hash is committed, not read from the release, because GitHub releases can have their assets
replaced after publication. A `checksums.txt` fetched at run time only proves the binary matches
whatever the release holds that day.

Linux and macOS on amd64 and arm64 are supported. The action needs `curl` and either `sha256sum`
or `shasum`, and no token.

## Usage

Pin the action by full commit sha. A tag or branch ref would let the action change under you,
which defeats the point.

```yaml
- name: Install tsuku
  uses: tsukumogami/tsuku/.github/actions/setup-tsuku@<40-character sha>  # installs tsuku 0.15.2
```

Later steps in the same job can run `tsuku` and anything tsuku installs.

The `version` and `checksums-sha256` inputs let a repository pin a different release. Set both
together or neither. Repositories that don't need a particular release use the defaults, so the
whole org moves together.

## Bumping tsuku

Bumps are by hand. The action lives in the tsuku repository, and tsuku's release tag is created
before that release's binaries exist, so no tag of this repository can name the release it
installs. That's also why callers pin a commit rather than a tag.

1. After a release's assets are published, get the new hash:

   ```bash
   gh release download v0.15.2 -R tsukumogami/tsuku -p checksums.txt -O - | sha256sum
   ```

   Check that the file lists `tsuku-linux-amd64`, `tsuku-linux-arm64`, `tsuku-darwin-amd64` and
   `tsuku-darwin-arm64`.
2. In `action.yml`, change the `version` and `checksums-sha256` defaults together, and update the
   version in the verification example above. Open a PR. The `Setup tsuku Action` workflow
   installs the new release on each supported runner and runs the wrong-checksum case.
3. Once it merges, update the pinned sha, and the version in the trailing comment, at each call
   site in the consumer repositories:

   ```bash
   grep -rn 'setup-tsuku@' .github/
   ```
