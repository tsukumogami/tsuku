#!/usr/bin/env bash
# Installs a pinned tsuku release for a GitHub Actions job. See README.md.
#
# Inputs (environment):
#   TSUKU_VERSION           release to install, with or without a leading v
#   TSUKU_CHECKSUMS_SHA256  sha256 of that release's checksums.txt
#
# Order matters: nothing is written to TSUKU_HOME, GITHUB_PATH or GITHUB_ENV until
# both checksums.txt and the binary have been verified. A mismatch exits non-zero
# with the job's PATH untouched.

set -euo pipefail

repo="tsukumogami/tsuku"
version="${TSUKU_VERSION:-}"
version="${version#v}"
expected_list_sha="${TSUKU_CHECKSUMS_SHA256:-}"

if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "::error::setup-tsuku: version must look like 1.2.3, got '${TSUKU_VERSION:-}'"
  exit 1
fi
if ! [[ "$expected_list_sha" =~ ^[0-9a-f]{64}$ ]]; then
  echo "::error::setup-tsuku: checksums-sha256 must be 64 lowercase hex characters"
  exit 1
fi

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "::error::setup-tsuku: unsupported OS $(uname -s)"; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "::error::setup-tsuku: unsupported architecture $(uname -m)"; exit 1 ;;
esac
asset="tsuku-${os}-${arch}"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "::error::setup-tsuku: neither sha256sum nor shasum is on PATH" >&2
    return 1
  fi
}

workdir="$(mktemp -d "${RUNNER_TEMP:-/tmp}/setup-tsuku.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT

base="https://github.com/${repo}/releases/download/v${version}"

curl -fsSL --retry 3 -o "$workdir/checksums.txt" "$base/checksums.txt"
list_sha="$(sha256_of "$workdir/checksums.txt")"
if [ "$list_sha" != "$expected_list_sha" ]; then
  echo "::error::setup-tsuku: checksums.txt for v${version} has sha256 ${list_sha}, expected ${expected_list_sha}. Nothing was installed."
  exit 1
fi
echo "Verified checksums.txt for tsuku v${version}: sha256 ${list_sha}"

# Exactly one line must name the asset. awk matches the whole second field, so
# tsuku-linux-amd64 does not also match tsuku-dltest-linux-amd64 or a -musl variant.
matches="$(awk -v a="$asset" '$2 == a || $2 == "*" a {print $1}' "$workdir/checksums.txt")"
if [ -z "$matches" ] || [ "$(printf '%s\n' "$matches" | wc -l | tr -d ' ')" != "1" ]; then
  echo "::error::setup-tsuku: expected exactly one ${asset} line in checksums.txt for v${version}"
  exit 1
fi
expected_bin_sha="$matches"

curl -fsSL --retry 3 -o "$workdir/tsuku" "$base/$asset"
bin_sha="$(sha256_of "$workdir/tsuku")"
if [ "$bin_sha" != "$expected_bin_sha" ]; then
  echo "::error::setup-tsuku: ${asset} v${version} has sha256 ${bin_sha}, checksums.txt says ${expected_bin_sha}. Nothing was installed."
  exit 1
fi
echo "Verified ${asset} against checksums.txt: sha256 ${bin_sha}"

tsuku_home="${TSUKU_HOME:-$HOME/.tsuku}"
mkdir -p "$tsuku_home/bin"
install -m 0755 "$workdir/tsuku" "$tsuku_home/bin/tsuku"

if [ -n "${GITHUB_PATH:-}" ]; then
  echo "$tsuku_home/bin" >> "$GITHUB_PATH"
  echo "$tsuku_home/tools/current" >> "$GITHUB_PATH"
fi
if [ -n "${GITHUB_ENV:-}" ]; then
  echo "TSUKU_NO_TELEMETRY=1" >> "$GITHUB_ENV"
fi

TSUKU_NO_TELEMETRY=1 "$tsuku_home/bin/tsuku" --version
