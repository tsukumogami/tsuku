# Maintainer Review: Issue #1642

## Issue: feat(llm): add download permission prompts and progress UX

## Review Focus: maintainability (clarity, readability, duplication)

## Files Reviewed

- `internal/llm/addon/prompter.go` (new)
- `internal/llm/addon/prompter_test.go` (new)
- `internal/progress/spinner.go` (new)
- `internal/progress/spinner_test.go` (new)
- `internal/llm/addon/manager.go` (modified)
- `internal/llm/addon/manager_test.go` (modified)
- `internal/llm/factory.go` (modified)
- `internal/llm/local.go` (modified)
- `cmd/tsuku/create.go` (modified)
- `internal/builders/builder.go` (modified)
- `internal/builders/homebrew.go` (modified)
- `internal/builders/github_release.go` (modified)
- `internal/discover/llm_discovery.go` (modified)

---

## Finding 1: NilPrompter godoc is stale

**File**: `internal/llm/addon/prompter.go:86-88`
**Severity**: Advisory

```go
// NilPrompter declines all download prompts.
// Used as a safe default when no prompter is configured.
type NilPrompter struct{}
```

The comment says "Used as a safe default when no prompter is configured." But the factory defaults to `InteractivePrompter` (factory.go:158-159), not `NilPrompter`. The `NilPrompter` is only used in tests (manager_test.go:411, prompter_test.go:151). The next developer reading this comment will think `NilPrompter` is wired as a production default somewhere and will go looking for that callsite.

**Suggestion**: Update to `// NilPrompter declines all download prompts. Useful in tests to verify decline behavior.`

---

## Finding 2: TestNewFactory_WithNilPrompterExplicit name misleads

**File**: `internal/llm/factory_test.go:681`
**Severity**: Advisory

```go
func TestNewFactory_WithNilPrompterExplicit(t *testing.T) {
    // ...
    factory, err := NewFactory(ctx, WithPrompter(nil))
```

This test passes a literal `nil`, not `&addon.NilPrompter{}`. The name `WithNilPrompterExplicit` reads as "with the NilPrompter struct, explicitly" when it actually means "with a nil value passed explicitly." A developer searching for `NilPrompter` usage will find this test and assume it exercises `NilPrompter`, but it doesn't -- it tests the distinct behavior of passing Go nil to `WithPrompter`.

**Suggestion**: Rename to `TestNewFactory_WithExplicitNilPromptValue` or `TestNewFactory_PrompterExplicitlySetToNil` to make it clear the test is about the nil value, not the NilPrompter type.

---

## Finding 3: Prompter interface return contract is ambiguous

**File**: `internal/llm/addon/prompter.go:20-26`
**Severity**: Advisory

```go
// ConfirmDownload asks the user to confirm a download.
// ...
// Returns true if the user approves, false if declined.
// Returns ErrDownloadDeclined if the user explicitly declines.
type Prompter interface {
    ConfirmDownload(ctx context.Context, description string, sizeBytes int64) (bool, error)
}
```

The contract says "Returns true if approved, false if declined" AND "Returns ErrDownloadDeclined if declined." This describes two separate signals for the same event. Looking at all three implementations:

- `InteractivePrompter`: returns `(false, ErrDownloadDeclined)` on decline -- both signals
- `AutoApprovePrompter`: returns `(true, nil)` -- clear
- `NilPrompter`: returns `(false, ErrDownloadDeclined)` -- both signals

The caller in `manager.go:125-130` checks both:
```go
ok, err := m.prompter.ConfirmDownload(...)
if err != nil { return "", err }
if !ok { return "", ErrDownloadDeclined }
```

The `!ok` branch is dead code for all current implementations. A new Prompter author reading the interface doc might return `(false, nil)` to signal decline, which would work correctly (the `!ok` guard catches it). But the ambiguity means they won't know whether they're supposed to return the error or not.

**Suggestion**: Clarify the contract. Either: (a) the bool is redundant and error is canonical (`// Returns ErrDownloadDeclined when the user declines. The bool return is always true when err is nil.`), or (b) document that returning `(false, nil)` is also valid. The current doc leaves the implementer guessing.

---

## Finding 4: Spinner hardcodes 80-column terminal width

**File**: `internal/progress/spinner.go:81,99,123-125`
**Severity**: Advisory

```go
// Stop
fmt.Fprintf(s.output, "\r%s\r", strings.Repeat(" ", 80))

// StopWithMessage
fmt.Fprintf(s.output, "\r%s\r%s\n", strings.Repeat(" ", 80), message)

// animate
if len(line) < 80 {
    line += strings.Repeat(" ", 80-len(line))
}
```

The magic number 80 appears in three places without a named constant. More importantly, if the spinner message is longer than 80 characters, the padding in `animate` does nothing, and the clear in `Stop` won't fully clear the line. This isn't a blocking issue because the messages used ("Generating...", "Generation failed.") are short, but the hardcoded 80 is a trap for anyone who later passes a longer message.

**Suggestion**: Extract `const termWidth = 80` with a comment explaining the assumption. This makes the limitation visible.

---

## Finding 5: Spinner not safe for reuse after Stop

**File**: `internal/progress/spinner.go:35-58,68-83`
**Severity**: Advisory

`NewSpinner` creates the `done` channel once. `Stop` closes it. `Start` resets `stopped = false` but does not recreate the `done` channel. If a caller reuses a spinner by calling `Start` after `Stop`, the goroutine from `animate` will exit immediately because `s.done` is already closed.

Current usage in `local.go:130` creates a fresh spinner per `Complete` call, so this is not reachable today. But the `Start`/`Stop` API looks reusable -- there's no comment or documentation warning that a Spinner is single-use.

**Suggestion**: Add a comment to `NewSpinner` or `Start`: `// A Spinner is single-use: do not call Start after Stop. Create a new Spinner instead.`

---

## Finding 6: Duplicated error message strings for cloud fallback

**File**: `internal/llm/local.go:98,119`
**Severity**: Advisory

```go
// Line 98 (addon decline)
return nil, fmt.Errorf("local LLM addon download declined; configure ANTHROPIC_API_KEY or GOOGLE_API_KEY for cloud inference instead")

// Line 119 (model decline)
return nil, fmt.Errorf("model download declined; configure ANTHROPIC_API_KEY or GOOGLE_API_KEY for cloud inference instead")
```

The cloud fallback guidance (`configure ANTHROPIC_API_KEY or GOOGLE_API_KEY for cloud inference instead`) is duplicated. If a new provider is added (e.g., OpenAI), both messages need updating. This is a minor duplication since there are only two instances, but the strings are long enough that divergence would be hard to spot in review.

**Suggestion**: Extract the cloud fallback suffix as a constant or helper: `const cloudFallbackHint = "configure ANTHROPIC_API_KEY or GOOGLE_API_KEY for cloud inference instead"`.

---

## Overall Assessment

The implementation is clean and well-structured. The Prompter interface, three implementations (Interactive, AutoApprove, Nil), and factory option pattern are all idiomatic Go. The wiring from CLI (`--yes` flag) through builders to the factory is complete and tested at each layer. The Spinner is simple, handles TTY/non-TTY correctly, and has good double-stop protection.

The test coverage is thorough: approve, decline, EOF, non-TTY, zero-size, double-stop, and the factory integration tests all verify meaningful behavior. Test names generally describe what they test.

None of the findings above would cause a misread that leads to a bug. The stale NilPrompter comment and the Prompter return-value contract ambiguity are the most likely to confuse a future contributor, but both are limited in blast radius.
