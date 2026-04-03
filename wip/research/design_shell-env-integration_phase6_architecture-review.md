# Architecture Review: Shell Env Integration Design

Date: 2026-04-02
Phase: 6 — Architecture Review

## Summary

The proposed architecture is implementable as specified. The key findings below note one confirmed gap and a few minor points to clarify.

---

## Q1: Is the architecture clear enough to implement?

Yes. The component model is straightforward:
- `$TSUKU_HOME/env` managed by tsuku, `env.local` user-owned
- `EnsureEnvFile()` checks equality against `envFileContent`, migrates `TSUKU_NO_TELEMETRY` to `env.local` if found, then rewrites
- `doctor --fix` calls `EnsureEnvFile()` and `RebuildShellCache()` then re-runs checks

No ambiguity in the component model.

---

## Q2: Missing components, edge cases, error paths

**Migration logic — env.local creation timing:**
The migration step (extract `TSUKU_NO_TELEMETRY` to `env.local`) must create `env.local` if it doesn't exist and append if it does. The design doesn't specify how to handle an existing `env.local` that already exports `TSUKU_NO_TELEMETRY` — deduplication would prevent writing it twice.

**env.local read-back in env:**
The design says `env` will source `env.local`. If the user had previously set `TSUKU_NO_TELEMETRY=1` in the old `env` file, migration must write it to `env.local` before overwriting `env`, or the variable will be lost in the same shell session (it would only take effect on the next shell launch after sourcing the new `env`).

**install.sh telemetry opt-out path:**
The installer writes to `env.local` before `env` is created. This is safe if `env` sources `env.local` at the end, but the relative write-order during first install needs to be confirmed (install.sh writes `env.local`, then `tsuku install` creates `env` via `EnsureEnvFile()`).

---

## Q3: Phase sequencing

Phases should proceed in this order:
1. Update `envFileContent` constant with shell detection + `env.local` sourcing
2. Update `EnsureEnvFile()` with migration logic (reads before writing)
3. Update `website/install.sh` to write telemetry opt-out to `env.local`
4. Add `doctor` env staleness check and `--fix` flag

No dependency issues. Phase 3 can proceed independently of 2 since `env.local` doesn't need `env` to exist first.

---

## Q4: Simpler alternatives

The migration logic (extract `TSUKU_NO_TELEMETRY`) adds meaningful complexity. A simpler alternative: treat `TSUKU_NO_TELEMETRY` in `env` as a breaking change and just overwrite `env` without migration, relying on `doctor` to inform users. However, silently dropping a user's telemetry opt-out would be hostile UX. The proposed migration is the right call.

---

## Q5: EnsureEnvFile coverage gap (confirmed)

**Confirmed gap: `runPlanBasedInstall` does not call `EnsureEnvFile()`.**

In `cmd/tsuku/plan_install.go`, the plan-based install path:
1. Creates `config.DefaultConfig()` and `install.New(cfg)`
2. Calls `mgr.InstallWithOptions(...)` — this **does** call `EnsureEnvFile()` via `manager.go:70`
3. Then calls `exec.ExecutePhase(globalCtx, plan, "post-install")` for `install_shell_init`

So plan-based installs go through `InstallWithOptions`, which calls `EnsureEnvFile()`. This path is covered.

**However**, there are two scenarios where `env` could be stale after the design is implemented:

1. **Existing users upgrading tsuku**: They have an old `env` file. The next `tsuku install` will call `EnsureEnvFile()`, which will rewrite `env` (string comparison fails since content changed). Migration runs. This is correct behavior.

2. **Users who never install a tool**: They only run `tsuku doctor`. The `doctor --fix` path handles this explicitly.

**Dependency install path**: `InstallWithOptions` is always the terminal write operation for both direct and plan-based installs. There's no alternative code path that finalizes an installation without going through the manager. Dependency installs (hidden tools) also call `InstallWithOptions` with `IsHidden: true`. Covered.

---

## Q6: RebuildCache export status

`RebuildShellCache` (in `internal/shellenv/cache.go`, line 29) is already exported with the exact signature `RebuildShellCache(tsukuHome string, shell string, contentHashes ...map[string]string) error`. The design references `shellenv.RebuildCache` — this is likely a naming shorthand in the design doc; the actual function to call is `shellenv.RebuildShellCache`. No new export needed.

---

## Recommendations

1. **Clarify `env.local` deduplication**: The migration step should check whether `TSUKU_NO_TELEMETRY` is already present in `env.local` before appending, to avoid duplicates on repeated `EnsureEnvFile()` calls.

2. **Correct function name in design**: The design references `shellenv.RebuildCache`; the actual exported function is `shellenv.RebuildShellCache`.

3. **`doctor --fix` flag location**: `doctor.go` has no existing flags — `--fix` and `--rebuild-cache` are referenced in check 5's error output (`run: tsuku doctor --rebuild-cache`) but neither flag is implemented yet. The implementation needs to add both, or unify them into a single `--fix` flag.

4. **doctor output references `--rebuild-cache` today** (line 163): the proposed `--fix` flag should subsume this, or the existing message will be misleading after the change.
