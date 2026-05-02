# Research: Issue #2368 - Alias Picker TTY Detection and Error Format

**Date:** 2026-04-30  
**Author:** Research Lead  
**Status:** Initial Findings

## Summary

This document answers two core questions for issue #2368 (alias picker):
1. How does tsuku detect "is stdout/stdin a TTY" and what packages are used?
2. What is the exact format of the existing "Multiple sources found" error for consistency?

---

## Section 1: TTY Detection

### Current Implementation

Tsuku uses **`golang.org/x/term`** for TTY detection across the codebase.

**Primary Location:** `cmd/tsuku/install.go:31-34`

```go
import "golang.org/x/term"

// isInteractive returns true if stdin is connected to a terminal.
// Uses term.IsTerminal for a proper ioctl check — the previous
// ModeCharDevice check incorrectly returned true for /dev/null.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
```

### Package Usage Across Codebase

| Package | Location | Purpose |
|---------|----------|---------|
| `golang.org/x/term` | `internal/progress/progress_writer.go:14` | Check if stdout is a TTY for progress display |
| `golang.org/x/term` | `cmd/tsuku/install.go:31-34` | Check if stdin is a TTY for interactive prompts |

**Important Note:** The codebase does NOT use `mattn/go-isatty` or any stdlib `io.IsTerminal`. It exclusively relies on `golang.org/x/term.IsTerminal()` which performs proper ioctl checks (unlike the older mode-based check that incorrectly accepted `/dev/null`).

### Recommendation for Picker

**Use `golang.org/x/term.IsTerminal(int(os.Stdin.Fd()))` for the picker's TTY check.**

**Rationale:**
- Consistent with all existing interactive prompts in tsuku
- Properly detects true TTY via ioctl (avoids `/dev/null` false positives)
- Already imported in the relevant module
- Follows established pattern

---

## Section 2: Existing Interactive Prompts

### Pattern 1: Yes/No Confirmation with (y/N) Default

**Location:** `cmd/tsuku/create.go:1065`

```go
func confirmWithUser(prompt string) bool {
	if !isInteractive() {
		return false  // Non-interactive: always decline
	}
	
	fmt.Fprintf(os.Stderr, "%s (y/N) ", prompt)  // stderr, not stdout
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}
```

**Usage Sites:**
- `create.go`: "Use this source?" prompt
- `install_distributed.go`: "Install from unregistered source X?" prompt
- `create.go`: "Install X using tsuku?" prompt
- `create.go`: "Continue without sandbox testing?" prompt
- `install_sandbox.go`: "Proceed with sandbox testing? [y/N]"

### Pattern 2: Project Install Confirmation with [Y/n] Default

**Location:** `cmd/tsuku/install_project.go:164`

```go
if !installYes && isInteractive() {
	fmt.Print("Proceed? [Y/n] ")  // stdout, not stderr
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "" && line != "y" && line != "yes" {
		exitWithCode(ExitUserDeclined)
	}
}
```

**Key Difference:** Uses uppercase default `[Y/n]` (yes is default) vs lowercase `(y/N)` (no is default).

### Pattern 3: Autoinstall Prompt

**Location:** `internal/autoinstall/run.go:168`

```go
prompt := fmt.Sprintf("Install %s", match.Recipe)
if version != "" {
	prompt += "@" + version
}
prompt += "? [y/N] "
_, _ = fmt.Fprint(r.stdout, prompt)

// ... read response
if answer != "y" && answer != "yes" {
	return ErrUserDeclined
}
```

### Prompt Patterns Summary

| Pattern | Default | Stream | Locations |
|---------|---------|--------|-----------|
| `(y/N)` | No | stderr | create.go, install_distributed.go, create.go |
| `[Y/n]` | Yes | stdout | install_project.go (batch install) |
| `[y/N]` | No | stdout | autoinstall (internal) |

### Recommendation for Picker

**The picker should follow the `(y/N)` pattern** (default: no, confirm to proceed):
- Consistent with most existing prompts
- Uses stderr (safer than stdout which may be redirected)
- Case-insensitive (`y` or `yes` accepted)
- Returns false (decline) for EOF or non-interactive

---

## Section 3: Error Format - "Multiple Sources Found"

### Existing Error Format

**Location:** `internal/discover/resolver.go:103-108`

```go
// AmbiguousMatchError indicates multiple ecosystem matches with no clear winner.
type AmbiguousMatchError struct {
	Tool    string           // The requested tool name
	Matches []DiscoveryMatch // Ranked matches for display
}

func (e *AmbiguousMatchError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Multiple sources found for %q. Use --from to specify:\n", e.Tool)
	for _, m := range e.Matches {
		fmt.Fprintf(&b, "  tsuku install %s --from %s:%s\n", e.Tool, m.Builder, m.Source)
	}
	return strings.TrimSuffix(b.String(), "\n")
}
```

### Exact Output Example

```
Multiple sources found for "bat". Use --from to specify:
  tsuku install bat --from crates:bat
  tsuku install bat --from homebrew:bat
```

### Exit Code and Streaming

| Aspect | Value |
|--------|-------|
| Exit Code | `ExitAmbiguous = 10` (from exitcodes.go:39-41) |
| Output Stream | stdout (standard error output via fmt.Fprintf) |
| Error Type | Logged via `printError()` which writes to stderr |

### JSON Error Output

**Flag:** `--json` enables structured output (install command only)

**Location:** `cmd/tsuku/install.go`

```go
type installError struct {
	Status         string   `json:"status"`
	Category       string   `json:"category"`
	Subcategory    string   `json:"subcategory,omitempty"`
	Message        string   `json:"message"`
	MissingRecipes []string `json:"missing_recipes"`
	ExitCode       int      `json:"exit_code"`
}

// For ExitAmbiguous (10):
// Category: "install_failed" (default catch-all)
// Subcategory: (empty)
// Message: the full AmbiguousMatchError string above
```

### Example JSON Output
```json
{
  "status": "error",
  "category": "install_failed",
  "subcategory": "",
  "message": "Multiple sources found for \"bat\". Use --from to specify:\n  tsuku install bat --from crates:bat\n  tsuku install bat --from homebrew:bat",
  "missing_recipes": [],
  "exit_code": 10
}
```

---

## Section 4: `-y` / `--yes` / `--force` Flag Semantics

### Current Behavior

**Flag Definition:** `cmd/tsuku/install.go:36`

```go
var installYes bool
// Registered as: --yes, -y shorthand
// Help: "Skip confirmation prompts (e.g., unregistered source approval)"
```

### Usage in Context

**Pattern A: Distributed Sources (install_distributed.go:120)**
```go
if !autoApprove && isInteractive() {
	prompt := fmt.Sprintf("Install from unregistered source %q?", source)
	if !confirmWithUser(prompt) {
		return fmt.Errorf("installation canceled: source %q not approved", source)
	}
}
```
- `-y` makes `autoApprove=true` → skips the prompt
- Non-TTY: `isInteractive()=false` → also skips the prompt
- **These are NOT the same:** TTY but no `-y` will prompt; non-TTY skips regardless

**Pattern B: Project Install (install_project.go:163)**
```go
if !installYes && isInteractive() {
	fmt.Print("Proceed? [Y/n] ")
	// ... read response
}
```
- `-y` bypasses the prompt
- Non-TTY: `isInteractive()=false` → no prompt, accepts default (yes)

### Distinct Concepts

| Scenario | `-y` | TTY | Behavior |
|----------|------|-----|----------|
| Interactive approve | false | true | **Prompt** |
| Non-interactive approve | false | false | **Auto-approve** (batch mode) |
| Forced approve | true | true | **Skip prompt** (CI mode) |
| Forced approve | true | false | **Skip prompt** (CI mode) |

**Key:** `-y` and non-TTY are **independent gates**. Non-TTY defaults to "yes"; `-y` explicitly forces "yes".

### Recommendation for Picker

For "Multiple recipes satisfy alias 'X'" under `-y`:
- Use `-y` to suppress the picker entirely (auto-select first ranked recipe)
- Non-TTY should **also** suppress the picker (same as install behavior)
- **Pattern:** `if !autoApprove && isInteractive() { /* show picker */ }`

---

## Section 5: Proposed "Multiple Recipes Satisfy" Error Format

Given consistency requirements, the new error under `-y` should mirror the existing pattern.

### Proposed Format

```
Multiple recipes satisfy alias 'X'. Use --recipe to specify:
  tsuku install --recipe crates:X
  tsuku install --recipe homebrew:X
```

**OR** (if this is truly a "picker" scenario requiring user choice):

```
Multiple recipes found for alias 'X'. Select one:
  1. crates:X (popular, 1200+ downloads)
  2. homebrew:X (popular, 800+ downloads)
```

### Implementation Guidance

1. **Non-interactive (non-TTY or `-y`):**
   - Auto-select first ranked recipe (by downloads/versions/priority)
   - Log selection reason to audit log
   - Do NOT error

2. **Interactive (TTY and no `-y`):**
   - Show picker (if issue #2368 implements one)
   - Allow user to select
   - Return `ErrUserDeclined` on Ctrl+C

3. **Error Handling:**
   - If picker returns error (Ctrl+C): use `ExitUserDeclined = 13`
   - If multiple recipes with no clear winner and non-interactive: auto-select + audit
   - Do NOT reuse `ExitAmbiguous = 10` (reserved for ecosystem discovery)

---

## Code Locations Summary

| Aspect | File | Lines |
|--------|------|-------|
| TTY detection | `cmd/tsuku/install.go` | 31-34 |
| TTY detection (progress) | `internal/progress/progress_writer.go` | 14-19 |
| Confirmation prompt | `cmd/tsuku/create.go` | ~1065 |
| Project confirm prompt | `cmd/tsuku/install_project.go` | 163-170 |
| Ambiguous error | `internal/discover/resolver.go` | 103-108 |
| Exit codes | `cmd/tsuku/exitcodes.go` | 1-70 |
| `-y` handling | `cmd/tsuku/install.go` | 36 + usage |

---

## References

- **Issue:** #2368 (alias picker)
- **Related:** autoinstall prompts, distributed source registration
- **Packages:** `golang.org/x/term` only (not go-isatty)
