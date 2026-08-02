---
schema: plan/v1
status: Draft
execution_mode: single-pr
upstream: docs/designs/DESIGN-shell-d-lifecycle.md
issue_count: 7
---

# PLAN: a correct lifecycle for share/shell.d

## Status

Draft

## Scope Summary

Version-key the `$TSUKU_HOME/share/shell.d/` filename as `<target>@<version>.<shell>` so
installing a second version no longer overwrites the first, and teach the cache builder to
exclude a recorded file whose version is not active. Route `set_env` through the same
delivery so its exports reach the user's shell (issue #2439). Hook the three sites that
assign `ToolState.ActiveVersion` so install, upgrade, `activate`, rollback, and
multi-version removal all select the correct version's fragment, and fix the five
consumers that the rename would otherwise regress.

## Decomposition Strategy

**Vertical, strictly sequenced.** Each issue leaves the tree building and green, so a
reviewer can read the PR commit by commit. Issue 1 introduces the second writer the
lifecycle work must cover; issues 2-4 are the mechanism; issue 5 is the collateral that
must land with the rename or it regresses; issues 6-7 are verification and documentation.
There is no parallelism worth exploiting — every issue touches the same files.

Issue 5 is the risk concentration. It contains the only change whose omission fails
silently rather than loudly.

## Issue Outlines

### Issue 1: fix(actions): make set_env write to shell.d and expand from ToolInstallDir

**Complexity:** testable

**Goal:** Resolve issue #2439. Rewrite `SetEnvAction.Execute` to write
`share/shell.d/00-env-<name>.<shell>` for bash and zsh instead of an `env.sh` nothing
reads, expanding `{install_dir}` from `ToolInstallDir` rather than the staging
`InstallDir`. Add an optional `PhaseDeclarer` interface to `internal/actions`, implemented
by `SetEnvAction` returning `post-install`, and have `executor.StepPhase` consult it
through the action registry when a recipe names no phase. Record each file as a
`CleanupAction` with a content hash.

**Acceptance Criteria:**
- `set_env` writes `share/shell.d/00-env-<recipe name>.<shell>` for bash and zsh; no
  `env.sh` is written anywhere
- `{install_dir}` expands from `ctx.ToolInstallDir`
- Variable names must match `[A-Za-z_][A-Za-z0-9_]*`; values may not contain newlines;
  values are single-quoted with POSIX escaping, validated *after* placeholder substitution
- `StepPhase` resolves an unnamed phase through the registry at execution time, not at
  plan-generation time, so stored plans keep routing correctly
- `set_env` is rejected in library recipes, and a `phase` override the action cannot
  honour is rejected, both in `internal/recipe/validator.go`
- Multiple `set_env` steps in one recipe append to one file rather than recording
  duplicate cleanup actions
- `recipes/n/nvm.toml` resolves `NVM_DIR` to the tool's install directory
- A test sources `$TSUKU_HOME/env` in a real bash subshell and reads the variable back
- `go test ./...`, `go vet ./...`, `gofmt -l` clean

**Dependencies:** None

---

### Issue 2: feat(actions): version-key shell.d filenames

**Complexity:** testable

**Goal:** Both writers produce `<target>@<version>.<shell>` (and
`00-env-<name>@<version>.<shell>`). Add the `target` charset preflight that makes the
naming provably injective and strengthens the exports-before-init sort guarantee.

**Acceptance Criteria:**
- `install_shell_init` writes `share/shell.d/<target>@<version>.<shell>`
- `set_env` writes `share/shell.d/00-env-<name>@<version>.<shell>`
- Recorded `CleanupAction.Path` values match what is written
- `install_shell_init` preflight rejects a `target` not matching
  `^[A-Za-z_][A-Za-z0-9._-]*$`, replacing the narrower `00-env-` prefix rejection
- A table test covers injectivity, including tool `foo-1` at version `2` versus tool `foo`
  at version `1-2`
- A test asserts every `00-env-*` file sorts before every legal init filename
- `go test ./...` clean

**Dependencies:** Issue 1

---

### Issue 3: feat(shellenv): exclude recorded fragments belonging to inactive versions

**Complexity:** testable

**Goal:** Add `shellenv.ShellDSelection` and `install.BuildShellDSelection`, and teach
`RebuildShellCache` to skip an entry that appears in `Known` but not in `Active`. Apply the
identical filter in `isCacheStale` so the two agree.

**Acceptance Criteria:**
- `ShellDSelection` has `Active` and `Known`, both `map[string]string` of
  `$TSUKU_HOME`-relative path to recorded SHA-256
- `BuildShellDSelection(*State)` is pure — no filesystem access — and is table-tested
- `RebuildShellCache` excludes a file iff it is in `Known` and not in `Active`; a file in
  neither is included exactly as before
- The parameter stays optional: passing nothing excludes nothing, so existing callers and
  tests keep their current semantics
- `isCacheStale` applies the same exclusion, so `doctor` does not report a permanent stale
  cache on a multi-version install
- A test asserts `doctor --fix` converges: run it twice, second run reports clean
- `go test ./...` clean

**Dependencies:** Issue 2

---

### Issue 4: fix(install): rebuild shell caches at every active-version change

**Complexity:** testable

**Goal:** Hook the three sites that assign `ToolState.ActiveVersion`, each after its state
write. Fix the two removal-path defects: `RemoveVersion` rebuilds caches before its
promotion, and `executeCleanupActions` populates `affectedShells` only for cleanups it
actually performed.

**Acceptance Criteria:**
- `Manager.Activate` rebuilds the affected shell caches after `UpdateTool`, covering
  `tsuku activate`, `tsuku rollback`, and auto-apply's failure rollback
- `Manager.RemoveVersion`'s promotion branch rebuilds after the promotion, not before
- `executeCleanupActions` marks a shell affected when it *skips* a path retained by
  another version
- All three sites pass a `ShellDSelection` built from post-write state
- A test asserts `activate` between two installed versions changes which fragment is in
  the cache
- `go test ./...` clean

**Dependencies:** Issue 3

---

### Issue 5: fix(install): correct the consumers the rename would otherwise regress

**Complexity:** testable

**Goal:** Five consumers assume one shell.d file per tool. Four are pre-existing bugs; the
first is a defect the rename creates and is the only change here whose omission fails
silently.

**Acceptance Criteria:**
- `StaleCleanupActions` / `ExecuteStaleCleanup` will not delete a path any installed
  version still records — the guard `executeCleanupActions` already has
- A dedicated test upgrades a tool and then rolls back, asserting the rollback target's
  shell.d fragment survives and the rolled-back shell gets its content
- `HasShellIntegration` no longer probes `{toolName}.<shell>` by filename; it reads the
  active version's recorded paths, which is also correct for a recipe whose `target`
  differs from its tool name
- `GarbageCollectVersions` runs the deleted version's `delete_file` cleanup actions and
  drops its `VersionState`
- `warnShellInitChanges`, the cache's `# tsuku: <name>` comment, and `doctor`'s
  `ActiveScripts` derive a display name of `nvm`, not `nvm@0.40.6`
- `cmd/tsuku/plan_install.go` records cleanup actions, via one shared helper extracted
  from the post-install block duplicated in `install_deps.go`
- `go test ./...` clean

**Dependencies:** Issue 4

---

### Issue 6: test: cover the multi-version shell.d lifecycle end to end

**Complexity:** testable

**Goal:** Prove the acceptance criteria behaviourally rather than by file inspection, and
mutation-test each guard.

**Acceptance Criteria:**
- A test installs two versions of a synthetic tool exercising *both* writers, removes one,
  sources `$TSUKU_HOME/env` in a hermetic `bash --norc --noprofile` subshell, and asserts
  the exported variable points at the surviving version's directory
- The same assertion runs for removing the active version and for removing the inactive one
- A test covers `activate` and `rollback` the same way
- A test asserts `tsuku doctor` reports no content-hash mismatch afterwards and that the
  recorded `ContentHash` matches disk
- Mutation tests applied and recorded in the PR body: revert the `StaleCleanupActions`
  guard; skip the cache rebuild after a promotion; invert the `Known`/`Active` exclusion;
  fix only one of the two writers. Each must fail the test meant to catch it
- No test is `testing.Short()`-gated; tests write only under `t.TempDir()`
- `go test ./...`, `go vet ./...`, `gofmt -l` clean; `git status --porcelain` empty after
  the run

**Dependencies:** Issue 5

---

### Issue 7: docs: update the plugin skills and file the deferred issues

**Complexity:** trivial

**Goal:** Satisfy the repo's plugin-maintenance table and record what was deliberately
left out.

**Acceptance Criteria:**
- `plugins/tsuku-recipes/skills/recipe-author/`: the `set_env` action reference no longer
  describes the `env.sh` behaviour that never worked; the `vars` parameter is documented as
  a list of `{name, value}` objects rather than a map; `install_shell_init` documents its
  output path, the version-keyed filename, the alphabetical ordering contract, and the
  reserved `00-env-` prefix; the phase concept is documented
- `plugins/tsuku-user/skills/tsuku-user/`: shell.d description updated for version-keyed
  filenames; `activate` and `rollback` documented; the doctor content-hash diagnostic named
- `plugins/tsuku-recipes/skills/recipe-test/`: unchanged if the `--plan` path now records
  cleanup actions; verified either way
- Follow-up issues filed for `install_completions`, library post-install, the dependency
  installer's discarded cleanup actions, the already-installed short-circuit and the
  missing `--reinstall` flag, and `recipes/n/nvm.toml`'s `NVM_DIR` model
- No committed file references a `wip/` path

**Dependencies:** Issue 6
