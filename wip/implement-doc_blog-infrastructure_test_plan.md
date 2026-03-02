# Test Plan: Blog Infrastructure for tsuku.dev

Generated from: docs/designs/DESIGN-blog-infrastructure.md
Issues covered: 5
Total scenarios: 14

## Progress

- [x] scenario-1: Hugo configuration file exists with correct settings
- [x] scenario-2: Hugo templates define correct page structure
- [x] scenario-3: Blog-specific CSS uses existing CSS variables
- [x] scenario-4: Generated blog output is gitignored
- [x] scenario-5: OG default image placeholder exists
- [x] scenario-6: Hello world post has correct frontmatter and content
- [x] scenario-7: Hugo builds the blog successfully from source (skipped: no Hugo, deferred to CI)
- [x] scenario-8: Generated post HTML contains correct stylesheet links and OG tags (skipped: no Hugo, deferred to CI)
- [x] scenario-9: Blog index lists the hello world post (skipped: no Hugo, deferred to CI)
- [x] scenario-10: CI workflow includes blog path triggers
- [x] scenario-11: CI workflow installs Hugo with checksum verification
- [x] scenario-12: CI workflow builds blog before recipe generation
- [ ] scenario-13: User-facing pages include blog navigation link
- [x] scenario-14: End-to-end blog rendering with dark theme (skipped: requires browser, manual verification)

---

## Scenario 1: Hugo configuration file exists with correct settings
**ID**: scenario-1
**Testable after**: #1974
**Commands**:
- `test -f blog/hugo.toml`
- `grep 'baseURL = "https://tsuku.dev/blog/"' blog/hugo.toml`
- `grep 'title = "tsuku blog"' blog/hugo.toml`
- `grep 'og-default.png' blog/hugo.toml`
**Expected**: `blog/hugo.toml` exists and contains the correct baseURL, title, and default OG image reference
**Status**: passed

---

## Scenario 2: Hugo templates define correct page structure
**ID**: scenario-2
**Testable after**: #1974
**Commands**:
- `test -f blog/layouts/_default/baseof.html`
- `test -f blog/layouts/_default/single.html`
- `test -f blog/layouts/posts/list.html`
- `grep '/assets/style.css' blog/layouts/_default/baseof.html`
- `grep '/assets/blog.css' blog/layouts/_default/baseof.html`
- `grep '_internal/opengraph.html' blog/layouts/_default/baseof.html`
- `grep 'block "main"' blog/layouts/_default/baseof.html`
- `grep 'blog-post' blog/layouts/_default/single.html`
- `grep 'blog-index' blog/layouts/posts/list.html`
**Expected**: All three template files exist. baseof.html links both stylesheets and includes the OpenGraph template. single.html uses the blog-post class. list.html uses the blog-index class.
**Status**: passed

---

## Scenario 3: Blog-specific CSS uses existing CSS variables
**ID**: scenario-3
**Testable after**: #1974
**Commands**:
- `test -f website/assets/blog.css`
- `grep 'var(--' website/assets/blog.css`
- `grep '.blog-post' website/assets/blog.css`
- `grep '.blog-index' website/assets/blog.css`
- `grep 'max-width: 800px' website/assets/blog.css`
**Expected**: `website/assets/blog.css` exists, references CSS variables (not hardcoded colors), defines both `.blog-post` and `.blog-index` classes, and constrains layout to 800px max-width
**Status**: passed

---

## Scenario 4: Generated blog output is gitignored
**ID**: scenario-4
**Testable after**: #1974
**Commands**:
- `grep 'blog/' website/.gitignore`
- `grep '.hugo_build.lock' blog/.gitignore`
**Expected**: `website/.gitignore` includes a `blog/` entry. `blog/.gitignore` ignores Hugo's local build artifacts.
**Status**: passed

---

## Scenario 5: OG default image placeholder exists
**ID**: scenario-5
**Testable after**: #1974
**Commands**:
- `test -f website/assets/og-default.png`
- `file website/assets/og-default.png`
**Expected**: `website/assets/og-default.png` exists and is a valid PNG image file
**Status**: passed

---

## Scenario 6: Hello world post has correct frontmatter and content
**ID**: scenario-6
**Testable after**: #1975
**Commands**:
- `test -f blog/content/posts/_index.md`
- `test -f blog/content/posts/hello-world.md`
- `head -10 blog/content/posts/hello-world.md | grep 'title: "Hello, World"'`
- `head -10 blog/content/posts/hello-world.md | grep 'date: 2026-03-01'`
- `head -10 blog/content/posts/hello-world.md | grep 'description:'`
- `grep '^## ' blog/content/posts/hello-world.md`
- `grep -E '\x60tsuku' blog/content/posts/hello-world.md`
**Expected**: Section index and hello-world post both exist. Post frontmatter has the required title, date, and description. Post body contains at least one h2 heading, inline code, and paragraph text.
**Status**: passed

---

## Scenario 7: Hugo builds the blog successfully from source
**ID**: scenario-7
**Testable after**: #1974, #1975
**Environment**: requires Hugo installed locally
**Commands**:
- `hugo --source blog --destination /tmp/blog-test-scenario7`
- `test -f /tmp/blog-test-scenario7/index.html`
- `test -f /tmp/blog-test-scenario7/posts/hello-world/index.html`
- `rm -rf /tmp/blog-test-scenario7`
**Expected**: Hugo exits 0 and produces `index.html` (blog index) and `posts/hello-world/index.html` (individual post) in the output directory
**Status**: skipped (Hugo not installed locally; deferred to CI after #1976)

---

## Scenario 8: Generated post HTML contains correct stylesheet links and OG tags
**ID**: scenario-8
**Testable after**: #1974, #1975
**Environment**: requires Hugo installed locally
**Commands**:
- `hugo --source blog --destination /tmp/blog-test-scenario8`
- `grep '/assets/style.css' /tmp/blog-test-scenario8/posts/hello-world/index.html`
- `grep '/assets/blog.css' /tmp/blog-test-scenario8/posts/hello-world/index.html`
- `grep 'og:title' /tmp/blog-test-scenario8/posts/hello-world/index.html`
- `grep 'og:description' /tmp/blog-test-scenario8/posts/hello-world/index.html`
- `grep 'Hello, World' /tmp/blog-test-scenario8/posts/hello-world/index.html`
- `grep '<pre' /tmp/blog-test-scenario8/posts/hello-world/index.html`
- `rm -rf /tmp/blog-test-scenario8`
**Expected**: The generated post HTML links both stylesheets, includes OpenGraph meta tags for title and description, renders the post title and content, and contains a pre tag for the code block
**Status**: skipped (Hugo not installed locally; deferred to CI after #1976)

---

## Scenario 9: Blog index lists the hello world post
**ID**: scenario-9
**Testable after**: #1974, #1975
**Environment**: requires Hugo installed locally
**Commands**:
- `hugo --source blog --destination /tmp/blog-test-scenario9`
- `grep 'Hello, World' /tmp/blog-test-scenario9/index.html`
- `grep 'blog-index' /tmp/blog-test-scenario9/index.html`
- `grep 'hello-world' /tmp/blog-test-scenario9/index.html`
- `rm -rf /tmp/blog-test-scenario9`
**Expected**: The blog index page contains the hello world post title, uses the blog-index class, and includes a link to the hello-world post
**Status**: skipped (Hugo not installed locally; deferred to CI after #1976)

---

## Scenario 10: CI workflow includes blog path triggers
**ID**: scenario-10
**Testable after**: #1976
**Commands**:
- `grep "blog/\*\*" .github/workflows/deploy-website.yml`
**Expected**: `deploy-website.yml` triggers on changes to `blog/**` for both push and pull_request events
**Status**: passed

---

## Scenario 11: CI workflow installs Hugo with checksum verification
**ID**: scenario-11
**Testable after**: #1976
**Commands**:
- `grep 'HUGO_VERSION' .github/workflows/deploy-website.yml`
- `grep '0.147.0' .github/workflows/deploy-website.yml`
- `grep 'sha256sum' .github/workflows/deploy-website.yml`
- `grep 'dpkg' .github/workflows/deploy-website.yml`
**Expected**: The workflow defines HUGO_VERSION as an environment variable set to 0.147.0, verifies the download with sha256sum, and installs the .deb package
**Status**: passed

---

## Scenario 12: CI workflow builds blog before recipe generation
**ID**: scenario-12
**Testable after**: #1976
**Commands**:
- `grep -n 'Build blog\|hugo --source\|Generate recipes' .github/workflows/deploy-website.yml`
**Expected**: The Hugo build step appears before the recipes.json generation step in the workflow file (lower line number)
**Status**: passed

---

## Scenario 13: User-facing pages include blog navigation link
**ID**: scenario-13
**Testable after**: #1977
**Commands**:
- `grep '/blog/' website/index.html`
- `grep '/blog/' website/recipes/index.html`
- `grep '/blog/' website/telemetry/index.html`
- `grep '/blog/' website/404.html`
**Expected**: All four user-facing pages (index, recipes, telemetry, 404) contain a link to `/blog/` in both the nav header and the footer
**Status**: pending

---

## Scenario 14: End-to-end blog rendering with dark theme
**ID**: scenario-14
**Testable after**: #1974, #1975, #1976
**Environment**: manual -- requires Hugo installed locally and a browser
**Commands**:
- `hugo --source blog --destination $PWD/website/blog`
- `python3 -m http.server 8000 --directory website &`
- Open `http://localhost:8000/blog/` in a browser
- Open `http://localhost:8000/blog/posts/hello-world/` in a browser
- Kill the HTTP server
- `rm -rf website/blog`
**Expected**: The blog index page at `/blog/` shows the hello world post in a list with the dark theme (dark background `#0d1117`, light text `#c9d1d9`). The hello world post at `/blog/posts/hello-world/` renders with the dark theme, code blocks use the code background color (`#1c2128`), inline code uses the monospace font, headings are properly spaced, and the page layout matches the rest of the site (same nav, same footer). The OpenGraph meta tags are present in the page source. Social sharing preview can be verified with an OG debugger tool after production deployment.
**Status**: skipped (requires browser for manual visual verification after deployment)
