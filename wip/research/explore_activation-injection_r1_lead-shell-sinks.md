# Lead: Where else does tsuku emit text that a shell evaluates, and how is each emission quoted?

## Findings

### 0. Summary of the sweep

I walked every place in the tree that produces shell syntax. The sweep was:

- `%q`/`%s` in a format string that also contains `export `, `set -gx`, `set -x `, `source `, `. `, `eval `, `alias `, `PATH=`
- `strconv.Quote` (zero occurrences outside tests)
- `sh -c` / `bash -c`
- `#!/bin/sh` and `#!/usr/bin/env` string literals (generated scripts)
- `os.WriteFile` to a path ending `.sh`/`.bash`/`.zsh`/`.fish`/`.env`/`.profile` (zero direct hits; every shell-text writer builds its path with `filepath.Join`)
- every file in `internal/hook`, `internal/hooks`, `internal/shellenv`, `internal/shim`, and the `shell`/`shellenv`/`hook-env`/`hook`/`doctor`/`completion` commands in `cmd/tsuku`

The headline result: **the `%q` mistake is confined to one function**, but the *absence of any shared answer to "how do I quote for a shell"* is not. Four different packages independently invented four different answers, one of which (`%q`) is simply wrong and one of which (no quoting at all) is missing:

| Package | Its answer to "how do I make this shell-safe?" | Correct? |
|---|---|---|
| `internal/shellenv` | Go's `%q` | **No** (`internal/shellenv/activate.go:134-152`) |
| `cmd/tsuku` | nothing; `%s` inside hand-written `"…"` | **No** (`cmd/tsuku/shellenv.go:40,47`) |
| `internal/actions` | `shellQuote` — single quotes, `'\''` for embedded quotes | **Yes, for POSIX** (`internal/actions/set_env.go:250-254`) |
| `internal/install` | `validateShellSafePath` — reject a denylist of characters | Adequate, but reject-not-quote (`internal/install/manager.go:685-696`) |

A correct POSIX quoter already exists in this repository. It is unexported, in `internal/actions`, and `internal/shellenv` never saw it.

### 1. Evaluated sinks (dangerous)

| # | Emitter | Shell construct | Interpolated values | Value origin | Quoting today | Attacker-reachable? | Evaluated / displayed / sourced |
|---|---|---|---|---|---|---|---|
| S1 | `internal/shellenv/activate.go:138,150-152` (bash/zsh branch) | `export PATH="…"`, `export _TSUKU_DIR="…"`, `export _TSUKU_PREV_PATH="…"` | `result.PATH`, `result.Dir`, `result.PrevPath` | `Dir` is the on-disk directory holding `.tsuku.toml` (`internal/project/config.go:99-103`); `PATH` is tool bin dirs joined with the inherited `$PATH` (`activate.go:104-110`); `PrevPath` is the prior `$PATH` | Go `%q` — escapes `"`, `\`, non-printables; does **not** escape `$` or backtick | **Yes.** A repository containing a directory named `$(…)` with a `.tsuku.toml` inside it is enough | **Evaluated** — `eval "$(tsuku hook-env bash)"` (`internal/hooks/tsuku-activate.bash:3`), `eval "$(tsuku hook-env zsh)"` (`tsuku-activate.zsh:2`), and `eval $(tsuku shell)` (`cmd/tsuku/shell.go:21`) |
| S2 | `internal/shellenv/activate.go:134,146-148` (fish branch) | `set -gx PATH "…"` etc. | same three values | same | same `%q` | **Yes**, same trigger | **Evaluated** — `tsuku hook-env fish \| source` (`internal/hooks/tsuku-activate.fish:2`) |
| S3 | `cmd/tsuku/shellenv.go:40` | `export PATH="<bin>:<current>:$PATH"` | `binDir`, `currentDir` | `filepath.Abs(cfg.HomeDir)`; `HomeDir` is `$TSUKU_HOME`, else `DefaultHomeOverride`, else `~/.tsuku` (`internal/config/config.go:350-366`) | **None at all** — bare `%s` inside a hand-written `"…"` | Narrow but real. See "the dev-build reach" below | **Evaluated** — documented as `eval $(tsuku shellenv)` at `cmd/tsuku/shellenv.go:21` and printed as advice at `cmd/tsuku/install.go:448`, `cmd/tsuku/doctor.go:137,157`, `cmd/tsuku/create.go:666` |
| S4 | `cmd/tsuku/shellenv.go:47` | `. "<cachePath>"` | `cachePath` | `filepath.Join(homeDir, "share", "shell.d", ".init-cache."+shell)` — same `homeDir` as S3 | **None** | same as S3 | **Evaluated**, same command |
| S5 | `internal/shellenv/cache.go:149-150` | comment lines `# tsuku: <name>` and `{ # begin <name>` in the sourced init cache | `toolName` = `DisplayName(fileName, shell)` (`internal/shellenv/selection.go:61-67`) | a filename in `$TSUKU_HOME/share/shell.d/` | none — raw concatenation into a `#` comment | **Not through tsuku's own writers.** Both writers validate: `install_shell_init` requires `^[A-Za-z_][A-Za-z0-9._-]*$` (`internal/actions/shell_init.go:51`), `set_env` requires a clean path segment with no `@` (`internal/actions/set_env.go:216-224`). Only something that can already write a file named `foo<newline>payload.bash` into a 0700 directory can reach it — i.e. it already has the win | **Sourced from a file** — `$TSUKU_HOME/env` sources `.init-cache.$shell` (`internal/config/config.go:477-485`) |

**The dev-build reach for S3/S4.** `cmd/tsuku/shellenv.go:32` calls `filepath.Abs(cfg.HomeDir)`. Dev builds set `DefaultHomeOverride` to the **relative** string `.tsuku-dev` via ldflags (`Makefile:8`, `cmd/tsuku/main.go:26-29`, `internal/config/config.go:353-356`), so `Abs` prepends the current working directory. On a dev build, `cd` into a checkout whose path contains `$(…)` and run `eval $(tsuku shellenv)` and the substitution fires. For release builds the value comes from `$TSUKU_HOME` or `~/.tsuku`, so it is mostly self-inflicted. I rate S3/S4 lower than S1/S2 for that reason, but the fix is the same and the emitter has strictly less quoting than the one that is already confirmed exploitable.

**Confirmation of the S1 mechanism.** `%q` output for shell-hostile inputs (Go 1.x, run locally):

```
in=/tmp/proj/$(echo INJECTED)      %q=> "/tmp/proj/$(echo INJECTED)"
in=/tmp/proj/`echo BACKTICK`       %q=> "/tmp/proj/`echo BACKTICK`"
in=/tmp/a"b                        %q=> "/tmp/a\"b"
in=                                %q=> ""
in=/tmp/new\nline                  %q=> "/tmp/new\nline"
in=$HOME/x                         %q=> "$HOME/x"
```

and the resulting line under bash (sourced, which has identical evaluation rules to `eval`; the sandbox here refuses a bare `eval`):

```
export _TSUKU_DIR="/tmp/proj/$(echo MARKER)"
_TSUKU_DIR=/tmp/proj/MARKER
```

Two side notes from that table. `%q` renders a real newline as the two characters `\n`, which inside bash double quotes is a literal backslash-n — so a value containing a newline is silently **corrupted**, not just unsafe. And `$HOME/x` expands, so any legitimate path containing a dollar sign is already broken today.

### 2. Scripts written to disk and later executed by a shell

These are shell text too — the file is `#!/bin/sh` and something runs it. None of them uses `%q`; the pattern here is validate-then-interpolate, with varying rigour. Inputs are recipe-controlled, which is a supply-chain surface the lead scopes out, so I rate these low and list them for completeness of the class rather than as live defects.

| # | Emitter | Construct | Interpolated | Origin | Quoting today | Verdict |
|---|---|---|---|---|---|---|
| W1 | `internal/install/manager.go:698-736` (`generateWrapperScript`) | `PATH="…:$PATH"`, `LD_LIBRARY_PATH="…"`, `exec "<target>" "$@"` | `targetPath`, `pathAdditions`, `libPathAdditions` | install dirs and dependency bin dirs | every value passes `validateShellSafePath` first (`manager.go:591,615,676`), which rejects `\n \r " ' ` $ \ ;` | **Guarded.** Reject-list rather than quoting, but the list covers everything that matters inside double quotes |
| W2 | `internal/actions/set_rpath.go:355-367` (`createLibraryWrapper`) | `exec "$SCRIPT_DIR"/'<name>' "$@"` | `origBaseName` | `filepath.Base(binaryPath)` | single-quoted **and** validated against `^[a-zA-Z0-9._-]+$` (`set_rpath.go:463-471`) | **Guarded** |
| W3 | `internal/actions/nix_install.go:238-260`, `nix_realize.go:357-380` | `export NP_LOCATION="…"` / `exec "<np>/nix-portable" nix shell "<ref>" -c <exe> "$@"` | `exe`, `npLocation`, `packageName`, `flakeRef` | recipe params | `packageName` via `isValidNixPackage` (`nix_install.go:182-`), `flakeRef` via `isValidFlakeRef` (`nix_realize.go:270-295`), `exe` via `validateExecutableName` (`nix_install.go:213-231`). `npLocation` is a tsuku-built path, unvalidated | **Weak spot worth noting:** `exe` is interpolated **unquoted** as the `-c` argument, and `validateExecutableName` permits space, `'`, `"`, `*`, `?`, `~`, `#`, `=`. It blocks `$ \` \| ; & < > ( ) [ ] { }`, so this is argv/glob injection into `nix shell`, not command execution. Still the one place here where the guard and the quoting context disagree |
| W4 | `internal/actions/gem_common.go:12-28,67` | `export GEM_HOME="$INSTALL_DIR/%s"`, `export PATH="%s:$PATH"`, `exec ruby "$SCRIPT_DIR/%s.gem"` | `exeName`, `gemHomeRel`, `rubyBinDir` | gem's own `bin/` names; tsuku-built paths | **none** | Lowest-confidence item in the table. All three land inside double quotes, so a gem shipping an executable named `$(…)` would inject. I did not trace whether the gem executable list is filtered upstream of `createGemWrapper` |
| W5 | `internal/actions/util.go:585-627` (zig wrappers) | `#!/bin/sh … exec "<zigPath>" ar "$@"` etc. | `zigPath`, `ldWrapper`, `arWrapper`, `ranlibWrapper` | tsuku-built install paths | none, but all are `filepath.Join` of `$TSUKU_HOME` subpaths | Low |

### 3. Developer/CI script builders (not on a user's machine)

`internal/sandbox/executor.go:639-716`, `internal/validate/executor.go:298-330`, `internal/validate/source_build.go:216,254` build `#!/bin/sh` scripts that run **inside a container** during recipe validation, and they splice `recipe.Verify.Command` in verbatim (`sandbox/executor.go:693`, `validate/executor.go:326`). That is by design — `verify.command` *is* a shell command. Same for `cmd/tsuku/verify.go:251,309,392` (`sh -c <verify command>`) and `internal/actions/run_command.go:96` (`sh -c` on the recipe's `command`, after `{install_dir}`-style expansion at `run_command.go:88`). These are the recipe-as-code surface, not a quoting defect. Out of scope per the lead. Noted so nobody re-derives them.

### 4. Places the lead suspected that turn out clean

- **`internal/hook`** — clean. The rc-file snippet is two constant lines with the shell *name* appended to a filename, nothing else: `markerBlock` and `activateMarkerBlock` at `internal/hook/install.go:40-47`. No path to the tsuku binary, no `$TSUKU_HOME` value, no user string. The shell name is validated by the `switch` at `install.go:58-65`/`119-133`. `uninstall.go` and `status.go` interpolate nothing into shell text (`grep` for `Sprintf` in those two files finds only error messages). The fish install path copies a byte-for-byte embedded file (`install.go:97-111`).
- **`internal/hooks`** — the six hook scripts are `//go:embed`ed constants written out verbatim (`internal/hooks/embed.go:15-39`). Nothing is templated.
- **`internal/shim`** — clean, and notably the *right* pattern: `ShimContent` (`internal/shim/manager.go:18`) is a single constant, `#!/bin/sh\nexec tsuku run "$(basename "$0")" -- "$@"\n`. The tool name is discovered at runtime from `$0` and correctly double-quoted, so no name ever reaches the file. One template for every tool.
- **`internal/shellenv/precedence.go`, `doctor.go`, `selection.go`** — no shell emission. `precedence.go` walks PATH and compares paths; `doctor.go` reads and hashes; `selection.go` is filename arithmetic. `doctor.go:120-131` shells out to `bash -n`/`zsh -n` for a syntax check, which is exec-argv territory, not text a shell parses as tsuku wrote it.
- **`internal/actions/completions.go`** — the completion script content is the tool's own output copied verbatim (`completions.go:131-168`); tsuku templates nothing into it. Only the *filename* is composed (`completionFileName`, line 113).

### 5. Where the quoter belongs

**Recommendation: a new leaf package, `internal/shellquote`, importing nothing but the standard library, exporting `POSIX(string) string` and `Fish(string) string`.**

The import graph makes this the only placement that does not force an awkward dependency. `go list -deps` gives:

```
internal/shellenv  -> internal/config, internal/project, internal/platform, …
internal/install   -> internal/shellenv (direct: precedence.go, shelld.go, update.go, remove.go)
internal/actions   -> internal/install -> internal/shellenv
cmd/tsuku          -> all of the above
```

So `internal/shellenv` sits *below* both `internal/install` and `internal/actions`, and putting the quoter next to `FormatExports` would be cycle-free. I still recommend against it, for two reasons:

1. **Three packages need it, not one.** `internal/shellenv` (S1, S2, S5), `cmd/tsuku` (S3, S4), and `internal/actions` (already has its own copy at `set_env.go:252`, and W3/W4 are unquoted). `internal/install` has a fourth variant. Leaving the quoter inside a package named for per-directory PATH activation guarantees the next author writes a fifth one, which is exactly the history this defect has.
2. **`internal/actions/set_env.go` importing `internal/shellenv` to get a string function reads as a layering mistake** even though the compiler allows it. A leaf package with no tsuku imports can never create a cycle and never will, whoever needs it later.

Precedent for tiny leaf packages already exists in the tree: `internal/httputil`, `internal/platform`, `internal/errmsg`.

Concretely: move the body of `internal/actions/set_env.go:250-254` into `shellquote.POSIX` verbatim (it is already correct), add `shellquote.Fish`, then rewrite the six `%q` calls in `activate.go` and the two `%s`-in-`"…"` calls in `cmd/tsuku/shellenv.go`. `internal/install`'s `validateShellSafePath` can stay as defence in depth or be replaced; it is not currently broken.

### 6. The quoting rules to implement

I could not run fish or zsh here — neither is installed on this machine (`/usr/bin/fish`, `/usr/bin/zsh`, `/bin/zsh` all absent), so the POSIX rules below are from the specification and confirmed under bash locally, and the fish rules are from fish's own documentation. That distinction matters for the fish rule, which is genuinely not the same as POSIX.

**POSIX shells (sh, bash, zsh).** Source: POSIX.1-2017, *Shell Command Language* §2.2.2 (Single-Quotes) and §2.2.3 (Double-Quotes). §2.2.2: "Enclosing characters in single-quotes shall preserve the literal value of each character within the single-quotes. A single-quote cannot occur within single-quotes." §2.2.3 lists `$`, `` ` ``, `\` and `"` as retaining special meaning inside double quotes, with parameter expansion, command substitution and arithmetic expansion all performed — which is precisely why the double-quoted form `%q` produces is the wrong container.

Therefore, the only correct general form is single quotes with the close-reopen idiom:

```go
func POSIX(s string) string {
    return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
```

- Empty string: yields `''`. This is required, not optional — an unquoted empty value disappears from the command line entirely, and `export X=` versus `export X=''` differ in some contexts.
- Newline: needs **no** escaping. A literal newline between single quotes is preserved verbatim (§2.2.2 admits no exception). The emitted "line" then spans two physical lines, which is ugly but correct. Do *not* try to escape it — there is no escape inside single quotes.
- Backslash, `$`, backtick, `"`, `;`, `&`, `|`, glob characters, whitespace: all literal inside single quotes, no special handling.

This is exactly what `internal/actions/set_env.go:252` already implements.

**fish.** Source: fish documentation, *The fish language → Quotes*. The rule that differs: **fish's single quotes are not fully literal.** Inside `'…'`, fish recognises two escapes — `\'` for a single quote and `\\` for a backslash — and treats every other character literally. POSIX single quotes recognise none. So a value containing a backslash that is correct under POSIX single-quoting is *wrong* under fish single-quoting: `'a\b'` is `a\b` to sh and `a\b` to fish only because `\b` is not one of the two recognised escapes, but `'a\\b'` is `a\\b` to sh and `a\b` to fish. The two dialects genuinely disagree.

```go
func Fish(s string) string {
    s = strings.ReplaceAll(s, `\`, `\\`)  // backslash FIRST
    s = strings.ReplaceAll(s, `'`, `\'`)
    return "'" + s + "'"
}
```

- Order is load-bearing: escape backslashes before quotes, or the backslash you just introduced to escape a quote gets escaped again.
- Empty string: `''`, same as POSIX.
- Newline: literal inside fish single quotes, no escaping.
- The POSIX `'\''` idiom does also happen to work in fish, because fish concatenates adjacent tokens with no whitespace between them the way POSIX shells do — but it works by a different mechanism than the one fish documents, and `\'` is what fish's own `string escape` emits. Use the native form.
- fish's double quotes expand `$var`, and support `$(…)` command substitution since fish 3.4 — so the current `%q` output is exploitable on fish too, on any reasonably current version. Backticks were deprecated in fish 3.4 and removed in fish 4.0, so the backtick half of the payload is version-dependent on fish while `$(…)` is not. This is the detail behind the retraction the lead mentions: the bug is not fish-specific, and fish is not immune either.

**One fish-specific correctness note beyond quoting.** `set -gx PATH '<colon-joined string>'` (`activate.go:134,146`) sets PATH to a **single list element containing colons**. fish models PATH as a list. It survives export because fish joins path-variable elements with `:` on the way out, but `$PATH[1]` and every idiom that treats PATH as a list is then wrong inside that shell. The fish branch should emit the directories as separate arguments — `set -gx PATH 'dir1' 'dir2' …` — each quoted with `Fish`. Not a security issue; worth fixing in the same change since the fish branch is being rewritten anyway.

## Implications

- The fix is not a one-line change in `FormatExports`. Two commands emit evaluated shell text (`tsuku hook-env`, `tsuku shell`, both through `FormatExports`) and a third emits it with no quoting at all (`tsuku shellenv`). All three need the same quoter.
- The correct POSIX quoter has existed in this repository the whole time, in `internal/actions`, unexported and one package away from the code that got it wrong. That is the strongest argument for making it shared and named: the knowledge was present and the packaging hid it.
- Two dialects means two functions. A single `ShellQuote` that gets fish wrong is a worse outcome than today's bug, because it would look correct.
- Every generated wrapper script currently relies on validate-then-interpolate. Those are fine as they stand, but once a real quoter exists, W3 and W4 are cheap to convert and should be, so there is one answer in the tree instead of four.

## Surprises

- `internal/hook` is entirely clean. The lead's prime suspect — a path to the tsuku binary or `$TSUKU_HOME` baked into `.bashrc` — does not exist; the marker block hardcodes `${TSUKU_HOME:-$HOME/.tsuku}` as *shell text the user's shell expands*, which is the right call (`internal/hook/install.go:40-47`).
- `internal/shim` is a model for the whole class: it refuses to interpolate at all and lets the shell find the name at runtime via `"$(basename "$0")"`.
- `cmd/tsuku/shellenv.go` has *less* protection than the known defect and nobody has flagged it. `%q` at least escapes `"`; line 40 escapes nothing.
- `%q` corrupts newlines in addition to being unsafe — `\n` becomes a literal backslash-n in the shell's view. Any test asserting round-trip fidelity on such a value would have caught this.
- `tsuku shellenv` emits POSIX `export PATH="…"` regardless of shell, yet `internal/actions/shell_init.go:38` tells fish users to run `tsuku shellenv | source`. fish cannot parse `export`. Unrelated to security, but it means that advertised path has never worked. `detectShellForEnv` (`cmd/tsuku/shellenv.go:56-65`) returns `"fish"` only to pick a cache filename that nothing ever writes, since both shell.d writers reject fish.

## Open Questions

- **W4, the gem wrapper.** Is `exeName` filtered anywhere upstream of `createGemWrapper` (`internal/actions/gem_common.go:41`)? It reaches three double-quoted contexts unquoted and unvalidated. I traced the emitter but not the two call sites in `gem_install.go` / `gem_exec.go` back to their source of executable names.
- **W3's `exe`.** `validateExecutableName` (`internal/actions/nix_install.go:213-231`) permits whitespace and glob characters, and `exe` is interpolated unquoted. I am confident this is not command execution; I have not worked out whether argument injection into `nix shell -c` is exploitable for anything worse than a confusing failure.
- **`result.PATH` reachability.** The `Dir` route into S1 is trivial. The `PATH` route needs a tool bin directory that both exists on disk (`activate.go:91`) and has an injecting name, which pushes it into the path-construction lead's territory (`cfg.ToolBinDir(name, version)` with `name`/`version` straight from `.tsuku.toml`). Worth a handoff note rather than duplicated work.
- **Whether S5 deserves a fix at all.** Reaching it requires write access to a 0700 directory under `$TSUKU_HOME`, at which point the attacker can write the fragment content itself. Quoting the comment costs nothing, but it is defence in depth, not a defect.

## Summary

The `%q`-as-shell-quoting mistake is confined to `FormatExports`, but it is one of four different and inconsistent answers to shell quoting in the tree — `internal/actions` has a correct POSIX quoter (`set_env.go:252`), `internal/install` rejects a denylist, and `cmd/tsuku/shellenv.go:40,47` does nothing at all while emitting text documented as `eval $(tsuku shellenv)`, giving it strictly less protection than the known defect. Because three packages plus `cmd/tsuku` emit evaluated shell text, the quoter must be a leaf package (`internal/shellquote`) with separate `POSIX` and `Fish` functions — fish's single quotes recognise `\'` and `\\` where POSIX's recognise nothing, so one function that covers both would be wrong for one of them. The biggest open question is the gem wrapper at `internal/actions/gem_common.go:12-28`, the one generated script that interpolates an externally-supplied name into double quotes with neither validation nor quoting.
