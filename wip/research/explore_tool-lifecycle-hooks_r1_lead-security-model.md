# Lead: What security constraints should lifecycle hooks operate under?

## Findings

### Other Package Managers' Security Models

#### npm: The Cautionary Tale
npm permits arbitrary `postinstall` scripts that execute during package installation without explicit user consent. The ecosystem widely uses this for:
- Compiling native extensions (via node-gyp)
- Building binaries
- Downloading additional dependencies
- Running setup scripts

Security implications:
- Scripts run with the privileges of the installing user
- No sandboxing or capability restrictions
- Supply chain attacks can propagate through dependencies (e.g., typosquatting packages that publish postinstall hooks)
- tsuku's docs explicitly document npm's postinstall risk in the ecosystem analysis
- The `--ignore-scripts` flag exists but is rarely used by default

**Key lesson:** npm's approach shows the risk of per-package arbitrary code execution without explicit controls. Hundreds of supply chain attacks have exploited this mechanism.

#### Homebrew: Formula PR Review + User Consent
Homebrew's post_install scripts:
- Are human-reviewed as part of formula PRs before merging to tap
- Run as the installing user (not root, even if installation requires sudo)
- Are embedded in the formula itself (not script downloads)
- Can reference caveats for user notification
- Are limited to setting up symlinks, shell completions, or environment hints

Security model: Community trust + code review + user privileges = moderate containment

#### dpkg/apt: Maintainer Scripts, Root Execution
Debian maintainer scripts (preinst, postinst, prerm, postrm):
- Run as root (elevated privilege)
- Are provided by the distro maintainer (not untrusted)
- Are reviewed by Debian security team before packaging
- Can do anything root can do (users rely on distro vetting)

Security model: Trust in distribution's review process + elevated privilege

#### Nix: Pure Builds, No Post-Install Scripts
Nix's approach is fundamentally different:
- No post-install hooks in the derivation itself
- Builds are pure (no network access, filesystem isolation)
- Activation scripts are separate from the build environment (evaluated after installation)
- Activation scripts cannot run arbitrary code - they're primarily for environment setup
- The Nix philosophy: builds should be reproducible and side-effect free

Security model: Purity + isolation + declarative activation (not imperative scripting)

#### Chocolatey: PowerShell Scripts with Review
PowerShell scripts for Windows packages:
- Reviewed before inclusion in the community repository
- Execute as the installing user
- Can call installers or perform setup tasks
- Require opt-in for automated runs (user consent)

### tsuku's Current Trust Boundary

**Pre-hook security baseline:**
1. Recipes are TOML files in a curated registry (no code execution during recipe definition)
2. Pre-built binaries are downloaded, verified by checksum, and symlinked
3. No arbitrary code execution happens during the install phase (actions are declarative: download, extract, chmod, install_binaries, set_env, run_command)
4. The `run_command` action exists but is rarely used (only pipx recipe uses it heavily)
5. Validation enforces:
   - Path traversal prevention (no ".." in paths)
   - URL scheme validation (https only for downloads)
   - SHA256 checksum validation
   - Dangerous patterns in verify commands (rm, sh piping, etc.) trigger warnings

**Current run_command constraints (from code review):**
- Preflight validation checks for hardcoded tsuku paths and warns
- Command runs in sh -c (shell escaping required by caller)
- Variables are variable-substituted ({install_dir}, {work_dir}, {PYTHON}, etc.)
- Requires_sudo flag allows skipping sudo-required commands during validation (safe default)
- No input validation on the command string itself (it's arbitrary shell)
- Runs as the user executing tsuku (no elevation)

**Recipe contribution model:**
- Public GitHub PRs with community review
- CI validates recipe TOML syntax and structure
- No automated security scanning of command strings
- Trust model relies on human review

### Threat Vectors

1. **Malicious recipe contribution**: A contributor adds a post-install hook that:
   - Exfiltrates SSH keys or credentials from $HOME
   - Adds a backdoor to PATH (replacing tools with compromised versions)
   - Launches a cryptominer
   - The hook runs with the user's privileges, so it can access anything the user can access

2. **Supply chain compromise**: An upstream tool is compromised and ships a new version with an init script that:
   - Gets downloaded and added to post-install hooks
   - Executes when installed
   - Tsuku's checksum validation only covers binary downloads, not init scripts in the archive

3. **Privilege escalation via hooks**: While tsuku itself runs as user:
   - If a recipe requires_sudo=true and has post-install hooks, the hooks might run with elevated privileges (depends on implementation)
   - This is dangerous if the hook is untrusted

4. **Shell injection in hook parameters**: If hooks allow parameter substitution:
   - `command = "echo {user_input}"` where user_input contains backticks or $() could execute arbitrary code
   - Variables passed through from recipes or user input are attack surface

5. **Dependency hooks executing**: If lifecycle hooks are allowed on dependencies:
   - Installing a tool could trigger post-install hooks on all transitive dependencies
   - This dramatically increases attack surface and trust requirements

### Current Action System Constraints

From code review, the Step/Action system has these properties:
- Actions are declarative (action name + parameters)
- Each action validates parameters via Preflight() method
- run_command's Preflight only warns about hardcoded paths; it does NOT validate the command content itself
- Actions can declare dependencies (install-time, runtime, eval-time)
- When steps have `when` conditions, they're evaluated at runtime against the target platform
- Steps don't currently have a "phase" field (e.g., post_install, pre_uninstall)

### Constraint Levels (Proposed)

#### Level 0: No Lifecycle Hooks (Current State)
- Post-install, pre-uninstall, pre-upgrade hooks do not exist
- run_command is treated as a regular step (executes during install)
- Provides maximum containment but limits tool setup capabilities

#### Level 1: Declarative Hooks Only
Hooks are NOT arbitrary shell scripts but a limited vocabulary of actions:
- `install_shell_completion` (copy prebuilt completion files to $TSUKU_HOME/completions/)
- `install_env_file` (copy env setup file to $TSUKU_HOME/tools/{name}/init.sh)
- `install_man_page` (register man pages)
- `register_shell_function` (define shell function in a sourced file, with template variables only)
- `cleanup_paths` (remove files during uninstall)

**Constraints:**
- No arbitrary shell execution
- Limited to copying/removing files from the recipe package
- Template variables are restricted (no user input)
- No network access
- No modification of system files outside $TSUKU_HOME

**Pros:** Very safe, sufficient for simple shell integration
**Cons:** Cannot handle complex setup (e.g., Nix's recursive nix activation)

#### Level 2: Constrained Scripts with Allowlist
Hooks can be shell scripts, but:
- Hooks are reviewed as part of recipe PR (human review required)
- Allowed operations: env var setup, symlink creation, file copying, function definitions
- Forbidden operations (detected by static analysis or runtime checks):
  - Network access
  - Modification of $HOME or system directories
  - Execution of arbitrary external commands
  - Accessing credentials or secrets
- Hooks cannot call out to external scripts (prevent supply chain via init scripts)
- Requires_sudo is not allowed in hooks (no privilege escalation)

**Constraints:**
- PR review + automated scanning for forbidden patterns (like current verify.command validation)
- Runs as user (no privilege escalation)
- Timeout (e.g., 30 seconds max execution)
- Cannot read/write outside $TSUKU_HOME and $HOME/.tsuku-temp

**Pros:** Handles most real-world use cases (shell init, env setup)
**Cons:** Still allows sophisticated attacks if reviewer misses malicious code; requires tooling investment

#### Level 3: Full Scripts with Security Model
Full arbitrary shell scripts:
- Only allowed for trusted recipes (Tier 1 - first-party maintained)
- Per-recipe opt-in audit/approval
- Same constraints as Level 2 plus:
  - Signature verification (recipe is GPG-signed)
  - User consent prompt (toolbox of tools: always ask, remember choice, never ask)
  - Audit logging (log all hook executions)
  
**Pros:** Enables advanced use cases
**Cons:** Complex, requires infrastructure (signing, consent UI, audit logs)

### Validation and Review Requirements

**Current state:** run_command validation only warns about hardcoded paths; does not inspect command content.

**For lifecycle hooks, recommended additions:**
1. **Static pattern detection** (expand verify.command validation):
   - Forbidden: `curl | sh`, `wget | bash`, `rm -rf /`, `dd if=/dev/zero`
   - Forbidden: Network commands that don't match allowlist (curl to tsuku registry OK, to arbitrary URLs suspicious)
   - Forbidden: System modification commands (chown, chmod on system files)
   - Warning: Commands with high entropy strings (potential base64-encoded malware)

2. **PR review policy**:
   - Hooks in new recipes require explicit review (not auto-merged)
   - Changes to hooks on existing recipes flag for review
   - Reviewers trained to spot exfiltration patterns (env var access, ssh key paths, credential stores)

3. **Dependency audit**:
   - If hooks are allowed on dependencies, each transitive dependency's hook must be reviewed
   - Or: hooks are only allowed on top-level recipes (not dependencies)

4. **Supply chain verification**:
   - If hooks download init scripts or additional setup from upstream: verify checksums
   - Don't allow inline hooks to call out to network resources

### Hook Ordering and Lifecycle

If hooks are implemented, clear ordering is critical:

**Install flow:** download -> extract -> run install steps -> run setup_build_env -> run other steps -> **run post_install hooks** -> copy to tool dir -> symlink binaries -> verify

**Pre-uninstall flow:** **run pre_uninstall hooks** -> remove symlinks -> remove files -> remove state

**Pre-upgrade flow:** (depends on design: atomic vs in-place?)

**Key constraint:** Hooks should NOT be able to modify what was just installed. They should only set up references (symlinks, env vars, shell functions), not modify the installation itself.

## Implications

### Declarative vs Imperative

The choice between declarative (Level 1) and imperative (Level 2+) has large implications:

**Declarative (Level 1):**
- Safer (limited vocabulary, harder to hide malicious code)
- Easier to audit and version across tsuku upgrades
- Sufficient for simple shell integration, env files, completions
- Niwa's shell-integration use case can be satisfied with `install_env_file`

**Imperative (Level 2+):**
- More flexible, handles complex setup scenarios
- Requires more review effort and better tooling
- Risk of reviewer fatigue leading to missed attacks

**Recommendation:** Start with declarative (Level 1) as a prototype. It solves the niwa use case and Homebrew's post_install pattern (which is mostly file copying). If future recipes need more complexity, upgrade to Level 2 with stricter review.

### Trust Model Shift

Adding hooks (even constrained ones) changes tsuku's trust model:

**Before hooks:**
- Trust the recipe author to have written correct action sequences
- Trust that actions themselves are safe (they're built into tsuku)
- Trust tsuku's validation of recipe syntax and checksums

**After hooks (Level 1 or 2):**
- Trust the recipe author to have written correct hook scripts
- Trust the reviewer to have caught malicious hooks in PR
- Trust that the hook vocabulary or constraints actually prevent bad behavior

This is a real trust shift. The current model (pre-built binaries, no post-install code) is significantly safer than adding hooks.

### Review and Audit Mechanism

For Level 2 (constrained scripts), a review/audit mechanism is essential:

1. **Automated scanning**: Validate hook scripts at PR time using a linter (similar to how verify.command warnings work)
2. **Human review labels**: PRs with hooks should require explicit "hook-reviewed" label from maintainer
3. **Audit trail**: Log which recipes have hooks, changes to hooks, and hook executions on user's system
4. **User consent**: Optional flag to skip hooks (e.g., `tsuku install --no-hooks`)

## Surprises

1. **npm's dominance as a cautionary tale:** The tsuku docs already document npm's postinstall risk in the ecosystem analysis. This wasn't a surprise in code, but it's remarkable how central this concern is to the overall design.

2. **run_command has no content validation:** The Preflight() method only warns about hardcoded paths; it does NOT inspect the command string for dangerous patterns. This is less rigorous than the verify.command validation (which checks for `rm`, `| sh`, etc.). This asymmetry suggests hooks will need additional validation infrastructure if adopted.

3. **Nix's pure build philosophy is orthogonal:** Nix doesn't really solve the lifecycle hook problem in the same way. It separates "build" (pure) from "activation" (post-install configuration), but activation scripts still need constraints. The lesson is that purity alone doesn't solve the trust problem - you still need to vet activation scripts.

4. **Homebrew's approach is practical but relies on review:** The fact that Homebrew post_install scripts are reviewed as part of formula PRs is the real security control. The actual constraints are minimal (runs as user, not root). This suggests that for tsuku, **human review is the primary control**, not technical constraints.

## Open Questions

1. **Should hooks be allowed on dependencies, or only top-level recipes?**
   - Allowing hooks on all transitive dependencies significantly increases review burden
   - Niwa's use case is top-level only (niwa itself needs shell setup, not its dependencies)
   - Recommendation: Start with top-level recipes only

2. **How should hook scripts be versioned and updated?**
   - Should updating a recipe's hooks trigger re-execution on already-installed tools?
   - Should there be a separate "hook version" field to allow detecting changes?
   - Nix solves this by treating activation as part of the derivation hash (pure approach)

3. **Should `requires_sudo` be allowed in hooks?**
   - Current run_command preflight skips sudo-required commands during validation
   - If a hook needs system-level changes (e.g., register as service), this becomes necessary
   - But sudo hooks are significantly more dangerous (privilege escalation attack surface)
   - Recommendation: Forbid in v1, revisit if real use case emerges

4. **How to handle hook failures during installation?**
   - Should a failed hook cause installation to fail, or just log a warning?
   - Should the tool remain usable if hook fails?
   - Recommendation: Hooks should be non-critical (fail gracefully, log, but don't block verification)

5. **Should hooks be sandboxed at the OS level (containers, seccomp)?**
   - This would enforce the constraints at runtime rather than relying on script analysis
   - Significant complexity, but could improve safety for Level 2+
   - Nix philosophically prefers purity; sandboxing is not the primary defense
   - npm/Homebrew do not use sandboxing, relying instead on review and user privileges

6. **How to handle the shell-init use case specifically?**
   - Niwa needs to source eval output from a shell function
   - Does this require hooks, or can it be achieved through tsuku's existing `set_env` + a separate activation mechanism?
   - Could tsuku's shellenv system be extended to source per-tool init scripts without needing recipe-defined hooks?

7. **Should there be a registry of "vetted" hooks that recipes can reference?**
   - Rather than inline scripts, recipes could reference predefined hooks by ID
   - E.g., `[[steps]] action = "post_install_hook" hook_id = "install-shell-completions"`
   - This would make review and auditing easier, but less flexible

## Summary

Tsuku's pre-hook security model (pre-built binaries, checksummed downloads, no post-install code execution) is significantly safer than models that permit arbitrary code. Adding lifecycle hooks requires choosing a trust/flexibility tradeoff: declarative hooks (Level 1) are safest but least flexible; constrained scripts (Level 2) require better tooling and review but handle most real use cases; full scripts (Level 3) require infrastructure like signing and consent prompts.

The primary security control across all peer package managers is **human code review**, not technical constraints. For tsuku to add hooks safely, it must invest in automated scanning (expanding beyond current verify.command validation), clear review policies for hook-containing PRs, and audit logging of hook executions. Nix's pure-build philosophy suggests starting with declarative hooks (Level 1) rather than attempting to constrain imperative scripts.

Hooks should NOT be allowed on dependencies in v1 (reduces review burden from O(n) transitive deps to O(k) top-level recipes), should NOT support requires_sudo (prevents privilege escalation attacks), and should fail gracefully rather than block installation (keeping hooks non-critical). The niwa use case can be satisfied with a simple `install_env_file` action rather than full script hooks.
