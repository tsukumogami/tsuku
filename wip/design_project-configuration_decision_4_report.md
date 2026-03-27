<!-- decision:start id="init-and-install-no-args" status="assumed" -->
### Decision: tsuku init and tsuku install (no args) CLI Behavior

**Context**

Tsuku needs two new CLI behaviors to support project configuration: `tsuku init` to create a config file, and `tsuku install` (no arguments) to batch-install all tools declared in that config. Today, `tsuku install` requires at least one tool name argument and exits with a usage error when called with none. There is no `tsuku init` command -- the existing `tsuku create` generates recipes from package ecosystems and serves a completely different purpose.

The commands serve different audiences at different times. `tsuku init` runs once per project by the person setting up tooling. `tsuku install` (no args) runs frequently by every contributor cloning the project. The design must handle both gracefully while staying consistent with tsuku's existing conventions: cobra-based CLI, TOML config, kebab-case tool names, and the `runInstallWithTelemetry` flow that already handles multi-tool installation.

A key implementation constraint is that the current multi-arg install loop calls `handleInstallError`, which terminates the process on first failure via `exitWithCode`. Batch project installs need a different error aggregation strategy.

**Assumptions**
- Sibling decisions will settle on a single config file name (likely `.tsuku.toml`) with a `[tools]` section using string values for the common case. If the schema changes, init's template and install's parsing adapt accordingly.
- Most users will run `tsuku init` once and `tsuku install` (no args) repeatedly. Init's polish matters less than install's reliability.
- Inter-tool dependencies declared within individual recipe files (via `installWithDependencies`) are sufficient for correctness. Cross-tool topological ordering at the project level is not needed for the initial implementation.

**Chosen: Minimal Non-Interactive Init + Lenient Batch Install**

`tsuku init` creates a minimal config file non-interactively:
- Writes the project config file (per Decision 1's file name) with an empty `[tools]` section and a brief comment explaining the format
- Errors if the file already exists; `--force` overwrites
- Does not prompt, does not add tools, does not run install
- Accepts but ignores `--yes`/`-y` for scripting compatibility

`tsuku install` (no arguments) reads the project config and installs all declared tools:
- Discovers the config file using the search strategy from Decision 1
- No config found: prints an error message suggesting `tsuku init`, exits with `ExitUsage` (2)
- Config found, no tools declared: prints "No tools declared in <path>", exits with `ExitSuccess` (0)
- Config found with tools: iterates tools, calling `runInstallWithTelemetry` for each, collecting errors instead of exiting on first failure
- Prints a summary at the end: installed count, already-current count, failed count with tool names and error messages
- Exit codes: 0 if all succeeded, new `ExitPartialFailure` (5) if some failed, `ExitInstallFailed` (4) if all failed
- Invalid config syntax (TOML parse error, unknown fields) is a hard failure -- exits immediately before attempting any installs

Flag compatibility with no-arg mode:
- `--dry-run`: supported (shows what would be installed)
- `--force`: supported (reinstalls all tools regardless of current state)
- `--fresh`: supported (clean reinstall)
- `--plan`, `--recipe`, `--from`, `--sandbox`: incompatible (error if combined with no-arg mode)

**Rationale**

Non-interactive init aligns with the Go/Cargo/devbox pattern that developer tools have converged on. Interactive wizards add complexity for marginal benefit -- the config file format is simple enough that editing it directly is faster than answering prompts. Every modern tool in this space has moved away from wizardry (npm's `--yes` flag being the canonical example of users opting out).

Lenient batch install is the right default because project configs often declare 5-15 tools, and a single transient failure (network timeout, rate limit, missing platform support for one tool) shouldn't block the other installations. mise validates this approach -- it continues past failures and reports a summary. asdf's stop-on-first-error approach frustrates users and generates repeated "why did it stop" questions.

The new `ExitPartialFailure` exit code gives CI scripts a way to distinguish "everything worked" from "some things failed" from "nothing worked." This is more useful than a binary success/failure for project setup scripts that may want to proceed with partial tooling.

Keeping init and install as separate concerns (init creates, install reads) avoids the confusing dual-purpose behavior where `tsuku install` might create a config file. A new user typing `tsuku install` in a project without config gets a clear directive ("no project config found -- run tsuku init to create one") rather than a surprising side effect.

**Alternatives Considered**

- **Interactive Init + Strict Install**: Init runs a wizard asking for tool names; install stops on first failure. Rejected because interactive wizards add friction without proportional value for a simple TOML file, and strict-failure is hostile to batch operations where partial success is the norm. The wizard pattern has fallen out of favor in modern developer tooling.

- **Smart Init with Detection + Topological Install**: Init auto-detects tools from project files (Makefile, go.mod, etc.); install resolves cross-tool dependencies and parallelizes. Rejected because detection heuristics are fragile (false positives from scanning Makefiles), topological ordering adds implementation complexity that isn't needed when individual tools already handle their own dependencies via `installWithDependencies`, and parallelism introduces output interleaving that complicates user experience. All three features can be added incrementally later without breaking the base behavior.

- **No Init Command (Install Creates Config)**: No `tsuku init` exists; `tsuku install` (no args) creates a config if none found, or `tsuku install <tool>` adds to config. Rejected because it violates the principle of least surprise -- an install command shouldn't create config files as a side effect. It also muddles the mental model of what `tsuku install node` means (does it install the tool, or add it to config, or both?). Discoverability suffers when there's no explicit init step in the getting-started flow.

**Consequences**

This design adds two new code paths: a new `initCmd` cobra command and a project-config branch in the existing `installCmd`'s Run function. The install command's `Args` validator stays as `cobra.ArbitraryArgs` (already set for `--plan`/`--recipe` support). The error handling refactor -- collecting errors instead of calling `exitWithCode` mid-loop -- is the most significant change to existing code, but it's localized to the install command's Run function.

The `ExitPartialFailure` exit code is a new convention that downstream tooling (CI scripts, shell integration) can rely on. It sets a precedent for other batch commands like `tsuku update` (no args) if that's added later.

Users get a two-step project setup workflow: `tsuku init` then edit the config, then `tsuku install`. Contributors cloning an existing project run `tsuku install` and get a clear summary of what installed and what didn't. This matches the mental model established by npm, cargo, and devbox.
<!-- decision:end -->
