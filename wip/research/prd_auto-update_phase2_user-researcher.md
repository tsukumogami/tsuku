# Phase 2 Research: User Researcher

## Lead 1: User stories and acceptance criteria

### Findings

Three distinct personas emerge from how tsuku is used today, each with different relationships to tool versioning.

#### Persona 1: Developer using tsuku daily

This user has 10-30 tools installed, works across multiple projects, and uses tsuku interactively from their terminal. They installed most tools with `tsuku install <tool>` (no version suffix) and occasionally with `tsuku install node@20` when a project requires a specific major line.

**User Stories:**

1. **As a daily developer, I want my tools to receive patch and minor updates automatically so that I get bug fixes and security patches without running `tsuku update` manually.**
   - AC1: Tools installed without a version constraint (`tsuku install ripgrep`) auto-update to the latest stable version within the configured check interval.
   - AC2: Tools installed with a major pin (`tsuku install node@20`) auto-update within 20.x.y only.
   - AC3: Tools installed with a minor pin (`tsuku install kubectl@1.29`) auto-update within 1.29.z only.
   - AC4: Tools installed with an exact version (`tsuku install terraform@1.6.3`) never auto-update.
   - AC5: The pin level is derived from what the user typed at install time (the `Requested` field already stores this).

2. **As a daily developer, I want to know when a new major version is available outside my pin so that I can decide whether and when to adopt it.**
   - AC1: When a tool is pinned to major 20 but major 22 exists, a periodic stderr hint says something like: "node 22.x is available (you're on 20.x). Run 'tsuku install node@22' to switch."
   - AC2: This notification appears at most once per week (separate cadence from in-channel update checks).
   - AC3: Out-of-channel notifications can be disabled via `tsuku config set updates.notify_out_of_channel false` or `TSUKU_NO_UPDATE_NOTIFIER=1`.

3. **As a daily developer, I want auto-updates to never break my workflow so that I can trust the system to do the right thing.**
   - AC1: If an auto-update fails (network error, checksum mismatch, disk issue), the previously working version remains active with no user-visible disruption.
   - AC2: Failed updates produce a deferred notice shown on the next tsuku command invocation (stderr, once).
   - AC3: Auto-update never runs while I'm actively using a tool -- it happens on tsuku command invocation, not during tool execution.
   - AC4: No interactive prompts during auto-update. All updates are silent except for the post-completion notification.

4. **As a daily developer, I want to control how often tsuku checks for updates so that it doesn't slow down my commands or use bandwidth I don't have.**
   - AC1: Default check interval is 24 hours (consistent with gh, Homebrew).
   - AC2: Configurable via `tsuku config set updates.check_interval 7d` or `TSUKU_UPDATE_CHECK_INTERVAL=7d`.
   - AC3: Valid range is 1 hour to 30 days.
   - AC4: Checks are non-blocking -- they run as a background goroutine and never add latency to the primary command.

5. **As a daily developer, I want to undo an auto-update that went wrong so that I can get back to a known-good state quickly.**
   - AC1: `tsuku install <tool>@<previous-version>` restores the previous version (the old version directory is still on disk).
   - AC2: If the old version was cleaned up, tsuku re-downloads it.
   - AC3: A `tsuku rollback <tool>` shorthand switches to the previously active version without re-downloading (if the version directory still exists).

#### Persona 2: CI pipeline

This user is a build script or CI configuration (GitHub Actions, GitLab CI, etc.) that uses tsuku to install tools deterministically. It runs non-interactively, produces machine-parseable output, and must never have its behavior change without an explicit config change.

**User Stories:**

6. **As a CI pipeline, I want auto-update to be disabled by default in non-interactive environments so that my builds are reproducible.**
   - AC1: When `CI=true` is set (standard across GitHub Actions, GitLab CI, CircleCI, etc.), auto-update checks are suppressed.
   - AC2: When stdout is not a TTY, update notifications are suppressed (matching existing `term.IsTerminal()` pattern).
   - AC3: Explicit opt-in via `TSUKU_AUTO_UPDATE=true` overrides CI detection if someone genuinely wants auto-updates in CI.
   - AC4: `tsuku outdated --json` still works in CI for scripted update checks -- the suppression only applies to passive auto-update, not explicit commands.

7. **As a CI pipeline, I want exact version pins in .tsuku.toml to be respected absolutely so that auto-update never overrides project-level configuration.**
   - AC1: If `.tsuku.toml` specifies `kubectl = "1.29.3"` (exact version), auto-update never touches kubectl in that project context, even if the global policy is "auto-update within minor."
   - AC2: If `.tsuku.toml` specifies `node = "20"` (major pin), auto-update within 20.x.y is allowed but 21+ is not.
   - AC3: `tsuku install` (project mode) resolves versions from `.tsuku.toml` constraints, not from auto-update state.

8. **As a CI pipeline, I want zero unexpected output so that my build logs are clean and parseable.**
   - AC1: In quiet mode (`--quiet` or `TSUKU_QUIET=true`), no update notifications appear on stderr.
   - AC2: In JSON mode (`--json`), no update notifications appear anywhere -- only structured output on stdout.
   - AC3: Auto-update state file writes don't produce any output.

#### Persona 3: Team lead managing shared tooling

This user maintains `.tsuku.toml` files across team repositories. They want consistent tooling versions across the team while still benefiting from automatic patches.

**User Stories:**

9. **As a team lead, I want .tsuku.toml to express both "install this version" and "auto-update within this range" so that my team gets patches without me updating the config for every point release.**
   - AC1: `node = "20"` in `.tsuku.toml` means "install latest 20.x, auto-update within 20.x."
   - AC2: `node = "20.16.0"` means "install exactly 20.16.0, never auto-update."
   - AC3: The semantics match what `tsuku install node@20` already does -- the number of version components determines the pin level.
   - AC4: Team members running `tsuku install` (project mode) get the latest version within the constraint, not necessarily the exact version the team lead had when they wrote the config.

10. **As a team lead, I want to understand what auto-update will do before enabling it for my team so that I can communicate changes clearly.**
    - AC1: `tsuku outdated` shows two columns: "Available (in pin)" and "Available (latest)" so the team lead can see both what auto-update would do and what's available if they widen the pin.
    - AC2: `tsuku update --dry-run --all` (or equivalent) previews all pending auto-updates without applying them.
    - AC3: Documentation clearly explains the interaction between `.tsuku.toml` pins and auto-update behavior.

11. **As a team lead, I want auto-update behavior to be configurable at the project level so that different projects can have different policies.**
    - AC1: `.tsuku.toml` can include an `[updates]` section: `enabled = true`, `check_interval = "7d"`.
    - AC2: Project-level config overrides user-level config when the user is in the project directory.
    - AC3: Absence of `[updates]` in `.tsuku.toml` means "defer to user config" (not "disable").

### Implications for Requirements

1. **The pin-level-from-version-components model is the right default.** It requires no new syntax, matches user intuition, and the `Requested` field already stores the raw input. The PRD should mandate this as the pinning semantic.

2. **CI detection must be multi-layered.** The `CI` env var handles most cases, but `TSUKU_NO_UPDATE_NOTIFIER` provides explicit control. Both should exist. The `term.IsTerminal()` check already used in the codebase is a third layer.

3. **Project-level config (.tsuku.toml) needs an `[updates]` section.** The current `ToolRequirement` struct only has a `Version` field. Adding per-tool update policy (or even just a global project-level toggle) requires extending the config schema.

4. **`tsuku outdated` needs a rework.** It currently only checks GitHub-based tools and doesn't respect pin constraints. Both gaps must be fixed for auto-update to make sense. The "Available (in pin)" vs "Available (latest)" dual-column display is important for team leads.

5. **Rollback UX needs to be simple.** The multi-version directory model already supports this. A `tsuku rollback <tool>` command (or flag on install) that switches `ActiveVersion` back to the previously active version is the minimum viable UX.

### Open Questions

1. **Should the default auto-update policy be "enabled" or "disabled"?** Homebrew auto-updates by default. Most other tools don't. Given tsuku's "just works" philosophy, a middle ground might be: auto-update is enabled by default but only for patch-level updates (tools pinned to major/minor get patches, but tools on "latest" still update freely). This is more conservative than Homebrew but more helpful than "off by default."

2. **How should auto-update interact with `tsuku run`?** If a user runs `tsuku run serve` and serve has an update available, should tsuku update before running? Or only check and notify? The `auto_install_mode` config already controls consent for `tsuku run` installs -- should a similar model apply to updates?

3. **Should there be a per-tool update policy override?** For example, a team lead might want `node` to auto-update within major but `terraform` to be exactly pinned. This could be expressed in `.tsuku.toml` as `terraform = { version = "1.6.3", update = "none" }` vs `node = { version = "20", update = "minor" }`. Is this in scope for the initial design?

4. **What happens when a user manually downgrades after an auto-update?** If auto-update moved node from 20.16.0 to 20.17.0, and the user runs `tsuku install node@20.16.0`, should the system remember this as "user explicitly wants 20.16.0" and stop auto-updating? Or should it treat this as a temporary pin that auto-update will override on the next cycle?

## Lead 4: Edge cases and failure scenarios

### Findings

I examined the codebase's existing error handling patterns (exit codes, stderr output, atomic operations) and the failure/rollback research to define what users should experience in each scenario.

#### Scenario 1: Offline during auto-update check

**What the user does:** Runs `tsuku install foo` while disconnected from the internet. The 24-hour update check interval has elapsed.

**What should happen:**
- The background update check goroutine starts, attempts a network call, and fails.
- The primary command (`install foo`) proceeds normally using cached data (recipe cache, version cache have stale-if-error fallback with 7-day max staleness).
- No error message is shown. The failed check is not recorded as a "failure" -- it simply doesn't update the cache timestamp, so the check will be retried on the next invocation when network is available.
- If the user explicitly runs `tsuku outdated`, the command reports the network error for each tool it can't reach and shows whatever cached information is available: "Could not check ripgrep: network error (last checked: 2 days ago)."

**Rationale:** The stale-if-error pattern already exists in the recipe cache (`internal/registry/cached_registry.go`). Update checks should follow the same model. Offline should be invisible during normal usage.

#### Scenario 2: Update fails mid-download

**What the user does:** Nothing -- auto-update triggers in the background during a normal tsuku command. The download of a new tool version starts but the connection drops halfway.

**What should happen:**
- The download writes to a temp file in the work directory. The incomplete file is cleaned up by `Executor.Cleanup()` (existing behavior).
- No state changes occur. `state.json` is untouched. Symlinks are untouched. The old version remains active.
- A deferred notice is written to `$TSUKU_HOME/notices/update-failed-<tool>.json` containing: tool name, attempted version, error message, timestamp.
- On the user's next tsuku command, a single stderr line appears: "Auto-update of ripgrep to 14.2.0 failed: download interrupted. Run 'tsuku update ripgrep' to retry."
- The notice is displayed once and then marked as shown (not deleted -- kept for `tsuku notices` history until explicitly cleared or aged out after 30 days).

**Rationale:** The staging-then-rename installation model means partial downloads never reach the tool directory. The deferred notice pattern follows the exploration research recommendation of a file-based notice queue.

#### Scenario 3: New version is broken at runtime

**What the user does:** An auto-update installs kubectl 1.30.1 successfully (download, checksum, extraction all pass). But when the user runs `kubectl get pods`, it crashes or produces wrong output because of a regression in the upstream release.

**What should happen:**
- Tsuku cannot detect this automatically. Checksum verification confirms the binary matches what upstream published, not that it works correctly.
- The user needs a fast path to revert: `tsuku rollback kubectl` switches `ActiveVersion` back to 1.30.0 (the previous version directory is still on disk).
- If the user doesn't know about `rollback`, running `tsuku install kubectl@1.30.0` also works.
- The old version directory is preserved for at least one auto-update cycle (configurable, default 7 days) before cleanup.
- `tsuku rollback kubectl` outputs: "Rolled back kubectl from 1.30.1 to 1.30.0." On stderr if verbose: "Previous version directory preserved at $TSUKU_HOME/tools/kubectl-1.30.0/."

**Future enhancement:** Recipes could define a `verify.command` (e.g., `kubectl version --client`) that auto-update runs after installation. If it exits non-zero, the update is rolled back automatically. This exists in a limited form today via the recipe `verify` section but isn't used during updates.

**Rationale:** This is the hardest failure mode because tsuku can't distinguish "broken" from "working differently." The mitigation is keeping rollback cheap and fast. The exploration research flagged this as the biggest open risk.

#### Scenario 4: Two terminals running tsuku simultaneously

**What the user does:** In terminal A, runs `tsuku update kubectl`. In terminal B, runs `tsuku install ripgrep`. Both trigger auto-update checks.

**What should happen:**
- **State file safety:** `state.json` uses `FileLock` (flock-based) for concurrent writes. Terminal A's kubectl update and terminal B's ripgrep install hold exclusive locks for their respective state writes. This is already safe.
- **Update check cache:** Both processes may read the stale cache simultaneously and both trigger network checks. This is harmless -- redundant network calls, last-writer-wins on the cache file (atomic writes). No locking needed for the cache (matches existing version cache behavior).
- **Same tool update race:** If both terminals somehow update the same tool, the staging-then-rename pattern means one succeeds and the other either: (a) finds the version already installed and skips, or (b) races on the rename. The rename is atomic, so one wins and the other fails. The failure should produce an informative error: "kubectl 1.30.1 was installed by another process."
- **User experience:** No corruption. Worst case is a redundant network call or a non-fatal error message about a concurrent install.

**Rationale:** The existing file lock on `state.json` handles the critical path. The exploration research identified a gap with per-tool directory races, but the atomic rename makes this a benign race (one wins, one fails cleanly) rather than a corruption risk.

#### Scenario 5: Disk full during update

**What the user does:** Nothing -- auto-update triggers, but the disk is full.

**What should happen:**
- **During download:** The temp file write fails. `Executor.Cleanup()` removes the work directory. No state change. A deferred notice is created (if there's enough space for the small notice file; if not, the failure is silent).
- **During staging copy:** `copyDir()` fails, staging directory is cleaned up (`manager.go` line 93). No state change.
- **During state.json write:** The temp file write fails, so the atomic rename never happens. Old `state.json` remains intact. However, the tool directory may already be in place without a state entry -- this is a known gap (flagged in failure-rollback research).
- **User experience:** The next tsuku command shows a stderr warning: "Auto-update of ripgrep failed: disk full. Free disk space and run 'tsuku update ripgrep' to retry."
- **Recovery:** `tsuku doctor` should detect orphaned tool directories (installed but not in state.json) and orphaned staging directories, and offer to clean them up.

**Rationale:** Disk-full is a system-level issue tsuku can't fix. The goal is: don't make things worse, don't corrupt state, and give the user a clear message about what happened and what to do.

#### Scenario 6: User downgrades manually after auto-update

**What the user does:** Auto-update moved node from 20.16.0 to 20.17.0. The user runs `tsuku install node@20.16.0` to go back.

**What should happen:**
- The explicit install records `Requested: "20.16.0"` (exact pin, three version components).
- Since this is an exact pin, auto-update no longer applies to this tool. The user has explicitly said "I want 20.16.0."
- If the 20.16.0 directory is still on disk, the install is instant -- just switch symlinks via `Activate()`.
- If the 20.16.0 directory was cleaned up, tsuku re-downloads and installs it.
- Output: "Switched node to 20.16.0. This version is pinned and won't auto-update."
- To resume auto-updating, the user would run `tsuku install node@20` (setting the pin back to major-level).

**Alternative: `tsuku rollback node`**
- Rollback doesn't change the `Requested` field. It just switches `ActiveVersion` back.
- The `Requested` field stays at whatever it was before (e.g., `"20"`), so auto-update will try again on the next cycle.
- This is intentional -- rollback is a temporary measure ("this version is broken, go back for now"), while `install @version` is an explicit preference change.
- Output: "Rolled back node from 20.17.0 to 20.16.0. Note: auto-update may re-apply this update. To stay on 20.16.0, run 'tsuku install node@20.16.0'."

**Rationale:** The distinction between rollback (temporary, auto-update will retry) and explicit install (permanent pin change) is important. Rollback is for "this specific release is broken," while install is for "I want to stay on this version." The user needs clear messaging about this difference.

#### Scenario 7: Auto-update of tsuku itself fails

**What the user does:** Nothing -- tsuku checks for a self-update, downloads the new binary, but verification fails (checksum mismatch).

**What should happen:**
- The new binary (in a temp file) is deleted. The current binary is untouched.
- A deferred notice is written: "Self-update to tsuku 0.8.0 failed: checksum mismatch. Run 'tsuku self-update' to retry, or download manually from https://get.tsuku.dev/now."
- The notice appears on the next tsuku invocation (stderr, once).
- If the new binary was already renamed into place (past the point of no return) but the new binary fails to run, the `.tsuku.old` backup enables manual recovery: `mv $TSUKU_HOME/bin/tsuku.old $TSUKU_HOME/bin/tsuku`.
- `tsuku doctor` checks for `.tsuku.old` files and reports: "Found backup binary from failed self-update. Run 'tsuku self-update' to retry or remove $TSUKU_HOME/bin/tsuku.old."

**Rationale:** Self-update failure is the most dangerous scenario because it can leave the user unable to use tsuku at all. The rename-aside pattern (keeping `.tsuku.old`) provides a manual recovery path, and the deferred notice tells the user what happened.

#### Scenario 8: Network timeout during version resolution (not download)

**What the user does:** Runs any tsuku command. The background update check starts but the GitHub API (or other version provider) is slow or timing out.

**What should happen:**
- The background goroutine has a context with a timeout (e.g., 10 seconds for the entire check).
- If the timeout fires, the check is abandoned silently. No notice is written -- transient timeouts don't warrant user attention.
- The cache timestamp is NOT updated, so the check will be retried on the next invocation.
- The primary command is unaffected -- the goroutine was non-blocking.
- Only persistent failures (3+ consecutive check failures) produce a deferred notice.

**Rationale:** Version resolution timeouts are common (rate limiting, slow APIs, brief outages). Nagging the user about every transient failure would erode trust in the notification system. Only persistent failures deserve attention.

#### Scenario 9: Recipe changes between versions (recipe incompatibility)

**What the user does:** Nothing -- auto-update resolves a new version, but the recipe for that version uses actions the installed tsuku doesn't support (e.g., a new action type added in tsuku 0.9.0, but the user has tsuku 0.7.0).

**What should happen:**
- The executor fails on the unknown action during plan generation (before any side effects).
- The old version remains active. A deferred notice appears: "Auto-update of ripgrep to 14.2.0 requires tsuku 0.9.0 or later. Run 'tsuku self-update' first, then 'tsuku update ripgrep'."
- This combines the recipe's `min_cli_version` metadata (if present) or falls back to the generic "unknown action" error.
- The tool's auto-update is not retried until either tsuku is updated or the check interval elapses and a new check finds a compatible version.

**Rationale:** This is a natural consequence of a recipe registry that evolves faster than installed binaries. The messaging should guide the user toward self-update as the fix, not retry the same failing update.

### Implications for Requirements

1. **Deferred notice system is a hard requirement.** At least five of the nine scenarios need it. The file-based notice queue at `$TSUKU_HOME/notices/` is the right pattern -- it's already recommended by the failure-rollback research.

2. **Rollback vs. explicit install must be clearly distinguished.** Rollback is temporary (auto-update will retry), explicit install changes the pin. The messaging must make this clear to users, or they'll be confused when auto-update "undoes" their rollback.

3. **Background checks must be non-blocking with timeout.** The 50ms drain window pattern from the update-check-caching research is correct. Add a 10-second absolute timeout on the check goroutine to prevent resource leaks on slow networks.

4. **`tsuku doctor` should grow auto-update diagnostics.** Orphaned staging directories, `.tsuku.old` backup binaries, stale notice files, and state-directory inconsistencies all need detection.

5. **Consecutive-failure suppression prevents notification fatigue.** Transient network errors (single failures) should be silent. Only persistent failures (3+ consecutive) or actionable errors (disk full, checksum mismatch, recipe incompatibility) should produce user-visible notices.

6. **Old version retention policy is needed.** Auto-update should keep the previous version on disk for at least one update cycle (default 7 days). A garbage collection mechanism removes older versions. Without this, rollback requires a re-download, which is slow and defeats the purpose.

### Open Questions

1. **What's the retention policy for old version directories?** Keep the previous version forever? Keep for N days? Keep until the next successful update? Disk usage is the tradeoff. A reasonable default might be: keep the immediately previous version indefinitely, garbage-collect versions two or more updates old after 30 days.

2. **Should auto-update have a "staging" mode where it downloads but doesn't activate?** The user would see "Updates downloaded and ready. Run 'tsuku update --apply' to activate." This adds safety but also friction. macOS App Store uses this pattern. It would be unusual for a CLI tool.

3. **How should concurrent auto-updates of the same tool be handled?** The atomic rename makes this a benign race, but should tsuku explicitly coordinate with a per-tool lock file? The exploration research recommends it but it adds complexity. The simpler approach (let the race happen, one wins) seems sufficient given the atomicity guarantees.

4. **Should deferred notices expire?** If the user doesn't run tsuku for 30 days and then comes back, should they see a month-old "update failed" notice? Probably yes, with the timestamp clearly shown so they know it's stale. But notice files should be pruned after some period (90 days?) to prevent unbounded growth.

5. **What exit code should a failed auto-update produce?** Auto-update failures shouldn't change the exit code of the primary command (which succeeded). The deferred notice system handles communication. But `tsuku update` (explicit) should use `ExitInstallFailed` on failure and `ExitNetwork` for network errors, following the existing exit code taxonomy.

## Summary

Three user personas -- daily developer, CI pipeline, and team lead -- have fundamentally different relationships with auto-update, requiring a layered system: daily developers want silent automatic patches with easy rollback, CI demands deterministic suppression (via `CI=true` detection and exact pins), and team leads need `.tsuku.toml` to express both minimum versions and update boundaries through the existing version-component pinning model. The most critical UX requirements across all failure scenarios are a file-based deferred notice system for reporting async failures, clear distinction between rollback (temporary, auto-update retries) and explicit version install (permanent pin change), and a non-blocking background check with consecutive-failure suppression to avoid notification fatigue from transient network errors.
