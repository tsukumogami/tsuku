# Crystallize: extract-symlink-escape

## Chosen artifact type

**Design Doc**, carried through to a **Plan**.

## Scoring

| Type | Score | Reasoning |
|---|---|---|
| **Design Doc** | **Strong** | "What to build" was never in question -- the issue states it. "How" was, and exploration resolved it by eliminating four alternatives with measured evidence. The framework's documentation-purpose rule is decisive: `wip/` is cleaned before merge, so the mechanism comparison, the traversal-vs-link-content distinction, and the reason `securejoin` and `EvalSymlinks` were rejected would be permanently lost. A future contributor asking "why does extraction go through `os.Root` instead of just validating link targets?" needs that on record. |
| Plan | Adequate, but not first | Signals fit ("work is understood well enough to break into issues", "exploration confirmed scope and approach"), but its precondition -- "an existing PRD or design doc covers this topic" -- is unmet. Plan follows the design, it does not replace it. |
| PRD | No | Requirements arrived as input. The issue already carries acceptance criteria and a verified reproduction. |
| No Artifact | No | Explicit anti-signal: architectural decisions were made during exploration. Also multi-file, security-sensitive, and needs a reviewer to follow the reasoning. |
| Rejection Record | No | Exploration reached a build conclusion, not a rejection. |

## Decisions that must survive to a permanent document

1. Traversal is the security boundary; link content is policy. These are separate rules
   and conflating them is what produced both #2473 and #2275.
2. `os.Root` over `EvalSymlinks`, `securejoin`, two-pass validation, and
   staging-then-verify -- with the concrete failure of each, not just the winner.
3. `validateSymlinkTarget` is deliberately retained as policy, so this change stays
   strictly additive on security and leaves #2275 a separate decision.
4. Three code paths share the defect, including `app_bundle.go`.
5. Compatibility was measured (146 archives / 49,590 symlink entries / zero traversals),
   not assumed.

## Scope boundary carried into design

In: `extract.go` tar + zip loops, `app_bundle.go` zip-with-symlinks loop, the two
`SECURITY:` comments, regression tests, `plugins/tsuku-recipes/` skill documentation.

Out: #2275, the recipe-controlled `dest` parameter, zip's symlink flattening, the
shared-`pathsafe`-helper refactor for the four divergent containment checks elsewhere in
the tree.

## Handoff

Proceed to `/scope` with this exploration as input. Design altitude first, then Plan as
the terminal artifact. BRIEF and PRD altitudes are not warranted -- the problem framing
and the requirements both already exist in tsukumogami/tsuku#2473.
