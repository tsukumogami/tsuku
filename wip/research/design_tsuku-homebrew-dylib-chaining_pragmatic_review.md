# Pragmatic Review: tsuku-homebrew-dylib-chaining (revised design)

Reviewer focus: over-engineering, dead code, scope creep. YAGNI and KISS.
One-line diagnosis, one-line fix per finding.

## Verdict

**Trim before plan.** Core fix is well-scoped; surrounding scaffolding
has gold-plating that should come out. Six trims applied; one trim
rejected (auto-include) on user-impact grounds.

## Findings (and resolutions)

### 1. `--strict` mode (scope creep) — APPLIED

- **Diagnosis.** The bug is "tools don't chain dylibs," not "we need a
  strict-mode CI gate." Adding `--strict` here is anticipating a need we
  don't have data for.
- **Fix.** Dropped from this design. Revisit only after Phase 5 produces
  real warning data.

### 2. Auto-include behavior (scope creep) — REJECTED

- **Diagnosis.** SONAME scanner + auto-include is a second mechanism
  doing the work the chain walk already does. Warning-only would force
  authors to fix recipes instead of letting them stay sloppy.
- **Decision: keep auto-include.** Round-2 empirical evidence showed
  100% of measured recipes are under-declared. Without auto-include,
  every minimal-container user gets a runtime failure on a "successful"
  install (debian/ubuntu CI passes via system shadowing). Warning-only
  is genuinely worse UX for end users. The pragmatic concern is real
  but the user-impact trade-off favors keeping the auto-include path.

### 3. Multi-valued SONAME index + collision policy (over-engineered) — APPLIED

- **Diagnosis.** No current library recipe exhibits a SONAME collision.
  Multi-valued index + auto-include disambiguation is anticipating a
  problem that hasn't occurred.
- **Fix.** Index is single-valued. On a duplicate `(platform, SONAME)`
  insert at index-build time, plan generation fails with a clear error
  pointing at both providing recipes. Loud failure replaces silent
  multi-valued handling. Simpler and equally safe.

### 4. SONAME basename regex (gold-plated) — APPLIED

- **Diagnosis.** The full regex
  `^lib[a-zA-Z0-9._+-]+\.(so|dylib)(\.[0-9.]+)?$` is over-validated for
  inputs that come from recipes the project itself authors. Recipe-name
  validator already covers traversal.
- **Fix.** Parser requires path starts with `lib/`, no `..`, basename
  starts with `lib`. Other entries skipped (they're not SONAMES).
  Trust the registry as a trust boundary; don't gold-plate validation
  against your own recipes.

### 5. All-or-nothing pre-pass validation (ceremony) — APPLIED

- **Diagnosis.** The `filepath.Join` post-check applied as a pre-pass
  over **all** entries before any patching call adds maintenance
  complexity. `work_dir` is already disposable.
- **Fix.** Per-entry validate-then-patch. Failure aborts the install;
  the disposable `work_dir` covers the partial-state concern. Same
  defense-in-depth, less ceremony.

### 6. Phase 7 (future-PR list, not work) — APPLIED

- **Diagnosis.** "Address SONAME coverage gaps" is a placeholder for
  future library-recipe PRs, not work to do in this design's scope.
- **Fix.** Deleted Phase 7. Replaced with a brief "Out of scope
  (follow-up work)" note. The Phase 2 known-gap allowlist (added per
  the architect review) handles the noise side.

### 7. Decision 4 prose re-litigates Decision 2 — REJECTED

- **Diagnosis.** "Recipe authors declaring deps explicitly is what
  `runtime_dependencies` already does."
- **Decision: keep Decision 4 as a substantive decision.** It's not
  re-litigating Decision 2 — the SONAME scan + auto-include path is
  genuinely new work (a new module, a new scanner, new install-time
  behavior). Compressing it to a paragraph would hide the design
  trade-offs. Decision 4 stays as-is structurally.

## Out of scope (correctly deferred)

- `tsuku doctor` RPATH walk (Consequences). Natural follow-up; not
  blocking.
- Authoring missing library recipes. Surfaced by the SONAME scan; one
  small PR per recipe.

## YAML summary

```yaml
outcome: trims_applied_before_plan
findings_count: 7
applied: 5
rejected: 2
rationale_for_rejected:
  auto_include: "round-2 empirical evidence shows 100% of measured recipes are under-declared; warning-only worsens minimal-container UX"
  decision_4_collapse: "SONAME scan + auto-include is substantive new work, not re-litigation of Decision 2"
```
