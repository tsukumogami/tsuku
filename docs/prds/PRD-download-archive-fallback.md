---
schema: prd/v1
status: In Progress
problem: |
  A tsuku recipe can name exactly one download host, and download_archive
  fetches the whole archive at plan time to compute the checksum it pins. So
  an unreachable host fails `tsuku eval`, not merely `tsuku install`, and
  because zig is an implicit build dependency of three build actions, one dead
  zig host turns Validate Embedded Dependencies red on every PR that touches
  recipes/. The only repair is a maintainer editing a recipe, which does not
  reach plans already published to R2.
goals: |
  A recipe author can declare an ordered list of hosts serving the same bytes.
  Plan generation and install both fall through the list until one answers, so
  a single host outage stops being a repo-wide CI event and stops being a
  dead end for anyone installing from a published plan. Recipes that name a
  single host behave exactly as they do today, byte-for-byte, including their
  golden plans.
upstream: docs/briefs/BRIEF-download-archive-fallback.md
source_issue: 2443
motivating_context: |
  Third round of the same outage. #2384 replaced ziglang.org with
  zigmirror.hryx.net; #2441 replaced hryx with ziglang.freetls.fastly.net.
  Both rounds named multi-source fallback as the real fix and deferred it. The
  first deferral was recorded only in PR #2391's description and never filed,
  which is why the second outage landed on another single-host swap.
---

# PRD: Multi-source fallback for archive downloads

## Status

In Progress

Requirements for issue #2443, framed by
`docs/briefs/BRIEF-download-archive-fallback.md`. The downstream design doc
owns where the list lives in the action funnel, the plan format, and the
download cache.

## Problem Statement

Every tsuku recipe that downloads an archive names exactly one host. The
recipe schema gives `download_archive` and its three siblings a single `url`
string, and nothing in `internal/actions/` or `internal/recipe/` accepts a
list. Whoever wrote the recipe chose the host once, and changing it is a code
change that has to be reviewed and merged.

The cost of that host going down is larger than it looks, because
`download_archive` does not defer the download to install time. It fetches the
whole archive during plan generation so it can compute the checksum it pins
into the plan. An unreachable host therefore fails `tsuku eval`. zig is an
implicit build-time dependency of `cmake_build`, `configure_make` and
`meson_build`, so a dead zig host fails plan generation for every recipe that
reaches any of those — which means `Validate Embedded Dependencies` goes red on
pull requests that have nothing to do with zig, and stays red until someone
lands a recipe edit.

The published-plan case is worse, because the repair does not reach it.
`scripts/r2-upload.sh` publishes generated plans to R2, and sandbox and
validation images run `tsuku install --plan` against those files weeks later.
A plan generated before an outage still names the dead host. The consumer
cannot regenerate it and cannot edit the recipe; editing the recipe does not
rewrite anything already published.

This has now happened twice in the same spot, and each round narrowed the
failure without removing it. #2384 moved zig off an intermittently unreachable
`ziglang.org`. #2441 moved it off a `zigmirror.hryx.net` that had gone away
entirely. The current host is a CDN rather than a bare origin, which is a real
improvement in blast radius, but it is still one URL and one operator, and the
recipe comment in `internal/recipe/recipes/zig.toml` says so in as many words.

## Goals

- A recipe author can name more than one host for the same archive, in
  preference order, in the recipe itself.
- A host being unreachable during plan generation costs nothing visible: the
  next host serves, and the plan that comes out is the one the first host
  would have produced.
- A plan published to R2 carries the same fallback the maintainer had, so the
  consumer who cannot regenerate or edit anything is not the one left with a
  single point of failure.
- Recipes that name one host are untouched — same validation, same generated
  plan bytes, same install path — so the change is reviewable and does not
  launder unrelated drift into the diff.
- The trust model does not widen. A pinned checksum is still the thing that
  decides whether bytes are acceptable, and it is checked identically no
  matter which host answered.

## User Stories

**As a recipe author** for a tool whose upstream publishes a mirror list, I
want to record the hosts I trust in preference order, so that the recipe stops
needing an edit every time one of them has a bad week — and so that a typo in
the third entry is caught when I write it, not during an outage two months
later.

**As a contributor** opening a pull request that touches `recipes/`, I want
plan generation to survive one host being unreachable from the GitHub runner,
so that my unrelated change is not blocked behind someone else's mirror outage.

**As someone installing from a published plan** in a sandbox or validation
image, I want the plan to carry every source the recipe declared, so that a
host that has died since the plan was generated does not leave me with a dead
end I have no way to repair.

**As a maintainer** triaging a report that one of a tool's hosts is failing, I
want the report to arrive without a red build attached, so that I can decide
whether the remaining hosts are healthy enough and land any change on a normal
schedule.

**As a reviewer** of the pull request that introduces this, I want single-source
recipes to produce byte-identical plans, so that the diff shows me the recipes
that actually changed rather than a repo-wide golden regeneration I cannot
read.

## Requirements

### Functional

- **R1.** A recipe step using `download_archive` can declare an ordered list of
  alternate download URLs in addition to its `url`. The alternates use the same
  placeholder expansion (`{version}`, `{os}`, `{arch}`) and the same
  `os_mapping` / `arch_mapping` as `url`.

- **R2.** The list is ordered and the order is honoured. `url` is attempted
  first; alternates are attempted in declaration order. Nothing reorders,
  probes, ranks, or remembers which entry answered last time.

- **R3.** During plan generation, when an attempt fails after that source's own
  retries are exhausted, the next source is attempted. Plan generation succeeds
  if any source serves the archive, and fails only when every source has been
  attempted and failed.

- **R4.** Failure over the whole list reports every source that was tried and
  why each one failed, so a maintainer triaging from CI logs alone can tell an
  outage from a retention gap from a typo.

- **R5.** The generated plan records the full ordered source list, not the
  single source that answered.

- **R6.** During install, a `download_file` step carrying a recorded source
  list falls back across that list the same way plan generation does, and
  verifies the pinned checksum identically regardless of which source served
  the bytes.

- **R7.** A `download_file` step carrying no source list — an older plan, or a
  single-source recipe — behaves exactly as it does today.

- **R8.** The same mechanism is available to the sibling actions that share the
  download funnel with `download_archive` (`github_archive`, `github_file`,
  `fossil_archive`), so the funnel does not have to be forked to serve one
  caller.

- **R9.** Recipe validation and action preflight apply to every declared
  source, not only to the first. A validation rule that holds for `url` — HTTPS
  scheme, well-formed URL, `{version}` placeholder expectations — holds for
  every alternate.

- **R10.** The download cache does not miss because of fallback. Bytes cached
  under one source in a list are found when a later attempt resolves to a
  different source in the same list.

- **R11.** `internal/recipe/recipes/zig.toml` uses the mechanism, with the
  current `ziglang.freetls.fastly.net` source first and at least one alternate
  behind it.

- **R12.** Golden-plan handling for multi-source recipes is documented in the
  repo, so the next person to regenerate a golden knows what the extra field is
  and when it appears.

### Non-functional

- **R13.** A recipe declaring a single source produces a byte-identical plan to
  the one it produces today. Every existing golden file that records a download
  URL is unchanged except those for recipes this PRD converts.

- **R14.** No additional network request is made when the first source answers.
  Fallback costs nothing in the common case.

- **R15.** The trust model is unchanged. Every source must serve byte-identical
  content for the one pinned checksum; a source serving different bytes fails
  checksum verification exactly as a corrupted download does today. Fallback
  never relaxes, skips, or defers a checksum check.

- **R16.** Where an upstream-published checksum is available, verification
  against it is applied independently of which source served the archive, so
  the widened set of hosts does not widen what plan generation will accept.

## Acceptance Criteria

- [ ] A recipe can declare more than one download source for `download_archive`
      and the recipe validator accepts it.
- [ ] Plan generation succeeds when the first declared source is unreachable
      and a later one is not, and the resulting plan is identical to the one
      generated when the first source is reachable.
- [ ] Plan generation fails only when every declared source fails, and the
      error names each source tried and the reason it failed.
- [ ] The generated plan for a multi-source recipe contains every declared
      source in declaration order.
- [ ] Install from a plan whose first recorded source is unreachable completes
      by falling through to a later recorded source, and the pinned checksum is
      verified.
- [ ] Install from a plan that records no source list behaves exactly as it
      does today.
- [ ] A source serving bytes that do not match the pinned checksum fails the
      install; fallback does not mask it.
- [ ] A malformed alternate source (non-HTTPS, unparseable, missing a
      `{version}` placeholder where `url` has one) is reported by
      `tsuku validate` against the recipe.
- [ ] A single-source recipe's generated plan is byte-identical to the golden
      currently in `testdata/golden/plans/`, verified by
      `scripts/validate-all-golden.sh` passing with no golden regenerated
      except for recipes this work converts.
- [ ] `internal/recipe/recipes/zig.toml` declares `ziglang.freetls.fastly.net`
      first with at least one alternate, and the zig and ninja goldens under
      `testdata/golden/plans/embedded/` reflect it.
- [ ] `Validate Embedded Dependencies` passes on the pull request.
- [ ] A cache entry populated while one source served is a cache hit when a
      later run falls through to a different source in the same list.
- [ ] `go test ./...` passes, with new tests covering: fallback to a later
      source, exhaustion of the whole list, single-source behaviour unchanged,
      checksum mismatch on an alternate, and cache reuse across sources.
- [ ] Golden-plan handling for multi-source recipes is documented.

## Out of Scope

- **Zig's community-mirror protocol.** Fetching `community-mirrors.txt` at plan
  time, ranking entries, and falling back to canonical is zig-specific and
  generalizes to nothing. A per-recipe list covers zig and every other recipe.
- **A hostname-keyed mirror registry inside the CLI.** Which host serves a
  project's archives is knowledge the recipe owns; a Go-side registry puts host
  policy where recipes cannot see it.
- **The R2 golden publish/consume pipeline (#2448).** Registry goldens are
  currently published to a layout no consumer reads, so nothing validates them.
  Real and related, deliberately excluded — and this work must not lean on that
  pipeline as a safety net.
- **Removing the plan-time download entirely.** `Decompose` fetches the whole
  archive only to compute a checksum the goldens already pin, and `--pin-from`
  does not pin it. Teaching plan generation to trust an already-pinned checksum
  would make an unreachable source harmless with zero fallback. Cheaper, more
  targeted at the CI symptom, and a different piece of work.
- **Re-litigating zig's first source.** Fastly was chosen in #2441 with CI
  evidence of reachability from GitHub runners. This work makes the
  single-source question moot rather than reopening the choice.
- **Automatic source discovery, health probing, or latency ranking.** All
  would make plan generation depend on network conditions, which is exactly
  what the pinned-checksum model exists to avoid.
- **Adding minisign verification.** zig publishes `.minisig` rather than a
  `.sha256` sidecar, so anchoring zig's plan-time checksum upstream would mean
  building minisign support. Worth doing; not this issue.

## Decisions and Trade-offs

### The source list is a new field alongside `url`, not a widened `url`

The upstream BRIEF left open whether the ordered list arrives as a new
parameter or as a `url` that accepts either a string or an array. Decided: a
new field, with `url` unchanged as the first source.

The alternative reads better in a recipe — one concept, one field — but it
changes the type of a parameter that is read as a string in a dozen places
(`GetString(params, "url")` in the actions, `actionVersionRules`'s
`{version}`-placeholder expectation, `validateURLParam` in the recipe
validator, the plan generator's URL extraction). Each of those would need a
string-or-array branch, and every one is a place a future reader could get it
wrong. Keeping `url` a string means the list is purely additive: absent, and
nothing anywhere behaves differently.

### The plan records the whole list, not the source that answered

The BRIEF deferred this too, and it drives most of the diff. Recording only the
winner keeps plans trivially deterministic; recording the list keeps
install-time fallback alive after publication.

Decided: record the list. Plans are a published artifact, not an intermediate —
they go to R2 and get installed from weeks later. Recording the winner leaves
the plan single-sourced at exactly the moment the consumer has no recourse, and
moves the failure from plan generation (where a maintainer can regenerate) to
install-from-published-plan (where nobody can do anything). The determinism
objection does not survive contact with the code: tsuku's reproducibility
invariant is already checksum identity, not URL identity, and `download_file`
requires and verifies a pinned checksum. Every source must serve byte-identical
content for the one pinned checksum — a condition that nests inside the
upstream mirror requirement rather than adding a new one.

### The list appears in a plan only when a recipe declares one

Not asked upstream, but it decides whether this change is reviewable. If every
plan grew a source list, every golden that records a download URL would need
regenerating — and `scripts/regenerate-golden.sh` cannot use `--pin-from` for
registry recipes, so a blanket regeneration re-resolves dependencies to latest
and churns unrelated checksums into the diff.

Decided: the field is emitted only when the recipe declares alternates. Every
single-source golden stays byte-identical and the diff is confined to the
recipes this work actually converts.

### Install-time fallback is identical to plan-time, not best-effort

The BRIEF asked whether best-effort was acceptable. Decided: identical. The
published-plan journey is the one the user cannot repair, so it is the last
place to accept a weaker guarantee. Plans that record no list — older tsuku, or
single-source recipes — are not a degraded case; they behave exactly as today.

### Upstream-anchored verification is bounded to "keeps working", plus a warning

The architectural review on #2443 argued that anchoring the plan-time checksum
to an upstream-published value belongs in this issue's scope, on the grounds
that a source list widens the plan-time trust set to whichever host answered
first, and that minisign machinery already exists in
`internal/actions/download.go`.

The concern is right and R16 carries it: where an upstream checksum is
available, it is applied whichever source answered. Mandatory anchoring is not
taken, for two checkable reasons. The machinery cited does not exist — that
file carries PGP signature verification, and `grep -rn minisign internal/
--include=*.go` returns nothing. And zig, the one recipe the acceptance
criteria require converting, publishes no `.sha256` sidecar at the mirror paths
this recipe uses (`zig-x86_64-linux-0.14.1.tar.xz.sha256` returns 404;
`.minisig` returns 200), so a mandatory anchor would block the acceptance
criteria on building minisign support first. Anchoring is instead surfaced as a
preflight warning when a recipe declares alternates without one, and minisign
is named in Out of Scope as the follow-up that would let zig satisfy it.

## Known Limitations

- Fallback does not help when every declared source is down, and a recipe with
  a list of hosts that are all fronted by the same origin is closer to one
  source than its length suggests. The zig recipe comment already records this
  for Fastly specifically; a source list makes it easier to add entries but
  does not make them independent.
- A source that has dropped an old release while still serving current ones
  produces a 404 that fallback will paper over on the way to a working source.
  That is the desired behaviour, but it means retention gaps get quieter, and
  R4's per-source reporting is what keeps them findable.
- Recording the list makes plans slightly larger and makes a recipe's source
  list visible in published artifacts. Both are accepted.
