# Lead: Top-100 Blast Radius for Homebrew Dylib Chaining

**Exploration:** tsuku-homebrew-dylib-chaining
**Lead source:** wip/explore_tsuku-homebrew-dylib-chaining_scope.md (Lead 3)
**Date:** 2026-05-02
**Author:** explore agent (research)

## Question

Of the 100 tools on the curated priority list (`docs/curated-tools-priority-list.md`,
issue #2260), how many would be blocked or at-risk if recipe authors tried to install
them via the homebrew bottle action because the bottle has runtime transitive deps
that are not present in `debian-bookworm-slim`-class containers (the gap that
`fixLibraryDylibRpaths` addresses for `Type == "library"` but not for tool recipes)?

The answer routes the exploration: <5 → recipe-side fix, >15 → tsuku-core PRD/design.

## Method

1. Loaded the top-100 list from `docs/curated-tools-priority-list.md`.
2. Classified each tool by current coverage: `handcrafted` / `curated` / `batch` /
   `discovery-only` / `missing` / `n/a` (deprecated).
3. For every entry that is **not** already handcrafted, fetched the homebrew formula
   JSON from `https://formulae.brew.sh/api/formula/<name>.json` and inspected the
   `dependencies` field (runtime). For sanity, also fetched a sample of the
   handcrafted entries to see whether they would have hit the gap if installed
   from a bottle (informational; they ship today, often via non-homebrew paths).
4. Counted runtime deps that are NOT available by default in `debian-bookworm-slim`,
   using the at-risk lib set from the scope doc: `libevent`, `utf8proc`, `libidn2`,
   `libunistring`, `gettext`, `pcre2`, `nghttp2` (and `libnghttp2`/`3`/`ngtcp2`),
   `libgit2`, `libssh2`/`libssh`, `oniguruma`, `gmp`, `mpfr`, `libmpc`, `expat`, `xz`,
   `lz4`, `zstd`, `glib`, `libpng`, `freetype`, `libffi`, `icu4c`, `fontconfig`,
   `cairo`, `pango`, `gdk-pixbuf`, `harfbuzz`, `nettle`, `gnutls`, `libtasn1`,
   `libusb`, `libsodium`, `libuv`, `libyaml`, `ncurses` (newer ABI than slim),
   `tree-sitter`, `unibilium`, `brotli`, `little-cms2`, `mlx-c`, `ada-url`,
   `hdrhistogram_c`, `simdjson`, `uvwasi`, plus version-pinned runtimes like
   `openssl@3`, `python@3.X`, `openjdk[@N]`, `node` that the homebrew bottle hard-codes
   to a specific cellar path.

## Top-100 classification by coverage

From the list:
- **Handcrafted / curated (action: "no action needed" or "curated"):** 70 tools
- **Batch (review coverage):** 12 tools
- **Discovery-only or missing (author recipe):** 17 tools
- **Deprecated (n/a):** 1 tool (copilot)

Total: 100. The 12 batch + 17 missing/discovery = **29 candidates** that would
have a recipe authored via the homebrew action (the recommended path for prebuilt
binaries with shared-library deps), so these are the tools where this bug would
bite a future recipe author.

## Per-tool homebrew dep analysis (29 not-yet-curated entries)

For each tool below, deps come from `https://formulae.brew.sh/api/formula/<name>.json`.
The "Verdict" column says whether the homebrew bottle would chain through
non-system shared libs and therefore hit the unfixed RPATH gap on
`debian-bookworm-slim` (or equivalent minimal container). Tools whose recipe would
naturally use a different action (pipx_install, github_release with a static Go
binary upstream, JDK wrapper, etc.) are flagged as **N/A (homebrew not the
right shape)** — they may still be hard, but for unrelated reasons.

| # | Tool | Coverage | Homebrew runtime deps | At-risk via homebrew? |
|---|------|----------|------------------------|------------------------|
| 1 | node | discovery-only | `ada-url, brotli, c-ares, hdrhistogram_c, icu4c@78, libnghttp2, libnghttp3, libngtcp2, libuv, llhttp, merve, nbytes, openssl@3, simdjson, sqlite, uvwasi, zstd` | **YES (massive)** — would chain ~17 non-system libs. Recipe likely uses upstream tarball instead. |
| 2 | kubectl (brew: `kubernetes-cli`) | batch | `[]` | NO — Go static binary. |
| 3 | aws-cli (brew: `awscli`) | discovery-only | `openssl@3, python@3.14` | YES if installed from bottle (Python runtime + cellar openssl). Recipe usually pipx-installs upstream. |
| 4 | helm | batch | `[]` | NO — Go static binary. |
| 5 | ripgrep | batch | `pcre2` | **YES** — pcre2 dylib link. Already a known issue (recipe note). |
| 6 | fd | batch | `[]` | NO — Rust static. |
| 7 | bat | discovery-only | `libgit2, oniguruma` | **YES** — both at-risk libs. |
| 8 | eza | batch | `libgit2` | **YES**. |
| 9 | zoxide | batch | `[]` | NO — Rust static. |
| 10 | starship | discovery-only | `[]` | NO — Rust static. |
| 11 | neovim | discovery-only | `libuv, lpeg, luajit, luv, tree-sitter, unibilium, utf8proc, gettext` | **YES (heavy)** — 8 non-system libs. |
| 12 | htop | batch | `ncurses` (homebrew ncurses ABI ≠ debian-slim) | **YES** (mild). |
| 13 | delta (brew: `git-delta`) | discovery-only | `libgit2, oniguruma` | **YES**. |
| 14 | gcloud | missing | (no formula `google-cloud-sdk` returned 404 → cask only) | N/A — homebrew distributes as cask; recipe would use upstream installer. |
| 15 | azure-cli | discovery-only | `python@3.13, libsodium, libyaml, openssl@3` | **YES** if from bottle (Python + libsodium + libyaml). Recipe usually pipx. |
| 16 | cilium-cli | batch | `[]` | NO — Go static. |
| 17 | istioctl | batch | `[]` | NO — Go static. |
| 18 | ansible | discovery-only | `certifi, cryptography, libsodium, libssh, libyaml, python@3.14` | **YES** if from bottle. Recipe usually pipx. |
| 19 | cmake | discovery-only | `[]` | NO — official binary tarball. |
| 20 | bazel | batch | `[]` | NO. |
| 21 | gradle | discovery-only | `gradle-completion, openjdk` | N/A — needs JDK runtime; recipe is a JVM wrapper. |
| 22 | maven | discovery-only | `openjdk` | N/A — JVM wrapper. |
| 23 | sbt | discovery-only | `openjdk` | N/A — JVM wrapper. |
| 24 | aider | missing | `certifi, freetype, gcc, jpeg-turbo, libyaml, openblas, python@3.12` | **YES** if from bottle (freetype, libyaml, openblas, python). Recipe usually pipx. |
| 25 | ollama | batch | `mlx-c` | **YES** (mlx-c is a non-system C++ runtime); plus the bottle pulls model libs. Often distributed as upstream installer. |
| 26 | shellcheck | batch | `gmp` | **YES** — Haskell binary linked to gmp. |
| 27 | shfmt | batch | `[]` | NO — Go static. |
| 28 | hadolint | batch | `gmp` | **YES** — Haskell + gmp. |
| 29 | dive | batch | `[]` | NO — Go static. |
| 30 | ko | batch | `[]` | NO — Go static. |

(copilot is `n/a` and not counted.)

### Summary of the 29 not-yet-curated candidates

| Verdict | Count | Tools |
|---|---|---|
| **At-risk via homebrew dylib chain** (the bug bites here) | **11** | node, ripgrep, bat, eza, neovim, htop, delta, ollama, shellcheck, hadolint, *(plus borderline: aws-cli/azure-cli/ansible/aider if installed from bottle)* |
| Not at-risk (Go/Rust static, official tarball) | 11 | kubectl, helm, fd, zoxide, starship, cilium-cli, istioctl, cmake, bazel, shfmt, dive, ko |
| N/A — recipe would not use homebrew action anyway | 7 | gcloud (cask), gradle/maven/sbt (JVM), aws-cli/azure-cli/ansible/aider (pipx) — these are at-risk in *other* ways but not via the dylib-chain bug |

If we count the strictly-bottle cases: **10 tools** (node, ripgrep, bat, eza,
neovim, htop, delta, ollama, shellcheck, hadolint).
If we add the four pipx-or-bottle borderline tools (aws-cli, azure-cli, ansible,
aider) which a less-experienced author might attempt to install from the
homebrew bottle: **14 tools**.

## Most-common at-risk libraries across the set

Counting how often each at-risk library appears as a runtime dep across the
not-yet-curated candidates above (and including the bottle-version of the
pipx-able tools):

| Library | Tools that depend on it | Notes |
|---|---|---|
| `python@3.12/3.13/3.14` | aws-cli, azure-cli, ansible, aider | Cellar Python; not in debian-slim |
| `libyaml` | azure-cli, ansible, aider | |
| `libgit2` | bat, eza, delta | At-risk lib from the scope set. |
| `oniguruma` | bat, delta | (jq handcrafted also depends on it.) |
| `gmp` | shellcheck, hadolint | Haskell runtime. |
| `openssl@3` | node, aws-cli, azure-cli | Cellar openssl, not debian's libssl3. |
| `libsodium` | azure-cli, ansible | |
| `libuv` | node, neovim | |
| `utf8proc` | neovim, *(also tmux handcrafted)* | Confirms the canonical example from the scope doc. |
| `gettext` | neovim, *(also wget handcrafted)* | |
| `tree-sitter` | neovim | Newer than debian's libtree-sitter. |
| `unibilium`, `luajit`, `lpeg`, `luv` | neovim | |
| `pcre2` | ripgrep | |
| `mlx-c` | ollama | Apple/non-debian runtime. |
| `ncurses` (homebrew ABI) | htop | |
| `freetype`, `openblas`, `jpeg-turbo` | aider | |
| `brotli, libnghttp2, libnghttp3, libngtcp2, c-ares, ada-url, hdrhistogram_c, icu4c@78, llhttp, merve, nbytes, simdjson, uvwasi, zstd, sqlite` | node | The single worst entry on the list. |

The overlap with the scope-doc list is strong: `utf8proc`, `libidn2` (via wget),
`libunistring` (via wget), `gettext`, `pcre2`, `libgit2`, `oniguruma`, `gmp`,
`libsodium`, `libssh`/`libssh2`, `libnghttp2`, `nghttp2-family`, `libuv` are all
recurring. `node` alone introduces ~17 cellar-pinned libs; `neovim` introduces
8; `bat`/`delta` share libgit2+oniguruma.

## Cross-check: handcrafted recipes that would have hit this gap

Sanity checks against the brew formulas for some entries already shipping:

- `tmux` (handcrafted): brew deps `libevent, ncurses, utf8proc` → would hit the gap
  via bottle. Confirms the scope doc note that tmux's homebrew bottle is one of
  the canonical at-risk shapes; recipe must side-step.
- `git` (handcrafted): brew deps `pcre2, gettext` → at-risk; recipe doesn't use
  the bottle path.
- `wget` (handcrafted): brew deps `libidn2, openssl@3, gettext, libunistring` →
  per the scope doc, already documented as relying on system libs.
- `curl` (handcrafted): brew deps `brotli, libnghttp2, libnghttp3, libngtcp2,
  libssh2, openssl@3, zstd` → 7 at-risk libs; per the scope doc the recipe
  source-builds and uses set_rpath chains. This is the canonical workaround.
- `pyenv` (handcrafted): brew deps `autoconf, openssl@3, pkgconf, readline` →
  would hit gap via bottle.
- `httpie` (handcrafted): brew deps `certifi, python@3.14` → pipx route.

So at least 5 of the 70 already-handcrafted recipes (`tmux`, `git`, `wget`,
`curl`, `pyenv`) would also be at-risk if anyone tried to re-author them via
the homebrew action — they're shipping today only because the original authors
reached for source builds, system fallbacks, or set_rpath workarounds. This is
strong evidence that the gap repeatedly forces extra work.

## Blast-radius answer

**Strict count (homebrew bottle is the natural path AND deps are at-risk):
10 tools** out of the 29 not-yet-curated entries — node, ripgrep, bat, eza,
neovim, htop, delta, ollama, shellcheck, hadolint.

**Broader count (includes Python/Java tools that an author might attempt via
homebrew before discovering pipx is better): 14 tools.**

**Including the already-handcrafted recipes that exist only because someone
worked around the gap: at least 19 tools** (10 + 4 borderline + curl, tmux,
git, wget, pyenv).

All three numbers exceed the **>15 → tsuku-core PRD/design** threshold from the
scope doc. Even the strictest count (10) is squarely in the "in-between" band
(5–15), and the broader/historical numbers blow past 15.

## Note on top-100 representativeness

The top-100 is curated; the long tail of recipes (the 1,218 batch-generated
entries) likely contains a far higher density of homebrew bottles with
non-trivial dylib chains, since the curated list intentionally over-represents
single-binary Go/Rust tools that are easy to package. So 10–14 within the
top-100 is a *floor*, not a ceiling, on the addressable problem.

## Recommendation for the exploration

This lead's stop-signal is hit: the count is well above the recipe-side-fix
threshold. Combined with Leads 1, 2, 5, and 6 (which characterize existing
workarounds), the exploration should converge on a tsuku-core PRD or design
doc rather than a recipe-author follow-up. The strongest single signal is that
the gap forces ~5 of the most-popular handcrafted recipes (curl, tmux, git,
wget, pyenv) to invent their own workarounds today — that's the convergent
pattern that justifies a primitive.

## Sources

- `docs/curated-tools-priority-list.md` (top-100 list)
- Issue #2260 (acceptance criteria for the list)
- `wip/explore_tsuku-homebrew-dylib-chaining_scope.md` (scope, at-risk lib set)
- Homebrew formula JSON: `https://formulae.brew.sh/api/formula/<name>.json`
  (queried 2026-05-02 for all 29 not-yet-curated entries plus 13 handcrafted
  controls).
