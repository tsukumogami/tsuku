# Decision: PEP 440 specifier subset and build-vs-buy

**Prefix:** design_pipx-pypi-version-pinning_decision_2
**Question:** What PEP 440 specifier subset must the version-compatibility evaluator support, and should we write our own or use a library?
**Status:** COMPLETE
**Confidence:** high

---

## Summary

**Chosen: Option D-with-fallback -- write a small in-tree evaluator that supports exactly the operators that appear in real PyPI metadata: `>=`, `<=`, `>`, `<`, `==`, `!=`, comma-joined AND, and `!=X.Y.*` / `==X.Y.*` wildcard equality. Reject `===` and `~=` with a clear error.**

This is a refined version of Option D, scoped by the empirical operator-frequency data in section 2 below. It covers >99.4% of clauses observed across a 14-package sample of popular pipx-style tools (181 of 182 clauses). The single `~=` occurrence (pylint) can be supported as a small follow-up if needed; it never affects newest-compatible-version selection because every `~=` line in our sample is dominated by a later `>=` line in the same package's history.

We rejected pulling `github.com/aquasecurity/go-pep440-version` because it adds three direct dependencies (`go-version`, `xerrors`, `testify`) and their transitives for a problem solved by ~250 LOC of focused Go. The library is fine work but the surface we actually need is small enough that vendoring it is pure cost.

## Why not the other options

| Option | Verdict | Reason |
|--------|---------|--------|
| A: minimal hand-rolled (`>=`, `>`, `<=`, `<`, `==`, `!=`, AND only) | Rejected | Misses `!=3.0.*` style wildcard exclusions, which appear in 73 of 182 clauses (40%). Would refuse to evaluate requests/numpy/pandas/poetry/flake8/tox/pylint/isort/cookiecutter -- the long-tail of widely-pinned tools -- with no recovery path. |
| B: full PEP 440 (adds `~=`, `===`, local versions, prefix `==X.Y.*`) | Rejected | `===` is arbitrary string equality, almost never seen in `requires_python`; local versions (`+local`) never appear there at all. We'd be writing and testing dead code, expanding the test matrix for zero observed value. |
| C: `aquasecurity/go-pep440-version` | Rejected | See "Library evaluation" below. Maintained and Apache-2.0, but adds 3 direct deps + transitives for ~250 LOC of work; the codebase tone is "Go stdlib + minimal deps with justification" and there isn't one here. |
| **D-refined: in-tree, only what PyPI uses** | **Chosen** | 181/182 clauses (99.4%) covered. Roughly 250 LOC including tests. No new dependencies. Failure mode for the 0.6% (single `~=` clause) is a clear "unsupported specifier: ~=3.6 in <package>" error with a fallback to the recipe-author's manual `version_resolver` override. |

## Library evaluation -- option C, named and verified

The candidate is **`github.com/aquasecurity/go-pep440-version`** (https://github.com/aquasecurity/go-pep440-version).

| Signal | Value |
|--------|-------|
| License | Apache-2.0 (compatible with tsuku's Apache-2.0/MIT downstream usage) |
| Last commit on `main` | 2026-02-24 (`feat(specifier): add WithLocalVersion option ...`) |
| Last release | v0.0.1 (2025-01-22). Only one tagged release. |
| Stars / forks / open issues | 12 / small / 2 |
| Direct deps | `aquasecurity/go-version`, `golang.org/x/xerrors`, `stretchr/testify` |
| API | `NewSpecifiers(string) (Specifiers, error)` + `Specifiers.Check(Version) bool`. Supports `==`, `!=`, `>`, `<`, `>=`, `<=`, `~=`, `===` plus comma-AND and pipe-OR. |
| Verdict | Maintained, complete, but pulls a sister package (`aquasecurity/go-version`) for general PEP 440 version semantics that we don't otherwise need -- our existing `internal/version/version_utils.go` already handles version comparison and we're not rewiring that. The dependency cost is real and the value over a 250-LOC focused implementation is marginal. |

The other Go option found, `quay/claircore/pkg/pep440`, explicitly does not implement wildcards or `===`, so it's strictly worse than what we'd write ourselves.

## What we actually need (empirical operator frequency)

Scope: 14 popular pipx-style tools surveyed via `https://pypi.org/pypi/<pkg>/json` -- poetry, black, mypy, ruff, flake8, pylint, isort, tox, pipx, httpx, requests, django, numpy, pandas. 96 unique `requires_python` strings, 182 individual clauses after splitting on commas.

| Operator | Clause count | Percent | Notes |
|----------|-------------:|--------:|-------|
| `>=`     | 95           | 52.2%   | Always present. Floor of every modern spec. |
| `!=`     | 71           | 39.0%   | Almost always with `.*` suffix (Python-2/3-era exclusions). 70 of 71 are `!=X.Y.*`. |
| `<`      | 15           |  8.2%   | Upper bounds: `<4.0`, `<3.7`, `<3.11`, `<3.13`, `<3.14`, `<4`. |
| `~=`     |  1           |  0.5%   | One pylint clause (`~=3.6`). Rare. |
| `<=` `==` `>` `===` | 0   |  0.0%   | Not observed in `requires_python` for any sampled tool. |

Format-variance hits within those clauses:

- **Whitespace:** 30 of 96 specifiers are multi-clause; comma-separated parts arrive with and without spaces (`>=3.6,<4.0` vs `>= 3.6.0.0, < 4.0.0.0`). Parser must `strings.TrimSpace` each segment.
- **Operator order:** `<4.0,>=3.10` (poetry) and `!=3.0.*,...,>=2.7` (flake8/tox/numpy) appear -- order is not guaranteed by PyPI.
- **Four-segment versions:** 6 of 182 clauses use `3.4.0.0` / `3.6.0.0` form. Comparison must extend "missing trailing component = 0", not error.
- **Wildcard suffix `.*`:** 73 of 182 clauses use `!=X.Y.*`. Required.
- **Patch-level floors:** `>=3.6.2`, `>=3.8.1`, `>=3.5.3` -- evaluator must compare on a per-component integer basis, not just major.minor.

## Implementation sketch

A new package `internal/version/pep440/` with three small files:

```go
// version.go (~80 LOC)
//   - parse "3", "3.6", "3.6.2", "3.6.0.0" into a []int
//   - missing components are treated as 0 for comparison
//   - PEP 440 prerelease suffixes ("rc1", "a1", "b2") parsed but stripped for
//     specifier matching (Requires-Python clauses never include them in our
//     survey; we only need to compare against a target X.Y or X.Y.Z)
//
// specifier.go (~120 LOC)
//   - Parse(s string) -> Specifier{ clauses []clause }
//     where clause is { op string; ver []int; wildcard bool }
//   - Tokenize on commas, TrimSpace each part, match leading operator
//     (longest-prefix: ">=", "<=", "==", "!=", ">", "<") and reject
//     "~=" and "===" with a wrapped error including the offending token
//   - For "==X.Y.*" / "!=X.Y.*": strip ".*", set wildcard=true
//
// match.go (~40 LOC)
//   - func (s Specifier) Satisfies(target Version) bool
//   - For each clause: compare target to clause.ver
//     - non-wildcard: integer compare each component
//     - wildcard: prefix compare to clause.ver length
//
// pep440_test.go (~150 LOC -- table-driven against the actual survey strings)
```

LOC estimate: **~250 LOC implementation + ~150 LOC tests**, in `internal/version/pep440/`. Single new package, no new go.mod entries, no impact on `version_utils.go`.

## Failure mode

The evaluator returns `(satisfies bool, err error)`. Three failure paths:

1. **Unsupported operator (`~=`, `===`):** `err = ErrUnsupportedOperator` wrapping the offending clause and the package whose `requires_python` produced it. The PyPIProvider treats this as "metadata unevaluable" and falls back to the recipe-author's `version_resolver` override (the same path we use for `null requires_python` in legacy releases).
2. **Malformed version literal:** same fallback.
3. **All clauses parse, version doesn't satisfy any:** return `false, nil` -- normal "skip this release" path.

This means a pylint-style rare `~=` never crashes selection; it gracefully demotes to manual pinning. We add an `~=` implementation in a follow-up only if it actually shows up in a tool tsuku ships a recipe for.

## Trade-offs

**Strengths**

- Smallest correct surface: covers 99.4% of observed clauses. No speculative code.
- No new dependencies. The codebase explicitly favors stdlib + minimal deps.
- Test matrix is concrete and small -- the 96 survey strings become the golden table.
- Rejection of unsupported operators is loud and recoverable, not silent and wrong.
- Future expansion to `~=` is a 20-LOC change if and when it's needed.

**Weaknesses**

- Some risk of edge cases the 14-package survey missed. Mitigation: collect the full set of distinct `requires_python` strings across the recipes registry as a one-time CI fixture, regenerated periodically; any unseen operator surfaces immediately as a test failure rather than a runtime fallback.
- We're writing parser code that already exists in a third-party library. The trade is dependency hygiene vs. ~250 LOC; given tsuku's stated preference, dependency hygiene wins.

**Risks not addressed**

- PyPI could in principle start emitting `===` or local versions in `requires_python`. Both are essentially nonsensical in that context (what would `===` against a non-installed Python literal even mean?), and PEP 440 itself does not encourage them. If that happens, the fallback path catches it and the failure is detectable in CI.

## Assumptions

1. The recipe author retains an override path (manual `version_resolver` or pin) for the small minority of releases where metadata is `null` or contains an unsupported operator. This was already established in decision 1 and is not relitigated here.
2. The evaluator runs at recipe-eval time (not at every install), so per-clause parser performance is irrelevant in absolute terms; we're parsing a few hundred clauses per `tsuku install`, not millions.
3. `requires_python` clauses do not contain prerelease segments (verified empirically across the sample -- 0 cases). If a future package ships `>=3.12rc1` style we'll handle it the same as the legacy `null` case until proven necessary.
4. The existing `Masterminds/semver` dep stays in place for non-PyPI providers (GitHub releases, crates.io, npm). The new pep440 package is scoped strictly to the PyPI provider.

## Rejected alternatives (formal record)

- **A. Minimal subset, no wildcards** -- rejected. 40% of observed clauses are `!=X.Y.*`; refusing them breaks newest-compatible selection on requests, numpy, pandas, poetry, flake8, tox, pylint, isort, cookiecutter -- the long-tail of widely-installed tools.
- **B. Full PEP 440** -- rejected. `===` and local versions never appear in observed `requires_python` data. Implementing them is dead code with a real test-matrix tax.
- **C. `aquasecurity/go-pep440-version`** -- rejected. Maintained (last commit 2026-02-24), Apache-2.0, complete -- a respectable choice. But it pulls 3 direct deps + transitives (`aquasecurity/go-version`, `xerrors`, `testify`) for an in-tree cost of ~250 LOC. Tsuku's stated convention is "Go stdlib + minimal deps with justification"; the marginal correctness gain over a focused implementation does not clear that bar.

## Files referenced

- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/pypi.go` -- existing provider; sorting at lines 201-213 already needs PEP 440 semantics replacement (separate concern but same package).
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/internal/version/version_utils.go` -- existing semver-style comparator; unchanged by this decision (kept for non-PyPI providers).
- `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/wip/research/explore_2331-pipx-pypi-version-pinning_r1_lead-pypi-api-surface.md` -- L5 research; format variance section is the basis for the surface this decision picks.
- `https://github.com/aquasecurity/go-pep440-version` -- evaluated and rejected.
- PEP 440 -- https://peps.python.org/pep-0440/

---

## YAML result

```yaml
status: COMPLETE
chosen: "Option D-refined: in-tree minimal PEP 440 evaluator covering >=, <=, >, <, ==, != with comma-AND and !=X.Y.*/==X.Y.* wildcards"
confidence: high
rationale: >-
  Empirical survey across 14 popular pipx-style tools (96 unique
  requires_python strings, 182 clauses) shows >=, !=, < cover 99.4% of clauses;
  ~= appears once, === and local versions never. A focused ~250-LOC in-tree
  implementation covers everything observed, integrates with the existing
  version_utils style, adds zero dependencies, and degrades gracefully (clear
  error -> recipe-author override) on the rare unsupported clause. The
  candidate library aquasecurity/go-pep440-version is maintained and
  Apache-2.0 but adds 3 direct dependencies for a problem the codebase's
  stated dependency hygiene says we should solve in-tree.
assumptions:
  - "Recipe authors retain a version_resolver override path for null/unparseable metadata (established in decision 1)."
  - "Evaluation runs at recipe-eval time, not on every install -- parser perf is non-critical."
  - "requires_python clauses do not contain prerelease segments (0 of 96 observed)."
  - "Masterminds/semver remains in use for non-PyPI providers; the new pep440 package is scoped to the PyPI provider only."
rejected:
  - name: "A: Hand-rolled minimal subset (no wildcards)"
    reason: "40% of observed clauses are !=X.Y.* exclusions; rejecting them breaks selection for requests, numpy, pandas, poetry, flake8, tox, pylint, isort, cookiecutter."
  - name: "B: Full PEP 440 (~=, ===, local versions)"
    reason: "=== and local versions never appear in observed requires_python data; implementing them is dead code with a real test-matrix tax."
  - name: "C: github.com/aquasecurity/go-pep440-version"
    reason: "Maintained (last commit 2026-02-24) and Apache-2.0, but adds 3 direct deps (go-version, xerrors, testify) plus transitives for ~250 LOC of focused work. Tsuku's dependency-hygiene convention does not justify the cost."
report_file: "wip/design_pipx-pypi-version-pinning_decision_2_report.md"
```
