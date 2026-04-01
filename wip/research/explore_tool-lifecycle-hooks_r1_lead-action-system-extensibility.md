# Lead: How close is tsuku's current action system to supporting lifecycle hooks?

## Findings

### ExecutionContext & Step Execution
The executor has a clean separation: ExecutionContext carries full metadata (version, install dir, tools dir, recipe reference, env vars). Step execution is straightforward -- steps are linear, ordered, and conditional via WhenClause (platform, package manager checks). The `shouldExecute()` method evaluates WhenClause conditions. Currently, there is no concept of "execution phase" -- all steps execute in order during a single pass through the recipe Steps array.

The executor does implement a weak form of sequencing already: after `install_binaries` completes, it dynamically adds the bin directory to ExecPaths so subsequent steps (like npm_exec) can find the installed binaries. This shows the system can track state changes between steps, but it's hardcoded for this one action.

### Recipe Types: Step Schema & Qualifiers
The Step struct has: Action, When, Note, Description, Params, and Dependencies (step-level only). The WhenClause supports platform matching (OS, arch, Linux family, libc, GPU) and package_manager filtering. **Critically, there is no "phase" or "lifecycle" field on Step.** The When clause is purely for platform/environment conditions, not for lifecycle phase qualification.

This means adding a phase qualifier would require either:
1. Extending WhenClause with a Phase field (cleanest, leverages existing conditional infrastructure)
2. Adding a Phase field directly to Step (most direct, highest change surface area)
3. Creating separate `[[post_install_steps]]` arrays at the recipe level (most explicit, requires recipe schema redesign)

The set_env action creates an env.sh file in the install directory with `export NAME=VALUE` lines. It's deterministic and produces a simple shell script. The run_command action executes arbitrary shell commands with variable substitution and working directory control. Both are stateless during execution -- they don't know about lifecycle context.

### Remove Flow: No Cleanup Hooks
The remove flow (RemoveVersion, RemoveAllVersions) performs only two operations:
1. Delete tool directory recursively (os.RemoveAll)
2. Remove symlinks from current/
3. Remove state entry

There is **no cleanup phase** or hook invocation. If a tool needs to run cleanup logic (remove shell init entries, unregister completions, clean environment files), it has no way to do so. The recipe has no way to register cleanup steps that would execute during `tsuku remove`.

The RemoveAllVersions/RemoveVersion flow is hardcoded; it doesn't load the recipe or consider any stored cleanup instructions. This makes it unsafe to add tools that require cleanup without manual intervention.

### Update Flow: Install + Swap
The update command loads the recipe, respects the "Requested" field to stay within version constraints (e.g., "18.x.y"), and runs the standard install flow via runInstallWithTelemetry(). It's simply `install new version → activate new version`. There is no pre-update or post-update phase. Cleanup of the old version happens implicitly via the filesystem's tool directory structure (you can have multiple versions installed).

No hooks or customization points exist for update-time logic (e.g., migrate config files, warn about breaking changes, run upgrade scripts).

### Current State: No Lifecycle Infrastructure
Tsuku has no concept of pre/post/cleanup phases at all. The only phase is "install" (which is monolithic). The when clause infrastructure is built and proven, but it's for platform matching, not phase qualification.

## Implications

**Path of least resistance:** Add a `phase` field to WhenClause and extend the Step struct minimally:
- Modify WhenClause to include `Phase []string` (values: "install", "post-install", "pre-remove", "post-remove", "pre-update", "post-update")
- Modify executor to check phase qualifier before executing (new condition in shouldExecute)
- Extend remove.go to reload the recipe and execute "pre-remove"/"post-remove" steps
- Extend update.go to execute "pre-update"/"post-update" steps around the install step
- This leverages existing conditional logic and requires minimal schema changes

**Clean design path:** New recipe primitives:
- Add `[lifecycle]` section with subsections: `[lifecycle.post-install]`, `[lifecycle.pre-remove]`, etc.
- Each subsection is an array of steps (same schema as Steps)
- Executor loads recipe phases and executes them at appropriate times
- This makes the schema explicit and separates concerns (core install vs. lifecycle)
- Higher upfront complexity, but cleaner semantics

**Constraints on what's possible:**
- set_env works well for post-install (creates env.sh file). But this file isn't automatically sourced by shells -- users must manually `source ~/.tsuku/env/tool/env.sh` in their .bashrc/.zshrc. To truly support lifecycle hooks, we'd need a complementary mechanism to auto-load these files.
- run_command can execute arbitrary cleanup (e.g., `rm -f ~/.config/tool/config.toml`), but it requires the recipe author to hardcode file paths or use variables
- No built-in actions for shell integration (registering completions, adding aliases). Could add new actions like `install_shell_completions` or `register_shell_alias` that are lifecycle-aware
- The state manager (state.json) doesn't track which steps have run -- you can't resume or replay lifecycle phases if interrupted

## Surprises

1. **Weak cleanup model:** The only "cleanup" today is removing directories. No in-recipe cleanup hooks exist. The `RemoveAllVersions` flow doesn't even load the recipe -- it just deletes files. This is a significant gap for tools that need to unregister themselves (completions, env variables, config files).

2. **Update is just reinstall:** `tsuku update` is entirely scripted as "install latest version" with no special handling. You can have multiple versions installed at once, which is nice for rollback, but the update command itself has no lifecycle awareness.

3. **set_env produces shell scripts but doesn't load them:** The set_env action creates env.sh files that are stored in the tool directory, but there's no mechanism to load them into the user's shell. This was probably intended as a manual setup step ("source ~/.tsuku/env/tool/env.sh in your .bashrc").

4. **Phase awareness at recipe level is zero:** Tools must complete everything in the main Steps array. There's no way to declare "these steps are cleanup" or "these steps run after other dependencies are installed." The only context is the action name itself (by convention, you'd use run_command for cleanup, but the executor doesn't know this).

## Open Questions

1. **How should lifecycle steps interact with variable expansion?** If a tool is removed, some variables (like {version}) may not be resolvable. Do we store a snapshot of resolved variables in state.json so cleanup can use them?

2. **Should lifecycle phases be recipe-level or tool-level?** If a tool has multiple versions installed, should each version have its own cleanup state, or should cleanup be shared? (Current design: tool directory is per-version, so cleanup logic would likely be per-version too, but recipes don't express this)

3. **Who owns shell integration?** If a tool needs to register completions or source env files, should this be:
   - The recipe (via lifecycle steps)
   - A separate mechanism (like a `.tsuku/hooks/` directory)
   - Manual user setup (current state)

4. **How do we test lifecycle hooks?** Executor.ExecutePlan() is deterministic for the install phase. How would we validate that pre-remove and post-update steps run correctly? State-based testing (checking for side effects) would be needed.

5. **Backward compatibility:** If we add phase-qualified steps, should old recipes (no phase qualifier) implicitly run in the "install" phase? (Yes, almost certainly, to avoid breaking existing recipes.)

## Summary

Tsuku's action system is **moderately close** to supporting lifecycle hooks. The executor's conditional step framework (WhenClause) is proven and could be extended with a phase qualifier with minimal changes. However, there is currently **zero lifecycle infrastructure**: no pre/post/cleanup phases exist, the remove flow doesn't consult recipes, and the update flow has no customization points. Adding lifecycle hooks is a matter of **extending existing patterns** (WhenClause) rather than building new abstractions, but the remove and update flows would need significant refactoring to consult recipes and execute lifecycle steps at appropriate moments. The set_env and run_command actions are flexible enough to handle most lifecycle needs once the framework exists.

