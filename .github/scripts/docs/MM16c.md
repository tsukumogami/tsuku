# MM16c: tracks-design label must use tracksDesign class

An issue with the `tracks-design` label has a class other than `tracksDesign` in the diagram. The label and class must agree.

## Why This Matters

The `tracks-design` label indicates the issue has spawned a child design whose implementation is in progress. The Mermaid class must reflect this so the diagram accurately shows the issue's state.

## How to Fix

Change the class to `tracksDesign`:

**Before (incorrect):**
```
class I123 ready
```

**After (correct):**
```
class I123 tracksDesign
```

## Related Rules

- MM16a: needs-design label must use needsDesign class
- MM16b: needsDesign class must have needs-design label
- MM16d: tracksDesign class must have tracks-design label
