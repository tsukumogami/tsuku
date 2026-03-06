# Issue 2076 Summary

## What Was Implemented

Added Twitter Card meta tags and replaced the blank OG default image with a branded version. Hugo's existing internal OG template already supports per-post images via frontmatter, so no changes were needed for that capability.

## Changes Made
- `blog/layouts/partials/twitter-card.html`: New partial that emits twitter:card, twitter:title, twitter:description, and twitter:image meta tags
- `blog/layouts/_default/baseof.html`: Added partial include for twitter-card.html
- `website/assets/og-default.png`: Replaced blank dark rectangle with branded image (tsuku title, tagline, URL using site theme colors)

## Key Decisions
- Custom partial over Hugo's `_internal/twitter_cards.html`: Avoids requiring `[social]` config block and gives direct control over output
- `summary_large_image` card type: Best for blog posts where the image should be prominent
- Image fallback logic mirrors Hugo's internal OG template: `.Params.images` -> `.Site.Params.images`

## Trade-offs Accepted
- Static default image rather than per-post generation: Simpler, no build-time dependencies. Per-post generation can be added later if needed.

## Test Coverage
- No automated tests (Hugo template changes verified by CI build)
- Template syntax follows standard Hugo patterns

## Known Limitations
- No `twitter:site` or `twitter:creator` tags (would need Twitter handle config)
- Per-post images must be absolute URLs in frontmatter

## Requirements Mapping

| AC | Status | Evidence / Reason |
|----|--------|-------------------|
| Default OG image is at least 1200x630px and visually represents tsuku | Implemented | website/assets/og-default.png (1200x630, 17KB, branded) |
| Blog posts support optional `images` field in frontmatter for per-post OG images | Implemented | Hugo internal OG template + twitter-card.html both read .Params.images |
| Twitter Card meta tags present | Implemented | blog/layouts/partials/twitter-card.html |
| Sharing a blog post URL shows title, description, and image | Implemented | OG tags (Hugo internal) + Twitter tags (partial) cover all platforms |
| Posts without custom image fall back to default | Implemented | Both templates fall back to .Site.Params.images in hugo.toml |
