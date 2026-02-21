# MM16b: needsDesign class must have needs-design label

A node has the `needsDesign` class but the corresponding GitHub issue does not have the `needs-design` label.

## Why This Matters

Label and Mermaid class must agree bidirectionally. If the diagram shows `needsDesign` but the issue lacks the label, the diagram is out of sync with the actual issue state. The issue may have already progressed to `tracks-design` or had its label removed entirely.

## How to Fix

Either add the `needs-design` label to the GitHub issue:

```bash
gh issue edit 123 --add-label "needs-design"
```

Or change the class to match the issue's actual state:

```
class I123 ready
```

## Related Rules

- MM16a: needs-design label must use needsDesign class
- MM16c: tracks-design label must use tracksDesign class
- MM16d: tracksDesign class must have tracks-design label
