# Security and Pragmatism Review: Optional Executables in Compile Actions

**Design:** `docs/designs/DESIGN-optional-executables.md`
**Reviewers:** Pragmatic Reviewer (Phase 4 + Phase 8)
**Date:** 2026-02-23

## Summary

The design proposes making `executables` optional in three compile actions (`configure_make`, `cmake_build`, `meson_build`) so library-only packages can use compile-from-source instead of `apk_install` workarounds. The change is ~6 lines across 3 files: replacing an early-return error with a fallback to an empty slice.

---

## Security Review (Phase 8)

### 1. Attack Vectors Considered

**Verdict: No new attack surface.**

The change removes a validation gate (`executables` required), but that gate was never a security boundary. It was a "did you remember to fill in this field" check for recipe authors. The actual security controls are:

- **Path traversal validation** on executable names: still runs when `executables` is provided, correctly skipped when absent (nothing to validate).
- **Argument sanitization** (`isValidConfigureArg`, `isValidCMakeArg`, `isValidMesonArg`): unchanged, these protect the build command, not the post-build check.
- **Download verification** (checksums, signatures): happens in separate actions before compile runs.
- **Output validation** in `install_binaries`: still validates that expected files exist before registering them.

The compile actions were already running arbitrary build systems (make, cmake, meson) on downloaded source code. If an attacker controls the source tarball, the build commands themselves are the attack surface, not the post-build executable check. Removing the executable check doesn't change the threat model.

### 2. Mitigation Sufficiency

**Verdict: Sufficient.**

The design correctly identifies that `install_binaries` with its `outputs` parameter serves as the real validation that the build produced expected artifacts. The delayed error (fail at `install_binaries` instead of at compile action) is acceptable because:

- Both happen in the same install pipeline, same transaction
- No partial state is committed between the compile step and install_binaries step
- Recipe CI catches misconfigured recipes before they reach users

### 3. Residual Risk

**Verdict: No escalation needed.**

The only residual risk is a recipe author omitting `executables` from a tool recipe (not a library recipe) and getting a delayed error. This is a usability concern, not a security one. The design acknowledges it in the "Negative" consequences section.

### 4. "Not Applicable" Justifications

The design marks four security categories as "Not affected": Download Verification, Execution Isolation, Supply Chain Risks, User Data Exposure.

**Verdict: All four justifications are correct.** The change modifies post-build validation logic only. It doesn't touch download paths, sandbox configuration, source provenance, or data handling. Each justification is one sentence and accurate.

### Security Review Result: PASS (no findings)

---

## Pragmatism Review (Phase 4)

### Finding 1: Implementation step 5 is scope creep (Advisory)

**Location:** Design doc, "Implementation Approach", step 5

> Update log output: When executables is empty, adjust the "Installed N executable(s)" message at the end to say "Build completed (library only, no executables)" or similar

The stated change is "~6 lines across 3 files." Step 5 adds conditional log formatting that isn't in the acceptance criteria ("make executables optional"). The existing `Installed 0 executable(s)` message is technically correct and harmless. If someone wants prettier output, that's a separate concern.

**Recommendation:** Remove step 5 from the implementation plan or note it as optional polish. Not blocking because the impact is trivial.

### Finding 2: `Executables: []` print statement will emit empty output (Advisory)

**Location:** `configure_make.go:140`, `cmake_build.go:100`, `meson_build.go:116`

Each action prints `fmt.Printf("   Executables: %v\n", executables)` early in Execute. With an empty/nil slice, this will print `Executables: []`. Not harmful, just noisy. Same category as Finding 1 -- log cosmetics, not worth designing for.

**Recommendation:** Don't add special-case log handling. If it bothers someone later, fix it then.

### Finding 3: Design is appropriately minimal (No finding)

The core change pattern is correct and minimal:

```go
// Before:
executables, ok := GetStringSlice(params, "executables")
if !ok || len(executables) == 0 {
    return fmt.Errorf(...)
}

// After:
executables, _ := GetStringSlice(params, "executables")
```

The downstream loops (`for _, exe := range executables`) naturally become no-ops with a nil/empty slice. No new parameters, no new action types, no new abstractions. This is the simplest correct approach.

### Finding 4: No speculative generality (No finding)

The design doesn't introduce optional parameters, feature flags, or configuration that serves no current caller. It removes a restriction. Good.

### Finding 5: The "Considered Options" section is proportionate (No finding)

Two alternatives considered and rejected with one-sentence rationales each. Appropriate for a ~6 line change.

### Pragmatism Review Result: PASS (no blocking findings)

---

## Overall Recommendation

**Accept the design.** It's a clean, minimal change that removes an unnecessary restriction. The security posture is unchanged. The only advisory note is that implementation step 5 (log message polish) could be dropped to keep the change as small as claimed.
