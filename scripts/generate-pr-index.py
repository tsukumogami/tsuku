#!/usr/bin/env python3
"""Generate wip/pr-index.md: assigns every new recipe to a concrete PR.

Reads recipe files from the branch diff against origin/main, builds a
dependency graph, and assigns recipes to PRs following a mixed format:
  - 3-5 homebrew library deps per PR
  - 3-5 homebrew tools that test those deps
  - 6 cargo OR gem recipes

Overflow PRs (after cargo/gem exhausted) contain medium batches of
homebrew tools (~50 per PR).

Usage:
    python3 scripts/generate-pr-index.py
"""

import json
import os
import subprocess
import sys
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path


# ── Configuration ────────────────────────────────────────────────────

DEFERRED = {
    "git", "spatialite",           # sandbox dep resolution failures (#1964)
    "fontconfig", "proj",          # macOS rpath bug (#1965)
    "cuda-runtime",                # system package, pre-existing bugs
    "mesa-vulkan-drivers",         # Alpine arm64 pre-existing bug
    "vulkan-loader",               # system package
}

MAX_CARGO_PER_PR = 6
MAX_GEM_PER_PR = 6
LIBS_PER_PR = 5
TOOLS_PER_PR = 5
OVERFLOW_BATCH_SIZE = 50

REPO_ROOT = Path(__file__).resolve().parent.parent


# ── Data classes ─────────────────────────────────────────────────────

@dataclass
class Recipe:
    name: str
    path: str           # relative to repo root (e.g. recipes/g/glib.toml)
    is_library: bool
    action: str         # homebrew, cargo_install, gem_install, etc.
    dependencies: list  # [metadata].dependencies
    runtime_deps: list  # [metadata].runtime_dependencies
    priority: int = 3   # from priority queue (1=highest, 3=default)
    placed: bool = False

    @property
    def all_deps(self):
        return set(self.dependencies + self.runtime_deps)


@dataclass
class PR:
    number: int
    description: str
    branch: str
    libraries: list = field(default_factory=list)
    tools: list = field(default_factory=list)
    cargo: list = field(default_factory=list)
    gem: list = field(default_factory=list)

    @property
    def all_recipes(self):
        return self.libraries + self.tools + self.cargo + self.gem

    @property
    def total(self):
        return len(self.all_recipes)


# ── TOML parsing (minimal, no external deps) ────────────────────────

def parse_toml_metadata(filepath):
    """Extract metadata fields from a recipe TOML file.

    This is a minimal parser that handles the fields we need without
    requiring a toml library. It reads [metadata] section values for
    name, type, dependencies, and runtime_dependencies, plus scans
    [[steps]] sections for action types.
    """
    metadata = {
        "name": "",
        "type": "",
        "dependencies": [],
        "runtime_dependencies": [],
    }
    actions = []

    try:
        with open(filepath, "r") as f:
            lines = f.readlines()
    except (OSError, UnicodeDecodeError):
        return metadata, actions

    in_metadata = False
    current_section = None
    # Track multi-line arrays
    in_array = None  # field name if we're collecting a multi-line array
    array_buf = []

    for line in lines:
        stripped = line.strip()

        # Section headers
        if stripped.startswith("[[steps]]"):
            in_metadata = False
            current_section = "steps"
            if in_array:
                metadata[in_array] = _parse_array_items(array_buf)
                in_array = None
                array_buf = []
            continue
        if stripped.startswith("[metadata]"):
            in_metadata = True
            current_section = "metadata"
            continue
        if stripped.startswith("[") and not stripped.startswith("[["):
            if in_array:
                metadata[in_array] = _parse_array_items(array_buf)
                in_array = None
                array_buf = []
            in_metadata = stripped == "[metadata]"
            current_section = stripped.strip("[]")
            continue

        # Inside multi-line array
        if in_array:
            if "]" in stripped:
                # End of array - include content before ]
                array_buf.append(stripped.split("]")[0])
                metadata[in_array] = _parse_array_items(array_buf)
                in_array = None
                array_buf = []
            else:
                array_buf.append(stripped)
            continue

        # Inside [metadata]
        if in_metadata and "=" in stripped:
            key, _, val = stripped.partition("=")
            key = key.strip()
            val = val.strip()

            if key == "name":
                metadata["name"] = val.strip('"').strip("'")
            elif key == "type":
                metadata["type"] = val.strip('"').strip("'")
            elif key in ("dependencies", "runtime_dependencies"):
                if val.startswith("[") and "]" in val:
                    # Single-line array
                    metadata[key] = _parse_inline_array(val)
                elif val.startswith("["):
                    # Multi-line array starts
                    in_array = key
                    array_buf = [val.lstrip("[")]
                # else: unexpected format, skip

        # Inside [[steps]] - grab action
        if current_section == "steps" and "=" in stripped:
            key, _, val = stripped.partition("=")
            if key.strip() == "action":
                actions.append(val.strip().strip('"').strip("'"))

    # Flush any pending array
    if in_array:
        metadata[in_array] = _parse_array_items(array_buf)

    return metadata, actions


def _parse_inline_array(val):
    """Parse a single-line TOML array like '["foo", "bar"]'."""
    items = []
    content = val.strip("[]")
    for item in content.split(","):
        item = item.strip().strip('"').strip("'")
        if item:
            items.append(item)
    return items


def _parse_array_items(lines):
    """Parse items from multi-line TOML array content."""
    items = []
    for line in lines:
        for item in line.split(","):
            item = item.strip().strip('"').strip("'")
            if item and not item.startswith("#"):
                items.append(item)
    return items


# ── Git helpers ──────────────────────────────────────────────────────

def get_new_recipe_files():
    """Get list of new recipe files (added, not modified) vs origin/main."""
    result = subprocess.run(
        ["git", "diff", "--name-only", "--diff-filter=A", "origin/main", "--", "recipes/"],
        capture_output=True, text=True, cwd=REPO_ROOT,
    )
    return [f for f in result.stdout.strip().split("\n") if f]


def get_modified_recipe_files():
    """Get list of modified recipe files vs origin/main."""
    result = subprocess.run(
        ["git", "diff", "--name-only", "--diff-filter=M", "origin/main", "--", "recipes/"],
        capture_output=True, text=True, cwd=REPO_ROOT,
    )
    return [f for f in result.stdout.strip().split("\n") if f]


# ── Priority queue ───────────────────────────────────────────────────

def load_priority_map():
    """Load priority-queue.json, return {name: priority_level}."""
    queue_path = REPO_ROOT / "data" / "queues" / "priority-queue.json"
    if not queue_path.exists():
        return {}
    with open(queue_path) as f:
        data = json.load(f)
    return {e["name"]: e["priority"] for e in data.get("entries", [])}


# ── Recipe loading ───────────────────────────────────────────────────

def load_recipes(file_list, priority_map):
    """Parse recipe files and return list of Recipe objects."""
    recipes = []
    for rel_path in file_list:
        full_path = REPO_ROOT / rel_path
        if not full_path.exists():
            continue

        metadata, actions = parse_toml_metadata(full_path)
        name = metadata["name"]
        if not name:
            # Derive from path: recipes/x/foo.toml -> foo
            name = Path(rel_path).stem

        # Determine primary action
        action = "homebrew"  # default
        for a in actions:
            if a in ("cargo_install", "gem_install", "go_install", "go_build",
                      "pip_install", "pipx_install", "npm_install", "npm_exec"):
                action = a
                break

        recipes.append(Recipe(
            name=name,
            path=rel_path,
            is_library=(metadata["type"] == "library"),
            action=action,
            dependencies=metadata["dependencies"],
            runtime_deps=metadata["runtime_dependencies"],
            priority=priority_map.get(name, 3),
        ))
    return recipes


# ── Existing recipe names (already on main) ──────────────────────────

def get_existing_recipe_names():
    """Get set of recipe names that already exist on origin/main."""
    result = subprocess.run(
        ["git", "ls-tree", "--name-only", "-r", "origin/main", "recipes/"],
        capture_output=True, text=True, cwd=REPO_ROOT,
    )
    names = set()
    for line in result.stdout.strip().split("\n"):
        if line.endswith(".toml"):
            names.add(Path(line).stem)
    return names


# ── Dependency graph ─────────────────────────────────────────────────

def build_dep_graph(recipes, existing_names):
    """Build maps of which new recipes depend on which new libraries.

    Returns:
        new_lib_names: set of new library recipe names
        lib_dependents: {lib_name: [recipe_names that depend on it]}
        recipe_new_deps: {recipe_name: set of new lib names it depends on}
    """
    new_names = {r.name for r in recipes}
    new_lib_names = {r.name for r in recipes if r.is_library}

    lib_dependents = defaultdict(list)
    recipe_new_deps = {}

    for r in recipes:
        # Only track deps on NEW libraries (not existing ones)
        new_deps = r.all_deps & new_lib_names
        recipe_new_deps[r.name] = new_deps
        for dep in new_deps:
            lib_dependents[dep].append(r.name)

    return new_lib_names, lib_dependents, recipe_new_deps


def topo_sort_libs(lib_recipes, recipe_new_deps, lib_dependents):
    """Sort libraries so prereqs come first, with dependents close behind.

    Strategy: place libraries that others depend on first, then immediately
    follow with their dependents rather than draining all independent libs.
    Libraries with more tool dependents are prioritized within each group.
    """
    lib_by_name = {r.name: r for r in lib_recipes}
    lib_names = set(lib_by_name.keys())

    # Compute depth: how many layers of lib deps
    depth = {}
    def get_depth(name):
        if name in depth:
            return depth[name]
        lib_deps = recipe_new_deps.get(name, set()) & lib_names
        if not lib_deps:
            depth[name] = 0
            return 0
        d = 1 + max(get_depth(dep) for dep in lib_deps)
        depth[name] = d
        return d

    for name in lib_names:
        get_depth(name)

    # Group by: prereqs (depended on by other libs), their dependents, orphans
    prereqs = []    # depth 0, but other libs depend on them
    dependents = [] # depth > 0
    high_fan = []   # depth 0, no lib dependents, but have tool dependents
    orphans = []    # depth 0, no lib or tool dependents

    lib_children = defaultdict(list)  # lib -> list of libs that depend on it
    for r in lib_recipes:
        lib_deps = recipe_new_deps.get(r.name, set()) & lib_names
        for dep in lib_deps:
            lib_children[dep].append(r.name)

    for r in lib_recipes:
        d = depth.get(r.name, 0)
        tool_deps = len(lib_dependents.get(r.name, []))
        has_lib_children = r.name in lib_children

        if d == 0 and has_lib_children:
            prereqs.append(r)
        elif d > 0:
            dependents.append(r)
        elif tool_deps > 0:
            high_fan.append(r)
        else:
            orphans.append(r)

    # Sort each group by dependent count descending
    sort_key = lambda r: (-len(lib_dependents.get(r.name, [])), r.name)
    prereqs.sort(key=sort_key)
    dependents.sort(key=lambda r: (depth.get(r.name, 0), sort_key(r)))
    high_fan.sort(key=sort_key)
    orphans.sort(key=lambda r: r.name)

    # Interleave: prereqs first, then their dependents, then high-fan, then orphans
    # But spread dependents across multiple PRs by inserting them after a few
    # high-fan libs (so they don't all stack at the end)
    result = list(prereqs)       # glib, libevent first
    result.extend(high_fan[:5])  # first batch of high-fan libs
    result.extend(dependents)    # glib-dependent libs right after
    result.extend(high_fan[5:])  # remaining high-fan libs
    result.extend(orphans)       # orphans fill remaining slots

    return result


# ── PR assignment ────────────────────────────────────────────────────

def assign_recipes(recipes, existing_names):
    """Assign all recipes to PRs and return the list of PRs."""
    new_lib_names, lib_dependents, recipe_new_deps = build_dep_graph(recipes, existing_names)
    recipe_by_name = {r.name: r for r in recipes}

    # Classify
    libs = [r for r in recipes if r.is_library and r.name not in DEFERRED]
    cargo = [r for r in recipes if r.action == "cargo_install" and r.name not in DEFERRED]
    gem = [r for r in recipes if r.action == "gem_install" and r.name not in DEFERRED]
    homebrew_tools = [
        r for r in recipes
        if not r.is_library
        and r.action not in ("cargo_install", "gem_install")
        and r.name not in DEFERRED
    ]

    # Sort libraries by dependency order + dependent count
    libs_sorted = topo_sort_libs(libs, recipe_new_deps, lib_dependents)

    # Sort cargo/gem by priority then name
    cargo.sort(key=lambda r: (r.priority, r.name))
    gem.sort(key=lambda r: (r.priority, r.name))

    # Sort tools: those with new-lib deps first (they need matching), then by priority
    def tool_sort_key(r):
        has_new_deps = 1 if recipe_new_deps.get(r.name, set()) else 2
        return (has_new_deps, r.priority, r.name)
    homebrew_tools.sort(key=tool_sort_key)

    prs = []
    pr_num = 2
    placed_libs = set()  # lib names placed in this or earlier PRs

    # Track what's been placed
    lib_queue = list(libs_sorted)
    cargo_queue = list(cargo)
    gem_queue = list(gem)
    tool_queue = list(homebrew_tools)

    # ── Phase 1: Mixed PRs with cargo ────────────────────────────────
    while cargo_queue:
        pr = _create_mixed_pr(
            pr_num, "cargo", lib_queue, tool_queue, cargo_queue, None,
            placed_libs, recipe_new_deps, new_lib_names, lib_dependents,
        )
        prs.append(pr)
        pr_num += 1

    # ── Phase 2: Mixed PRs with gem ──────────────────────────────────
    while gem_queue:
        pr = _create_mixed_pr(
            pr_num, "gem", lib_queue, tool_queue, None, gem_queue,
            placed_libs, recipe_new_deps, new_lib_names, lib_dependents,
        )
        prs.append(pr)
        pr_num += 1

    # ── Phase 3: Remaining libs get their own mixed PRs with tools ───
    while lib_queue:
        pr = PR(
            number=pr_num,
            description=f"Library batch {pr_num}",
            branch=f"backfill/pr{pr_num}-libs",
        )

        # Pick libs whose deps are satisfied
        added = 0
        for lib in list(lib_queue):
            if added >= LIBS_PER_PR:
                break
            deps_on_new_libs = recipe_new_deps.get(lib.name, set()) & new_lib_names
            if deps_on_new_libs <= placed_libs:
                pr.libraries.append(lib)
                lib_queue.remove(lib)
                placed_libs.add(lib.name)
                added += 1

        # If we couldn't place any (circular or stuck), force-place
        if added == 0 and lib_queue:
            lib = lib_queue.pop(0)
            pr.libraries.append(lib)
            placed_libs.add(lib.name)

        # Add matching tools
        _add_matching_tools(pr, tool_queue, placed_libs, recipe_new_deps, new_lib_names, TOOLS_PER_PR)

        pr.description = _pr_description(pr)
        pr.branch = f"backfill/pr{pr_num}-libs-{_pr_suffix(pr)}"
        prs.append(pr)
        pr_num += 1

    # ── Phase 4: Overflow homebrew tool batches ──────────────────────
    # Sort remaining tools by priority
    tool_queue.sort(key=lambda r: (r.priority, r.name))
    batch_num = 1
    while tool_queue:
        pr = PR(
            number=pr_num,
            description=f"Homebrew tools batch {batch_num}",
            branch=f"backfill/pr{pr_num}-tools-{batch_num}",
        )
        batch = tool_queue[:OVERFLOW_BATCH_SIZE]
        tool_queue = tool_queue[OVERFLOW_BATCH_SIZE:]
        pr.tools = batch
        prs.append(pr)
        pr_num += 1
        batch_num += 1

    return prs


def _create_mixed_pr(pr_num, ecosystem, lib_queue, tool_queue,
                     cargo_queue, gem_queue, placed_libs,
                     recipe_new_deps, new_lib_names, lib_dependents):
    """Create one mixed PR with libs + tools + cargo or gem."""
    pr = PR(number=pr_num, description="", branch="")

    # Pick libs whose deps are satisfied
    added = 0
    for lib in list(lib_queue):
        if added >= LIBS_PER_PR:
            break
        deps_on_new_libs = recipe_new_deps.get(lib.name, set()) & new_lib_names
        if deps_on_new_libs <= placed_libs:
            pr.libraries.append(lib)
            lib_queue.remove(lib)
            placed_libs.add(lib.name)
            added += 1

    # Add matching tools (prefer those that depend on just-placed libs)
    _add_matching_tools(pr, tool_queue, placed_libs, recipe_new_deps, new_lib_names, TOOLS_PER_PR)

    # Add cargo or gem
    if ecosystem == "cargo" and cargo_queue:
        batch = cargo_queue[:MAX_CARGO_PER_PR]
        del cargo_queue[:MAX_CARGO_PER_PR]
        pr.cargo = batch
    elif ecosystem == "gem" and gem_queue:
        batch = gem_queue[:MAX_GEM_PER_PR]
        del gem_queue[:MAX_GEM_PER_PR]
        pr.gem = batch

    pr.description = _pr_description(pr)
    eco_tag = "cargo" if ecosystem == "cargo" else "gem"
    batch_idx = pr.number - 1  # PR 2 = batch 1
    pr.branch = f"backfill/pr{pr_num}-{eco_tag}-{_pr_suffix(pr)}"
    return pr


def _add_matching_tools(pr, tool_queue, placed_libs, recipe_new_deps,
                        new_lib_names, max_tools):
    """Add tools whose new-lib deps are all satisfied."""
    # First pass: tools that depend on libs in this PR
    pr_lib_names = {lib.name for lib in pr.libraries}
    added = 0

    # Prefer tools that specifically test this PR's libraries
    for tool in list(tool_queue):
        if added >= max_tools:
            break
        new_deps = recipe_new_deps.get(tool.name, set()) & new_lib_names
        if new_deps and new_deps <= placed_libs and new_deps & pr_lib_names:
            pr.tools.append(tool)
            tool_queue.remove(tool)
            added += 1

    # Second pass: tools that depend on any already-placed libs
    for tool in list(tool_queue):
        if added >= max_tools:
            break
        new_deps = recipe_new_deps.get(tool.name, set()) & new_lib_names
        if new_deps and new_deps <= placed_libs:
            pr.tools.append(tool)
            tool_queue.remove(tool)
            added += 1

    # Third pass: independent tools (no new-lib deps) - fill remaining slots
    for tool in list(tool_queue):
        if added >= max_tools:
            break
        new_deps = recipe_new_deps.get(tool.name, set()) & new_lib_names
        if not new_deps:
            pr.tools.append(tool)
            tool_queue.remove(tool)
            added += 1


def _pr_description(pr):
    """Generate a short description for the PR."""
    parts = []
    if pr.libraries:
        lib_names = [lib.name for lib in pr.libraries[:3]]
        if len(pr.libraries) > 3:
            lib_names.append("...")
        parts.append(", ".join(lib_names))
    if pr.cargo:
        parts.append(f"{len(pr.cargo)} cargo recipes")
    if pr.gem:
        parts.append(f"{len(pr.gem)} gem recipes")
    if pr.tools:
        parts.append(f"{len(pr.tools)} tools")
    return " + ".join(parts) if parts else "recipes"


def _pr_suffix(pr):
    """Short suffix for branch name."""
    if pr.libraries:
        return pr.libraries[0].name.replace("-", "")[:8]
    if pr.tools:
        return pr.tools[0].name.replace("-", "")[:8]
    return str(pr.number)


# ── Validation ───────────────────────────────────────────────────────

def validate(prs, all_recipes, existing_names):
    """Validate the PR assignments. Returns list of error strings."""
    errors = []
    new_lib_names = {r.name for r in all_recipes if r.is_library}

    # Check every non-deferred recipe is placed exactly once
    placed = {}
    for pr in prs:
        for r in pr.all_recipes:
            if r.name in placed:
                errors.append(
                    f"DUPLICATE: {r.name} in PR {placed[r.name]} and PR {pr.number}"
                )
            placed[r.name] = pr.number

    for r in all_recipes:
        if r.name not in DEFERRED and r.name not in placed:
            errors.append(f"MISSING: {r.name} not assigned to any PR")

    # Check dependency ordering
    libs_available = set(existing_names)
    for pr in sorted(prs, key=lambda p: p.number):
        # Add this PR's libraries to available set
        pr_lib_names = {lib.name for lib in pr.libraries}
        libs_available |= pr_lib_names

        for r in pr.all_recipes:
            new_deps = r.all_deps & new_lib_names
            unsatisfied = new_deps - libs_available
            if unsatisfied:
                errors.append(
                    f"DEP VIOLATION: {r.name} (PR {pr.number}) depends on "
                    f"{unsatisfied} not yet available"
                )

    # Check cargo/gem limits
    for pr in prs:
        if len(pr.cargo) > MAX_CARGO_PER_PR:
            errors.append(f"PR {pr.number}: {len(pr.cargo)} cargo recipes (max {MAX_CARGO_PER_PR})")
        if len(pr.gem) > MAX_GEM_PER_PR:
            errors.append(f"PR {pr.number}: {len(pr.gem)} gem recipes (max {MAX_GEM_PER_PR})")

    return errors


# ── Output ───────────────────────────────────────────────────────────

def write_index(prs, all_recipes, deferred_recipes, output_path):
    """Write the PR index to a markdown file."""
    total_mixed_cargo = sum(1 for pr in prs if pr.cargo)
    total_mixed_gem = sum(1 for pr in prs if pr.gem)
    total_overflow = sum(1 for pr in prs if not pr.cargo and not pr.gem and not pr.libraries)
    total_lib_only = sum(1 for pr in prs if pr.libraries and not pr.cargo and not pr.gem)
    total_recipes = sum(pr.total for pr in prs)

    lines = [
        "# PR Index: System Library Backfill",
        "",
        "Generated by `scripts/generate-pr-index.py`.",
        f"Source branch: `docs/system-lib-backfill`",
        "",
        "## Summary",
        "",
        "| Phase | PRs | Description |",
        "|-------|-----|-------------|",
        f"| Mixed (cargo) | {total_mixed_cargo} | 5 libs + 5 tools + 6 cargo each |",
        f"| Mixed (gem) | {total_mixed_gem} | 5 libs + 5 tools + 6 gem each |",
    ]
    if total_lib_only:
        lines.append(f"| Library batches | {total_lib_only} | Remaining libs + matching tools |")
    lines.extend([
        f"| Overflow | {total_overflow} | ~{OVERFLOW_BATCH_SIZE} homebrew tools each |",
        f"| **Total** | **{len(prs)}** | **{total_recipes} recipes** |",
        "",
    ])

    if deferred_recipes:
        lines.extend([
            "## Deferred recipes",
            "",
            "These recipes are excluded until their blocking issues are resolved:",
            "",
        ])
        for r in sorted(deferred_recipes, key=lambda r: r.name):
            lines.append(f"- `{r.path}` ({r.name})")
        lines.append("")

    lines.extend([
        "---",
        "",
    ])

    for pr in prs:
        lines.append(f"## PR {pr.number}: {pr.description}")
        lines.append("")
        lines.append(f"Branch: `{pr.branch}`")
        lines.append("")

        if pr.libraries:
            lines.append(f"### Libraries ({len(pr.libraries)})")
            lines.append("")
            for r in pr.libraries:
                dep_note = ""
                deps = r.all_deps
                if deps:
                    dep_note = f" (deps: {', '.join(sorted(deps))})"
                lines.append(f"- `{r.path}`{dep_note}")
            lines.append("")

        if pr.tools:
            lines.append(f"### Homebrew tools ({len(pr.tools)})")
            lines.append("")
            for r in pr.tools:
                notes = []
                if r.priority < 3:
                    notes.append(f"P{r.priority}")
                new_deps = r.all_deps & {lib.name for lib in pr.libraries}
                if new_deps:
                    notes.append(f"tests: {', '.join(sorted(new_deps))}")
                note_str = f" ({', '.join(notes)})" if notes else ""
                lines.append(f"- `{r.path}`{note_str}")
            lines.append("")

        if pr.cargo:
            lines.append(f"### Cargo recipes ({len(pr.cargo)})")
            lines.append("")
            for r in pr.cargo:
                note = f" (P{r.priority})" if r.priority < 3 else ""
                lines.append(f"- `{r.path}`{note}")
            lines.append("")

        if pr.gem:
            lines.append(f"### Gem recipes ({len(pr.gem)})")
            lines.append("")
            for r in pr.gem:
                note = f" (P{r.priority})" if r.priority < 3 else ""
                lines.append(f"- `{r.path}`{note}")
            lines.append("")

        lines.append("---")
        lines.append("")

    with open(output_path, "w") as f:
        f.write("\n".join(lines))


# ── Main ─────────────────────────────────────────────────────────────

def main():
    os.chdir(REPO_ROOT)

    print("Loading priority queue...")
    priority_map = load_priority_map()

    print("Finding new recipe files...")
    new_files = get_new_recipe_files()
    print(f"  {len(new_files)} new recipe files")

    modified_files = get_modified_recipe_files()
    print(f"  {len(modified_files)} modified recipe files")

    print("Loading existing recipe names from main...")
    existing_names = get_existing_recipe_names()
    print(f"  {len(existing_names)} recipes on main")

    print("Parsing recipe files...")
    recipes = load_recipes(new_files, priority_map)
    print(f"  {len(recipes)} recipes parsed")

    # Separate deferred
    deferred = [r for r in recipes if r.name in DEFERRED]
    active = [r for r in recipes if r.name not in DEFERRED]

    # Stats
    libs = [r for r in active if r.is_library]
    cargo = [r for r in active if r.action == "cargo_install"]
    gem = [r for r in active if r.action == "gem_install"]
    tools = [r for r in active if not r.is_library and r.action not in ("cargo_install", "gem_install")]
    print(f"\nClassification:")
    print(f"  Libraries:  {len(libs)}")
    print(f"  Cargo:      {len(cargo)}")
    print(f"  Gem:        {len(gem)}")
    print(f"  Other tools:{len(tools)}")
    print(f"  Deferred:   {len(deferred)}")

    print("\nAssigning recipes to PRs...")
    prs = assign_recipes(active, existing_names)
    print(f"  {len(prs)} PRs created")

    print("\nValidating...")
    errors = validate(prs, active, existing_names)
    if errors:
        print(f"\n  {len(errors)} validation errors:")
        for e in errors:
            print(f"    {e}")
    else:
        print("  All checks passed.")

    # Print summary
    print("\nPR summary:")
    for pr in prs:
        parts = []
        if pr.libraries:
            parts.append(f"{len(pr.libraries)}L")
        if pr.tools:
            parts.append(f"{len(pr.tools)}T")
        if pr.cargo:
            parts.append(f"{len(pr.cargo)}C")
        if pr.gem:
            parts.append(f"{len(pr.gem)}G")
        print(f"  PR {pr.number:3d}: {'+'.join(parts):>12s} = {pr.total:3d}  {pr.description}")

    output_path = REPO_ROOT / "wip" / "pr-index.md"
    print(f"\nWriting {output_path}...")
    write_index(prs, active, deferred, output_path)
    print("Done.")

    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
