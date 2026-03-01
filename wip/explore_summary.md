# Exploration Summary: Blog Infrastructure

## Problem (Phase 1)

tsuku.dev is a static HTML site with no build system. Adding a blog section requires introducing a markdown-to-HTML pipeline while keeping the result visually consistent with the existing dark theme. The current deployment workflow deploys the `website/` directory as-is to Cloudflare Pages with only a Python script generating `recipes.json`.

## Decision Drivers (Phase 1)

- No build system exists today; any SSG is a new dependency
- Site uses custom CSS variables and system font stack -- blog must inherit these
- Deployment is via Cloudflare Pages direct upload from GitHub Actions
- OpenGraph meta tags needed for social sharing (LinkedIn)
- Adding a post should be as simple as committing a markdown file
- A hello world post is needed to validate the infrastructure works

## Research Findings (Phase 2)

- Hugo: Single Go binary, built-in OG templates, existing HTML goes in static/, mature ecosystem
- Zola: Single Rust binary, but deletes output dir on rebuild, no built-in OG, smaller community
- Custom Python: ~80 lines, Python already in CI, but every feature needs manual implementation
- Custom Go: ~225 lines, fits monorepo, but same maintenance burden as Python approach
- Existing site: pure static, CSS vars in :root, system font stack, no build step, deploy-website.yml workflow

## Options (Phase 3)

- Build tool: Hugo (Go SSG) vs Zola (Rust SSG) vs Custom Python/Go script
- Site structure: Hugo manages whole site vs Hugo scoped to blog/ subdirectory
- OpenGraph: Hugo built-in template vs Custom meta tags in template

## Decision (Phase 5)

**Problem:**
tsuku.dev needs a blog section but has no build system. The site is pure static HTML with a custom dark theme deployed directly to Cloudflare Pages. Adding blog support means introducing a markdown-to-HTML pipeline that integrates with the existing deployment workflow, inherits the site's CSS, and produces OpenGraph meta tags for social sharing -- without disrupting the current pages.

**Decision:**
Use Hugo as a blog-only build tool, with source in `blog/` at the repo root and generated output written to `website/blog/`. Hugo converts markdown posts to static HTML using templates that reference the shared `/assets/style.css` stylesheet. Blog-specific styles live in `website/assets/blog.css`. Blog posts use YAML frontmatter for metadata, and Hugo's built-in OpenGraph template generates social sharing tags automatically. The CI pipeline installs Hugo via checksum-verified `.deb` from GitHub releases and builds the blog into the deployment directory alongside existing pages. A hello world post validates the full pipeline.

**Rationale:**
Hugo provides built-in support for OpenGraph, draft handling, date formatting, and content organization that would otherwise require custom code. As a single Go binary, it adds no runtime dependencies and fits the project's Go-based toolchain. Scoping Hugo to a blog subdirectory avoids restructuring the existing site, which would be a high-risk migration for no benefit. Zola was rejected because it destroys its output directory on each build, requiring an extra merge step. Custom scripts were rejected because they trade Hugo's mature features for initial simplicity that doesn't hold up as blog requirements grow.

## Current Status
**Phase:** 7 - Security (complete)
**Last Updated:** 2026-03-01
