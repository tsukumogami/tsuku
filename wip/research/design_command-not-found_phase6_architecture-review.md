# Architecture Review: command-not-found Hook Design

## Scope

Design document sections reviewed: Solution Architecture, Implementation Approach.
Codebase reference: `cmd/tsuku/`, `internal/index/`, `cmd/tsuku/index_adapter.go`, `cmd/tsuku/exitcodes.go`, `cmd/tsuku/cmd_which.go`.

---

## Finding 1: `tsuku suggest` duplicates `tsuku which` — parallel pattern introduction [Blocking]

`tsuku which <command>` already exists at `cmd/tsuku/cmd_which.go`. It calls `index.Open()`, calls `idx.Lookup()`, handles `ErrIndexNotBuilt` and `StaleIndexWarning`, and formats per-match output. The proposed `tsuku suggest` command does the same lookup over the same `BinaryIndex` interface and produces equivalent human-readable output.

The only behavioral difference is exit-code semantics (the design assigns exit 0/1/2 to suggest vs. the which command using `ExitGeneral=1` for all failures) and the addition of a `--json` flag.

This is the parallel pattern heuristic in direct form: two commands that call the same interface on the same data, with diverging output format and exit-code conventions. Once both exist, callers will use one or the other and the two will drift — error handling, staleness warnings, and output format will diverge independently.

**Resolution options (choose one):**

Option A — Extend `tsuku which` rather than adding `tsuku suggest`. Add `--json` to `which`. Assign the suggest-specific exit codes to `which` (or to a new `--suggest` mode flag). The hook script calls `tsuku which --json "$1"`. This is one command, one lookup path.

Option B — Keep `suggest` as a thin wrapper that delegates to `which`'s underlying logic via a shared helper function (not by shelling out). Both commands share exit-code handling and staleness logic from a single source. This is acceptable if there's a strong reason to present two distinct user-facing names.

Option C — If the design intent is that `suggest` is a machine interface and `which` is a human interface, make that distinction explicit in the design and ensure they share a single internal function for lookup + error handling, with only the output formatting forked.

The current design introduces two independent implementations of the same lookup path. That will compound.

---

## Finding 2: `cmd_suggest.go` placement mixes package layers — advisory

The design proposes `internal/cmd/cmd_suggest.go`. No such package exists. All CLI command files live in `cmd/tsuku/` (e.g., `cmd_which.go` is at `cmd/tsuku/cmd_which.go`). Placing a command handler in `internal/cmd/` would create a new package with no clear boundary from `cmd/tsuku/`, and would require the `cmd/tsuku/main.go` init() to import it, reversing the current pattern where all command files are in the same `main` package.

The correct placement is `cmd/tsuku/cmd_suggest.go` (and same for `cmd_hook.go`). This is likely a documentation artifact rather than an intent to create a new package — but it needs to be explicit in the design to avoid the implementor creating `internal/cmd/`.

---

## Finding 3: `internal/hook/` package boundary is clean [No issue]

The proposed `internal/hook/` package (install, uninstall, status) contains only filesystem and rc-file operations. It doesn't need to import `internal/index`, `internal/install`, or `internal/config` beyond what's needed for path resolution. This follows the dependency direction correctly — the hook subcommand wires configuration at `cmd/tsuku/cmd_hook.go` and passes paths down to `internal/hook/`.

The design is clear here: the package is self-contained and the wiring layer is at cmd/.

---

## Finding 4: Exit code 2 for "index not built" conflicts with `ExitUsage = 2` [Blocking]

`cmd/tsuku/exitcodes.go` defines `ExitUsage = 2` as "invalid arguments or usage error." The design proposes exit code 2 for "index not built" from `tsuku suggest`. These are different semantics on the same exit code.

Shell hook scripts that call `tsuku suggest` and branch on exit code 2 will receive that code for both "index not built" and "wrong number of arguments," producing wrong behavior silently (e.g., the hook might suppress output when the user typed a malformed invocation rather than an unknown command).

The design must either use an unused exit code for "index not built" (codes 11+ are free) or use exit code 1 for both no-match and index-not-built conditions and have the hook inspect stderr text (worse). Given the existing exit code table, define a new constant, e.g. `ExitIndexNotBuilt = 11`, and document it alongside the others.

---

## Finding 5: `BinaryIndex` interface in the design matches the real interface [No issue]

The design's `BinaryIndex` interface declaration (Lookup + BinaryMatch) matches `internal/index/binary_index.go` exactly. The design correctly states that `tsuku suggest` calls `BinaryIndex.Lookup()` directly rather than shelling out. The `BinaryMatch` struct in the design omits the `Command` and `Source` fields that the real struct has, but that's an omission in the design doc, not an architectural problem.

---

## Finding 6: Hook files as embedded assets — placement fits the pattern [No issue]

The design proposes `internal/hooks/embed.go` with an `embed.FS`. This follows the same pattern as the embedded recipe registry (`recipe.NewEmbeddedRegistry()`). The hook files themselves are static shell scripts with no runtime dependencies on Go packages, so embedding them is correct — they're assets, not code. No concern here.

---

## Finding 7: Phase sequencing is correct [No issue]

Phase 1 (`suggest`) → Phase 2 (hook files) → Phase 3 (`hook` subcommands) → Phase 4 (install script) is the right order. Each phase's deliverables are prerequisites for the next. No inversion.

The only sequencing note: Phase 3's `internal/hook/install.go` needs to know the path where hook files are written (`$TSUKU_HOME/share/hooks/`). This path should be derived from `config.DefaultConfig()` at the cmd/ layer and passed to `internal/hook/` as a parameter — not obtained by importing `internal/config` from within the hook package. The design doesn't show this wiring explicitly but the package structure implies it's handled at cmd/. Worth calling out to the implementor.

---

## Summary

| # | Finding | Severity |
|---|---------|----------|
| 1 | `tsuku suggest` duplicates `tsuku which` lookup path | Blocking |
| 4 | Exit code 2 conflicts with `ExitUsage` | Blocking |
| 2 | `internal/cmd/` package placement is wrong; should be `cmd/tsuku/` | Advisory |
| 7 | Config path threading into `internal/hook/` needs explicit wiring note | Advisory |
| 3 | `internal/hook/` boundary is clean | Clean |
| 5 | `BinaryIndex` interface matches existing implementation | Clean |
| 6 | Embedded hook files follow existing asset pattern | Clean |

The two blocking issues are distinct problems: one is structural (parallel lookup paths that will diverge) and one is a contract problem (exit code collision that will cause silent misbehavior in shell hooks). Both need resolution before implementation starts, since fixing them afterward requires changing the public CLI surface and the hook scripts.
