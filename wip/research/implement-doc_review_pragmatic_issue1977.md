# Pragmatic Review: Issue #1977 (Add blog link to site navigation)

## Findings

### 1. Dead CSS file with zero consumers (Blocking)

`/home/dangazineu/dev/workspace/tsuku/tsuku-4/public/tsuku/website/assets/blog.css` -- This file defines `.blog-post`, `.blog-index`, `.blog-entry`, and related styles, but no HTML file in the repo links to it (`<link rel="stylesheet" href="/assets/blog.css">`). The blog directory itself is gitignored and doesn't exist. This is speculative generality: styles for a page that isn't part of this PR. If a future blog PR needs these styles, it should ship them with the blog templates. Remove `blog.css`.

### 2. Inconsistent nav update -- stats page missed (Advisory)

`/home/dangazineu/dev/workspace/tsuku/tsuku-4/public/tsuku/website/stats/index.html` -- The nav and footer were updated in 4 HTML files but `stats/index.html` still has the old nav (no Blog link, no `.nav-links` wrapper). If the issue scope is "add blog link to site navigation," this page was missed. Not blocking because the stats page already had a divergent nav structure, but worth noting for consistency.

## Summary

- Blocking: 1 (dead `blog.css` file)
- Advisory: 1 (stats page nav inconsistency)

The nav/footer link additions across the 4 HTML files are straightforward and correctly scoped. The `.gitignore` addition for `blog/` is fine -- it matches the existing pattern for `recipes.json` (generated during deployment).
