---
schema: design/v1
status: Planned
upstream: docs/prds/PRD-download-archive-fallback.md
problem: |
  A recipe names one download host, and download_archive fetches the whole
  archive at plan time to compute the checksum it pins, so an unreachable host
  fails `tsuku eval` repo-wide rather than failing one install. The repair is a
  recipe edit, which cannot reach plans already published to R2.
decision: |
  Add an optional ordered `fallback_urls` array beside the existing `url` on
  the four actions that share the `decomposeDownload` funnel. Convert that
  funnel's positional parameters to a struct so the change reaches all four
  without a fork. Carry the expanded list into the emitted `download_file`
  step's params, and only when a recipe declares alternates, so every
  single-source plan stays byte-identical. Put the failover loop in
  `internal/actions/download_file.go` next to the retry loop that already owns
  acquisition policy, and give the download cache a candidate-key lookup so
  fallback does not miss bytes it already holds.
rationale: |
  Keeping `url` a string means the feature is purely additive: absent, nothing
  anywhere behaves differently, and no golden moves. Recording the whole list
  rather than the winner is what keeps install-from-published-plan working,
  which is the one journey the consumer cannot repair. Putting failover next to
  retry keeps host policy in the recipe and acquisition policy in one place,
  rather than building the hostname-keyed registry the architectural review
  warned against.
---

# DESIGN: Multi-source fallback for archive downloads

## Status

Planned

Design for issue #2443, from `docs/prds/PRD-download-archive-fallback.md`.

## Context and Problem Statement

`DownloadArchiveAction` takes one `url` string. It calls `decomposeDownload`
(`internal/actions/composites.go:33`), which builds a params map and delegates
to `DownloadAction.Decompose` (`internal/actions/download.go:118`), which
expands placeholders, downloads the archive through `ctx.Downloader` purely to
compute a SHA256, and emits exactly one `download_file` step with that one URL
baked in. There is no `mirrors`, `fallback_urls`, or list-shaped parameter
anywhere in `internal/actions/` or `internal/recipe/`.

Three properties of that path make a single dead host expensive.

The download happens at plan time, not install time, because the checksum has
to be minted from the bytes. So an unreachable host fails `tsuku eval`. zig is
an implicit build-time dependency of `cmake_build`, `configure_make` and
`meson_build`, so one dead zig host fails plan generation for a large fraction
of the registry and turns `Validate Embedded Dependencies` red on unrelated
pull requests.

The funnel is shared. `decomposeDownload` has four callers —
`download_archive` (`composites.go:319`), `github_archive`
(`composites.go:658`), `github_file` (`composites.go:1072`) and
`fossil_archive` (`fossil_archive.go:142`) — and takes six positional
parameters. Anything added to it reaches all four, and anything added as a
seventh positional parameter invites the next caller to fork it instead.

The generated plan is a published artifact. `scripts/r2-upload.sh` publishes
plans to `plans/{category}/{recipe}/v{version}/{platform}.json`, and sandbox
and validation images install from those files weeks later. A recipe edit
cannot reach them, so whatever the plan records is what the consumer is stuck
with.

There is also a constraint on the shape of the change rather than its
behaviour. `testdata/golden/plans/` holds 96 golden files that record a
download URL. If plans grew a source list unconditionally, every one of them
would need regenerating — and `scripts/regenerate-golden.sh` cannot use
`--pin-from` for registry recipes, so a blanket regeneration re-resolves
dependencies to latest and launders unrelated version drift into the diff. The
R2 golden pipeline that might otherwise catch that is itself broken (#2448,
out of scope), so there is no safety net underneath a mass regeneration.

## Decision Drivers

- **Single-source recipes must be byte-identical.** PRD R13. This is the
  hardest constraint and it eliminates several otherwise-reasonable shapes.
- **The published plan must carry the whole list.** PRD R5, R6. The
  install-from-plan consumer is the one with no recourse.
- **The shared funnel must not fork.** PRD R8. Four callers, one change.
- **Host policy belongs to recipes.** The architectural review on #2443 is
  explicit that a hostname-keyed mirror registry in Go puts host policy where
  recipes cannot see it. Failover *policy* goes next to the existing retry
  loop; *which hosts* stays in the recipe.
- **The trust model must not widen.** PRD R15. Checksum identity, not URL
  identity, is already tsuku's reproducibility invariant.
- **No extra network cost when the first source answers.** PRD R14.

## Considered Options

### D1 — Recipe-facing shape

**Chosen: a new `fallback_urls` array beside `url`.**

```toml
[[steps]]
action = "download_archive"
url = "https://ziglang.freetls.fastly.net/zig-{arch}-{os}-{version}.tar.xz"
fallback_urls = [
  "https://zig.squirl.dev/zig-{arch}-{os}-{version}.tar.xz",
  "https://ziglang.org/download/{version}/zig-{arch}-{os}-{version}.tar.xz",
]
```

*Rejected: widen `url` to accept a string or an array.* It reads better — one
concept, one field — but `url` is read as a string in roughly a dozen places:
`GetString(params, "url")` in `DownloadAction`, `DownloadArchiveAction`,
`DownloadFileAction` and the plan generator; the `{version}`-placeholder
expectation in `actionVersionRules` (`internal/recipe/hardcoded.go:46`);
`validateURLParam` in the recipe validator (`validator.go:556`);
`DetectArchiveFormat(url)`, which infers the archive format from the URL
suffix. Every one would need a string-or-array branch, and every branch is a
place a later reader can get it wrong. Keeping `url` a string makes the feature
purely additive.

*Rejected: name it `mirrors`.* Implies the mirror-registry semantics the
architectural review warns against, and reads as a claim about the hosts'
relationship to each other rather than about tsuku's fallback order.

*Rejected: name it `sources` with `url` folded in.* Leaves the ordering
relationship between `url` and the list ambiguous at exactly the moment
ordering is the contract.

### D2 — How the shared funnel absorbs the change

**Chosen: convert `decomposeDownload`'s positional parameters to a struct.**

```go
// downloadSpec carries everything decomposeDownload needs to build a
// download_file step. It replaces the six positional parameters the function
// used to take: four callers share this funnel, and a seventh positional
// parameter is how a shared funnel acquires its first fork.
type downloadSpec struct {
    URL          string
    FallbackURLs []string
    Dest         string
    OSMapping    map[string]string
    ArchMapping  map[string]string
    ChecksumURL  string
}

func decomposeDownload(ctx *EvalContext, spec downloadSpec) (Step, error)
```

*Rejected: add a seventh positional parameter.* The call sites already read as
`decomposeDownload(ctx, url, "", osMapping, archMapping, checksumURL)` — two
empty-ish positional arguments in the middle. An extra `nil` in that sequence
is unreadable, and unreadable call sites are what make the next author write
their own helper.

*Rejected: extend `DownloadArchiveAction` only, leave the funnel alone.* Named
directly in the review as the thing to avoid. `github_archive` and
`github_file` resolve their URL from the GitHub API and so have less need for
fallback today, but `fossil_archive` has exactly the same exposure as
`download_archive`, and a mechanism that lives in one composite has to be
rebuilt in the next.

### D3 — Where the list lives in the plan

**Chosen: `params["fallback_urls"]`, with no hoisted `ResolvedStep` field.**

```json
{
  "action": "download_file",
  "params": {
    "checksum": "24aeee...",
    "checksum_algo": "sha256",
    "dest": "zig-x86_64-linux-0.14.1.tar.xz",
    "fallback_urls": [
      "https://zig.squirl.dev/zig-x86_64-linux-0.14.1.tar.xz"
    ],
    "url": "https://ziglang.freetls.fastly.net/zig-x86_64-linux-0.14.1.tar.xz"
  },
  "url": "https://ziglang.freetls.fastly.net/zig-x86_64-linux-0.14.1.tar.xz",
  "checksum": "24aeee...",
  "size": 49086504
}
```

The hoisted `ResolvedStep.URL` keeps naming the primary source and is never
rewritten, before or after fallback. The review on #2443 asked for that
explicitly, and the reason holds: `resolveDownloadDest`
(`internal/executor/executor.go:701`) and `ChecksumMismatchError` both read
`step.URL`, and both want one canonical URL rather than a list.

*Rejected: add `ResolvedStep.FallbackURLs` hoisted beside `URL`.* Symmetric,
but it duplicates state that lives in params, gives two places to keep in sync,
and has no consumer — the executor reads params, and the dest is resolved from
the `dest` param, which `Decompose` always sets. `URL` is hoisted because plan
consumers wanted a canonical URL without digging into params; nothing wants a
canonical *list*.

*Rejected: rewrite `step.URL` to whichever source served.* Makes plan output
depend on network conditions and breaks golden stability outright.

### D4 — Where the failover loop lives

**Chosen: `internal/actions/download_file.go`, wrapping the existing retry
loop.**

```go
// downloadFileHTTPWithFallback tries each source in order. Each source gets
// the full existing retry treatment (3 attempts, exponential backoff) before
// the next source is tried, so a flaky first source is not abandoned for a
// transient error.
func downloadFileHTTPWithFallback(
    ctx context.Context, urls []string, destPath string, reporter progress.Reporter,
) error
```

That file already owns acquisition policy — the retry count, the backoff
schedule, and `isRetryableStatusCode`. Failover is the same kind of policy one
level out, and putting it anywhere else splits the two.

*Rejected: a hostname-keyed mirror registry in Go.* Named in the review as the
anti-pattern. It would put "which host serves zig" in the CLI, where a recipe
author cannot see or change it, and would need updating in lockstep with
recipes.

*Rejected: a loop in each composite action.* Four copies of the same policy,
and `download_file` — which is what actually runs at install time from a
published plan — would still need its own.

### D5 — Download-cache reachability across sources

`DownloadCache` keys entries by `sha256(url)` (`download_cache.go:45`).
Left alone, fallback silently misses a cache that already holds the right
bytes: plan generation saves under whichever source answered, and install
checks under the primary.

**Chosen: candidate-key lookup.** `Check` gains a list-shaped sibling that
walks the ordered URL list and returns the first key that hits, with the
existing checksum verification applied unchanged to whatever it finds.

```go
// CheckAny tries each URL's cache key in order and returns on the first hit.
// Any source in a list serves byte-identical content for one pinned checksum,
// so an entry saved under one source is valid for every other source in the
// same list — and the existing checksum verification in Check is what proves
// it before the bytes are used.
func (c *DownloadCache) CheckAny(urls []string, destPath, expectedChecksum, checksumAlgo string) (bool, error)
```

*Rejected: re-key the cache by checksum.* Content-addressing is the
theoretically right key, and it would make the lookup a single probe instead of
a walk. But it invalidates every entry on disk, needs a migration or a silent
mass re-download, and buys nothing behavioural over the walk — the list is two
or three entries long. The `download` composite action also has no inline
checksum at cache-check time (`download.go:301` passes `""`), so a
checksum-keyed cache could not serve it at all.

*Rejected: leave the cache alone.* The miss is silent and the cost is a full
re-download of an archive that is already on disk — for zig, roughly 50 MB per
miss, in CI.

### D6 — Plan format version

**Chosen: stay at 5.**

`fallback_urls` is an optional key inside an existing map. A tsuku that
predates it unmarshals `params` as `map[string]interface{}`, ignores the key,
and downloads from `url` — which is exactly today's behaviour, so old readers
degrade to the status quo rather than breaking. `ValidatePlan` rejects only
`FormatVersion < 2`, so nothing gates on the exact value.

*Rejected: bump to 6.* It would be the honest signal that the format grew a
field, but `format_version` is recorded in every golden, so the bump rewrites
all 96 of them. That is precisely the mass regeneration driver R13 exists to
prevent, and there is no R2 safety net underneath it (#2448). The format
version is reserved for changes that actually break older readers.

### D7 — Emission rule

**Chosen: emit `fallback_urls` into the step params only when the recipe
declares a non-empty list.**

This is what makes R13 hold mechanically rather than by care. A recipe with no
`fallback_urls` produces a params map with no `fallback_urls` key, so its
serialized plan is byte-identical to the one on disk today. Only the recipes
this work converts move.

*Rejected: always emit, as `[]` or as `[url]`.* Either changes every golden
byte for no behavioural gain, and `[url]` additionally duplicates the primary
in two places in the same step.

## Decision Outcome

The pieces fit together as one additive path.

A recipe declares `url` plus an optional ordered `fallback_urls`. The composite
action reads both and hands them to `decomposeDownload` in a `downloadSpec`.
`DownloadAction.Decompose` expands placeholders over the primary and every
alternate with the same `vars` map — same `os_mapping`, same `arch_mapping`, so
a mirror that uses the same path layout needs no special handling — then
downloads for checksum computation by walking the expanded list until one
source answers. The emitted `download_file` step carries the primary in `url`
and, when there were alternates, the expanded alternates in `fallback_urls`.

At install time `DownloadFileAction.Execute` reads `url` and `fallback_urls`
from the step params, asks the cache for any of them, and on a miss walks the
same ordered list through the retry-and-failover loop. The pinned checksum is
verified once, against whatever arrived, exactly as today — the source that
answered is not recorded and does not matter, because checksum identity is the
invariant.

When every source fails, the error names each source and the reason it failed,
so CI logs distinguish an outage from a retention gap from a typo without
anyone having to reproduce it.

Nothing about this path activates for a recipe with no `fallback_urls`. The
funnel gets a struct instead of positional parameters, and the download call
becomes a one-element walk, but the params map, the emitted step, and the
serialized plan are unchanged byte-for-byte.

## Solution Architecture

### Components and the change to each

| File | Change |
|------|--------|
| `internal/actions/composites.go` | `decomposeDownload` takes a `downloadSpec` struct instead of six positional parameters. `DownloadArchiveAction.Preflight` validates every entry in `fallback_urls`; `Decompose` reads the list and passes it through. `github_archive` and `github_file` call sites updated to the struct form. |
| `internal/actions/fossil_archive.go` | Call site updated to the struct form. |
| `internal/actions/download.go` | `Decompose` expands `fallback_urls` with the same `vars`, walks the expanded list for checksum computation, and writes the alternates into the emitted step params when non-empty. `Preflight` validates the list. |
| `internal/actions/download_file.go` | `downloadFileHTTPWithFallback` wraps the existing retry loop. `Execute` reads `fallback_urls` and uses it for the cache probe and the download. |
| `internal/actions/download_cache.go` | `CheckAny` walks candidate keys. |
| `internal/recipe/validator.go` | `validatePathParams` validates every `fallback_urls` entry through the existing `validateURLParam`. |
| `internal/recipe/hardcoded.go` | `actionVersionRules` gains `fallback_urls` for `download` and `download_archive`, so the `{version}`-placeholder expectation covers alternates. |
| `internal/recipe/recipes/zig.toml` | Declares alternates behind the current Fastly source; the comment block is updated to say what changed. |
| `testdata/golden/plans/embedded/{zig,ninja}/` | Nine goldens gain `fallback_urls` in the zig download step. No other golden moves. |
| `docs/EMBEDDED_RECIPES.md` | Documents the field and the golden-handling rule. |

### Plan-time flow

```
recipe step (url + fallback_urls)
  -> DownloadArchiveAction.Decompose
  -> decomposeDownload(ctx, downloadSpec{URL, FallbackURLs, ...})
  -> DownloadAction.Decompose
       expand url and each fallback_url with the same vars
       walk [url, ...fallback] through ctx.Downloader.Download
         first success -> checksum, size
         all fail       -> error naming every source and its failure
       emit download_file{ params: {url, dest, checksum, checksum_algo,
                                    fallback_urls?} }
```

### Install-time flow

```
plan step download_file{url, fallback_urls?, checksum}
  -> DownloadFileAction.Execute
       cache.CheckAny([url, ...fallback], dest, checksum, algo)
         hit  -> done
         miss -> downloadFileHTTPWithFallback([url, ...fallback])
                   per source: existing retry loop (3 attempts, backoff)
                   first success -> stop
                   all fail      -> error naming every source
       VerifyChecksum(dest, checksum, algo)   // unchanged, always runs
       cache.Save(servingURL, dest, checksum)
```

### The one behavioural subtlety worth naming

A non-retryable status — a 404, say — currently aborts the download
immediately, because `isRetryableStatusCode` returns false and the retry loop
returns without further attempts. Under fallback, a non-retryable status ends
attempts *against that source* and moves to the next one. That is deliberate
and is the retention case from the zig recipe's own triage notes: a mirror that
has dropped an old release while still serving current ones answers 404, and
falling through to a source that kept it is the whole point. The cost is that
a genuinely wrong URL — a typo in the recipe — now costs one round trip per
source before failing, and the error has to name each one for the typo to stay
findable. R4 is what keeps that visible.

## Implementation Approach

Five commits, sequenced so each one is separately reviewable and the tree
builds and tests green at every step.

1. **Struct-ify the funnel.** Convert `decomposeDownload` to `downloadSpec`
   and update all four call sites. Pure refactor: no behaviour change, no new
   field, no golden movement. Lands first so the later diffs are about
   fallback rather than about parameter plumbing.

2. **Plan-time fallback.** `fallback_urls` on the recipe schema, expansion and
   walk in `DownloadAction.Decompose`, conditional emission into step params,
   preflight and recipe-validator coverage of every entry. Tests: fallback to a
   later source, exhaustion with a per-source error, single-source behaviour
   unchanged, placeholder expansion applied to alternates.

3. **Install-time fallback and the cache.** `downloadFileHTTPWithFallback`,
   `DownloadFileAction.Execute` reading the list, `CheckAny`. Tests: install
   falls through, checksum mismatch on an alternate still fails, a plan with no
   `fallback_urls` behaves as today, cache entry saved under one source is a
   hit under another.

4. **Convert zig and regenerate its goldens.** `zig.toml` gains alternates;
   the three zig goldens are regenerated and the six ninja goldens get the
   URL-only edit their recipe comment already prescribes (ninja sits on the
   #988 code-validation exclusion list, so a full regeneration would sweep in
   unrelated format drift). `scripts/validate-all-golden.sh` confirms nothing
   else moved.

5. **Document.** The field, the emission rule, and the golden-handling
   consequence in `docs/EMBEDDED_RECIPES.md`.

### Which alternates zig gets

The recipe's own triage notes name `zig.squirl.dev` as the strongest
independent candidate — independently operated, CDN-backed, and storing its own
copies rather than proxying ziglang.org — and record that it was not taken in
#2441 only because reachability from GitHub runners was proven for Fastly and
unproven for it. Fallback changes that calculus: an alternate whose
reachability is unproven costs nothing if it is second, because it is only
reached when the proven source is already down. Canonical `ziglang.org` goes
last, since it is the origin both prior incidents ran into but is also the one
host guaranteed to carry every release.

Reachability of the alternates from GitHub runners is what CI will demonstrate,
not something to assert here — but note that a *failure* to reach an alternate
is invisible in a normal run, because the primary answers. The zig golden
regeneration is the observation point: it exercises each URL directly.

## Security Considerations

**The trust model is unchanged, and the reason is mechanical rather than
argued.** `download_file` requires a checksum (`ValidatePlan` rejects a
`download_file` step without one) and `VerifyChecksum` runs on the downloaded
bytes before anything else touches them. Adding sources does not add a way to
skip that check: the fallback loop's only job is to produce bytes at
`destPath`, and every path out of `Execute` still runs the same verification.
A malicious or compromised alternate serving different bytes fails exactly as a
corrupted download fails today.

**What does widen is the plan-time trust set**, and this is the real
consideration. Plan generation is where the checksum gets minted, so whichever
source answers first defines what every later install will accept. Under a
single URL that set is one host; under a list it is the first *reachable* host.
The mitigation available today is `checksum_url`, which is already plumbed
through `DownloadAction.Decompose` and validates the plan-time-computed
checksum against an upstream-published value, failing plan generation on
mismatch. That check is applied independently of which source answered (PRD
R16). See the correction below on why a preflight warning for the
no-anchor case is deliberately absent.

Mandatory anchoring is not taken, because it would block the acceptance
criteria. The architectural review on #2443 stated that minisign machinery
exists in `internal/actions/download.go`; it does not — that file carries PGP
verification, and `grep -rn minisign internal/ --include=*.go` returns
nothing. zig publishes `.minisig` and no `.sha256` sidecar
(`zig-x86_64-linux-0.14.1.tar.xz.sha256` returns 404 while `.minisig` returns
200), so requiring an anchor would mean building minisign support before zig
could satisfy its own acceptance criterion. That is named in the PRD's Out of
Scope as the follow-up.

**Correction, made during implementation.** This design originally called for
a preflight *warning* when a recipe declares alternates without an anchor. That
was wrong, and CI said so: `tsuku validate --strict` promotes warnings to
errors, and the `Tests / Validate Recipes` job validates every recipe that way,
so the warning made `zig.toml` fail validation. A warning under `--strict` is
not a nudge — it is the mandatory anchoring this section just rejected, arriving
through a side door. The warning was removed. The per-entry checks that remain
(HTTPS scheme, empty entry, unreachable duplicate, static alternate behind a
version-templated primary) are all genuine authoring mistakes that do not fire
for a correct recipe. The widened plan-time trust set is recorded in prose here,
in the PRD, and in `zig.toml`'s own comment, which is where a reader looking for
it will be.

**Ordering is fixed and recipe-declared**, which closes the obvious attack on a
fallback mechanism. Nothing probes hosts, ranks by latency, or remembers which
source answered last time, so an attacker who can degrade one host cannot
thereby promote another into first position — they can only cause a fall
through to the next host the recipe author already chose to trust.

**HTTPS enforcement applies per source.** `downloadFileHTTPWithFallback` calls
the same per-attempt path that rejects non-HTTPS URLs, and the recipe validator
runs `validateURLParam` over every entry, so an `http://` alternate is caught
at authoring time rather than at fetch time.

**The cache change does not weaken verification.** `CheckAny` only changes
which keys are probed; the checksum verification inside `Check` is unchanged
and still gates whether a cached file is used. An entry cached under one source
is used for another only after it verifies against the same pinned checksum.

**No new SSRF surface.** Alternates go through `httputil.NewSecureClient`, the
same client the primary uses, with the same SSRF protections.

## Consequences

### Positive

- A single host outage stops being a repo-wide CI event, and stops being a
  dead end for anyone installing from a published plan.
- Plans published to R2 carry the fallback with them, so the consumer who
  cannot regenerate or edit gets the same resilience the maintainer had.
- The change is additive: single-source recipes produce byte-identical plans,
  so the diff is confined to the recipes actually converted and a reviewer can
  read it.
- The funnel refactor makes the next parameter addition cheap for all four
  callers instead of tempting a fork.
- Recipe authors gain a place to record mirror knowledge they already have and
  currently have to throw away.

### Negative

- A wrong URL costs one round trip per source before failing. The
  per-source error required by R4 is what keeps the typo findable, but the
  failure is slower than it is today.
- A source that has dropped an old release is now papered over on the way to
  one that kept it. Desired, but it makes retention gaps quieter.
- The plan-time trust set is the first reachable source rather than one fixed
  host. Bounded by the recipe author's declared list and by `checksum_url`
  where available; fully closed only when minisign support lands.
- A recipe can list hosts that are all fronted by the same origin and look more
  resilient than it is. The zig recipe comment already records this for Fastly
  specifically; the mechanism makes adding entries easy without making them
  independent.
- One more optional field on the four download actions, and one more thing for
  a recipe author to know about.

### Mitigations

- R4's per-source error reporting keeps typos and retention gaps legible in CI
  logs without reproduction.
- The widened plan-time trust set is recorded in this document, in the PRD,
  and in `zig.toml`'s own comment. It is deliberately not a preflight warning:
  `--strict` would turn that into the mandatory anchoring the design rejected.
- `docs/EMBEDDED_RECIPES.md` documents the emission rule, so the next person to
  regenerate a golden knows why `fallback_urls` appears in some plans and not
  others.
- The zig recipe comment is updated in place, keeping the "this is still one
  URL" note from going stale and recording what independence actually requires.
