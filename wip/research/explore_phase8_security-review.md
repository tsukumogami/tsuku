# Security Review and Pragmatism Analysis: Cargo Subcommand Verification

## Document Reviewed

`docs/designs/DESIGN-cargo-subcommand-verification.md`

Grounded against: `internal/builders/cargo.go`, `internal/builders/orchestrator.go`, `internal/validate/verify_analyzer.go`, and existing cargo subcommand recipes in `recipes/c/`.

---

## Security Analysis

### 1. Attack Vectors

**No new attack surface introduced.** The change modifies which string gets placed in `Verify.Command` -- from `cargo-hack --version` to `cargo hack --version`. Both strings are constructed from executable names that pass through `isValidExecutableName()` at `cargo.go:347`, which restricts to `[a-zA-Z0-9._-]`. No shell metacharacters can reach the verify command through this path.

The `cargo <subcommand>` form does invoke cargo as an intermediary, but cargo is already a trusted dependency for installation (`cargo_install` action). Verify commands run in the same sandbox/execution context as installation. No privilege escalation.

**One minor vector not discussed in the design**: if `discoverExecutables()` falls back to the crate name (line 233, 240, 247, 252 in `cargo.go`) and the crate name itself starts with `cargo-`, the verify command would become `cargo <subcommand> --version`. This is fine -- `isValidCrateName` at line 338 enforces `^[a-zA-Z][a-zA-Z0-9_-]*$`, and cargo will just say the subcommand doesn't exist. Not exploitable, but worth noting that the fallback path also triggers the `cargo-` prefix detection.

### 2. Are Mitigations Sufficient?

Yes. The design correctly identifies that the only negative consequence (cargo must be in PATH at verify time) is already guaranteed by the `cargo_install` action's own requirements. The mitigation is complete.

### 3. Residual Risk

None worth escalating. The existing `isValidExecutableName` regex prevents injection. The cargo dependency is already required. The sandbox execution model is unchanged.

### 4. "Not Applicable" Justifications

All four "not applicable" marks are correctly applied:

- **Download Verification**: Correct. This is about verify command strings, not download integrity.
- **Supply Chain Risks**: Correct. The source of binaries is unchanged.
- **User Data Exposure**: Correct. Verify commands produce version strings locally.
- **Execution Isolation**: Addressed inline rather than marked N/A. The explanation is adequate.

No gaps.

---

## Pragmatism Analysis

### 5. Is the Design Over-Engineered?

**The CargoBuilder fix (Component 1) is well-scoped.** It's a simple `strings.HasPrefix` check at the point where the verify command is generated. Five lines of code. Correct level of effort for the problem.

**The repair loop fallback (Component 2) is borderline YAGNI.** See finding below.

### 6. Dead Code / Scope Creep

**Phase 3 (Existing Recipe Cleanup) is scope creep -- but justified.** Updating `cargo-edit.toml` to use `cargo add --version` instead of the `--help` workaround is a direct consequence of the fix and restores version validation. This is good scope creep -- it validates the fix works on an existing recipe. I wouldn't flag it.

No dead code in the proposed changes.

### 7. Repair Loop Fallback: Real Value or YAGNI?

**Advisory: the repair loop fallback adds marginal value.**

The design states the fallback handles two cases:
1. "Recipes generated before the fix" -- these are existing recipes. Looking at the actual codebase, `cargo-audit.toml` and `cargo-watch.toml` already use `cargo-<name> --version` and it works (they wouldn't be in the repo otherwise). `cargo-deny.toml` uses `github_archive`, not `cargo_install`, so it doesn't go through CargoBuilder. The only broken existing recipe is `cargo-edit.toml`, which Phase 3 fixes manually.
2. "Edge cases where cargo subcommand --version doesn't work in initial generation but works at repair time" -- this is speculative. The design itself argues that `cargo <subcommand> --version` always works.

The repair loop fallback catches zero known cases. The CargoBuilder fix prevents new cases. Phase 3 fixes the one existing case.

However, the fallback is bounded: it's a single `if` block prepended to the existing fallback list. It doesn't introduce new abstractions, new types, or new test infrastructure beyond one additional test case. The code cost is low.

**Verdict: Advisory, not blocking.** The fallback is defense-in-depth for a scenario with no known instances. If the team values belt-and-suspenders for batch-generated recipes, keep it. If minimalism is preferred, drop Phase 2 and the design still solves the problem completely.

---

## Summary of Findings

| # | Severity | Location | Finding |
|---|----------|----------|---------|
| 1 | Advisory | Design Phase 2 / `orchestrator.go` | Repair loop cargo fallback has no known caller. The CargoBuilder fix (Phase 1) prevents new broken recipes; Phase 3 fixes the one existing broken recipe. The fallback is cheap but has zero known trigger cases. Consider dropping. |
| 2 | Not a finding | Security | All "not applicable" security justifications are correct. No new attack surface. `isValidExecutableName` prevents injection into verify command strings. |
| 3 | Not a finding | Design scope | Phase 3 recipe cleanup is scope creep but justified -- it validates the fix and restores version checking on `cargo-edit.toml`. |

---

## Recommendations

1. **Ship Phase 1 + Phase 3 as the minimal correct fix.** The CargoBuilder change prevents bad verify commands. The cargo-edit cleanup validates it works.
2. **Phase 2 is optional.** Include it if the team wants defense-in-depth for batch operations. Drop it if you want the smallest possible change. Either way, the design is correct.
3. **No security concerns.** The design doesn't need security escalation.
