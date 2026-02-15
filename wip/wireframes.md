# Page Wireframes (from DESIGN-pipeline-dashboard.md)

Reference wireframes for dashboard observability implementation issues. Extracted from
the parent design's Page Wireframes section (lines 851-1508).

Every element described below is clickable unless marked (static).

## Main Dashboard (`index.html`)

```
+---------------------------------------------------------------------+
|  Pipeline Dashboard                                    [R Refresh]  |
|  Generated: 2026-02-15T14:00:00Z (static)                          |
+---------------------------------------------------------------------+
|                                                                     |
|  +- Pipeline Health --------------------------------------------+  |
|  |                                                               |  |
|  |  Status: Running (static)                                     |  |
|  |                                                               |  |
|  |  Last Run         1 hour ago (0/10)        > run.html?id=... |  |
|  |  Last Success     9 days ago (2 recipes)   > run.html?id=... |  |
|  |  Runs Since       156 runs                 > runs.html?since |  |
|  |                                                               |  |
|  |  Circuit Breakers:                                            |  |
|  |    homebrew: closed    > failures.html?ecosystem=homebrew     |  |
|  |    cargo:    closed    > failures.html?ecosystem=cargo        |  |
|  |    npm:      half-open > failures.html?ecosystem=npm          |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Queue Status -------------------+  +- Top Blockers ----------+ |
|  |  [View All >]     pending.html   |  |  [View All >] blocked   | |
|  |                                  |  |                         | |
|  |  Total: 5,144                    |  |  glib (4 deps)     >    | |
|  |  +- Pending: 4,988          >    |  |  openssl (3 deps)  >    | |
|  |  +- Success: 138            >    |  |  libffi (2 deps)   >    | |
|  |  +- Failed: 14              >    |  |                         | |
|  |  +- Blocked: 4              >    |  |  Each row links to      | |
|  |                                  |  |  blocked.html?pkg=      | |
|  |  By Ecosystem:                   |  |                         | |
|  |    homebrew: 5,100          >    |  +-------------------------+ |
|  |    cargo: 44                >    |                              |
|  |                                  |                              |
|  +----------------------------------+                              |
|                                                                     |
|  +- Recent Failures (5 of 42) ----------------------------------+  |
|  |  [View All >]                              failures.html      |  |
|  |                                                               |  |
|  |  Package    | Category              | When      | Details    |  |
|  |  -----------+-----------------------+-----------+----------- |  |
|  |  neovim     | verify_pattern_mis... | 1h ago    | [>]        |  |
|  |  bat        | no_bottle             | 1h ago    | [>]        |  |
|  |  fd         | no_bottle             | 1h ago    | [>]        |  |
|  |  rg         | no_bottle             | 1h ago    | [>]        |  |
|  |  jq         | archive_extract_f...  | 1h ago    | [>]        |  |
|  |                                                               |  |
|  |  Each row > failure.html?id=<failure-id>                      |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Recent Runs (3 of 156) -------------------------------------+  |
|  |  [View All >]                                    runs.html    |  |
|  |                                                               |  |
|  |  Batch ID              | Ecosystem | Result    | When        |  |
|  |  ----------------------+-----------+-----------+------------ |  |
|  |  2026-02-15-homebrew   | homebrew  | 0/10 X    | 1h ago  [>] |  |
|  |  2026-02-15-cargo      | cargo     | 2/5 !     | 2h ago  [>] |  |
|  |  2026-02-15-npm        | npm       | 5/5 ok    | 3h ago  [>] |  |
|  |                                                               |  |
|  |  Each row > run.html?id=<batch-id>                            |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Failure Categories ---------------+  +- Disambiguations -----+ |
|  |  [View All >]    failures.html     |  |  [View All >] disamb  | |
|  |                                    |  |                       | |
|  |  validation_failed: 30        >    |  |  Total: 32            | |
|  |  +- verify_pattern: 18        >    |  |  Need Review: 5    >  | |
|  |  +- no_bottle: 8              >    |  |                       | |
|  |  +- install_failed: 4         >    |  |  By Reason:           | |
|  |  deterministic: 8             >    |  |    better_source: 20> | |
|  |  api_error: 4                 >    |  |    no_homebrew: 8  >  | |
|  |                                    |  |    manual: 4       >  | |
|  |  Each links to filtered            |  |                       | |
|  |  failures.html?category=           |  +-----------------------+ |
|  +------------------------------------+                            |
|                                                                     |
+---------------------------------------------------------------------+
```

## Failures List (`failures.html`)

```
+---------------------------------------------------------------------+
|  < Back to Dashboard                                                |
|                                                                     |
|  Failures (42 total)                                                |
+---------------------------------------------------------------------+
|                                                                     |
|  Filters:                                                           |
|  +-------------+ +-------------+ +-------------+ +---------------+ |
|  | Category v  | | Ecosystem v | | Date From   | | Date To       | |
|  | (all)       | | (all)       | | 2026-02-01  | | 2026-02-15    | |
|  +-------------+ +-------------+ +-------------+ +---------------+ |
|                                                           [Apply]   |
|                                                                     |
|  +---------------------------------------------------------------+ |
|  | Package  | Ecosystem | Category        | Subcategory    |When | |
|  +----------+-----------+-----------------+----------------+-----+ |
|  | neovim   | homebrew  | validation      | verify_pattern | 1h  | |
|  | [> detail page]                                               | |
|  +----------+-----------+-----------------+----------------+-----+ |
|  | bat      | homebrew  | deterministic   | no_bottle      | 1h  | |
|  | [> detail page]                                               | |
|  +----------+-----------+-----------------+----------------+-----+ |
|  | fd       | homebrew  | deterministic   | no_bottle      | 1h  | |
|  | [> detail page]                                               | |
|  +----------+-----------+-----------------+----------------+-----+ |
|  | ...      |           |                 |                |     | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  Showing 1-20 of 42                        [< Prev] [Next >]       |
|                                                                     |
+---------------------------------------------------------------------+
```

## Failure Detail (`failure.html?id=homebrew-2026-02-15T13-45-21Z-neovim`)

```
+---------------------------------------------------------------------+
|  < Back to Failures                                                 |
|                                                                     |
|  Failure: neovim                                                    |
+---------------------------------------------------------------------+
|                                                                     |
|  +- Summary ------------------------------------------------------+|
|  |                                                               |  |
|  |  Package:      neovim           > package.html?id=neovim     |  |
|  |  Ecosystem:    homebrew         > failures.html?eco=homebrew |  |
|  |  Category:     validation_failed> failures.html?cat=valid.. |  |
|  |  Subcategory:  verify_pattern_mismatch                       |  |
|  |  Timestamp:    2026-02-15T13:45:21Z                          |  |
|  |  Batch:        2026-02-15-homebrew > run.html?id=...         |  |
|  |  Platform:     linux-x86_64-debian                           |  |
|  |  Workflow:     [View on GitHub >]                            |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Error Message ------------------------------------------------+  |
|  |                                                               |  |
|  |  Verification failed: version pattern mismatch                |  |
|  |                                                               |  |
|  |  Expected: v0.10.0                                            |  |
|  |  Got:      NVIM v0.10.0                                       |  |
|  |                                                               |  |
|  |  The verify command output did not match the expected         |  |
|  |  version pattern. This usually means the recipe's verify      |  |
|  |  pattern needs adjustment.                                    |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Full CLI Output ----------------------------------------------+  |
|  |                                                               |  |
|  |  $ tsuku install --json --recipe recipes/n/neovim.toml       |  |
|  |                                                               |  |
|  |  {                                                            |  |
|  |    "status": "failed",                                        |  |
|  |    "category": "validation_failed",                           |  |
|  |    "subcategory": "verify_pattern_mismatch",                  |  |
|  |    "details": {                                               |  |
|  |      "expected": "v0.10.0",                                   |  |
|  |      "actual": "NVIM v0.10.0",                                |  |
|  |      "command": "nvim --version",                             |  |
|  |      "exit_code": 0                                           |  |
|  |    }                                                          |  |
|  |  }                                                            |  |
|  |                                                               |  |
|  |  exit code: 6                                                 |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Recipe Snippet -----------------------------------------------+  |
|  |                                                               |  |
|  |  [verify]                                                     |  |
|  |  command = "nvim --version"                                   |  |
|  |  pattern = "v0.10.0"    < Problem: missing "NVIM " prefix    |  |
|  |                                                               |  |
|  |  [View full recipe >] (links to GitHub)                       |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Actions ------------------------------------------------------+  |
|  |                                                               |  |
|  |  [File issue on GitHub]  (opens pre-filled GitHub issue)     |  |
|  |                                                               |  |
|  |  --- Authenticated actions (link to GitHub, require login) -- |  |
|  |  [Retry this package]  (triggers workflow_dispatch)          |  |
|  |  [Mark as won't fix]   (adds to exclusions via PR)           |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  Note: "Retry" and "Mark as won't fix" link to GitHub Actions      |
|  workflow_dispatch or create PRs. They don't execute directly      |
|  from the dashboard. This keeps authentication on GitHub's side.   |
|                                                                     |
+---------------------------------------------------------------------+
```

## Runs List (`runs.html`)

```
+---------------------------------------------------------------------+
|  < Back to Dashboard                                                |
|                                                                     |
|  Batch Runs (156 total)                                             |
+---------------------------------------------------------------------+
|                                                                     |
|  Filters:                                                           |
|  +-------------+ +-------------+ +---------------+                 |
|  | Ecosystem v | | Status v    | | Since Success |                 |
|  | (all)       | | (all)       | | [ ] only      |                 |
|  +-------------+ +-------------+ +---------------+     [Apply]     |
|                                                                     |
|  +---------------------------------------------------------------+ |
|  | Batch ID            | Eco      | Success | Failed | When      | |
|  +---------------------+----------+---------+--------+-----------+ |
|  | 2026-02-15-homebrew | homebrew | 0       | 10     | 1h ago    | |
|  | [> run detail page]                                           | |
|  +---------------------+----------+---------+--------+-----------+ |
|  | 2026-02-15-cargo    | cargo    | 2       | 3      | 2h ago    | |
|  | [> run detail page]                                           | |
|  +---------------------+----------+---------+--------+-----------+ |
|  | 2026-02-15-npm      | npm      | 5       | 0      | 3h ago    | |
|  | [> run detail page]                                           | |
|  +---------------------+----------+---------+--------+-----------+ |
|  | ...                 |          |         |        |           | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  Showing 1-20 of 156                       [< Prev] [Next >]       |
|                                                                     |
|  +- Summary ------------------------------------------------------+|
|  |  Last 24h: 24 runs, 12 recipes generated                     |  |
|  |  Last 7d:  168 runs, 45 recipes generated                    |  |
|  |  Success rate: 8.2% (packages), 26.8% (runs with >=1 recipe) |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
+---------------------------------------------------------------------+
```

## Run Detail (`run.html?id=2026-02-15-homebrew`)

```
+---------------------------------------------------------------------+
|  < Back to Runs                                                     |
|                                                                     |
|  Batch Run: 2026-02-15-homebrew                                     |
+---------------------------------------------------------------------+
|                                                                     |
|  +- Summary ------------------------------------------------------+|
|  |                                                               |  |
|  |  Batch ID:    2026-02-15-homebrew                            |  |
|  |  Ecosystem:   homebrew          > pending.html?eco=homebrew  |  |
|  |  Timestamp:   2026-02-15T13:45:21Z                           |  |
|  |  Duration:    3m 34s                                         |  |
|  |  Workflow:    [View on GitHub >]                             |  |
|  |                                                               |  |
|  |  Result:      0 succeeded, 10 failed, 0 blocked              |  |
|  |  Recipes:     (none generated)                               |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Packages Processed -------------------------------------------+  |
|  |                                                               |  |
|  |  Package  | Status  | Category           | Details           |  |
|  |  ---------+---------+--------------------+-------------------+  |
|  |  neovim   | X fail  | verify_pattern     | [> failure]       |  |
|  |  bat      | X fail  | no_bottle          | [> failure]       |  |
|  |  fd       | X fail  | no_bottle          | [> failure]       |  |
|  |  rg       | X fail  | no_bottle          | [> failure]       |  |
|  |  jq       | X fail  | archive_extract    | [> failure]       |  |
|  |  fzf      | X fail  | no_bottle          | [> failure]       |  |
|  |  exa      | X fail  | no_bottle          | [> failure]       |  |
|  |  delta    | X fail  | no_bottle          | [> failure]       |  |
|  |  zoxide   | X fail  | no_bottle          | [> failure]       |  |
|  |  lazygit  | X fail  | no_bottle          | [> failure]       |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Platform Results ---------------------------------------------+  |
|  |                                                               |  |
|  |  Platform              | Tested | Passed | Failed | Skipped  |  |
|  |  ----------------------+--------+--------+--------+--------- |  |
|  |  linux-x86_64-debian   | 10     | 0      | 10     | 0        |  |
|  |  linux-x86_64-ubuntu   | 10     | 0      | 10     | 0        |  |
|  |  linux-x86_64-fedora   | 10     | 0      | 10     | 0        |  |
|  |  linux-x86_64-arch     | 10     | 0      | 10     | 0        |  |
|  |  linux-x86_64-alpine   | 10     | 0      | 10     | 0        |  |
|  |  linux-arm64-debian    | 10     | 0      | 10     | 0        |  |
|  |  linux-arm64-ubuntu    | 10     | 0      | 10     | 0        |  |
|  |  linux-arm64-fedora    | 10     | 0      | 10     | 0        |  |
|  |  linux-arm64-alpine    | 10     | 0      | 10     | 0        |  |
|  |  darwin-x86_64         | 10     | 0      | 10     | 0        |  |
|  |  darwin-arm64          | 10     | 0      | 10     | 0        |  |
|  |                                                               |  |
|  |  Each row links to filtered failures for that platform        |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Actions ------------------------------------------------------+  |
|  |                                                               |  |
|  |  [Retry this batch]    (re-runs same 10 packages)            |  |
|  |  [View workflow logs]  (GitHub Actions)                      |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
+---------------------------------------------------------------------+
```

## Pending Packages (`pending.html`)

```
+---------------------------------------------------------------------+
|  < Back to Dashboard                                                |
|                                                                     |
|  Pending Packages (4,988 total)                                     |
+---------------------------------------------------------------------+
|                                                                     |
|  Filters:                                                           |
|  +-------------+ +-------------+ +-------------+                   |
|  | Ecosystem v | | Priority v  | | Search...   |      [Apply]     |
|  | (all)       | | (all)       | |             |                   |
|  +-------------+ +-------------+ +-------------+                   |
|                                                                     |
|  +---------------------------------------------------------------+ |
|  | Package     | Ecosystem | Pri | Added      | Attempts        | |
|  +-------------+-----------+-----+------------+-----------------+ |
|  | neovim      | homebrew  | 1   | 2026-01-15 | 12 (last: 1h)   | |
|  | [> package detail]                                            | |
|  +-------------+-----------+-----+------------+-----------------+ |
|  | vim         | homebrew  | 1   | 2026-01-15 | 8 (last: 2h)    | |
|  | [> package detail]                                            | |
|  +-------------+-----------+-----+------------+-----------------+ |
|  | emacs       | homebrew  | 1   | 2026-01-15 | 5 (last: 3h)    | |
|  | [> package detail]                                            | |
|  +-------------+-----------+-----+------------+-----------------+ |
|  | ...         |           |     |            |                 | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  Showing 1-50 of 4,988                     [< Prev] [Next >]       |
|                                                                     |
|  +- By Ecosystem -------------------------------------------------+|
|  |  homebrew: 4,850 > | cargo: 44 > | npm: 32 > | pypi: 28 >    |  |
|  |  rubygems: 18 >    | go: 12 >    | cpan: 4 > | cask: 0 >     |  |
|  |  (each links to filtered pending.html?ecosystem=)            |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
+---------------------------------------------------------------------+
```

## Package Detail (`package.html?id=homebrew:neovim`)

```
+---------------------------------------------------------------------+
|  < Back to Pending                                                  |
|                                                                     |
|  Package: neovim                                                    |
+---------------------------------------------------------------------+
|                                                                     |
|  +- Status -------------------------------------------------------+|
|  |                                                               |  |
|  |  Queue Status:  pending                                       |  |
|  |  Ecosystem:     homebrew                                      |  |
|  |  Queue ID:      homebrew:neovim                               |  |
|  |  Priority:      1 (critical)                                  |  |
|  |  Added:         2026-01-15                                    |  |
|  |  Attempts:      12                                            |  |
|  |  Last Attempt:  2026-02-15T13:45:21Z (1 hour ago)            |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Disambiguation -----------------------------------------------+  |
|  |                                                               |  |
|  |  Status: No override configured                               |  |
|  |                                                               |  |
|  |  Available sources:                                           |  |
|  |    * homebrew:neovim  (current)                               |  |
|  |    * github:neovim/neovim                                     |  |
|  |    * cask:neovim                                              |  |
|  |                                                               |  |
|  |  [Configure disambiguation >] (opens disambiguations editor)  |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Attempt History ----------------------------------------------+  |
|  |                                                               |  |
|  |  #  | Timestamp           | Result  | Category        | Det  |  |
|  |  ---+---------------------+---------+-----------------+----- |  |
|  |  12 | 2026-02-15 13:45:21 | X fail  | verify_pattern  | [>]  |  |
|  |  11 | 2026-02-14 05:45:18 | X fail  | verify_pattern  | [>]  |  |
|  |  10 | 2026-02-13 21:45:15 | X fail  | verify_pattern  | [>]  |  |
|  |  9  | 2026-02-13 13:45:12 | X fail  | verify_pattern  | [>]  |  |
|  |  8  | 2026-02-12 05:45:09 | X fail  | verify_pattern  | [>]  |  |
|  |  ... (show more)                                              |  |
|  |                                                               |  |
|  |  Each row > failure.html?id=                                  |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
|  +- Actions ------------------------------------------------------+  |
|  |                                                               |  |
|  |  [Retry now]           (triggers immediate batch for this)   |  |
|  |  [Skip temporarily]    (removes from queue for 7 days)       |  |
|  |  [Exclude permanently] (adds to exclusion list)              |  |
|  |  [Change ecosystem]    (opens disambiguation editor)         |  |
|  |  [File issue]          (opens GitHub with context)           |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
+---------------------------------------------------------------------+
```

## Blocked Packages (`blocked.html`)

```
+---------------------------------------------------------------------+
|  < Back to Dashboard                                                |
|                                                                     |
|  Blocked Packages (4 total)                                         |
+---------------------------------------------------------------------+
|                                                                     |
|  +---------------------------------------------------------------+ |
|  | Package    | Ecosystem | Blocked By          | Since          | |
|  +------------+-----------+---------------------+----------------+ |
|  | gtk+3      | homebrew  | glib, cairo         | 2026-02-10     | |
|  | [> package detail with dependency graph]                      | |
|  +------------+-----------+---------------------+----------------+ |
|  | imagemagick| homebrew  | libpng, libtiff     | 2026-02-08     | |
|  | [> package detail]                                            | |
|  +------------+-----------+---------------------+----------------+ |
|  | ffmpeg     | homebrew  | libvpx, x264, x265  | 2026-02-05     | |
|  | [> package detail]                                            | |
|  +------------+-----------+---------------------+----------------+ |
|  | opencv     | homebrew  | ffmpeg              | 2026-02-05     | |
|  | [> package detail]                                            | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  +- Top Blockers (missing dependencies) --------------------------+|
|  |                                                               |  |
|  |  Dependency | Blocks           | Status                      |  |
|  |  -----------+------------------+---------------------------- |  |
|  |  glib       | 4 packages       | pending (last try: 2h ago)  |  |
|  |  [> package.html?id=homebrew:glib]                           |  |
|  |  cairo      | 3 packages       | pending (last try: 2h ago)  |  |
|  |  libpng     | 2 packages       | failed (no_bottle)          |  |
|  |  libtiff    | 2 packages       | failed (no_bottle)          |  |
|  |                                                               |  |
|  |  Resolving glib would unblock 4 packages                      |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
+---------------------------------------------------------------------+
```

## Disambiguations (`disambiguations.html`)

```
+---------------------------------------------------------------------+
|  < Back to Dashboard                                                |
|                                                                     |
|  Disambiguations (32 total, 5 need review)                          |
+---------------------------------------------------------------------+
|                                                                     |
|  Filters:                                                           |
|  +-----------------+ +-------------+                               |
|  | Status v        | | Reason v    |                    [Apply]    |
|  | (all)           | | (all)       |                               |
|  | * All           | +-------------+                               |
|  | * Needs review  |                                               |
|  | * Configured    |                                               |
|  +-----------------+                                               |
|                                                                     |
|  +---------------------------------------------------------------+ |
|  | Package | From           | To                  | Reason       | |
|  +---------+----------------+---------------------+--------------+ |
|  | rg      | homebrew:rg    | github:BurntSushi/  | better_source| |
|  | [> disambiguation detail]                                     | |
|  +---------+----------------+---------------------+--------------+ |
|  | bat     | homebrew:bat   | github:sharkdp/bat  | better_source| |
|  | [> disambiguation detail]                                     | |
|  +---------+----------------+---------------------+--------------+ |
|  | fd      | homebrew:fd    | github:sharkdp/fd   | better_source| |
|  | [> disambiguation detail]                                     | |
|  +---------+----------------+---------------------+--------------+ |
|  | exa     | homebrew:exa   | (needs review)      | !!           | |
|  | [> disambiguation editor - tool has multiple viable sources]  | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  +- By Reason ----------------------------------------------------+|
|  |  better_source: 20   (GitHub has pre-built binaries)         |  |
|  |  no_homebrew: 8      (tool not in Homebrew)                  |  |
|  |  manual: 4           (manually configured)                   |  |
|  |  Each links to filtered list                                 |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
+---------------------------------------------------------------------+
```

## Seeding Stats (`seeding.html`)

Deferred to Phase 3.

```
+---------------------------------------------------------------------+
|  < Back to Dashboard                                                |
|                                                                     |
|  Seeding Stats                                                      |
+---------------------------------------------------------------------+
|                                                                     |
|  +- Last Seeding Run -------------------------------------------+ |
|  |                                                               | |
|  |  Timestamp:    2026-02-15T06:00:00Z (9 hours ago)            | |
|  |  Duration:     47 minutes                                     | |
|  |  Packages:     5,244 total in queue                           | |
|  |  Processed:    312 (new: 52, stale: 248, retries: 12)        | |
|  |  Source changes: 3 (2 auto-accepted, 1 flagged for review)   | |
|  |  Workflow:     [View on GitHub >]                             | |
|  |                                                               | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  +- Ecosystem Coverage ------------------------------------------+ |
|  |                                                               | |
|  |  Ecosystem   | Packages | % of Queue | Trend (30d)           | |
|  |  ------------+----------+------------+--------------------- | |
|  |  homebrew    | 3,850    | 73.4%      | down 2% (was 75.4%)   | |
|  |  cargo       | 644      | 12.3%      | up 1.5%               | |
|  |  npm         | 320      | 6.1%       | up 0.3%               | |
|  |  github      | 218      | 4.2%       | up 0.2%               | |
|  |  pypi        | 128      | 2.4%       | flat                  | |
|  |  rubygems    | 52       | 1.0%       | flat                  | |
|  |  go          | 32       | 0.6%       | flat                  | |
|  |                                                               | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  +- Disambiguation Breakdown ------------------------------------+ |
|  |                                                               | |
|  |  auto (68%)                                                   | |
|  |  curated (12%)                                                | |
|  |  requires_manual (20%)                                        | |
|  |                                                               | |
|  |  auto: 3,566 packages (10x threshold met)                    | |
|  |  curated: 629 packages (manual overrides)                    | |
|  |  requires_manual: 1,049 packages (need LLM/human)            | |
|  |                                                               | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  +- Recent Source Changes ----------------------------------------+ |
|  |                                                               | |
|  |  Package | Old Source       | New Source        | Status       | |
|  |  --------+------------------+-------------------+------------- | |
|  |  tokei   | homebrew:tokei   | cargo:tokei       | ok accepted  | |
|  |  dust    | homebrew:dust    | cargo:du-dust     | ok accepted  | |
|  |  procs   | homebrew:procs   | cargo:procs       | !! review    | |
|  |  [> procs flagged because priority=1, needs manual approval]  | |
|  |                                                               | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  +- Seeding History ---------------------------------------------+ |
|  |                                                               | |
|  |  Date       | Processed | Changes | Errors | Duration        | |
|  |  -----------+-----------+---------+--------+---------------- | |
|  |  2026-02-15 | 312       | 3       | 0      | 47m             | |
|  |  2026-02-08 | 287       | 5       | 1      | 52m             | |
|  |  2026-02-01 | 5,102     | 0       | 0      | 4h 12m (init)   | |
|  |  [> each row links to seeding run detail]                     | |
|  |                                                               | |
|  +---------------------------------------------------------------+ |
|                                                                     |
+---------------------------------------------------------------------+
```

## Curated Overrides (`curated.html`)

```
+---------------------------------------------------------------------+
|  < Back to Dashboard                                                |
|                                                                     |
|  Curated Overrides (28 total, 2 have validation errors)            |
+---------------------------------------------------------------------+
|                                                                     |
|  These are manual source selections that override algorithmic       |
|  disambiguation. They represent expert knowledge about where a      |
|  package should be sourced from.                                    |
|                                                                     |
|  +---------------------------------------------------------------+ |
|  | Package  | Source               | Reason            | Status  | |
|  +----------+----------------------+-------------------+---------+ |
|  | ripgrep  | cargo:ripgrep        | canonical crate   | valid   | |
|  | bat      | github:sharkdp/bat   | pre-built bins    | valid   | |
|  | fd       | github:sharkdp/fd    | pre-built bins    | valid   | |
|  | exa      | cargo:exa            | canonical crate   | !! 404  | |
|  | [> source no longer exists, needs update]                     | |
|  | delta    | github:dandavison/d  | pre-built bins    | valid   | |
|  | ...                                                           | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  Actions (all link to GitHub - no direct dashboard execution):      |
|  +---------------------------------------------------------------+ |
|  |  [Add Override]     > Opens PR template to edit queue          | |
|  |  [Remove Override]  > Opens PR template to set confidence=null | |
|  |  [Fix Invalid]      > Opens issue for broken curated sources   | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  +- Summary ------------------------------------------------------+|
|  |  Total: 28                                                      ||
|  |  Valid: 26 (sources exist and respond)                          ||
|  |  Invalid: 2 (source 404 or validation failed)                   ||
|  |  Last validated: 2026-02-15T06:00:00Z (by seeding workflow)    ||
|  +---------------------------------------------------------------+  |
|                                                                     |
+---------------------------------------------------------------------+
```

## Success Packages (`success.html`)

```
+---------------------------------------------------------------------+
|  < Back to Dashboard                                                |
|                                                                     |
|  Successful Packages (138 total)                                    |
+---------------------------------------------------------------------+
|                                                                     |
|  Filters:                                                           |
|  +-------------+ +-------------+ +-------------+                   |
|  | Ecosystem v | | Date From   | | Date To     |      [Apply]     |
|  | (all)       | | 2026-01-01  | | 2026-02-15  |                   |
|  +-------------+ +-------------+ +-------------+                   |
|                                                                     |
|  +---------------------------------------------------------------+ |
|  | Package    | Ecosystem | Generated   | Recipe              |   | |
|  +------------+-----------+-------------+---------------------+   | |
|  | gh         | homebrew  | 2026-02-06  | [View recipe >]     |   | |
|  | [> package detail]                                            | |
|  +------------+-----------+-------------+---------------------+   | |
|  | jq         | homebrew  | 2026-02-06  | [View recipe >]     |   | |
|  | [> package detail]                                            | |
|  +------------+-----------+-------------+---------------------+   | |
|  | ripgrep    | cargo     | 2026-02-05  | [View recipe >]     |   | |
|  | [> package detail]                                            | |
|  +------------+-----------+-------------+---------------------+   | |
|  | ...        |           |             |                     |   | |
|  +---------------------------------------------------------------+ |
|                                                                     |
|  +- Success Timeline ---------------------------------------------+|
|  |                                                               |  |
|  |  Feb 1  ########....  12 recipes                              |  |
|  |  Feb 2  ######......   8 recipes                              |  |
|  |  Feb 3  ############. 15 recipes                              |  |
|  |  ...                                                          |  |
|  |  Feb 6  ##..........   2 recipes (last success)               |  |
|  |  Feb 7  ............   0 recipes                              |  |
|  |  ...                                                          |  |
|  |  Feb 15 ............   0 recipes                              |  |
|  |                                                               |  |
|  +---------------------------------------------------------------+  |
|                                                                     |
+---------------------------------------------------------------------+
```
