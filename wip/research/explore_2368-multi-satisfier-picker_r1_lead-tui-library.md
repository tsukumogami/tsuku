# Issue #2368: TUI Library Selection for Arrow-Key-Driven Multi-Satisfier Alias Picker

**Research Lead:** @danielgazineu  
**Date:** 2026-04-30  
**Status:** Discovery Complete  

## Executive Summary

**Recommendation: golang.org/x/term + hand-rolled ANSI solution**

For tsuku's single-select picker on #2368 (multi-satisfier alias install), **do NOT add an external TUI library**. Instead, build a lightweight picker using `golang.org/x/term` (already a direct dependency) with raw ANSI escape sequences. This approach:
- Adds **zero binary size** (x/term already imported)
- Maintains tsuku's **minimal dependency footprint** (important for a distributed single binary)
- Integrates seamlessly with existing **progress.Reporter** and TTY-detection patterns
- Supports the **one specific feature** needed (arrow keys + single select) without framework overhead

A full framework like bubbletea or huh would add ~2-5 MB in transitive dependencies for functionality tsuku doesn't need.

---

## Investigation Results

### 1. Current Dependencies Analysis

**go.mod findings:**
- `golang.org/x/term v0.37.0` — **already a direct dependency**
- `golang.org/x/sys v0.42.0` — **already a direct dependency**
- `github.com/spf13/cobra v1.10.1` — CLI framework (no TUI features)
- **NO charmbracelet/*, manifoldco/promptui, AlecAivazis/survey, or fzf bindings present**

**Binary size context:**
- Current cmd/tsuku/*.go total: ~637 KB source code
- Typical release binary: ~15-25 MB (gzipped across platforms)
- Each added TUI framework adds 2-5 MB uncompressed (bubbletea ~800 KB, huh adds lipgloss + other deps)

### 2. Candidate Libraries Status

| Library | Already Imported | Added Binary Size | API Quality for Single-Select | Cross-Platform | Status |
|---------|------------------|-------------------|-------------------------------|-----------------|--------|
| `golang.org/x/term` | ✓ YES (direct) | **+0 bytes** | Fair (manual impl) | ✓ Linux/macOS/Windows | **RECOMMENDED** |
| `charmbracelet/bubbletea` | ✗ NO | ~1.2 MB | Excellent (Elm model) | ✓ Full | Overkill for single-select |
| `charmbracelet/huh` | ✗ NO | ~1.5 MB | Excellent (form-focused) | ✓ Full | Over-engineered; designed for multi-field forms |
| `manifoldco/promptui` | ✗ NO | ~200 KB | Good (select, confirm primitives) | ⚠ Limited (Windows weak) | Unmaintained since 2020 |
| `AlecAivazis/survey` | ✗ NO | ~300 KB | Good (select, prompt) | ⚠ Windows edge cases | Last release 2022; Windows TTY issues |
| fzf (as library) | ✗ NO | ~2-3 MB | Excellent (fuzzy) | ✓ Full | Overkill; no Rust→Go bindings maintained |

### 3. Existing Interactive Patterns in tsuku

**Discovered callback pattern** in `internal/discover/resolver.go`:
```go
type ConfirmDisambiguationFunc func(matches []ProbeMatch) (int, error)
```

**Current TTY detection** in `internal/progress/progress_writer.go`:
```go
var IsTerminalFunc = term.IsTerminal
func ShouldShowProgress() bool {
    return IsTerminalFunc(int(os.Stdout.Fd()))
}
```

**Reporter abstraction** in `internal/progress/reporter.go`:
- Already handles TTY/non-TTY separation
- Spinner is built with raw ANSI escape sequences (braille chars: ⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏)
- Pattern: **no external TUI framework**, just careful ANSI handling

**Implication:** tsuku already has all the infrastructure needed; adding a framework is architectural debt for a single feature.

### 4. Cross-Platform Testing Notes

**Linux (glibc + musl):**
- x/term: ✓ Excellent
- All libraries: ✓ Work well

**macOS:**
- x/term: ✓ Excellent
- All libraries: ✓ Work well

**Windows (edge case):**
- x/term: ✓ Windows 10+ console API support
- promptui, survey: ⚠ Windows TTY detection flaky in edge cases (legacy console)
- bubbletea, huh: ✓ Full support via x/term

---

## Implementation Approach (for #2368)

### Option A: Hand-Rolled (Recommended)

**Cost:** 150-200 lines of Go  
**Benefits:** Zero dependencies, consistent with existing patterns, ~40 KB binary impact (code only)

```
internal/tui/picker.go
├── SingleSelectPicker interface
├── RunPicker(ctx, items []string) (int, error)
├── Handle arrow keys (^[[A, ^[[B)
├── Handle Enter/Escape
├── Clear line, render highlight, restore cursor
└── Fail gracefully on non-TTY (error or default)
```

**Uses:**
- `golang.org/x/term` for raw mode, window size
- `os.Stdin`/`Stdout` for I/O
- Manual ANSI escape sequences (re-use spinner patterns from `progress/reporter.go`)

### Option B: If Weight Is Acceptable (promptui)

**If** binary size creep becomes acceptable:
- `promptui` has clean, simple API for single-select
- Lightweight relative to bubbletea
- BUT: Unmaintained; Windows edge cases not guaranteed

---

## Recommendation Rationale

1. **Minimal Feature Need:** Issue #2368 requires one thing—arrow keys + single select. A framework is over-engineered.

2. **Existing Precedent:** tsuku's `progress` module already rolls ANSI handling. Consistency matters.

3. **Binary Distribution:** As a single-binary tool, every dependency MB affects download time and distribution size.

4. **Proven Stability:** x/term is Go's official TTY package; zero risk.

5. **Maintainability:** Hand-rolled code in `internal/tui` is auditable and debuggable. Framework bugs become your problem.

6. **Graceful Degradation:** Easy to detect non-TTY and provide fallback (list with index, or error + --from flag).

---

## Validation Checklist

- [ ] Confirm issue #2368 requires single-select only (not multi-select, not fuzzy)
- [ ] Check if multi-satisfier alias install needs TTY at all vs. --from fallback
- [ ] Review `internal/discover/` to confirm callback integration point
- [ ] Draft picker.go interface; validate against ConfirmDisambiguationFunc signature
- [ ] Test on Linux (glibc), macOS (M1/x86), Windows 11 console

---

## References

- `internal/progress/reporter.go` — TTY detection + spinner ANSI patterns
- `internal/discover/resolver.go:ConfirmDisambiguationFunc` — callback signature for interactive disambiguation
- `go.mod` — confirmed x/term, x/sys direct deps
- golang.org/x/term package docs: https://pkg.go.dev/golang.org/x/term
