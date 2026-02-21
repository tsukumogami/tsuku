# Pipeline Dashboard QA Findings

QA exploration of https://tsuku.dev/pipeline/ performed 2026-02-21.

Four independent agents analyzed the dashboard from different angles: live index page, live list pages, live detail pages with real data, and source code analysis of all 12 HTML files. Findings were deduplicated and merged below.

## Issue Groups

Each group becomes one GitHub issue. Groups are ordered by priority.

---

### Group 1: Broken cross-links between failures, runs, and packages

**Severity: high** -- multiple navigation paths lead to "not found" pages.

| # | Finding | Pages | Root Cause |
|---|---------|-------|------------|
| 1a | `failed.html` does not exist; "failed" status bar on index is a 404 | `index.html:542` | `failed.html` was deleted; the status bar still links to it |
| 1b | `health.last_run.batch_id` is date-only (`2026-02-21`), so Pipeline Health "Last Run" and "Last Success" links always show "Run not found" | `index.html:495,502` | `dashboard.json` stores a date-only batch_id in `health.last_run` but `run.html` expects the full timestamp format from `runs[]` |
| 1c | Failure detail "View batch run" link always shows "Run not found" | `failure.html:357,392` | `failure_details[].batch_id` uses prefixed format (`batch-2026-02-21T07-27-58Z`) while `runs[].batch_id` uses unprefixed format (`2026-02-21T07-27-59Z`) with a 1-second offset. Only 9 of 200 records match after stripping the prefix. |
| 1d | `run.html` matches failures by timestamp proximity (120s window), not batch_id | `run.html:431-435` | Consequence of 1c. Fragile: can show wrong failures when two runs complete within 2 minutes. The latest run shows "Failed: 0" in summary but 5 failures in the table below. |
| 1e | Package links for "batch"-ecosystem failures point to non-existent IDs | `failure.html:308-309` | 49 of 200 failure records have `ecosystem: "batch"`. Links construct `package.html?id=batch:ansifilter` but the real queue ID is `homebrew:ansifilter`. |
| 1f | GitHub package links use short name but queue uses full repo path | `failure.html:308-309`, `package.html:606` | Failures store `package: "jq"` but queue ID is `github:jqlang/jq`. Also the reverse: `package.html` extracts `pkgName = "sharkdp/bat"` but failures use `f.package = "bat"`. Affects 9 github-ecosystem failures. |

**Fix approach:**
- Create `failed.html` (queue-status list page, like `pending.html`) or update the link target
- Fix `health.last_run.batch_id` format in `internal/dashboard/dashboard.go` to use full timestamp
- Normalize `failure_details[].batch_id` to match `runs[].batch_id` in the dashboard generator, so `run.html` and `failure.html` cross-links work
- For batch/github ecosystem mismatches: either fix at the data level (store canonical ecosystem + full package ID in failure records) or add fallback lookups in the HTML pages

---

### Group 2: `package.html` broken for `requires_manual` status and blocked dependencies

**Severity: high** -- 2001 packages and all dependency links are broken.

| # | Finding | Pages | Root Cause |
|---|---------|-------|------------|
| 2a | All 2001 `requires_manual` packages show "Package not found" | `package.html:355` | `findPackageInQueue` only searches `['pending', 'blocked', 'failed', 'success']`; `requires_manual` is missing |
| 2b | All "Blocked By" dependency links are broken | `blocked.html:335,365`, `package.html:461` | `blocked_by` array contains bare names (`gmp`) but queue IDs use `ecosystem:name` format (`homebrew:gmp`). Links always resolve to nothing. |
| 2c | No CSS for `.status-requires_manual` badge | `package.html` CSS | Badge renders unstyled even if 2a is fixed |
| 2d | When package not found in queue, Name and Ecosystem render blank | `package.html` `renderStatus()` | `pkg` is null but the `pkgName` extracted from the URL is available and not passed to the renderer |

**Fix approach:**
- Add `'requires_manual'` to the statuses array in `findPackageInQueue`
- Add `.status-requires_manual` CSS class
- Prefix bare dependency names with the parent package's ecosystem (all current blocked packages are `homebrew:`)
- Fall back to `pkgName` and ecosystem-from-ID when `pkg` is null

---

### Group 3: Make data clickable across dashboard pages

**Severity: medium** -- many data elements that should be links are plain text.

| # | Finding | Pages |
|---|---------|-------|
| 3a | Ecosystem names are plain text in all list pages despite supporting `?ecosystem=` filter | `pending.html:339`, `requires_manual.html:339`, `success.html:346`, `failures.html:355` |
| 3b | "Selected Source" and "Alternatives" columns in disambiguations are plain text | `disambiguations.html:371-373` |
| 3c | Index "Needs Review" disambiguation tools are plain text | `index.html:725-730` |
| 3d | Index curated override stats (Total/Valid/Invalid) are not clickable | `index.html:688-700` |
| 3e | Recent Runs panel has no "View all" link (inconsistent with other panels) | `index.html:781-787` |
| 3f | "Runs Since Last Success" link loses ecosystem context | `index.html:506` |
| 3g | Ecosystem tags in run rows are not clickable | `index.html:624-631`, `runs.html:435` |
| 3h | No link to recipe source file from failure detail | `failure.html` Related section |

**Fix approach:**
- Wrap ecosystem text in `<a>` tags that apply the ecosystem filter on the current page
- Make disambiguation source/alternatives link to `package.html` or GitHub repos
- Add `href` to curated stats pointing to `curated.html?status=<value>`
- Add "View all" link to Recent Runs panel header
- Pass ecosystem parameter in "Runs Since" link
- Add recipe file link to failure detail

---

### Group 4: Fix cross-page action link inconsistencies

**Severity: medium** -- action buttons link to wrong targets or are inconsistent.

| # | Finding | Pages |
|---|---------|-------|
| 4a | "File issue" body has wrong dashboard link: `failures.html?id=` (list) instead of `failure.html?id=` (detail) | `failure.html` `buildIssueURL()` |
| 4b | "Retry" workflow links are inconsistent: `batch-generation.yml` on failure/run pages vs `batch.yml` on package page | `failure.html`, `run.html`, `package.html` |
| 4c | "View workflow logs" links to generic Actions page, not the specific run | `run.html` Actions section |
| 4d | `curated.html` "Add Override" and "Remove Override" link to the same URL | `curated.html:311-312` |
| 4e | `failures.html` batch_id filter parameter exists but has no UI control and no page links to it | `failures.html:233-239` |

**Fix approach:**
- Fix the `failures.html` -> `failure.html` typo in issue body template
- Standardize retry workflow link across all pages
- Either populate `workflow_url` in dashboard data or remove the button
- Differentiate curated action URLs (or merge into one button)
- Either add batch_id filter UI or wire `run.html` to link to `failures.html?batch_id=`

---

### Group 5: Data rendering and filter behavior issues

**Severity: low-medium** -- filters misbehave or display misleading data.

| # | Finding | Pages |
|---|---------|-------|
| 5a | `success.html` date filters (from/to) only affect the timeline chart, not the package list | `success.html:259` |
| 5b | `success.html` timeline uses `run.ecosystem` (old format) instead of `run.ecosystems` (new object format) | `success.html:291-327` |
| 5c | `runs.html` "Since last success" filter shows empty table when most recent run succeeded (all current runs) | `runs.html:344-358` |
| 5d | `runs.html` summary cards always show aggregate stats for all runs, ignoring active filters | `runs.html:465` |
| 5e | `blocked.html` top blockers chart ignores the active blocker filter | `blocked.html:393` |
| 5f | `requires_manual.html` doesn't show the "category" field (~15 packages have it) | `requires_manual.html` |
| 5g | `pending.html` "Failures" column is always 0 (pending packages haven't been attempted) | `pending.html` |
| 5h | `run.html` duration field units are ambiguous (small integers treated as seconds, may be minutes) | `run.html` `formatDuration()` |
| 5i | `run.html` platform column shows literal "multiple" instead of expanding the platforms list | `run.html:380` |
| 5j | `failure.html` `message` and `workflow_url` fields are always absent from data | `failure.html`, `internal/dashboard/failures.go` |
| 5k | "Succeeded" label on run detail is misleading; the underlying metric is "merged" (recipes merged) | `run.html` summary |
| 5l | `runs.html` "Ecosystem" column header is singular but can show multiple ecosystems | `runs.html` |
| 5m | "batch" ecosystem in failures filter dropdown is unexplained | `failures.html` |
| 5n | `dashboard.json` `by_tier` priority breakdown data is generated but never displayed | `index.html` |

**Fix approach:**
- Either make date filters apply to the package list or label them as "Timeline date range"
- Use `getEcosystems(run)` helper (already exists in `runs.html`) in `success.html`
- Show a message like "Most recent run succeeded" instead of empty table for since_success
- Pass `filtered` instead of `allRuns` to `renderSummary()`
- Add category column to `requires_manual.html`; remove dead Failures column from `pending.html`
- Clarify duration units; expand "multiple" platforms; rename "Succeeded" to "Merged"
- Either populate `message`/`workflow_url` in the dashboard generator or remove the UI placeholders
- Consider displaying `by_tier` breakdown on the index page

---

### Group 6: Automated dashboard validation tests

**Severity: medium** -- prevents regressions as fixes land.

Currently there are zero automated tests for the pipeline dashboard. The website CI (`website-ci.yml`) only runs ShellCheck on `install.sh`. Dashboard changes deploy via `deploy-website.yml` (Cloudflare Pages) with no validation. All 32 findings above were discovered through manual QA.

**What to build:**

A validation script that checks the dashboard HTML + JSON contract. It runs against three environments:

| Environment | When | Data Source | URL |
|-------------|------|-------------|-----|
| **Local** | During development | Local `dashboard.json` | `http://localhost:8000/pipeline/` |
| **Staging (PR)** | CI on PRs touching `website/pipeline/` or `internal/dashboard/` | PR preview deploy | `https://<branch>.tsuku-dev.pages.dev/pipeline/` |
| **Production** | Post-deploy or scheduled | Live data | `https://tsuku.dev/pipeline/` |

Cloudflare Pages already deploys PR previews via `deploy-website.yml` (`branch: ${{ github.head_ref }}`), so staging URLs are available without additional infrastructure.

**Test categories:**

1. **Link integrity** (catches Groups 1, 2, 4)
   - All status bar links in `index.html` map to existing HTML files
   - `health.last_run.batch_id` format matches `runs[].batch_id` format
   - `failure_details[].batch_id` format matches `runs[].batch_id` format
   - All `blocked_by` entries include ecosystem prefix
   - Package IDs constructed from failure records (`ecosystem:package`) resolve in the queue

2. **Data contract** (catches Groups 1, 5)
   - `dashboard.json` schema: required fields present, no null IDs
   - All statuses in `queue.packages` keys are covered by `package.html`'s lookup list
   - `run.ecosystems` uses the new object format (not the legacy string)
   - `message` and `workflow_url` fields presence tracking (warn if absent)

3. **Page availability** (catches Group 1a)
   - Every page file that status bars or navigation links reference actually exists
   - Each detail page returns content (not error state) for at least one valid ID from the data

**Implementation approach:**

A `scripts/validate-dashboard.sh` script that:
1. Takes `--url` (defaults to local), `--json-only` (skip HTTP checks), or `--json-path` (validate a local file)
2. Fetches `<url>/pipeline/dashboard.json` and validates schema + cross-references
3. Checks each HTML page returns HTTP 200
4. Reports pass/fail with specific findings

CI integration in `website-ci.yml`:
- Add path trigger for `website/pipeline/**` and `internal/dashboard/**`
- Job: build `queue-analytics`, generate `dashboard.json`, run `validate-dashboard.sh --json-path website/pipeline/dashboard.json`
- For PR preview validation: a post-deploy step that runs against the preview URL

---

## Pages Inventory

| Page | Status | Issues |
|------|--------|--------|
| `index.html` | Has issues | 1a, 1b, 3c, 3d, 3e, 3f, 3g, 5n |
| `pending.html` | Minor | 3a, 5g |
| `success.html` | Has issues | 3a, 5a, 5b |
| `blocked.html` | Has issues | 2b, 5e |
| `requires_manual.html` | Minor | 3a, 5f |
| `failures.html` | Minor | 3a, 4e, 5m |
| `failure.html` | Has issues | 1c, 1e, 1f, 3h, 4a, 4b, 5j |
| `package.html` | Has issues | 1f, 2a, 2b, 2c, 2d, 4b |
| `run.html` | Has issues | 1d, 4b, 4c, 5h, 5i, 5k |
| `runs.html` | Minor | 3g, 5c, 5d, 5l |
| `disambiguations.html` | Minor | 3b |
| `curated.html` | Minor | 4d |
| `failed.html` | Missing | 1a |

## What Works Well

- All 12 existing pages load and render from `dashboard.json` without errors
- Edge case handling is consistent (no ID, invalid ID, missing data all show clear error states)
- XSS protection via `esc()` is applied consistently throughout
- Filter state is preserved in URL via `history.pushState` on all list pages
- Navigation (back links, footer links) is consistent across all pages
- Pagination on `failures.html` works correctly
