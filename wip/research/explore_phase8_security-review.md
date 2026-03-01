# Security Review: Blog Infrastructure Design

**Document reviewed:** `docs/designs/DESIGN-blog-infrastructure.md`, Security Considerations section (lines 437-462)

**Reviewer role:** Pragmatic security review -- focus on real attack vectors, not theoretical ones.

---

## 1. Checksum Verification for Hugo Binary

### What the design says

The CI step downloads a `.deb` from GitHub releases and verifies its SHA-256 checksum against `hugo_<version>_checksums.txt` from the same release.

### The problem

**The checksums file is fetched from the same source as the binary.** If an attacker compromises the Hugo GitHub release (the threat the design explicitly identifies), they control both the `.deb` and the checksums file. Verifying the binary against an attacker-controlled checksum proves nothing.

Hugo's release checksums are *not* GPG-signed as of v0.147.0. There is no detached signature (`.sig` or `.asc`) published alongside the checksums file. This means there is no way to independently verify the checksums came from the Hugo maintainers.

### Residual risk assessment

This is a real but bounded risk. The attack requires compromising Hugo's GitHub release infrastructure (high difficulty), and the binary runs only in CI with `contents: read` and `deployments: write` permissions -- it can't push code or exfiltrate secrets beyond the deployment token. The output is static HTML that can be visually inspected in PR preview deploys.

### Recommendation

Acknowledge the limitation explicitly in the design doc. The current approach is standard practice (most CI setups do exactly this), but the document should not imply the checksum verification protects against a compromised release. It protects against download corruption and CDN cache poisoning, which is still useful.

Consider hardcoding the expected SHA-256 hash directly in the workflow file rather than downloading it. This turns the version bump into a manual two-value update (version + hash), but it means the trusted checksum lives in your repo, not at the download source.

**Severity: Advisory.** Standard practice, bounded blast radius, but the doc overstates what the mitigation achieves.

---

## 2. XSS via Frontmatter in Templates

### What the design says

"User-supplied content in frontmatter fields (title, description) is escaped in template output" (line 456). The design relies on Go's `html/template` auto-escaping.

### Analysis

Go's `html/template` does auto-escape values interpolated into HTML element content (e.g., `{{ .Title }}` inside `<h1>` tags). This is correct and sufficient for the templates shown in the design.

However, auto-escaping is context-dependent. It protects against:
- HTML injection in element content: `{{ .Title }}` in `<h1>{{ .Title }}</h1>` -- escaped correctly
- Attribute injection in quoted attributes: `content="{{ .Description }}"` -- escaped correctly

It does NOT protect against:
- Values placed in unquoted attributes (not present in the templates shown)
- Values used inside `<script>` blocks (not present)
- Values passed through `safeHTML` or `| safeHTML` pipe (not present)

**The templates in the design are safe.** The `single.html` and `list.html` templates only use frontmatter values in element content and standard HTML attributes. Hugo's internal OpenGraph template also uses proper escaping.

The real risk would be a future contributor adding `{{ .Params.customField | safeHTML }}` to a template. This is a maintenance awareness issue, not a design flaw.

### Recommendation

No change needed. The templates as designed are safe. If the design wants to be explicit, it could note: "Templates must not use `safeHTML` on frontmatter values."

**Severity: Not a finding.** The design is correct on this point.

---

## 3. Hugo's `html/template` Auto-Escaping

### What the design says

"Hugo templates use Go's `html/template` package, which auto-escapes HTML by default." (line 456)

### Analysis

This is accurate for Hugo's layout templates. Hugo uses `html/template` (not `text/template`) for layouts, which provides contextual auto-escaping.

One nuance: the **markdown body content** (`.Content`) is rendered by Hugo's markdown processor (Goldmark by default) and then inserted as trusted HTML via Hugo's internal rendering pipeline. This is by design -- you need the rendered HTML to display formatted posts. Goldmark does sanitize raw HTML in markdown by default (the `markup.goldmark.renderer.unsafe` config defaults to `false`), meaning inline `<script>` tags in markdown source are stripped.

The design doesn't mention this, but it's safe by default. The risk would be someone setting `unsafe = true` in `hugo.toml` to allow raw HTML in markdown, which would then allow script injection through committed markdown files. Since only repo committers can modify content, and commits go through PR review, this is acceptable.

### Recommendation

No change needed. Default Goldmark settings are secure.

**Severity: Not a finding.**

---

## 4. Same-Origin Risks (Blog under tsuku.dev/blog/)

### What the design says

Nothing. The design does not discuss same-origin implications.

### Analysis

The blog is served from `tsuku.dev/blog/`, which shares the same origin as the main site (`tsuku.dev`). This means:

- Blog pages can read/write cookies set on `tsuku.dev`
- Blog pages can access `localStorage`/`sessionStorage` for the `tsuku.dev` origin
- JavaScript on blog pages can make fetch requests to any `tsuku.dev` path (including `/install.sh`)

**Actual risk: low.** Looking at the current site:
- No JavaScript files are served (no `.js` files exist in `website/`)
- No cookies are set by the site
- No localStorage usage
- The site is entirely static HTML/CSS with no authentication or user sessions
- The telemetry worker runs on a separate path and doesn't set cookies

The same-origin concern would matter if:
1. The blog introduced JavaScript (e.g., analytics, comments), AND
2. The main site had sensitive client-side state

Neither condition holds today. Hugo's generated output is static HTML with no JavaScript (unless explicitly added to templates or markdown). The templates in the design contain no `<script>` tags.

The one concrete same-origin consideration: if the blog later adds third-party JavaScript (e.g., a comment widget), that script could interact with any future client-side state on the main site. This is a future risk, not a current one.

### Recommendation

Worth a brief mention in the design doc: "The blog shares the tsuku.dev origin. Blog templates must not include third-party JavaScript without considering same-origin implications for the main site." One sentence, not a section.

**Severity: Advisory.** No current risk, but the design should note the constraint for future reference.

---

## 5. "Not Applicable" Justifications

### "Download Verification -- Not applicable for blog content"

The opening line of the Download Verification section (line 441) says "Not applicable for blog content." This is misleading because the section then immediately discusses Hugo binary verification, which IS applicable. The "not applicable" framing seems to be saying blog content itself doesn't need download verification (true -- it's committed to the repo), but the section header implies download verification as a whole was dismissed.

**Recommendation:** Remove "Not applicable for blog content" and start directly with the Hugo installation verification discussion.

**Severity: Advisory.** Confusing framing, but the actual content is substantive.

---

## 6. Attack Vectors Not Discussed

### 6a. CI Workflow Trigger Paths

The existing `deploy-website.yml` triggers on `website/**` and `recipes/**/*.toml` paths. The design says Hugo build will be added to this workflow, but `blog/**` is not in the current trigger paths. If the implementation doesn't add `blog/**` to the path filter, blog content changes won't trigger deployments.

This is a correctness issue, not strictly security, but it matters for the deployment pipeline.

**Severity: Out of scope (correctness, not security).**

### 6b. Hugo Build Output Overwriting Existing Files

Hugo outputs to `website/blog/`. If a misconfigured `hugo.toml` (e.g., `publishDir` set to `../website/` instead of using `--destination`) or a template with an unexpected path structure generates files outside `website/blog/`, it could overwrite existing site pages (like `index.html`).

The design mitigates this by using the `--destination website/blog` flag rather than configuring the output directory in `hugo.toml`. This is correct. The residual risk is someone changing `hugo.toml` later and breaking the invariant.

**Recommendation:** No action needed. The `--destination` flag approach is sound.

**Severity: Not a finding.**

### 6c. Dependency Confusion / Typosquatting (Hugo Modules)

The design explicitly notes "No third-party Hugo themes or modules are used" (line 458). Good. Hugo modules pull from Go module proxies and could be a vector. Since no modules are configured, this is a non-issue.

**Severity: Not a finding.**

---

## Summary Table

| Finding | Severity | Action Needed |
|---------|----------|---------------|
| Checksum verification doesn't protect against compromised release | Advisory | Clarify what the checksum actually protects against; consider hardcoding hash |
| XSS via frontmatter | Not a finding | Templates are safe as designed |
| `html/template` auto-escaping | Not a finding | Accurate claim, safe defaults |
| Same-origin risk (blog under tsuku.dev) | Advisory | Add one sentence noting the constraint for future JS additions |
| "Not applicable" framing on download verification | Advisory | Remove misleading opener |
| Hugo output overwriting existing pages | Not a finding | `--destination` flag is the right approach |

## Overall Assessment

The security considerations section is adequate for the threat model. The blog is static HTML generated from committed markdown, built in a CI environment with limited permissions, and served from a CDN. The attack surface is genuinely small.

The main gap is the checksum verification claim: the design presents it as a mitigation against a compromised Hugo release, but it doesn't actually protect against that scenario. This deserves a correction in framing, not a change in approach (the approach is standard practice and sufficient given the bounded blast radius).

No blocking security findings. The three advisory items are documentation clarifications, not architectural changes.
