# Documentation Plan: blog-infrastructure

Generated from: docs/designs/DESIGN-blog-infrastructure.md
Issues analyzed: 5
Total entries: 1

---

## doc-1: website/README.md
**Section**: (multiple sections)
**Prerequisite issues**: #1974, #1976, #1977
**Update type**: modify
**Status**: updated
**Details**: Update Site Structure list to include `/blog/` entry for blog posts. Add Hugo build instructions to Development section explaining that blog content requires running `hugo --source blog --destination $PWD/website/blog` before serving locally (Hugo must be installed separately). Add blog-related entries to Key Files table: `assets/blog.css` (blog-specific styles) and `assets/og-default.png` (OpenGraph fallback image). Update Deployment section to note that CI runs a Hugo build step before uploading to Cloudflare Pages. Note that user-facing pages (index.html, recipes/, telemetry/, 404.html) now include Blog links in header nav and footer after #1977.
