# Decision: Shell Environment Activation Mechanism

## Question

What activation mechanism should tsuku use to detect directory changes and trigger environment updates? Covers prompt hooks vs explicit command, shell-specific hook installation, and whether hooks are opt-in or opt-out.

## Decision

**Chosen approach: Prompt hook with early-exit guard, plus explicit `tsuku shell` fallback**

Use a single prompt-based hook per shell (PROMPT_COMMAND for bash, precmd for zsh, fish_prompt event for fish) that calls `tsuku hook-env`. The hook-env command exits immediately (< 1ms) when the directory hasn't changed, and only performs config lookup + PATH rewrite on actual directory changes. Hooks are opt-in, installed via `tsuku hook install`. The explicit `tsuku shell` command provides identical functionality without hooks.

## Confidence

High. This is the established pattern across mise and direnv. The early-exit optimization is proven to keep per-prompt cost negligible. The explicit fallback satisfies the "hooks are optional" constraint with zero additional design complexity.

## Alternatives Considered

### Alternative A: cd/pushd/popd wrapper (event-driven)

Wrap `cd`, `pushd`, and `popd` to detect directory changes at the moment they happen, rather than polling on every prompt.

**Strengths:**
- Only runs on actual directory changes, not every prompt
- No per-prompt overhead whatsoever
- Conceptually clean: activation is triggered by the cause (directory change), not a side effect (prompt rendering)

**Weaknesses:**
- Misses directory changes from external sources: `CDPATH`, shell scripts that manipulate `PWD`, subshells returning to a different directory, `git worktree` switches
- Bash `cd` wrapping is fragile -- other tools (rvm, nvm, virtualenvwrapper) also wrap `cd`, creating ordering conflicts
- Fish doesn't have `cd` as a wrappable function in the same way; requires `--on-variable PWD` which is effectively a prompt hook anyway
- zsh has `chpwd_functions` (clean) but bash/fish don't have an equivalent, so shell implementations diverge significantly
- Three wrappers (cd/pushd/popd) per shell, with edge cases around `cd -`, `cd ~`, and CDPATH resolution

**Why rejected:** The failure modes are subtle and hard to debug. A user does `git checkout` in a worktree, the directory context changes but no `cd` was called, and they silently get the wrong tool version. The prompt hook with early-exit has identical performance in practice (sub-millisecond when directory is unchanged) without any of these correctness gaps.

### Alternative B: Dual hooks (chpwd + precmd)

Use zsh's `chpwd_functions` for directory-change events, combined with `precmd_functions` as a safety net. For bash, wrap cd + use PROMPT_COMMAND. For fish, use `--on-variable PWD` + `fish_prompt`.

**Strengths:**
- Activates at the earliest possible moment (chpwd fires before the prompt)
- Marginally faster in zsh: chpwd avoids the directory-comparison check since zsh guarantees it only fires on change
- This is what mise does

**Weaknesses:**
- More complex: two hook registration points per shell instead of one
- The "earliest moment" advantage is negligible -- users don't notice 0.5ms of latency difference between chpwd and precmd
- Bash doesn't have a chpwd equivalent, so the implementation diverges: bash uses PROMPT_COMMAND only (or cd wrappers), while zsh uses chpwd + precmd. Fish uses `--on-variable PWD`. Three different strategies for three shells
- The precmd safety net exists because chpwd alone misses the initial shell startup case. This means precmd does the same work anyway -- chpwd is redundant for correctness
- More hook registrations mean more potential conflicts with other tools

**Why rejected:** Added complexity without meaningful benefit. The prompt-only hook already handles all cases (including initial startup) with a single codepath per shell. The sub-millisecond early-exit makes the "unnecessary check on same directory" cost irrelevant. Mise uses dual hooks, but mise also has a much larger surface area of shell state to manage (environment variables, PATH, aliases). For tsuku's narrower scope (PATH only), the simpler approach is sufficient.

### Alternative C: Shim-based resolution (no hooks at all)

Like asdf, place shim scripts in `$TSUKU_HOME/bin/` that resolve the correct version at invocation time by reading `.tsuku.toml` from the current directory.

**Strengths:**
- Zero shell integration required -- no hooks, no shell-specific code
- Works in every shell and in non-interactive contexts (cron, scripts, CI)
- Conceptually simple: each shim is a small script that calls `tsuku exec <tool> -- "$@"`

**Weaknesses:**
- Per-invocation cost: every command execution pays the config lookup + version resolution penalty. Even with caching, this adds 20-50ms per invocation for common tools like `node`, `go`, `python`
- Shims break tools that inspect their own `argv[0]` or follow symlinks to determine behavior (busybox-style multi-call binaries, python's `sys.executable`)
- `which node` returns the shim path, not the real binary -- confusing for debugging
- Shims must be regenerated when new tools are installed
- PATH ordering becomes critical and error-prone: shims must come before system binaries but after any version-specific overrides

**Why rejected:** The per-invocation overhead violates the performance constraint. The 50ms budget is for the prompt hook; adding 20-50ms to every tool invocation is a worse trade-off. Shims also create confusing debugging experiences. The design doc explicitly calls out shims as the domain of Block 6 (project-aware exec wrapper, #2168), where they serve a different purpose (auto-install on first use in non-interactive contexts).

## Detailed Design

### Hook Architecture

A new `tsuku hook-env` subcommand serves as the single entry point called by all shell hooks. It:

1. Compares `$PWD` against a cached "last seen directory" (stored in `$_TSUKU_DIR`)
2. If unchanged, exits with no output (cost: fork + exec + string comparison, well under 5ms)
3. If changed, calls `LoadProjectConfig($PWD)` to find `.tsuku.toml`
4. Computes the required PATH entries from the config
5. Outputs shell commands to update PATH and `$_TSUKU_DIR`

### Shell Hook Scripts

**Bash:**
```bash
_tsuku_hook() {
  local previous_exit_status=$?
  eval "$(tsuku hook-env bash)"
  return $previous_exit_status
}
if [[ ";${PROMPT_COMMAND[*]:-};" != *";_tsuku_hook;"* ]]; then
  PROMPT_COMMAND=("_tsuku_hook" "${PROMPT_COMMAND[@]}")
fi
```

**Zsh:**
```zsh
_tsuku_hook() {
  eval "$(tsuku hook-env zsh)"
}
if (( ! ${precmd_functions[(I)_tsuku_hook]} )); then
  precmd_functions=(_tsuku_hook $precmd_functions)
fi
```

**Fish:**
```fish
function _tsuku_hook --on-event fish_prompt
  tsuku hook-env fish | source
end
```

### Early-Exit Optimization

The `hook-env` command's fast path does no filesystem I/O. The shell hook stores the last-activated directory in `$_TSUKU_DIR`. The hook-env command receives this as an environment variable and compares it against `$PWD`. If they match, it prints nothing and exits. The shell `eval`/`source` of empty output is a no-op.

This means the per-prompt cost in the common case (no directory change) is:
- One fork+exec of `tsuku hook-env` (~2-4ms on Linux)
- One string comparison
- Exit with empty stdout

### Hook Installation

Extend the existing `tsuku hook install` command. Today it installs command-not-found hooks only. After this change, it installs both command-not-found and activation hooks. The two hook types share the same source file, so a single `source` line in the rc file covers both.

Hooks remain opt-in. Users who don't run `tsuku hook install` get no prompt hooks. They can use `eval $(tsuku shell)` or the future `tsuku exec` for explicit activation.

### Coexistence with Existing Hooks

- The prompt hook function uses a unique name (`_tsuku_hook`) that won't collide with existing command-not-found hooks (`command_not_found_handle`)
- PROMPT_COMMAND (bash) is treated as an array and prepended to, not overwritten
- precmd_functions (zsh) is prepended to, preserving existing entries
- fish_prompt event handler coexists with other handlers by design

### `tsuku shell` as Explicit Alternative

For users who don't want prompt hooks:

```bash
eval "$(tsuku shell)"
```

This reads `.tsuku.toml` from `$PWD`, computes the PATH modification, and prints export statements. It's a one-shot activation, not a hook. Users call it manually when they enter a project. This is simpler but requires discipline -- forgotten activations lead to wrong versions.

## Assumptions

1. Fork+exec cost for `tsuku hook-env` stays under 5ms on modern Linux/macOS. This is consistent with measured performance of similar tools (mise hook-env, direnv export).
2. The `.tsuku.toml` config lookup (filesystem stat calls walking up directories) completes under 10ms even on networked filesystems. The ceiling at `$HOME` bounds traversal depth.
3. Users who want automatic activation will accept a one-time `tsuku hook install` setup step.
4. Bash 4+ array syntax for PROMPT_COMMAND is acceptable. Bash 3 (macOS default before Catalina) treats PROMPT_COMMAND as a string, requiring different handling. The hook should handle both forms.

## Risks

- **Bash PROMPT_COMMAND array vs string**: Older bash versions treat PROMPT_COMMAND as a single string. The hook must detect this and fall back to string concatenation with a semicolon separator.
- **Shell startup time**: If many tools register prompt hooks, cumulative cost grows. Tsuku's hook should be defensive about this by keeping its fast path as cheap as possible.
- **NFS/slow filesystems**: The directory comparison optimization eliminates filesystem access on the fast path, but the slow path (directory changed, need to find `.tsuku.toml`) could be slow on NFS. The ceiling-path mechanism (`TSUKU_CEILING_PATHS`) provides an escape hatch.
