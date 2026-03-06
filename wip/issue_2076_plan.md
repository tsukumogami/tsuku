# Issue 2076 Implementation Plan

## Summary

Add Twitter Card meta tags via a Hugo partial template and replace the blank OG default image with a branded one. Hugo's internal OG template already handles per-post `images` frontmatter, so that works out of the box.

## Approach

Hugo's `_internal/opengraph.html` already reads `.Params.images` for per-post OG images and falls back to `.Site.Params.images`. We only need to:
1. Add a Twitter Card partial (Hugo's internal template doesn't cover twitter:* tags)
2. Replace the blank placeholder OG image with a branded one
3. Verify the `images` frontmatter field works with a blog post example

### Alternatives Considered
- **Hugo's internal twitter_cards.html**: Hugo has `_internal/twitter_cards.html` but it requires `[social]` config and `.Site.Params.social.twitter` which adds unnecessary config. A custom partial gives us more control with less config.
- **Generate OG images at build time**: Tools like `tcardgen` or custom Hugo shortcodes could generate per-post images. Overkill for now - a good default image plus optional per-post override covers the need.

## Files to Modify
- `blog/layouts/_default/baseof.html` - Add twitter card partial include
- `blog/hugo.toml` - No changes needed (images param already set)
- `website/assets/og-default.png` - Replace with branded image

## Files to Create
- `blog/layouts/partials/twitter-card.html` - Twitter Card meta tags partial

## Implementation Steps
- [x] Create Twitter Card partial template
- [x] Add partial include to baseof.html
- [x] Generate and replace OG default image with branded version
- [ ] Verify hello-world post frontmatter supports images field (add example comment)

## Testing Strategy
- Manual: Share a built blog URL through social media preview validators (opengraph.dev, cards-dev.twitter.com)
- Structural: Verify Hugo template syntax is correct by reviewing against Hugo docs
- CI: Hugo build in CI will catch template syntax errors

## Risks and Mitigations
- **Image too large**: Keep PNG under 300KB (social platforms have size limits around 5-8MB, so this is safe)
- **Hugo version compatibility**: Using standard Hugo template functions (.Params, .Site.Params, .Permalink) which are stable across versions

## Success Criteria
- [ ] og-default.png shows tsuku branding at 1200x630px
- [ ] Twitter Card meta tags rendered in blog post HTML
- [ ] Per-post images field supported via Hugo's built-in OG template
- [ ] Posts without custom image fall back to default

## Open Questions
None - all acceptance criteria are clear and implementable.
