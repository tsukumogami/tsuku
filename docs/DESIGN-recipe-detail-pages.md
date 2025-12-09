# Design: Recipe Detail Pages with Dependency Visualization

**Status**: Planned

## Context and Problem Statement

The tsuku.dev website has a `/recipes/` page that shows a searchable grid of all available recipes with basic information (name, description, homepage link). However, users cannot view detailed information about individual recipes before installing them.

This creates several friction points:

1. **Hidden dependencies**: Users cannot see what dependencies a tool requires before installation. A user wanting to install Jekyll doesn't know it requires Ruby and Zig until after running `tsuku install jekyll`.

2. **No dedicated URLs**: There's no way to link to a specific tool's information. A blog post recommending k9s cannot link directly to tsuku's k9s page.

3. **Limited discoverability**: Users browsing the recipe list see only a brief description. They must visit external homepages to learn more about tools.

### Current State

Users can discover dependency information through the CLI:
- `tsuku info <tool>` shows recipe metadata but not dependencies
- `tsuku install <tool>` shows dependencies being installed as they happen
- No pre-installation preview of what will be installed

The website shows a searchable grid of tools with name, description, and homepage link only. **The grid already requires JavaScript** - it fetches `recipes.json` and renders cards client-side, with a `<noscript>` fallback linking to GitHub.

### Scope

**In scope:**
- Individual detail pages for each recipe at `/recipes/<tool>/`
- Display of install dependencies (runtime and build-time)
- Visual representation of dependency relationships
- Navigation between recipe grid and detail pages

**Out of scope:**
- Version history or changelog
- Installation statistics per recipe (covered by `/stats/`)
- User reviews or ratings
- "Related tools" recommendations
- Recipe editing or submission through the website

### Assumptions

1. **Dependency data exists**: Recipe TOML files already include `dependencies` and `runtime_dependencies` fields where applicable.
2. **No circular dependencies**: The dependency graph is acyclic. (Verified: tsuku's dependency resolution validates this at build time.)
3. **Sparse dependencies**: Most recipes (pre-built binaries) have zero dependencies. This feature primarily benefits language-ecosystem tools (Ruby gems, Python packages, Rust crates).
4. **JavaScript is acceptable**: The existing recipe browser already requires JavaScript; detail views can follow the same pattern.

## Decision Drivers

- **Architectural consistency**: Solution should match existing patterns (client-side rendering from JSON)
- **Minimal complexity**: Avoid adding build steps or generated files
- **Dependency visibility**: Users should understand prerequisites before installing
- **URL stability**: Detail page URLs should be predictable and shareable
- **Schema evolution**: Solution must handle recipes with or without dependency data

## Implementation Context

### Existing Patterns

**Recipe browser page** (`website/recipes/index.html`):
- Client-side fetches `https://registry.tsuku.dev/recipes.json`
- Vanilla JavaScript renders recipe cards using safe DOM APIs
- Debounced search filters in-memory array
- `<noscript>` fallback links to GitHub
- No build step - static HTML with inline script

**Current JSON schema** (v1.0.0):
```json
{
  "schema_version": "1.0.0",
  "recipes": [{
    "name": "k9s",
    "description": "Kubernetes CLI and TUI",
    "homepage": "https://k9scli.io/"
  }]
}
```

**Recipe TOML dependency fields**:
- `dependencies`: Tools required to build/install (e.g., `["ruby", "zig"]`)
- `runtime_dependencies`: Tools required to use after installation (e.g., `["golang"]`)
- Most recipes have neither field (pre-built binaries with no dependencies)

**JSON generation** (`scripts/generate-registry.py`):
- Reads TOML files, extracts metadata, writes JSON
- Currently only extracts name, description, homepage
- Would need modification to include dependency data

### Conventions to Follow

- Use `textContent` not `innerHTML` for user data
- All external links use `target="_blank" rel="noopener noreferrer"`
- Validate URLs are HTTPS before rendering
- Match existing dark theme CSS variables

## Considered Options

This design involves two independent decisions:

### Decision 1: Page Generation Strategy

How should individual recipe detail pages be created and served?

#### Option 1A: Static HTML Pages (Build-Time)

Generate individual `recipes/<tool>/index.html` files during the registry build step.

**Pros:**
- Pages work without JavaScript
- SEO-friendly: search engines can index individual tool pages
- Fast initial render - no fetch required
- No redirect/routing complexity

**Cons:**
- Requires build step changes (Python script modifications)
- Does not scale: tsuku aims to cover thousands of recipes
- Cloudflare Pages has 20,000 file limit - could become a constraint
- Inconsistent with existing architecture (grid is client-rendered)
- Updating page template requires full rebuild

#### Option 1B: Single-Page with History API Routing

One HTML page at `/recipes/index.html` handles both grid and detail views using the History API for clean URLs.

**Pros:**
- No build step changes - pure JavaScript solution
- Recipe data already fetched; navigation between grid and detail is instant
- Consistent with existing client-side rendering pattern
- Scales to thousands of recipes without deployment concerns
- One HTML file handles all recipe detail pages

**Cons:**
- Requires JavaScript (but grid already requires it)
- Requires catch-all redirect: `/recipes/*` → `/recipes/index.html`
- Direct links show brief loading state before content
- Cloudflare Pages splat redirect bug requires workaround (see Known Limitations)

### Decision 2: Dependency Data Location

Where should dependency information be stored and how should it be accessed?

#### Option 2A: Extend recipes.json Schema

Add `dependencies` and `runtime_dependencies` arrays to each recipe object in recipes.json.

**Pros:**
- Single fetch gets all data needed for detail pages
- Consistent with existing data flow
- Simple schema change (additive, backwards compatible)

**Cons:**
- Larger JSON payload (~10% increase estimated)
- Requires `generate-registry.py` changes

#### Option 2B: Separate Dependencies JSON

Create a new `dependencies.json` file with dependency data only.

**Pros:**
- Grid page continues to use lean recipes.json
- Dependency data fetched only when needed

**Cons:**
- Two fetches for detail pages
- More complex data joining in JavaScript
- Two files to keep in sync

### Option Evaluation Matrix

| Decision | Driver: Consistency | Driver: Minimal Complexity | Driver: URL Stability |
|----------|---------------------|---------------------------|----------------------|
| 1A: Static HTML | Poor | Poor | Good |
| 1B: SPA + History API | Good | Good | Good |
| 2A: Extend JSON | Good | Good | N/A |
| 2B: Separate JSON | Fair | Poor | N/A |

## Decision Outcome

**Chosen: 1B + 2A**

### Summary

Use client-side routing with the History API to render detail views from the same `recipes.json` data, extended with dependency information. No static HTML generation.

### Rationale

This combination prioritizes **architectural consistency** and **minimal complexity** - the recipe browser already uses client-side rendering, so detail pages should follow the same pattern.

**Decision 1: SPA with History API (1B)** was chosen because:
- Consistent with existing architecture - the grid is already client-rendered from JSON
- No build step changes - keeps the static site simple
- Instant navigation - data is already loaded, switching views is immediate
- The "requires JavaScript" concern is moot since the grid already requires it

**Decision 2: Extend recipes.json (2A)** was chosen because:
- Single data source - one fetch powers both grid and detail views
- Minimal payload increase (~5KB for dependency arrays)
- Backwards compatible - clients ignoring new fields still work

### Alternatives Rejected

- **Option 1A (Static HTML)**: Adds build complexity and 267+ generated files for marginal benefit (SEO for individual recipes is not valuable). Creates architectural inconsistency - grid is client-rendered but details would be static.
- **Option 2B (Separate JSON)**: Extra complexity for marginal payload savings; requires coordination between two files.

### Trade-offs Accepted

1. **No SEO for individual recipes**: Search engines won't index `/recipes/k9s/`. This is acceptable because users search for "tsuku" not "tsuku k9s recipe".

2. **Brief loading state on direct links**: Users navigating directly to `/recipes/k9s/` see a loading spinner while JSON fetches. This matches the existing grid behavior and is acceptable for a developer tools site.

3. **Requires JavaScript**: Detail pages won't work without JS. This is acceptable because the grid already requires JS, and the `<noscript>` fallback (link to GitHub) covers users without JS.

## Solution Architecture

### Overview

The solution extends the existing recipe browser page to handle both grid and detail views via client-side routing. The same `recipes.json` powers both views.

```
┌─────────────────────────────────────────────────────┐
│  recipes/index.html                                 │
│  (single page, handles grid + detail views)         │
└─────────────────────────────────────────────────────┘
                        │
                        │ fetch()
                        ▼
┌─────────────────────────────────────────────────────┐
│  registry.tsuku.dev/recipes.json (extended)         │
│  { recipes: [{ name, description, homepage,         │
│               dependencies, runtime_dependencies }] │
└─────────────────────────────────────────────────────┘
                        │
          ┌─────────────┴─────────────┐
          ▼                           ▼
┌─────────────────────┐     ┌─────────────────────┐
│  Grid View          │     │  Detail View        │
│  /recipes/          │     │  /recipes/k9s/      │
│  (search + cards)   │     │  (deps + install)   │
└─────────────────────┘     └─────────────────────┘
```

### Components

#### 1. Extended JSON Schema (v1.1.0)

```json
{
  "schema_version": "1.1.0",
  "generated_at": "2025-12-07T12:00:00Z",
  "recipes": [{
    "name": "jekyll",
    "description": "Static site generator for personal, project, or organization sites",
    "homepage": "https://jekyllrb.com/",
    "dependencies": ["ruby", "zig"],
    "runtime_dependencies": []
  }, {
    "name": "k9s",
    "description": "Kubernetes CLI and TUI",
    "homepage": "https://k9scli.io/",
    "dependencies": [],
    "runtime_dependencies": []
  }]
}
```

**Schema changes:**
- `schema_version` bumped to "1.1.0" (minor version, backwards compatible)
- New optional fields: `dependencies` (array of strings), `runtime_dependencies` (array of strings)
- Both default to empty array if not present in TOML

#### 2. Client-Side Router

JavaScript in `recipes/index.html` handles URL routing:

```javascript
// Determine view from URL
function getViewFromURL() {
    const path = window.location.pathname;
    const match = path.match(/^\/recipes\/([a-z0-9-]+)\/?$/);
    if (match) {
        return { view: 'detail', recipe: match[1] };
    }
    return { view: 'grid' };
}

// Navigate between views
function navigateTo(path) {
    history.pushState(null, '', path);
    renderCurrentView();
}

// Handle browser back/forward
window.addEventListener('popstate', renderCurrentView);
```

#### 3. Detail View Renderer

```javascript
function renderDetailView(recipeName) {
    const recipe = allRecipes.find(r => r.name === recipeName);
    if (!recipe) {
        render404();
        return;
    }

    const detail = document.createElement('section');
    detail.className = 'recipe-detail';

    // Recipe name
    const h1 = document.createElement('h1');
    h1.textContent = recipe.name;
    detail.appendChild(h1);

    // Description
    const desc = document.createElement('p');
    desc.className = 'description';
    desc.textContent = recipe.description;
    detail.appendChild(desc);

    // Homepage link
    const homepage = document.createElement('p');
    const link = document.createElement('a');
    link.href = recipe.homepage;
    link.textContent = 'Official Homepage';
    link.target = '_blank';
    link.rel = 'noopener noreferrer';
    homepage.appendChild(link);
    detail.appendChild(homepage);

    // Install command
    renderInstallCommand(detail, recipe.name);

    // Dependencies (if any)
    if (recipe.dependencies?.length || recipe.runtime_dependencies?.length) {
        renderDependencies(detail, recipe);
    }

    // Back link
    const back = document.createElement('a');
    back.href = '/recipes/';
    back.className = 'link-btn';
    back.textContent = 'Back to Recipes';
    back.addEventListener('click', (e) => {
        e.preventDefault();
        navigateTo('/recipes/');
    });
    detail.appendChild(back);

    container.appendChild(detail);
}
```

#### 4. Cloudflare Pages Redirect

Add to `_redirects`:
```
/recipes/*  /recipes/index.html  200
```

This ensures direct navigation to `/recipes/k9s/` serves the SPA, which then renders the correct view.

### Data Flow

1. **Page load** (any `/recipes/*` URL):
   - HTML page loads with skeleton UI
   - JavaScript fetches `recipes.json`
   - Router determines view from URL
   - Appropriate view renders (grid or detail)

2. **Grid to detail navigation**:
   - User clicks recipe card
   - `navigateTo('/recipes/<name>/')` called
   - History API updates URL without page reload
   - Detail view renders instantly (data already loaded)

3. **Detail to grid navigation**:
   - User clicks "Back to Recipes"
   - `navigateTo('/recipes/')` called
   - Grid view renders instantly

4. **Direct link to detail**:
   - User navigates to `/recipes/k9s/`
   - Cloudflare redirect serves `index.html`
   - JSON fetches, router sees detail view
   - Detail renders after brief loading state

### Key Interfaces

**Recipe object (extended):**
```typescript
interface Recipe {
    name: string;
    description: string;
    homepage: string;
    dependencies?: string[];
    runtime_dependencies?: string[];
}
```

**Router state:**
```typescript
type View =
    | { view: 'grid' }
    | { view: 'detail', recipe: string }
    | { view: '404' };
```

## Implementation Approach

**Milestone:** [Recipe Detail Pages](https://github.com/tsukumogami/tsuku/milestone/14)
**Parent Issue:** [#263](https://github.com/tsukumogami/tsuku/issues/263)

| Issue | Title | Dependencies |
|-------|-------|--------------|
| [#339](https://github.com/tsukumogami/tsuku/issues/339) | feat(registry): add dependency fields to recipes.json schema | None |
| [#340](https://github.com/tsukumogami/tsuku/issues/340) | feat(website): add client-side router for recipe pages | None |
| [#341](https://github.com/tsukumogami/tsuku/issues/341) | feat(website): implement recipe detail view renderer | #339, #340 |
| [#342](https://github.com/tsukumogami/tsuku/issues/342) | feat(website): update grid cards to navigate to detail pages | #340 |
| [#343](https://github.com/tsukumogami/tsuku/issues/343) | feat(website): add Cloudflare Pages redirect for recipe URLs | None |
| [#344](https://github.com/tsukumogami/tsuku/issues/344) | feat(website): style recipe detail pages | #341 |

### Phase 1: Extend JSON Schema

1. Modify `generate-registry.py` to extract `dependencies` and `runtime_dependencies` from TOML
2. Add validation: dependency arrays must be lists, each name matches `NAME_PATTERN`
3. Add validation: each dependency references an existing recipe (prevents broken links)
4. Update JSON output to include these fields (empty arrays if not present)
5. Bump `schema_version` to "1.1.0"

**Deliverable:** Extended recipes.json with dependency data

### Phase 2: Add Client-Side Router

1. Implement `getViewFromURL()` to parse current path
2. Implement `navigateTo()` using History API
3. Add `popstate` listener for browser navigation
4. Add `renderCurrentView()` dispatcher

**Deliverable:** URL-based view switching works

### Phase 3: Implement Detail View

1. Create `renderDetailView(recipeName)` function
2. Render recipe metadata (name, description, homepage)
3. Render install command with copy button
4. Render dependency lists (grouped by type)
5. Add back navigation link
6. Handle 404 for unknown recipes

**Deliverable:** Detail view renders correctly

### Phase 4: Update Grid Navigation

1. Modify recipe cards to use `navigateTo()` instead of href
2. Update card structure to be clickable
3. Maintain existing search/filter functionality

**Deliverable:** Grid cards navigate to detail views

### Phase 5: Add Redirect Rule

1. Add `/recipes/*` redirect to `_redirects`
2. Test direct URL navigation
3. Verify browser back/forward works

**Deliverable:** Direct links work correctly

### Phase 6: Style Detail Pages

1. Add CSS for `.recipe-detail` component
2. Style dependency lists
3. Ensure responsive layout
4. Match existing dark theme

**Deliverable:** Polished detail view styling

## Known Limitations

### Cloudflare Pages Splat Redirect Bug

Cloudflare Pages ignores splat patterns (like `/recipes/*`) in `_redirects` when deploying via direct upload through wrangler. This is a known bug: [cloudflare/workers-sdk#2671](https://github.com/cloudflare/workers-sdk/issues/2671).

**Impact:** The `_redirects` rule `/recipes/* /recipes/index.html 200` does not work with the GitHub Actions deployment workflow.

**Workaround implemented:**
1. A `404.html` file disables Cloudflare's implicit SPA fallback behavior
2. JavaScript in `404.html` detects `/recipes/<name>/` paths and redirects to `/recipes/?p=<path>`
3. The recipes SPA reads the `?p=` parameter and uses `history.replaceState` to restore the clean URL

**User experience:** Direct links to `/recipes/k9s/` briefly show the 404 page before redirecting. Navigation within the SPA (grid to detail, back button) works instantly without this workaround.

**Future resolution:** If Cloudflare fixes the bug, the workaround can be removed and the `_redirects` rule will work as intended.

## Consequences

### Positive

- **Dependency visibility**: Users can see all prerequisites before installing
- **Direct linking**: Tools can be referenced with shareable URLs
- **Architectural consistency**: Same client-rendering pattern as grid
- **Instant navigation**: No page reloads when switching between views
- **Simple deployment**: No generated files, just one HTML page

### Negative

- **No SEO**: Individual recipes not indexed by search engines
- **Requires JavaScript**: No content without JS (but grid already requires it)
- **Loading state on direct links**: Brief spinner before content appears

### Mitigations

- **SEO**: Not a priority - users search for "tsuku" not individual recipes
- **JavaScript requirement**: Existing `<noscript>` fallback links to GitHub
- **Loading state**: Minimal impact; same UX as current grid page

## Security Considerations

### Download Verification

**Not applicable** - This feature does not download or execute binaries. It displays metadata from `recipes.json`.

### Execution Isolation

**Not applicable** - Standard browser JavaScript execution with no elevated privileges.

### Supply Chain Risks

**Data source trust model:**

Recipe metadata originates from TOML files in the tsuku repository, controlled by:
1. Branch protection requiring PR review
2. Maintainer approval for recipe changes
3. Automated validation in CI

**Risk: Malicious recipe metadata could be rendered**

| Attack Vector | Likelihood | Impact | Mitigation |
|--------------|------------|--------|------------|
| XSS via recipe name/description | Very Low | High | Use `textContent`, never `innerHTML` |
| Phishing via homepage URL | Very Low | Medium | Validate HTTPS-only; use `rel="noopener noreferrer"` |
| Dependency name injection | Very Low | Low | Validate deps match `^[a-z0-9-]+$` in generator |

**Mitigations:**

1. **Safe DOM APIs**: All dynamic content rendered via `textContent`, `createElement`, `setAttribute` - never `innerHTML`
2. **URL validation**: Homepage URLs validated as HTTPS during JSON generation
3. **Dependency validation**: Generator validates dependency names match recipe name pattern
4. **Link attributes**: All external links use `target="_blank" rel="noopener noreferrer"`

### User Data Exposure

**Data accessed**: None. The page displays public recipe metadata only.

**Data transmitted**: Standard HTTP request for `recipes.json`. No cookies, localStorage, or analytics.

**Privacy implications**: None.
