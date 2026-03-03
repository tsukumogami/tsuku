# Issue 2076 Implementation Plan

## Summary

Add Twitter Card meta tags to the blog's base template and replace the placeholder OG image with a proper 1200x630px visual. Hugo's internal OG template already handles the `images` frontmatter field and site-level fallback, so the main gaps are: Twitter Cards aren't included, and the default OG image is a 3.1KB placeholder despite already being 1200x630px.

## Approach

Hugo ships a built-in `_internal/twitter_cards.html` partial that reuses the same `get-page-images` logic as the OG template. Adding one line to `baseof.html` covers all Twitter Card requirements. The `images` frontmatter field is already natively supported by Hugo's OG template - no template changes are needed for per-post images.

The OG image needs to be replaced with one that visually represents tsuku (the current 1200x630 PNG is the right dimensions but 3.1KB suggests it contains minimal content). The new image should be committed to `website/assets/` to keep assets co-located with the website.

### Alternatives Considered

- **Custom OG/Twitter partial**: Writing a custom partial instead of using Hugo's built-in ones. Not chosen because Hugo's internals already handle the full OG and Twitter Card spec correctly, including the `images` frontmatter fallback chain.
- **Per-post template injection**: Adding meta tags only in `single.html`. Not chosen because the base template already includes OG tags for all pages; Twitter Cards belong there too for consistency.

## Files to Modify

- `blog/layouts/_default/baseof.html` - Add `{{ template "_internal/twitter_cards.html" . }}` after the existing OG template include
- `website/assets/og-default.png` - Replace with a proper 1200x630px image that visually represents tsuku (currently 3.1KB, which suggests it's a minimal placeholder)

## Files to Create

None. Hugo's built-in OG and Twitter Card templates handle all the logic. No new partials or layouts are needed.

## Implementation Steps

- [x] Replace `website/assets/og-default.png` with a proper 1200x630px image that visually represents tsuku (the tsuku name/logo on a styled background)
- [x] Add `{{ template "_internal/twitter_cards.html" . }}` to `blog/layouts/_default/baseof.html` after the existing OG template line
- [x] Add an example `images` field (commented out) to `blog/content/posts/hello-world.md` frontmatter, or document the pattern in a code comment, so future authors know per-post images are supported
- [x] Verify `hugo.toml` `params.images` points to the correct absolute URL for the deployed image (currently `https://tsuku.dev/assets/og-default.png`)

## Success Criteria

- [ ] `baseof.html` includes `_internal/twitter_cards.html`
- [ ] Built HTML for a blog post contains `twitter:card`, `twitter:title`, `twitter:description`, and `twitter:image` meta tags
- [ ] Built HTML for a blog post contains `og:image` pointing to the default OG image URL
- [ ] `og-default.png` is visually representative of tsuku (not a blank or placeholder image)
- [ ] A blog post with `images: ["path/to/image.png"]` in frontmatter produces `og:image` and `twitter:image` pointing to that post-specific image
- [ ] A blog post without `images` frontmatter falls back to the site-level default from `hugo.toml`

## Open Questions

None blocking. The `images` frontmatter field is already supported by Hugo's internal template - no changes needed to support per-post images beyond authors knowing the field exists.
