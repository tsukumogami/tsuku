# Maintainer review — fix/2440-verify-additional

Perspective: can the next developer read this, form the right mental model, and change it safely?

Overall the change is well documented for its size. Doc comments explain *why* (the "no evidence a check ran is not evidence it passed" note in `checkAdditionalVerify`, the ToTOML array-of-tables note, the validator's rationale for rejecting empty patterns and library recipes). Most of what follows is about the two implementations disagreeing on one substitution rule, and about a test that records a contract it does not actually enforce.

Note: no `Bash` tool was available in this session, so I reviewed the working-tree state of the touched files rather than the literal `git diff`. Files inspected: `cmd/tsuku/verify.go`, `cmd/tsuku/verify_test.go`, `internal/sandbox/executor.go`, `internal/sandbox/executor_test.go`, `internal/recipe/types.go`, `internal/recipe/types_test.go`, `internal/recipe/validator.go`, `internal/recipe/validator_test.go`, `internal/executor/plan.go`, `internal/executor/plan_generator.go`, `internal/executor/plan_generator_test.go`, `internal/executor/plan_cache.go`, `internal/executor/plan_test.go`, `plugins/tsuku-recipes/skills/recipe-author/references/verification-reference.md`.

---

## BLOCKING

### B1. Host and sandbox disagree on `{install_dir}` in an additional `pattern`, and the canonical doc comment describes only one of them

- `internal/recipe/types.go:862-865` — "`{version}` is substituted in both fields; `{install_dir}` is substituted in Command only, since the sandbox resolves it to a shell path that never appears in a tool's output."
- `cmd/tsuku/verify.go:237-245` — the host `subst` closure replaces **both** `{version}` and `{install_dir}`, and it is applied to `pattern` as well as `command`.
- `internal/sandbox/executor.go:536` — the sandbox replaces only `{version}` in the pattern.
- `cmd/tsuku/verify_test.go:196-201` — a test asserts that `Pattern: "{install_dir}"` matches output containing the install dir, i.e. it locks in the behavior the type comment says does not exist.

So the sentence in `types.go` is true of the sandbox and false of the host, and the host has a test written to defend the opposite rule.

What the next developer does with this: they read the type comment (the natural place to look — it is the only place both fields are described together), conclude the pattern never sees `{install_dir}`, then hit `verify.go`'s `subst` and cannot tell whether the host is the bug or the comment is. If they "fix" the host to match the comment, `TestRunAdditionalVerifications_InstallDirSubstitution` starts failing for a reason the test name does not explain. If instead they trust the host, they write a recipe with `{install_dir}` in an additional pattern; it passes `tsuku verify` locally and fails sandbox verification in CI with a message quoting a literal `{install_dir}` — the worst kind of debugging detour because the two runs disagree, not the code.

Fix, either direction, but pick one and say it in both places:
- Drop `{install_dir}` from the pattern substitution in `runAdditionalVerifications` (matches the doc, matches the sandbox, matches the recipe-author reference which already says "don't rely on it inside `pattern`"), and rewrite the test as described in B2; **or**
- Keep host behavior and change the `types.go` comment to state plainly that the host substitutes `{install_dir}` in the pattern, the sandbox does not, and therefore a recipe must not use it there.

The same asymmetry exists for the *primary* pattern (`substitutedVerifyPatterns` substitutes `{install_dir}`, `planVerifyPatterns` does not), so option one also makes the new field consistent with the pre-existing shape rather than adding a second exception.

### B2. `TestRunAdditionalVerifications_InstallDirSubstitution` passes whether or not substitution happens

`cmd/tsuku/verify_test.go:189-211`.

Positive case: `Command: "echo {install_dir}"`, `Pattern: "{install_dir}"`. If substitution is a no-op, the command echoes the literal string `{install_dir}` and the pattern is the literal string `{install_dir}` — `strings.Contains` still succeeds. The assertion is satisfied in both worlds.

Negative case: `Command: "echo <realdir>"`, `Pattern: "{install_dir}/nowhere"`. With substitution the pattern becomes `<realdir>/nowhere`, which does not match; without substitution the pattern stays `{install_dir}/nowhere`, which also does not match. The assertion is satisfied in both worlds too.

So the pair proves nothing about `{install_dir}` handling — delete the substitution from `subst` entirely and both assertions still pass. The inline comment makes this worse by claiming otherwise: "The same command with an install dir that was never substituted must fail, otherwise the passing case above proves nothing." That is a description of a control the test does not implement (the difference between the two cases is the `/nowhere` suffix, not whether substitution ran).

This matters because B1 has to be resolved by *someone reading this test as the record of intent*. As written, the test tells them the contract is enforced when it is not.

Fix: assert against the concrete value, e.g. `Command: "echo {install_dir}"` with `Pattern: installDir` (the real temp dir string) for the positive case, and drop or rewrite the comment on the negative case to say what actually makes it fail.

---

## ADVISORY

### A1. `{version}` is substituted for the additional command but not for the primary command, on adjacent lines, with no comment

`internal/sandbox/executor.go:689` vs `698-699`:

```go
verifyCmd := strings.ReplaceAll(plan.Verify.Command, "{install_dir}", installDir)
...
addCmd := strings.ReplaceAll(a.Command, "{install_dir}", installDir)
addCmd = strings.ReplaceAll(addCmd, "{version}", plan.Version)
```

Two blocks four lines apart that do almost the same thing, with one extra line on the second. Nothing says whether the primary's missing `{version}` is deliberate. It is not currently reachable (no recipe in `recipes/` uses `{version}` in `verify.command`), which is exactly why it will survive as a trap. A next developer will either assume the primary is broken and "fix" it blind, or assume the additional line is redundant and delete it — and the test comment at `internal/sandbox/executor_test.go:800-803` ("keeps the sandbox in step with the host path") nudges them toward believing both paths already behave the same.

Cheapest fix: one comment line on 698-699 noting that the primary command has no `{version}` expansion here and why (no recipe needs it / tracked separately).

### A2. "Entries run after the primary verify command passes" is not true of the sandbox

`internal/recipe/types.go:859-860` and `verification-reference.md:120`. In the sandbox the commands are emitted unconditionally into the script under `set +e` (`internal/sandbox/executor.go:693-702`); only the *evaluation* of their markers is gated on the primary passing (`readVerifyResults:496-502`). The script comment gets this right; the type doc and the author-facing reference do not.

A maintainer who believes the ordering guarantee could reasonably declare a check with a side effect, or spend time explaining why an additional command's output shows up in a container log for a run whose primary check failed. Suggested wording: "results are evaluated only after the primary check passes; the sandbox runs every entry and evaluates the markers afterwards."

### A3. `checkAdditionalVerify` returns a failure string, not a bool or error

`internal/sandbox/executor.go:513`. The `check`-prefixed name with an untyped `string` return where `""` means success is a sentinel the caller has to know about. The doc comment does explain it, but the name works against the comment. `additionalVerifyFailure(...) string` would say it in the signature. Also worth noting the pair `runAdditionalVerifications` (plural) / `checkAdditionalVerify` (singular) both operate on the whole list — the pluralization difference reads as if one handles a single entry.

### A4. `planAdditionalVerify` and `PlanAdditionalVerify` differ only in leading case, in the same package

`internal/executor/plan_generator.go:20` and `internal/executor/plan.go:112`. Legal Go, but in review diffs and grep output the converter and the type are indistinguishable at a glance. `toPlanAdditionalVerify` or `newPlanAdditionalVerify` costs nothing and removes the ambiguity.

### A5. `"additional verification N did not run"` claims a cause the code cannot know, and the number does not match the marker filename

`internal/sandbox/executor.go:520`. All the function knows is that it could not read `.sandbox-verify-additional-<i>-exit`. "Did not run" points the reader at the recipe command; the cause could equally be a container that died before reaching that line or an output-mount problem. Naming the marker file in the message ("no marker `.sandbox-verify-additional-0-exit` in the output dir") shortens the trip.

Related: messages are 1-based (`i+1`) while marker files are 0-based, so someone told "additional verification 2 did not run" will go looking for `-2-` and find `-1-`. Also, the non-zero-exit message at line 534 is the only one of the four that omits the index — worth adding for consistency.

### A6. The user-facing guide still tells authors to verify only the primary binary

`docs/guides/GUIDE-recipe-verification.md:181-190`, section "Multiple Binaries": "When a recipe installs multiple binaries, verify the primary one." That is the exact heading a recipe author will land on now that the feature works, and it gives the pre-fix answer. (The section also uses `[verification]` rather than `[verify]`, which was already wrong.) The new prose went only into `plugins/tsuku-recipes/skills/recipe-author/references/verification-reference.md`, which is the agent-facing reference.

### A7. `tsuku verify --help` does not mention additional checks

`cmd/tsuku/verify.go:1074-1080`. The Long text enumerates what verification does ("1. Running the recipe's verification command ...") and is now incomplete for both visible and hidden tools. One clause is enough.

### A8. A stray `exit_code` inside `[[verify.additional]]` is silently ignored

The "no exit_code override" contract is stated in the type comment and the reference doc but is not enforceable: nothing in the repo rejects unknown TOML keys, so an author who writes `exit_code = 1` under `[[verify.additional]]` gets a check that fails with a message about a non-zero exit and no hint that the field was dropped. This is a repo-wide limitation rather than something this PR introduced, and `validateAdditionalVerify` is the natural place to add a targeted check later if it ever bites.

### A9. `PlanVerify` → `VerifySection` reconstruction drops `Additional`

`internal/executor/executor.go:473-486` builds a `recipe.VerifySection` from `plan.Verify` and copies only Command/Pattern/Patterns — no `ExitCode` (pre-existing) and now no `Additional`. Nothing reads `Additional` off that value today, so this is not a live bug, but the plan→recipe direction is now lossy in a way the recipe→plan direction (`planAdditionalVerify`) is not. A next developer who assumes the round trip is faithful will be wrong. Either copy both fields or add a line saying this context object exists only for action metadata.

### A10. Two test comments overstate what they cover

- `internal/executor/plan_generator_test.go:557-559` — "guards the regression this test file exists to prevent". `plan_generator_test.go` exists for plan generation generally; the sentence reads like the file was created for this issue.
- `internal/recipe/types_test.go:1294-1296` — "covers the round-trip that feeds a recipe into container validation" is accurate as far as I could trace it (`internal/validate/executor.go:311-313` copies the recipe into the container so that `tsuku install`'s post-install verification, which does run additional checks via the loaded recipe, can find it), but the chain is three hops long and undocumented at either end. A pointer to `buildPlanInstallScript` in that comment would save the next reader the trace.

---

## Things that read well

- The validator's reasons for rejecting an empty pattern and for rejecting `verify.additional` on library recipes are written as "why", not "what", and the tests restate them in the same terms.
- Per-entry marker pairs with a single `additionalVerifyMarkers` helper used by production code *and* the test helper — there is no second place for the filename convention to drift.
- Adding `Additional` to `verifyForHash` and bumping `PlanFormatVersion` to 6 closes the stale-cache path, and `TestPlanContentHash_AdditionalVerify` documents exactly why.
- Attributing the dangerous-pattern warning to `verify.additional[i].command` rather than `verify.command`, with a test that asserts the *absence* of the misattributed warning.
- Style conventions hold: no emojis, `$TSUKU_HOME` used in comments, none of the discouraged vocabulary in the new prose.
