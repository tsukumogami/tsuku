# Lead: The nvm upstream data-root contract

All line numbers below refer to **nvm.sh at released tag `v0.40.6`**
(`https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.6/nvm.sh`, 4960 lines) unless
marked "master". v0.40.6 is the current latest release and therefore what the tsuku
recipe's `github_repo = "nvm-sh/nvm"` + `tag_prefix = "v"` resolves to today. I
verified the same code exists on `master` (5122 lines) at different line numbers —
the contract is identical on both.

Everything in the "Findings" section was verified by reading the actual source, and
the load-bearing claims in sub-question 3 were additionally verified by **running
nvm in a split program/data layout** (real `nvm install 22` against a real Node
tarball). Empirical transcripts are quoted inline.

---

## Findings

### 1. What lives under `$NVM_DIR`: PROGRAM vs DATA

The release tarball
(`https://github.com/nvm-sh/nvm/archive/refs/tags/v0.40.6.tar.gz`, which is exactly
what `recipes/n/nvm.toml` downloads) has this top level:

```
.dockerignore  .editorconfig  .gitattributes  .github/  .gitignore  .gitmodules
.mailmap  .npmrc  AGENTS.md  CLAUDE.md  CODE_OF_CONDUCT.md  CONTRIBUTING.md
Dockerfile  GOVERNANCE.md  LICENSE.md  Makefile  PROJECT_CHARTER.md  README.md
ROADMAP.md  bash_completion  install.sh  nvm-exec  nvm.sh  package.json
rename_test.sh  test/  update_test_mocks.sh
```

Classification:

| Path under `$NVM_DIR` | Class | Created by | Where nvm.sh derives it |
|---|---|---|---|
| `nvm.sh` | PROGRAM | tarball / git checkout | never referenced by `$NVM_DIR` from inside nvm.sh (see §3) |
| `nvm-exec` | PROGRAM | tarball / git checkout | **`${NVM_DIR}/nvm-exec`, line 4379** — the one and only exception |
| `bash_completion` | PROGRAM | tarball / git checkout | not referenced by nvm.sh at all; the *user's profile* sources `$NVM_DIR/bash_completion` (install.sh:463) |
| `install.sh`, `README.md`, `Makefile`, `test/`, `package.json`, … | PROGRAM | tarball | never referenced at runtime |
| `.git/` | PROGRAM (git installs only) | `install.sh` git method | `install_nvm_from_git` (install.sh:169) probes `$INSTALL_DIR/.git` to decide update-vs-clone |
| `versions/node/<v>/` | **DATA** | `nvm install` | `nvm_version_dir` line 712: `${NVM_DIR}/versions/node` |
| `versions/io.js/<v>/` | **DATA** | `nvm install iojs` | line 714 |
| `v<x.y.z>/` (pre-0.12 layout) | **DATA** | `nvm install` of ancient versions | `nvm_version_dir old` → `${NVM_DIR}` itself |
| `alias/` | **DATA** | `nvm alias`, and auto-written LTS aliases | `nvm_alias_path` line 723: `$(nvm_version_dir old)/alias` = `${NVM_DIR}/alias` |
| `alias/default` | **DATA** | `nvm alias default` / `nvm_ensure_default_set` | `nvm_alias_path`/`${ALIAS}` |
| `alias/lts/*` | **DATA** (nvm-managed) | any remote query re-creates these; README:447 says "you should not modify, remove, or create these files" | `mkdir -p "$(nvm_alias_path)/lts"` |
| `.cache/{bin,src}/` | **DATA** | download/build cache | `nvm_cache_dir` line 3167: `${NVM_DIR}/.cache` |
| `.cache/locks/` | **DATA** | per-version install advisory locks | `${NVM_DIR}/.cache/locks` (README:881) |
| `current` (symlink) | **DATA** | only when `NVM_SYMLINK_CURRENT=true` | line 4185: `rm -f "${NVM_DIR}/current" && ln -s ...` |
| `default-packages` | **DATA** (user-authored) | the user creates it by hand | line 4813: `NVM_DEFAULT_PACKAGE_FILE="${NVM_DIR}/default-packages"` |

Upstream's own `.gitignore` corroborates the split — it ignores exactly the DATA
paths: `HEAD`, `.cache`, `v*`, `alias`, `current`, `/default-packages`.

**Complete enumeration of `${NVM_DIR}/`-relative paths in nvm.sh v0.40.6** (this is
the whole surface; `grep -n '\${NVM_DIR}/' rel-nvm.sh`):

```
712:    nvm_echo "${NVM_DIR}/versions/node"
714:    nvm_echo "${NVM_DIR}/versions/io.js"
1054-1069: PATH/MANPATH/NODE_PATH rewriting regexes (nvm_change_path) — string ops, not files
1666:            s#^${NVM_DIR}/##;                      — display trimming in `nvm ls`
3167:  nvm_echo "${NVM_DIR}/.cache"
4032-4061: `nvm deactivate` messages about ${NVM_DIR}/*/bin etc.
4185:        command rm -f "${NVM_DIR}/current" && ln -s "${NVM_VERSION_DIR}" "${NVM_DIR}/current"
4379:      NODE_VERSION="${VERSION}" "${NVM_DIR}/nvm-exec" "$@"     <-- ONLY PROGRAM FILE
4699:      command rm -f "${NVM_DIR}/v*" "$(nvm_version_dir)" 2>/dev/null
4813:  NVM_DEFAULT_PACKAGE_FILE="${NVM_DIR}/default-packages"
4839:  nvm_echo "Installing default global packages from ${NVM_DIR}/default-packages..."
```

Every one of those is DATA except line 4379.

### 2. How upstream installs and upgrades — it never moves or recreates `$NVM_DIR`

`install.sh` (v0.40.6 / master, 533 lines) resolves the target directory once:

```sh
nvm_default_install_dir() {
  [ -z "${XDG_CONFIG_HOME-}" ] && printf %s "${HOME}/.nvm" || printf %s "${XDG_CONFIG_HOME}/nvm"
}
nvm_install_dir() {
  if [ -n "$NVM_DIR" ]; then printf %s "${NVM_DIR}"; else nvm_default_install_dir; fi
}
```
(install.sh:35-45)

Two install methods, both **strictly in-place**:

- **git method** (`install_nvm_from_git`, install.sh:152-230). If `$INSTALL_DIR/.git`
  exists it prints "nvm is already installed in $INSTALL_DIR, trying to update using
  git" and does `git fetch origin tag "$NVM_VERSION" --depth=1` then
  `git checkout -f --quiet FETCH_HEAD`. Note `checkout -f` is *not* `git clean` — it
  only touches tracked files, so untracked `versions/`, `alias/`, `.cache/` survive
  an upgrade untouched. That is precisely why those paths are gitignored.
  A wrinkle: if the directory exists and is non-empty but has no `.git`, install.sh
  does `git init` in place and adds the remote (install.sh:182-192) — i.e. it
  converts an existing data directory into a git working tree rather than moving
  anything.
- **script method** (`install_nvm_as_script`, install.sh:255-295). `mkdir -p
  "$INSTALL_DIR"`, then `curl -o "$INSTALL_DIR/nvm.sh"`, `-o
  "$INSTALL_DIR/nvm-exec"`, `-o "$INSTALL_DIR/bash_completion"`, then `chmod a+x
  nvm-exec`. Three file overwrites. Nothing else is touched.

Documented **Manual Upgrade** (README.md:357-372) is the same in-place git dance:

```sh
(
  cd "$NVM_DIR"
  git fetch --tags origin
  git checkout `git describe --abbrev=0 --tags --match "v[0-9]*" $(git rev-list --tags --max-count=1)`
) && \. "$NVM_DIR/nvm.sh"
```

There is **no `nvm upgrade` subcommand** — `grep -n '"upgrade"' nvm.sh` returns
nothing. (README:314 mentions `nvm upgrade`, but that is the third-party `zsh-nvm`
plugin's command, not nvm's.)

**The decision-relevant consequence:** upstream's upgrade model assumes program and
data share one directory and that upgrading means overwriting three files *in situ*.
The versioned-directory model tsuku applies to every other tool is structurally
incompatible with that assumption — not because nvm forbids the split, but because
upstream never has to think about it.

**Also relevant — install.sh actively refuses a non-default `$NVM_DIR` that does not
already exist** (install.sh:402-417):

```sh
if [ -n "${NVM_DIR-}" ] && ! [ -d "${NVM_DIR}" ]; then
  if [ -e "${NVM_DIR}" ]; then
    nvm_echo >&2 "File \"${NVM_DIR}\" has the same name as installation directory."
    exit 1
  fi
  if [ "${NVM_DIR}" = "$(nvm_default_install_dir)" ]; then
    mkdir "${NVM_DIR}" || { ...; exit 2; }
  else
    nvm_echo >&2 "You have \$NVM_DIR set to \"${NVM_DIR}\", but that directory does not exist. ..."
    exit 1
  fi
fi
```

So install.sh will create `~/.nvm` but will *not* create a custom `$NVM_DIR` for you.
This only matters if tsuku ever shells out to install.sh — the current recipe does
not (it extracts the tarball itself), so tsuku is free of this constraint. Worth
recording anyway, because it is a live footgun for anyone who tries "just run the
official installer with NVM_DIR set".

### 3. What nvm.sh requires of the directory it is sourced from — **nothing**

**This is the answer the exploration hinges on, so here is the evidence in full.**

#### 3a. `NVM_DIR` computation (nvm.sh:479-495, "Auto detect the NVM_DIR when not set")

```sh
# Auto detect the NVM_DIR when not set
if [ -z "${NVM_DIR-}" ]; then
  # shellcheck disable=SC2128
  if [ -n "${BASH_SOURCE-}" ]; then
    NVM_SCRIPT_SOURCE="${BASH_SOURCE}"
  fi
  # shellcheck disable=SC2086
  NVM_DIR="$(nvm_cd ${NVM_CD_FLAGS} "$(dirname "${NVM_SCRIPT_SOURCE:-$0}")" >/dev/null && \pwd)"
  export NVM_DIR
else
  # https://unix.stackexchange.com/a/198289
  case $NVM_DIR in
    *[!/]*/)
      NVM_DIR="${NVM_DIR%"${NVM_DIR##*[!/]}"}"
      export NVM_DIR
      nvm_err "Warning: \$NVM_DIR should not have trailing slashes"
    ;;
  esac
fi
unset NVM_SCRIPT_SOURCE 2>/dev/null
```

Read the control flow carefully:

- Self-location is a **fallback taken only when `NVM_DIR` is empty/unset**.
- When `NVM_DIR` *is* set, the `else` branch runs and does exactly one thing: strip a
  trailing slash and warn. It does **not** verify that `nvm.sh` lives there, does not
  compare against `BASH_SOURCE`, does not check that the directory exists.
- `nvm_default_install_dir` (the `~/.nvm` / `$XDG_CONFIG_HOME/nvm` fallback) **exists
  only in install.sh, not in nvm.sh**. nvm.sh has no notion of a default location; if
  `NVM_DIR` is unset it is *always* the sourced script's own directory.

There is no other code anywhere in nvm.sh that reassigns `NVM_DIR`.

#### 3b. Empirical proof: split layout works

Layout: `split/prog/` holds `nvm.sh` + `nvm-exec` + `bash_completion`;
`split/data2/` starts **completely empty** (no `versions/`, no `alias/`, no
`nvm.sh`). Run with `env -i` so nothing leaks in:

```
env -i HOME=$T/home2 PATH=$PATH NVM_DIR=$T/data2 bash -c '
  . "$T/prog/nvm.sh"
  nvm install 22 ; nvm ls ; nvm which 22 ; nvm use 22 && node -v && npm -v
  nvm alias default 22 ; nvm run 22 -e ... ; nvm exec 22 node -v'
```

Result:

```
Downloading https://nodejs.org/dist/v22.23.2/node-v22.23.2-linux-x64.tar.xz...
Computing checksum with sha256sum
Checksums matched!
Now using node v22.23.2 (npm v10.9.8)
Creating default alias: default -> 22 (-> v22.23.2 *)
[ls]        v22.23.2 *
            default -> 22 (-> v22.23.2 *)
            lts/jod -> v22.23.2 *
[which]     .../split/data2/versions/node/v22.23.2/bin/node
[node -v]   v22.23.2      [npm]  10.9.8
[alias default]  default -> 22 (-> v22.23.2 *)
[nvm run]   Running node v22.23.2 (npm v10.9.8)
            .../split/prog/nvm.sh: line 4540: .../split/data2/nvm-exec: No such file or directory
[nvm exec]  Running node v22.23.2 (npm v10.9.8)
            .../split/prog/nvm.sh: line 4540: .../split/data2/nvm-exec: No such file or directory
            exec rc=127
```

Resulting tree of the previously-empty data dir:

```
data2/.cache/{bin,locks}
data2/alias/{lts,default}
data2/versions/node/v22.23.2/...
```

Also verified the helper functions resolve into the data dir, not the program dir:

```
nvm_version_dir: .../split/data2/versions/node
nvm_alias_path:  .../split/data2/alias
nvm_cache_dir:   .../split/data2/.cache
```

**Verdict: yes, nvm.sh can live in a versioned tsuku tool directory while `NVM_DIR`
points at a stable data directory.** `install`, `ls`, `ls-remote`, `use`, `which`,
`alias`, `deactivate`, `unload`, `current`, `--version`, `.nvmrc` resolution, npm,
default-alias auto-set, LTS alias sync, and the `.cache` all work correctly.

#### 3c. The single exception: `nvm exec` and `nvm run`

nvm.sh:4379 (v0.40.6) / master:4540:

```sh
NODE_VERSION="${VERSION}" "${NVM_DIR}/nvm-exec" "$@"
```

This is the **only** place nvm.sh resolves a sibling program file through `$NVM_DIR`
instead of through its own location. It breaks `nvm exec` outright (`rc=127`).

`nvm run` breaks too, because `run` is implemented by delegating to `exec`
(master:4463-4465, in the `"run")` case):

```sh
  elif [ "${NVM_IOJS}" = true ]; then
    nvm exec "${NVM_SILENT_ARG-}" "${LTS_ARG-}" "${VERSION}" iojs "$@"
  else
    nvm exec "${NVM_SILENT_ARG-}" "${LTS_ARG-}" "${VERSION}" node "$@"
  fi
```

Everything else that could plausibly be a sibling lookup is *not* one:

- **`nvm-exec` itself self-locates.** The whole file:
  ```sh
  DIR="$(command cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  unset NVM_CD_FLAGS
  \. "$DIR/nvm.sh" --no-use
  ```
  It finds nvm.sh relative to itself, not via `$NVM_DIR`.
- **`bash_completion` is never referenced from nvm.sh.** `grep -n
  'bash_completion' nvm.sh` → nothing. It is sourced independently from the user's
  profile as `[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"`
  (install.sh:463, README:333). Internally it only reads `${NVM_DIR}/alias` (DATA):
  ```
  60:  if [ -d "${NVM_DIR}/alias" ]; then
  61:    aliases="$(command cd "${NVM_DIR}/alias" && command find "${PWD}" -type f | ...)"
  ```
  The current tsuku recipe does not wire completion at all
  (`install_shell_init` names only `source_file = "nvm.sh"`), so this is moot today.
- **`nvm_get_arch` reads no files under `$NVM_DIR`** — it shells out to `uname`,
  `isainfo`, `/usr/bin/file`, etc. No `NVM_DIR` reference in it.
- **`nvm --version` is a hardcoded literal** (`nvm_echo '0.40.6'`, v0.40.6:4901) — it
  does not read `$NVM_DIR/.git` or `package.json`. So a split layout does not confuse
  version reporting.

#### 3d. Fixing `nvm exec` — a bare symlink does NOT work

Tested three variants:

| Variant | Result |
|---|---|
| **A.** `ln -s $PROG/nvm-exec $NVM_DIR/nvm-exec` only | **FAILS** |
| **B.** symlink *both* `nvm-exec` **and** `nvm.sh` into `$NVM_DIR` | works |
| **C.** symlink both, leave `NVM_DIR` unset, source `$DATA/nvm.sh` | works |

Variant A's failure:
```
.../data2/nvm-exec: line 8: .../data2/nvm.sh: No such file or directory
.../data2/nvm-exec: line 11: nvm: command not found
```
Cause: bash sets `BASH_SOURCE[0]` to the **invocation path, not the resolved symlink
target**. So `nvm-exec`'s `DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"`
evaluates to `$NVM_DIR`, and it then tries to source `$NVM_DIR/nvm.sh`, which is not
there. `nvm-exec` re-sources nvm.sh in a fresh non-interactive bash — it cannot
inherit the parent shell's `nvm` function.

Variants B and C both print `v22.23.2` correctly.

The practical implication: **restoring `nvm exec`/`nvm run` requires two symlinks
(or a shim) in the data dir, not one** — `nvm.sh` and `nvm-exec`. Variant C is
especially interesting: with both symlinks present in the data dir and `NVM_DIR`
*unset*, nvm.sh self-locates to the data dir and everything including `nvm exec`
works. That is a design that needs no `set_env` step at all.

### 4. Source-time writes: nvm.sh writes **nothing** at source time

The last executable statement of nvm.sh is `nvm_process_parameters "$@"`
(master:5120), which calls `nvm_auto "${NVM_AUTO_MODE}"` with mode `use` by default,
`none` under `--no-use`, `install` under `--install`. In `use` mode `nvm_auto` calls
`nvm_ls_current`, `nvm_resolve_local_alias default`, possibly `nvm_rc_version`, then
`nvm use --silent`. All reads.

`nvm use` only writes if you opted in: line 4185 `rm -f "${NVM_DIR}/current" && ln -s
...` is guarded by `if [ "${NVM_SYMLINK_CURRENT-}" = true ]`.

Every `mkdir` in nvm.sh sits inside a function reachable only from an explicit
command — `nvm alias` / LTS sync (`mkdir -p "${NVM_ALIAS_DIR}/lts"`), `nvm install`
(version path + cache tmpdirs), lock acquisition. None run at source time.

Empirically, sourcing with `NVM_DIR` pointed at a **directory that does not exist at
all** is completely harmless:

```
env -i ... NVM_DIR=$T/does-not-exist bash -c '. $T/prog/nvm.sh; echo rc=$?; nvm ls; nvm --version'
rc=0
NVM_DIR=.../split/does-not-exist
->       system * (-> v26.5.1)
iojs -> N/A (default)
node -> stable (-> N/A) (default)
0.40.6
```

**Minimum requirements: none at source time.** `$NVM_DIR` need not exist and need not
be writable to source nvm.sh and run read-only commands. It must be *creatable and
writable* by the time the user runs `nvm install` or `nvm alias`; nvm creates
`versions/node/`, `alias/`, `alias/lts/`, and `.cache/{bin,src,locks}/` on demand via
`mkdir -p`. The empty-data-dir test above confirms nvm bootstraps all of them itself.

### 5. Is there any env var other than `NVM_DIR` for splitting program from data? **No.**

README.md:865-871 documents the complete list:

```
- `NVM_DIR` - nvm's installation directory.
- `NVM_BIN` - where node, npm, and global packages for the active version of node are installed.
- `NVM_INC` - node's include file directory (useful for building C/C++ addons for node).
- `NVM_CD_FLAGS` - used to maintain compatibility with zsh.
- `NVM_RC_VERSION` - version from .nvmrc file if being used.
```

`NVM_BIN` and `NVM_INC` are **outputs, not inputs** — nvm.sh assigns them inside
`nvm use` and never reads a caller-supplied value:

```
4343:      export NVM_BIN="${NVM_VERSION_DIR}/bin"
4344:      export NVM_INC="${NVM_VERSION_DIR}/include/node"
```
(master line numbers; `NVM_VERSION_DIR` is `$(nvm_version_path "${VERSION}")`, itself
derived from `$NVM_DIR`.)

I enumerated every `NVM_*` variable nvm.sh references (~100 names). The ones that are
genuinely caller-settable configure downloads and behavior, not layout:
`NVM_NODEJS_ORG_MIRROR`, `NVM_IOJS_ORG_MIRROR`, `NVM_SYMLINK_CURRENT`,
`NVM_AUTH_HEADER`, `NVM_MAKE_JOBS`, `NVM_CPU_CORES`, `NVM_NO_PROGRESS`,
`NVM_NO_COLORS`, `NVM_COLORS`, `NVM_SILENT`, `NVM_DEBUG`,
`NVM_INSTALL_THIRD_PARTY_HOOK`, `NVM_INSTALL_LOCK_TIMEOUT`,
`NVM_INSTALL_LOCK_STALE`, `NVM_NO_SOURCE_FALLBACK`, `NVM_METHOD_PREFERENCE`,
`NVM_OFFLINE`. **None relocates `versions/`, `alias/`, `.cache/`, or
`default-packages`.** There is exactly one knob — `NVM_DIR` — and everything hangs
off it.

`NVM_SYMLINK_CURRENT` is worth a footnote: it is the only setting that changes what
nvm writes into `$NVM_DIR` proper (the `current` symlink). It is off by default and
the current recipe does not set it.

### 6. How tsuku actually installs nvm today (context for the above)

Reading the recipe plus the two action implementations:

`recipes/n/nvm.toml`:
- `download_archive` with `install_mode = "directory"`, `strip_dirs = 1` → the whole
  tarball (nvm.sh, nvm-exec, bash_completion, install.sh, test/, docs) lands in
  `$TSUKU_HOME/tools/nvm-<version>/`.
- `set_env` with `NVM_DIR = "{install_dir}"`.
- `install_shell_init` with `source_file = "nvm.sh"`.

`internal/actions/shell_init.go:245-271` (`executeSourceFile`) **copies** the file:

```go
srcPath := filepath.Join(ctx.InstallDir, sourceFile)
...
destPath := filepath.Join(shellDDir, shellDFileName(target, version, shell))
if err := copyFile(srcPath, destPath); err != nil { ... }
```

with `shellDFileName(target, version, shell) = fmt.Sprintf("%s@%s.%s", ...)`
(shell_init.go:179-181) and `shellDDir = $TSUKU_HOME/share/shell.d`
(shell_init.go:144).

So **today, in the shipped recipe, nvm.sh already runs from outside `$NVM_DIR`** — it
executes as `$TSUKU_HOME/share/shell.d/nvm@<version>.bash`, a *copy*, while `NVM_DIR`
points at `$TSUKU_HOME/tools/nvm-<version>/`. The program/data split the exploration
is contemplating is, in one direction, already the status quo. What makes it work
today is that `$NVM_DIR` happens to also contain the extracted tarball, so
`${NVM_DIR}/nvm-exec` resolves.

`set_env` writes to a *different* shell.d file — `envTargetName` returns
`EnvFilePrefix + name` where the prefix is `"00-env-"`, and the comment
(set_env.go:202-203) states: "The `00-env-` prefix keeps exports sorted ahead of tool
init scripts." So `00-env-nvm@<version>.bash` sorts before `nvm@<version>.bash`, and
the `export NVM_DIR=...` genuinely does reach the shell before nvm.sh runs. The
recipe's comment at `recipes/n/nvm.toml:10-12` is accurate.

---

## Implications

**The decisive one: the program/data split is viable.** nvm.sh imposes no
requirement whatsoever that it live inside `$NVM_DIR`. When `NVM_DIR` is set, nvm.sh
honors it verbatim; self-location is a fallback for the unset case only. Every DATA
path — `versions/`, `alias/`, `.cache/`, `default-packages`, `current` — is derived
from `$NVM_DIR` and nothing else. A design where the nvm program stays in a
versioned, GC-able `$TSUKU_HOME/tools/nvm-<version>/` while `NVM_DIR` points at a
stable location (e.g. `$TSUKU_HOME/data/nvm` or `$TSUKU_HOME/state/nvm`) is sound.
This does **not** eliminate a class of designs; it opens one.

**The cost of the split is exactly one line of nvm.sh: 4379.** `nvm exec` and
`nvm run` (which delegates to `exec`) hard-code `${NVM_DIR}/nvm-exec`. Any split
design must place a working `nvm-exec` in the data root or accept that those two
subcommands break with `rc=127`. Silently breaking them would be a real regression:
`nvm exec` is how people run one-off commands under a pinned version in CI and
scripts.

**A bare symlink is not a sufficient fix, and this is easy to get wrong.** Because
`nvm-exec` self-locates via unresolved `BASH_SOURCE[0]` and re-sources
`$DIR/nvm.sh`, dropping only `nvm-exec` into the data dir fails. Two symlinks
(`nvm.sh` and `nvm-exec`) or a two-line generated shim are needed. Whatever the
design picks, it needs a test that actually runs `nvm exec <v> node -v` — a test that
only checks `nvm install` + `nvm use` will pass while `exec` is broken.

**Once both symlinks are in the data root, `set_env NVM_DIR` becomes optional.** The
variant-C result — symlinks in the data dir, `NVM_DIR` unset, source
`$DATA/nvm.sh` — works end to end including `nvm exec`, because nvm.sh self-locates
into the data dir and `nvm-exec` finds its sibling `nvm.sh` symlink there. That is a
strictly simpler shape than the current one, and it is also closer to what upstream
expects (nvm.sh living in `$NVM_DIR`, program-and-data-together), which reduces the
surface for future upstream changes to bite. It does mean the data root holds two
version-pinned symlinks that an upgrade must repoint — a real coupling, but a
2-file one rather than a whole-tree one. The design should weigh this against the
"NVM_DIR points elsewhere, nvm.sh runs from shell.d" shape.

**tsuku currently has no way to express "this recipe needs a stable, non-GC'd data
directory."** Whatever the design chooses, `{install_dir}` is the wrong template
variable here and something new is needed (a `{data_dir}` template var, a recipe-level
`[data]` declaration, or similar). That is a recipe-schema question, not an nvm
question.

**Migration is a first-class concern, not an afterthought.** Existing users have Node
installs sitting inside `$TSUKU_HOME/tools/nvm-<version>/versions/node/`. Repointing
`NVM_DIR` without moving that data makes every installed Node version vanish from
`nvm ls` — the same user-visible symptom as the GC bug the issue is about. The good
news from §4: `versions/`, `alias/`, and `.cache/` are self-contained and
location-independent (nvm never records absolute paths inside them), so a plain
`mv` of those three subtrees is a valid migration. The one caveat is that node
binaries' shebangs and npm's own config can embed absolute paths — worth a
verification pass, though upstream's own docs describe `$NVM_DIR` relocation as
supported.

**Upstream's own upgrade model reinforces that the data must not be versioned.**
`install.sh` overwrites three files in place and leaves everything else alone;
manual upgrade is `git fetch && git checkout -f` in place. Upstream never moves,
copies, or recreates the data. tsuku's versioned-tool-directory model is the outlier,
and the issue's framing (nvm's data root is being treated as a disposable program
directory) is correct.

---

## Surprises

1. **nvm.sh already runs from outside `$NVM_DIR` in tsuku today.**
   `install_shell_init` with `source_file` *copies* nvm.sh to
   `$TSUKU_HOME/share/shell.d/nvm@<version>.bash` (shell_init.go:245-271) rather than
   sourcing it from the tool dir. So the "can nvm.sh live outside NVM_DIR" question
   is already answered affirmatively by the shipped recipe — it works today only
   because `$NVM_DIR` coincidentally also holds the tarball. This narrows the change
   considerably: the split already exists, it is pointed at the wrong directory.

2. **Exactly one line stands between the current layout and a clean split.** I
   expected several sibling lookups (completion, arch detection, a version file).
   There is one: `${NVM_DIR}/nvm-exec` at v0.40.6:4379. `bash_completion` is never
   referenced from nvm.sh; `nvm_get_arch` touches no files; `nvm --version` is a
   hardcoded string literal. The blast radius is far smaller than the scope
   assumptions imply.

3. **The obvious one-symlink fix silently fails.** `ln -s $PROG/nvm-exec
   $NVM_DIR/nvm-exec` does *not* work, because bash's `BASH_SOURCE[0]` is the
   invocation path rather than the symlink target, so `nvm-exec` looks for
   `$NVM_DIR/nvm.sh`. I would have shipped that fix without testing it. It needs both
   symlinks.

4. **Upstream `install.sh` refuses to create a non-default `$NVM_DIR`**
   (install.sh:413-415: "You have $NVM_DIR set to ..., but that directory does not
   exist"). Only `~/.nvm` / `$XDG_CONFIG_HOME/nvm` gets auto-created. Harmless for
   tsuku (which extracts the tarball itself) but a trap for anyone who reaches for
   the official installer.

5. **`versions` is absent from upstream's `.gitignore`** (which lists `HEAD`,
   `.cache`, `v*`, `alias`, `current`, `/default-packages` — the `v*` entry is the
   *pre-0.12* layout, not the modern `versions/` one). Harmless in practice, since
   `git checkout -f` does not remove untracked files, but it means a git-method nvm
   install shows `versions/` as untracked in `git status`. Minor upstream nit; no
   effect on the design.

6. **`nvm run` was not on the risk list but breaks with `nvm exec`,** because `run`
   is implemented as a call to `exec` (master:4463-4465). Two subcommands, one root
   cause.

7. **`nvm --version` is a hardcoded literal, not read from the install tree.** Nice
   property: a split layout cannot produce a version-reporting mismatch.

---

## Open Questions

1. **Which split shape does the design want?** (a) `NVM_DIR` → stable data dir, nvm.sh
   sourced from shell.d, plus `nvm.sh` + `nvm-exec` symlinks in the data dir to keep
   `nvm exec` alive; or (b) symlink both into the data dir, drop the `set_env` step,
   and let nvm.sh self-locate (variant C — works, simpler, but the data root then
   holds two version-pinned symlinks the upgrade path must repoint). Both are
   verified working. This needs a decision, and it is squarely a design question, not
   a research one.

2. **Where does tsuku's stable per-tool data root live, and what creates it?** There
   is no `{data_dir}` template variable today. Needs a recipe-schema answer:
   `$TSUKU_HOME/data/<tool>/`? `$TSUKU_HOME/state/<tool>/`? XDG? Does `tsuku
   uninstall` remove it, prompt, or leave it? I did not investigate tsuku's GC or
   uninstall implementation — that is a separate lead.

3. **Migration of existing installs.** Concretely: move
   `$TSUKU_HOME/tools/nvm-<version>/{versions,alias,.cache,default-packages,current}`
   into the new root. Does this happen on next `tsuku install nvm`, via an explicit
   migration command, or not at all with a notice? And does anything inside a node
   version directory embed the old absolute path (npm `prefix` config in
   `$NVM_DIR/versions/node/<v>/etc/npmrc`, shebangs in global bin shims)? Worth an
   empirical check before committing to `mv`.

4. **Should `bash_completion` be wired up as part of this work?** The recipe does not
   install it today. If the design puts symlinks in the data root anyway, adding a
   third is nearly free and closes a real gap — but it is scope creep relative to
   issue #2464.

5. **Is `NVM_SYMLINK_CURRENT` worth supporting?** It is the one setting that changes
   what nvm writes into `$NVM_DIR` proper. Off by default; if the data root is stable
   it becomes safe to enable, but nobody has asked for it.

6. **Do other tsuku recipes have the same shape?** nvm is unlikely to be the only
   tool whose "install dir" doubles as a user data root (pyenv, rbenv, sdkman, asdf
   all follow the same `$TOOL_ROOT` pattern). If so, the fix should be a general
   mechanism rather than an nvm special case. I did not survey `recipes/`.

---

## Summary

nvm.sh imposes **no** requirement that it live inside `$NVM_DIR` — when the variable
is set, nvm.sh honors it verbatim (v0.40.6:479-495, the self-location branch runs
only when it is unset), and I confirmed empirically that a real `nvm install 22`,
`use`, `which`, `alias`, and `ls` all work with nvm.sh sourced from one directory and
`NVM_DIR` pointing at a previously-empty directory elsewhere, with nothing written at
source time. Exactly one line breaks the split — `NODE_VERSION="${VERSION}"
"${NVM_DIR}/nvm-exec" "$@"` at v0.40.6:4379, which takes `nvm exec` and `nvm run`
down with `rc=127`; fixing it needs **both** `nvm.sh` and `nvm-exec` present in the
data root, because `nvm-exec` self-locates through unresolved `BASH_SOURCE[0]` and a
bare `nvm-exec` symlink alone fails. The biggest open question is which split shape to
adopt — `NVM_DIR` at a stable data root with two compat symlinks, versus symlinking
both files in and letting nvm.sh self-locate with no `set_env` at all — closely
followed by how to migrate existing users' Node installs out of the versioned tool
directory without reproducing the very data loss issue #2464 is about.

---

# Round 1 Follow-Up: Empirical Verification

Three decision-blocking questions, answered by running nvm rather than reading it.

**Sandbox.** Two *real* release tarballs extracted the way tsuku's
`download_archive` (`strip_dirs = 1`, `install_mode = "directory"`) would leave them:

- `$W/prog` — nvm **v0.40.6** (`nvm.sh` md5 `31e19c75…`, `nvm-exec` md5 `b36045c7…`)
- `$W/prog2` — nvm **v0.40.3** (`nvm.sh` md5 `db681a1c…`, `nvm-exec` md5 `aca21638…`)

Both `nvm.sh` *and* `nvm-exec` genuinely differ between the two releases, so the
upgrade test in Q3 swaps real content, not identical bytes. Every shell runs under
`env -i HOME=… PATH=…` so nothing leaks in from my environment. The host has a system
Node v26.5.1 on `PATH`, which is why `nvm ls` always shows a `system` entry.

---

## Q1. Does nvm create `$NVM_DIR` when it does not exist? — **YES, unconditionally, including missing parents**

### Q1a — deeply nested, no parent exists at all

`NVM_DIR=$W/q1a/deeply/nested/nvm` with `$W/q1a` itself absent (three missing levels):

```
exists before? no
source rc=0
install rc=0
->     v22.23.2 *
         system * (-> v26.5.1)
default -> 22 (-> v22.23.2 *)
ls rc=0
v22.23.2
node rc=0
```

Resulting tree — nvm built the entire path itself:

```
q1a/deeply/nested/nvm/{.cache/bin, alias/lts, versions/node}
```

### Q1b — root exists but is empty

`install rc=0`, `node -v` → `v22.23.2`, and the root gained exactly `alias .cache
versions`. No complaint about the missing subdirectories.

### Q1c — does a *non-install* write command also create it?

`nvm alias myalias 22` against a nonexistent `$W/q1c/nvm`:

```
! WARNING: Version '22' does not exist.
myalias -> 22 (-> N/A)
alias rc=0
created? yes
q1c/nvm/alias/{lts, myalias}
```

So it is not an `nvm install` special case — any write path creates the root.

### Why

Every directory creation in nvm.sh is `mkdir -p`, and `-p` builds missing parents:
`mkdir -p "${VERSION_PATH}"` (master:2430, 2449, 2457), `mkdir -p
"$(nvm_alias_path)/lts"` (master:1850), `mkdir -p "${NVM_ALIAS_DIR}"` (master:4753),
`mkdir -p "${TMPDIR}"` for cache tmpdirs (master:2419, 2907), and `mkdir -p
"${LOCK_ROOT}"` (master:3261). All of those paths are `$NVM_DIR`-rooted, so the first
one to run materializes `$NVM_DIR` and everything above it.

Note this is nvm.sh behavior and is the **opposite** of `install.sh`, which *refuses*
to create a non-default `$NVM_DIR` (install.sh:413-415, recorded in the main findings).
tsuku extracts the tarball itself and never runs install.sh, so nvm.sh's permissive
behavior is the one that governs.

### Failure mode when the parent is not writable (Q1d)

`$W/q1d/ro` at mode 500, `NVM_DIR=$W/q1d/ro/nvm`:

```
Binary download failed, trying source.
Detected that you have 32 CPU core(s)
Running with 31 threads to speed up the build
mkdir: cannot create directory '.../q1d/ro/nvm': Permission denied
creating directory .../q1d/ro/nvm/.cache/src/node-v22.23.2/files failed
install rc=1
```

It fails correctly (`rc=1`) but the diagnostics are bad: nvm reports the *binary
download* as failed, starts a **from-source build** on 31 threads, and only then dies
on the same `mkdir`. A user hitting a permissions problem on the data root gets a
message about compiling Node.

### Decision impact

**tsuku does not need a directory-creating action.** No new action, no registry entry,
no plan-evaluability entry, no preflight, no tests. Pointing `NVM_DIR` at a path that
does not exist is completely safe — nvm materializes it on first write. The only
requirement is that the path be *creatable*, i.e. some ancestor is writable. Since the
data root would live under `$TSUKU_HOME`, which tsuku already owns and creates, that
holds by construction.

One caveat worth a line in the design: because nvm creates the root lazily on first
write, a freshly-installed nvm has **no data directory on disk at all** until the user
runs `nvm install`. Anything that reasons about "is nvm's data root present" (an
uninstall path, a `tsuku doctor` check, GC bookkeeping) must treat absence as normal
rather than as corruption.

---

## Q2. Two-symlink shape, nvm.sh sourced from a third-location copy — **YES, everything works**

### Layout (exactly what tsuku would produce)

```
$W/prog/                       # the versioned tool dir, $TSUKU_HOME/tools/nvm-0.40.6/
$W/data/nvm.sh   -> $W/prog/nvm.sh      (symlink)
$W/data/nvm-exec -> $W/prog/nvm-exec    (symlink)
$W/shelld/00-env-nvm@0.40.6.bash        # set_env output:  export NVM_DIR=$W/data
$W/shelld/nvm@0.40.6.bash               # install_shell_init output: a COPY of nvm.sh, mode 0600
```

The test shell sources `$W/shelld/*.bash` in sort order — the same thing a tsuku
login shell does — so `NVM_DIR` is exported first and nvm.sh runs **from the copy at a
third path**, neither `$PROG` nor `$DATA`.

### Results — every command rc=0

```
NVM_DIR=/…/fu-1324137/data
nvm is a: function
nvm --version                rc=0   | 0.40.6
nvm install 22               rc=0   | Local cache found: ${NVM_DIR}/.cache/bin/…  Checksums matched!
nvm alias default 22         rc=0   | default -> 22 (-> v22.23.2 *)
nvm ls                       rc=0   | v22.23.2 * / system * / default -> 22 (-> v22.23.2 *) …
nvm which 22                 rc=0   | /…/data/versions/node/v22.23.2/bin/node
nvm exec 22 node -v          rc=0   | Running node v22.23.2 (npm v10.9.8)  v22.23.2
nvm run 22 -e ...            rc=0   | Running node v22.23.2 (npm v10.9.8)  run-ok
nvm use default              rc=0   | Now using node v22.23.2 (npm v10.9.8)
npm -v                       rc=0   | 11.17.0
nvm use system               rc=0   | Now using system version of node: v26.5.1
nvm deactivate               rc=0   |
```

**`nvm exec` and `nvm run` both return 0.** That is the specific thing asked about, and
it works because `$DATA/nvm-exec` exists and, on being invoked, self-locates to
`$DATA` and finds the `$DATA/nvm.sh` symlink next to it. The nvm.sh that the
interactive shell sourced (the shell.d copy) is irrelevant to `nvm-exec` — it
re-sources nvm.sh in a fresh bash regardless.

*Harness caveat, so the table is not misread:* my `t()` helper ran each command inside
a command-substitution subshell, so `nvm use default`'s `PATH` export did not survive
into the next row — that is why the `node -v` row reports the system v26.5.1. It is a
test artifact, not a finding. The fresh-shell run below is the real check.

### Q2b — fresh shell, nothing installed in it, `nvm use default`

```
NVM_DIR=/…/fu-1324137/data
Now using node v22.23.2 (npm v10.9.8)
use default rc=0
v22.23.2                 node rc=0
Running node v22.23.2 (npm v10.9.8)
v22.23.2                 exec rc=0
Running node v22.23.2 (npm v10.9.8)
v22.23.2                 run rc=0
```

A brand-new shell picks up `NVM_DIR` from `00-env-…`, auto-uses the default alias,
`node -v` resolves to the nvm-managed v22.23.2, and `exec`/`run` work. The shape is
sound end to end.

---

## Q3. Upgrade and migration

### Q3a — swap the program underneath the data: **YES, data fully survives**

Repointed **both** symlinks from `$W/prog` (v0.40.6) to `$W/prog2` (v0.40.3), deleted
the old version-keyed shell.d fragments and wrote new ones (`nvm@0.40.3.bash`,
`00-env-nvm@0.40.3.bash`) — i.e. precisely what a tsuku upgrade plus its shell.d
cleanup does. `$W/data` was not touched.

```
=== BEFORE (program v0.40.6) ===          === AFTER swap to v0.40.3 ===
  nvm --version : 0.40.6                    nvm --version : 0.40.3
  nvm ls        : ->v22.23.2* system*       nvm ls        : ->v22.23.2* system*
  default alias : default -> 22 (-> v22.23.2 *)   default alias : default -> 22 (-> v22.23.2 *)
  nvm exec rc   : 0                         nvm exec rc   : 0
  nvm run  rc   : 0                         nvm run  rc   : 0
  use default   : v22.23.2                  use default   : v22.23.2
```

Then `rm -rf $W/prog` — the actual GC event that issue #2464 is about:

```
=== after old tool dir deleted ===
  nvm --version : 0.40.3
  nvm ls        : ->v22.23.2* system*
  default alias : default -> 22 (-> v22.23.2 *)
  nvm exec rc   : 0
  nvm run  rc   : 0
  use default   : v22.23.2
```

Node 22 survives, the `default` alias survives, `exec`/`run` keep working, and
`nvm --version` correctly reports the new program version. **Deleting the old
versioned tool directory is a no-op once the symlinks are repointed** — which is
exactly the property the issue needs.

### Q3a negative — symlinks left dangling

If an upgrade GCs the old tool dir *without* repointing the symlinks:

```
nvm ls   : ->v22.23.2* system*          <- still fine
nvm exec 22 node -v  ->  rc=127
  …/shelld/nvm@0.40.3.bash: line 4083: …/data/nvm-exec: No such file or directory
nvm run  20          ->  rc=127
nvm ls               ->  rc=0
nvm install 20       ->  rc=0
```

(Clean rc measured separately with `out=$(nvm exec …); echo $?`.)

So the failure is *partial and quiet*: install, ls, use, which, alias all keep
working; only `exec` and `run` break, with a 127 and a bash "No such file" line.
**The two symlinks are load-bearing upgrade state.** Repointing them is not an
optional polish step, and because the breakage does not surface in the common
commands, an upgrade that forgets it would very plausibly ship unnoticed. The design
should make repointing part of the install action itself rather than a separate step,
and the test suite needs an `nvm exec` assertion *after* a simulated upgrade — not
just after a fresh install.

### Q3b — migration: **YES, a plain `mv` is sufficient; nothing embeds the old path**

Setup under `OLD=$W/old`: node v22.23.2, `default` alias, plus two global npm
packages — `cowsay` (pure JS) and `esbuild` (ships a **native binary**, the harder
case).

**Pre-move inspection for embedded absolute paths:**

```
$ ls -l $OLD/versions/node/v22.23.2/bin/cowsay
  … bin/cowsay -> ../lib/node_modules/cowsay/cli.js     <- RELATIVE symlink
$ head -1 $(readlink -f …/bin/cowsay)
  #!/usr/bin/env node                                    <- relocatable shebang
$ cat $OLD/versions/node/v22.23.2/etc/npmrc
  (no etc/npmrc)
$ grep -rIl -- "$OLD" $OLD/versions/node/v22.23.2 | wc -l
  0                                                      <- ZERO files embed the old root
```

npm's own config is *computed at runtime* from node's location, not stored:
`npm config get prefix` → `$OLD/versions/node/v22.23.2`, `globalconfig` →
`$OLD/versions/node/v22.23.2/etc/npmrc` (a file that does not exist), `cache` →
`$HOME/.npm` (outside `$NVM_DIR` entirely).

**The migration** — `mv $OLD/versions $OLD/alias $DATA3/`, add the two program
symlinks, point `NVM_DIR=$DATA3`:

```
nvm ls        : ->v22.23.2* system*
default alias : default -> 22 (-> v22.23.2 *)
nvm which 22  : /…/data3/versions/node/v22.23.2/bin/node
use default rc=0
node -v       : v22.23.2
npm -v        : 10.9.8
which cowsay  : /…/data3/versions/node/v22.23.2/bin/cowsay
cowsay rc     : 0
esbuild rc    : 0
esbuild ver   : 0.28.1
npm prefix    : /…/data3/versions/node/v22.23.2      <- recomputed to the NEW root
npm ls -g     : +-- cowsay@1.6.0  +-- esbuild@0.28.1  `-- npm@10.9.8
nvm exec rc   : 0
nvm run rc    : 0
```

Post-move residual check: `grep -rIl -- "$OLD" $DATA3 | wc -l` → **0**.

**Nothing breaks.** The pure-JS global, the native-binary global, npm's global package
list, the default alias, and `exec`/`run` all work. Node installs are genuinely
location-independent.

**One thing the migration must not forget:** I deliberately moved only `versions/` and
`alias/`, leaving `.cache/` behind. nvm silently recreated it and re-downloaded on the
next install — harmless, but it wastes bandwidth. The complete move list is
`versions/`, `alias/`, `.cache/`, plus `default-packages` and `current` when they
exist. A safer formulation for the design: move *everything* under the old root except
the extracted program files, rather than enumerating known data paths — that way a
future nvm release adding a new data path does not silently get dropped.

---

## Follow-Up Implications

The three answers collapse a lot of the design space:

**No new tsuku action is needed.** Q1 removes the directory-creation work entirely —
action, registry entry, plan-evaluability entry, preflight, tests. `set_env NVM_DIR`
pointed at a path that does not exist yet is sufficient and safe.

**The shape is confirmed, and it is the two-symlink one.** Q2 shows the full
tsuku-shaped layout — copy in shell.d, symlinks in the data root, `NVM_DIR` exported
by a `00-env-` fragment — works for every command including `exec` and `run`. The
alternative from the main findings (no `set_env`, let nvm.sh self-locate) also works,
but Q2 proves the shape that requires the *least* change to the current recipe, and
that shape keeps `NVM_DIR` explicit rather than implicit. I would take it.

**The GC problem is genuinely solved by this, not just moved.** Q3a's `rm -rf $PROG`
after repointing is the exact event that destroys user data today, and it becomes a
no-op.

**Migration is low-risk and should be done automatically.** Q3b found zero embedded
absolute paths across a native-binary package, so a `mv` is safe. Combined with Q1
(nvm creates whatever is missing), a migration step is: create nothing, move the old
root's data subtrees, repoint. No fallback, no user prompt, no "run this command
yourself" notice needed.

**The one thing that can silently regress is `nvm exec` after an upgrade.** Everything
else fails loudly or not at all. That deserves a dedicated regression test that
upgrades and *then* runs `nvm exec`.

## Follow-Up Open Questions

1. Should `bash_completion` become a third symlink? It is free now that a symlink
   mechanism exists, and it only reads `$NVM_DIR/alias`. Still scope creep for #2464.
2. What happens on `tsuku uninstall nvm` — is the data root removed, kept, or
   prompted? Q1 shows it may not exist at all, so "kept" is the low-surprise default.
3. Does anything else in tsuku need to know the data root exists (GC bookkeeping,
   `tsuku doctor`, disk-usage reporting)? Not investigated.
