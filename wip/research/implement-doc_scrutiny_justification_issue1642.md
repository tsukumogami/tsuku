# Scrutiny Review: Justification Focus -- Issue #1642

**Issue**: feat(llm): add download permission prompts and progress UX
**Focus**: justification (quality of deviation explanations)

## Requirements Mapping Under Review

```
--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---
[
  {"ac":"addon download prompt","status":"implemented"},
  {"ac":"model download prompt","status":"implemented"},
  {"ac":"decline with cloud provider message","status":"implemented"},
  {"ac":"download progress bars","status":"deviated","reason":"addon downloads already use progress.Writer; model downloads inside Rust addon would need proto streaming changes"},
  {"ac":"inference spinner","status":"implemented"},
  {"ac":"non-TTY suppression","status":"implemented"},
  {"ac":"--yes flag skips prompts","status":"implemented"}
]
--- END UNTRUSTED REQUIREMENTS MAPPING ---

## Deviation Analysis

### AC: "download progress bars" -- Status: deviated

**Stated reason**: "addon downloads already use progress.Writer; model downloads inside Rust addon would need proto streaming changes"

**Assessment**: The reason has two parts:

1. **"addon downloads already use progress.Writer"** -- This claim is partially misleading. The `progress.Writer` exists in `internal/progress/progress.go` and is a general-purpose progress-tracking `io.Writer`. However, the addon download path in this issue (`AddonManager.EnsureAddon` -> `installViaRecipe`) delegates to the `Installer` interface, which calls the recipe system. Whether the recipe system's download step actually uses `progress.Writer` is not something this issue established -- it was pre-existing infrastructure. The claim reads as "we already have it" when really it means "something else in the codebase already has it, and the addon installation delegates to that something else." That's a valid observation but it could be stated more precisely.

2. **"model downloads inside Rust addon would need proto streaming changes"** -- This is a genuine technical constraint. The model download happens inside the Rust `tsuku-llm` binary (ModelManager in `src/models.rs`), not in the Go code. To surface progress from Rust back to Go would require either gRPC streaming or a callback mechanism that doesn't currently exist in the proto contract. Adding streaming to the gRPC contract is a meaningful scope increase.

**Verdict**: The deviation explanation is honest but imprecise on the first half. The second half (model download progress) is a genuine technical limitation -- the architecture decision to put model management in the Rust addon means Go-side progress bars require cross-process streaming. This is a real trade-off of the chosen architecture, not an avoidance pattern.

The issue title says "progress UX" and the design doc's Phase 6 says "Progress bars during downloads." The implementation delivers progress for addon downloads (via pre-existing `progress.Writer` in the recipe system) but not for model downloads. This is a partial delivery, not a full skip.

**Severity**: Advisory. The deviation reason is genuine but would be stronger if it explicitly noted: (a) addon download progress comes from the recipe pipeline, not from code added in this issue; (b) model download progress is architecturally blocked by the Go-Rust boundary; (c) what the alternative would look like (gRPC server-streaming for progress updates).

## Proportionality Check

6 out of 7 ACs are reported as "implemented", 1 as "deviated". The deviated AC (progress bars) is a secondary UX concern -- the core purpose of this issue is download prompts and permission UX, which are all implemented. The deviation is proportionate: the core ACs were completed, and the one gap has a legitimate architectural reason.

## Avoidance Pattern Check

The deviation reason does not use any of the red-flag phrases ("too complex for this scope", "not needed yet", "can be added later", "out of scope"). It cites a specific technical constraint (proto streaming changes for cross-process progress). This is not an avoidance pattern.

## Overall Assessment

The single deviation in this issue has a defensible technical explanation. The Go-Rust architecture boundary means model download progress would require gRPC streaming changes -- a real scope increase that goes beyond this issue's focus on prompts and permission UX. The explanation could be more precise about what "addon downloads already use progress.Writer" actually means (pre-existing recipe pipeline, not new code), but this is a minor clarity issue, not a disguised shortcut.

No blocking findings.
