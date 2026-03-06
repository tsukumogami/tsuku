---
design_rationale: "Improve social sharing experience for blog posts by adding proper OG images and Twitter Card meta tags"
constraints:
  - "Hugo-based blog with static site deployment via Cloudflare Pages"
  - "No build-time image generation - images must be static assets"
  - "Must work with Hugo's internal opengraph template already in baseof.html"
integration_points:
  - "blog/layouts/_default/baseof.html - already has Hugo internal OG template"
  - "blog/hugo.toml - defines site params including fallback images array"
  - "website/assets/og-default.png - current placeholder (dark solid rectangle)"
  - "blog/content/posts/*.md - frontmatter needs images field support"
exit_criteria:
  - "Default OG image is 1200x630px and visually represents tsuku"
  - "Blog posts support optional images field in frontmatter"
  - "Twitter Card meta tags present"
  - "Posts without custom image fall back to default"
---

## Key Findings

1. `baseof.html` already calls `{{ template "_internal/opengraph.html" . }}` which handles OG tags
2. Hugo's internal OG template uses `.Params.images` (page-level) then `.Site.Params.images` (site-level) for og:image
3. The current `og-default.png` is 1200x630px but is a blank dark rectangle - needs visual content
4. Twitter Card meta tags are NOT included by Hugo's internal OG template - need a custom partial
5. Site uses dark theme: bg `#0d1117`, accent `#58a6ff`, text `#c9d1d9`
