# Scrutiny Review: Intent - Issue #1977

## Issue: Add blog link to site navigation

## Sub-check 1: Design Intent Alignment

The design doc's Phase 4 says:
- "Add 'Blog' link to the site's navigation (in existing pages that should link to the blog)"
- "Update footer links if appropriate"

The implementation adds Blog links to nav and footer of all four user-facing pages (index.html, recipes/, telemetry/, 404.html). The design doc's issue table confirms the scope: "Adds 'Blog' links to the nav and footer of user-facing pages (`index.html`, `recipes/`, `telemetry/`, `404.html`)."

The nav structure introduces a `.nav-links` wrapper div around Blog link + GitHub icon. The design doc's baseof.html example shows the GitHub icon placed directly in the nav (no wrapper), but the main site pages now wrap it in a flex container to group Blog + GitHub. This is a necessary structural change to add a text link alongside the existing icon -- it's the simplest correct approach for the layout requirement and aligns with AC5 ("Between logo and GitHub icon via .nav-links flex container").

The blog template's own nav (baseof.html) was intentionally left unchanged per AC8. The blog pages will have a slightly different nav structure (no `.nav-links` wrapper, no Blog link) than the main site pages. This is an acceptable divergence: the blog is a separate build system (Hugo) and adding a "Blog" link from within the blog itself would be redundant.

Footer links are consistent across all four pages: Blog | Privacy | GitHub.

**Finding: None.** Implementation matches design intent.

## Sub-check 2: Cross-issue Enablement

Issue #1977 has no downstream dependents. Skipped.

## Backward Coherence

Previous issues (#1974, #1975, #1976) established the Hugo project structure, content, and CI pipeline. This issue adds navigation links to existing pages, which is orthogonal to those changes. No conflicts with prior work.

The nav structure divergence between main site pages (with `.nav-links`) and blog template (without) is a minor inconsistency that could be harmonized later, but it's not a contradiction -- the blog template was written before this nav pattern was introduced, and the issue explicitly excludes blog template changes.

**Finding: None.**

## Summary

No blocking or advisory findings. The implementation is the simplest correct approach to the stated requirements: Blog links added to nav and footer of four pages, CSS for layout, nothing else touched.
