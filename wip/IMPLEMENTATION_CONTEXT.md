## Goal

Make blog post URLs produce rich thumbnails when shared on social media, Slack, Discord, and other platforms.

## Context

The blog already includes Hugo's internal OpenGraph template in `baseof.html`, and `hugo.toml` defines a fallback image (`og-default.png`). But the current setup doesn't produce good social previews:

- The default `og-default.png` is 3.1KB and likely too small for social platforms (recommended: 1200x630px)
- Blog posts have no way to specify per-post images via frontmatter
- Twitter/X Card meta tags aren't included (Hugo's internal OG template doesn't cover these)

## Acceptance Criteria

- [ ] Default OG image is at least 1200x630px and visually represents tsuku
- [ ] Blog posts support an optional `images` field in frontmatter for per-post OG images
- [ ] Twitter Card meta tags are present (`twitter:card`, `twitter:title`, `twitter:description`, `twitter:image`)
- [ ] Sharing a blog post URL on a social platform shows: title, description, and image
- [ ] Posts without a custom image fall back to the default

## Dependencies

None
