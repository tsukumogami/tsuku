# Scrutiny review — JUSTIFICATION focus

Commit under review: `e1a3fdf5` on `fix/2440-verify-additional`
Issue: tsukumogami/tsuku#2440
Question: are the deviations and choices genuinely explained, and do the stated
reasons reflect real trade-offs rather than shortcuts?

Note on scope: the worktree has uncommitted modifications to
`internal/executor/plan_cache.go`, `internal/recipe/types.go`,
`internal/recipe/validator.go`, `internal/sandbox/executor.go`,
`plugins/tsuku-recipes/skills/recipe-author/references/verification-reference.md`
(and several test files) that were still changing while this review ran. Those
edits already address two of the findings below. This review is of the commit as
committed; where a working-tree edit remediates a finding, that is called out.

---

## Verification of stated claim 1 — "no parse path checks MetaData.Undecoded()"

**Verdict: TRUE, and stronger than stated.**

`grep -rn "Undecoded" --include=*.go .` returns nothing anywhere in the repo.
Every recipe parse site uses `toml.Unmarshal`, which silently discards keys that
do not map to a struct field:

- `/home/dgazineu/dev/niwaw/tsuku/tsuku+verify_additional_fix-da229b04/public/tsuku/.claude/worktrees/issue-2440-verify-additional/internal/recipe/loader.go:344`
- `.../internal/recipe/loader.go:686`
- `.../internal/recipe/validator.go:78`
- `.../internal/recipe/embedded.go:89`
- `.../internal/recipe/provider_unified.go:70`, `:158`, `:240`

Two other TOML readers touch recipes but decode into reduced structs and would
not help either: `internal/index/rebuild.go:235` (`toml.Unmarshal` into
`recipeMinimal`) and `internal/batch/bootstrap.go:222` (`toml.Decode` — it does
capture a `toml.MetaData`, but only to call `PrimitiveDecode` on steps; it never
calls `Undecoded()`).

There is also no out-of-band schema that would catch the key: no JSON schema for
recipes exists (`data/schemas/` holds only priority-queue and failure-record
schemas), and neither `scripts/` nor `.github/` contains any unknown-key check.
`go vet`-style lint (`lint_test.go`) does not cover TOML keys either.

So the author's factual claim holds without qualification: delete
`VerifySection.Additional` today and `[[verify.additional]]` becomes a no-op key
that parses clean, which is a strictly worse outcome than the current bug —
issue #2440's own removal branch says "the schema field should be dropped so an
existing recipe using it fails loudly", and nothing in the codebase can make that
happen.

**Advisory nuance (A1 below):** the claim is true about the code as it stands, but
it is phrased as though removal were impossible rather than as though removal
would require an additional, larger change (teaching `ValidateBytes` to call
`MetaData.Undecoded()` and rejecting unknown keys). That larger change is a real
cost — it would need an audit of every recipe in `recipes/` for stray keys and
would change validation behavior repo-wide — but the commit message does not say
so, and a reader could come away thinking there is no removal path at all.

## Verification of stated claim 2 — "documented for multi-binary verification, a case patterns cannot cover"

**Verdict: both halves TRUE.**

The reference doc did document the field before this commit.
`git show e1a3fdf5^:plugins/tsuku-recipes/skills/recipe-author/references/verification-reference.md`
contains an "Additional Verification Commands" section headed "When a tool
installs multiple binaries, verify each one:" with a `black`/`blackd` example.
So the field is not an undocumented vestige; recipe authors are actively pointed
at it. Removing it would have required a doc change too, which the commit's
framing implies.

`patterns` genuinely cannot express the multi-binary case.
`substitutedVerifyPatterns` (`cmd/tsuku/verify.go:196`) builds the pattern list
from `VerifySection`, and `matchVerifyPatterns` (`:224`) does
`strings.Contains(output, p)` against the output of the single `Verify.Command`.
There is no second command anywhere in that path.

The one loophole worth naming — and the commit does not name it — is that
`Verify.Command` runs through `exec.Command("sh", "-c", command)`
(`cmd/tsuku/verify.go:309`, `:392`), so an author could in principle write
`command = "black --version && blackd --version"` with `patterns = ["black",
"blackd"]`. That loophole is closed in practice by the validator:
`validateDangerousPatterns` (`internal/recipe/validator.go:718`) emits a warning
for `&&`, and `tsuku validate --strict` treats warnings as errors
(`cmd/tsuku/validate.go:138`), which is the mode CI and the registry index build
use. So the workaround exists but is rejected by the project's own gate. The
claim survives; the argument would have been more convincing had it said so.

---

## BLOCKING findings

### B1. The commit's own documentation states a placeholder-substitution rule the sandbox path does not implement

The commit adds two statements of the same rule:

- `internal/recipe/types.go` godoc on `AdditionalVerify`: "`{version}` and
  `{install_dir}` are substituted in both fields."
- `verification-reference.md`: "`{version}` and `{install_dir}` are substituted
  in both fields."

That is true only of the host path. In the sandbox path as committed:

- `buildSandboxScript` (`internal/sandbox/executor.go:696`) does
  `addCmd := strings.ReplaceAll(a.Command, "{install_dir}", installDir)` —
  `{version}` is **not** substituted in the command.
- `checkAdditionalVerify` (`internal/sandbox/executor.go:513`) does
  `pattern := strings.ReplaceAll(a.Pattern, "{version}", plan.Version)` —
  `{install_dir}` is **not** substituted in the pattern.

So the two paths substitute complementary halves and the doc describes neither.
A recipe author following the reference this commit ships writes
`command = "toold --version {version}"` or `pattern = "{install_dir}/bin/toold"`,
sees it pass on the host, and sees it fail only in sandbox verification (CI) with
a message about a literal `{version}` string. The failure is loud rather than
silent, so this is not a repeat of #2440's silent-skip class, but the
justification "run each entry after the primary check on both verification
paths" reads as though the two paths are equivalent, and the doc claim is simply
false for one of them.

Remediation status: the working tree already adds
`addCmd = strings.ReplaceAll(addCmd, "{version}", plan.Version)` to
`buildSandboxScript` and rewrites both doc statements to say `{install_dir}` is
substituted in `command` only. Fold into the commit and the finding clears.

### B2. Library recipes can still declare an additional check that never runs, which is the exact failure mode the issue asks to eliminate

`validateVerify` (`internal/recipe/validator.go:663`) returns early for
`RecipeTypeLibrary` before reaching the new `validateAdditionalVerify` call, so
the new "command and pattern are required" checks never fire for a library
recipe. Library verification does not go through `runHiddenToolVerification` or
`runVisibleToolVerification` at all — it goes through `verifyLibrary`
(`cmd/tsuku/verify.go:641`), which does dlopen/header/dependency checks and never
reads `r.Verify`. Plan generation also drops it: `GeneratePlan` only builds a
`PlanVerify` when `Verify.Command != ""`, which library recipes generally leave
empty.

Result: `type = "library"` plus `[[verify.additional]]` validates clean, installs
clean, verifies green, and the declared check never executes. That is issue
#2440's acceptance criterion 4 ("a recipe cannot end up in a state where a
declared verification check is silently skipped") unmet, in a narrower shape,
while the commit says `Fixes #2440`.

Remediation status: the working tree adds an explicit
`result.addError("verify.additional", "library recipes do not run verification
commands; remove verify.additional")` inside the library early-return, plus a doc
paragraph. Fold in and the finding clears.

---

## ADVISORY findings

### A1. The "implement rather than remove" argument understates that removal is possible, just more expensive

Covered above. The commit says removal "would make the key silently discarded
rather than rejected," which is true of today's code but reads as a hard
constraint. The honest version is: removal would additionally require adding
`MetaData.Undecoded()` rejection to the validator, which is a repo-wide
validation behavior change needing an audit of existing recipes — a real cost,
and a better argument than an implied impossibility. Not misleading enough to
block; the conclusion (implement) is the right one either way, reinforced by the
documented multi-binary use case.

### A2. The plan-format bump is necessary, but the cache story is presented as more complete than it is

Necessary: yes. The install path builds its cache key with
`executor.CacheKeyFor` (`cmd/tsuku/install_deps.go:94`), which leaves
`ContentHash` empty, and `ValidateCachedPlan`
(`internal/executor/plan_cache.go:56`) skips the content-hash comparison when the
hash is empty. Format version and platform are therefore the *only* validity
checks on that path, so bumping to 6 is the one mechanism that flushes v5 plans
that predate the field. Without the bump a previously-installed tool would keep
running a plan with no `additional` entries.

Cost statement: "which costs a regeneration" is literally true and is not hiding
anything large — plan regeneration is a network operation, but version
resolution "ALWAYS runs" on that path anyway, so the marginal cost is one plan
generation per installed tool/version, once.

What is not stated: the bump only flushes the cache once. Because the install
path never supplies a `ContentHash`, a recipe that *later* gains an
`[[verify.additional]]` entry at an already-cached version will keep validating
its stale v6 plan, and the new check will be dropped from the sandbox path
exactly as before. This is a pre-existing property of the cache design (the same
is true today of a changed `pattern` or `exit_code`), not something the commit
introduced, which is why this is advisory rather than blocking — but the commit
message's cache paragraph reads as though the invalidation question is fully
settled by the bump, and it is not.

Related, and worth noting because it interacts: as committed, `verifyForHash`
(`internal/executor/plan_cache.go:145`) does not include `Additional`, so two
plans differing only in their additional checks hash identically. That does not
affect the install path (which never computes the hash) but it does affect any
caller that uses `CacheKeyWithHash`. The working tree already adds `Additional`
to `verifyForHash` and to both `planContentForHashing` and
`convertDepsForHashing`.

Also worth flagging for scope honesty: `plan.Verify.Additional` is consumed only
by `internal/sandbox/executor.go`. Nothing on the host install/verify path reads
the plan's verify section — `cmd/tsuku/verify.go` reads `r.Verify` from the
recipe directly. So the format bump invalidates every user's cached plans to
carry a field whose only consumer today is sandbox verification, for a feature no
registry recipe currently uses (`grep -rln "verify.additional" recipes/` returns
nothing). Still the right call — a stale plan feeding the sandbox is a real
regression vector once a recipe adopts the field — but the commit message does
not disclose the asymmetry.

### A3. Requiring both `command` and `pattern` as errors is sound and consistent, but the ergonomic cost is unstated

The reasoning is correct on the mechanics: `strings.Contains(output, "")` is
always true, so an empty pattern reduces the entry to "the command exited 0"
while reading like a substantive assertion. That is the same silent-overclaim
shape as the bug being fixed, so error-not-warning is the consistent choice —
and it matches adjacent precedent added for `patterns` in the same file
(`internal/recipe/validator.go:679-694` errors on `patterns = []` and on empty
entries for identical reasons).

Compatibility cost is genuinely zero: no recipe in `recipes/` uses
`verify.additional`, so no existing recipe is broken by the stricter rule. The
commit does not say this, and it is the fact that makes error-rather-than-warn
safe rather than merely opinionated — worth one sentence.

The unstated trade-off is that there is now no way to express "this second binary
just needs to run and exit 0." The primary check has an escape hatch for that
shape (`mode = "output"` with a `reason`); additional entries have no analogue,
and no `exit_code` override either. That is a defensible narrowing, but it is a
narrowing, and the commit presents it purely as a safety win.

### A4. Dangerous-pattern warnings on additional commands are attributed to the wrong field

`validateAdditionalVerify` calls `validateDangerousPatterns(result, a.Command)`,
and that function hardcodes `result.addWarning("verify.command", ...)`
(`internal/recipe/validator.go:718-770`). So a `$(...)` or `&&` inside
`verify.additional[2].command` surfaces as a `verify.command` warning pointing at
a command that does not contain it. This is minor, but it sits directly against
the commit's stated goal that "`tsuku validate` also lists the additional
commands so the summary shows everything that will run" — the listing is right,
the diagnostics attribution is not.

### A5. Running additional checks only after the primary check passes — reasoning is sound

The stated reason is failure attribution: "so the error a user sees names the
additional check rather than looking like a plain version-check failure." That
holds up. Nothing is silently skipped by the ordering, because a failed primary
check already fails verification; the only lost information is that a user fixing
a primary failure will not simultaneously see additional failures, which is a
normal fail-fast trade-off.

Worth noting that in the sandbox the additional commands still *run*
unconditionally inside the container (`buildSandboxScript` emits them
unguarded) and only their *evaluation* is gated host-side in `readVerifyResults`.
So the gating buys error attribution, not execution savings — which is exactly
what the commit claims, and the comment in `buildSandboxScript` ("the sandbox
script stays a plain recorder") says so. Consistent. No finding.

### A6. The `plan_cache_test` rewrite is legitimate, not a weakened guard

The changed assertion is in `TestValidateCachedPlan`'s "outdated format version"
case, whose purpose is to prove a v1 plan is rejected — not to pin the constant.
The substantive half of the expectation ("plan format version 1 is outdated")
still asserts real behavior; only the `(current: N)` tail is now interpolated
from `PlanFormatVersion`.

The deliberate guard on the number itself is `TestFormatVersionConstant`
(`internal/executor/plan_test.go:314`), which still hardcodes the literal — the
commit updates it from `!= 5` to `!= 6` and adds a matching history comment. So a
future bump still has to be an explicit, reviewed edit in exactly one place. The
justification given in the test comment is accurate and checkable. No finding.

---

## Summary of counts

- BLOCKING: 2 (B1 placeholder-substitution doc claim false on the sandbox path;
  B2 library recipes retain the silent-skip hole). Both are already fixed in
  uncommitted working-tree edits; remediation is to fold them into the commit.
- ADVISORY: 4 (A1 removal framed as impossible rather than costlier; A2 cache
  story presented as more complete than it is, plus undisclosed
  sandbox-only consumption; A3 unstated ergonomic narrowing and unstated
  zero-compatibility-cost fact; A4 dangerous-pattern warnings misattributed).
  A5 and A6 were examined and cleared.

Both headline claims in the commit message check out against the code. The
justification prose is unusually specific and mostly earns its confidence; where
it fails, it fails by presenting a partial mechanism as a complete one rather
than by asserting anything untrue.
