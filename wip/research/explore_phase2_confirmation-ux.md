# LLM Discovery: Confirmation UX Research

**Phase**: 2 (LLM Discovery)
**Role**: Security UX Research
**Date**: 2026-02-10
**Status**: Complete

## Executive Summary

This research analyzes confirmation UX patterns for security-sensitive actions in CLI tools, with focus on tsuku's LLM-discovered source confirmation. The key finding is that **explicit trust signals matter more than frequency of prompts**. Best practice pattern: confirm critical decisions once, provide rich metadata to justify the trust, offer ways to bypass (like `--yes` or `--force`) for automation.

---

## Part 1: Existing tsuku Confirmation Patterns

### Pattern 1: Sandbox Testing Confirmation (`create.go`)

**Code location**: `cmd/tsuku/create.go:114-133`

```go
func confirmSkipSandbox() bool {
    if !isInteractive() {
        fmt.Fprintln(os.Stderr, "Error: --skip-sandbox requires interactive mode")
        return false
    }
    
    fmt.Fprintln(os.Stderr, "WARNING: Skipping sandbox testing. The recipe has NOT been tested.")
    fmt.Fprintln(os.Stderr, "Risks: Binary path errors, missing extraction steps, failed verification")
    fmt.Fprint(os.Stderr, "Continue without sandbox testing? (y/N) ")
    
    reader := bufio.NewReader(os.Stdin)
    response, err := reader.ReadString('\n')
    // ...
    return response == "y" || response == "yes"
}
```

**Characteristics**:
- **Explicit risk statement**: Lists concrete risks (binary path errors, missing extraction steps)
- **Default deny**: `(y/N)` makes "no" the default
- **Non-interactive handling**: Returns false if not a TTY
- **Simple yes/no format**: Single-character or word responses

### Pattern 2: Generic User Confirmation (`create.go`)

**Code location**: `cmd/tsuku/create.go:135-149`

```go
func confirmWithUser(prompt string) bool {
    if !isInteractive() {
        return false
    }
    
    fmt.Fprintf(os.Stderr, "%s (y/N) ", prompt)
    reader := bufio.NewReader(os.Stdin)
    response, err := reader.ReadString('\n')
    // ...
    return response == "y" || response == "yes"
}
```

**Characteristics**:
- **Minimal overhead**: Reusable function for various confirmations
- **Consistent prompt format**: Same `(y/N)` pattern
- **Non-interactive safety**: Returns false (deny) by default

### Pattern 3: LLM Budget/Rate-limit Confirmation (`create.go`)

**Code location**: `cmd/tsuku/create.go:437-449`

```go
if confirmErr, ok := err.(builders.ConfirmableError); ok {
    fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
    if confirmWithUser(confirmErr.ConfirmationPrompt()) {
        sessionOpts.ForceInit = true
        orchResult, err = orchestrator.Create(ctx, builder, buildReq, sessionOpts)
        // ...
    } else {
        exitWithCode(ExitGeneral)
    }
}
```

**Characteristics**:
- **Error-triggered confirmation**: Only prompts when an exceptional condition occurs
- **Custom prompt message**: Error object provides context-specific message
- **Retry with override**: User can choose to proceed anyway with `ForceInit`
- **Exit on denial**: Aborts the operation if user declines

### Pattern 4: Recipe Preview and Approval (`create.go`)

**Code location**: `cmd/tsuku/create.go:534-587`

```go
func previewRecipe(r *recipe.Recipe, result *builders.BuildResult) (bool, error) {
    fmt.Printf("Generated recipe for %s:\n\n", r.Metadata.Name)
    
    // Show downloads
    fmt.Println("  Downloads:")
    urls := extractDownloadURLs(r)
    // ... display URLs
    
    // Show actions (steps)
    fmt.Println("  Actions:")
    for i, step := range r.Steps {
        fmt.Printf("    %d. %s\n", i+1, describeStep(step))
    }
    
    // Show verification
    if r.Verify.Command != "" {
        fmt.Printf("  Verification: %s\n", r.Verify.Command)
    }
    
    // Show LLM cost
    if result.Provider != "" {
        fmt.Printf("  LLM: %s (cost: $%.4f)\n", result.Provider, result.Cost)
    }
    
    // Show repair attempts
    if result.RepairAttempts > 0 {
        fmt.Printf("  Note: Recipe required %d repair attempt(s)\n", result.RepairAttempts)
    }
    
    return promptForApproval(r)
}

func promptForApproval(r *recipe.Recipe) (bool, error) {
    for {
        fmt.Print("[v]iew full recipe, [i]nstall, [c]ancel? ")
        input, err := reader.ReadString('\n')
        
        switch input {
        case "v", "view":
            // Show full TOML
        case "i", "install":
            return true, nil
        case "c", "cancel", "":
            return false, nil
        }
    }
}
```

**Characteristics**:
- **Rich information display**: Shows downloads, actions, cost, warnings
- **Multi-step approval**: User chooses action from a menu
- **Optional drill-down**: User can view full recipe before deciding
- **Loop until clear answer**: Stays in prompt until user makes explicit choice
- **Skip with `--yes`**: Batch mode bypasses preview entirely

### Pattern 5: First-time Install Decision

**Code location**: `cmd/tsuku/create.go:153-169`

```go
func offerToolchainInstall(info *toolchain.Info, ecosystem string, autoApprove bool) bool {
    fmt.Fprintf(os.Stderr, "%s requires %s, which is not installed.\n", ecosystem, info.Name)
    
    if autoApprove {
        fmt.Fprintf(os.Stderr, "Installing %s (required toolchain)...\n", info.TsukuRecipe)
    } else {
        if !confirmWithUser(fmt.Sprintf("Install %s using tsuku?", info.TsukuRecipe)) {
            return false
        }
    }
    
    if err := runInstallWithTelemetry(info.TsukuRecipe, "", "", false, "create", nil); err != nil {
        fmt.Fprintf(os.Stderr, "Error: failed to install required toolchain '%s': %v\n", info.TsukuRecipe, err)
        return false
    }
    
    fmt.Fprintf(os.Stderr, "%s installed successfully.\n", info.TsukuRecipe)
    return true
}
```

**Characteristics**:
- **Contextual explanation**: Explains why the tool is needed
- **Auto-approve option**: Respects `--yes` / `autoApprove` flag
- **Shows what's being installed**: Clear message about next steps
- **Success confirmation**: Prints confirmation when done

---

## Part 2: How `--yes` and `--force` Are Used

### The `--force` Flag in tsuku

**Used for**: Bypass recipe existence checks, allow overwriting
- In `create.go`: `--force` allows overwriting existing recipes
- In `install.go`: `--force` passed from `install` to `create` when using `--from`

```go
var installForce bool
// ...
createAutoApprove = installForce
createForce = true  // overwrite existing recipe
```

### The `--yes` Flag in tsuku

**Used for**: Skip interactive confirmations and preview screens
- Affects: Recipe preview, sandbox skip confirmation, toolchain install confirmation
- Behavior: All confirmations default to "yes" (approve)

```go
if createAutoApprove {
    fmt.Fprintln(os.Stderr, "Skipping recipe review (--yes). The recipe will be installed without confirmation.")
}
```

**Warning when set**: Users are warned that recipe will be installed without confirmation

### Non-interactive Mode Detection

**Code location**: `cmd/tsuku/install.go:228-235`

```go
func isInteractive() bool {
    fileInfo, err := os.Stdin.Stat()
    if err != nil {
        return false
    }
    return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
```

**Behavior when not interactive**:
- All confirmation functions return false (deny)
- Sandbox skip confirmation: Error and fail
- Generic confirmations: Silent deny
- Non-interactive mode prevents hanging on prompts

---

## Part 3: How Other CLI Tools Handle Security Confirmations

### brew install (Homebrew)

**Pattern**: Minimal prompts, trust system based on package source

```
brew install stripe-cli
# No confirmation needed if already in Homebrew registry
# Output: ==> Downloading https://formulae.brew.sh/...
# Only prompts if installing from unverified tap
```

**Key insight**: Trust is established by membership in the curated registry. Homebrew doesn't ask "are you sure?" for known packages. It only prompts when adding a new tap (source).

### npm install (Node.js)

**Pattern**: Requires confirmation for security-sensitive operations

```
npm install --unsafe-perm
npm audit --audit-level=moderate
# Prompts if package has known vulnerabilities
```

**Key insight**: Confirmation is tied to specific risks (unsafe permissions, security vulnerabilities), not general install operations. The risk context is explicit.

### pip install (Python)

**Pattern**: Requires `--force-reinstall` or external confirmation for dangerous operations

```
pip install --force-reinstall package==1.0.0
pip install --no-deps package
# No interactive prompts; flags control behavior
```

**Key insight**: Python delegates to flags rather than interactive prompts. No stdin interaction during install. This works well for pipelines but requires users to know the flags.

### cargo install (Rust)

**Pattern**: No confirmation prompts; security via artifact verification

```
cargo install ripgrep
# No prompts; trust via crates.io registry + checksum verification
```

**Key insight**: Rust delegates security to the registry's integrity (checksums, signatures). No UX overhead for common case.

### github cli (gh)

**Pattern**: Minimal confirmation for risky operations; explicit tokens for sensitive operations

```
gh auth login
# Prompts for token, but only once during setup
gh pr merge --auto  # Never prompts; uses auth from setup
```

**Key insight**: Security-sensitive decisions are made at setup time (authentication), not during each operation. Subsequent operations are declarative.

### wget/curl

**Pattern**: No confirmation for downloads; flags control behavior

```
wget https://example.com/file.sh
# No confirmation; user is responsible for URL trust
```

**Key insight**: No interactive prompts expected. Users verify URLs before passing them. This works in scripting but is less safe for exploratory use.

---

## Part 4: Metadata That Helps Users Make Informed Decisions

From the DESIGN-discovery-resolver.md, LLM-discovered sources should display:

1. **Repository age**: Days since first publish
   - Indicates maturity and abandonment risk
   - Example: "Published 3 years ago"

2. **Star count**: GitHub stars (popularity signal)
   - High stars = community adoption
   - Example: "2.4K stars"

3. **Last commit**: Date of last code change
   - Indicates active maintenance
   - Example: "Last updated 2 weeks ago"

4. **Owner name**: Repository owner
   - Helps verify official vs. community projects
   - Example: "github.com/cli/cli (GitHub official)"

5. **Download count**: For ecosystem packages
   - Shows adoption in that ecosystem
   - Example: "45K downloads/month on crates.io"

6. **Repo status**: Archived, fork, mirror
   - Critical for security
   - Example: "WARNING: Repository is archived"

### Metadata Display Format (from discovery resolver design)

```
Found bat (sharkdp/bat via crates.io, 45K downloads/day). 
Also available: npm (bat-cli, 200 downloads/day). 
Use --from to override.
```

For LLM-discovered sources, richer format:
```
Repository: github.com/sharkdp/bat
Stars: 43,289
Age: 6 years old (first publish: 2018-04-29)
Last commit: 2 weeks ago
Owner: sharkdp (unverified)

Install and verify in sandbox before running.
Continue? (y/N)
```

---

## Part 5: Interactive vs. Non-interactive Mode Behavior

### Interactive Mode (TTY connected)

1. **Confirmation enabled**: All confirmations are active
2. **Rich display**: Full metadata, descriptions, options
3. **User input expected**: Reads from stdin
4. **Error handling**: Can retry or abort gracefully

**Example: LLM discovery confirmation**
```
Discovery: Found via web search

Repository: github.com/stripe/stripe-cli
Stars: 398
Age: 5 years old
Last commit: 3 days ago

Continue with installation? (y/N)
```

### Non-interactive Mode (stdin is piped or redirected)

1. **Confirmation behavior**: 
   - All confirmations default to **deny** (fail-safe)
   - `--yes` flag overrides and defaults all to **approve**
2. **No waiting for input**: Never blocks on stdin
3. **Fail-fast behavior**: Error immediately if confirmation needed without `--yes`
4. **Suitable for**: CI/CD pipelines, automated scripts

**Example: Pipeline behavior**
```bash
echo "install-tool" | tsuku install  # Fails: confirmation needed, not interactive
tsuku install --yes install-tool     # Succeeds: explicit approval via flag
```

### The Decision Tree

```
isInteractive() ?
  |
  ├─ YES (TTY):
  |   ├─ Show confirmation with metadata
  |   ├─ Read stdin for y/N response
  |   └─ Proceed or abort based on response
  |
  └─ NO (piped/redirected):
      ├─ --yes flag set?
      |   ├─ YES: Approve all confirmations silently
      |   └─ NO: Deny and error with actionable message
      └─ (never block on stdin)
```

---

## Part 6: Key Findings for LLM Discovery Confirmation

### Finding 1: One Confirmation Per Critical Decision

**Best practice**: Prompt once for the LLM-discovered source, not multiple times.

**Why**: User fatigue + security habituation lead to approval by default. Multiple confirmations for the same decision train users to say "yes" without thinking.

**Implementation**:
- Single confirmation after LLM discovery completes
- Rich metadata display to justify trust
- Skip entirely with `--yes` for automation

### Finding 2: Trust Signals Matter More Than Warnings

**Evidence**:
- Homebrew: No confirmation for registry packages (trust via membership)
- npm: Only confirms for packages with known vulnerabilities (risk-specific)
- GitHub CLI: Trust established at auth time (delegated)

**For tsuku**: LLM-discovered sources are inherently riskier than registry entries, so confirmation is justified. But the confirmation should focus on **trust signals** (repo age, stars, owner) rather than generic "are you sure?" warnings.

**Implementation**:
```
Discovered: sharkdp/bat via GitHub

✓ Stars: 43,289 (well-established)
✓ Age: 6 years (mature project)
✓ Last commit: 2 weeks ago (actively maintained)
✓ Owner: sharkdp (not GitHub staff, but verified by stars/age)

This will be tested in a sandbox before installation.
Continue? (y/N)
```

### Finding 3: Skip Confirmations With Automation Flags

**Best practice**: `--yes` skips all confirmations for batch use. Batch must still verify integrity (sandbox validation).

**Current implementation in tsuku**:
- `--yes` skips preview
- `--yes` skips sandbox skip confirmation
- `--force` allows overwriting existing recipes

**For LLM discovery**: 
- `--yes` should skip LLM confirmation
- Sandbox validation is still required (defense in depth)
- Warning message when `--yes` is used

**Implementation**:
```go
if autoApprove {
    fmt.Fprintln(os.Stderr, "Warning: Skipping confirmation (--yes). Recipe will be tested in sandbox.")
} else {
    if !confirmWithUser(confirmErr.ConfirmationPrompt()) {
        return false
    }
}
```

### Finding 4: Non-interactive Needs Clear Error Messages

**Current pattern**: Return false (deny) when not interactive, causing a clean error message.

**For LLM discovery**:
```
Error: LLM-discovered source requires confirmation.
stdin is not a terminal. Use --yes to approve, or run interactively.
  tsuku install stripe-cli --yes
  tsuku install stripe-cli  # in terminal
```

### Finding 5: "Remember This Source" Is Complex

**Question from research request**: Should there be a "remember this source" option?

**Answer**: **Not recommended** for LLM-discovered sources.

**Why**:
1. Creates a side channel of trust decisions
2. Complicates state management (what if repo is compromised later?)
3. Discourages explicit decisions (users trust cache over re-evaluating)
4. Recipe registry already provides "remember" semantics

**Instead**:
- LLM discovery generates a recipe
- Recipe is cached at `$TSUKU_HOME/recipes/<tool>.toml`
- Subsequent installs use the cached recipe (no LLM call)
- Users can update with `tsuku create --from github:... --force`

This separates concerns: discovery is a one-time event, recipes are the persistent cache.

### Finding 6: Metadata Freshness and Staleness

**From security section of discovery resolver**:

Registry entries can point to compromised repos. Mitigation layers:
1. **User confirmation**: repo age, stars, owner
2. **GitHub API verification**: confirm repo exists, not archived
3. **Sandbox validation**: recipe actually works
4. **Periodic freshness checks**: CI validates registry entries

For LLM discovery specifically:
- Verify repo exists at time of discovery (GitHub API call)
- Display age and stars from GitHub API response
- Don't cache LLM-recommended sources in registry (they're temporary)
- Sandbox validation is the final defense

---

## Part 7: Visual Design and Formatting

### Terminal Width Considerations

**Assumption**: 80-character terminal (common minimum)

**Current pattern in tsuku**:
```
        fmt.Fprintln(os.Stderr, "WARNING: Skipping sandbox testing. The recipe has NOT been tested.")
        fmt.Fprintln(os.Stderr, "Risks: Binary path errors, missing extraction steps, failed verification")
```

**For LLM discovery confirmation**:
- Line wrapping at ~80 chars
- Use indentation for structure
- Bold/color only if terminal supports it (libraries: color, bold, underline)

### Recommended Format

```
Discovered via LLM web search:

  Repository: github.com/stripe/stripe-cli
  
  Trust signals:
    Stars: 398
    Age: 5 years (first publish 2019-02)
    Last commit: 3 days ago
    Owner: stripe (GitHub staff? no)
  
  This will be tested in a sandbox before installation.
  
  Continue? (y/N) 
```

Or more compact:
```
Discovered: stripe-cli (github.com/stripe/stripe-cli)
Stars: 398 | Age: 5 years | Updated: 3 days ago
[Sandbox testing enabled]
Continue? (y/N)
```

### Character Encoding

- Use ASCII only (no emoji, no Unicode symbols)
- Exception: `✓` (U+2713) for checkmarks if terminal supports UTF-8
- Fallback to `[ok]` for ASCII-only terminals

---

## Part 8: Interaction Flow Diagram

```
tsuku install stripe-cli (no recipe found)
                  |
                  v
        Discovery resolver chain
                  |
        ┌─────────┼─────────┐
        |         |         |
      MISS      MISS      HIT
    (Registry) (Ecosystem) (LLM)
        |
        v
  LLM web search + GitHub API verification
        |
        v
  isInteractive() ?
    |        |
   YES      NO
    |        |
    v        v
  Show   Check --yes
  info     |     |
    |    YES   NO
    |     |     |
    v     v     v
  Confirm Approve  Error
    |       |    (needs --yes)
    |       |
    └───┬───┘
        |
        v
  Create recipe + sandbox test
```

---

## Part 9: Example Prompts for Implementation

### For LLM-Discovered GitHub Source (Common Case)

```
Discovered via LLM web search:

Repository: github.com/stripe/stripe-cli
  Stars: 398
  Age: 5 years old (published 2019-02-01)
  Last commit: 2 weeks ago
  Owner: stripe (organization)

This recipe will be tested in a sandbox before installation.

Install stripe-cli? (y/N) 
```

### For LLM-Discovered Archived Repository (Warning Case)

```
Discovered via LLM web search:

Repository: github.com/example/old-project
  ⚠ WARNING: Repository is archived (no longer maintained)
  Stars: 1,200
  Age: 7 years old (published 2017-01-15)
  Last commit: 3 years ago
  Owner: example

This recipe will be tested in a sandbox before installation.

Install old-project? (y/N)
```

### For Non-interactive Error

```
Error: LLM-discovered source requires confirmation
  Tool: stripe-cli
  Source: github.com/stripe/stripe-cli
  
stdin is not a terminal. Cannot prompt for confirmation.

To approve this source automatically, use:
  tsuku install stripe-cli --yes

To review before installing, run in an interactive terminal.
```

### For Piped Input With --yes

```
Warning: Installing stripe-cli from LLM-discovered source (--yes)
Source: github.com/stripe/stripe-cli
This recipe will be tested in a sandbox before installation.
```

---

## Part 10: Recommendations for Issue #1318 (LLM Discovery Implementation)

### UX Layer (Confirmation Display)

1. **Location**: New function `confirmLLMDiscovery()` in `cmd/tsuku/create.go`
2. **Inputs**: `DiscoveryResult` with metadata
3. **Returns**: bool (approve/deny)
4. **Interactive check**: Call `isInteractive()` first

```go
func confirmLLMDiscovery(result *discover.DiscoveryResult) bool {
    if !isInteractive() {
        return false  // Fail-safe
    }
    
    // Display discovery result with metadata
    displayLLMDiscoveryInfo(result)
    
    return confirmWithUser("Continue installation?")
}

func displayLLMDiscoveryInfo(result *discover.DiscoveryResult) {
    fmt.Fprintf(os.Stderr, "Discovered via LLM web search:\n\n")
    fmt.Fprintf(os.Stderr, "  Repository: %s\n", result.Source)
    
    if result.Metadata.Stars > 0 {
        fmt.Fprintf(os.Stderr, "  Stars: %d\n", result.Metadata.Stars)
    }
    if result.Metadata.AgeDays > 0 {
        fmt.Fprintf(os.Stderr, "  Age: ~%d years\n", result.Metadata.AgeDays/365)
    }
    if result.Metadata.Description != "" {
        fmt.Fprintf(os.Stderr, "  %s\n", result.Metadata.Description)
    }
    
    fmt.Fprintf(os.Stderr, "\nThis recipe will be tested in a sandbox.\n\n")
}
```

### Control Flow Integration

1. After LLM discovery returns a result
2. Call `confirmLLMDiscovery(result)`
3. If false and interactive: abort with "Canceled"
4. If false and non-interactive: error with message suggesting `--yes`
5. If true: proceed to recipe generation and sandbox testing

### Non-interactive Handling

```go
if !isInteractive() && !createAutoApprove {
    fmt.Fprintf(os.Stderr, "Error: LLM-discovered source requires confirmation\n")
    fmt.Fprintf(os.Stderr, "Source: %s\n", result.Source)
    fmt.Fprintf(os.Stderr, "stdin is not a terminal. Use --yes to approve.\n")
    exitWithCode(ExitUsage)
}
```

### Batch Mode (`--yes`)

- Confirmation is skipped silently
- Warning message printed to stderr
- Sandbox testing still enforced
- Recipe preview still shown (but doesn't block)

---

## Conclusion

**For #1318 (LLM Discovery Implementation)**:

1. **Confirmation is necessary** for LLM-discovered sources (security-critical)
2. **One prompt is enough** (user fatigue avoidance)
3. **Rich metadata > generic warnings** (trust signals matter)
4. **Non-interactive defaults to deny** (fail-safe for pipelines)
5. **`--yes` bypasses prompt** but not sandbox validation
6. **No "remember source" cache** (recipe registry handles persistence)
7. **GitHub API verification + sandbox validation** provide defense in depth
8. **Clear error messages** when confirmation can't be obtained

The pattern is: **Show trust signals → Get explicit approval → Run sandbox test → Install recipe**

This balances security (mandatory confirmation, verification) with usability (rich information, automation support).
