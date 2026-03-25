<!-- decision:start id="auto-install-mode-resolution" status="assumed" -->
### Decision: Auto-Install Mode Resolution Order and Non-TTY Fallback

**Context**

The `tsuku run <command>` feature resolves an installation consent mode from multiple sources: a CLI flag, an environment variable, a config file key, and a compiled-in default. The mode determines whether tsuku silently installs a missing tool (`auto`), prompts interactively (`confirm`), or just prints install instructions (`suggest`). The default is `confirm` per the security spec requirement.

The critical security scenario: a user with `auto_install_mode = confirm` in their global config runs `tsuku run jq` inside a CI pipeline where stdin is not a TTY. Without explicit handling, the tool either hangs waiting for keyboard input (blocking the build indefinitely) or takes some fallback action. The fallback action — error-out, silent-suggest, or silent-auto — has security implications because two of the three options either hide the problem or install software without human consent.

Two codebase precedents shape this decision. First, `TSUKU_TELEMETRY` and `TSUKU_LLM_IDLE_TIMEOUT` demonstrate that env vars already override config for behavioral preferences in tsuku — not just transient runtime tuning. Second, `internal/userconfig.LLMIdleTimeout()` shows the canonical pattern: check env var first, fall through to config, then return the default. The same structure applies cleanly to mode resolution.

**Assumptions**

- `TSUKU_AUTO_INSTALL_MODE` follows the existing `TSUKU_` prefix naming pattern. If the team prefers an alternate name, the design holds with any name.
- The design requirement "Non-TTY safety: CI and scripts running with confirm mode must fail fast with a clear error" is binding. If this requirement were relaxed to allow silent degradation, Option B (silent fallback to suggest) would become viable.
- CI pipeline configurations are sometimes generated or templated in ways that make per-command flag injection impractical. This use case justifies the env var layer.

**Chosen: Compound 1 — Flag > Env Var > Config > Default, with Error-Out on Non-TTY Confirm**

Resolution order (highest to lowest priority):
1. `--mode=<value>` cobra flag passed to `tsuku run`
2. `TSUKU_AUTO_INSTALL_MODE` environment variable (values: `suggest`, `confirm`, `auto`)
3. `auto_install_mode` config key in `$TSUKU_HOME/config.toml`
4. Default: `confirm`

Non-TTY behavior: when the resolved mode is `confirm` and `term.IsTerminal(int(os.Stdin.Fd()))` returns false, tsuku prints a clear error to stderr and exits with a distinct exit code (`ExitNotInteractive`):

```
tsuku: confirm mode requires a TTY; set TSUKU_AUTO_INSTALL_MODE=auto or use --mode=auto for non-interactive use
```

No installation is attempted. The error message names both escape hatches (env var and flag) so operators know immediately what to change.

**Rationale**

The env var layer is justified on two grounds: codebase precedent (`TSUKU_TELEMETRY` already follows this pattern for a stable behavioral preference) and the CI escape-hatch use case (generated or templated CI configs can inject environment variables without modifying per-command flags). Error-out is the only non-TTY behavior that satisfies all three stated constraints — safety default (auto requires deliberate opt-in), non-TTY safety (fail fast with a clear error), and exit code fidelity (distinct `ExitNotInteractive` code). Silent fallback to suggest hides the mode mismatch and violates the explicit "fail fast" requirement. Silent fallback to auto directly violates the security constraint that auto is never an implicit behavior.

**Alternatives Considered**

- **Compound 2 (R2+A) — Flag > Config > Default, error-out:** Rejected because the absence of an env var layer creates friction for generated/templated CI configs. The "simplicity" benefit is outweighed by the inconsistency with existing codebase env var patterns (`TSUKU_TELEMETRY`, `TSUKU_LLM_IDLE_TIMEOUT`). Validators conceded this point during cross-examination.

- **Compound 3 (R1+B) — Flag > Env Var > Config > Default, silent fallback to suggest:** Rejected because it violates the explicit design requirement "fail fast with a clear error." Silent behavior changes make debugging harder — the operator sees suggest output and doesn't know their confirm configuration was silently ignored. Exit code 1 from suggest and exit code 1 from "confirm degraded to suggest" are indistinguishable.

- **Compound 4 (R1+C) — Flag > Env Var > Config > Default, silent fallback to auto:** Rejected because it directly violates the core security constraint. Silent escalation from confirm to auto installs software without human consent, defeats the purpose of confirm mode, and creates a supply chain attack vector. All validators converged on rejection immediately.

- **R3 (Config only, no flag or env var):** Rejected because it removes per-invocation control and makes `tsuku run` unusable in environments where config file modification is impractical (ephemeral containers, read-only home directories). The flag is load-bearing.

**Consequences**

- `internal/userconfig.Config` gains an `AutoInstallMode` string field with TOML key `auto_install_mode`
- `userconfig.AvailableKeys()` gains `"auto_install_mode"` with description `"Default install consent mode for tsuku run (suggest/confirm/auto)"`
- `cmd/tsuku/exitcodes.go` gains `ExitNotInteractive = 3` (or next available code)
- `cmd/tsuku/cmd_run.go` implements `resolveMode(flagMode string, cfg *userconfig.Config) (AutoInstallMode, error)` following the four-step priority chain
- `cmd/tsuku/cmd_run.go` calls `isInteractive()` (injectable var, same pattern as `stdinIsTerminal` in `config.go`) after resolution when mode is `confirm`
- The non-TTY error message names `TSUKU_AUTO_INSTALL_MODE=auto` and `--mode=auto` explicitly
- `tsuku run --help` must document all three modes and the env var

What becomes easier: CI operators can inject `TSUKU_AUTO_INSTALL_MODE=auto` as a pipeline-level env var to enable auto-install across all `tsuku run` invocations. What becomes harder: users who configure `confirm` globally and run in non-TTY contexts must explicitly opt in to auto — this is intentional.
<!-- decision:end -->
