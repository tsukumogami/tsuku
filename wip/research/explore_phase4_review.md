# Architect Review: DESIGN-blog-infrastructure.md

## Summary

The design is well-structured and makes sound choices for the stated requirements. Hugo scoped to `website/blog/` is the right call. There are three issues that need resolution before implementation: a file collision between Hugo source files and Hugo output in the same `website/blog/` path, a missing `.gitignore` strategy for generated content, and an underspecified CI failure mode. The alternatives analysis is fair and specific, with no strawman options. The problem statement is clear enough to evaluate solutions against.

---

## 1. Problem Statement Specificity

The problem statement is well-scoped. It identifies the concrete gap (markdown-to-HTML conversion for blog posts with OpenGraph support), explains why the current architecture can't handle it (hand-written HTML doesn't scale to blog content), and defines clear boundaries via the In Scope / Out of Scope sections.

One small gap: the problem statement doesn't quantify the expected volume. "Ongoing technical posts" appears only in the rationale section. This matters because the Hugo vs. custom script tradeoff depends on volume. For 3-5 posts total, a custom script is defensible. For a sustained blog with 20+ posts, Hugo's built-in features (date-based ordering, draft handling, tag/category support) justify the learning curve. The design implicitly assumes the latter but should state it.

---

## 2. Missing Alternatives

No significant alternatives are missing. The design covers the three reasonable approaches: established SSG (Hugo), established SSG in a different language (Zola), and custom build scripts (Python and Go variants). Two minor omissions:

- **11ty (Eleventy)**: A JavaScript SSG that could be dismissed on the same grounds as introducing Node.js, which the design already identifies as a decision driver against. Not a gap.
- **Pandoc**: A single binary that converts markdown to HTML. Could serve as a simpler middle ground between Hugo and a custom script. However, it lacks built-in OpenGraph support, index generation, and frontmatter-driven templating -- so it would still require wrapper code. Reasonable to omit.

---

## 3. Rejection Rationale Fairness

The rejections are specific and fair. No strawmen.

**Zola rejection** -- The claim that Zola "deletes and recreates its output directory on each build" is accurate (Zola's `build` command wipes `public/` before writing). The design correctly identifies this as a problem for the merge-into-existing-site workflow. The note about lacking built-in OpenGraph templates is also accurate; Zola expects themes to provide this.

**Custom Python script rejection** -- The ~80-100 line estimate is realistic for a minimal implementation. The rejection rationale (maintenance grows with each feature) is fair and specific, listing RSS, syntax highlighting, and reading time as examples.

**Custom Go tool rejection** -- Same reasoning as Python, applied correctly. The "initial simplicity offset by ongoing maintenance" framing is honest rather than dismissive.

**Hugo-manages-entire-site rejection** -- Correct that migrating existing pages into Hugo's `static/` is high-risk for zero benefit. The existing pages work fine as plain HTML.

**Hugo at repo root rejection** -- Correct that Hugo's directory conventions would conflict with Go's. A `hugo.toml` at the monorepo root alongside `go.mod` would be confusing.

---

## 4. Unstated Assumptions

**A. Hugo output and source files can share the `website/blog/` path.** This is the most consequential unstated assumption. The design puts Hugo source files (hugo.toml, content/, layouts/) in `website/blog/` and then copies Hugo's output *back* into `website/blog/`. After the CI build step, the directory will contain both source files (hugo.toml, content/, layouts/) and generated files (index.html, posts/hello-world/index.html). When Cloudflare Pages uploads `website/`, all of these go to the CDN -- including hugo.toml, raw markdown files, and the layouts/ directory. This is a minor information leak (not a security problem, but untidy). See finding F1 below for details.

**B. Cloudflare Pages direct upload handles nested directories without routing issues.** The existing site uses direct upload (not a build-on-Cloudflare workflow). The design assumes `/blog/` and `/blog/posts/hello-world/` will route correctly. This is correct -- Cloudflare Pages direct upload serves static files by path, and the existing `_redirects` only has rules for `/recipes/*` and `/pipeline/*`. No routing conflict. However, if someone navigates to `/blog` (no trailing slash), Cloudflare should auto-redirect to `/blog/` since `website/blog/` will contain an `index.html`. This is standard Cloudflare Pages behavior.

**C. Hugo's `hugo server` will work for local development with the CSS at `/assets/style.css`.** When running `hugo server` from `website/blog/`, the dev server serves content rooted at `website/blog/`. The stylesheet reference `/assets/style.css` resolves to `website/blog/assets/style.css`, which doesn't exist -- the real file is `website/assets/style.css`. Local preview will render unstyled pages. The design mentions "test locally: `cd website/blog && hugo server`" but doesn't address this. Not blocking for CI deployment (which copies output into the right place), but it affects the author experience.

**D. The deploy workflow's PR behavior.** The current `deploy-website.yml` deploys on both `push` and `pull_request`. On PR, it deploys to a preview branch. The Hugo build step will need to work in both contexts. This should be fine but is worth noting since the design doesn't mention PR preview behavior.

---

## 5. Specific Technical Findings

### F1: Source/output collision in `website/blog/` (Blocking)

The design puts Hugo source files in `website/blog/` and copies Hugo output back into the same directory. After the build:

```
website/blog/
  hugo.toml              <-- source (committed, also deployed)
  content/posts/hello-world.md  <-- source (committed, also deployed)
  layouts/...            <-- source (committed, also deployed)
  index.html             <-- generated (deployed)
  posts/hello-world/index.html  <-- generated (deployed)
```

All of this gets uploaded to Cloudflare Pages. Hugo source files become publicly accessible at URLs like `https://tsuku.dev/blog/hugo.toml` and `https://tsuku.dev/blog/content/posts/hello-world.md`.

**Recommendation**: Either (a) put Hugo source in a directory that's *not* under `website/` (e.g., `blog/` at the repo root, outputting to `website/blog/`), or (b) keep the source in `website/blog-src/` and output to `website/blog/`, or (c) add the generated files to `website/blog/` during CI but exclude the source files from the upload (the Cloudflare Pages action's `directory` parameter takes the whole `website/` dir, so this requires careful coordination). Option (a) is cleanest: it matches the existing pattern where `scripts/generate-registry.py` lives outside `website/` but writes into it.

### F2: `.gitignore` strategy is underspecified (Advisory)

The design mentions "Add `.gitignore` entry for Hugo's generated files in `website/blog/`" in Phase 3, but doesn't define what's generated vs. committed. If Hugo source lives in `website/blog/` and output is also copied there, the gitignore must exclude the generated HTML files (index.html, posts/) while keeping the source files (hugo.toml, content/, layouts/). This is a fragile setup -- any new file Hugo generates needs a corresponding gitignore entry, and reviewers can't easily tell generated from authored files.

With option (a) from F1 (source outside `website/`), the gitignore becomes simple: `website/blog/` is entirely generated, so `website/blog/` goes in the root `.gitignore` just like `_site/` already does (implicitly, since `recipes.json` is gitignored in `website/.gitignore`).

### F3: CSS works correctly in production but not in local dev (Advisory)

The absolute path `/assets/style.css` in the Hugo template will resolve correctly after CI copies the output into `website/blog/`, because Cloudflare Pages serves `website/` as the root. In production, `/assets/style.css` maps to `website/assets/style.css`. This is correct.

However, during local development with `hugo server`, the server root is `website/blog/`, so `/assets/style.css` resolves within that directory. The stylesheet won't load. Fix options:
- Document that local preview requires running a server from `website/` rather than `website/blog/` (e.g., build Hugo, copy output, then `python3 -m http.server` from `website/`).
- Or symlink `website/blog/assets/` to `website/assets/` during local dev.
- Or add `website/assets/style.css` to Hugo's `static/` directory as a symlink (but Hugo follows symlinks only with `--ignoreVendorPaths`-style config, and this adds complexity).

Not blocking -- the CI path works -- but the design's "test locally" instruction will produce unstyled pages without a workaround.

### F4: CI failure mode is underspecified (Advisory)

The design states "The build step is isolated and won't affect deployment if it fails (the blog directory simply won't update)." This is not quite true given the current workflow structure. If the Hugo build step fails in the GitHub Actions job, the entire `deploy` job will fail (steps after the failing step don't run by default). The Cloudflare Pages deploy step won't execute, meaning the existing site also doesn't deploy. Either:
- Add `continue-on-error: true` to the Hugo build step (so a blog build failure doesn't block site deployment), or
- Acknowledge that blog build failures block all site deployments (which is actually fine -- you don't want to deploy a broken blog).

The design should pick one and be explicit.

### F5: Hugo version pinning without checksum (Advisory)

The design pins Hugo by version in the `.deb` download URL, which is good. It mentions optionally verifying checksums. For a CI pipeline that downloads a binary and runs it, checksum verification should be the default, not optional. The Hugo releases include `hugo_<version>_checksums.txt` signed with the maintainer's PGP key. At minimum, download the checksums file and verify the `.deb` hash with `sha256sum --check`. This matches how the project handles tool verification in its own recipes.

### F6: `baseURL` trailing slash semantics (Non-issue)

The `hugo.toml` sets `baseURL = "https://tsuku.dev/blog/"`. Hugo uses this for generating absolute URLs in `.Permalink` and in the OpenGraph template. The trailing slash is correct and standard. No issue here.

---

## 6. Cloudflare Pages URL Routing

The scoped Hugo approach does not create routing problems. Cloudflare Pages direct upload serves files by their filesystem path:

- `website/blog/index.html` serves at `/blog/` (and `/blog` redirects to `/blog/`)
- `website/blog/posts/hello-world/index.html` serves at `/blog/posts/hello-world/`

The existing `_redirects` file has no rules that would intercept `/blog/*` paths. The `/recipes/*` splat rule is limited to that prefix. No conflict.

One consideration: if blog posts are ever served at `/blog/<slug>/` rather than `/blog/posts/<slug>/`, Hugo's URL configuration (`[permalinks]`) controls this. The design uses the default permalink structure (section-based), which puts posts under `/blog/posts/`. This is fine but should be a conscious choice documented in the config.

---

## 7. CSS Integration via Absolute Path

Using `/assets/style.css` is the correct approach. It avoids duplicating the stylesheet and ensures blog pages stay in sync with the rest of the site's theme.

The `baseof.html` template in the design correctly uses `<link rel="stylesheet" href="/assets/style.css">` rather than a relative path. Since the final deployment has `website/assets/style.css` at the root, this resolves correctly in production.

The blog-specific CSS (post typography, article layout) should go in `website/assets/blog.css` or be appended to `website/assets/style.css`. The design mentions both options (inline `<style>` block or separate file) but doesn't decide. A separate `blog.css` file in `website/assets/` is preferable because it keeps the shared stylesheet unchanged and makes blog styles independently cacheable.

---

## Recommendations

1. **(Blocking)** Separate Hugo source from deployment output. Move Hugo source to `blog/` at the repo root (or `website/blog-src/`), outputting to `website/blog/` during CI. This eliminates the source/output collision and simplifies gitignore handling.

2. **(Advisory)** Be explicit about CI failure behavior. Either add `continue-on-error: true` to the Hugo step or document that blog build failures block all site deployments.

3. **(Advisory)** Document the local development limitation. Either provide a working local preview workflow or note that `hugo server` won't render the site's styles and describe the alternative.

4. **(Advisory)** Default to checksum verification for the Hugo `.deb` download rather than marking it optional.

5. **(Advisory)** Choose `website/assets/blog.css` over an inline `<style>` block for blog-specific styles. Add a second `<link>` in the Hugo base template.
