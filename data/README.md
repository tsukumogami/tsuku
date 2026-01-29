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

### schemas/

JSON Schema (draft-07) definitions for data files:

- **priority-queue.schema.json** - Validates `data/priority-queue.json`. Defines the package queue structure with tiered priority (1-3), status tracking, and source metadata.
- **failure-record.schema.json** - Validates `data/failures/*.json`. Defines per-ecosystem-environment failure records with categorized failure types and optional `blocked_by` dependencies.

### examples/

Sample data files demonstrating valid structure:

- **priority-queue.json** - Example priority queue with packages at each tier level
- **failure-record.json** - Example failure records showing different failure categories
