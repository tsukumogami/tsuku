# tsuku Website

Static website for tsuku package manager, deployed via Cloudflare Pages.

## Site Structure

- **Main site**: `/` - Landing page with installation command and feature overview
- **Install endpoint**: `/install.sh` - Installation script
- **Coverage Dashboard**: `/coverage/` - Recipe platform coverage visualization
- **Pipeline Dashboard**: `/pipeline/` - Batch recipe generation tracking
- **Recipe Directory**: `/recipes/` - Browse available recipes

## Coverage Dashboard

The coverage dashboard shows which recipes support which platforms (glibc, musl, darwin).

- **View the dashboard**: `/coverage/`
- **Documentation**: [`docs/coverage-dashboard.md`](docs/coverage-dashboard.md)
- **Regenerate locally**: `go run cmd/coverage-analytics/main.go`

### Quick Reference

The dashboard provides three views:
1. **Coverage Matrix**: Sortable table showing all recipes and their platform support
2. **Gap List**: Recipes with incomplete platform support
3. **Category Breakdown**: Platform coverage statistics by recipe type (library vs tool)

Coverage data is automatically updated by CI whenever recipe files change. See the [full documentation](docs/coverage-dashboard.md) for details on how the system works.

## Development

Local development requires no build step - just serve the files:

```bash
# Python
python3 -m http.server 8000

# Node.js
npx serve
```

Then visit `http://localhost:8000` in your browser.

## Deployment

The site is deployed via Cloudflare Pages from the `main` branch. Changes merged to `main` are automatically deployed.

## Key Files

| File | Purpose |
|------|---------|
| `index.html` | Landing page |
| `install.sh` | Installation script (synced from repo root) |
| `assets/style.css` | Dark theme styles |
| `coverage/` | Coverage dashboard (HTML + JSON data) |
| `pipeline/` | Pipeline dashboard (HTML + JSON data) |
| `_redirects` | Cloudflare Pages routing |
| `_headers` | Custom HTTP headers |

## Documentation

- [`docs/coverage-dashboard.md`](docs/coverage-dashboard.md) - Coverage dashboard user guide
