# Maintainer Review: Issue #1977 - Add blog link to site navigation

**Reviewer focus:** Can the next developer understand and modify this with confidence?

## Summary

The change adds a "Blog" link to the header navigation (`.nav-links`) and footer across all four HTML pages (index, recipes, telemetry, 404), plus a `blog.css` file for styling the blog pages themselves. The nav/footer HTML is copy-pasted identically in all four files, and the link points to `/blog/`.

## Findings

### 1. Blog link targets a gitignored, non-existent directory

**Files:** `website/index.html:15`, `website/recipes/index.html:15`, `website/telemetry/index.html:15`, `website/404.html:30`, plus all footer instances.

All eight link instances point to `/blog/`, but:
- There is no `website/blog/` directory in the repository.
- `website/.gitignore` line 3 explicitly excludes `blog/`.
- The `_redirects` file has no routing rule for `/blog/*`.

The next developer will see a Blog link in the nav, click it locally, get a 404, and have no way to figure out where the blog content comes from or how it gets deployed. There's no comment in any file explaining that `blog/` is generated at deploy time (similar to how `recipes.json` is generated and also gitignored). A developer trying to fix a blog-related bug or add a new blog post will be stuck.

**Recommendation:** Add a brief comment in `website/.gitignore` explaining the generation mechanism (e.g., `# Generated during deployment by <script/pipeline>`), or add a note in the website's `CLAUDE.local.md` under Key Files. The `recipes.json` entry in `.gitignore` has no comment either, but it at least has a matching generation flow visible in the codebase. For `blog/`, the generation source is invisible.

**Severity: Advisory.** The link won't break in production (presumably the deploy pipeline populates it), but a local dev will be confused. Not blocking because the gitignore pattern at least signals "generated content."

### 2. Duplicated nav/footer HTML across four files with no shared template

**Files:** `website/index.html:11-23`, `website/recipes/index.html:11-23`, `website/telemetry/index.html:11-23`, `website/404.html:26-38` (header); plus corresponding footer blocks.

The header nav block (logo + nav-links div with Blog link + GitHub SVG icon) is duplicated verbatim across all four HTML files. The footer block (Blog | Privacy | GitHub) is similarly duplicated in all four files.

This is a pre-existing pattern (the GitHub icon was already duplicated), and the commit simply adds `<a href="/blog/">Blog</a>` consistently to all copies. The duplication itself predates this PR.

However, this commit makes the duplication worse: previously the nav had only one text link (the GitHub icon). Now there are two items, and the next time someone adds a nav link, they need to remember to update four files. The divergent-twins risk increases with each new nav item.

**Severity: Advisory.** The current copies are identical, so no immediate misread risk. But this is the kind of thing that silently diverges over time. The website CLAUDE.local.md says "No build step: Static files only," so a shared template system would be a larger change outside the scope of this issue.

### 3. blog.css exists but is not referenced by any HTML file

**File:** `website/assets/blog.css`

The CSS file defines styles for `.blog-post`, `.blog-index`, `.blog-entry`, etc. None of the four HTML files in this commit include a `<link>` to `blog.css`. The blog pages themselves (which would presumably `<link>` to this file) don't exist in the repository.

The next developer who sees `blog.css` in `website/assets/` might wonder if it's dead code. Since the blog HTML is generated and gitignored, the stylesheet is effectively an orphan in the tracked codebase.

**Severity: Advisory.** The generated blog pages presumably reference this file. But a developer running `grep -r blog.css website/` on tracked files will find zero references and may conclude it's unused.

### 4. Footer "Privacy" link inconsistency with nav links

**Files:** All four footers, e.g., `website/index.html:91`

The footer contains `<a href="/telemetry">Privacy</a>` without a trailing slash, while the Blog link uses `/blog/` with a trailing slash, and the recipes link in the main page uses `/recipes/`. The telemetry page itself lives at `website/telemetry/index.html`.

This is a pre-existing inconsistency (not introduced by this commit), but the next developer adding links will see conflicting patterns and won't know which convention to follow.

**Severity: Out of scope** (pre-existing, not introduced by this change).

## Overall Assessment

The change is straightforward and applied consistently across all files. The main concern is that `/blog/` is a link to generated content that doesn't exist in the repo, with no documentation trail explaining how it materializes. For a static site with no build step, having navigation that points to content from an invisible pipeline is a small documentation gap. No blocking issues.
