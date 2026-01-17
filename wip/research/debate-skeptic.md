# Skeptic Analysis: Tier 2 Dependency Validation

**Date:** 2026-01-17
**Role:** Skeptic Agent
**Verdict:** CRITICAL BLOCKERS - proposal needs revision

## Summary

The proposal solves the right problem (visibility into binary deps) but proposes an unsustainable solution (require all deps as recipes). Three critical blockers, plus detection accuracy is only ~42%.

---

## CRITICAL BLOCKERS

### 1. Pre-Built Binary Opacity

**The Problem:** tsuku downloads pre-built binaries from Homebrew and GitHub releases. These binaries have DT_NEEDED entries baked in at compile time. tsuku cannot:
- Retroactively modify DT_NEEDED entries
- Know what optional features were enabled at compile time
- Satisfy "must declare all deps" without rebuilding from source

**Example:** If upstream `ruff` was built with JEMALLOC support, it links against libjemalloc.so. When tsuku downloads the pre-built binary, that dep is embedded. You can't remove it.

**Conclusion:** "Must declare all deps" is impossible for pre-built binaries.

### 2. System Dependency Detection is Intractable

**macOS Nightmare:**
- dyld shared cache hides 200+ system libraries
- `/usr/lib/libSystem.B.dylib` vs `/usr/lib/libSystem.dylib` varies by OS version
- No algorithm can distinguish "system" from "tsuku-provided" without hardcoded registry

**Linux is Only Slightly Better:**
- Musl vs glibc: Binary can't tell you which
- Version symbols: `libc.so.6` might need GLIBC_2.30 but binary just lists `libc.so.6`
- Optional runtime deps: Binary links against libfoo but works without it

**Conclusion:** Need hardcoded registry of ~200+ system libs per OS/arch. This is maintenance burden, not algorithmic solution.

### 3. "All Deps Must Be Recipes" is Unsustainable

**Scale Problem:**
- `git` depends on: curl, openssl, zlib, expat, perl, readline, ncurses
- Each of those has transitive deps
- At 5000+ recipes, obscure libs create "bikeshedding": "I want to add tool-x but need libpsl recipe first"

**Transitive Dep Hell:**
When libpsl 0.20 → 0.21 changes libidn2 requirements, does every downstream recipe need update?

**Conclusion:** For large recipe ecosystem, requiring "all deps are recipes" creates barrier-to-entry friction.

---

## ACCEPTABLE RISKS

### dlopen() Loaded Deps

**Issue:** Dynamic loading doesn't appear in DT_NEEDED.

**Why Acceptable:** dlopen() deps are typically optional plugins. If missing, tool fails at runtime (which execution validation catches).

### Platform-Specific Deps

**Issue:** libssl.so.3 vs libssl.so.1.1, macOS frameworks vs Linux libs.

**Why Addressable:** tsuku already has `when` clauses. Validator can check deps against current platform.

### Versioned Sonames

**Issue:** Different soname versions require different recipe versions.

**Why Addressable:** Version matching at install time, not validation time.

---

## DETECTION ACCURACY

| Scenario | Detectable? | Reliable? | Actionable? |
|----------|-------------|-----------|-------------|
| Homebrew deps | Yes | ~80% | Yes |
| System deps (Linux) | Partially | ~60% | With hardcoded registry |
| System deps (macOS) | No | ~20% | Would need 200+ lib list |
| dlopen() deps | No | 0% | Skip, catch at runtime |
| Bundled deps | Maybe | ~40% | Only if recipe declares |
| Optional deps | Partially | ~50% | Only if recipe marks |

**Average accuracy: ~42%**

Below threshold for reliable validation. Execution validation (actually running tool) is more useful than static analysis.

---

## ALTERNATIVE: 3-TIER SYSTEM

### Tier 1: Informational (MVP)

- Infer DT_NEEDED/LC_LOAD_DYLIB from binaries
- Warn if deps aren't declared in recipe
- **Don't fail installation** (warning only)

### Tier 2: Validation (Enhanced)

- Cross-reference against recipe declarations
- Fail only on MISMATCH (recipe declares X, binary needs Y)
- Allow exceptions via `bundled_dependencies` field

### Tier 3: Execution (Verification)

- Run tool's `--version` or custom verify command
- Capture runtime errors indicating missing deps
- Feed back to recipe maintainers

---

## RECOMMENDATIONS

1. **Don't fail installation based on static analysis** - UX disaster
2. **Use hardcoded system library registry** - don't try to auto-detect
3. **Implement as Tier 1 first** - informational warnings
4. **Add recipe metadata for edge cases**: `bundled_dependencies`, `static_binary`
5. **Rely on execution validation** as the ultimate safety net

---

## VERDICT

**Proposal is directionally correct but operationally incomplete.**

Implement as informational system (Tier 1) first. If hard validation is needed later, you'll have better data to make that decision.

**Critical:** Warn + verify, don't fail.
