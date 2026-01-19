---
status: Planned
problem: Users cannot discover what tools tsuku can install without installing the CLI and running `tsuku recipes`, despite the marketing claim of "150+ tools".
decision: Build a static recipe browser page at `/recipes/` that fetches recipe metadata via client-side JavaScript and provides search/filter functionality.
rationale: This approach maintains tsuku.dev's static-file architecture without adding a build step, enables instant updates when the registry publishes, and provides sufficient functionality for discovery and search needs. A framework-based solution would be overkill for a filtered list, and build-time HTML generation would introduce complexity and delays.
---

# Design: Recipe Browser Page

**Status**: Planned

## Context and Problem Statement

Users have no way to discover what tools tsuku can install without using the CLI. The tsuku.dev website shows a landing page with "150+ tools" claim but doesn't list the actual recipes.

### Current User Journey

1. User visits tsuku.dev, sees "150+ tools" feature
2. User must install tsuku to run `tsuku recipes` to see the list
3. No way to browse, search, or filter before committing to installation

### Why This Matters

- **Discovery friction**: Users can't evaluate if tsuku supports their tools before installing
- **Trust building**: Showing the actual recipe list validates the "150+ tools" claim
- **Informed decisions**: Users can see tool descriptions and homepages before installing

### Scope

**In scope:**
- Static page at `/recipes/` listing all recipes from tsuku-registry
- Client-side search and filter functionality
- Recipe cards showing name, description, homepage link
- Integration with existing site theme

**Out of scope:**
- Individual recipe detail pages (Phase 2)
- Server-side search or API
- Installation statistics per recipe (covered by `/stats/`)
- Version information display

### Requirements

1. **Data source**: Consume `https://registry.tsuku.dev/recipes.json` generated from embedded recipes
2. **Search**: Filter recipes by name and description as user types
3. **Performance**: Search results update within 50ms of keystroke; initial load under 500ms on broadband
4. **Mobile**: Responsive layout at 320px, 480px, 768px breakpoints
5. **Accessibility**: Keyboard navigation, ARIA labels, works with screen readers
6. **Theme**: Match existing dark theme aesthetic
7. **Graceful degradation**: Show `<noscript>` message for users without JavaScript

### Assumptions

1. **CORS enabled**: `registry.tsuku.dev` serves `recipes.json` with `Access-Control-Allow-Origin: *` header
2. **Schema stability**: The JSON schema (name, description, homepage fields) is stable; breaking changes will increment `schema_version`
3. **Recipe scale**: Recipe count stays under 1,000 (acceptable for in-memory filtering without pagination)
4. **URL validation upstream**: All recipe homepage URLs are validated as HTTPS during generation (tsuku-registry CI)
5. **Directory routing**: Cloudflare Pages serves `recipes/index.html` for `/recipes/` requests

## Decision Drivers

- **Simplicity**: No build step; static HTML/CSS/JS only
- **Performance**: Fast initial load, instant search
- **Consistency**: Match existing tsuku.dev minimal aesthetic
- **Security**: Prevent XSS from recipe metadata
- **Maintainability**: Minimal JavaScript, easy to understand

## Considered Options

### Option 1: Build-Time HTML Generation

Fetch `recipes.json` during Cloudflare Pages build, generate static HTML with all recipes embedded.

**Pros:**
- Fastest initial render (HTML already contains recipes)
- No runtime fetch needed
- SEO-friendly (recipes in HTML source)

**Cons:**
- Requires build step (currently none exists for tsuku.dev)
- Adds build complexity (Node.js, build scripts)
- 5-15 minute delay between registry update and site update (Cloudflare Pages build time)
- Harder to add client-side search (data not in JS)

### Option 2: Client-Side Fetch with Vanilla JavaScript

Load page with skeleton, fetch `recipes.json` at runtime, render with vanilla JavaScript.

**Pros:**
- No build step needed
- Simple implementation
- Easy to add search (data already in JavaScript)
- Updates instantly when registry updates

**Cons:**
- Slightly slower initial render (fetch required, ~100-300ms)
- Not SEO-friendly for recipe names
- Brief loading state before recipes appear (mitigated with skeleton UI)
- Requires CORS headers on registry.tsuku.dev

### Option 3: Client-Side with Framework (React/Vue/Svelte)

Use a JavaScript framework for the recipe browser component.

**Pros:**
- Rich component model
- Virtual DOM for efficient updates
- Familiar patterns for developers

**Cons:**
- Adds significant bundle size (React ~40KB, Vue ~35KB minified)
- Requires build step
- Overkill for a single search/filter list
- Inconsistent with rest of site (vanilla JS)

**Note:** Frameworks would be appropriate if the site grew to include recipe detail pages with complex state, user favorites, or multi-dimensional filtering. For the current scope, this complexity is not justified.

## Decision Outcome

**Chosen option: Option 2 - Client-Side Fetch with Vanilla JavaScript**

### Rationale

This option was chosen because:
- **No build step**: Maintains tsuku.dev's static-file simplicity
- **Instant updates**: When registry publishes new JSON, the page shows it immediately
- **Simple implementation**: Vanilla JavaScript is sufficient for search/filter
- **Consistency**: Matches existing site architecture (index.html uses vanilla JS)

Alternatives were rejected because:
- **Build-time generation (Option 1)**: Adds complexity for marginal SEO benefit; recipe names are not high-value search terms
- **Framework (Option 3)**: Massive overkill for displaying a filtered list; adds 50KB+ bundle for no benefit

### Trade-offs Accepted

- **Loading state**: Users see a brief loading state before recipes appear (mitigated with skeleton UI)
- **No SEO for recipe names**: Search engines won't index individual recipe names (acceptable; users search for "tsuku" not "tsuku k9s")
- **Relies on external URL**: Page depends on `registry.tsuku.dev` being available (GitHub Pages has 99.9%+ uptime)

## Solution Architecture

### Overview

```
User loads /recipes/
        │
        ▼
┌─────────────────┐
│ recipes/        │
│ index.html      │
│ (skeleton UI)   │
└────────┬────────┘
         │ fetch()
         ▼
┌─────────────────────────────────┐
│ registry.tsuku.dev/recipes.json │
│ (GitHub Pages, CDN-cached)      │
└────────┬────────────────────────┘
         │
         ▼
┌─────────────────┐
│ JavaScript      │
│ renders cards,  │
│ enables search  │
└─────────────────┘
```

### Components

1. **HTML structure** (`recipes/index.html`)
   - Search input field
   - Recipe count display
   - Recipe grid container
   - Loading skeleton
   - Error state

2. **JavaScript** (inline in HTML)
   - Fetch `recipes.json` on page load
   - Render recipe cards using safe DOM APIs
   - Implement search with debounced input handler
   - Handle loading and error states

3. **CSS** (existing `assets/style.css` extended)
   - Recipe card styles
   - Search input styles
   - Grid layout
   - Loading skeleton animation

### Data Flow

1. Page loads with skeleton UI visible
2. JavaScript fetches `https://registry.tsuku.dev/recipes.json`
3. On success: Parse JSON, validate each recipe, store valid recipes in memory, render, hide skeleton
4. On error: Show error message with retry option (guard against concurrent fetches)
5. User types in search: Filter in-memory array, re-render matching recipes

### Input Validation

Before rendering, each recipe is validated:

```javascript
function isValidRecipe(recipe) {
  return recipe &&
         typeof recipe.name === 'string' && recipe.name.length > 0 &&
         typeof recipe.description === 'string' &&
         typeof recipe.homepage === 'string' &&
         isValidUrl(recipe.homepage);
}

function isValidUrl(url) {
  try {
    const u = new URL(url);
    return u.protocol === 'https:';
  } catch {
    return false;
  }
}
```

Invalid recipes are silently skipped (logged to console for debugging). This provides defense-in-depth even though upstream validation should catch issues.

### Key Interfaces

**Input JSON** (from tsuku-registry):

```json
{
  "schema_version": "1.0.0",
  "generated_at": "2025-11-29T12:00:00Z",
  "recipes": [
    {
      "name": "k9s",
      "description": "Kubernetes CLI and TUI",
      "homepage": "https://k9scli.io/"
    }
  ]
}
```

**Recipe Card HTML** (generated by JavaScript):

```html
<article class="recipe-card">
  <h3 class="recipe-name">k9s</h3>
  <p class="recipe-description">Kubernetes CLI and TUI</p>
  <a class="recipe-homepage" href="https://k9scli.io/"
     target="_blank" rel="noopener noreferrer">Homepage</a>
</article>
```

### URL Routing

- Page served at `/recipes/` via `recipes/index.html`
- No changes to `_redirects` needed (Cloudflare Pages serves `index.html` for directories)

## Implementation Approach

### Prerequisites

Before starting implementation, verify:

- [ ] `registry.tsuku.dev/recipes.json` is live and accessible
- [ ] CORS headers allow requests from `tsuku.dev` origin
- [ ] JSON schema matches expected format (name, description, homepage fields)

### Phase 1: Static Page Structure

Create `recipes/index.html` with:
- Header/footer matching main site
- Search input with ARIA label
- Empty recipe grid
- Placeholder loading skeleton
- `<noscript>` message with link to GitHub registry

Deliverable: Page renders with skeleton, no data yet.

### Phase 2: Data Fetching and Rendering

Add JavaScript to:
- Fetch `recipes.json` on DOMContentLoaded
- Validate each recipe (name, description, homepage fields; HTTPS URL)
- Render recipe cards using `textContent` (not innerHTML)
- Show recipe count ("Showing X recipes")

Deliverable: Page shows all recipes after loading.

### Phase 3: Search Functionality

Implement search:
- Debounced input handler (150ms delay)
- Case-insensitive substring match on name and description
- Update recipe count on filter ("Showing X of Y recipes")
- Show "no results" message when empty

Deliverable: Users can search and filter recipes.

### Phase 4: Error Handling

Handle failure cases:
- Network error: Show error message with retry button (guard against concurrent fetches)
- Invalid JSON: Show error message
- Empty recipes array: Show "no recipes available"
- CORS failure: Show error with link to GitHub registry as fallback

Deliverable: Graceful degradation on failures.

### Phase 5: CSS Styles

Add to `assets/style.css`:
- Recipe card styles (border, padding, hover state)
- Recipe grid layout (responsive)
- Search input styles
- Loading skeleton animation

Deliverable: Polished visual design matching site theme.

### Phase 6: Navigation

Update main site:
- Add "Recipes" link to header nav (all pages)
- Update "Recipes" link in footer (currently points to GitHub)

Deliverable: Users can navigate to recipe browser from any page.

## Security Considerations

### Download Verification

**Not applicable** - This page does not download or execute binaries. It only displays metadata (names, descriptions, URLs) fetched from a trusted source (GitHub Pages).

### Execution Isolation

**Not applicable** - No code execution occurs beyond rendering recipe metadata. The JavaScript runs in the browser sandbox with standard web security restrictions.

### Supply Chain Risks

**Data source**: Recipe metadata comes from `registry.tsuku.dev/recipes.json`, generated by tsuku-registry GitHub Actions and hosted on GitHub Pages.

**Trust model:**
1. tsuku-registry repository has branch protection requiring PR review
2. GitHub Actions workflow is pinned to commit SHAs
3. GitHub Pages serves content over HTTPS with valid certificates

**Risks and mitigations:**

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Malicious recipe name/description | Low | Medium (XSS) | Use `textContent` not `innerHTML`; validate on generation side |
| Malicious homepage URL | Low | Medium (phishing) | Validate HTTPS-only on generation; use `rel="noopener noreferrer"` |
| registry.tsuku.dev unavailable | Very low | Low (page degrades) | Show error message; GitHub Pages SLA |
| MITM attack on JSON fetch | Very low | Medium | HTTPS-only; HSTS enabled |

### User Data Exposure

**Data accessed**: None. The page does not access cookies, localStorage, or any user data.

**Data transmitted**: None beyond the standard HTTP request to fetch `recipes.json`.

**Privacy implications**: None. No tracking, no analytics on this page (telemetry is a separate feature).

### Additional Security Measures

1. **Content Security Policy**: Add CSP header in `_headers` to restrict script sources
2. **Safe DOM APIs**: Always use `textContent`, `setAttribute`, `createElement` - never `innerHTML` with user data
3. **URL validation (defense-in-depth)**: Validate homepage URLs are valid HTTPS before creating links (see Input Validation section); skip recipes with invalid URLs
4. **Link attributes**: All external links use `target="_blank" rel="noopener noreferrer"`
5. **Fetch guard**: Prevent concurrent fetches when user clicks retry rapidly

## Consequences

### Positive

- **Discovery**: Users can browse all recipes before installing
- **Trust**: Visible recipe count validates marketing claims
- **Simplicity**: No build step added; static files only
- **Freshness**: Updates appear immediately when registry publishes

### Negative

- **Loading delay**: Brief fetch delay before recipes appear (~100-300ms)
- **No offline support**: Page requires network to show recipes
- **SEO limitation**: Recipe names not indexed by search engines
- **External dependency**: Page depends on registry.tsuku.dev availability

### Mitigations

- Loading skeleton provides immediate visual feedback
- GitHub Pages has excellent uptime; error state handles failures gracefully
- SEO for individual tools is not a priority (users search for "tsuku" not specific tools)

## Implementation Issues

The recipe browser feature has been implemented as part of the monorepo consolidation. The website now lives at `website/` in the tsuku repository.
