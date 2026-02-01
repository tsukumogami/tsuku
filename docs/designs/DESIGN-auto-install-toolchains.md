---
status: Accepted
problem: |
  When `tsuku create --from crates.io` (or rubygems, npm, pypi) runs on a system
  without the required toolchain, it fails with exit code 8 and a message like
  "Cargo is required to create recipes from crates.io. Install Rust or run:
  tsuku install rust". The user must then manually install the toolchain and
  re-run the create command.

  This is frustrating because tsuku already has recipes for these toolchains
  (rust, ruby, nodejs, pipx). The error message even names the right recipe.
  The plumbing exists but the create command doesn't use it.

  The problem affects anyone using `tsuku create` in a fresh environment or CI
  pipeline. It's particularly painful with `--yes` mode, where the whole point
  is unattended operation but the command still fails on missing toolchains.
decision: |
  When `tsuku create` detects a missing toolchain, it offers to install it
  using tsuku's own recipes instead of failing. In interactive mode, the user
  gets a yes/no prompt. With `--yes`, the install happens automatically.

  The implementation modifies `create.go` at the existing `toolchain.CheckAvailable`
  call site. On failure, it extracts the recipe name from `toolchain.GetInfo()`,
  calls the existing `installTool()` function, then re-checks availability.
  If the install succeeds and the binary is now on PATH, create continues
  normally.

  This follows the same pattern as `install_deps.go`'s dependency resolution,
  where missing dependencies are installed automatically before proceeding.
  The change is contained to `create.go` and `toolchain.go` (adding a method
  to return structured info for auto-install).
rationale: |
  Handling the auto-install in `create.go` rather than pushing it into the
  toolchain package keeps the toolchain package focused on detection and the
  command layer responsible for user interaction. This matches how the install
  command handles dependencies at the command layer.

  The alternative of making the toolchain package handle installation would
  create a circular dependency (toolchain -> install logic) and mix concerns.
  Another alternative, adding toolchains as recipe metadata dependencies, would
  be cleaner architecturally but requires schema changes and affects all recipes.
  The command-layer approach ships faster and can be refactored later if a
  pattern emerges across more commands.

  Using `--yes` to control auto-install behavior (rather than a separate flag)
  is consistent with how `--yes` already works in create: it suppresses all
  interactive prompts.
---

# DESIGN: Auto-install Required Toolchains for Ecosystem Builders

## Status

Accepted

## Context and Problem Statement

The `tsuku create --from <ecosystem>` command requires external toolchains for certain ecosystems: `cargo` for crates.io, `gem` for rubygems, `npm` for npm, and `pipx` for pypi. When these binaries aren't on PATH, the command fails with exit code 8.

The failure message already knows the exact tsuku recipe to fix the problem (`tsuku install rust`, `tsuku install ruby`, etc.), but the command doesn't act on that knowledge. Users must manually install the toolchain and re-run the command. In batch/CI scenarios with `--yes`, this means the pipeline fails even though tsuku could self-heal.

The `install` command already handles dependency auto-installation through `installWithDependencies()`. The create command should follow the same principle: if tsuku knows how to fix a missing dependency, it should offer to do so.

### Scope

**In scope:**
- Auto-installing toolchains for the 4 ecosystem builders that use `toolchain.CheckAvailable()`
- Interactive confirmation in normal mode
- Automatic installation with `--yes` flag
- Re-checking toolchain availability after installation

**Out of scope:**
- Adding toolchains to recipe metadata (dependency field changes)
- Auto-installing toolchains for the install command itself
- Supporting external/system toolchains not managed by tsuku

## Decision Drivers

- **Consistency**: Follow existing patterns in install's dependency resolution
- **User experience**: Don't fail when tsuku knows how to fix the problem
- **Batch support**: `--yes` must enable fully unattended operation
- **Minimal change**: Contained modification, no schema or API changes
- **Safety**: User confirmation before installing unexpected software (interactive mode)

## Considered Options

### Decision 1: Where to Handle Auto-install

The toolchain check currently happens in `create.go` at lines 305-310. When `toolchain.CheckAvailable()` returns an error, the command prints it and exits. The question is where to add the auto-install logic.

#### Chosen: Command layer (create.go)

Handle auto-install in `create.go` at the existing check site. On failure, prompt the user (or auto-install with `--yes`), call the existing install machinery, then re-check. This keeps the toolchain package as a pure detection layer and puts user interaction in the command layer where it belongs.

The implementation adds roughly 20-30 lines to `create.go`:
1. Check if the toolchain has a tsuku recipe (`toolchain.GetInfo()` already returns `TsukuRecipe`)
2. Prompt the user or auto-proceed with `--yes`
3. Call `installToolByName()` (or equivalent exposed install function)
4. Re-check `toolchain.CheckAvailable()`
5. Fail with current error if install didn't help

#### Alternatives Considered

**Toolchain package handles installation**: Move install logic into `internal/toolchain/`, making `CheckAvailable` return an installable dependency instead of just an error. Rejected because it creates a dependency from the toolchain package back to install logic, mixing detection with remediation. The toolchain package is currently clean and focused.

**Recipe metadata dependencies**: Add a `toolchain_dependency` field to recipe TOML that the install command resolves automatically. Rejected because it requires schema changes, affects all recipes, and solves a broader problem than what's needed. Could be the right approach later if more commands need toolchain resolution, but it's over-engineering for the current scope.

### Decision 2: User Confirmation Model

When auto-installing, the user is getting software they didn't explicitly request. The question is how to handle consent.

#### Chosen: Prompt in interactive mode, auto-install with --yes

In interactive mode, show a prompt like:
```
crates.io requires Cargo, which is not installed.
Install rust using tsuku? [Y/n]
```

Default to yes (capital Y) since the user already expressed intent to create a recipe from that ecosystem. With `--yes`, skip the prompt and install directly, matching how `--yes` handles all other confirmations in the create command.

#### Alternatives Considered

**Always auto-install without prompting**: Skip confirmation entirely since the user's intent (create from ecosystem X) implies they want the toolchain. Rejected because installing software without explicit consent is surprising, even if contextually reasonable. The `--yes` flag exists for users who want this behavior.

**New --auto-deps flag**: Add a dedicated flag for toolchain auto-installation. Rejected because it fragments the confirmation model. `--yes` already means "don't ask me, just do it" throughout the create command. Adding another flag for the same concept adds confusion.

## Decision Outcome

**Chosen: Command-layer auto-install with interactive confirmation**

### Summary

When `toolchain.CheckAvailable()` fails in `create.go`, instead of exiting immediately, the command checks if the missing toolchain has a tsuku recipe (via `toolchain.GetInfo().TsukuRecipe`). If a recipe exists, it prompts the user to install it (or auto-installs with `--yes`). After installation, it re-checks availability and continues with the create flow if the binary is now on PATH.

The `toolchain.Info` struct already has the `TsukuRecipe` field ("rust", "ruby", "nodejs", "pipx") that maps to the correct recipe. The create command already has access to install functionality. The change is adding a conditional branch between the check failure and the exit.

If the user declines the install (interactive mode), or if the install fails, or if the binary still isn't on PATH after installation, the command fails with the existing error message and exit code 8.

Edge cases:
- If `TsukuRecipe` is empty (unknown ecosystem), fall through to current behavior
- If the recipe install succeeds but the binary still isn't on PATH (e.g., PATH not configured), fail with a message suggesting `eval $(tsuku shellenv)`
- If the user is already mid-install of the toolchain (concurrent tsuku processes), the re-check handles this gracefully

### Rationale

Handling this at the command layer keeps the change small and contained. The toolchain package stays focused on detection, and `create.go` handles the user interaction and remediation flow. This matches how install handles dependencies at the command layer in `install_deps.go`.

Using `--yes` rather than a new flag avoids fragmenting the confirmation model. The create command already uses `--yes` to mean "skip all prompts", and toolchain installation is just another prompt.

## Solution Architecture

### Overview

The change touches two files:

1. **`internal/toolchain/toolchain.go`**: Add a public function to get the `Info` struct (currently `GetInfo` exists but only returns the struct, which is sufficient)
2. **`cmd/tsuku/create.go`**: Replace the check-and-exit block with check-prompt-install-recheck

### Data Flow

```
create.go: toolchain.CheckAvailable(ecosystem)
  ├─ available → continue normally
  └─ not available
       ├─ toolchain.GetInfo(ecosystem).TsukuRecipe == "" → exit 8 (current behavior)
       └─ TsukuRecipe exists
            ├─ interactive mode → prompt user
            │    ├─ user says yes → install → recheck
            │    └─ user says no → exit 8
            └─ --yes mode → install → recheck
                 ├─ recheck passes → continue
                 └─ recheck fails → exit 8 with shellenv suggestion
```

### Key Interface

The `toolchain.GetInfo()` function already returns:

```go
type Info struct {
    Binary      string // e.g., "cargo"
    Name        string // e.g., "Cargo"
    Language    string // e.g., "Rust"
    TsukuRecipe string // e.g., "rust"
}
```

No new types or interfaces needed. The create command calls the existing install machinery.

## Implementation Approach

### Step 1: Modify create.go toolchain check block

Replace lines 305-310 in `create.go`:

```go
// Current:
if err := toolchain.CheckAvailable(builderName); err != nil {
    fmt.Fprintf(os.Stderr, "Error: %v\n", err)
    exitWithCode(ExitDependencyFailed)
}

// New:
if err := toolchain.CheckAvailable(builderName); err != nil {
    info := toolchain.GetInfo(builderName)
    if info != nil && info.TsukuRecipe != "" {
        if !offerToolchainInstall(info, builderName, createAutoApprove) {
            exitWithCode(ExitDependencyFailed)
        }
        // Re-check after install
        if err := toolchain.CheckAvailable(builderName); err != nil {
            fmt.Fprintf(os.Stderr, "Error: %s was installed but '%s' is still not on PATH.\n", info.TsukuRecipe, info.Binary)
            fmt.Fprintf(os.Stderr, "Try running: eval $(tsuku shellenv)\n")
            exitWithCode(ExitDependencyFailed)
        }
    } else {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        exitWithCode(ExitDependencyFailed)
    }
}
```

### Step 2: Add offerToolchainInstall function

```go
func offerToolchainInstall(info *toolchain.Info, ecosystem string, autoApprove bool) bool {
    fmt.Fprintf(os.Stderr, "%s requires %s, which is not installed.\n",
        ecosystem, info.Name)

    if !autoApprove {
        if !confirmWithUser(fmt.Sprintf("Install %s using tsuku?", info.TsukuRecipe)) {
            return false
        }
    } else {
        fmt.Fprintf(os.Stderr, "Installing %s (required toolchain)...\n", info.TsukuRecipe)
    }

    // Use existing install machinery with explicit=false (toolchain is a dependency, not user-requested)
    visited := make(map[string]bool)
    err := installWithDependencies(info.TsukuRecipe, "", "", false, "create", visited, telemetryClient)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: failed to install required toolchain '%s': %v\n", info.TsukuRecipe, err)
        return false
    }
    fmt.Fprintf(os.Stderr, "%s installed successfully.\n", info.TsukuRecipe)
    return true
}
```

### Step 3: Add functional test

Add a scenario to `test/functional/features/create.feature` that verifies the auto-install flow works when a toolchain is missing.

### Step 4: Update existing tests

Update `internal/toolchain/toolchain_test.go` to verify `GetInfo` returns correct recipe names for all ecosystems.

## Security Considerations

### Download Verification

Toolchains installed via auto-install go through the same install path as manual `tsuku install` commands. Checksum verification, if configured in the recipe, applies equally. No new download paths are introduced.

### Execution Isolation

The auto-installed toolchain runs in the same $TSUKU_HOME environment as any manually installed tool. No elevated privileges are needed. The toolchain binary ends up in `$TSUKU_HOME/tools/` and gets symlinked to `$TSUKU_HOME/bin/`, same as any other tool.

### Supply Chain Risks

The toolchain recipes (rust, ruby, nodejs, pipx) are embedded in tsuku and download from their official sources (rustup.rs, ruby-lang.org, nodejs.org, pypi.org). Auto-install doesn't change the supply chain, it just automates a step the user would do manually. The `installWithDependencies` function resolves recipes through the standard registry loader, which checks embedded recipes first. A user-created local recipe with the same name would take precedence, but that's the same trust model as manual `tsuku install`.

One consideration: auto-install with `--yes` means the user might not realize a toolchain was installed. The stderr output ("Installing rust (required toolchain)...") makes this visible, but in batch mode stderr might not be reviewed. This is acceptable because the user explicitly opted into unattended mode with `--yes`.

### User Data Exposure

No additional user data is accessed or transmitted. The install flow sends the same telemetry (if enabled) as a normal `tsuku install` command. No new data collection points.

## Consequences

### Positive

- `tsuku create --from crates.io` works in fresh environments without manual setup
- `--yes` mode enables fully unattended recipe creation pipelines
- Existing error messages and exit codes preserved for genuinely unresolvable failures
- Small change, low risk

### Negative

- Users might be surprised by a toolchain install prompt when they expected create to fail. Mitigated by clear messaging ("X requires Y, which is not installed. Install Y using tsuku?").
- In `--yes` mode, toolchain installation happens without explicit consent for that specific tool. Mitigated by stderr output and by the fact that `--yes` is an explicit opt-in to unattended behavior.
- If the toolchain install fails (network issue, etc.), the error might be confusing since the user asked to create, not install. Mitigated by wrapping the install error with context ("Failed to install required toolchain 'rust': ...").
