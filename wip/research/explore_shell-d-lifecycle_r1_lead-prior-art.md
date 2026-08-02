# Lead: How do comparable version managers and package managers keep generated shell fragments consistent with the active version?

Scope note: this lead surveys external prior art and then grounds a compatibility
assessment in the tsuku checkout on branch `docs/shell-d-lifecycle`. Every claim
about an external tool is marked **[doc]** (stated in official documentation) or
**[src]** (read in the project's source). Every claim about tsuku cites a file and
line in this worktree.

Headline: **almost nobody does what tsuku does.** Of the eight mechanisms surveyed,
exactly one — `/etc/profile.d` — renders version-specific content once at install
time into a version-independent filename and never re-renders it. That one is the
mechanism with the worst reputation, the one Debian Policy explicitly tells
packagers not to depend on, and the one whose failure mode is precisely the bug
under investigation. Everyone else either regenerates on activation, computes live
at shell time, or keeps only a version-free indirection in the durable artifact.

---

## Findings

### asdf

**Artifact lifetime: nothing version-specific is ever written for the shell.**
`asdf.sh` prepends `$ASDF_DIR/bin` and `$ASDF_DATA_DIR/shims` to `PATH` and defines
an `asdf` shell function; it sets no per-tool variables at all **[src,
`asdf.sh` v0.15]**. Shims are written at install/reshim time into
`$ASDF_DATA_DIR/shims/<name>`, but every shim is functionally identical —
`exec asdf exec "<name>" "$@"` — with a block of `# asdf-plugin: <plugin> <version>`
comments that is metadata, not behavior **[src, `internal/shims/shims.go`]**.

Per-tool environment comes from a plugin's `bin/exec-env`, and it is **sourced live
on every single command invocation**, never copied anywhere. The v0.16 Go rewrite
makes the mechanism explicit in a comment: it forks a shell to run
`. "<exec-env>"; env -0`, parses the output back into a map, and `syscall.Exec`s the
real binary **[src, `internal/execenv/execenv.go`]**. asdf 0.16 removed all shell
code and `asdf shell` outright, because that command needed asdf to be a shell
function **[doc, upgrade guide]**.

**Stable indirection: none.** I confirmed zero uses of `Symlink` anywhere under
`internal/` in asdf master. Install paths are concrete:
`$ASDF_DATA_DIR/installs/<plugin>/<version>` **[src, `internal/installs/installs.go`]**.
`asdf where` is a query command that resolves the version at call time, not a
symlink.

**Version removal:** `uninstall` calls `shims.RemoveAll` then `shims.GenerateAll` —
a full nuke-and-regenerate of the shims directory **[src, `internal/cli/cli.go`]**.
Even a hypothetically stale shim degrades safely: `FindExecutable` intersects the
shim's claimed versions with the resolved ones and errors rather than running the
wrong binary.

**Cost:** structurally, one bash process for the shim, one for `asdf exec`, a config
load, a parent-directory walk for `.tool-versions`, `list-bin-paths`, and a second
fork to source `exec-env` — per command. mise's author claims ~120 ms per runtime
call **[doc, mise comparison page]**; treat that number as an interested party's
characterization, though the chain above makes it plausible.

Staleness risk: structurally zero. Price: paid on every command, forever.

### mise

**Artifact lifetime: computed at hook time, cached in the environment.**
`mise activate bash` emits a script that registers `_mise_hook_prompt_command` into
`PROMPT_COMMAND` and `_mise_hook_chpwd` into the chpwd chain, each doing
`eval "$(mise hook-env ...)"` **[src, `src/assets/bash/activate.sh`]**. The cache is
not on disk: `__MISE_SESSION` holds an rmp-serialized, zlib-compressed, base64
session (loaded tools, config paths, computed env, watch files, `latest_update`
timestamp) and `__MISE_DIFF` holds the reverse-diff so the next run can undo its own
edits precisely **[src, `src/hook_env.rs`]**.

Staleness detection is two-tier and elaborate. `should_exit_early_fast()` runs before
config loads and forces a full run on: cwd change, a changed hash of all `MISE_*`
vars, or **managed-env drift** — mise re-reads every key it claims to own and every
PATH entry it added, and reloads if any doesn't match. Then it stats every loaded
config, every tera file, every watch file, the trust dirs, `dirs::DATA`, and the
mtime of every config-search ancestor directory against `latest_update` **[src]**.
Two settings widen the window deliberately: `hook_env.cache_ttl` (default `0s`, "for
slow filesystems like NFS", with the documented caveat that new config files may not
be detected until it expires) and `hook_env.chpwd_only` **[doc, `settings.toml`]**.

**Stable indirection: yes, partially — and it is a liability.** `src/runtime_symlinks.rs`
creates partial-version and `latest` symlinks: `installs/node/20 -> ./20.11.0`,
`installs/node/latest -> ./20.11.0`. These leak into `PATH`: the shims doc notes that
with a fuzzy version, PATH "may use the requested-version symlink, such as
`~/.local/share/mise/installs/python/3.15/bin`" **[doc]**. Uninstalling `3.15.0`
moves or breaks that target while the PATH *string* still looks valid, so drift
detection won't fire on it — mise relies on the `DATA` mtime check to catch this,
not on PATH-content validation.

**mise is the one tool that does persist per-version env to disk**, and it is worth
noting how it handles invalidation: for asdf-backed plugins it caches the *result* of
running `exec-env` at `<cache>/<tool>/<version>/exec_env.msgpack.z`, guarded by
`.with_fresh_file(plugin_path)` and `.with_fresh_file(tv.install_path())` **[src,
`src/backend/external_plugin_cache.rs`]**. Note the shape: the cache path is keyed by
tool *and version*, so it can never be stale for the wrong version — it can only be
absent. That is the opposite of tsuku's version-independent filename.

**Version removal:** `mise uninstall` calls `rebuild_shims_and_runtime_symlinks`,
which reshims and rebuilds symlinks, ending in `remove_missing_symlinks_in_dir`
**[src, `src/cli/uninstall.rs`]**. For the shell, the stale window is bounded by one
prompt (`DATA` mtime bumps), except under `chpwd_only`, where PATH stays stale until
the next `cd`.

**Cost:** claimed ~4 ms unchanged / ~14 ms full reload **[doc]**. Community reports
of 100–200 ms in deep trees and on network filesystems are common
(discussions #4821, #4327, #3658), because the fast path stats every ancestor
directory times every config filename variant.

### rbenv / pyenv / nodenv

**Rendered at install: one generic script, copied N times.** `rbenv-rehash` builds a
single prototype at `$RBENV_ROOT/shims/.rbenv-shim` and `cp`s it for every registered
executable **[src, `libexec/rbenv-rehash`]**. Every shim is byte-identical apart from
`${0##*/}`. No Ruby version appears anywhere in it.

Two things *are* baked: the literal `RBENV_ROOT` value and the absolute path to the
`rbenv` binary. `remove_outdated_shims()` diffs the fresh prototype against an
existing shim and wipes the whole directory if they differ — so if rbenv itself moves
(exactly what happens when a package manager reinstalls rbenv under a new versioned
directory), the next rehash silently regenerates everything. **The shims are
version-agnostic with respect to Ruby, but not with respect to rbenv's own install
path.** That is directly relevant: it's the same class of hazard as tsuku baking
`{install_dir}` into shell.d, just one level up.

**Resolved at exec time:** `RBENV_VERSION` env var → nearest `.ruby-version` walking
up from `$RBENV_DIR`/`$PWD` → `$RBENV_ROOT/version` → `system`. `rbenv-which` then
string-concatenates `${RBENV_ROOT}/versions/${RBENV_VERSION}/bin/${cmd}` freshly on
every invocation **[src, `libexec/rbenv-which`]**.

**Stable indirection: none, in any of the three.** `grep -rn "ln -s"` returns zero
hits across rbenv and nodenv (whole repos, tests included), and in pyenv only two
hits inside `python-build` for download caching. `versions/current` does not exist
anywhere **[src]**. The indirection is a *computation*, not a filesystem pointer. The
only durable pointers are plain-text files containing a version *string* —
`$RBENV_ROOT/version`, `.ruby-version` — never a path, never a symlink.

**Version removal:** rbenv has no `uninstall`; ruby-build's `rbenv-uninstall` ends
with `rm -rf "$PREFIX"; rbenv-rehash`, and pyenv/nodenv are identical **[src]**.
Without rehash, a stale shim shadows what's behind it and errors on invocation
(rbenv/nodenv) or falls through to system PATH (pyenv, whose `pyenv-which` degrades
more gracefully).

**No version-specific shell-sourced file exists anywhere in this family.** The rc
file gets one fixed line, `eval "$(rbenv init - --no-rehash bash)"`, and the eval'd
script is regenerated at every shell start containing only PATH, `RBENV_SHELL`,
completions, and the dispatcher function **[src, `libexec/rbenv-init`]**. I checked
pyenv-virtualenv specifically: it also evals at startup and installs a prompt hook;
zero Python versions are baked into the emitted text **[src,
`bin/pyenv-virtualenv-init`]**.

**Cost:** ~50 ms per ruby invocation by rbenv's own wiki estimate; `which` returns the
shim (hence the separate `rbenv which`); and rehash is a manual sync step, mitigated
by a RubyGems plugin and by running it at every shell start — which rbenv's README
concedes "slows down your shell startup," offering `--no-rehash` as the escape.

### nvm — the recipe that exposed the bug

**`NVM_DIR` is nvm's own installation directory, and it doubles as the data root.**
Documented as "nvm's installation directory" **[doc, README]**, and source-confirmed:
when unset, nvm derives it from where `nvm.sh` itself lives via
`dirname "${NVM_SCRIPT_SOURCE:-$0}"` **[src, `nvm.sh` ~L525-543]**. It is definitionally
the directory containing nvm.sh, and definitively not a per-node-version directory.

But the same variable roots all runtime data **[src, `nvm.sh`]**:

| Path | Contents |
|---|---|
| `$NVM_DIR/versions/node/vX.Y.Z` | every installed node, and every global npm package inside it |
| `$NVM_DIR/alias/` | `default`, and the `lts/*` aliases |
| `$NVM_DIR/.cache/` | downloads and per-version install locks |
| `$NVM_DIR/default-packages` | auto-install-on-new-node list |
| `$NVM_DIR/nvm.sh`, `nvm-exec`, `bash_completion` | the program itself |

Program files and user data are deliberately co-located. There is no
`NVM_VERSIONS_DIR` or equivalent to split them.

**Stable `current` symlink: exists, opt-in, and discouraged.** Exactly one creation
site in the whole file, gated on `NVM_SYMLINK_CURRENT=true` **[src, `nvm_use`]**. The
README explains the default-off: "using `nvm` in multiple shell tabs with this
environment variable enabled can cause race conditions" **[doc]**. nvm's model is
per-shell, so a single global filesystem symlink is a shared mutable resource two
tabs will fight over.

**Is pointing `NVM_DIR` at a per-version directory the right model? No — and this is
a modelling error independent of the staleness bug.** Set `NVM_DIR=tools/nvm-0.40.5`
and every node version the user installs becomes a child of a directory whose name
encodes *nvm's* version. Install `nvm-0.40.6` and repoint: the new directory has no
`versions/`, no `alias/`, no `default-packages`. `nvm ls` reports nothing installed,
`node` disappears from new shells because the `default` alias is gone, and every
global npm package is orphaned. If tsuku garbage-collects the old versioned
directory, those node installs are *deleted* — potentially tens of gigabytes of user
state nvm never treated as disposable. No nvm command relocates or imports a
versions tree.

What nvm actually does on upgrade is operate **in place**: if `$INSTALL_DIR/.git`
exists it prints "nvm is already installed in $INSTALL_DIR, trying to update using
git" and runs `git checkout -f --quiet FETCH_HEAD` in the same directory
**[src, `install.sh`, `install_nvm_from_git`]**. `versions/`, `alias/`, `.cache/` are
untracked, so they survive. The documented manual upgrade is the same
`cd "$NVM_DIR"; git fetch --tags; git checkout <tag>` **[doc]**. nvm's whole upgrade
story assumes a stable directory that is part program and part data, and swaps only
the program half.

There is even a hard guard: `nvm_do_install` refuses to proceed when `NVM_DIR` points
at a nonexistent non-default path **[src]**. The installer defaults to `$HOME/.nvm`,
or `$XDG_CONFIG_HOME/nvm` **[src, `nvm_default_install_dir`]**. Every documented
value is a single stable per-user directory; nothing in the docs contemplates a
rotating `NVM_DIR`.

### Nix profiles and home-manager

**Chain:** `~/.nix-profile` → `/nix/var/nix/profiles/per-user/<user>/profile` (or
`$XDG_STATE_HOME/nix/profiles/profile` on Nix ≥ 2.14) → `profile-<N>-link` → a store
path **[doc, nix manual; verified locally that the XDG variant is what current Nix
produces]**.

**What the indirection buys, in the manual's own words:** the final repoint "is
atomic on Unix, which explains how we can do atomic upgrades"; `nix-env --rollback`
"will just make the current generation link point at the previous link" (O(1), no
rebuild); and "each of these symlinks is a root for the Nix garbage collector"
**[doc]**.

**home-manager rebuilds the entire generation on every activation. There is no
incremental patching.** `linkNewGen` does an unconditional `find` over the whole new
generation and relinks everything **[src, `modules/files.nix`]**. `cleanOldGen` then
deletes links present in the old generation and absent from the new. The ordering is
deliberate: link-new-then-clean-old gives FA → FA ∪ FB → FB, so a mid-activation
failure leaves a superset, never a gap. The only incremental behavior is a
content-equality skip on writes and the `onChange` hooks, which gate *side effects*
(cache rebuilds, service restarts) on a diff even though file rendering is not gated
**[src]**.

`hm-session-vars.sh` is generated at build time by `pkgs.writeTextFile` and lands at
`etc/profile.d/hm-session-vars.sh` *inside the profile* — home-manager reuses the
profile.d convention but scopes it to the Nix profile rather than `/etc` **[src,
`modules/home-environment.nix`]**. It opens with an idempotence guard
(`__HM_SESS_VARS_SOURCED`) that makes double-sourcing free.

**Version removal:** building a new generation without it. The old generation still
references the store path, so it survives until that generation is deleted *and* GC
runs; rollback stays available until then **[doc]**.

**The failure mode worth flagging:** a running shell is *not* itself protected.
Protection comes from generation links being GC roots, not from any process. Nix's
runtime `temproots` mechanism protects Nix's own in-flight operations, not arbitrary
user processes **[src, `src/libstore/gc.cc`]**. Run `nix-collect-garbage -d` while a
shell has an old `user-env/bin` on PATH and that directory is deleted out from under
it — and bash's command hash table makes it worse, reporting "No such file or
directory" for a command that worked a second ago until `hash -r`.

**Cost:** highest of the eight. Full Nix evaluation and build per activation (seconds
to minutes for a one-line change); monotonic store growth until GC; DAG-ordered
activation with a `writeBoundary` separating check from write phases. Staleness risk
is near zero by construction — you cannot have a partially-updated environment.

### Homebrew — `brew link`/`unlink` and the opt prefix

Three path families: the **keg** at `HOMEBREW_PREFIX/Cellar/<formula>/<version>/`
(versioned), the **opt prefix** at `HOMEBREW_PREFIX/opt/<formula>` (stable), and the
per-file symlinks in `bin/`, `lib/` etc. **[doc, Formula Cookbook glossary]**.

**Source-verified: `opt/<formula>` is a relative symlink to the active keg,
delete-and-recreated as part of linking.** `opt_record` is literally
`HOMEBREW_PREFIX/"opt/#{name}"`, and `optlink` does
`opt_record.delete if opt_record.symlink? || opt_record.exist?` followed by
`make_relative_symlink(opt_record, path, ...)`; `link` calls `optlink` **[src,
`Library/Homebrew/keg.rb`]**. The doc comment on `Formula#opt_prefix` states the
property exactly: "A stable path for this formula, when installed. Contains the
formula name but no version number. Only the active version will be linked here if
multiple versions are installed" **[src, `formula.rb`]**.

**This is the closest prior art to what the exploration is reaching for, and the
documentation says why in one line.** Formula Cookbook: "The `opt_` variants generate
paths that are stable between updates, which can be useful for e.g. replacing
versioned paths in files," with the canonical example
`inreplace lib/"pkgconfig/zlib.pc", prefix, opt_prefix` **[doc]**. Autotools burns
`--prefix` into `.pc` files, wrapper scripts and RPATHs at build time; `inreplace`
rewrites those occurrences to the stable opt path after the build and before
installation, so the artifact survives the next upgrade. For consumers rather than
authors: "Use `brew --prefix <formula>` rather than embedding `/opt/homebrew`,
`/usr/local` or a Cellar version" **[doc, keg-only guide]**.

A subtlety: `Formula#prefix` itself returns the opt path when the keg is optlinked
**[src, `formula.rb`]** — Homebrew papers over the distinction at runtime while still
asking authors to be explicit at build time.

**Upgrade:** the new version installs to a new Cellar directory; linking calls
`optlink`, which repoints `opt/<formula>`. Anything referencing `opt/<formula>/...`
follows automatically. The old Cellar version is *not* deleted — `brew cleanup` is a
separate command **[doc, manpage]**. So anything that baked a versioned Cellar path
keeps working until cleanup runs, then breaks. That delay-and-decouple is the nastiest
property of the model: the upgrade happens Monday, the breakage Friday.

**`unlink` does not remove the opt symlink** — `unlink` removes the bin/lib symlinks;
`remove_opt_record` is a separate method **[src, `keg.rb`]**. So after
`brew unlink foo`, `foo` is off PATH but `$(brew --prefix foo)` still resolves. This
is deliberate, and it is exactly the keg-only design: for keg-only formulae the
bin/lib step is skipped but the opt step still runs, "so that you can always access
the 'active' version via `/usr/local/opt/sqlite`" **[doc]**.

**Cost:** lowest of the eight. No per-shell latency whatsoever — PATH holds
`HOMEBREW_PREFIX/bin` and symlink resolution is free. One level of indirection.
Bounded, well-understood staleness landing on whatever escaped `inreplace`.

### `/etc/profile.d`

**It is not a standard.** I read the FHS 3.0 `/etc` section: it enumerates `profile`
among optional config files. **`profile.d` does not appear anywhere in the
specification** **[doc, FHS 3.0]**. Debian Policy §9.9 goes further and treats it as
actively unreliable: programs on the system PATH "must not depend on custom
environment variable settings," because such variables "would have to be set in a
system-wide configuration file such as a file in `/etc/profile.d`, **which is not
supported by all shells**." Policy's prescribed alternative is a wrapper script
**[doc]**. That is a distribution's own policy manual telling packagers not to rely
on this mechanism.

**Coverage:** bash login shells yes; dash/ksh login shells yes; **zsh not at all by
default on Debian/Ubuntu** (it reads `/etc/zprofile`, and Debian ships that empty;
Fedora works around it with `emulate -L ksh; source /etc/profile`); non-login
interactive shells never. Ubuntu's `/etc/bash.bashrc` has the profile.d loop present
but **commented out**, with the comment "This is commented out by default, for not
cluttering the shell with things a user might not expect" **[verified locally]**.

**Ordering contract: glob order, and glob order is locale-dependent.** The loop from
Ubuntu 24.04's `/etc/profile`, verbatim **[verified locally]**:

```sh
if [ -d /etc/profile.d ]; then
  for i in /etc/profile.d/*.sh; do
    if [ -r $i ]; then . $i; fi
  done
  unset i
fi
```

Same directory, two locales, two orders **[verified locally]**:

```
LC_ALL=C           bash -c 'echo *.sh'  →  B.sh Z.sh a.sh c.sh
LC_ALL=en_US.UTF-8 bash -c 'echo *.sh'  →  a.sh B.sh c.sh Z.sh
```

So the sourcing order is not stable across locales if any filenames differ in case.
Distros work around it with numeric prefixes (`01-locale-fix.sh` first,
`Z99-cloud-locale-test.sh` last — uppercase `Z99` sorts last in *both* collations).
There is no dependency mechanism beyond filename sorting. Note the recursion:
`01-locale-fix.sh` is itself in the directory it affects.

**Written once at install, never regenerated.** Most are plain packaged files listed
in RPM `%files` or shipped by dpkg, not scriptlet output. Upgrade replaces the file;
removal deletes it; nothing re-renders on any other event.

**The failure mode, observed live on this machine [verified locally]:**

```
/etc/profile.d/java.sh      → export PATH=/opt/jdk-21.0.5+11/bin:$PATH
/etc/profile.d/nodejs.sh    → export PATH=/opt/node-v24.8.0-linux-x64/bin:$PATH
/etc/profile.d/terraform.sh → export PATH=/opt/terraform_1.9.5:$PATH
/etc/profile.d/go.sh        → export PATH=/opt/go/bin:$PATH
```

`/opt` holds **seven JDKs** and two Node versions. Each file hardcodes exactly one,
chosen at write time. Delete `/opt/jdk-21.0.5+11` and `java.sh` silently prepends a
nonexistent directory — no error, `java` just resolves elsewhere or nowhere.
Five of the six files are not owned by any package (`dpkg -S` returns "no path found"),
so even the remove-on-uninstall guarantee doesn't apply.

**`go.sh` is the control case.** It points at unversioned `/opt/go/bin` and is
therefore the only one of the five immune. That is a hand-rolled opt prefix, built by
whoever provisioned the box. The convention gives you a stable *directory*; it does
not give you a stable *target*. Fedora's Environment Modules packaging layers
`/etc/alternatives` underneath to get the indirection profile.d lacks:
"`/etc/profile.d/modules.{csh,sh}` are links to `/etc/alternatives/modules.{csh,sh}`"
**[doc, Fedora wiki]**.

### direnv

**Everything is computed live at hook time; nothing is rendered at install.**
"Before each prompt it checks for the existence of an `.envrc` file in the current
and parent directories. If the file exists, it is loaded into a bash sub-shell and
all exported variables are then captured by direnv and then made available to your
current shell, while unset variables are removed" **[doc, direnv(1)]**. Even the hook
itself is generated on the fly — you `eval "$(direnv hook bash)"`. Because
`direnv export` emits a *diff*, direnv can un-set variables on directory exit, not
just set them; prior state lives in `DIRENV_DIFF`.

**Staleness:** a watch list rather than a cache. `watch_file` is a thin shim over the
binary (`eval "$("$direnv" watch bash "$@")"`) **[src, `stdlib.sh`]**; the list is
serialized with mtimes into `DIRENV_WATCHES` and compared each prompt.

**Permission model:** `$XDG_DATA_HOME/direnv/allow` records allowed files, keyed to
content, so editing `.envrc` revokes the grant — fail-closed. Rationale stated
bluntly: "Otherwise any git repo that you pull … would be able to wipe your hard
drive once you cd into it" **[doc]**. Documented gap: `source_env`'s target "is not
checked by the security framework."

**nix-direnv is the interesting hybrid.** It caches the evaluated environment dump at
`${layout_dir}/flake-profile<hash>.rc`, where `<hash>` derives from the flake
expression **[src, `direnvrc`]**. Invalidation is mtime comparison — and the trick is
that **the cached `.rc` file's own mtime is the cache timestamp**, so there is no
separate metadata: it reads the watch list back out of `DIRENV_WATCHES` via
`direnv show_dump` and, per watched file, tests `if [[ $file -nt $profile_rc ]]`
**[src]**. Watched set: `~/.direnvrc`, `.envrc`, `flake.nix`, `flake.lock` **[doc]**.

Note the direction of the naming: `flake-profile<hash>` deliberately *encodes* the
inputs rather than hiding them — content-addressed, the opposite of an opt prefix.
And nix-direnv is the only mechanism of the eight that treats **rooting** as
first-class, calling `_nix_add_gcroot` on both the profile and every flake input
**[src]** — it learned the lesson from Nix's GC failure mode.

`nix_direnv_manual_reload` exists because even a cached 1–2 s rebuild is too much to
pay unexpectedly at a prompt; it prints "cache is out of date. use
'nix-direnv-reload' to reload" instead **[doc]**.

**Cost:** per-prompt latency is the binding constraint. The hook itself is a stat of
the watch list plus a no-op export, but the moment `.envrc` must re-evaluate you pay
interactively. Staleness is mtime-based: `touch flake.nix` invalidates spuriously,
and an mtime-preserving checkout can miss.

### Cross-tool summary

| | (1) When rendered | (2) Stable indirection | (3) Version removal | (4) Cost |
|---|---|---|---|---|
| asdf | never (exec-env sourced per command) | none — concrete `installs/<plugin>/<version>` | shims fully regenerated | ~120 ms per command; zero staleness |
| mise | hook time; cached in `__MISE_SESSION`; per-version `exec_env` cache on disk | `installs/<tool>/<major>` fuzzy symlinks (a liability) | reshim + symlink rebuild; one-prompt window | ~4–14 ms per prompt; elaborate invalidation |
| rbenv family | install time, but content is version-free | none — resolution is a computation | rehash regenerates shims | ~50 ms per exec; breaks `which` |
| nvm | nothing generated; `nvm.sh` sourced live | opt-in `current`, documented as race-prone | n/a (per-shell PATH) | sourcing cost only |
| Nix / home-manager | **regenerated in full every activation** | **yes** — `~/.nix-profile`; store path never exposed | old generation holds refs until GC | slow build; GC can vaporize a live shell's PATH |
| Homebrew | **once at install, `inreplace`d to opt paths**; symlinks repointed on link | **yes** — `opt/<formula>` | non-active harmless; `cleanup` breaks baked Cellar paths | zero runtime cost; delayed staleness |
| /etc/profile.d | **once at install. never again.** | **no** — stable directory only | nothing happens; dangling PATH persists forever | free; highest staleness; no ordering or shell guarantee |
| direnv | **live, every prompt** (add-ons memoize on mtime) | n/a by design | auto-invalidates on watched mtime | per-prompt cost; allow-model friction |

Three patterns fall out. **First, axis (2) is what separates the working mechanisms
from the rotting one.** Nix and Homebrew both solve version-independence with a stable
symlink and both work; profile.d has none and rots. The evidence is on one machine:
`go.sh` names `/opt/go/bin` and is fine, `java.sh` names one of seven JDKs and is one
`rm -rf` from silent breakage.

**Second, there is a genuine trilemma between (1) and (4).** Render-once is free but
rots. Regenerate-on-activation never rots but costs a rebuild. Compute-live never rots
but taxes every prompt. profile.d pays nothing and therefore pays in correctness.
nix-direnv and mise are the two hybrids: memoize the live computation, invalidate on
watched-file mtime — cheap in the common case, correct in the changed case, at the
price of a cache that can miss on mtime-preserving edits.

**Third, stable indirection and lifetime management are separate problems.** Nix's
profile link *is* its GC root — one mechanism, both jobs. Homebrew splits them, and
the seam is exactly where damage happens: `opt` stays correct across upgrade, but
`cleanup` deletes kegs with no knowledge of who referenced them. Any design needs an
answer to "what keeps the target alive?" alongside "what makes the path stable?"

---

### tsuku compatibility

#### What tsuku does today

`install_shell_init` writes `$TSUKU_HOME/share/shell.d/{target}.{shell}` in one of two
ways (`internal/actions/shell_init.go:82-133`):

- **`source_file`** copies a file from the tool's install directory verbatim
  (`shell_init.go:153-179`). For nvm this copies the whole of `nvm.sh`.
- **`source_command`** runs a binary from inside the tool directory and captures
  stdout, substituting `{shell}` and `{install_dir}` (`shell_init.go:182-236`).

Either way the file is written once, its SHA-256 recorded as a `CleanupAction`
(`shell_init.go:143-150`), and `RebuildShellCache` concatenates all `*.{shell}` files
alphabetically into `.init-cache.{shell}`, wrapping each in a brace group so one
tool's runtime failure doesn't break the others
(`internal/shellenv/cache.go:29-169`). `$TSUKU_HOME/env` sources that cache
(`internal/config/config.go:446-465`).

The filename is version-independent by construction (`{target}.{shell}`, and `target`
is a recipe-level constant). The content is whatever the version installed at write
time produced. Nothing re-renders it.

**Confirming the reproduction from the code.** `RemoveVersion` calls
`executeCleanupActions` (`internal/install/remove.go:144`), which skips any cleanup
path that another version of the same tool also references
(`remove.go:332-349`) — "another version still references this path -- skip." Both
nvm 0.40.5 and 0.40.6 recorded `share/shell.d/nvm.bash`, so removing 0.40.6 skips the
delete and the file survives holding **0.40.6's** copy of `nvm.sh` while 0.40.5
becomes active (`remove.go:170-185`). `rebuildShellCaches` then faithfully
re-concatenates the stale content (`remove.go:397-404`). The multi-version safety
check is doing exactly what it was written to do; the missing step is re-rendering
from the surviving version.

The same gap exists on the other lifecycle events: `Manager.Activate`
(`internal/install/manager.go:411-469`) repoints binary symlinks and updates
`ActiveVersion`, and touches nothing under `share/shell.d/`. Neither does `Rollback`
(`manager.go:353`). On upgrade, `cmd/tsuku/update.go:172-192` deletes files the old
version created that the new one doesn't recreate, and `warnShellInitChanges`
(`update.go:232-259`) *warns* when a shared path's content hash changed — so tsuku
already detects the class of drift it doesn't correct.

Note also `set_env` writes `{install_dir}/env.sh` inside the tool directory
(`internal/actions/set_env.go:46`) and nothing sources it. The `00-env-nvm.bash` in
the reproduction is not on this branch; peer leads have traced it to an unmerged
prototype. The `nvm.bash` half of the reproduction is fully reproducible from the
code above.

#### Which model does tsuku resemble today?

**`/etc/profile.d`, almost exactly** — and that is the one model in the survey with no
defenders. Same shape: a directory of shell fragments with version-independent
filenames, written once by an install step, sourced in alphabetical glob order, never
re-rendered on any subsequent event. tsuku is strictly better on three counts: the
ordering is deterministic (Go's `sort.Strings` on ASCII bytes,
`cache.go:105`, not a locale-dependent shell glob); it records content hashes and
verifies them on rebuild (`cache.go:119-131`); and `tsuku doctor` already detects
cache staleness, hash mismatches, symlinks and syntax errors
(`internal/shellenv/doctor.go:52-142`). But the fundamental property — version-specific
content in a version-independent filename with no re-render trigger — is identical,
and so is the failure mode.

tsuku also *already has* the mise-shaped machinery, unused for this problem. There is a
`PROMPT_COMMAND` hook that runs `tsuku hook-env bash` on every prompt
(`internal/hooks/tsuku-activate.bash`), and `ComputeActivation` resolves per-project
tool versions to bin directories **at hook time** using `cfg.ToolBinDir(name, version)`
(`internal/shellenv/activate.go:42`). So per-project PATH is computed live in the mise
style, while shell.d is frozen in the profile.d style. Two different models coexist in
one product.

#### Verifying the "stable indirection is closed off" claim

The claim: `{install_dir}` means the versioned directory by definition, and
`tools/current/` holds binary symlinks rather than tool directories.

**Both halves are correct.**

`{install_dir}` in an `install_shell_init` `source_command` expands to
`ctx.ToolInstallDir` (`shell_init.go:194`), which callers set to
`cfg.ToolDir(name, version)` (`cmd/tsuku/install_deps.go:578`,
`cmd/tsuku/plan_install.go:118`), and `ToolDir` is
`filepath.Join(c.ToolsDir, fmt.Sprintf("%s-%s", name, version))`
(`internal/config/config.go:406-408`). Versioned by construction. (Worth flagging
separately: in the *install* phase `ctx.InstallDir` is a staging directory,
`filepath.Join(workDir, ".install")` at `internal/executor/executor.go:71` — so
`{install_dir}` denotes two different things depending on which action reads it, since
`set_env` uses `ctx.InstallDir` while `install_shell_init` uses `ctx.ToolInstallDir`.)

`tools/current/` entries are created only by `createBinarySymlink`
(`internal/install/manager.go:498-519`) and `createBinaryWrapper`
(`manager.go:559`). Both name the link `filepath.Base(binaryPath)` and target
`ToolDir(tool, version)/<binaryPath>` — a *file* inside the tool directory, not the
directory. And `tools/current` is on `PATH` (`config.go:448`), so its namespace is
binary names, not tool names. Adding directory symlinks there would collide whenever a
tool's binary shares the tool's name, which is the common case.

**But the conclusion the claim supports — that a stable indirection path is closed off
— does not follow. A stable per-tool directory symlink is cheap to add.** Three
reasons:

1. **The primitive already exists and is generic.** `AtomicSymlink` does
   create-temp-then-rename in the same directory (`internal/install/symlink.go:14-41`),
   and `ValidateSymlinkTarget` already enforces containment under `ToolsDir`
   (`symlink.go:46-68`). Neither is specific to binaries. An `opt/<tool> ->
   tools/<tool>-<version>` link is the same two calls with different arguments.

2. **The activation sites are already centralized.** Every event that changes which
   version is active runs through one of four Manager methods that *already* repoint
   symlinks: `InstallWithOptions` (`manager.go:260`), `Activate`
   (`manager.go:449-456`), `Rollback` (`manager.go:353`), and `RemoveVersion`
   (`remove.go:190-204`). Each calls `createSymlinksForBinaries`. Repointing one more
   link alongside is a line in that function plus a removal in `removeToolEntirely`
   (`remove.go:290-294`).

3. **Adding a directory is a well-worn path in `Config`.** `DefaultConfig` and
   `EnsureDirectories` already enumerate ten directories (`config.go:353-403`); an
   `OptDir` follows the existing `LibsDir`/`AppsDir` pattern exactly.

So the prior art's favored shape — Homebrew's `opt_prefix`, Nix's `~/.nix-profile` — is
available to tsuku at low cost. **The important caveat is that it only solves half the
bug.**

A stable indirection path fixes generated content that *names a path*. That is the
`NVM_DIR={install_dir}` half: rewrite it to `NVM_DIR=$TSUKU_HOME/opt/nvm` and the
fragment stops caring which version is active — exactly the
`inreplace lib/"pkgconfig/zlib.pc", prefix, opt_prefix` idiom, which Homebrew
documents as being "useful for e.g. replacing versioned paths in files."

It does nothing for generated content that *is a copy of the version's own bytes*.
`install_shell_init` with `source_file` copies the whole of `nvm.sh` into
`share/shell.d/nvm.bash` (`shell_init.go:161`). No path substitution makes that copy
version-agnostic; it *is* 0.40.6's program text sitting in a file the system will keep
sourcing after 0.40.6 is gone. Note how Homebrew avoids this: its `inreplace`d copies
live *inside the keg*, so they are per-version by location and a stale copy is
impossible. tsuku's copies live in a global directory under a version-independent
filename. That, not the absence of an opt prefix, is the structural mismatch. Fixing it
requires a re-render — recopy from whichever version is now active — on the events
listed above. That is the home-manager model (rebuild the generation on activation),
scoped down to one file.

And for nvm specifically, neither fix is sufficient, because the recipe's model is
wrong independent of staleness. `NVM_DIR` is nvm's data root as well as its program
directory; pointing it at anything tsuku versions or garbage-collects puts the user's
installed node versions and global npm packages inside a directory tsuku considers
disposable. A stable `opt/nvm` symlink would make the *fragment* correct while still
migrating the user's node installs to a fresh empty tree on every nvm upgrade. The
model nvm's own docs and installer assume is a single stable directory whose program
half is swapped in place.

#### What tsuku could adopt cheaply, in rough order of cost

- **Re-render on lifecycle events** (home-manager's shape, minimal scope). Recompute
  `share/shell.d/<target>.<shell>` from the newly-active version at the four sites
  above, then `RebuildShellCache`. Needs the recipe's shell-init step, or its captured
  output, to be replayable per version — which is a state/plan question outside this
  lead. Cost: no runtime overhead, correct at every event, but only at events tsuku
  observes.
- **Stable per-tool opt prefix** (Homebrew's shape). Cheap mechanically as argued above;
  fixes path-naming content permanently, including drift from events tsuku *doesn't*
  observe. Doesn't fix copied content. The two are complementary, not alternatives.
- **Detect-and-report, then detect-and-repair.** `doctor` already computes exactly the
  right thing (`doctor.go:52-142`) and `warnShellInitChanges` already fires on hash
  drift during upgrade (`update.go:232-259`). Wiring those to a repair path is the
  smallest possible increment, and it matches how mise handles the case it can't
  reason about: invalidate on a freshness signal rather than prove correctness.
- **Version-keyed on-disk artifacts** (mise's `exec_env.msgpack.z` shape). Write
  `share/shell.d/<target>-<version>.<shell>` and have the cache builder select the
  active one. A wrong-version file becomes unreachable rather than sourced. Larger
  change to the cache builder and cleanup bookkeeping, but it makes the bug
  *unrepresentable* rather than merely corrected.
- **Compute at hook time** (mise/direnv). tsuku already has the hook
  (`internal/hooks/tsuku-activate.bash`, `cmd/tsuku/hook_env.go`), so the plumbing
  exists. But sourcing several kilobytes of `nvm.sh` on every prompt is a different
  cost class from what the hook does today, and mise's own escape hatches
  (`cache_ttl`, `chpwd_only`) exist because that cost bites in practice. Not cheap.

---

## Implications

The exploration should stop treating "stable indirection" and "re-render" as competing
answers. Homebrew and Nix, the two mechanisms with the best track record, use *both*:
Homebrew has `opt_prefix` **and** rewrites artifacts at build time so they reference it;
Nix has `~/.nix-profile` **and** rebuilds the whole generation on activation. tsuku has
neither. The two candidate fixes address disjoint halves of the bug — indirection fixes
`{install_dir}`-shaped content, re-render fixes byte-copied content — and neither alone
closes the reproduction.

The belief that indirection is architecturally closed off should be retired. It rests on
two true observations that don't support the conclusion: an opt-style symlink needs a new
directory (not `tools/current/`), and the primitives plus the four centralized activation
sites make adding one a small change.

tsuku most resembles `/etc/profile.d`, which is the survey's cautionary tale and the
mechanism a major distribution's policy manual advises against depending on. It is
already better on ordering determinism, hash verification, and health checking — the
remaining gap is precisely the re-render trigger.

nvm should probably be treated as a recipe-level bug in addition to a lifecycle bug.
Even a perfectly re-rendered `NVM_DIR` pointing at `tools/nvm-<version>` puts the user's
node installs and global packages inside a directory tsuku garbage-collects. Any fix that
makes the fragment consistent while leaving that model in place will look correct and
still destroy user data on upgrade.

If the design goes toward detection rather than correction, mise's invalidation discipline
is the model worth copying, and its lesson is that a freshness signal beats a correctness
proof: mise doesn't try to reason about whether a cached `exec-env` is still right, it
just invalidates on install-dir mtime.

---

## Surprises

**mise does persist per-version env to disk, and the way it names the file is the whole
lesson.** `<cache>/<tool>/<version>/exec_env.msgpack.z` is keyed by tool *and version*, so
a wrong-version file is unreachable rather than sourced. tsuku's version-independent
filename is what converts "cache miss" into "wrong content." That reframes the bug: the
filename convention is as much the cause as the missing re-render.

**rbenv has the same class of bug tsuku has, one level up.** Shims bake `RBENV_ROOT` and
the absolute path to the rbenv binary. rbenv's mitigation is a blunt one — if the
freshly-generated prototype differs from an existing shim, `remove_outdated_shims()`
wipes and regenerates the entire directory. That's a real precedent for "detect drift by
regenerating and comparing, then replace wholesale," and it's simpler than anything mise
does.

**Homebrew's `unlink` deliberately leaves the opt symlink in place.** I expected unlink to
be the inverse of link. It isn't: `unlink` removes bin/lib symlinks, `remove_opt_record`
is separate. So `brew unlink foo` takes foo off PATH while `$(brew --prefix foo)` keeps
resolving — a distinction between "not on PATH" and "not addressable" that tsuku's
`tools/current/` currently has no way to express.

**Nobody in the version-manager family uses a stable `current` symlink for env.** rbenv,
pyenv and nodenv have no symlinks at all (verified by grep across all three repos); nvm's
is opt-in and its README documents it as race-prone under multiple shell tabs; asdf has
none. The opt-prefix pattern comes entirely from the *package manager* side (Homebrew,
Nix), not the version-manager side. Version managers solve it by deferring resolution to
invocation time instead. Since tsuku is a package manager that also does version
selection, it inherits the problem from both families but only the second family's
architecture.

**`{install_dir}` means two different directories depending on which action reads it.**
`set_env` expands it from `ctx.InstallDir`, which during the install phase is the staging
directory `workDir/.install` (`executor.go:71`); `install_shell_init` expands it from
`ctx.ToolInstallDir`, the final versioned tool directory. Recipe authors see one
placeholder name. This is adjacent to the lead question but likely relevant to anyone
designing a re-render.

**tsuku already warns about exactly this drift and then does nothing.**
`warnShellInitChanges` compares content hashes across an upgrade and emits "shell init
changed for X" (`update.go:232-259`). The detection is built; only the correction is
missing.

---

## Open Questions

Is the shell-init step replayable per version after install? Re-rendering needs either
the recipe step re-executed against the surviving version's directory, or its output
captured per version in state. `install_shell_init` records a path and a hash in
`CleanupActions` but not the produced content, and `source_command` runs a binary from
inside the tool directory — which for a removed version no longer exists. Peer leads on
the write path and on state/plan replayability own this.

What is the right `NVM_DIR` for a tsuku-installed nvm? Prior art says a single stable
directory whose program half is swapped in place. Whether that means
`$TSUKU_HOME/share/nvm`, `~/.nvm`, or not packaging nvm as a versioned tool at all is a
recipe-design question this lead did not settle.

Which other recipes are affected? I examined only `recipes/n/nvm.toml`.
`recipes/b/bashdb.toml` and `recipes/m/murex.toml` also reference shell.d, and the
`source_command` recipes may be affected differently from the `source_file` ones, since
their content is generated rather than copied.

Does anything besides `RemoveVersion` need the multi-version skip? The skip at
`remove.go:345-349` is correct in intent — deleting a path another version still owns
would break the survivor. The question is whether re-render should replace the skip or
sit alongside it.

Do fish users get any of this? `install_shell_init` accepts `fish`
(`shell_init.go:16-20`), but `RebuildShellCache` is only ever called with bash and zsh
in the paths I traced, `doctor.go:89` iterates only `{"bash", "zsh"}`, and
`EnvFileContent` only sources bash and zsh caches (`config.go:451-455`). Possibly a
separate gap.

What is the actual cost of the re-render? Homebrew pays it at build time, home-manager at
activation. For tsuku, re-rendering on activate/rollback/remove means re-running a recipe
step during what is currently a symlink-repoint. Whether that's milliseconds or a network
round trip depends on the action, and it determines whether re-render can be
unconditional or needs the drift check as a gate.

---

## Summary

Of eight mechanisms surveyed, tsuku's `share/shell.d/` most closely resembles
`/etc/profile.d` — version-specific content under a version-independent filename, written
once and never re-rendered — which is the one model in the survey with a documented
reputation for rotting, while the two mechanisms with the best track record (Homebrew's
`opt_prefix`, Nix generations) both combine a stable indirection path with a re-render
step rather than choosing between them. The claim that stable indirection is closed off is
half right and leads to the wrong conclusion: `{install_dir}` really is versioned
(`config.go:406-408`) and `tools/current/` really is a binary namespace
(`manager.go:498-519`), but `AtomicSymlink` plus the four centralized activation sites make
an `opt/<tool>` link cheap — it just only fixes path-naming content, not
`install_shell_init`'s byte-copy of `nvm.sh` (`shell_init.go:161`), which needs a
re-render. The biggest open question is whether the shell-init step is replayable per
version at all, since `source_command` executes a binary inside a tool directory that a
removal has already deleted.
