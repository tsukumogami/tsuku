# MM16d: tracksDesign class must have tracks-design label

A node has the `tracksDesign` class but the corresponding GitHub issue does not have the `tracks-design` label.

## Why This Matters

Label and Mermaid class must agree bidirectionally. If the diagram shows `tracksDesign` but the issue lacks the label, the diagram is out of sync with the actual issue state. The issue may still be in `needs-design` or may have had its label removed.

## How to Fix

Either add the `tracks-design` label to the GitHub issue:

```bash
gh issue edit 123 --add-label "tracks-design"
```

Or change the class to match the issue's actual state:

```
class I123 needsDesign
```

## Related Rules

- MM16a: needs-design label must use needsDesign class
- MM16b: needsDesign class must have needs-design label
- MM16c: tracks-design label must use tracksDesign class
