# Data Directory

Static data files used by batch recipe generation infrastructure.

## Files

### dep-mapping.json

Maps ecosystem-specific dependency names to tsuku recipe names. Used by the batch pipeline to validate `blocked_by` entries in failure records.

**Structure:**

```json
{
  "<ecosystem>": {
    "<ecosystem-dep-name>": "<tsuku-recipe-name>"
  }
}
```

- Top-level keys are ecosystem names (e.g., `homebrew`)
- Values are either a tsuku recipe name or `"pending"` if no recipe exists yet
- Mappings are code-reviewed as a supply chain control point

**Maintenance:**

- Add new entries when the batch pipeline encounters unmapped dependencies
- Change `"pending"` to a recipe name when the corresponding recipe is added
- Multiple ecosystem names can map to the same tsuku recipe (e.g., `sqlite3` and `sqlite` both map to `sqlite`)
