# Architecture Review: Blog Infrastructure Design

**Reviewer:** architect-reviewer
**Design:** `/home/dangazineu/dev/workspace/tsuku/tsuku-4/public/tsuku/docs/designs/DESIGN-blog-infrastructure.md`
**Date:** 2026-03-01

---

## 1. Is the architecture clear enough to implement?

**Yes, with a few gaps that need closing before implementation.**

The design is well-structured. The directory layout, data flow, template examples, and CI integration steps are all concrete enough to code against. The decision structure (build tool, site structure, OpenGraph) is explicit about what was chosen and why, which reduces ambiguity during implementation.

The architecture follows the existing site pattern: source files live outside `website/`, a build step generates artifacts into `website/`, and `website/` is deployed as a unit. This matches `scripts/generate-registry.py` writing `recipes.json` into `website/`. No parallel pattern is introduced.

Gaps that need closing are enumerated below.

---

## 2. Missing components or interfaces

### 2a. Hugo `--destination` with relative path (Advisory)

The design specifies:

```bash
hugo --source blog --destination website/blog
```

Hugo's `--destination` flag interprets relative paths relative to the `--source` directory, not the working directory. When `--source` is `blog/`, a `--destination` of `website/blog` resolves to `blog/website/blog`, not `./website/blog`.

The correct invocation from the repo root is one of:

```bash
hugo --source blog --destination ../website/blog
```

or using an absolute path:

```bash
hugo --source blog --destination "$GITHUB_WORKSPACE/website/blog"
```

This appears in the design three times (solution architecture, data flow, local development note). All three need the same fix. **This is a build-breaking issue** -- if implemented as written, Hugo will create output in the wrong location and no blog content will be deployed.

### 2b. `baseURL` and `--destination` interaction (Advisory)

The `hugo.toml` sets:

```toml
baseURL = "https://tsuku.dev/blog/"
```

This is correct for the deployment scenario. Hugo will generate absolute links like `https://tsuku.dev/blog/posts/hello-world/` and relative resource references rooted at `/blog/`. Since the output lands in `website/blog/`, the URL paths align with the file paths on Cloudflare Pages. No issue here.

### 2c. Workflow path triggers need `blog/**` (Blocking for CI)

The existing `deploy-website.yml` triggers on pushes to specific paths:

```yaml
paths:
  - 'website/**'
  - 'internal/recipe/recipes/**/*.toml'
  - 'recipes/**/*.toml'
  - 'scripts/generate-registry.py'
  - '.github/workflows/deploy-website.yml'
```

The design adds Hugo source files in `blog/` at the repo root, but does not mention adding `blog/**` to the workflow's path triggers. Without this, committing a new blog post to `blog/content/posts/` will not trigger a deployment. The design's Phase 3 says "Update `deploy-website.yml`" but only mentions adding the Hugo install and build steps, not updating the `paths` filter.

Add to the implementation plan:

```yaml
paths:
  - 'blog/**'
```

### 2d. `website/blog/` gitignore placement (Advisory)

The design says to add `website/blog/` to `.gitignore`. The repo has two gitignore files:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-4/public/tsuku/.gitignore` (root)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-4/public/tsuku/website/.gitignore` (website-specific, currently only contains `recipes.json`)

The website-specific gitignore already follows the pattern of ignoring generated artifacts (`recipes.json`). Adding `blog/` there is the cleaner choice -- it keeps the "generated during deployment" contract in one place. The root gitignore is for build artifacts, IDE files, and OS artifacts.

The design should specify that `blog/` goes in `website/.gitignore`, not the root `.gitignore`. This mirrors the existing `recipes.json` pattern exactly.

### 2e. No `_redirects` or `_headers` updates mentioned

The existing `_redirects` file handles SPA routing for `/recipes/*` paths. The blog uses Hugo's pretty URLs (`/blog/posts/hello-world/` -> `index.html`), so no redirect rules are needed -- Cloudflare Pages serves `index.html` from directories by default. This is fine.

However, the design does not mention whether `_headers` needs a cache policy for blog content. The existing site has specific caching for `install.sh`. Blog posts are immutable once published, so a longer cache TTL could be set. Not blocking, but worth noting in implementation.

### 2f. Missing `og-default.png` asset

The `hugo.toml` references `https://tsuku.dev/assets/og-default.png` as the fallback OG image, but the design doesn't include creating this image as a deliverable. If the file doesn't exist at deploy time, social sharing previews will show a broken image link. Either create a placeholder image or remove the default from `hugo.toml` until one is ready.

---

## 3. Implementation phases: sequencing

The four phases are:

1. Hugo project setup (templates, CSS)
2. Content and validation post
3. CI pipeline integration
4. Navigation and links

**Sequencing is correct.** Templates must exist before content can be tested (Phase 1 before 2). CI can only be configured once local builds work (Phase 2 before 3). Navigation links should come last since they touch existing pages (Phase 4).

One ordering improvement: `website/assets/blog.css` is listed in Phase 1, but the gitignore update for `website/blog/` is in Phase 3. The gitignore should move to Phase 1 -- as soon as someone runs the local build command from Phase 2, `website/blog/` will be created and show up as untracked files. Having the gitignore in place first prevents accidental commits of generated output.

---

## 4. Simpler alternatives

The design already evaluates three alternatives (Zola, custom Python, custom Go) and rejects them with specific reasoning. The evaluation is fair.

One alternative not considered: **using Hugo as a Go module dependency** rather than installing it via `.deb` in CI. Since tsuku is already a Go project, Hugo could be invoked as a library. However, Hugo's library API is not stable or intended for embedding, so this would be more fragile than the binary approach. The `.deb` installation is the right call.

Another unconsidered option: **GitHub Pages + Jekyll** integration built into GitHub. Rejected implicitly by the Cloudflare Pages deployment, which is already established. Not worth mentioning in the design.

The design's choice is the right one for this context. Hugo adds toolchain weight but the alternative (custom code) creates a maintenance surface that grows with each blog feature. For a project that expects ongoing posts, the investment is justified.

---

## 5. Specific technical checks

### 5a. Hugo config (`baseURL`, `publishDir`)

The design uses `baseURL = "https://tsuku.dev/blog/"` in `hugo.toml` and the `--destination` CLI flag to control output location. There is no `publishDir` in the config, which is fine -- `--destination` overrides it at build time.

The `baseURL` trailing slash is important. Hugo normalizes URLs based on `baseURL`, and the trailing slash ensures generated links like `{{ .Permalink }}` produce `https://tsuku.dev/blog/posts/hello-world/` rather than `https://tsuku.dev/blogposts/hello-world/`. This is correct.

However, the `single.html` template has a hardcoded back link:

```html
<a href="/blog/">Back to blog</a>
```

This works in production but will break during `hugo server` local development because the dev server mounts at the root. Use Hugo's `relref` or just `{{ .Site.BaseURL }}` instead:

```html
<a href="{{ "posts/" | absURL }}">Back to blog</a>
```

Or accept the hardcoded path as a pragmatic choice since local dev already has the CSS limitation noted in the design. **Advisory.**

### 5b. Gitignore strategy

As noted in 2d, the cleanest approach is adding `blog/` to `website/.gitignore` alongside the existing `recipes.json` entry. This keeps all "generated during deployment" ignores in one file.

The design's approach of gitignoring `website/blog/` will work regardless of which gitignore file it goes in. The key point is that it must be ignored -- committing generated HTML would create merge conflicts on every blog change. The design correctly calls this out.

### 5c. Template correctness

**`baseof.html`**: Correct. Uses `{{ .Description | default .Site.Params.description }}` for the meta description, `{{ .Title }}` for the page title, `{{ block "main" . }}{{ end }}` for content injection, and `{{ template "_internal/opengraph.html" . }}` for OG tags. The HTML structure (header/nav/main/footer) matches the existing site pages exactly.

**`single.html`**: Correct. `{{ define "main" }}` matches the block in `baseof.html`. Date formatting uses Go's reference time correctly (`"2006-01-02"` and `"January 2, 2006"`). `{{ .Content }}` renders the markdown body.

**`list.html`**: Mostly correct but has a subtlety. The template uses `{{ range .Pages }}` to iterate posts. For a section list template at `layouts/posts/list.html`, `.Pages` returns the direct pages in the `posts` section. This works for a flat blog structure. If nested sections are added later, `.Pages` won't recurse -- `.RegularPages` would be needed instead. For the current single-level structure, `.Pages` is fine.

The `{{ .Description | default .Summary }}` fallback in `list.html` is a good pattern. Hugo auto-generates `.Summary` from the first ~70 words if no explicit summary delimiter is set.

**One omission**: None of the templates set `{{ .Scratch }}` or use `.Hugo` deprecated variables. The templates use only current Hugo APIs. Clean.

### 5d. Hello world post

The hello world post at `blog/content/posts/hello-world.md` has:

```yaml
---
title: "Hello, World"
date: 2026-03-01
description: "The tsuku blog is live. Here's what we'll be writing about."
---
```

This is sufficient for validation. It exercises:
- Frontmatter parsing (title, date, description)
- Template rendering (single.html)
- Index generation (list.html showing one post)
- OG tag generation (title and description populated)
- CSS integration (dark theme via shared stylesheet)

The post doesn't include an `images` field, which means the OG image will fall back to the site-level `og-default.png`. If that file doesn't exist (see 2f), the OG image tag will reference a 404. For validation purposes, either create the default image or add an `images` field to the hello world post.

The design does not show any markdown body content for the hello world post. The frontmatter-only snippet above would produce a post with a title, date, and empty body. The implementer should include at least a paragraph or two to verify that `{{ .Content }}` renders correctly and the blog CSS styles (line-height, code blocks, headings) work as intended.

---

## 6. Structural fit assessment

### Fits the existing architecture

- **Source-outside-output pattern**: `blog/` source outside `website/` matches `scripts/generate-registry.py` writing into `website/`. No new pattern introduced.
- **Static deployment model**: Generated HTML merges into `website/` and deploys via the existing Cloudflare Pages upload. No new deployment mechanism.
- **CSS inheritance via absolute paths**: Blog templates reference `/assets/style.css`, same as every existing page. No stylesheet duplication.
- **CI pipeline extension**: New steps insert before the existing deploy step, same as the Python registry generation. Pipeline structure preserved.

### Does not introduce parallel patterns

- No second CSS framework or theme system
- No second deployment pipeline
- No second content management approach for the existing pages
- No Node.js or npm dependency chain alongside the Go toolchain

### Structural risk: template divergence from existing pages

The `baseof.html` template duplicates the header/nav/footer HTML from the existing static pages. If the site nav changes (adding a "Blog" link in Phase 4, for example), both the Hugo template and every existing HTML page need updating. This is a known trade-off of not migrating the entire site to Hugo, and the design acknowledges it implicitly.

This is acceptable because the site has very few pages (landing, recipes, telemetry, 404, pipeline pages) and changes to the nav/footer are infrequent. The duplication is bounded and unlikely to diverge silently.

---

## Summary of findings

| # | Finding | Severity | Location in design |
|---|---------|----------|--------------------|
| 1 | `--destination website/blog` resolves relative to `--source`, not cwd. Will create output in `blog/website/blog/` instead of `./website/blog/`. Use `--destination ../website/blog`. | **Blocking** | Solution Architecture, Data Flow, Phase 2 |
| 2 | `deploy-website.yml` path triggers need `blog/**` added, or blog post commits won't trigger deployment. | **Blocking** | Phase 3 (omitted) |
| 3 | `og-default.png` referenced in `hugo.toml` but not included as a deliverable. Social previews will show broken image. | **Advisory** | OpenGraph section |
| 4 | Gitignore for `website/blog/` should go in `website/.gitignore` (alongside `recipes.json`) rather than root `.gitignore`. | **Advisory** | Phase 3 |
| 5 | Gitignore update should move to Phase 1 (before local builds create `website/blog/`). | **Advisory** | Phase 3 |
| 6 | Hello world post needs markdown body content to validate CSS styling of rendered content. | **Advisory** | Hello World section |
| 7 | Hardcoded `/blog/` back link in `single.html` breaks under `hugo server`. Acceptable if documented. | **Advisory** | Template Structure |
