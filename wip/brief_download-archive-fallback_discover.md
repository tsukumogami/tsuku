# Discover — download-archive-fallback

Scoping notes for `docs/briefs/BRIEF-download-archive-fallback.md`. Run
non-interactively (`--auto`) inside a `/scope` chain; the operator asked not
to be consulted mid-flight, so every question below is answered from evidence
in the repo and the tracker rather than from a live conversation.

## Artifact decision

Produce a durable BRIEF rather than passing evidence forward to the PRD.

Issue #2443 states the problem well, but the framing that matters is not in
the issue body — it is spread across three outage cycles (#2384 → PR #2391,
#2441 → PR #2444), a review comment on #2443, and a 45-line comment block in
`internal/recipe/recipes/zig.toml`. That framing has already been lost once:
PR #2391 wrote the follow-up into its own description in May, filed no issue,
and the second outage landed on another single-source swap. A durable brief is
the direct countermeasure to the failure mode that produced this issue.

## Upstream grounding

No ROADMAP or PRD upstream. The upstream is issue #2443 plus the merged
history above. `upstream:` is therefore omitted from the frontmatter — the
format allows it, and pointing at an issue URL is not a repo-relative artifact
path.

## Problem/outcome pair

- **Problem.** A recipe names exactly one download host. `download_archive`
  pre-downloads the archive at plan time to compute a checksum, so an
  unreachable host fails `tsuku eval`, not just `tsuku install`. zig is an
  implicit build-time dependency of `cmake_build`, `configure_make` and
  `meson_build`, so one dead host turns `Validate Embedded Dependencies` red
  on every PR that touches `recipes/`. The only available repair is a recipe
  edit by a maintainer, which does not reach plans already published to R2.
- **Outcome.** A recipe author can name more than one host for the same
  bytes. When the first is unreachable, plan generation and install both move
  to the next one without anybody editing a recipe. Published plans carry the
  whole list, so a consumer installing from a plan weeks later has the same
  fallback the maintainer had.

## Framing shift since the last two rounds

Both previous rounds framed the problem as "zig has a bad mirror" and fixed it
by swapping the mirror. The framing this brief holds is that the recipe schema
allows only one host, so every outage is a code change, and the blast radius is
repo-wide plan generation rather than one tool's install. That shift is why the
third round is a feature rather than a fourth swap.

## Journey entry points identified

1. Recipe author declaring several sources for a new or existing recipe.
2. CI, on a PR that touches `recipes/`, when the first source is unreachable.
3. A user installing from a published R2 plan weeks after generation.
4. A maintainer triaging an outage, deciding whether a recipe edit is needed
   at all.

## Scope boundary sketch

IN: multi-source declaration for the four actions that share the
`decomposeDownload` funnel; plan-time and install-time fallback; the full
ordered list recorded in the plan; validator and preflight coverage of every
entry; `zig.toml` converted with the current Fastly source first.

OUT: Zig's community-mirror protocol as a tsuku-level feature; a hostname-keyed
mirror registry in Go; the R2 golden publish/consume pipeline (#2448); making
`--pin-from` pin download checksums so plan generation stops downloading at
all; re-litigating Fastly as zig's first source.

## Open framing questions deferred to the PRD

- Whether the ordered list is a new parameter alongside `url` or a widened
  `url` that accepts a string or an array.
- Whether install-time fallback is required to be behaviourally identical to
  plan-time fallback, or merely best-effort.
