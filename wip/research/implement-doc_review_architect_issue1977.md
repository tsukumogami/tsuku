# Architect Review: Issue #1977 - Add blog link to site navigation

## Review Scope

Commit: b7f6b6bb
Files changed: `website/index.html`, `website/recipes/index.html`, `website/telemetry/index.html`, `website/404.html`, `website/assets/style.css`

## Summary

The change adds a "Blog" link to the header navigation and footer of four website pages. It follows the existing HTML/CSS patterns correctly -- the blog link sits inside the `<div class="nav-links">` wrapper alongside the GitHub icon, and uses the existing `.nav-links a` CSS rules for styling. No new CSS rules were required for the nav link itself. The `website/assets/blog.css` file already existed for blog-specific page styles.

## Findings

### 1. Incomplete nav update across pages (Advisory)

**Files:** `website/stats/index.html`, `website/pipeline/*.html`

The commit updated 4 pages (index, recipes, telemetry, 404) but did not update:
- `website/stats/index.html` -- uses an older nav pattern (no `<div class="nav-links">` wrapper, no blog link, no blog in footer)
- `website/pipeline/*.html` (12 files) -- same older nav pattern, no blog link

The stats and pipeline pages have a structurally different nav:
```html
<!-- stats/pipeline pattern (OLD) -->
<nav>
    <a href="/" class="logo">tsuku</a>
    <a href="..." class="github-link">...</a>
</nav>
```

vs the updated pages:
```html
<!-- index/recipes/telemetry/404 pattern (NEW) -->
<nav>
    <a href="/" class="logo">tsuku</a>
    <div class="nav-links">
        <a href="/blog/">Blog</a>
        <a href="..." class="github-link">...</a>
    </div>
</nav>
```

This creates an inconsistency where some pages have the blog link and the `nav-links` wrapper div, while others do not. The root cause is pre-existing -- the website uses duplicated HTML with no shared template system -- so bringing all pages to the new pattern was not trivial. But the divergence means users navigating from stats or pipeline pages won't see the blog link.

**Severity:** Advisory. This doesn't create a structural problem that compounds; it's an incomplete rollout across a template-less static site. The 4 updated pages are consistent with each other and are the primary user-facing pages. The stats and pipeline pages are secondary.

### 2. No new CSS rules needed (Positive)

The blog link in the nav inherits styling from the existing `.nav-links a` rule at `website/assets/style.css:60-65`. No parallel styling pattern was introduced. The change correctly relies on the existing cascade.

### 3. Footer pattern consistency (Positive)

The footer across all 4 updated pages follows an identical pattern:
```html
<footer>
    <p>
        <a href="/blog/">Blog</a>
        <span class="sep">|</span>
        <a href="/telemetry">Privacy</a>
        <span class="sep">|</span>
        <a href="https://github.com/tsukumogami/tsuku">GitHub</a>
    </p>
</footer>
```

This is consistent and doesn't introduce a new structural pattern.

## Blocking Issues

None.

## Advisory Issues

1. Stats page (`website/stats/index.html`) and pipeline pages (`website/pipeline/*.html`) were not updated with the blog link or the `nav-links` wrapper pattern. Consider updating these pages for consistency in a follow-up.
