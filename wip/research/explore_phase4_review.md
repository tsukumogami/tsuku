# Design Review: Pipeline Blocker Tracking

## 1. Problem Statement Specificity

The problem statement is strong. It identifies three distinct layers of the same problem (recording gap, exit code precedence, transitive key mismatch) with concrete evidence: specific function names, exact exit codes, and the format mismatch between `"homebrew:ffmpeg"` and `"dav1d"` in the blocker map. The scope section cleanly separates what the design will and won't address.

One area that could be sharper: the problem statement says the blocker panel is "nearly empty despite hundreds of failed recipes." It would strengthen the argument to quantify this -- how many records in `data/failures/*.jsonl` have `category: "validation_failed"` but messages containing "not found in registry"? Examining actual data (e.g., `homebrew.jsonl` line 7) shows failures like `homebrew:gnupg` categorized as `validation_failed` with message "recipe gnutls not found in registry" and no `blocked_by` field. The same file shows `homebrew:jq` as `validation_failed` with "recipe oniguruma not found in registry." These confirm the problem is real and the description is accurate.

**Verdict: Specific enough.** The three-layer decomposition gives clear criteria for evaluating whether each proposed fix addresses its target.

## 2. Missing Alternatives

### Alternative not considered: Fix at `RegistryError` level

The `RegistryError` in `internal/registry/registry.go:160-164` produces the message `"recipe %s not found in registry"` using `fmt.Sprintf`. An alternative approach would be to add a structured field to `RegistryError` (e.g., `DependencyOf string`) that the dependency installation code populates. This would let `classifyInstallError` distinguish "direct recipe not found" from "dependency recipe not found" without relying on string matching at all. The design doesn't consider this option, likely because it requires changes to the error chain threading in `install_deps.go`, but it's worth at least mentioning and rejecting.

### Alternative not considered: Hybrid remediation (Go for scanning, jq for queue)

The design proposes a bash+jq remediation script but also mentions extending `cmd/batch-generate` with `--remediate`. It picks bash but doesn't explain why. Given that the orchestrator's `FailureFile` struct and the dashboard's `FailureRecord` struct are both Go types with known serialization, a Go-based scanner would handle format edge cases more reliably than jq regex matching. The bash script in the design uses `recipe \S+ not found in registry` as a regex pattern -- but jq doesn't natively support `\S`, which is a PCRE construct. The script would need to use jq's `test()` with a different regex syntax or shell-level `grep` first.

### Alternative not considered: Add `--json` to `tsuku create` as a parallel workstream

The design rejects `--json` for `tsuku create` because "it's more work than needed." This is fair for the immediate fix, but doesn't acknowledge that the regex-based extraction creates permanent coupling. An alternative would be to accept the regex fix now but open an issue tracking `--json` support for create, and note that as a future decoupling step.

## 3. Rejection Rationale Fairness

### Decision 1: "Fix only at the dashboard level"
**Fair rejection.** The rationale correctly identifies that treating symptoms at the display layer leaves the queue data wrong and forces every consumer to duplicate parsing logic.

### Decision 1: "Add --json output to tsuku create"
**Fair but could be more specific.** The rejection says "more work than needed" and "the regex extraction approach is battle-tested." Both claims are accurate -- `extractMissingRecipes()` in `cmd/tsuku/install.go:408-426` uses the same regex and has test coverage. However, the rejection should acknowledge the long-term maintenance risk of relying on error message text. The design does note this in Trade-offs Accepted, which partially addresses it.

### Decision 2: "Let data self-correct over time"
**Fair rejection.** The pipeline processes packages at most once per retry cycle with exponential backoff (24h, 72h, 144h+). Many entries won't be retried for weeks.

### Decision 2: "Fix only in dashboard generation"
**Fair rejection.** Creating a split-brain between raw data and rendered dashboard would confuse debugging.

### Decision 3: "Build a full dependency graph at queue load time"
**Fair rejection.** Recipe files aren't in the repo; they're in `$TSUKU_HOME/registry/`. The dashboard generates from committed data files.

### Decision 3: "Keep the flat (non-transitive) model"
**Fair rejection.** The user explicitly wants transitive aggregation. The design correctly notes this is the more useful metric for prioritization decisions.

**Verdict: No strawman alternatives.** All rejected options represent genuine approaches that someone might reasonably propose. The rejections are specific and focus on concrete problems rather than vague dismissals.

## 4. Unstated Assumptions

### A1: The CLI's `categoryFromExitCode` is already fixed

The design describes the CLI's `categoryFromExitCode` (in `cmd/tsuku/install.go:326-337`) as needing an exit code 3 case, but examining the actual code, it **already has** `ExitRecipeNotFound` -> `"recipe_not_found"`. The design's proposed code for the orchestrator's `categoryFromExitCode` in Section 2 of the Solution Architecture adds exit code 3 -- but the orchestrator's version (`internal/batch/orchestrator.go:456-471`) is a separate function that genuinely does lack this case. The design conflates the two `categoryFromExitCode` functions across different packages. This needs to be explicit: there are **two separate** `categoryFromExitCode` functions -- one in the CLI (`cmd/tsuku/install.go`) and one in the orchestrator (`internal/batch/orchestrator.go`). Only the orchestrator's version needs the fix.

### A2: The `classifyInstallError` reorder is necessary

The design says `errors.As` fires before the string check. Examining `cmd/tsuku/install.go:299-313`, this is exactly the current order. **However**, the test at `install_test.go:283-286` shows that `fmt.Errorf("failed to install dependency 'dav1d': registry: recipe dav1d not found in registry")` already returns `ExitDependencyFailed` with the current code. This is because `fmt.Errorf` with a format string (no `%w`) doesn't wrap the error -- so `errors.As` can't unwrap a `RegistryError` from it. The bug only manifests when the error is *wrapped* with `%w`, e.g., `fmt.Errorf("failed to install dependency 'dav1d': %w", registryErr)`. The design should verify whether the actual dependency error wrapping in `install_deps.go` uses `%w` or not, since that determines whether the reorder is actually needed.

### A3: Recipe/dependency names never contain whitespace

The regex `recipe (\S+) not found in registry` uses `\S+` (one or more non-whitespace). Looking at actual data:
- `tree-sitter@0.25` -- contains `@`, matched by `\S+`
- `openssl@3` -- contains `@`, matched by `\S+`
- `bdw-gc` -- contains `-`, matched by `\S+`

These all work. However, if a recipe name ever contains a space (unlikely given kebab-case convention), the regex would truncate it. The registry error in `internal/registry/registry.go:163` uses `%s` format which would include spaces if the name has them. The convention section of CLAUDE.md says "Recipe names: kebab-case" which rules out spaces, but this assumption should be documented in the regex comment.

### A4: The remediation script and `requeue-unblocked.sh` use compatible queue formats

The `requeue-unblocked.sh` script (line 93) reads `.packages[]` from the queue JSON, but the `UnifiedQueue` struct uses `.entries[]` (see `internal/batch/bootstrap.go:32-36`). There is also a path mismatch: `requeue-unblocked.sh` references `priority-queue-$ECOSYSTEM.json` (ecosystem-specific) while the design references `priority-queue.json` (unified). This means `requeue-unblocked.sh` is either targeting a legacy queue format or is already broken for the unified queue. The design should acknowledge this discrepancy -- the remediation script targets the unified queue format (`.entries[]`), but the existing requeue script hasn't been updated. The design mentions backward compatibility as a driver but doesn't note that `requeue-unblocked.sh` may need updates too.

### A5: Per-recipe format failures have no message field to parse

The `batch-` prefixed JSONL files (per-recipe format) contain records like:
```json
{"schema_version":1,"recipe":"watchexec","platform":"linux-alpine-musl-x86_64","exit_code":6,"category":"deterministic","timestamp":"..."}
```
These have no `message` field and no `blocked_by` field. The remediation script proposes scanning for `recipe \S+ not found in registry` in the message, but per-recipe format records don't have messages to scan. This format doesn't need remediation for blocker data because it doesn't contain dependency failure information. The design should explicitly state that remediation only targets legacy batch-format records (those with the `failures` array).

### A6: Cycle detection in transitive computation is incomplete

The proposed `computeTransitiveBlockers` function handles the `bare != dep` self-cycle case, but the memoization approach (setting `memo[dep] = 0` as an in-progress marker) doesn't prevent infinite recursion in cycles of length > 1 (A blocks B, B blocks A). The existing implementation in `dashboard.go:452-474` has the same pattern. However, looking at actual dependency chains, cycles in missing-dependency relationships are extremely unlikely (if A requires B and B requires A, neither would have a recipe). Still, the design should document this assumption or add proper visited-set tracking.

## 5. Strawman Analysis

**No options are strawmen.** Each rejected alternative represents a legitimate engineering choice:

- "Fix only at dashboard level" is a real temptation when the fix needs to ship fast
- "Add --json to tsuku create" is the architecturally cleaner approach and was likely seriously considered
- "Let data self-correct" is a valid strategy when processing is frequent (it just isn't here)
- "Build a full dependency graph" is the gold-standard approach in other package managers
- "Keep flat model" is the simplest thing that could work

The rejection criteria are specific to the project's constraints, not designed to make any option look bad.

## 6. Dual Format Handling

### Legacy batch format (FailureFile)
Records look like:
```json
{"schema_version":1,"ecosystem":"homebrew","environment":"...","updated_at":"...","failures":[
  {"package_id":"homebrew:jq","category":"validation_failed","message":"recipe oniguruma not found in registry...","timestamp":"..."}
]}
```
The dashboard's `loadFailures()` at `dashboard.go:412-423` handles this by iterating `.failures[]` and extracting `blocked_by` from each `PackageFailure`.

### Per-recipe format
Records look like:
```json
{"schema_version":1,"recipe":"pkgconf","platform":"linux-alpine-musl-x86_64","exit_code":6,"category":"deterministic","timestamp":"..."}
```
The dashboard's `loadFailures()` at `dashboard.go:427-444` handles this by checking `record.Recipe != ""`.

**The design does not explicitly address both formats.** The Solution Architecture section focuses on the orchestrator's `generate()` method (which produces legacy batch format via `WriteFailures`), and the remediation script section describes scanning for the message pattern. But:

1. The remediation script needs to handle both formats in `data/failures/*.jsonl`. The bash script's jq filter handles both via the `if has("failures") then ... elif ... end` pattern (similar to the existing `requeue-unblocked.sh`), but this isn't discussed in the design.

2. Per-recipe format records don't have a `message` field, so the regex scan won't find anything in them. This is correct behavior (per-recipe records don't represent dependency failures from the generate phase), but should be stated.

3. The `batch-` prefixed files I examined contain only per-recipe format entries with categories like `"deterministic"`. None contain dependency failure information. The legacy batch format files (`homebrew-*.jsonl`) are the ones with the misclassified dependency failures.

**Recommendation:** Add a paragraph clarifying that remediation targets only legacy batch-format records (those with a `failures` array) because per-recipe format records don't contain the error messages needed for extraction.

## 7. Remediation Script Format Handling

The design proposes a bash+jq remediation script. Key concerns:

1. **jq regex syntax.** The design says the script matches `recipe \S+ not found in registry`. jq's `test()` function uses Oniguruma regex, where `\S` is supported. This is fine.

2. **Legacy format.** The script needs to iterate `.failures[]` entries, check `.message`, and update `.category` and `.blocked_by` in-place. jq can do this but the in-place update is awkward -- jq reads from stdin and writes to stdout, so the script would need temp files.

3. **Per-recipe format.** These records lack `.message` fields, so the regex won't match. No harm, no foul.

4. **Mixed-format files.** The `homebrew.jsonl` file contains *both* legacy batch-format lines and per-recipe format lines interleaved. The remediation script must handle both gracefully -- processing each line independently and only modifying lines that match.

**Recommendation:** The design should note that `homebrew.jsonl` (the legacy single file) contains mixed-format lines and the remediation must be line-by-line.

## 8. Regex Edge Cases

The regex `recipe (\S+) not found in registry` has these edge cases in actual data:

### Version-qualified names
- `tree-sitter@0.25` appears in failure data: "recipe tree-sitter@0.25 not found in registry"
- `openssl@3` appears in failure data: "recipe openssl@3 not found in registry"
- `\S+` matches both correctly because `@` is non-whitespace.

### Kebab-case names
- `bdw-gc`, `ada-url`, `mlx-c`, `libidn2` -- all standard, all matched by `\S+`.

### Names with periods
- No examples seen in the data, but recipe names like `acme.sh` exist. `\S+` would match `acme.sh`.

### Names with slashes
- Homebrew tap formulae like `5ouma/tap/gh-poi` appear in the queue. If such a name appeared in a "not found" message, `\S+` would match the full `5ouma/tap/gh-poi`. However, these aren't dependency names -- they're top-level package IDs, so this isn't a practical concern for the regex.

### Spaces in names
- The kebab-case convention and the `%s` format in `RegistryError` mean a name with spaces would appear as `recipe some name not found in registry`, where `\S+` would only capture `some`. However, recipe names with spaces would violate the kebab-case convention and likely break many other parts of the system, so this is a theoretical concern only.

**Verdict:** The regex is safe for all realistic inputs. The `@` character in versioned dependencies is the most likely "surprising" character and it works correctly.

## 9. Additional Observations

### The orchestrator's `categoryFromExitCode` differs from the CLI's

The CLI version (`cmd/tsuku/install.go:326-337`) already includes `ExitRecipeNotFound -> "recipe_not_found"` and uses `"network_error"` for exit code 5 and `"install_failed"` as default. The orchestrator version (`internal/batch/orchestrator.go:456-471`) uses `"api_error"` for exit code 5 and `"validation_failed"` as default. The design's proposed code for "2. Orchestrator: categoryFromExitCode addition" shows different category strings than what's currently in the CLI version -- notably `"api_error"` vs `"network_error"` and `"validation_failed"` vs `"install_failed"` for the defaults. This inconsistency between the two functions is pre-existing but the design doesn't address it. Should they be unified?

### The `requeue-unblocked.sh` script uses legacy queue format

As noted in A4 above, the script uses `.packages[]` and per-ecosystem queue files (`priority-queue-$ECOSYSTEM.json`), while the unified queue uses `.entries[]` and a single file. This script may already be broken. The design lists "Backward compatibility: changes must not break existing queue processing, CI workflows, or the requeue-unblocked.sh script" as a decision driver, but the script may need its own update. At minimum, the design should note whether this script is currently functional.

### The `generate()` phase doesn't set `StatusBlocked`

The design's proposed change to `Run()` (Section 4 of Solution Architecture) adds logic to set `StatusBlocked` for generate-phase failures with non-empty `BlockedBy`. This is important because the current code at `orchestrator.go:138-145` unconditionally increments `result.Failed` and calls `o.recordFailure(idx)` for generate failures. This means blocked entries get exponential backoff applied to them, which is wrong -- they should stay blocked without backoff until their dependency is resolved. The proposed fix correctly skips `recordFailure()` for blocked entries.

### Dashboard `Blocker` struct change is a breaking JSON schema change

The current `Blocker` struct has `Count int` (`dashboard.go:122`). The proposed change replaces it with `DirectCount` and `TotalCount`. This is a breaking change for any consumer of `dashboard.json` that reads `blocker.count`. The frontend in `website/pipeline/index.html` will need updating, which the design addresses, but any other consumers (monitoring, external tools) would break. The design should note this is a breaking schema change and consider keeping `Count` as an alias for `TotalCount` during a transition period.

## Summary of Recommendations

1. **Clarify the two `categoryFromExitCode` functions.** The design conflates the CLI and orchestrator versions. Only the orchestrator's needs the exit code 3 addition.

2. **Verify whether the `classifyInstallError` reorder is actually needed** by checking whether `install_deps.go` wraps dependency errors with `%w`. If it uses string formatting without `%w`, `errors.As` already can't find the `RegistryError`.

3. **Add a note about dual format handling.** Remediation targets legacy batch-format records only. Per-recipe records don't contain messages and don't need remediation for blocker data.

4. **Address `requeue-unblocked.sh` compatibility.** The script uses `.packages[]` and per-ecosystem queue files, while the unified queue uses `.entries[]`. Either update the script as part of this work or note it as a known issue.

5. **Consider keeping `Blocker.Count` as a backward-compatible alias** for `TotalCount` to avoid breaking consumers of `dashboard.json`.

6. **Note the `homebrew.jsonl` mixed-format issue.** This file contains both legacy and per-recipe lines, and the remediation script must handle both.

7. **Document the no-spaces assumption** in the regex pattern, referencing the kebab-case convention.
